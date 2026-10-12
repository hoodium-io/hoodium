# Validator Setup

How to join an **official, already-running** Hoodium network as a validator.

> Running a node for the first time? Start with [Node Setup](./node-setup.md),
> which covers `runed setup` and its two modes.
>
> Launching a brand-new chain's genesis? That is a **team-only** action — see
> [Mainchain Genesis Initialization](./team/mainchain/genesis-init.md).

## ⚠️ First, the one thing that decides everything

There are **two different paths**, and picking the wrong one is the most common
mistake:

| Situation | What you need |
|---|---|
| The network **already exists** (genesis published, chain running) | **`create-validator`** — you join a live chain |
| You are **assembling the genesis** for a network that has not started yet | **`gentx`** — your validator is baked into genesis |

**If you are joining a live network, you do NOT create a gentx.** A gentx is a
genesis-time artifact: it is only meaningful before block 1. Once the chain is
running, your validator is added by broadcasting a `create-validator`
transaction.

> ⚠️ **Heads-up on `runed setup`:** in **automated** mode, `runed setup` runs
> `init` + `keys add` + **`gentx`**. That is correct **only** when you are
> contributing to a *not-yet-launched* genesis. When joining a **live** network,
> the gentx step is not what you want — use **manual** mode, or run the steps
> below yourself.

## Joining a live network — full guide

### Step 0 — get the network artifacts from the Hoodium team

You need three things:

1. **`genesis.json`** — the official genesis file. Its hash identifies the
   network.
2. The **chain ID** (e.g. `rune_6590-1`).
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
> no premine and no validators, and will not connect you to the network.

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
Acquire RUNE from the team / an existing holder — on a live network there is no
premine for you.

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

In a **second terminal** (the node keeps running):

```bash
runed tx staking create-validator \
  --amount 500000000000000000000000arune \
  --pubkey "$(runed tendermint show-validator --home ~/.runed)" \
  --moniker <your-moniker> \
  --commission-rate 0.05 \
  --commission-max-rate 0.20 \
  --commission-max-change-rate 0.01 \
  --min-self-delegation 500000000000000000000000 \
  --from <key-name> \
  --chain-id rune_6590-1 \
  --keyring-backend file \
  --home ~/.runed \
  --gas auto --gas-adjustment 1.3 \
  -y
```

### Step 7 — confirm you are in the active set

```bash
runed query staking validator $(runed keys show <key-name> --bech val -a --home ~/.runed) --home ~/.runed
runed query staking validators --home ~/.runed
```

Your validator is **bonded** once it is in the top **101** by voting power.

### What you need after `runed setup`

If you used `runed setup`, it ends by telling you to `runed start`. **That is not
the whole job** — depending on your situation:

| You used | Still to do |
|---|---|
| `runed setup` (manual or automated) **to join a live network** | Replace `genesis.json` with the official one, set peers, **fund the wallet**, then **`create-validator`** (Steps 1–7). The `gentx` it produced is **not** usable for a live chain. |
| `runed setup` **as part of a not-yet-launched genesis** | Give your gentx to whoever assembles the genesis; they run `collect-gentxs`. |

In short: **`runed setup` gets the node files in place; it does not, by itself,
make your validator live on an existing network.**

## Staking parameters (Hoodium)

| Parameter | Value |
|-----------|-------|
| Max validators | 101 |
| Min self-delegation | 500,000 RUNE |
| Min commission rate | 5% |
| **Unbonding time** | **14 days** |

> A minimum **delegation** of 0.1 RUNE is enforced at the message/precompile level
> (not a staking param).

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
- To unjail after downtime: `runed tx slashing unjail --from <key-name> --home ~/.runed`.
