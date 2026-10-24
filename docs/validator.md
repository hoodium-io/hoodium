# Validator Setup

How to join an **official, already-running** Hoodium network as a validator.

> Running a node for the first time? Start with [Node Setup](./node-setup.md),
> which covers `runed setup` and its two modes.

## ✅ There is only ONE path — `create-validator`

Joining a Hoodium network is always the same action: **run a node against the
official genesis, then broadcast a `create-validator` transaction.**

## Joining a live network — full guide

### Step 0 — get the network artifacts from the Hoodium team

You need three things:

1. **`genesis.json`** — the official genesis file. Its hash identifies the
   network.
2. The **chain ID** (e.g. `rune_6592-1`).
3. The **seed / persistent peer** addresses.

**Get the official genesis** by cloning the chains repository:

```bash
git clone https://github.com/hoodium-io/chains.git
```

The Mainchain genesis lives at:

```
chains/mainchain/genesis.json
```

> The `hoodium-io/chains` repository publishes the canonical genesis file per
> network. **Never** use the default genesis that `runed init` generates — it has
> no validators, and will not connect you to the network.

### Step 1 — initialise the node with the official genesis

```bash
runed init <your-moniker> --chain-id rune_6590-1 --home ~/.runed
```

Then **replace** the generated default genesis with the official one:

```bash
cp chains/mainchain/genesis.json ~/.runed/config/genesis.json
```

(Adjust the source path to wherever you cloned `hoodium-io/chains`.)

**Verify the genesis hash** against the value published by the team before
continuing — if it does not match, you are on the wrong network.

### Step 2 — configure peers

Add the seeds/persistent peers the team published, in `~/.runed/config/config.toml`:

```toml
persistent_peers = "<nodeID>@<host>:26656,..."
seeds = "<nodeID>@<host>:26656,..."
```

### Step 3 — create (or restore) your wallet key

```bash
runed keys add <key-name> --keyring-backend file --home ~/.runed
# or, to restore an existing key:
runed keys add <key-name> --recover --keyring-backend file --home ~/.runed
```

**Back up the mnemonic.** This account funds your self-delegation.

### Step 4 — fund the wallet

Your wallet needs at least **500,000 RUNE** for the self-delegation (plus gas).
Acquire RUNE from DEXs, CEXs etc...

```bash
runed query bank balances <your-rune1-address> --home ~/.runed
```

### Step 5 — start the node and let it sync

```bash
runed start --home ~/.runed
```

Watch it catch up:

```bash
runed status --home ~/.runed        # look at latest_block_height / catching_up
```

**Let it fully sync before creating the validator** — otherwise your
`create-validator` tx may fail or be created against a stale view.

### Step 6 — create the validator

`create-validator` takes a **JSON file**, not inline flags. Build the file, then
submit it.

**6a — get your consensus public key.** This prints a JSON object (your node's
CometBFT validator key, in the form the next step needs):

```bash
runed tendermint show-validator --home ~/.runed
```

It looks like:

```json
{"@type":"/cosmos.crypto.ed25519.PubKey","key":"oWg2ISpLF405Jcm2vXV+2v4fnjodh6aafuIdeoW+rUw="}
```

**6b — write `validator.json`** with that object in the `pubkey` field:

```json
{
  "pubkey": {"@type":"/cosmos.crypto.ed25519.PubKey","key":"oWg2ISpLF405Jcm2vXV+2v4fnjodh6aafuIdeoW+rUw="},
  "amount": "500000000000000000000000arune",
  "moniker": "your-moniker",
  "commission-rate": "0.05",
  "commission-max-rate": "0.25",
  "commission-max-change-rate": "0.01",
  "min-self-delegation": "500000000000000000000000"
}
```

| Field | Meaning |
|---|---|
| `pubkey` | Your consensus key, exactly as `show-validator` prints it |
| `amount` | Self-delegation, in **base denom** (`arune`). Minimum 500,000 RUNE. |
| `moniker` | Your validator's display name |
| `commission-rate` | Initial commission — must be **≥ 5%** (chain minimum) |
| `commission-max-rate` | Upper bound the rate can ever reach |
| `commission-max-change-rate` | Largest single rate change |
| `min-self-delegation` | Minimum self-bond; must be ≤ `amount` |

