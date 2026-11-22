# PoNA — Proof of Network Activity (RUNE rewards)

> **Status:** Implemented as **`x/pona`**. This is the authoritative document for
> both the **mechanism** and the **module**.
>
> **Ten-band ladder + dPona.** The ladder has **ten** bands (max, high, mid, low,
> quint, quart, trio, duo, single, zero) and a **dynamic difficulty controller**
> (dPona) that shifts every band's transaction threshold by ±10. The controller,
> its state machine, and the fee-funded fallback tier are specified in
> [`dpona.md`](./dpona.md); this document covers the module and the mechanism.
> The original proposal is preserved in [`original-design.md`](./original-design.md).
>
> **Code:** `x/pona/types/pona.go` (bands, rates, floor, validation) ·
> `x/pona/types/dpona.go` (difficulty controller) ·
> `x/pona/keeper/rewards.go` (payout + shared computation) ·
> `app/abci/preblock.go` (pseudo-tx exclusion) · `app/app.go` (tx-count capture) ·
> `x/pona/client/cli/query.go` (the `query pona` commands).
>
> **Naming:** the module is **`x/pona`** — the whole RUNE reward system is
> structured around Proof of Network Activity. In user-facing prose we still say
> **"PoNA (RUNE rewards)"** so readers connect it to the RUNE token.

## 0. The module at a glance

`x/pona` owns the entire RUNE block-reward system:

- The **validator reward pool** module account (`validator_reward_pool`): funding
  at genesis, balance queries, top-up.
