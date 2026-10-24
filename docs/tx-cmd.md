# Sending transactions (`runed tx`)

Everything you can send from the command line: plain RUNE transfers, EVM
transactions, validator creation and delegation. If you only read one section,
read **Fees** — a wrong fee is the single most common reason a `runed tx` fails.

---

## Fees — read this first

Hoodium enforces a **global minimum gas price** of **20 × 10⁹ arune per gas**
(`x/feemarket` → `min_gas_price`). A transaction is rejected if:

```
fee  <  min_gas_price × gas
```

Because `1 RUNE = 10^18 arune`, that floor is tiny per gas but is multiplied by
the whole gas limit. A typical transaction costs a fraction of a RUNE:

| Transaction | Gas | Minimum fee |
|---|---|---|
| Simple transfer | 21,000 | 0.00042 RUNE |
| `bank send` (typical) | ~200,000 | 0.004 RUNE |
| `delegate` | ~250,000 | 0.005 RUNE |
| `create-validator` | ~300,000 | 0.006 RUNE |

### Always use `--gas-prices`, never a hardcoded `--fees`

```bash
--gas auto --gas-adjustment 1.3 --gas-prices 25000000000arune
```

That is `25 × 10⁹ arune` per gas = `0.000000025 RUNE` per gas, a **1.25×** margin
over the floor. With `--gas auto` the SDK:

1. **simulates** the transaction to learn the gas limit, then
2. multiplies that limit by `--gas-prices` to derive the fee.

So the fee always clears the floor and you never have to estimate gas yourself.

> ⚠️ **Use the BASE denom (`arune`), never `rune`, for `--gas-prices`.**
> `--gas-prices` is parsed **without** denom normalization (unlike `--fees`), so
> `0.000000025rune` stays a `rune`-denominated value. The ante handler builds its
> required fee in `arune`, so the two never compare equal and the transaction is
> rejected. Write the whole amount in `arune`.

> ⚠️ **A fixed `--fees` amount cannot do this.** The fee is compared against
> `min_gas_price × gas` before the message executes, so any amount below the
> floor is rejected outright. A hardcoded `--fees 1000arune` is
> **10⁻¹⁵ RUNE** — about **6 × 10¹² times too small** for a `create-validator`,
> and it will fail. If you prefer `--fees`, you must size it yourself:
> `min_gas_price × gas` or above.

**For validator creation, use a larger margin** — `runed setup` prints
`--gas-prices 50000000000arune` (2× the floor) because a failed
`create-validator` costs a full sync cycle.

Check the current floor at any time:

```bash
runed query feemarket params --home ~/.runed
```

---

## Addresses: bech32 and 0x

Hoodium accounts are **one account with two spellings of the same 20 bytes**:

| Form | Example | Used by |
|---|---|---|
| **bech32** | `rune1mk89p6yppzvm9zys7ccg8r2jwx92ak260yjp7a` | Cosmos CLI (`runed tx …`) |
| **EVM / 0x** | `0x3fC91A3afd70395Cd496C647d5a6CC9D4B2b7FAD` | EVM wallets, JSON-RPC, Solidity |

Both point at the **same balance**. Send RUNE to either and it arrives.

