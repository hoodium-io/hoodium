# EVM transaction mechanism

How a transaction travels through Hoodium, from the wallet's signature to a
committed balance — and, specifically, how the EVM layer's view of an account's
balance is reconciled with `x/bank`, which is the authoritative ledger.

This document is about the **mechanism**. For the JSON-RPC method surface see
[`evm-compatibility.md`](./evm-compatibility.md); for connecting a wallet see
[`evm-wallets.md`](./evm-wallets.md); for the fee floors see
[`tx-fees.md`](../tx-fees.md).

---

## 1. The central fact: two ledgers, one authority

Hoodium is a Cosmos SDK application with an EVM execution environment
(`x/evm`) bolted onto the same state. That produces two views of "an account
balance":

| View | Owner | Storage | Authority |
|---|---|---|---|
| **Bank balance** | `x/bank` | `arune` balance per address | **authoritative** |
| **EVM balance** | `x/evm/statedb` | in-memory `stateObject`, journalled per tx | cache, reconciled at commit |

The EVM executes against its own in-memory state objects because that is what
geth's semantics require: a transaction must be able to read its own uncommitted
writes, and a reverted call must undo them. But that memory is **transient**. It
is discarded at the end of a transaction, and the only durable record is the bank
balance.

Everything below follows from that. The whole design problem is: *how do we turn
a bag of absolute final balances in memory into correct, supply-preserving
movements in the authoritative ledger?*

---

## 2. Lifecycle of a transaction

### 2.1 Submission — `eth_sendRawTransaction`

1. The wallet builds and signs the transaction **locally**. The node never sees
   the private key.
2. `SendRawTransaction` (`rpc/backend/call_tx.go`) decodes the payload into an
   `MsgEthereumTx` and submits it to the app's mempool.
3. The tx is wrapped with the `ExtensionOptionsEthereumTx` option. That marker is
   what routes it to the EVM ante chain rather than the Cosmos one — see §2.2.

`eth_sendTransaction` (node-held key) is also supported, but the wallet-signs
model is the normal path.

### 2.2 Mempool admission — the ante handler

`NewAnteHandler` (`app/ante/ante.go`) inspects the extension option and picks one
of **two entirely separate decorator chains**:

| Extension option | Chain | Used for |
|---|---|---|
| `/ethermint.evm.v1.ExtensionOptionsEthereumTx` | **EVM** (`newEVMAnteHandler`) | every EVM tx |
| `/ethermint.types.v1.ExtensionOptionDynamicFeeTx` | Cosmos (`newCosmosAnteHandler`) | Cosmos tx opting into 1559 |
| *(none)* | Cosmos (`newCosmosAnteHandler`) | `MsgSend`, `MsgDelegate`, … |

The EVM chain, in order (`app/ante/handler_options.go:84-105`):

```
EthSetUpContextDecorator        → infinite gas meter, per-EVM gas accounting
EthMempoolFeeDecorator          → node's minimal-gas-prices
EthMinGasPriceDecorator         → x/feemarket MinGasPrice floor
EthDeploymentGasPriceDecorator  → higher floor for contract creation
EthValidateBasicDecorator
EthSigVerificationDecorator
EthAccountVerificationDecorator → account exists / nonce matches
CanTransferDecorator            → balance covers value + gas
EthGasConsumeDecorator          → DEDUCTS THE FEE from the bank
EthIncrementSenderSequenceDecorator
GasWantedDecorator
EthEmitEventDecorator
```

Two things to note, because they shape what the commit has to do later:

- **The fee is deducted here, in the ante, straight from the bank balance.** It
  is not deducted during EVM execution and it is not deducted at commit.
- **The refund is *not* given here.** Unused gas is refunded only after
  execution, in `RefundGas` (§2.5), because until the EVM has run nobody knows how
  much gas was actually used.

Admission is `CheckTx`: the tx enters the mempool only if this chain passes. It
will be re-run at delivery.

### 2.3 Block construction and delivery

Normal CometBFT flow — nothing EVM-specific:

1. **`PrepareProposal`** — the proposer reaps the mempool.
2. **`PreBlock`** (`app/app.go:709`) — Hoodium records the count of *regular*
   transactions for PoNA, stripping the injected vote-extension pseudo-tx.
3. **`FinalizeBlock`** — `DeliverTx` for each tx; the ante chain runs again, then
   the message handler executes.
4. **`EndBlock`** — `x/pona` pays the block reward (§5).

### 2.4 Execution — `ApplyTransaction`

`MsgEthereumTx` routes to `Keeper.ApplyTransaction`
(`x/evm/keeper/state_transition.go`), which is the single choke point **every**
EVM transaction passes through — `eth_sendRawTransaction`, internal calls, and
simulation:

1. Build a `statedb.StateDB` over the current `sdk.Context`.
2. `applyMessageWithConfig` creates the EVM and executes the call/create. Every
   balance change lands in the StateDB's in-memory `stateObject`s and its
   journal; nothing has touched `x/bank` yet.
