# Rewards — querying and withdrawing

Everything about validator and delegator rewards: how to see what you have earned, how to
withdraw it, and where the money actually comes from on Hoodium.

All commands below run against the **CometBFT RPC on 26657**, which stays bound to
`127.0.0.1` on a validator. `--home ~/.runed` and the chain-id come from
`~/.runed/config/client.toml`, so neither needs to be passed.

---

## Where rewards come from on Hoodium

Hoodium is **not** a plain Cosmos chain for rewards, but the queries are the standard ones. The
path is:

```
x/pona  (validator_reward_pool — the 1B RUNE reserve)
   │
   │  every block: emits a reward for the block + a per-tx component,
   │  capped so the pool cannot be drained early
   ▼
x/distribution fee collector
   │
   │  BeginBlock: AllocateTokens() splits it across the ACTIVE validator set
   │  by voting power
   ▼
x/distribution  →  validator commission + delegator rewards
```

Because the last hop is ordinary `x/distribution`, **every standard query works**.
`x/pona` exposes **no gRPC query service** (its only proto is `genesis.proto`),
but `runed query pona` reads the module's KV store directly, so the reward
schedule, the current band and the remaining reserve are all queried with plain
Walk-through: `runed query pona block-reward` reports the band, the amount and a
one-line explanation, so the log line and the paid reward can always be
reconciled — see [PoNA-specific queries](#pona-specific-queries) below.

> **Transaction fees** land in the same fee collector, so what you see is
> *block rewards + fees* combined. On a quiet chain fees are negligible; under load they matter.

---

## Querying

### Addresses you need

```bash
runed keys show <key-name> -a                     # rune1…        (account / delegator)
runed keys show <key-name> --bech val -a          # runevaloper1… (validator operator)
runed tendermint show-address                     # runevalcons1… (consensus)
```

### Validator commission — what you earn as the operator

```bash
runed query distribution commission <valoper-addr>
```

```yaml
commission:
  commission:
  - 12345.678901234567890123arune
```

This is the **operator cut** (your `commission-rate`, minimum 5% on Hoodium). It is separate from
delegator rewards and is **only paid out when you withdraw with `--commission`**.

### Total unwithdrawn rewards on a validator

```bash
runed query distribution validator-outstanding-rewards <valoper-addr>
```

```yaml
rewards:
  rewards:
  - 123456.789012345678901234arune
```

This is the **whole pot**: every delegator's pending rewards *plus* the commission, none of it
withdrawn yet. Not all of it is yours.

### Your delegator rewards

```bash
runed query distribution rewards <rune1-account-addr>
```

```yaml
rewards:
- reward:
  - 123456.789012345678901234arune
  validator_address: runevaloper1<...>
total:
- 123456.789012345678901234arune
```

One entry per validator you delegate to, plus a `total`.

### Rewards from one specific validator

```bash
runed query distribution rewards-by-validator <rune1-account-addr> <valoper-addr>
```

### Validator distribution info (commission + outstanding in one call)

```bash
runed query distribution validator-distribution-info <valoper-addr>
```

### Slash history

`slashes` takes **three** arguments — validator, start height, end height:

```bash
runed query distribution slashes <valoper-addr> 0 <current-height>
```

Get the current height first:

```bash
curl -s localhost:26657/status | jq -r '.result.sync_info.latest_block_height'
```

```yaml
pagination: {}
slashes: null      # null = never slashed
```

### Community pool

```bash
runed query distribution community-pool
```

Community tax is **0** on Hoodium (`runed query distribution params` →
`community_tax: "0.000000000000000000"`), so the pool is not fed by inflation. Anything in it
arrives via an explicit `fund-community-pool` transaction.

### Where your rewards are sent

```bash
runed query distribution delegator-withdraw-address
```

`withdraw_addr_enabled: true`, so you may redirect rewards to a different account — useful for
keeping the validator operator key cold.

### Distribution parameters

```bash
runed query distribution params
```

```yaml
params:
  base_proposer_reward: "0.000000000000000000"
  bonus_proposer_reward: "0.000000000000000000"
  community_tax: "0.000000000000000000"
  withdraw_addr_enabled: true
```

---

## Withdrawing

Rewards accrue every block but sit in `x/distribution` — **they are not in your bank balance
until you withdraw**. `runed query bank balances` will not show them. They also do not compound;
you must withdraw and re-delegate for that.

### Delegator rewards only

```bash
runed tx distribution withdraw-rewards <valoper-addr> \
  --gas auto --gas-adjustment 1.3 --gas-prices 25000000000arune -y
```

### Delegator rewards **and** validator commission

```bash
runed tx distribution withdraw-rewards <valoper-addr> --commission \
  --gas auto --gas-adjustment 1.3 --gas-prices 25000000000arune -y
```

> ⚠️ **Validator operators:** `withdraw-rewards` **without `--commission`** claims only the
> delegator share and leaves your commission sitting there. Use `--commission`.

### Everything, across all your delegations

```bash
runed tx distribution withdraw-all-rewards \
  --gas auto --gas-adjustment 1.3 --gas-prices 25000000000arune -y
```

### Change where rewards are paid

```bash
runed tx distribution set-withdraw-addr <rune1-destination> \
  --gas auto --gas-adjustment 1.3 --gas-prices 25000000000arune -y
```

### Fund the pools

```bash
# community pool
runed tx distribution fund-community-pool <amount>rune \
  --gas auto --gas-adjustment 1.3 --gas-prices 25000000000arune -y

# validator reward pool (x/pona) — tops the block-reward reserve back up
runed tx distribution fund-validator-rewards-pool <amount>rune \
  --gas auto --gas-adjustment 1.3 --gas-prices 25000000000arune -y
```

---

## What the numbers mean

| Figure | Meaning |
|---|---|
| `commission` | the operator's cut, always yours |
| `validator-outstanding-rewards` | the whole unwithdrawn pot (commission + every delegator) |
| `rewards <addr>` | **your** share as a delegator, per validator |
| `community-pool` | chain-level pool, funded explicitly (tax is `0`) |

A validator that self-delegates appears in **both** places: it earns commission *and* a delegator
reward on its own bond. That is why the delegator figure can approach the outstanding total for a
100%-self-delegated validator.

## Rewards depend on being in the active set

Only **bonded, non-jailed** validators earn. If you are jailed, your validator stops earning and
your delegators stop earning too. Check:

```bash
runed query slashing signing-info $(runed tendermint show-address)
runed query tendermint-validator-set
```

See [validator.md](./validator.md#unjailing-after-downtime) for unjailing.

## PoNA-specific queries

`runed query pona` answers everything about the RUNE block-reward system itself:
which tier is active, what a block pays and why, and how much of the 1B RUNE
reserve is left.

| Command | Answers |
|---|---|
| `runed query pona params` | the full emission schedule and the PoNA rates of every tier |
| `runed query pona tier` | which tier is active now, and how many blocks until the next one |
| `runed query pona block-reward` | what the last block paid, which band applied, and why |
| `runed query pona block-reward --tx-count 25` | what a block with 25 transactions *would* pay |
| `runed query pona tx-count` | the transaction count PoNA used for the last block |
| `runed query pona pool` | reserve balance, and how long it lasts at the current tier |

All of them accept the standard query flags: `--height` to ask about a specific
block, and `-o json` for machine-readable output (`-o text` is the default and
renders YAML).

### What a block pays right now

```bash
runed query pona block-reward
```

```yaml
band: zero
band_explanation: the block contains no regular transactions, so the zero-activity rate of 1 RUNE applies
height: 32495
reward: "1000000000000000000"
reward_rune: "1"
tier:
  full_reward: {arune: "50000000000000000000", rune: "50"}
  low_reward: {arune: "20000000000000000000", rune: "20"}
  zero_reward: {arune: "1000000000000000000", rune: "1"}
  effective_low: {arune: "20000000000000000000", rune: "20"}
  floor_applied: false
  pona_enabled: true
  tx_count_threshold: 10
  tier_index: 0
  tier_count: 3
tx_count: 0
tx_count_source: chain (recorded for the last finalized block)
```

The three bands are selected by the number of **regular** transactions in the
block, and `zero` is also the **floor** — no block is ever paid less:

| Band | Condition | Tier 1 pays |
|---|---|---|
| `full` | `tx_count >= 10` | 50 RUNE |
| `low` | `tx_count > 0` | 20 RUNE |
| `zero` | `tx_count == 0` | 1 RUNE (floor) |

`runed query pona params` reports both the configured `low` rate and the
`effective_low` actually paid, so a tier whose low rate sits below its zero floor
is visible rather than silently clamped. The default schedule never needs it.

> **`num_txs=1` in the node log is not a transaction.** CometBFT's
> `finalizing commit of block ... num_txs=1` counts every entry in the proposal,
> and the app-level pseudo-transaction carrying oracle vote extensions is one of
> them. It is never delivered as a transaction, so PoNA does not count it —
> `tx_count` reads `0` on an idle chain, which is exactly what pays the 1 RUNE
> zero-activity rate instead of 20. Use `runed query pona tx-count` whenever the
> log and the reward seem to disagree.

### Which tier is active, and when the next one starts

```bash
runed query pona tier
```

```yaml
height: 32495
tier:
  start_height: 0
  ends_at_height: 10519200
  ends_at_height_meaning: exclusive
  tier_index: 0
  tier_count: 3
  full_reward: {arune: "50000000000000000000", rune: "50"}
blocks_until_next_tier: "10486705"
time_until_next_tier: 1y 364d
next_tier_full_reward: "25000000000000000000"
next_tier_full_reward_rune: "25"
```

Estimates use Hoodium's 6-second target block time, the same constant the
schedule's tier boundaries are defined in.

### The remaining reserve

```bash
runed query pona pool
```

```yaml
address: rune1...
balance: "999999999999999..."
balance_rune: "999999999..."
remaining_blocks: "19999999"
remaining_duration: 3y 295d
```

`remaining_blocks` assumes **every** future block pays the tier's full rate, so
it is a *lower* bound: quiet blocks pay less and make the reserve last longer.
When the balance reaches zero, emission stops and the chain runs on fees only.

### Why the pool still needs the module address

The pool has no key and no operator; its address is derived from the module
account name. To read the raw bank balance without `query pona`:

```bash
runed query bank balance <validator-reward-pool-addr> arune
```

The address is deterministic — it does not depend on genesis or on the network —
so it is safe to derive it locally:

```
sha256("validator_reward_pool") first 20 bytes, then bech32-encoded with the rune HRP
```

> **Watch out:** the derivation is **sha256 truncated to 20 bytes**, *not*
> ripemd160. Several older comments in this repository (including
> `cmd/runed/mainchain.go`) still say `ripemd160(sha256(...))` and give the
> resulting wrong address. `authtypes.NewModuleAddress` → `address.Module` →
> `crypto.AddressHash` → `tmhash.SumTruncated`, which is sha256[:20]. Using the
> ripemd160 form yields an account that exists but holds **zero**, which
> silently misleads you.

There is **no command to inspect the validator reward pool balance** or the emission tier
boundaries directly, because `x/pona` exposes no query service. To read the remaining reserve,
query the pool's **module account**:

```bash
runed query bank balance <validator-reward-pool-addr> arune
```

The address is deterministic — it does not depend on genesis or any particular network — so it is
safe to derive it locally rather than copy it from a live chain:

```
sha256("validator_reward_pool") first 20 bytes, then bech32-encoded with the rune HRP
```

> **Watch out:** the derivation is **sha256 truncated to 20 bytes**, *not* ripemd160. Several older
> comments in this repository (including `cmd/runed/mainchain.go`) still say
> `ripemd160(sha256(...))` and give the resulting wrong address. `authtypes.NewModuleAddress` →
> `address.Module` → `crypto.AddressHash` → `tmhash.SumTruncated`, which is sha256[:20]. Using the
> ripemd160 form yields an account that exists but holds **zero**, which silently misleads you.

> Pool exhaustion is logged at runtime: `"validator reward pool exhausted, switching to fee-only"`.
> Once the reserve is spent, rewards become **fees only** — the intended end state.

## Common mistakes

| Symptom | Cause |
|---|---|
| `bank balances` shows nothing after weeks of validating | rewards are held in `x/distribution`, not your balance — withdraw them |
| Commission never arrives | withdrew without `--commission` |
| `slashes` errors with "accepts 3 arg(s)" | needs `<valoper> <start-height> <end-height>` |
| Rewards stopped growing | validator jailed or out of the active set |

## See also

- [validator.md](./validator.md) — onboarding, liveness, jailing, relocating a validator
- [tx-fees.md](./tx-fees.md) — the two-gas-price model
- [tx-cmd.md](./tx-cmd.md) — sending transactions
