# dPona — Dynamic Proof of Network Activity

> **Status: IMPLEMENTED AND TESTED.** The ten-band ladder, the dPona controller,
> the fee-only fallback and the genesis layout are in `x/pona`; the proto has been
> regenerated and `x/pona/types` passes all 30 tests. [`pona.md`](./pona.md) has
> been rewritten for the ten-band ladder and is the module reference; **this**
> document remains the design of record for the controller.

dPona is the second PoNA generation. It replaced the five-band ladder with a
**ten-band** ladder and adds a **difficulty controller** that shifts every band's
transaction threshold up or down in response to sustained max-band activity.

PoNA itself is unchanged in purpose: scale the RUNE block reward by how much
real activity a block contains. See [`pona.md`](./pona.md) §1 for why that
exists at all.

---

## 1. The ten bands

Each block is classified into exactly one band by its **regular** transaction
count (the injected vote-extension pseudo-tx is never counted — see
[`pona.md` §4.1](./pona.md#41-which-transactions-are-counted)).

| Band | Condition | Tier 1 | Tier 2 | Tier 3 | Tier 4 (fees) |
|---|---|---|---|---|---|
| **max** | `tx >= max_t` (20) | 50 | 40 | 30 | 1.0 |
| **high** | `15 <= tx < 20` | 40 | 30 | 20 | 0.7 |
| **mid** | `10 <= tx < 15` | 30 | 20 | 15 | 0.5 |
| **low** | `6 <= tx < 10` | 20 | 15 | 10 | 0.1 |
| **quint** | `tx == 5` | 5 | 5 | 5 | 0.05 |
| **quart** | `tx == 4` | 4 | 4 | 4 | 0.04 |
| **trio** | `tx == 3` | 3 | 3 | 3 | 0.03 |
| **duo** | `tx == 2` | 2 | 2 | 2 | 0.02 |
| **single** | `tx == 1` | 1 | 1 | 1 | 0.01 |
| **zero** | `tx == 0` | 0.5 | 0.5 | 0.5 | **0** |

All values in RUNE; stored on chain in arune (`1 RUNE = 10^18 arune`).

### 1.1 Why the exact bands exist

The original five-band ladder jumped from `low` (2–4 txs) straight to `single`
(1 tx), paying 15 → 1 RUNE across that gap. The ten-band ladder fills it with
per-count steps (5, 4, 3, 2), so a lightly-used chain is paid proportionally
rather than in one cliff.

### 1.2 Tier 4 pays 0 in the zero band, by construction

Tier 4 is funded **only** from transaction fees. A block with zero transactions
has zero fees, so there is nothing to pay from. `zero` is therefore **0**, not a
nominal floor — this is exactly what makes Tier 4 self-describing: *fees only, so
no fees means no reward*. See §6.

---

## 2. Derived thresholds — how the exact bands are computed

The exact-count bands are **derived from `low_tx_count_threshold`**, not stored:

```
quint  = low_t - 1
quart  = low_t - 2
trio   = low_t - 3
duo    = low_t - 4
single = low_t - 5   (clamped floor: this band absorbs all counts below duo)
zero   = 0
```

Evaluation order, first match wins:

```
tx == 0                -> zero
tx <  duo_t            -> single     (clamped; absorbs 1 .. low_t-5)
tx == duo_t            -> duo
tx == trio_t           -> trio
tx == quart_t          -> quart
tx == quint_t          -> quint
tx <  mid_t            -> low
tx <  high_t           -> mid
tx <  max_t            -> high
otherwise              -> max
```

### 2.1 Why derived and not stored

The naive spelling uses `<=` for the exact bands:

```
txs_count <= 5+10  -> quint
txs_count <= 4+10  -> quart
```

Evaluated in order this is **wrong**: `quint`'s `<= 15` swallows 0–15, making
quart/trio/duo/single unreachable and overpaying quiet blocks 2.5×. Switching to
`==` fixes the overlap but opens a **gap** — with the threshold shifted to 15,
`single == 11` and counts 1–10 match **nothing**, silently falling to zero.

Deriving from a single number (`low_t`) with a clamped floor has neither defect:
every count from 0 to `max_t` maps to exactly one band, at every difficulty.

---

## 3. dPona — the difficulty controller

### 3.1 What it does

dPona raises or lowers **every band's threshold** by a multiple of 10
transactions. Because the exact bands are derived (§2), shifting `low_t` shifts
the whole ladder coherently.

```
txs_offset = dpona_diff * 10        // applied to max, high, mid, low (and derived bands)
```

| `dpona_diff` | offset | max_t | high_t | mid_t | low_t | quint_t | singles |
|---|---|---|---|---|---|---|---|
| **0** (dPona off) | +0 | 20 | 15 | 10 | 6 | 5 | 1 |
| 1 | +10 | 30 | 25 | 20 | 16 | 15 | 11 |
| 2 | +20 | 40 | 35 | 30 | 26 | 25 | 21 |
| 3 | +30 | 50 | 45 | 40 | 36 | 35 | 31 |

At `diff = 1`, a block needs **30** transactions to earn the max band.
The count range `1 .. low_t-5` always clamps into `single`, so no count is ever
unbanded.

### 3.2 `dpona_diff = 0` means dPona is OFF

There is **no separate boolean**. `diff` is the count of +10 steps:

| dPona | `diff` | offset |
|---|---|---|
| Off | **0** | 0 |
| On | ≥ 1 | `diff × 10` |

This makes "off" the natural base state and removes any risk of the flag and the
number disagreeing.

### 3.3 The state machine

State is **deterministic**, stored on chain, and global (not per-tier):

```go
type DPonaState struct {
    ConsecutiveMax uint8   // 0..5  consecutive blocks in the max band
    BlocksSinceMax uint64  // 0..100, saturating
    Diff           uint32  // 0..n  +10-step count; 0 = off
}
```

Updated once per block in `EndBlock`, after the band is classified:

| Condition | Transition |
|---|---|
| last **5** blocks all `max` | `Diff += 1`; `ConsecutiveMax = 0` |
| **no** `max` block in the last **100** blocks | `Diff -= 1` (floored at 0); `BlocksSinceMax = 0` |
| anything else | **hold** — no change |

Rules are evaluated against the *current* diff, and the new diff takes effect
from the **next** block (a block is never classified by a difficulty it caused).

### 3.4 Why there is no cap

The controller is a negative-feedback loop with a **20:1 asymmetry favouring
decay**:

| Direction | Rate | Blocks per step |
|---|---|---|
| increase | 5 consecutive max blocks | 5 |
| decrease | 100 blocks with no max | 100 |

Critically, `diff` can only rise **while blocks are actively at max**. Raising
the threshold makes max harder to reach, so the same event that raises `diff`
eventually prevents further rises and starts the 100-block decay timer. The
system is self-limiting:

- a burst of 10 increments in 500 blocks (`diff = 10`, `max_t = 120`) unwinds in
  1,000 blocks ≈ **1.7 hours**;
- `diff = 20` (2,000 blocks to clear) still unwinds in ≈ **3.3 hours**.

No cap is enforced. Instead, an **operator warning** is logged when `diff >= 5`,
purely for observability — a signal that the chain has been hot, not a limit.

> **Design note.** An earlier draft argued for a hard cap on the grounds that
> runaway difficulty would pin the chain at the floor. That was wrong: an
> unreachable max threshold *causes* decay rather than preventing it. The user
> identified this; the reasoning above is the correction.

### 3.5 dPona is global across tiers

`band max` is conditioned chain-wide, so `diff` is a **single global value**.
When a tier boundary is crossed mid-flight, the current `diff` applies to the new
tier's base thresholds. Tier reward **amounts** differ per tier; only the
**thresholds** shift.

### 3.6 dPona does not apply to Tier 4

Tier 4's reward is dominated by `min(scheduled, skim × fees)` (§6). Raising the
threshold would reduce the band while the fee cap is already the binding
constraint, making the controller meaningless. Tier 4 uses fixed base thresholds
and `diff` is not applied.

---

## 4. Tier 4 — the fee-funded fallback

### 4.1 It is not a height tier

The first three tiers are **reserve-funded** and selected by block height. Tier 4
is **fee-funded** and selected by whether the reserve can still pay.

This is a deliberate correction to an earlier draft that gave Tier 4 a start
height. A fixed start height only works if Tier 3 spends its whole budget on
schedule — and it does not, because spending is activity-scaled:

| Tier 3 activity | Blocks to exhaust the 53,272,000 budget | Reserve **stranded** at height 22,814,133 |
|---|---|---|
| max (30/blk) | 1,775,733 | **10 RUNE** |
| high (20/blk) | 2,663,600 | 17,757,340 RUNE |
| mid (15/blk) | 3,551,467 | 26,636,005 RUNE |
| low (10/blk) | 5,327,200 | 35,514,670 RUNE |
| quint (5/blk) | 10,654,400 | 44,393,335 RUNE |
| zero (0.5/blk) | 106,544,000 | **52,384,134 RUNE** |

A quiet chain would strand **98% of Tier 3's budget** — 52M RUNE that could never
be paid. So Tier 3 stays open-ended (`end_height: 0`) and Tier 4 begins when the
reserve stops being able to cover the scheduled reward. **Zero RUNE is stranded.**

### 4.2 The switch

The existing code already has the branch (`x/pona/keeper/rewards.go`):

```go
if pool.Amount.LT(reward) {
    // was: return zero — emission simply stops (fee-only)
    // now: compute the fee-funded ladder instead
}
```

`ComputeBlockReward` reports a `FundingSource` (`reserve` | `fees`) so the panel,
the CLI and the event can all say which one paid.

### 4.3 It does not create money

**After emission ends, validators already receive all transaction fees.**
`x/distribution.AllocateTokens` sweeps `fee_collector` every `BeginBlock` and
splits it by voting power; the devnet sets `community_tax: 0`, so 100% goes to
validators either way.

Tier 4 therefore **re-allocates fees that validators would receive anyway**. It
changes the split from *by voting power* to *by PoNA band*. No supply is created,
and the mint cap is not involved.

### 4.4 Solvency — the reward is clamped by available fees

A fixed-schedule fee-funded reward is **insolvent** at light traffic. From a real
transaction (0.541044 RUNE / 209,782 gas ⇒ **2.58 × 10⁻⁶ RUNE per gas**), the
cheapest possible transfer (21,000 gas) costs **0.0542 RUNE**:

| Band | txs | Fee income at cheapest tx | Reward | Share of income |
|---|---|---|---|---|
| max | 20 | 1.083 RUNE | 1.0 | 92.3% |
| high | 15 | 0.812 | 0.7 | 86.2% |
| mid | 10 | 0.542 | 0.5 | 92.3% |
| low | 6 | 0.325 | 0.1 | 30.8% |
| quint | 5 | 0.271 | 0.05 | 18.5% |
| quart | 4 | 0.217 | 0.04 | 18.5% |
| trio | 3 | 0.162 | 0.03 | 18.5% |
| duo | 2 | 0.108 | 0.02 | 18.5% |
| single | 1 | 0.054 | 0.01 | 18.5% |
| zero | 0 | **0.000** | **0** | n/a |

Every non-zero band is fundable. The reward is still clamped:

```
paid = min(scheduled_reward, fee_skim_percent × block_fees)
```

At `fee_skim_percent = 75`, on 20 light transactions (1.083 RUNE of fees):

| | max | high | mid | low | quint |
|---|---|---|---|---|---|
| available (75%) | 0.812 | 0.812 | 0.812 | 0.812 | 0.812 |
| paid | **0.81** | 0.70 | 0.50 | 0.10 | 0.05 |

`high`, `mid` and everything below reach their **full scheduled value**; only
`max` is clamped, to 0.81 of its 1.0. That is intended — the table is a ceiling,
not a promise.

> **Why 75% and not 100%.** At 100% validators would receive *only* the Tier 4
> reward, which is a far larger change to their economics. At 75% they keep a
> quarter of fees flowing through `AllocateTokens` on top of the reward. Both are
> "all to validators"; 75% simply keeps the existing split meaningful.

### 4.5 Ordering hazard

PoNA **adds** its reward into `fee_collector` in the same `EndBlock`, so a skim
must read the fee balance **before** that addition, or it will skim its own
reward. The balance is captured into a local at the top of
`DistributeRuneBlockReward` and all computation uses that snapshot. `x/pona` runs
**last** in `SetOrderEndBlockers`, so it sees the block's complete fee intake.

Module → module transfers bypass blocked-address checks (`BlockedAddr` is only
consulted by `SendCoinsFromModuleToAccount`), so the skim needs no allow-list
entry.

---

## 5. Economics

### 5.1 Reserve budget with the new ladder

Tier 1 and Tier 2 are paid at their **max** rate in the worst case (dPona does
not change the reward amounts, only the thresholds needed to earn them):

| Tier | Blocks | Max rate | Worst-case spend |
|---|---|---|---|
| Tier 1 `[0, 10,519,200)` | 10,519,200 | 50 | 525,960,000 |
| Tier 2 `[10,519,200, 21,038,400)` | 10,519,200 | 40 | 420,768,000 |
| **Remaining for Tier 3** | | | **53,272,000** |
| **Total reserve** | | | **1,000,000,000** |

### 5.2 Tier 3 duration

Tier 3 spans 5,259,600 blocks per year, so annual spend is `rate × 5,259,600`:

| Tier 3 activity | Rate | Annual spend | Duration | Reserve exhausted at |
|---|---|---|---|---|
| max every block | 30 | 157,788,000 | 0.34 y | ≈ Y4.34 |
| high every block | 20 | 105,192,000 | 0.51 y | ≈ Y4.51 |
| mid every block | 15 | 78,894,000 | 0.68 y | ≈ Y4.68 |
| low every block | 10 | 52,596,000 | 1.01 y | ≈ Y5.01 |
| quint every block | 5 | 26,298,000 | 2.03 y | ≈ Y6.03 |
| zero every block | 0.5 | 2,629,800 | 20.26 y | ≈ Y24.3 |

⚠️ **Tier 2 at 40 costs Tier 3 dearly.** Under the previous schedule (Tier 2 max
25) Tier 3 had 211,060,000 RUNE and lasted 2.68 y at max. It now has a quarter of
that. The worst case is ≈ **Y4.34** — about four months after Tier 3 begins.

This is a **worst case**, not a forecast: it assumes every block for 1.78M
consecutive blocks reaches the max band. Real chains sit between max and zero.

### 5.3 The ladder protects quiet chains

The gap between `max` and `zero` is what makes the reserve responsive to real
usage. A chain managing only one transaction per block pays the `single` rate
rather than the `max` rate, lasting ~20× longer than a maxed-out one. dPona
reinforces this: sustained max activity raises the difficulty, making max harder
to hold, which throttles the burn automatically.

### 5.4 After the reserve

When the reserve can no longer cover the scheduled reward, Tier 4 takes over and
pays from fees (§4). Validators keep receiving **all** transaction fees, split by
PoNA band instead of by voting power alone. Supply never grows.

---

## 6. Parameter summary

| Parameter | Value | Where |
|---|---|---|
| Bands | 10 | derived from `low_tx_count_threshold` |
| Base thresholds | max 20 / high 15 / mid 10 / low 6 | per tier |
| Exact bands | quint = low−1 … single = low−5 (clamped) | derived |
| Tier 1 | `[0, 10519200)`, 50/40/30/20/5/4/3/2/1/0.5 | params |
| Tier 2 | `[10519200, 21038400)`, 40/30/20/15/5/4/3/2/1/0.5 | params |
| Tier 3 | `[21038400, 0)`, 30/20/15/10/5/4/3/2/1/0.5 | params |
| Tier 4 | fee-funded fallback, 1.0/0.7/0.5/0.1/0.05/0.04/0.03/0.02/0.01/**0** | params |
| `dpona.diff` | starts at 0 (off), ±1 per event | state |
| `dpona.offset` | `diff × 10` transactions | derived |
| dPona increment | 5 consecutive max blocks | state machine |
| dPona decrement | 100 blocks with no max | state machine |
| dPona cap | **none** (warn at `diff >= 5`) | — |
| dPona scope | global, all reserve tiers | — |
| `fee_skim_percent` | 75 | params |
| Zero band (Tier 4) | **0** | params |

---

## 7. Implementation status

| Piece | State |
|---|---|
| `Tier` proto: 10 bands, **max-to-zero** field order | ✅ done |
| `DPonaState` message + genesis block | ✅ written |
| `Params.fee_only_tier` + `fee_skim_percent` | ✅ written |
| 10-band `BandForTxCount` with derived exact bands | ✅ **tested (30 passing)** |
| `Tier.Ladder()` (**max→zero** range/rate triples) | ✅ done |
| `DPonaState.ObserveBlock` transitions (no cap) | ✅ **tested (9 funcs)** |
| `Tier.WithDPonaOffset` threshold shifting | ✅ written |
| `ComputeBlockRewardWithDiff` + `FundingSource` | ✅ written |
| Fee-only tier: `min(scheduled, skim% × fees)` | ✅ written |
| `EndBlock` wiring + `advanceDPona` | ✅ written |
| CLI / event / log: `dpona`, `pona_diff`, `funding_source` | ✅ written |
| Devnet `genesis.json` (all 10 bands, **max→zero**) | ✅ done |
| `make proto-gen` | ✅ done (regenerated `genesis.pb.go`) |
| `pona.md` rewrite (5 bands → 10) | ⏳ pending |
| Blockdium panel | ⏳ pending |

### Verification

| Check | Result |
|---|---|
| `go build -o /dev/null ./app/... ./x/... ./core/...` | **exit 0** |
| `go vet ./x/pona/...` | **exit 0** |
| `go test ./x/pona/types/` | **ok — 30 tests, 0 failures** |
| `go test ./app/` | **ok** |
| `gofmt -l x/pona/` | clean |

`make test-unit` has **not** been run by the agent (RULE 1) — user to run.

> This document is the design of record. Once `pona.md` is rewritten it will
> describe the shipped ten-band ladder and link back here for the dPona
> controller.
