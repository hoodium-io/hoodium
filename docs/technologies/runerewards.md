# `x/runerewards` — RUNE Block Rewards

> **Status:** Implemented (`x/runerewards`), with a **pending correction** to the
> Tier-3 rate (see [§4](#4-emission-schedule)). The **activity policy** that
> scales these rewards is documented separately in [`pona.md`](./pona.md).

## 1. What it does

Hoodium pays validators a **static RUNE block reward** funded from a pre-funded
**1B RUNE reserve** — not from Cosmos-style inflation. This module owns:

- The **validator reward pool** module account (`validator_reward_pool`): funding
  at genesis, balance queries, top-up.
- The **base emission schedule**: which full reward applies at a given block
  height.
- **Payment plumbing**: transferring the reward from the pool to the block
  proposer's operator address.

Transaction fees are **not** handled here — they follow the standard
`x/distribution` flow (fee pool split across the active validator set by voting
power). See [`pona.md` §8](./pona.md#8-interaction-with-the-fee-market).

## 2. The reward pool

| Property | Value |
|---|---|
| Module account | `validator_reward_pool` |
| Genesis funding | **1,000,000,000 RUNE** (bank genesis balance) |
| Source of funds | Pre-mined, not inflated — part of the fixed 10B RUNE supply |
| Supply split | 9B premine to the project owner + 1B to this reserve |
| On exhaustion | Emission stops (fee-only); the pool is **never overdrawn** |

The pool is a **one-way budget**: once it reaches zero, there is no static reward
and the chain runs on fees permanently.

## 3. Block lifecycle

```
EndBlock
  └─ x/runerewards: DistributeRuneBlockReward(ctx, proposerConsAddr)
       1. reward  = RewardForHeight(height)        ── base schedule lookup
       2. if pool < reward  → stop (fee-only), never overdraw
       3. resolve proposer consensus addr → operator addr
       4. send reward from validator_reward_pool → operator
       5. emit rune_block_reward event
```

- The proposer is taken from `ctx.BlockHeader().ProposerAddress`.
- The reward is paid to the proposer **only**; delegators receive their share via
  the proposer's commission through `x/distribution`.
- When PoNA is implemented, the reward in step 1 is the **effective** (activity-
  scaled) amount rather than the full base rate.

## 4. Emission schedule

Block time is **6 seconds** → `5,259,600` blocks/year (`5_259_600` at 6 s).

| Tier | Epoch | Heights | Full rate | PoNA low | PoNA zero |
|---|---|---|---|---|---|
| 1 | Y0–2 (2 years) | `[0, 10,519,200)` | **50 RUNE/block** | 20 | 1 |
| 2 | Y2–4 (2 years) | `[10,519,200, 21,038,400)` | **25 RUNE/block** | 10 | 1 |
| 3 | Y4 → infinite | `[21,038,400, 0)` (open-ended) | **10 RUNE/block** | 5 | 1 |

Each tier carries its own full/low/zero rates (see
[`pona.md` §3](./pona.md#3-per-tier-rates)). All three are stored on-chain as
`Tier` proto fields.

### 4.1 Fee-only floor — REMOVED

The module used to carry a `min_reward_per_block` parameter below which emission
stopped. **That floor has been removed**: PoNA's zero-activity band pays
**1 RUNE**, which is below the old 5-RUNE floor, so a floor of 5 would have
silently forced the zero-activity reward to **0**.

Emission now ends only when **no tier covers the height** (the schedule is
exhausted) or when the reward pool runs dry. The `min_reward_per_block` field no
longer exists in `Params`.

## 5. Reserve lifetime

With a fixed 1B reserve and the schedule above:

| Scenario | Assumption | Fee-only at |
|---|---|---|
| Busy | Tier 3 at full (10) | **≈ Y8.0** |
| Typical | Tier 3 at low (5) | **≈ Y12.0** |
| Quiet | Tier 3 at zero (1) | **≈ Y44.1** |

Full arithmetic in [`pona.md` §6](./pona.md#6-economic-effect-on-the-1b-reserve).

## 6. Genesis wiring

| Item | Value |
|---|---|
| `cmd/runed/testnet.go` | Sets the 1B reserve balance for `validator_reward_pool` |
| `app/app.go` `maccPerms` | `validator_reward_pool: nil`, `runerewards: {Minter}` |
| EndBlockers order | `runerewards` **before** `feemarket` |

## 7. Relationship to PoNA

| | `x/runerewards` | `x/pona` |
|---|---|---|
| Role | **Money** — reserve in, coins out | **Policy** — how much comes out |
| Owns | Pool, tier boundaries, full rates, payment | Tx-count capture, banding, low/zero rates |

`runerewards` stays a pure schedule + transfer module; PoNA decides the
**effective** reward. See [`pona.md` §7](./pona.md#7-module-boundary).