3. VM errors do **not** abort the tx — they mark the receipt `status: 0x0`. The
   state changes are still committed at step 4 (the fee is kept, the state is
   not).
4. `stateDB.Commit()` is called if `commit == true` — see §2.6.

### 2.5 Gas refund

`RefundGas` (`x/evm/keeper/gas.go`) returns the *unused* portion of the gas that
was charged up front:

```go
SendCoinsFromModuleToAccount(ctx, authtypes.FeeCollectorName, msg.From.Bytes(), refundedCoins)
```

This is a **module → account transfer**, never a mint. It has to be, because the
ante already took the full `gasLimit × price` out of the account and the fee
collector is holding it.

### 2.6 Commit — where EVM state becomes bank state

`StateDB.commit()` (`x/evm/statedb/statedb.go`) collects every dirty account into
one batch and hands it to the keeper as a single `CommitAccounts` call:

```go
dirties := s.journal.sortedDirties()
updates := make([]AccountUpdate, 0, len(dirties))
// ... for each dirty object: code, storage writes, and an AccountUpdate ...

if err := s.keeper.CommitAccounts(ctx, updates); err != nil {
    return errorsmod.Wrap(err, "failed to set account")
}
```

The batch is the important part, and it is the subject of §3.

### 2.7 Mempool update

CometBFT's `Update` removes the committed tx from the mempool, **keeps** every
uncommitted one, and re-runs `CheckTx` on them against the new state
(`recheckTxs`). Invalid txs are dropped from the cache and may be resubmitted.

> **There is no TTL.** An uncommitted, still-valid transaction stays in the
> mempool indefinitely. Hoodium raises the size to `10000`
> (`cmd/runed/mainchain.go:1240`) so a burst of EVM traffic survives a slow
> block; everything else is stock CometBFT (`type = flood`, `recheck = true`).

---

## 3. Balance settlement — the core mechanism

### 3.1 What the EVM gives us

The EVM reports **absolute final balances**. It has no notion of "this account
gained 50 and that one lost 50"; it says "this account ends at 400, that one ends
at 600". So each account's delta has to be recovered by comparison against the
authoritative bank balance.

### 3.2 The naive approach, and why it breaks

Settling each account independently means: delta > 0 → mint, delta < 0 → burn.
That turns a plain value transfer — one account up, one account down, **supply
unchanged** — into a mint/burn pair.

This works only while supply has headroom. The devnet genesis emits the entire
10B RUNE emission up front (`premine + 1B reward reserve + 500k bonded pool`), so
supply sits exactly at `runetoken.MaxSupply`, headroom is zero, and the mint half
of *every* transfer is rejected:

```
failed to commit stateDB: failed to set account: mint rejected:
projected supply of denom arune would exceed max supply
```

The transaction reverts even though it created no coins. This was a real bug; it
is fixed by batch settlement.

### 3.3 Batch settlement — `CommitAccounts`

```go
func (k *Keeper) CommitAccounts(ctx sdk.Context, updates []statedb.AccountUpdate) error
```

1. For each account, recover its delta **against the live bank balance**, not
   against the balance the state object was loaded with. This matters because a
   precompile (e.g. the RUNE token) can move coins through `x/bank` *during*
   execution; those moves are already in the bank while the state object still
   holds the pre-call value.
2. `net = Σ credits − Σ debits`.
3. Mint **only if `net > 0`** — and therefore still subject to the supply cap.
4. Debits → EVM module account.
5. Credits ← EVM module account (`SendCoinsFromModuleToAccount`, so the bank's
   blocked-address rule still applies to EVM recipients).
6. Burn `−net` **only if `net < 0`**.
7. Account metadata (nonce, code hash) and self-destruct removal happen **last**,
   so a rejected balance cannot leave a half-updated account behind.

### 3.4 The invariant

| Transaction shape | Σ credits | Σ debits | net | Mint/burn |
|---|---|---|---|---|
| Value transfer | 50 | 50 | **0** | **none** |
| Contract payout to N recipients | 50 | 50 | **0** | none |
| Self-destruct with beneficiary | X | X | **0** | none |
| Self-destruct, no beneficiary | 0 | X | **−X** | burn X |
| Genuine inflation (module mint) | X | 0 | **+X** | mint X, capped |

**A transfer never needs headroom, because it never creates a coin.** The cap
still guards the only path that can create one.

> **Per commit, not necessarily per transaction.** A custom precompile
> mid-execution calls `CommitCacheContext`, which commits the deltas accumulated
> so far. Such a partial commit can be unbalanced and will mint or burn the
> difference; the surrounding cache context rolls it back if the precompile or the
> transaction fails. The guarantee is that a *balanced* set of deltas never
> touches supply.

### 3.5 Reconciliation across the ERC-20 boundary

