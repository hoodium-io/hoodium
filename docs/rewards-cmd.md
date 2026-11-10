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
`x/pona` itself exposes **no query service** (`GetQueryCmd` returns `nil`; its only proto is
`genesis.proto`) — there is nothing pona-specific to query.

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
  - 18062601027437499999491.999839282500000000arune
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
  - 361252020548749999989839.996785650000000000arune
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
  - 343189419521312499500000.000000000000000000arune
  validator_address: runevaloper1upqhyf2w64juryqu98g734ccem62j48602zv5m
total:
- 343189419521312499500000.000000000000000000arune
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

## Known limitation — `x/pona` has no queries

There is **no command to inspect the validator reward pool balance** or the emission tier
boundaries directly, because `x/pona` exposes no query service. To infer the remaining reserve,
query the module account directly — its address is deterministic
(`ripemd160(sha256("validator_reward_pool"))`):

```bash
runed query bank balance rune1f9nhzk7zhfwv9gnzq4994zf796g0e5lej732lk arune
```

> Pool exhaustion is logged at runtime: `"validator reward pool exhausted, switching to fee-only"`.
> Once the 1B RUNE reserve is spent, rewards become **fees only** — the intended end state.

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
