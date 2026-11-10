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
runed setup
```

`runed setup` initialises the node home (genesis placeholder, config, node keys),
creates your wallet key, and writes `validator.json` for you. Answer the prompts:

| Prompt | Answer |
|---|---|
| Mode | `m` (manual) or `a` (automated) |
| Network | `mainnet` · `testnet` · `devnet` |
| Node type | `v` (validator) |
| Moniker | **Type your own** — a short, recognisable node name. Do **not** accept the machine hostname: on a VPS that is often long and meaningless (e.g. `ip-10-0-1-23.eu-west-1.compute.internal`). |
| Wallet key name | Press **Enter** to accept your moniker, or type a different key name |

> ⚠️ **The moniker is your node's public display name** and it is written into
> `config.toml` and `validator.json`. It is *not* the wallet key name, though
> reusing the same name for both is the recommended convention.

Then **replace** the generated default genesis with the official one:

```bash
git clone https://github.com/hoodium-io/chains
cp chains/mainchain/<network>/genesis.json ~/.runed/config/genesis.json
```

`<network>` is the one you selected (`mainnet`, `testnet` or `devnet`).

**Verify the genesis hash** against the value published by the team before
continuing — if it does not match, you are on the wrong network.

### What you need after `runed setup`

`runed setup` prepares the **node files** (`init` + your wallet key). It does
**not** make you a live validator. You still must, in order:

1. Replace the placeholder `genesis.json` with the **official** one
2. Set the published seeds / `persistent_peers`
3. **Fund your wallet** (≥ 500,001 RUNE): <br>
Your account has ONE balance reachable as both its 0x and rune1 
address. Export the key once and import it into an EVM wallet,
then send RUNE to that address from anywhere - cmd:
```
runed keys unsafe-export-eth-key test-node \
--keyring-backend file --home /home/muroko/.runed
```
4. Start the node and let it **fully sync**
5. Broadcast **`create-validator`**

That is Steps 1–7 above. There is **no gentx** anywhere in this flow.

### Step 2 — configure peers

Add the seeds/persistent peers the team published, in `~/.runed/config/config.toml`.
They are the raw peer list for your network, published in the
`hoodium-io/chains` repository:

| Network | Seeds file |
|---|---|
| **devnet** | https://github.com/hoodium-io/chains/mainchain/devnet/seeds.txt |
| **testnet** | https://github.com/hoodium-io/chains/mainchain/testnet/seeds.txt |
| **mainnet** | https://github.com/hoodium-io/chains/mainchain/mainnet/seeds.txt |

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

`create-validator` takes a **JSON file**, not inline flags. `runed setup` already
generated it for you at **`~/.runed/validator.json`**, containing your consensus
pubkey and the terms below. Edit it first if you want a different moniker or amount.

| Field | Meaning |
|---|---|
| `pubkey` | Your consensus key, taken from `config/priv_validator_key.json` |
| `amount` | Self-delegation, in **base denom** (`arune`). Minimum 500,000 RUNE. |
| `moniker` | Your validator's display name |
| `commission-rate` | Initial commission — must be **≥ 5%** (chain minimum) |
| `commission-max-rate` | Upper bound the rate can ever reach |
| `commission-max-change-rate` | Largest single rate change |
| `min-self-delegation` | Minimum self-bond; must be ≤ `amount` |

Optional fields: `identity`, `website`, `security`, `details`.

**Submit it** (in a second terminal, while the node runs and is synced):

```bash
runed tx staking create-validator validator.json \
  --from <key-name> \
  --gas auto --gas-adjustment 1.3 --gas-prices 25000000000arune \
  -y