`CommitAccounts` settles **only** `params.EvmDenom` (`arune`). Token balances are
not EVM-native balances — they live in `x/bank` under their own denom and are
moved by a plain `MsgSend` inside the precompile. `core/go-erc20/transfer.go`
makes this explicit: it writes EVM journal entries **only** when the precompile's
denom *is* the gas denom.

| Path | Ledger movement | Touches `arune` supply? |
|---|---|---|
| Native RUNE value transfer | `CommitAccounts` | no (net 0) |
| `RUNE.transfer()` / `transferFrom()` (precompile) | journal → `CommitAccounts` | no (net 0) |
| `HOODI.transfer()` or any ERC-20 | `MsgSend`, bank only | **no, ever** |
| Gas fee | ante `DeductFees`, bank only | no |
| Gas refund | `fee_collector → account` transfer | no |

That is why an ERC-20 payout never depends on `arune` headroom — and why HOODI
claims kept working while RUNE payouts were failing.


`eth_sendTransaction` (node-held key) is also supported, but the wallet-signs
model is the normal path.

---

## 4. Supply: who may create coins

A transfer must never mint. That makes it worth stating exactly which code paths
are *allowed* to create RUNE, and what guards each one.

| Path | Creates coins? | Guard |
|---|---|---|
| Genesis (`x/bank` `InitGenesis`) | writes supply directly | `ValidateGenesisSupply` in `InitChainer` |
| `CommitAccounts` net > 0 | yes | bank mint restriction (`app/mint_cap.go`) |
| `x/pona` reward (`FundRewardPool`) | yes, genesis-only | bank mint restriction |
| Slashing redirect (`app/slashingAdapter.go`) | yes | bank mint restriction |
| **Value transfer** | **no** | — |
| **ERC-20 / precompile transfer** | **no** | — |
| Gas fee / refund | no (deduct / transfer) | — |

### 4.1 The cap has exactly one enforcement point

The `maxSupply()` method on the RUNE and HOODI precompiles is declared
`core.Read` — it is a pure getter and enforces nothing. What enforces the cap is
the `MintingRestrictionFn` installed on the bank keeper at construction
(`app/mint_cap.go`), which the SDK invokes at the top of `MintCoins`, before any
state change.

### 4.2 Why genesis needs a separate check

`x/bank`'s `InitGenesis` writes balances and supply with a direct `setSupply`
call — it never goes through `MintCoins`, so it bypasses the restriction. That
makes genesis the only supply-writing path outside the cap.

`ValidateGenesisSupply` closes it: `InitChainer` validates the genesis bank
supply against both token caps before any module writes state, and **panics** on
excess.

A panic is the correct response rather than a returned error. An over-cap genesis
is not recoverable:

- `totalSupply()` would permanently report more than `maxSupply()`, and
- headroom would be negative, so **no mint could ever succeed again** — PoNA
  rewards included.

Sitting *exactly at* the cap is legal and is the intended fully-emitted state, so
the check rejects only a strict excess.

---

## 5. Where PoNA meets the EVM

PoNA (`x/pona`) is a *block* reward, not a transaction reward. The two systems
touch at exactly three points:

| Point | Direction | Mechanism |
|---|---|---|
| `PreBlock` | app → PoNA | counts **regular** txs (pseudo-tx stripped) for the activity band |
| `EndBlock` | PoNA → `fee_collector` | transfers the reward from the 1B reserve |
| `BeginBlock` (next block) | `x/distribution` → validators | `AllocateTokens` sweeps `fee_collector` |

Two consequences worth keeping in mind:

- **Transaction fees and PoNA rewards share one path.** Both land in
  `fee_collector` and are split by voting power in the same `AllocateTokens`
  call. Nothing distinguishes them downstream.
- **PoNA never mints at runtime.** `DistributeRuneBlockReward` is a
  `SendCoinsFromModuleToModule` out of the pre-funded reserve; `FundRewardPool`'s
  mint is genesis-only and unused on the devnet.
- **A transaction that fails still counts** toward the activity band. The count
  comes from the block's transaction list in `PreBlock`, independently of
  execution result.

---

## 6. Failure modes worth knowing

| Symptom | Real cause |
|---|---|
| `mint rejected: projected supply … would exceed max supply` on a **transfer** | pre-batch-settlement bug; a transfer was being minted. Fixed. |
| Same error on a **module mint** | genuine cap hit — the chain is at 10B and something tried to create coins. |
| `gas used 0` with a non-zero `gas wanted` | failure at **commit**, after the VM ran. Look at the commit error, not the VM. |
| A tx stuck in the mempool, never mined | still valid but never reaped; there is no TTL. Check `recheck` and the fee floor. |
| An ERC-20 payout failing while RUNE works | token/inventory issue in the *contract*, not the chain — ERC-20 never touches `arune` supply. |
| `totalSupply() > maxSupply()` | genesis was written above the cap. Since `ValidateGenesisSupply`, this fails at block 1 instead. |