- The **emission schedule**: which full reward applies at a given block height.
- The **activity policy (PoNA)**: the block's transaction count selects one of
  **ten** bands, from zero to max ([§2](#2-the-activity-bands)).
- The **difficulty controller (dPona)**: sustained max-band activity raises every
  band's transaction threshold, throttling emission automatically
  ([`dpona.md`](./dpona.md)).
- **Payment plumbing**: transferring the reward from the pool into the
  `x/distribution` fee pool, which splits it across the active validator set.

Transaction fees take the **same path** — both end up in the distribution fee
pool and are split across the active validator set by voting power. See
[§8](#8-interaction-with-the-fee-market).

### The reward pool

| Property | Value |
|---|---|
| Module account | `validator_reward_pool` |
| Genesis funding | **1,000,000,000 RUNE** (bank genesis balance) |
| Source of funds | Pre-mined, not inflated — part of the fixed 10B RUNE supply |
| Supply split | 9B premine to the project owner + 1B to this reserve |
| On exhaustion | Reserve-funded emission **stops**; the fee-funded tier takes over |

The pool is a **one-way budget**: once it reaches zero, there is no reserve-funded
reward and the fee-funded ladder (Tier 4) pays from transaction fees instead. The
pool is **never overdrawn** — a block whose scheduled reward exceeds the remaining
balance switches straight to the fee ladder.

### Block lifecycle

```
EndBlock
  └─ x/pona: DistributeRuneBlockReward(ctx, proposerConsAddr)
       1. reward  = RewardForHeightAndTxCount(height, txCount)   ── PoNA lookup
       2. if pool < reward  → stop (fee-only), never overdraw
       3. send reward from validator_reward_pool → fee_collector
       4. emit rune_block_reward event (records the proposer)

Next block, x/distribution BeginBlock:
       5. sweep the fee collector → split across the ACTIVE validator set
          by voting power → delegators receive their share via commission
```

- **The reward is not sent proposer-only.** It goes to the distribution fee pool,
  exactly like transaction fees. Paying only the proposer would concentrate reward
  on whichever validator produces the block and risk block-skipping / MEV
  centralisation.
- The proposer (`ctx.BlockHeader().ProposerAddress`) is used **only for the
  emitted event**, for observability.
- The reward amount is the **PoNA** (activity-scaled) amount.

### Genesis wiring

| Item | Value |
|---|---|
| `cmd/runed/mainchain.go` | Sets the 1B reserve balance for `validator_reward_pool` |
| `app/app.go` `maccPerms` | `validator_reward_pool: nil`, `pona: {Minter}` |
| EndBlockers order | `pona` **before** `feemarket` |

## 1. Why PoNA exists

Hoodium pays a **static RUNE block reward** from a pre-funded 1B RUNE reserve
(see [§0](#0-the-module-at-a-glance)). A naive static reward pays the same
amount whether the chain is processing 10,000 transactions per block or is
completely idle.

That creates a perverse incentive:

- An **idle chain still emits RUNE** at the full rate, burning the finite reserve
  with no network activity to show for it.
- Validators have no marginal incentive to *include* transactions, since the
  reward is invariant to block content.
- The reserve is a **one-way budget** — every block spent while idle is a block
  the network cannot be rewarded for later.

**PoNA (Proof of Network Activity)** fixes this by making the block reward
**a function of how many transactions the block actually contains**. Reward
scales with real network usage:

> **More activity → full reward. Less activity → reduced reward.
> No activity → nominal (dust) reward.**

PoNA is a **reward *policy***, not a consensus change: it never blocks a
transaction, never changes block validity, and never changes who may propose.
It only decides **how much of the scheduled reward is actually minted/paid** for
a given block, based on that block's transaction count.

## 2. The activity bands

Each block is classified into exactly one of **ten bands** by its transaction
count. The bands form a graded ladder from zero to max: more activity pays more.

| Band | Condition (base thresholds) | Reward field |
|---|---|---|
| **Max** | `txCount >= max_tx_count_threshold` (20+) | `max_activity_reward` |
| **High** | `high_tx_count_threshold <= txCount < max` (15–19) | `high_activity_reward` |
| **Mid** | `mid_tx_count_threshold <= txCount < high` (10–14) | `mid_activity_reward` |
| **Low** | `low_tx_count_threshold <= txCount < mid` (6–9) | `low_activity_reward` |
| **Quint** | `txCount == low - 1` (5) | `quint_activity_reward` |
| **Quart** | `txCount == low - 2` (4) | `quart_activity_reward` |
| **Trio** | `txCount == low - 3` (3) | `trio_activity_reward` |
| **Duo** | `txCount == low - 4` (2) | `duo_activity_reward` |
| **Single** | `txCount <= low - 5` (1 and below) | `single_activity_reward` |
| **Zero** | `txCount == 0` | `zero_activity_reward` |

`txCount` is the number of **regular** transactions in the block being
finalised — *not* mempool size, *not* a rolling average, and *not* the injected
vote-extension pseudo-transaction (see [§4.1](#41-which-transactions-are-counted)).

### 2.1 The five exact-count bands are DERIVED, not stored

`Quint`, `Quart`, `Trio`, `Duo` and `Single` are computed from the **low**
threshold — they have no thresholds of their own:

```
quint  = low - 1
quart  = low - 2
trio   = low - 3
duo    = low - 4
single = low - 5   (clamped floor: absorbs every count below duo)
```

This is deliberate, and it is what makes the dPona offset safe. Storing five more
thresholds would allow an inconsistent ladder (a quint threshold above the quart
one, or a count that matches no band). Deriving them from a single anchor makes
those states **unrepresentable**: shifting `low` shifts the whole group
coherently.

`low_tx_count_threshold` must therefore be **at least 6**, so that `single` lands
on transaction count 1 rather than colliding with the zero band
(`MinLowTxCountThreshold` in `x/pona/types/pona.go`).

`Tier.Ladder()` returns the ten bands **max → zero** with their resolved count
ranges and rates, so the CLI and the block explorer render exactly what is paid
instead of recomputing the derivation. The order matches the proto field
declaration, so the genesis file reads the same way.

> **Every field is required.** Each tier carries all ten rates and all four
> thresholds. `Params.Validate` rejects a tier that omits any of them, so a
> partially-specified schedule fails at `InitChain` instead of silently paying a
> different amount at runtime. There are no optional fields and no fallbacks.

### 2.2 `zero_activity_reward` is the FLOOR

`zero_activity_reward` does double duty for every **reserve-funded** tier:

1. it is the rate for a block with **no** transactions, and
2. it is the **minimum** any block can pay.

So whatever band a block falls into, the paid amount is
`max(band_rate, zero_activity_reward)` (`Tier.RewardForTxCount` →
`Tier.ApplyFloor`).

For the shipped schedule (zero = **0.5 RUNE**, lowest other rate = **1 RUNE**)
the floor never changes an amount — every other rate is above it. It matters in
two situations:

- **Misconfiguration.** If a tier were ever set with a rate below its zero rate,
  the floor clamps the payout up so a busy block is never worth less than an idle
  one. Validation rejects that ordering on chain
  (`zero <= single <= duo <= trio <= quart <= quint <= low <= mid <= high <= max`,
  see `Tier.validatePoNA`), so the floor is a safety net, not a normal path.
  `runed query pona params` reports `floor_applied` so a clamped rate is visible.
- **A sub-RUNE zero rate.** Setting `zero_activity_reward` to `0.5 RUNE` (as
  shipped) halves the cost of a completely idle block without touching any other
  band.

> ⚠️ **The fee-only tier is the exception.** It is funded by transaction fees, so
> its zero band must be **0** — a block with no transactions collects no fees, and
> there is nothing to pay from. `validateFeeOnlyTier` enforces this. The floor
> rule above applies to reserve-funded tiers only.

The distinctions between the bands are deliberate: a block with a couple of real
transactions is *some* activity and earns a meaningfully smaller reward; a busy
block earns the max; a **completely empty** block earns only a nominal amount, so
an idle chain drains the reserve at the slowest possible rate while still paying
the proposer something for keeping the chain alive.

## 3. Per-tier rates

### 3.1 Why the first tier starts at height 0, not 1

Tier windows are **half-open intervals**: `[start_height, end_height)`. The start
is inclusive, the end is **exclusive**, and each tier begins exactly where the
previous one ended.

```
tier 1: [0,          10,519,200)   covers heights        0 .. 10,519,199
tier 2: [10,519,200, 21,038,400)   covers heights 10,519,200 .. 21,038,399
tier 3: [21,038,400, open-ended)   covers heights 21,038,400 .. forever
```

Tier 1 therefore holds exactly `10,519,200 - 0 = 10,519,200` blocks, and tier 2
exactly `21,038,400 - 10,519,200 = 10,519,200` — both 2 schedule-years.

**Why not `1 .. 10` and `11 .. 20`?** That is the *inclusive-both-ends* convention.
It cannot be chained without an assumption: tier 1 "ends at 10" and tier 2 "starts
at 11" implies height 10 belongs to tier 1 and 11 to tier 2, so **no height is
ever left uncovered only because the two ranges happen to be adjacent integers**.
The moment either number is edited the ranges can overlap or leave a hole, and
nothing in the data says which convention was intended.

There is also a hard requirement that settles it: height **0** (genesis) is a real
block that must be paid, so the schedule has to start at `0`. With a half-open
end, tier 1 covering `[0, 10519200)` ends *before* `10519200`, so tier 2 must
start at exactly `10519200` for the windows to touch. Writing `end = 10519200` and
`next start = 10519201` instead would leave height `10519200` with **no tier at
all** — which is precisely the gap the validator rejects.

### 3.2 The rate ladder

PoNA rates are defined **per emission tier**. The max rate is the ceiling owned by
`x/pona`; every other band is PoNA policy. All ten bands are listed **max → zero**,
the order `Tier.Ladder()` returns and the order the proto — and therefore the
genesis file — declares them in.

| Tier | Epoch | Max `≥20` | High `15–19` | Mid `10–14` | Low `6–9` | Quint `5` | Quart `4` | Trio `3` | Duo `2` | Single `1` | Zero `0` |
|---|---|---|---|---|---|---|---|---|---|---|---|
| 1 | Y0–2 | **50** | **40** | **30** | **20** | **5** | **4** | **3** | **2** | **1** | **0.5** |
| 2 | Y2–4 | **40** | **30** | **20** | **15** | **5** | **4** | **3** | **2** | **1** | **0.5** |
| 3 | Y4 → | **30** | **20** | **15** | **10** | **5** | **4** | **3** | **2** | **1** | **0.5** |
| 4 (fees) | after reserve | **1.0** | **0.7** | **0.5** | **0.1** | **0.05** | **0.04** | **0.03** | **0.02** | **0.01** | **0** |

All values are in **RUNE** (stored on-chain as **arune**, `1 RUNE = 10^18 arune`).
The Zero column is also the **floor** applied to every band
([§2.2](#22-zero_activity_reward-is-the-floor)) — except for Tier 4, which is
fee-funded and whose zero band is deliberately `0`.

Tier 4 is **not a height tier**. It has no start/end height, is never part of
`params.tiers`, and is paid once the reserve can no longer cover the scheduled
reward. Its payout is clamped to the block's actual fee income:

```
paid = min(scheduled_reward, fee_skim_percent% × block_fees)
```

See [`dpona.md` §4](./dpona.md#4-tier-4--the-fee-funded-fallback) for the full
rationale, including why it is a fallback rather than a fourth height tier.

In arune, the tier-1 rates are:

```
zero_activity_reward    = 500000000000000000        (0.5 RUNE)
single_activity_reward  = 1000000000000000000       (1 RUNE)
duo_activity_reward     = 2000000000000000000       (2 RUNE)
trio_activity_reward    = 3000000000000000000       (3 RUNE)
quart_activity_reward   = 4000000000000000000       (4 RUNE)
quint_activity_reward   = 5000000000000000000       (5 RUNE)
low_activity_reward     = 20000000000000000000      (20 RUNE)
low_tx_count_threshold  = 6
mid_activity_reward     = 30000000000000000000      (30 RUNE)
mid_tx_count_threshold  = 10
high_activity_reward    = 40000000000000000000      (40 RUNE)
high_tx_count_threshold = 15
max_activity_reward     = 50000000000000000000      (50 RUNE)
max_tx_count_threshold  = 20
```

Notes:

- Tier 3 runs **from year 4 forever** (open-ended). It is the terminal tier of the
  reserve-funded schedule; PoNA only shapes it for as long as the reserve lasts,
  after which Tier 4 takes over.
- The rates are **not** a fixed fraction of the max across tiers, and not a fixed
  fraction of each other (tier 1: 50→40→30→20→5→4→3→2→1→0.5). They are therefore
  stored as **explicit per-tier fields**, not derived by a multiplier. This is
  intentional — a derived model could not express the flat `5/4/3/2/1 RUNE`
  exact-count bands, which are deliberately far below any percentage of the max.
- The five exact-count bands carry the **same amounts in every reserve tier**
  (5/4/3/2/1). Only the four activity bands (max/high/mid/low) taper across tiers.
  That keeps "a handful of transactions" worth roughly the same throughout the
  emission schedule while the ceiling falls.

## 4. How it works — block lifecycle

PoNA needs the block's transaction count at the moment the reward is decided.
In Cosmos SDK v0.50 the SDK `Context` at `EndBlock` does **not** expose the
block's transaction list, so the count is captured earlier and stashed in the
store:

```
FinalizeBlock (ABCI)
│
├─ PreBlock        ── app-level PreBlocker(ctx, req) captures len(req.Txs)
│                     ──► x/pona keeper: store "current block tx count"
│                     (req is visible ONLY here; module-level PreBlock(ctx)
│                      hooks receive just an sdk.Context, and EndBlock cannot
│                      see the block's transactions at all)
│
├─ BeginBlock      ── (no-op for the reward path)
│
├─ DeliverTx …     ── transactions execute as normal (PoNA does not interfere)
│
└─ EndBlock
   ├─ x/pona ── reads the stored tx count,
   │                  classifies the band at the current dPona difficulty,
   │                  pays the *effective* reward:
   │                    · resolve normally       → from validator_reward_pool
   │                    · reserve < reward       → fee-only ladder instead,
   │                                               clamped to fee_skim_percent%
   │                                               of this block's fees
   │                    · reward is zero         → pay nothing
   │               → into the fee collector (fee_collector), never proposer-only
   ├─ x/pona ── advances the dPona controller (+1 per 5 consecutive max blocks,
   │                  −1 per 100 blocks without a max block) for the NEXT block
   └─ (emits rune_block_reward with tx_count, band, funding_source, dpona, pona_diff)
```

Key properties:

1. **Deterministic.** The tx count is a property of the block, identical on every
   node, so every node computes the same reward. PoNA introduces no new consensus
   surface beyond the stored count.
2. **Proposer-only payout.** Like the base reward, the PoNA-scaled reward is paid
   to the **block proposer's operator address** (from
   `ctx.BlockHeader().ProposerAddress`). Delegators receive their share through the
   proposer's commission split via `x/distribution`.
3. **Never blocks/alters transactions.** PoNA is a pure reward-scaling policy.
4. **Fails safe.** If no tx count has been recorded for the block (e.g. genesis, or
   a store miss), `BlockTxCount` returns `0`, so the block is paid the **zero**
   band rather than the max rate. It can never over-pay.

### 4.1 Which transactions are counted?

**Regular chain transactions only.** The app-level injected
**pseudo-transaction** that vote extensions put at index 0 of every block is
**excluded**, because it is not a transaction: `BaseApp` never delivers it (it
does not decode as an `sdk.Tx`), and counting it would make every block look like
it had ≥1 tx, making the Zero band unreachable.

Hoodium enables vote extensions at **height 1** (genesis consensus params), so
this exclusion is not theoretical — before it existed, every block on an idle
chain was counted as `tx_count = 1` and paid the **low-activity** rate.

The split lives in `app/abci/preblock.go`:

| Function | Responsibility |
|---|---|
| `SplitBlockTxs` | separates the injected pseudo-tx from the regular txs |
| `CountRegularTxs` | the count stored for PoNA (`app.Hoodium.PreBlocker`) |

Detection requires **both** conditions, which is what makes it safe:

1. vote extensions are enabled at that height (`isVoteExtensionsEnabled`), and
2. the payload decodes as an `InjectedTx` **with at least one part**
   (`len(injectedTx.Parts) > 0`).

Condition 2's "at least one part" clause is not cosmetic: gogoproto skips
unknown fields, so `InjectedTx.Unmarshal` of an arbitrary payload can succeed
with an **empty** `Parts` map. Treating that as a pseudo-tx would silently drop a
real transaction from the count. The pre-block handler applies the same check
before running its sub-handlers.

As a final guard, `CountRegularTxs` is given the app's tx decoder
(`app.TxDecode`): a payload that decodes as a valid `sdk.Tx` is always counted as
a real transaction, even if it also parses as an `InjectedTx`.

> **`num_txs=1` in the CometBFT log is expected** and is *not* a PoNA bug.
> `finalizing commit of block ... num_txs=1` counts every entry in the proposal,
> and `finalized block ... num_txs_res=1` is the matching per-tx result from
> `BaseApp`. Both count the pseudo-tx because it *is* an entry in the block. To
> see what PoNA counted, use `runed query pona tx-count`.

## 5. Running example

Assume the base thresholds (`max` = 20, `high` = 15, `mid` = 10, `low` = 6),
dPona **off** (`pona_diff = 0`), 6-second blocks (5,259,600 blocks/year), and the
three reserve tiers of [§3](#3-per-tier-rates).

| Block | Height epoch | `txCount` | Band | Reward |
|---|---|---|---|---|
| A | Y1 | 42 | Max | **50 RUNE** |
| B | Y1 | 17 | High | **40 RUNE** |
| B1 | Y1 | 12 | Mid | **30 RUNE** |
| B2 | Y1 | 7 | Low | **20 RUNE** |
| B3 | Y1 | 5 | Quint | **5 RUNE** |
| B4 | Y1 | 2 | Duo | **2 RUNE** |
| B5 | Y1 | 1 | Single | **1 RUNE** |
| C | Y1 | 0 | Zero | **0.5 RUNE** |
| D | Y3 | 25 | Max | **40 RUNE** |
| E | Y3 | 16 | High | **30 RUNE** |
| E1 | Y3 | 11 | Mid | **20 RUNE** |
| E2 | Y3 | 8 | Low | **15 RUNE** |
| E3 | Y3 | 1 | Single | **1 RUNE** |
| E4 | Y3 | 0 | Zero | **0.5 RUNE** |
| F | Y5 | 128 | Max | **30 RUNE** |
| G | Y5 | 18 | High | **20 RUNE** |
| G1 | Y5 | 14 | Mid | **15 RUNE** |
| G2 | Y5 | 9 | Low | **10 RUNE** |
| G3 | Y5 | 4 | Quart | **4 RUNE** |
| H | Y5 | 0 | Zero | **0.5 RUNE** |

With **dPona active at `pona_diff = 1`** every threshold moves up by 10, so the
same transaction counts fall into lower bands:

| Block | Height epoch | `txCount` | Band at diff 1 | Reward |
|---|---|---|---|---|
| A′ | Y1 | 42 | Max (needs 30) | **50 RUNE** |
| B″ | Y1 | 29 | High (25–29) | **40 RUNE** |
| B‴ | Y1 | 20 | Mid (20–24) | **30 RUNE** |
| C″ | Y1 | 5 | Single (≤ 11) | **1 RUNE** |

That last row is the point of dPona: at base difficulty 5 transactions earn
`quint` (5 RUNE); at `diff = 1` the same block earns `single` (1 RUNE). Reward
amounts never change — only how much activity is needed to reach them.

## 6. Economic effect on the 1B reserve

The reserve is a fixed **1,000,000,000 RUNE**. Tier 1 and Tier 2 are *not*
PoNA-dependent in the worst case — they always consume their full budget at their
**max** rate:

| Tier | Blocks | Max rate | RUNE spent (worst case) |
|---|---|---|---|
| Tier 1 (Y0–2) | 10,519,200 | 50 | 525,960,000 |
| Tier 2 (Y2–4) | 10,519,200 | 40 | 420,768,000 |
| **Total** | | | **946,728,000** |
| **Remaining for Tier 3** | | | **53,272,000** |

Tier 3 (max 30 / high 20 / mid 15 / low 10 / 5 / 4 / 3 / 2 / 1 / 0.5) then drains
the remainder at a rate set by how busy the chain is. Tier 3 spans **5,259,600
blocks/year**, so annual spend is `rate × 5,259,600`:

| Chain activity | Tier-3 rate | Annual spend | Tier-3 duration | Reserve exhausted at |
|---|---|---|---|---|
| **Max** — every block `tx >= 20` | 30 | 157,788,000 RUNE/y | 53.27M ÷ 157.79M ≈ **0.34 y** | **≈ Y4.34** |
| **High** — every block `15–19` | 20 | 105,192,000 RUNE/y | ≈ **0.51 y** | ≈ Y4.51 |
| **Mid** — every block `10–14` | 15 | 78,894,000 RUNE/y | ≈ **0.68 y** | ≈ Y4.68 |
| **Low** — every block `6–9` | 10 | 52,596,000 RUNE/y | ≈ **1.01 y** | ≈ Y5.01 |
| **Quint** — every block exactly `5` | 5 | 26,298,000 RUNE/y | ≈ **2.03 y** | ≈ Y6.03 |
| **Quart** — every block exactly `4` | 4 | 21,038,400 RUNE/y | ≈ **2.53 y** | ≈ Y6.53 |
| **Trio** — every block exactly `3` | 3 | 15,778,800 RUNE/y | ≈ **3.38 y** | ≈ Y7.38 |
| **Duo** — every block exactly `2` | 2 | 10,519,200 RUNE/y | ≈ **5.06 y** | ≈ Y9.06 |
| **Single** — every block exactly `1` | 1 | 5,259,600 RUNE/y | ≈ **10.13 y** | ≈ Y14.1 |
| **Zero** — every block `tx == 0` | 0.5 | 2,629,800 RUNE/y | ≈ **20.26 y** | ≈ Y24.3 |

> ⚠️ **Tier 2 at 40 RUNE costs Tier 3 dearly.** Under the previous schedule
> (Tier 2 max = 25) Tier 3 had **211,060,000 RUNE** and lasted **2.68 y** at max
> rate, putting final exhaustion at **≈ Y6.7**. It now has a quarter of that, and
> a permanently-maxed chain exhausts the reserve **≈ Y4.34** — about four months
> after Tier 3 begins.
>
> **This is a worst case, not a forecast.** It assumes every single block for
> 1,775,733 consecutive blocks reaches the max band (20+ transactions). Real
> chains sit somewhere between max and zero.
>
> **dPona is the intended mitigation.** The controller raises the max threshold by
> 10 transactions after five consecutive max blocks, so a chain hot enough to
> sustain 20+ tx/block gets pushed toward 30, then 40 — throttling the burn
> automatically. That is precisely the "max won't be hit every block" argument
> behind the Tier 2 rate. The exposure remains if activity is high but *spiky*
> (max every block, never five in a row), so watch the reserve once Tier 3 begins.

The graded ladder is what makes the reserve respond smoothly to real usage instead
of in one big jump: a chain running mostly 10–14 tx/block burns it at half the rate
of a maxed-out one, and a chain with a single stray tx per block lasts 30× longer
than a busy one.

**Read this table carefully.** The ladder is deliberately steep at the top: a
fully-loaded Tier 3 (30 RUNE/block) exhausts the reserve by **≈ Y4.34**, while a
chain that only manages a single transaction per block lasts to **≈ Y14**. PoNA
therefore **does not lengthen** the busy-network lifetime — it **protects the
reserve on quiet chains** (out to ~Y24 when completely idle), which is precisely
the intended behaviour: emission is throttled by real usage.

The `single` band is what stops a barely-used chain from being paid like a *busy*
one: one transaction earns `single_activity_reward` (1 RUNE in tier 3), not the
10-RUNE low rate, and not the 0.5-RUNE empty rate.

**Fee-only end state:** when the reserve reaches zero, the reserve-funded
schedules stop. From that block on **Tier 4** takes over — the same ten-band
ladder, paid from the block's own transaction fees and clamped to
`fee_skim_percent%` of them ([§3.2](#32-the-rate-ladder)). No new money is
created: those coins were already destined for validators through
`x/distribution`, so Tier 4 changes the *split*, not the *income*. Transaction
fees continue to be distributed by voting power exactly as before.

## 7. Module structure

PoNA is implemented **inside the `x/pona` module**, split along a
**money / policy** line:

| Concern | Owns |
|---|---|
| **Money** | The 1B `validator_reward_pool` (genesis funding, balance, top-up) and the coin-transfer plumbing into the fee pool |
| **Policy (PoNA)** | The activity bands, the low/zero rates, and the effective-reward decision |

In short: **the pool is the wallet; the PoNA bands are the rule that decides how
much comes out of it each block.**

Historically these were two concepts (`x/runerewards` = money, a separate `x/pona`
= policy). They are now one module — the whole RUNE reward system *is* PoNA — so
the module, its proto package and its docs all use the **`pona`** name. Splitting
it back into two modules is not planned.

### 7.1 Where each parameter lives (implemented)

| Field | Proto | Meaning |
|---|---|---|
| `tiers[].start_height` | `Tier` | First height the tier applies to, **inclusive** |
| `tiers[].end_height` | `Tier` | First height the tier stops applying at, **exclusive** (`0` = open-ended) |
| `tiers[].max_activity_reward` | `Tier` | **Max** rate, paid at/above `max_tx_count_threshold` |
| `tiers[].max_tx_count_threshold` | `Tier` | Txs at/above which the max rate is paid (`20`); **required**, `high < max` |
| `tiers[].high_activity_reward` | `Tier` | Rate when `high <= tx < max`; **required** |
| `tiers[].high_tx_count_threshold` | `Tier` | Lower bound of the high band (`15`); **required**, `mid < high < max` |
| `tiers[].mid_activity_reward` | `Tier` | Rate when `mid <= tx < high`; **required** |
| `tiers[].mid_tx_count_threshold` | `Tier` | Lower bound of the mid band (`10`); **required**, `low < mid < high` |
| `tiers[].low_activity_reward` | `Tier` | Rate when `low <= tx < mid`; **required** |
| `tiers[].low_tx_count_threshold` | `Tier` | Lower bound of the low band (`6`) **and the anchor the exact bands are derived from**; **required**, at least 6 |
| `tiers[].quint_activity_reward` | `Tier` | Rate when `tx == low - 1` (5); **required** |
| `tiers[].quart_activity_reward` | `Tier` | Rate when `tx == low - 2` (4); **required** |
| `tiers[].trio_activity_reward` | `Tier` | Rate when `tx == low - 3` (3); **required** |
| `tiers[].duo_activity_reward` | `Tier` | Rate when `tx == low - 4` (2); **required** |
| `tiers[].single_activity_reward` | `Tier` | Rate when `tx <= low - 5` (1 and below); **required** |
| `tiers[].zero_activity_reward` | `Tier` | Rate when `tx == 0`; also the **floor** for every band |
| `fee_only_tier` | `Params` | The fee-funded ladder ("Tier 4"). No start/end height; zero band **must be 0** |
| `fee_skim_percent` | `Params` | Max share of a block's fees the fee-only ladder may pay (`75`) |
| `dpona` | `GenesisState` | Persisted `DPonaState`: `consecutive_max`, `blocks_since_max`, `diff` |

The tx count itself is **not** a parameter: it is captured per block from
`len(req.Txs)` in the app's `PreBlocker` and stored under
`pona/types.KeyBlockTxCount`. The dPona controller state is stored separately
under `pona/types.KeyDPona`.

> **Field numbering.** The band fields are numbered and declared **max → zero**,
> so the genesis file reads as a ladder top-to-bottom, busiest band first:
> `max_activity_reward + max_tx_count_threshold, high_… + its threshold,
> mid_… + its threshold, low_… + its threshold, quint_…, quart_…, trio_…, duo_…,
> single_…, zero_activity_reward`. Declaration order is also the order the
> generated Go struct — and therefore the genesis JSON — lists them.
>
> The ten-band renumbering **deliberately broke wire compatibility** with the
> five-band layout. That was safe because there was no live chain (devnet only);
> on a chain with history the fields would have had to be appended instead.

### 7.2 Code map

| Piece | Location |
|---|---|
| PoNA bands, rates, validation | `x/pona/types/pona.go` |
| Band selection | `Tier.BandForTxCount` (`pona.go`) |
| Band → raw rate | `Tier.RewardForBand` (`pona.go`) |
| Zero-rate floor | `Tier.ApplyFloor` (`pona.go`) |
| Tx count → paid reward | `Tier.RewardForTxCount` (`pona.go`) |
| Height + tx count → reward | `Params.RewardForHeightAndTxCount` (`pona.go`) |
| Height + tx count → band | `Params.BandForHeightAndTxCount` (`pona.go`) |
| Height → tier (with index) | `Params.TierForHeight` (`schedule.go`) |
| Shared reward computation | `keeper.ComputeBlockReward` (`keeper/rewards.go`) |
| Tx-count capture | `app.Hoodium.PreBlocker` → `appabci.CountRegularTxs` → `keeper.SetBlockTxCount` |
| Pseudo-tx exclusion | `appabci.SplitBlockTxs` (`app/abci/preblock.go`) |
| Reward payout | `Keeper.DistributeRuneBlockReward` (`keeper/rewards.go`) |
| CLI queries | `x/pona/client/cli/query.go` |
| Consensus version | `pona/types.ConsensusVersion` = **2** |

### 7.3 File layout

PoNA lives in its own file, **`x/pona/types/pona.go`**, separate from the
generic schedule (`schedule.go`) and the default parameters (`params.go`). A
dedicated `x/pona` **module** is not used, but the code is isolated so the
mechanism has a single reference point:

| File | Responsibility |
|---|---|
| `types/pona.go` | **All PoNA**: the ten bands, rates, `BandForTxCount`, `Ladder`, `RewardForBand`, `ApplyFloor`, `RewardForTxCount`, `WithDPonaOffset`, `RewardForHeightTxCountAndDiff`, `BandForHeightTxCountAndDiff`, `validatePoNA`, `DefaultMaxTxCountThreshold`, `DefaultHighTxCountThreshold`, `DefaultMidTxCountThreshold`, `DefaultLowTxCountThreshold`, `MinLowTxCountThreshold` |
| `types/dpona.go` | **The dPona controller**: `DPonaState`, `ObserveBlock`, `IsActive`, `TxCountOffset`, `Validate`, and the step/threshold constants |
| `types/schedule.go` | Generic tier lookup (`RewardForHeight`, `TierForHeight`, `tierForHeight`) |
| `types/params.go` | `DefaultTiers`/`DefaultFeeOnlyTier`/`DefaultParams`/`Validate`/`validateFeeOnlyTier` + RUNE helpers (`RuneAmount`, `RuneString`) |
| `types/pona_test.go` | Band/floor/validation tests |
| `types/dpona_test.go` | Ladder derivation + offset tests (exhaustive 0–60 × diff 0–3) |
| `types/dpona_state_test.go` | Difficulty-controller transition tests |
| `types/schedule_test.go` | Schedule/boundary tests |
| `keeper/rewards.go` | `DistributeRuneBlockReward`, `ComputeBlockReward[WithDiff]`, `FundingSource` (shared by payout and CLI) |
| `keeper/keeper.go` | `Get/Set/UpdateDPona`, `FeeOnlyReward`, `blockFeeIncome`, `advanceDPona`, `logRewardPaid` |
| `client/cli/query.go` | `runed query pona {params,tier,block-reward,tx-count,pool}` |

## 8. Interaction with the fee market

The two-gas-price model ([`../../tx-fees.md`](../../tx-fees.md)) is
**independent** of PoNA for the reserve-funded tiers:

| Flow | Source | Recipient | Shaped by PoNA? |
|---|---|---|---|
| Reserve block reward (Tiers 1–3) | 1B reserve (`x/pona`) | Fee collector → all active validators by power | **Yes** — by tx count only |
| Transaction fees | Fee pool (`x/distribution`) | All active validators by power | No |
| **Fee-only reward (Tier 4)** | **the same fee pool** | Fee collector → all active validators by power | **Yes** — by tx count, clamped to `fee_skim_percent%` of block fees |

For Tiers 1–3, PoNA must **not** key off gas used or fee revenue — only the
**transaction count** — so the two mechanisms stay decoupled.

Tier 4 is the one deliberate coupling: once the reserve is spent, the ladder is
paid *from* fees. This does not create money and does not reduce what validators
receive in total — it only changes the **split rule** from "purely by voting
power" to "PoNA band, capped at a percentage of the block's fees". The remainder
still flows through `x/distribution` by power exactly as before.

> **Ordering hazard (already handled).** PoNA runs **last** in `SetOrderEndBlockers`,
> so it sees the block's complete fee intake. The fee balance is snapshotted
> *before* PoNA credits any reward into the collector — otherwise the skim would
> pay itself. See `Keeper.blockFeeIncome` in `x/pona/keeper/keeper.go`.

## 9. Querying the module

`x/pona` still has **no gRPC Query service**, but that is no longer a limitation
for operators: `runed query pona` reads the module's KV store directly through
the `store/pona/key` ABCI path, so every question about the reward system is
answerable without a new protobuf or a chain upgrade.

| Command | Answers |
|---|---|
| `runed query pona params` | the whole emission schedule + all ten band rates per tier, plus the fee-only ladder and `fee_skim_percent` |
| `runed query pona tier` | which tier is active **now**, and how far away the next one is |
| `runed query pona block-reward` | what a block pays **now**, which band, at which dPona difficulty, and from which funding source |
| `runed query pona block-reward --tx-count 25` | what a block with 25 txs *would* pay |
| `runed query pona tx-count` | the regular-tx count PoNA used for the last block, **plus** the block's raw entry count |
| `runed query pona pool` | RUNE left in `validator_reward_pool`, and how long it lasts |
| `runed query pona dpona` | the difficulty-controller state: on/off, `pona_diff`, the countdown to the next change, and the **effective threshold of every band** at the current difficulty |
| `runed query pona pool` | RUNE left in `validator_reward_pool`, and how long it lasts |

Typical output:

```console
$ runed query pona dpona
active: false
blocks_since_max: 0
consecutive_max: 0
decrement_after_blocks: 100
effective_thresholds:
  duo: 2
  high: 15
  low: 6
  max: 20
  mid: 10
  quart: 4
  quint: 5
  single: 1
  trio: 3
height: 252
increment_after_blocks: 5
next_decrement_in_blocks: 0
next_increment_in_blocks: 5
note: 'dPona is OFF: the base ladder applies (max needs 20 regular txs)'
pona_diff: 0
tx_count_step: 10
txs_offset: 0
warn: false
```

With dPona active at `pona_diff: 2` the same command shows `txs_offset: 40`,
`warn: true`, and every entry under `effective_thresholds` raised by 20 (the
`max` band would need 40 transactions).

```console
$ runed query pona block-reward
band: zero
band_explanation: the block contains no regular transactions, so the zero-activity
  rate of 0.5 RUNE applies
denom: arune
height: 252
pool_balance: "999999873000000000000000000"
pool_balance_rune: "999999873"
pool_sufficient: true
reward: "500000000000000000"
reward_rune: "0.5"
tier:
  ends_at_height: 10519200
  ends_at_height_meaning: exclusive
  floor_applied: false
  high:
    arune: "40000000000000000000"
    rune: "40"
  high_tx_count_threshold: 15
  low:
    arune: "20000000000000000000"
    rune: "20"
  low_tx_count_threshold: 6
  max:
    arune: "50000000000000000000"
    rune: "50"
  max_tx_count_threshold: 20
  mid:
    arune: "30000000000000000000"
    rune: "30"
  mid_tx_count_threshold: 10
  quint:  { arune: "5000000000000000000", rune: "5" }
  quart:  { arune: "4000000000000000000", rune: "4" }
  trio:   { arune: "3000000000000000000", rune: "3" }
  duo:    { arune: "2000000000000000000", rune: "2" }
  single:
    arune: "1000000000000000000"
    rune: "1"
  start_height: 0
  tier_count: 3
  tier_index: 0
  zero:
    arune: "500000000000000000"
    rune: "0.5"
tx_count: 0
tx_count_source: chain (recorded for the last finalized block)
```

Read it as: **the chain is idle, and it is paying 0.5 RUNE — not 15.** That is the
behaviour the zero band exists for.

`tx-count` is the command that reconciles the node log with the reward. It prints
**both** counts, because they answer different questions:

```console
$ runed query pona tx-count
band: zero
block_tx_count: 1
block_tx_count_note: every entry in the block, including the injected pseudo-tx
  (CometBFT num_txs)
height: 253
low_tx_count_threshold: 6
max_tx_count_threshold: 20
mid_tx_count_threshold: 10
high_tx_count_threshold: 15
note: regular transactions only; the injected vote-extension pseudo-tx is excluded
reward: "500000000000000000"
reward_rune: "0.5"
tier_index: 0
tx_count: 0
```

| Field | Meaning |
|---|---|
| `tx_count` | **what PoNA counted** — regular transactions only; selected the band |
| `block_tx_count` | **every entry in the block**, including the pseudo-tx; equals CometBFT's `num_txs` |

On a vote-extension chain the two differ by exactly the one injected pseudo-tx, so
`block_tx_count = tx_count + 1`. Pass `--height H` to inspect an older block:

```console
$ runed query pona tx-count --height 109   # a block that held one real tx
band: single
block_tx_count: 2      # pseudo-tx + 1 real tx
tx_count: 1            # what PoNA counted → single band → 1 RUNE
```

`block_tx_count: 1` with `tx_count: 0` on an idle chain is the **expected, healthy
case** — never a bug.

## 10. Open questions / follow-ups

1. **Pseudo-tx counting** — **resolved**. The injected pseudo-transaction is
   excluded via `appabci.CountRegularTxs`, so an idle block counts as
   `tx_count = 0` and reaches the zero band
   (see [§4.1](#41-which-transactions-are-counted)).
2. **Threshold configurability** — the four thresholds
   (`low_tx_count_threshold`, `mid_tx_count_threshold`,
   `high_tx_count_threshold`, `max_tx_count_threshold`) are per-tier on-chain
   fields (mutable via params), defaulting to `6 / 10 / 15 / 20`. The five
   exact-count bands (quint..single) are **derived from `low`**, so they are not
   separately configurable by design. Governance-adjustable in practice; no
   dedicated gov Msg exists yet.
3. **Zero-band floor** — **resolved**: `min_reward_per_block` was removed so a
   sub-1-RUNE zero-activity reward is expressible. The zero rate is **0.5 RUNE**
   for reserve tiers and also acts as the **floor** of every band (see
   [§2.2](#22-zero_activity_reward-is-the-floor)). The fee-only tier's zero band
   is `0` by design.
4. **Standalone `x/pona` module** — deferred. The policy is implemented inside
   `x/pona`; splitting it later is a refactor, not a behaviour change
   (see [§7](#7-module-structure)).
5. **Genesis / first block** — before any `PreBlocker` has run, the stored tx
   count is absent and `BlockTxCount` returns `0` (the zero-activity band).
6. **gRPC Query service** — not implemented, and not needed for operators
   (`runed query pona` reads the store directly, see [§9](#9-querying-the-module)).
   A real `Query` service is only worth adding when a proto-breaking change is
   being made anyway.
7. **Tier 2 = 40 RUNE exposure** — see the warning in [§6](#6-economic-effect-on-the-1b-reserve).
   A permanently-maxed chain exhausts the reserve at **≈ Y4.34**. dPona throttles
   this for sustained load, but spiky traffic (max every block, never five in a
   row) is not caught by the controller. **Watch the reserve once Tier 3 begins**
   and be prepared either to lower Tier 2's max or to accept an earlier handover
   to the fee-funded tier.
8. **dPona observability** — **resolved**: `runed query pona dpona` reports the
   controller state (on/off, `pona_diff`, countdowns to the next transition, and
   every band's effective threshold at the current difficulty). The same fields
   appear in the per-block log line and the `rune_block_reward` event.

---

#### Credits: PoNA Mechanism is designed and built by Evorz Labs