Optional fields: `identity`, `website`, `security`, `details`.

**6c — submit it** (in a second terminal, while the node runs and is synced):

```bash
runed tx staking create-validator validator.json \
  --from <key-name> \
  --chain-id rune_6590-1 \
  --keyring-backend file \
  --home ~/.runed \
  --gas auto --gas-adjustment 1.3 --fees 1000arune \
  -y
```

> ⚠️ **The `--amount` / `--pubkey` / `--moniker` flags do not exist.** In Cosmos SDK
> v0.50 those values are read from the JSON file only; passing them as flags is
> rejected. Keep the file — it documents the terms you bonded under.

### Step 7 — confirm you are in the active set

```bash
runed query staking validator $(runed keys show <key-name> --bech val -a --home ~/.runed) --home ~/.runed
runed query staking validators --home ~/.runed
```

Your validator is **bonded** once it is in the top **101** by voting power.

### What you need after `runed setup`

`runed setup` prepares the **node files** (`init` + your wallet key). It does
**not** make you a live validator. You still must, in order:

1. Replace the placeholder `genesis.json` with the **official** one
2. Set the published seeds / `persistent_peers`
3. **Fund your wallet** (≥ 500,000 RUNE)
4. Start the node and let it **fully sync**
5. Broadcast **`create-validator`**

That is Steps 1–7 above. There is **no gentx** anywhere in this flow.

## Staking parameters (Hoodium)

| Parameter | Value |
|-----------|-------|
| Max validators | 101 |
| Min self-delegation | 500,000 RUNE |
| Min commission rate | 5% |
| Max commission rate | 25% |
| **Unbonding time** | **14 days** |

> A minimum **delegation** of 0.1 RUNE is enforced at the **staking precompile**
> (`core/staking`). Cosmos staking has no min-delegation parameter, so Hoodium
> checks it at the precompile, which is the entry point delegation flows through.

### About the unbonding time

Hoodium sets **14 days**, applied explicitly in the genesis (rather than
inheriting the Cosmos default of 21 days).

**This single value covers both:**
- a **validator** unbonding (de-registering / losing bonded status), and
- a **delegator** undelegating their stake.

> ⚠️ Cosmos staking exposes **one** unbonding parameter. There is **no separate
> delegator timeframe** — the SDK applies the same period to delegations. So a
> delegator cannot have a shorter window than a validator without forking the
> staking module.

During the unbonding period the tokens are **locked and still slashable**; they
return to the account once the period elapses.

## Operating notes

- The bootstrap validator has **no special role** once others are bonded: if it
  goes down the chain keeps running, provided less than one third of voting power
  is offline.
- Only a validator's **public consensus key** is public; `config/priv_validator_key.json`
  never leaves your node. Back it up — losing it means you cannot sign.

## Unjailing after downtime

If your validator misses too many blocks it is **jailed** and stops earning. Once
the jail period has elapsed, unjail it.

**There is no `runed tx slashing unjail` command** — Hoodium exposes slashing
actions through the EVM, not as native Cosmos transactions. Call the **staking
precompile's `unjail` method** instead.

| | |
|---|---|
| Precompile address | `0x19be000000000000000000000000000000000003` |
| Method | `unjail(address validator)` |
| Argument | The validator's **operator address** (the `0x…` form of your `runevaloper1…`) |
| Caller | **Must be the operator itself** — the precompile rejects any other sender |

Because the caller must be the operator, call it **from the same EVM account that
owns the validator** (the account holding your self-delegation). An operator
address and its account address are the same 20 bytes, so the check is exact.

```javascript
// viem / ethers — send from the operator account
const abi = ["function unjail(address validator) returns (bool)"];
await walletClient.writeContract({
  address: "0x19be000000000000000000000000000000000003",
  abi,
  functionName: "unjail",
  args: ["0x<validator-operator-address>"],
});
```

> The method simply forwards to `x/slashing`'s unjail, so the usual preconditions
> apply: the jail period must have elapsed, and your self-delegation must still be
> at or above `min-self-delegation`. Calling it early fails with a clear error.