> **The Cosmos CLI accepts bech32 only.** `runed tx bank send` requires a
> `rune1…` recipient. Passing a `0x…` address there fails with an address-decode
> error. To move funds using a 0x address, use an **EVM transaction** instead
> (see [EVM transactions](#evm-transactions)).

---

## `runed tx bank send` — plain RUNE transfers

```
runed tx bank send [from_key_or_address] [to_address] [amount] [flags]
```

### What is `[from_key_or_address]`?

It is **either a key NAME in your keyring, or a bech32 `rune1…` address** —
*not* a 0x address.

| Value | Meaning |
|---|---|
| `mykey` | A key **name** in the keyring (`runed keys list`). The CLI looks it up and signs with it. |
| `rune1abc…` | A bech32 **address**. Must already exist in the keyring to sign; otherwise you need `--generate-only`. |

The `--from` flag is **ignored** for `bank send` — the first positional argument
*is* the sender (the CLI sets `--from` from it internally). This differs from
most other `runed tx` commands, where `--from` is the way to choose the signer.

### `[to_address]`

The **recipient**. For `bank send` this must be a **bech32 `rune1…` address**.
A 0x address is rejected.

### `[amount]`

Amount **plus denom**. `rune` is a registered display denom, so both of these are
the same amount:

```bash
1000rune                      # display denom
1000000000000000000000arune   # base denom (1 RUNE = 10^18 arune)
```

### Example — send 1000 RUNE

```bash
runed tx bank send mykey rune1dx67l23hz9l0k9hcher8xz04uj7wf3yuckl253 1000rune \
  --chain-id rune_6590-1 \
  --keyring-backend file \
  --home ~/.runed \
  --gas auto --gas-adjustment 1.3 --gas-prices 25000000000arune \
  -y
```

| Flag | Why |
|---|---|
| `--chain-id` | Must match the network, else the signature is rejected. |
| `--keyring-backend file` | Where the key lives. `file` is the Hoodium default (headless-safe). |
| `--home` | Node home directory (`~/.runed` by default). |
| `--gas auto` | Simulate to estimate gas. |
| `--gas-adjustment 1.3` | 30% headroom over the simulation. |
| `--gas-prices` | The fee floor (see [Fees](#fees--read-this-first)). |
| `-y` | Skip the interactive confirmation. |

### Multi-send (one tx, many recipients)

This is a **separate subcommand** — `bank send` takes exactly three positional
arguments, so passing several recipients to it is an error.

```bash
runed tx bank multi-send mykey \
  rune1recipientA... rune1recipientB... 1000rune \
  --gas auto --gas-adjustment 1.3 --gas-prices 25000000000arune -y
```

By default the amount is sent to **each** recipient. Add `--split` to divide it
**equally between** them instead.

### Check a balance

```bash
runed query bank balances $(runed keys show mykey -a --keyring-backend file --home ~/.runed) --home ~/.runed
```

---

## EVM transactions

EVM transactions are the way to move funds using a **0x address**, and the way
to call contracts and precompiles.

### Can `runed` send an EVM tx from a 0x address?

**Yes — but not with `bank send`.** The Cosmos CLI speaks bech32; EVM
transactions are broadcast as raw, already-signed Ethereum transactions. There
are two supported routes.

#### Route 1 — `runed tx evm raw` (CLI)

```
runed tx evm raw TX_HEX [flags]
```

`TX_HEX` is a **fully signed RLP Ethereum transaction** (hex). The node decodes
it, wraps it in `MsgEthereumTx`, and broadcasts it. The sender is whatever 0x
address signed it — the CLI never sees your key.

```bash
# Sign locally with cast (Foundry), ethers.js, viem, web3.py, etc., then:
runed tx evm raw 0x02f8...  --home ~/.runed --node tcp://localhost:26657 -y
```

Note that **no `--from`, `--fees` or `--gas` are needed**: the fee and gas are
already inside the signed transaction, and they are validated against the
**EVM** fee rules (the two-gas-price model), not the Cosmos floor.

#### Route 2 — JSON-RPC `eth_sendRawTransaction` (recommended)

This is what MetaMask and every EVM tool use. Point any wallet at the node and
send RUNE or ERC-20 tokens as you would on Ethereum.

| Setting | Value |
|---|---|
| RPC URL | `http://<node>:8545` |
| Chain ID | `6590` (mainnet) · `6591` (testnet) · `6592` (devnet) |
| Currency symbol | `RUNE` |
| Decimals | `18` |

```bash
# Example with cast
cast send 0xRecipient --value 1ether \
  --private-key $PRIV \
  --rpc-url http://localhost:8545
```

All standard methods are available: `eth_chainId`, `eth_blockNumber`,
`eth_getBalance`, `eth_estimateGas`, `eth_sendRawTransaction`,
`eth_getTransactionCount`, `eth_gasPrice`, `eth_getCode`, `eth_call`.

> ⚠️ **EVM fees use a different rule.** EVM transactions pay
> `max(base fee + priority tip, applicable minimum)` under EIP-1559, where the
> applicable minimum depends on whether the transaction **deploys a contract**
> (see [tx-fees](./features/tx-fees.md)). Wallets handle this automatically.

### Precompiles (chain-native features over EVM)

| Precompile | Address |
|---|---|
| RUNE token (ERC-20) | `0x19be000000000000000000000000000000000000` |
| HOODI token (ERC-20) | `0x19be000000000000000000000000000000000001` |
| Price oracle | `0x19be000000000000000000000000000000000002` |
| **Staking** | `0x19be000000000000000000000000000000000003` |

---

## Staking from an EVM wallet (the staking precompile)

`0x19be…0003` exposes delegation, undelegation, validator creation and unjail,
so an EVM-only user never needs the Cosmos CLI.

### `delegate(address validator, uint256 amount) → bool`

`amount` is in **arune**. The caller (`msg.sender`) is the delegator.

### `undelegate(address validator, uint256 amount) → bool`

Unbonding takes **14 days**; tokens stay slashable until it completes.

### `getDelegation(address delegator, address validator) → uint256`

Returns the delegated amount in arune.

### `unjail(address validator) → bool`

Recovery path for a jailed validator. **Only the validator's own operator** may
call it (the caller must equal `validator`). Tombstoned validators cannot be
unjailed.


### `createValidator(...) → bool`

Creates a validator whose **operator is the caller** (`msg.sender`), so an
account can never create a validator it does not control.

```solidity
function createValidator(
    bytes   consensusPubKey,          // 32-byte ed25519 consensus public key
    uint256 amount,                   // self-delegation, in arune
    string  moniker,                  // display name
    uint256 commissionRate,           // fixed-point, 18 decimals (1e18 = 100%)
    uint256 commissionMaxRate,        // fixed-point, 18 decimals
    uint256 commissionMaxChangeRate,  // fixed-point, 18 decimals
    uint256 minSelfDelegation         // in arune
) external returns (bool);
```

Get `consensusPubKey` from your node:

```bash
runed tendermint show-validator --home ~/.runed
# -> {"@type":"/cosmos.crypto.ed25519.PubKey","key":"oWg2ISpLF405Jcm2vXV+2v4fnjodh6aafuIdeoW+rUw="}
```

The `key` value is **base64**; decode it to 32 bytes before passing it to
Solidity (`ethers.getBytes("0x" + Buffer.from(key,'base64').toString('hex'))`).

**Commission values are fixed-point with 18 decimals** — pass `5e16` for 5%,
`25e16` for 25%. The chain enforces a **5% minimum commission rate**.

Example (ethers.js):

```js
const staking = new ethers.Contract(
  "0x19be000000000000000000000000000000000003",
  ["function createValidator(bytes,uint256,string,uint256,uint256,uint256,uint256) returns (bool)"],
  signer
);

await staking.createValidator(
  consensusPubKeyBytes,            // 32 bytes
  ethers.parseUnits("500000", 18), // 500,000 RUNE self-delegation
  "my-validator",
  ethers.parseUnits("0.05", 18),   // 5% commission
  ethers.parseUnits("0.25", 18),   // 25% max
  ethers.parseUnits("0.01", 18),   // 1% max change
  ethers.parseUnits("500000", 18)  // min self-delegation
);
```

All preconditions (duplicate key, bond denom, description length, pubkey type,
minimum commission, min-self-delegation ≤ amount) are enforced by the staking
module itself — identical to the `MsgCreateValidator` transaction path.

---

## Validator creation from the Cosmos CLI

Equivalent to the precompile method above, using a JSON file.

```bash
runed tx staking create-validator validator.json \
  --from mykey \
  --keyring-backend file \
  --home ~/.runed \
  --chain-id rune_6590-1 \
  --gas auto --gas-adjustment 1.3 --gas-prices 50000000000arune \
  -y
```

`validator.json`:

```json
{
  "pubkey": {"@type":"/cosmos.crypto.ed25519.PubKey","key":"oWg2ISpLF405Jcm2vXV+2v4fnjodh6aafuIdeoW+rUw="},
  "amount": "500000000000000000000000arune",
  "moniker": "my-validator",
  "commission-rate": "0.05",
  "commission-max-rate": "0.25",
  "commission-max-change-rate": "0.01",
  "min-self-delegation": "500000000000000000000000"
}
```

> ⚠️ **`--amount`, `--pubkey` and `--moniker` are NOT flags.** In Cosmos SDK
> v0.50 these values come from the JSON file only. `runed setup` writes this file
> for you.

See [validator.md](./validator.md) for the full onboarding flow.

---

## Delegating from the Cosmos CLI

```bash
runed tx staking delegate <validator-operator-address> 1000rune \
  --from mykey --keyring-backend file --home ~/.runed \
  --gas auto --gas-adjustment 1.3 --gas-prices 25000000000arune -y
```

Get the operator address (note the `--bech val` — validators use a different
prefix from accounts):

```bash
runed keys show mykey --bech val -a --keyring-backend file --home ~/.runed
```

> ⚠️ **The 0.1 RUNE minimum delegation is enforced by the staking PRECOMPILE, not
> by this CLI command.** Cosmos `x/staking` has no min-delegation parameter, and
> the precompile is the only place Hoodium checks it. A `delegate` issued through
> the CLI therefore is **not** subject to that floor. Use the precompile
> (`0x19be…0003`) if you want the minimum enforced; the CLI is a raw staking
> entry point.

Unbonding takes **14 days**.

---

## Useful queries

```bash
# Fee floor and current base fee
runed query feemarket params --home ~/.runed
runed query feemarket base-fee --home ~/.runed

# Account balance (bech32)
runed query bank balances rune1... --home ~/.runed

# Validators / your delegations
runed query staking validators --home ~/.runed
runed query staking delegations rune1... --home ~/.runed

# Confirm a transaction landed
runed query tx <TX_HASH> --home ~/.runed
```

---

## Troubleshooting

| Symptom | Cause | Fix |
|---|---|---|
| `fee not provided…` / `insufficient fees; got: … required: …` | Fee below `min_gas_price × gas` | Use `--gas-prices 25000000000arune` with `--gas auto`. |
| `invalid transaction type` | Pre-`tx`-command binary, or a Cosmos msg the node cannot route | Rebuild (`make install`). |
| `decoding bech32 failed` / address error | A `0x` address passed to a Cosmos command | Use bech32 (`rune1…`) for `runed tx`, or send via EVM instead. |
| `key not found` | Key name/backend/home mismatch | Check `runed keys list --keyring-backend <b> --home <h>`. |
| `account sequence mismatch` | Another tx from the same account is pending | Wait for it to be included, or use `--sequence`. |
| `out of gas` | Gas limit too low | Use `--gas auto` (or raise `--gas-adjustment`). |
| `commission cannot be less than min commission rate` | `commission-rate` below 5% | Use `≥ 0.05` (or `≥ 5e16` via the precompile). |

---

## Related documentation

- [validator.md](./validator.md) — full validator onboarding
- [node-setup.md](./node-setup.md) — running a node
- [features/tx-fees.md](./features/tx-fees.md) — the two-gas-price model (EVM)
- [evm-wallets.md](./evm-wallets.md) — connecting EVM wallets
- [precompile.md](./precompile.md) — precompile reference