```

> **`--home`, `--keyring-backend` and `--chain-id` are omitted on purpose.** All three
> are already in `~/.runed/config/client.toml` (written when you ran `runed init`), and
> every CLI command falls back to it. Passing `--chain-id` **overrides** that file: if
> the value does not match your node you get
> `signature verification failed … please verify chain-id (rune_…-1)` and the tx never
> reaches a block. Copy-pasting `rune_6590-1` (mainnet) onto a devnet/testnet node is
> the usual cause — let the config file supply it instead.

> **Why `--gas-prices` and not `--fees`?** The chain rejects any transaction whose
> fee is below `min_gas_price × gas`. With `--gas auto` the SDK first *simulates* the
> gas limit, then multiplies it by `--gas-prices` — so the fee always clears the floor
> and you never have to guess the gas. A fixed `--fees` amount cannot do that, and any
> amount below the floor is rejected outright before the message runs.
>
> `25000000000arune` is **1.25× the Cosmos-path floor of 20 × 10⁹ arune per gas**.
> At ~300,000 gas that is ~0.0075 RUNE. If you prefer a wider margin for a one-shot
> operation like this, `50000000000arune` (2×) also works.
>
> ⚠️ This value is tuned for **Cosmos CLI transactions** (`runed tx …`), which are
> checked against `min_gas_price`. **EVM transactions** (`eth_sendRawTransaction`, raw
> `runed tx evm raw`) are checked against the higher `min_regular_gas_price`
> (`2 500 000 000 000` arune/gas) and a `25 × 10⁹` price is **100× too low** there —
> see [tx-fees.md](./tx-fees.md). Wallets handle the EVM side; you only need this for
> `runed tx`.
>
> ⚠️ **The denom must be `arune`, not `rune`.** `--gas-prices` is parsed without
> denom normalization, so `rune`-denominated values never match the `arune`
> requirement and are rejected.
>
> The old `--fees 1000arune` (= 10⁻¹⁵ RUNE) was ~6 × 10¹² times too small.
>
> Run `runed query feemarket params --home ~/.runed` to read the current floor.

> ⚠️ **The `--amount` / `--pubkey` / `--moniker` flags do not exist.** In Cosmos SDK
> v0.50 those values are read from the JSON file only; passing them as flags is
> rejected. Keep the file — it documents the terms you bonded under.

### Step 7 — confirm you are in the active set

```bash
runed query staking validator $(runed keys show <key-name> --bech val -a --home ~/.runed) --home ~/.runed
runed query staking validators --home ~/.runed
```

Your validator is **bonded** once it is in the top **101** by voting power.

## Querying your validator

Every command below is read-only. `--home ~/.runed` and `--node tcp://localhost:26657`
are assumed; add them if your home or RPC address differ.

> Your validator's **JSON-RPC (8545) and REST API (1317) are disabled by default** —
> see [Node profiles](#node-profiles-what-runs-on-your-node). All queries here go
> through the **CometBFT RPC on 26657**, which stays bound to `127.0.0.1`.

### Identity

```bash
runed tendermint show-node-id     # node ID  — the <nodeID> in persistent_peers
runed tendermint show-address     # consensus address (runevalcons1…) — used by signing-info
runed tendermint show-validator   # consensus ed25519 pubkey
runed keys show <key-name> --bech val -a   # operator address (runevaloper1…)
```

### Liveness and jail risk — the most important checks

```bash
# Live status: height, catching_up, voting power.
runed status

# Signing info: the direct answer to "am I about to be jailed?"
runed query slashing signing-info $(runed tendermint show-address)
```

`signing-info` returns `index_offset` (blocks tracked in the current window) and
`missed_blocks_counter`. With a **10,000-block** window at 50% required, you are
jailed once `missed_blocks_counter` exceeds **5,000**. At 6s blocks the window is
~16.7h, so ~8.3h of downtime jails you.

```bash
runed query slashing signing-infos   # every validator at once
runed query slashing params          # window size, slash fractions, jail duration
```

> `Enumerable` missed-block bitmaps are **not** exposed by the gRPC query — only the
> counters above. `missed_blocks_counter` is the number that matters.

### Validator set

```bash
runed query staking validators                     # all validators, full detail
runed query staking validator <runevaloper…>       # just yours
runed query staking params                         # max validators, bond denom, unbonding time
runed query staking pool                           # bonded / not-bonded totals
runed query comet-validator-set                    # consensus set + voting power (CometBFT view)
runed query comet-validator-set 12345              # at a specific height
```

### Delegations and rewards

```bash
runed query staking delegations <rune1…>                    # your delegations
runed query staking delegations-to <runevaloper…>           # everyone delegating to you
runed query staking unbonding-delegations-from <runevaloper…>
runed query distribution rewards <rune1…>                   # claimable rewards
runed query distribution commission <runevaloper…>          # your commission
runed query distribution validator-outstanding-rewards <runevaloper…>
runed query distribution slashes <runevaloper…> 0 <height>  # needs start+end height
```

> 📘 **Full rewards reference:** [rewards-cmd.md](./rewards-cmd.md) — every query, how to withdraw
> commission vs delegator rewards, the `--commission` gotcha, and where Hoodium's block rewards
> actually come from (`x/pona` → distribution fee collector).

### Balance and blocks

```bash
runed query bank balances <rune1…>
runed query bank balance <rune1…> arune
runed query block                 # latest block
runed query block 12345           # a specific height
runed query tx <HASH>
```

### Raw CometBFT RPC

Useful for monitoring scripts, since it returns JSON:

```bash
curl -s localhost:26657/status                | jq '.result.validator_info, .result.sync_info'
curl -s localhost:26657/validators            | jq '.result.validators'
curl -s localhost:26657/net_info              | jq '.result.n_peers'
curl -s localhost:26657/dump_consensus_state  | jq '.result.round_state.height'
```

`dump_consensus_state` is the quickest way to see a **stalled** chain: a height that
stops incrementing with `step` stuck at `RoundStepPropose` means validators are not
reaching agreement.

### Node profiles — what runs on your node

`runed setup` writes one of two profiles, chosen by your `v` (validator) or `s`
(seed) answer. **Validator is the default** and is deliberately locked down:

| Service | Port | Validator | Seed |
|---|---|---|---|
| p2p | 26656 | open | open |
| CometBFT RPC | 26657 | `127.0.0.1` only | `0.0.0.0` |
| REST API | 1317 | **disabled** | enabled |
| EVM JSON-RPC | 8545 / 8546 | **disabled** | enabled |
| telemetry | (scraped via 26657) | enabled | enabled |

A validator needs none of the disabled services: `create-validator` broadcasts over
**26657**, not REST, and dedicated public API / JSON-RPC nodes serve those endpoints.
Telemetry stays on because it opens no listening socket — it is scraped over the
loopback RPC path — and it is what lets you see uptime before a missed-block window
jails you.

To re-enable a service on a node you intend to publish, edit `~/.runed/config/app.toml`:

```toml
[api]
enable = true
address = "tcp://0.0.0.0:1317"

[json-rpc]
enable = true
address = "0.0.0.0:8545"
ws-address = "0.0.0.0:8546"
```

Only do this on a node you intend to expose — and firewall the ports accordingly.

## Staking parameters (Hoodium)

| Parameter | Value |
|-----------|-------|
| Max validators | 101 |
| Min self-delegation | 500,000 RUNE |
| Min commission rate | 5% |
| Max commission rate | 25% |
| **Unbonding time** | **14 days** |

> A minimum **delegation** of 0.1 RUNE is enforced at **both** delegation entry
> points: the staking precompile (`core/staking`, for EVM wallets) and the Cosmos
> ante chain (`app/ante/cosmos.MinDelegationDecorator`, for `runed tx staking
> delegate`). Cosmos staking has no min-delegation parameter, so Hoodium checks
> it itself — both paths read the same `types.MinDelegation` value, so neither can
> be used to bypass the floor.

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

## Staking your validator — delegate, unbond, redelegate

These three commands all operate on **stake (tokens)**, never on a validator's identity.

> **`delegate` vs `redelegate` — they are not interchangeable.**
>
> | Command | Moves | When to use |
> |---|---|---|
> | `delegate` | **liquid** tokens (your wallet balance) → a validator | you hold RUNE and want to stake it |
> | `redelegate` | **illiquid** stake, validator → validator | the stake is *already delegated* and you want it elsewhere |
>
> `redelegate` cannot pull coins from a liquid balance — there is nothing to *re*-delegate. It is
> specifically a **validator-to-validator** move, which is why it skips the 14-day unbonding that a
> `unbond` → `delegate` round trip would require. The CLI help says it plainly: *"Redelegate an
> amount of **illiquid** staking tokens from one validator to another."*

```bash
# Add stake to a validator (self-delegation when signing with the validator's own key)
runed tx staking delegate <valoper-addr> <amount>rune \
  --from <key> --gas auto --gas-adjustment 1.3 --gas-prices 25000000000arune -y

# Remove stake (enters the 14-day unbonding queue)
runed tx staking unbond <valoper-addr> <amount>rune \
  --from <key> --gas auto --gas-adjustment 1.3 --gas-prices 25000000000arune -y

# Move stake between validators — INSTANT, no unbonding
runed tx staking redelegate <src-valoper> <dst-valoper> <amount>rune \
  --from <key> --gas auto --gas-adjustment 1.3 --gas-prices 25000000000arune -y

# Reverse a pending unbond
runed tx staking cancel-unbond <valoper-addr> <amount>rune \
  --from <key> --gas auto --gas-adjustment 1.3 --gas-prices 25000000000arune -y
```

- `--from` is the **delegator**. Using the validator's own key makes it a self-delegation.
- Get a valoper from `runed query staking validators` (`operator_address`).
- **Minimum delegation is 0.1 RUNE**, enforced by Hoodium (`types/coin.go`), since the SDK
  has no such parameter. Applied at both the CLI and the staking precompile.
- `redelegate` cannot target a validator that is itself still mid-redelegation
  (`ErrBadRedelegationDst`).

> **`redelegate` moves STAKE, not a validator.** Delegating to validator B makes B
> richer; it does **not** move validator A's identity, moniker, commission or consensus
> key to B. To move a *validator* to a new server, see
> [Relocating Validator](#relocating-validator).

### How much stake do you actually need?

`500,000 RUNE` is a **Hoodium CLI default** (`cmd/runed/setup.go`), not a chain rule.
The real constraints are:

| Requirement | Value |
|---|---|
| Consensus power ≥ 1 | **1 RUNE** (power = `floor(tokens / 10^18)`) |
| `min_self_delegation` | **your choice**, must be **≤** your bonded tokens |
| To be in the active set | must outrank others (max **101** validators) |
| Minimum commission rate | **5%** (chain-wide parameter) |

`min_self_delegation` is a **per-validator field**, not a chain parameter — set it in
`validator.json` (or change it later with `edit-validator --min-self-delegation`).

### Voting-power arithmetic — when does the chain halt?

CometBFT requires signatures from **strictly more than** `floor(total_power * 2/3)`;
it fails with `ErrNotEnoughVotingPowerSigned` when the tally is `<=` that threshold
(`cometbft/types/validation.go`). This is why equally-staked sets are unforgiving:

| Validators @ 500k | Total power | Needed (must exceed) | If 1 dies | Survives? |
|---|---|---|---|---|
| 1 | 500,000 | 333,333 | 0 | ❌ halts |
| 2 | 1,000,000 | 666,666 | 500,000 | ❌ halts |
| **3** | 1,500,000 | 1,000,000 | **1,000,000** | ❌ **halts — fails by exactly 0** |
| **4** | 2,000,000 | 1,333,333 | 1,500,000 | ✅ survives |

**With equal stakes you need 4 validators to tolerate one failure**, because 3 lands
exactly on the 2/3 boundary and the comparison is strict. General rule: a set tolerates
`floor((N-1)/3)` failures.

**Unequal stake is the cheaper way to get fault tolerance with few validators.** If one
validator holds more than 2/3 of a set-minus-one, it alone can carry consensus:

| Split | Needed | Losing the small one | Losing the big one |
|---|---|---|---|
| 1,100,000 + 500,000 | > 1,066,666 | ✅ 1,100,000 carries | ❌ halts |
| 700,000 + 700,000 | > 933,333 | ❌ halts | ❌ halts |

So `1.1M + 500k` lets you retire the 500k node — but the 1.1M node becomes a **single
point of failure**. That is asymmetric tolerance, not true redundancy.

> **A single validator never "proved" 1-of-1 is safe.** One validator that is *up* signs
> 100% of blocks, so the threshold is always met. The +2/3 rule only bites once someone
> *stops* signing — with one validator, "one down" means zero signatures and an immediate
> halt.

## Relocating Validator

There is **no consensus-key rotation**: `MsgEditValidator` carries only description,
commission and min-self-delegation — **there is no pubkey field** in SDK v0.50. A validator
*is* its key pair, so relocating one is never a plain staking operation.

That leaves **two valid approaches**, and the right one depends on whether you have external
delegators. Pick deliberately — this is a **tradeoff, not a single right answer**.

**Approach B is the ONLY safe option on a production chain with external delegators.** Approach A
is included because it is correct when the validator has *zero* external stake — otherwise it
strands them (see the warning below).

| | **A. Redelegate** (no external delegators only) | **B. Relocate the key** (production-safe) |
|---|---|---|
| Consensus key over the wire | ✅ never leaves the old box | ❌ plaintext transfer |
| Double-sign risk | ✅ **none** (different keys) | ❌ **real** if both ever run |
| Downtime | ✅ **none** | ⚠️ **~minutes** (sync first, swap last) |
| Delegators preserved | ❌ must redelegate manually | ✅ **same valoper, zero action** |
| Source-validator lock-up | ❌ **14 days** | ✅ n/a |
| Works with external delegators | ❌ **NO — strands them** | ✅ yes |

**Choose B whenever the validator has external delegators.** In that case A does not merely lose
delegators — it actively harms them:

> 🔴 **Why A fails with external delegators.** A validator leaves the active set only when its
> *total* tokens drop below 1 RUNE (`x/staking/keeper/val_state_change.go` — `PotentialConsensusPower
> == 0`). Redelegating **your own** self-delegation does not achieve that if other people's stake
> remains:
>
> ```
> before: 700,000 RUNE  (500k self + 200k external)
> you redelegate 500,000 → 200,000 RUNE still bonded → power 200,000 ≠ 0
> ```
>
> The old validator **stays in the active set**. If you then decommission its node because you
> "moved", it misses blocks: **1% downtime slash on every delegator (yours and theirs)**, then a
> 24h jail. Those delegators also cannot exit — `unbond` takes **14 days**. You would be running
> two nodes forever, or slashing people who trusted you.
>
> A only works when the validator is **100% self-delegated**, verified first:
> ```bash
> runed query staking delegations-to <old-valoper>   # only your own address should appear
> ```

### Approach A — redelegate to a new validator (self-delegated validators only)

> 🚫 **Prerequisite — verify this FIRST.** Approach A is only valid when the old validator has
> **no external delegators**. Check:
> ```bash
> runed query staking delegations-to <old-valoper>
> ```
> If any address other than your own appears, **stop and use Approach B**. Their stake would
> otherwise keep the old validator in the active set, and decommissioning its node slashes them.

The old validator's stake moves to a **new** validator on the new server, which then drops
out of the active set on its own — but **only because its total tokens reach zero**.

```bash
# ── 1. On the NEW server: bootstrap the node ──────────────────────────────
runed setup            # automated → <network> → validator
                       # setup writes a PLACEHOLDER genesis; replace it with the real one
cp <real-genesis.json> ~/.runed/config/genesis.json
runed start
runed status           # wait for catching_up: false
```

Wait for a **fully synced** node before creating the validator — a validator that enters the
set while behind will miss blocks.

```bash
# ── 2. Create the NEW validator with a minimal self-delegation ────────────
#     min_self_delegation must be <= the amount you bond.
#     1 RUNE is the practical minimum (power = floor(tokens / 10^18)).
runed tx staking create-validator ~/.runed/validator.json \
  --from <new-key> \
  --gas auto --gas-adjustment 1.3 --gas-prices 25000000000arune -y

# ── 3. Confirm BOTH validators are visible ────────────────────────────────
runed query staking validators
runed query tendermint-validator-set
```

```bash
# ── 4. Move the stake from old → new (INSTANT, no unbonding) ──────────────
runed tx staking redelegate \
  <old-valoper-addr> <new-valoper-addr> <amount>rune \
  --from <delegator-key> \
  --gas auto --gas-adjustment 1.3 --gas-prices 25000000000arune -y

# ── 5. The old validator now has zero power and leaves the set by itself ──
#     staking's ApplyAndReturnValidatorSetUpdates skips zero-power validators
#     (x/staking/keeper/val_state_change.go), so CometBFT stops expecting it.
runed query tendermint-validator-set     # old valoper gone, new one present

# ── 6. Only now stop the old server ───────────────────────────────────────
sudo systemctl stop runed && sudo systemctl disable runed    # on the OLD server
```

> ⚠️ **The source validator is locked for 14 days.** A redelegation writes an unbonding
> record on the source, so you cannot redelegate *out of that validator again* until
> `unbonding_time` (**1209600s = 14 days**) elapses. Plan one clean move, not a trial run.
> `max_entries` is 10, and transitive redelegations are rejected.

> ⚠️ **Delegators are not migrated.** Approach A creates a **different** validator under a new
> valoper. If the old one had any external delegators, see the prerequisite check above — their
> stake must be zero before you decommission the old node, otherwise you slash them.

### Approach B — relocate the consensus key (preserves delegators, production-safe)

Use this when the valoper address must stay the same — i.e. you have external delegators who
would otherwise have to redelegate manually.

**Sync first, swap last.** The new node is fully caught up *before* the old one is touched, so
downtime is only the few minutes it takes to move two small files. The old node keeps signing
the whole time the new one syncs.

> **Two files must travel together, not one.** `config/priv_validator_key.json` is the key, but
> `data/priv_validator_state.json` is the last-signed-height guard that stops the node from
> signing the same height twice (`CheckHRS` rejects any height regression). Copy them as a pair,
> captured while the old node is stopped, so they describe the same last-signed point.

```bash
# ── 1. On the NEW server: bootstrap and sync fully ─────────────────────────
runed setup                        # automated → <network> → validator
cp <real-genesis.json> ~/.runed/config/genesis.json
runed start
runed status                       # wait for catching_up: false
```

The old node is still signing during this phase — no downtime yet.

```bash
# ── 2. On the OLD server: stop it (new node is already synced) ────────────
sudo systemctl stop runed
sudo systemctl disable runed       # a reboot must NOT revive it
pgrep -af runed                    # MUST print nothing before you continue

# ── 3. Package BOTH validator files ──────────────────────────────────────
cd ~/.runed
tar czf /tmp/valkeys.tgz config/priv_validator_key.json data/priv_validator_state.json
scp /tmp/valkeys.tgz new:/tmp/
shred -u /tmp/valkeys.tgz          # do not leave key material in /tmp
```

```bash
# ── 4. On the NEW server: STOP it before swapping the key ────────────────
sudo systemctl stop runed          # swapping a key under a running node is unsafe

# ── 5. Replace the throwaway key with the real one, plus its state ───────
cd ~/.runed && tar xzf /tmp/valkeys.tgz
ls -l config/priv_validator_key.json          # the REAL key is now in place
cat  data/priv_validator_state.json           # height matches the old node's
shred -u /tmp/valkeys.tgz
```

```bash
# ── 6. Start the NEW node and verify it SIGNS ────────────────────────────
sudo systemctl start runed
runed status                                  # height must ADVANCE past the copied height
runed query slashing signing-info $(runed tendermint show-address)
```

```bash
# ── 7. Only now destroy the old node ─────────────────────────────────────
# on the OLD server: the key is still there (only the tarball was shredded)
shred -u ~/.runed/config/priv_validator_key.json
shred -u ~/.runed/data/priv_validator_state.json
# then decommission the VPS
```

**Why step 5 must run with the new node stopped:** the validator key is loaded into memory at
startup. Replacing the file under a running process leaves the old (throwaway) key active, and
the node would sign with the wrong identity until restarted.

#### Hardening the key transfer (step 3)

Step 3 is the only point where key material crosses a network, so treat it as the weakest link.
`scp` encrypts in transit, but the file lands **decrypted** on any host it passes through — so a
compromised or shared jump box sees the raw key. Pick the option that matches your threat model:

**Option 1 — direct host-to-host (minimum viable).** No intermediate hop:

```bash
# run FROM the old server, pulling/pushing straight to the new one
tar czf - -C ~/.runed config/priv_validator_key.json data/priv_validator_state.json \
  | ssh new 'cat > /tmp/valkeys.tgz'
```

Never route the file through a third machine. If you must use a bastion, see Option 2.

**Option 2 — encrypted archive (recommended).** The payload is unreadable even if the transport
or an intermediate host is compromised. `age` is the simplest:

```bash
# on the OLD server: passphrase-encrypt the tarball
tar czf - -C ~/.runed config/priv_validator_key.json data/priv_validator_state.json \
  | age -p -o /tmp/valkeys.tar.gz.age
scp /tmp/valkeys.tar.gz.age new:/tmp/
shred -u /tmp/valkeys.tar.gz.age
```

```bash
# on the NEW server: decrypt (prompts for the passphrase you chose)
age -d -o - /tmp/valkeys.tar.gz.age | tar xzf - -C ~/.runed
shred -u /tmp/valkeys.tar.gz.age
```

Store the passphrase **out of band** (a password manager, not the same SSH session), and transfer
it by a different channel than the file itself.

**Option 3 — external signer / HSM (strongest, biggest change).** Run a separate signer process
(`tmkms`, or any remote signer speaking the CometBFT privval socket protocol) and point the node at
it with `--priv_validator_laddr`:

```bash
runed start --priv_validator_laddr tcp://127.0.0.1:26659
```

The consensus key then lives in the signer (which can be backed by a YubiKey/HSM) instead of a
plaintext file, so a compromised node host never exposes it. Costs: you must run, secure and
monitor a second service, and the signer itself becomes the critical component.

**After any of these:** verify the received file is intact before trusting it — compare a checksum
computed on the old box with one computed on the new:

```bash
# before transfer (old)
sha256sum ~/.runed/config/priv_validator_key.json ~/.runed/data/priv_validator_state.json

# after extraction (new) — hashes must match exactly
sha256sum ~/.runed/config/priv_validator_key.json ~/.runed/data/priv_validator_state.json
```

A truncated or partially-written key file is a silent failure: the node starts, then fails to sign,
and you are burning your jail budget while you debug.

### 🔴 The rule that applies to both approaches

**Never run both nodes with the same `priv_validator_key.json` at the same time.**

Two nodes signing with one consensus key is **equivocation (double-sign)**:

- slashed **10%** of the validator's stake (`slash_fraction_double_sign: 0.10`)
- **permanently tombstoned** — removed from the set, and **cannot be unjailed**

Downtime is recoverable; tombstoning is permanent. This is also why Approach A is the safer
default: two *different* keys can never equivocate.

### Downtime budget

`min_signed_per_window` is **0.50** over a **10,000-block** window (~16.7h at 6s blocks), so a
validator can miss up to ~8.3h before being jailed. `downtime_jail_duration` is **24h** and the
downtime slash is **1%**.

- **Approach A:** effectively zero downtime — the old validator only loses power after the new
  one is already producing blocks.
- **Approach B:** downtime is only the swap window (a few minutes), because the new node is
  already synced before the old one is stopped. Nothing needs to re-sync from genesis; only two
  small files move.

If you do get jailed, see [Unjailing after downtime](#unjailing-after-downtime).

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

## Securing the node and keeping it running

A validator that stops is a validator that gets jailed. Two things keep it up:
a **systemd unit** so it starts on boot and restarts if it crashes, and sensible
host hardening so the node key is not exposed.

### 1. Run it as a dedicated user

Do not run a validator as `root`. Create a service account and give it the node
home only:

```bash
sudo useradd --system --home /home/runed --create-home --shell /usr/sbin/nologin runed
sudo chown -R runed:runed ~/.runed
sudo chmod 700 ~/.runed
```

Then move the node home under that account (or create it there from the start with
`runed setup --home /home/runed/.runed`).

> ⚠️ **`~/.runed/key_seed.json` holds your mnemonic in plaintext.** If that account
> is compromised, the premine and the validator's funds are gone. Keep the file mode
> `0600`, and keep the keyring directory (`~/.runed/keyring-file/`) at `0700`.

### 2. systemd service

Create `/etc/systemd/system/runed.service`:

```ini
[Unit]
Description=Hoodium node (runed)
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=runed
Group=runed
# Raise if your node holds many open peers / RPC clients.
LimitNOFILE=65535
ExecStart=/usr/local/bin/runed start --home /home/runed/.runed
Restart=always
RestartSec=5
# Graceful shutdown so the last state is flushed before exit.
TimeoutStopSec=30
KillSignal=SIGINT
# Modest hardening; the node needs its own home writable.
NoNewPrivileges=true
ProtectSystem=full
ProtectHome=read-only
ReadWritePaths=/home/runed/.runed
PrivateTmp=true

[Install]
WantedBy=multi-user.target
```

Enable and start it:

```bash
sudo systemctl daemon-reload
sudo systemctl enable runed      # start on boot
sudo systemctl start runed
systemctl status runed           # confirm it is running
sudo journalctl -u runed -f      # follow the logs
```

> **Adjust `ExecStart`** to the real binary path (`which runed`) and home directory.
> `ProtectHome=read-only` plus `ReadWritePaths` is what keeps the service account
> from writing anywhere except its node home.

### 3. Restart on reboot

`systemctl enable runed` already handles reboots — that is its purpose. If you
cannot use systemd, a **cron `@reboot`** entry is the fallback:

```bash
crontab -e -u runed
```

```cron
@reboot sleep 30 && /usr/local/bin/runed start --home /home/runed/.runed >> /home/runed/runed.log 2>&1
```

> Prefer systemd. `@reboot` gives you no restart-on-crash, no log integration and no
> graceful shutdown — it only covers the boot case.

### 4. Keep the validator signing

- **Back up `~/.runed/config/priv_validator_key.json`** and keep it offline. It is
  plaintext, and losing it means you can never sign again; leaking it means someone
  else can double-sign on your behalf.
- **Never copy a validator key to a second machine.** Running the same key twice
  produces an equivocation, which is slashed at **10%** and **tombstoned** — the
  validator is permanently removed from the set.
- **Monitor liveness.** Miss more than 50% of a 10,000-block window and you are
  jailed. `runed status --home ~/.runed` shows `catching_up` and the latest height.
- **Firewall the RPC ports.** `26657` (CometBFT RPC) and `8545` (EVM JSON-RPC) should
  not be exposed publicly unless you intend to serve them. Keep `26656` (p2p) open.
- **Do not enable the `personal` JSON-RPC namespace** — see
  [`evm-wallets.md`](./evm-wallets.md).
