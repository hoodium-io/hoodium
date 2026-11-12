# PoNA — Proof of Network Activity (RUNE rewards)

> **Status:** Implemented as **`x/pona`**. This is the authoritative document for
> both the **mechanism** and the **module**.
>
> **Code:** `x/pona/types/pona.go` (bands, rates, floor, validation) ·
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
- The **activity policy (PoNA)**: the block's transaction count selects the full,
  low, single, or zero rate.
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
| On exhaustion | Emission stops (fee-only); the pool is **never overdrawn** |

The pool is a **one-way budget**: once it reaches zero, there is no static reward
and the chain runs on fees permanently.

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

Each block is classified into exactly one of **five bands** by its transaction
count. The bands form a graded ladder: more activity pays more.

| Band | Condition (default thresholds) | Reward paid |
|---|---|---|
| **Max** | `txCount >= max_tx_count_threshold` (10+) | `max_activity_reward` |
| **Mid** | `mid_tx_count_threshold <= txCount < max` (5–9) | `mid_activity_reward` |
| **Low** | `low_tx_count_threshold <= txCount < mid` (2–4) | `low_activity_reward` |
| **Single** | `txCount == 1` | `single_activity_reward` |
| **Zero** | `txCount == 0` | `zero_activity_reward` |

`txCount` is the number of **regular** transactions in the block being
finalised — *not* mempool size, *not* a rolling average, and *not* the injected
vote-extension pseudo-transaction (see [§4.1](#41-which-transactions-are-counted)).

> **Every field is required.** Each tier carries all five rates and all three
> thresholds. `Params.Validate` rejects a tier that omits any of them, so a
> partially-specified schedule fails at `InitChain` instead of silently paying a
> different amount at runtime. There are no optional fields and no fallbacks.

### 2.1 `zero_activity_reward` is the FLOOR

`zero_activity_reward` does double duty:

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
  (`zero <= single <= low <= mid <= max`, see `Tier.validatePoNA`), so the floor
  is a safety net, not a normal path. `runed query pona params` reports
  `floor_applied` so a clamped rate is visible.
- **A sub-RUNE zero rate.** Setting `zero_activity_reward` to `0.5 RUNE` (as
  shipped) halves the cost of a completely idle block without touching any other
  band.

> **Note on "threshold 0":** a tier whose `max_tx_count_threshold` is **0** has
> PoNA *switched off* and always pays the max rate. Zero is a sentinel meaning
> "disabled" — it is **not** a claim that the threshold is 0 transactions. The
> live max threshold is **10** (`DefaultTxCountThreshold`).

The distinctions between the bands are deliberate: a block with a couple of real
transactions is *some* activity and earns a meaningful reduced reward; a busy
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
`x/pona`; the Mid, Low, Single and Zero rates are PoNA policy.

| Tier | Epoch | Max (`tx >= 10`) | Mid (`5–9`) | Low (`2–4`) | Single (`tx == 1`) | Zero (`tx == 0`) |
|---|---|---|---|---|---|---|
| 1 | Y0–2 | **50 RUNE** | **25 RUNE** | **15 RUNE** | **1 RUNE** | **0.5 RUNE** |
| 2 | Y2–4 | **25 RUNE** | **15 RUNE** | **10 RUNE** | **1 RUNE** | **0.5 RUNE** |
| 3 | Y4 → infinite | **15 RUNE** | **10 RUNE** | **5 RUNE** | **1 RUNE** | **0.5 RUNE** |

All values are in **RUNE** (stored on-chain as **arune**, `1 RUNE = 10^18 arune`).
The Zero column is also the **floor** applied to every band ([§2.1](#21-zero_activity_reward-is-the-floor)).

In arune, the tier-1 rates are:

```
max_activity_reward    = 50000000000000000000   (50 RUNE)
max_tx_count_threshold = 10
mid_activity_reward    = 25000000000000000000   (25 RUNE)
mid_tx_count_threshold = 5
low_activity_reward    = 15000000000000000000   (15 RUNE)
low_tx_count_threshold = 2
single_activity_reward = 1000000000000000000    (1 RUNE)
zero_activity_reward   = 500000000000000000     (0.5 RUNE)
```

Notes:

- Tier 3 runs **from year 4 forever** (open-ended). It is the terminal tier of the
  reserve-funded schedule; PoNA only shapes it for as long as the reserve lasts.
- The rates are **not** a fixed fraction of the max across tiers, and not a fixed
  fraction of each other (tier 1: 50→25→15→1→0.5). They are therefore stored as
  **explicit per-tier fields**, not derived by a multiplier. This is intentional —
  a derived model could not express `1 RUNE` for the Single band or `0.5 RUNE` for
  Zero, which are below any sane percentage of the max.
  which is below the reserve's old 5-RUNE floor.

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
   ├─ x/pona ── reads the stored tx count, classifies the band,
   │                  pays the *effective* reward from validator_reward_pool
   │                  → to the block proposer's operator address
   └─ (emits rune_block_reward with the tx_count attribute)
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

Assume the default thresholds (`max` = 10, `mid` = 5, `low` = 2), 6-second blocks
(5,259,600 blocks/year), and the three tiers of [§3](#3-per-tier-rates).

| Block | Height epoch | `txCount` | Band | Reward |
|---|---|---|---|---|
| A | Y1 | 42 | Max | **50 RUNE** |
| B | Y1 | 7 | Mid | **25 RUNE** |
| B2 | Y1 | 3 | Low | **15 RUNE** |
| B3 | Y1 | 1 | Single | **1 RUNE** |
| C | Y1 | 0 | Zero | **0.5 RUNE** |
| D | Y3 | 15 | Max | **25 RUNE** |
| E | Y3 | 6 | Mid | **15 RUNE** |
| E2 | Y3 | 4 | Low | **10 RUNE** |
| E3 | Y3 | 1 | Single | **1 RUNE** |
| E4 | Y3 | 0 | Zero | **0.5 RUNE** |
| F | Y5 | 128 | Max | **15 RUNE** |
| G | Y5 | 8 | Mid | **10 RUNE** |
| G2 | Y5 | 2 | Low | **5 RUNE** |
| G3 | Y5 | 1 | Single | **1 RUNE** |
| H | Y5 | 0 | Zero | **0.5 RUNE** |

## 6. Economic effect on the 1B reserve

The reserve is a fixed **1,000,000,000 RUNE**. Tier 1 and Tier 2 are *not*
PoNA-dependent in the worst case — they always consume their full budget:

| Tier | Blocks | Rate | RUNE spent |
|---|---|---|---|
| Tier 1 (Y0–2) | 10,519,200 | 50 | 525,960,000 |
| Tier 2 (Y2–4) | 10,519,200 | 25 | 262,980,000 |
| **Remaining for Tier 3** | | | **211,060,000** |

Tier 3 (max 15 / mid 10 / low 5 / single 1 / zero 0.5) then drains the remainder at
a rate set by how busy the chain is:

| Chain activity | Tier-3 rate | Annual spend | Tier-3 duration | Fee-only reached at |
|---|---|---|---|---|
| **Max** — every block `tx >= 10` | 15 | 78,894,000 RUNE/y | 211.06M ÷ 78.894M ≈ **2.68 y** | **≈ Y6.7** |
| **Mid** — every block `5 <= tx < 10` | 10 | 52,596,000 RUNE/y | 211.06M ÷ 52.596M ≈ **4.01 y** | **≈ Y8.0** |
| **Low** — every block `2 <= tx < 5` | 5 | 26,298,000 RUNE/y | 211.06M ÷ 26.298M ≈ **8.03 y** | **≈ Y12.0** |
| **Single** — every block exactly `tx == 1` | 1 | 5,259,600 RUNE/y | 211.06M ÷ 5.2596M ≈ **40.1 y** | **≈ Y44.1** |
| **Zero** — every block `tx == 0` | 0.5 | 2,629,800 RUNE/y | 211.06M ÷ 2.6298M ≈ **80.3 y** | **≈ Y84.3** |

The Tier-3 budget is `211,060,000 RUNE` and Tier 3 spans `5,259,600` blocks/year,
so the annual spend is `rate × 5,259,600` RUNE. The graded ladder is what makes
the reserve respond smoothly to real usage instead of in one big jump: a chain
running mostly 5–9 tx/block burns it at half the rate of a maxed-out one, and a
chain with a single stray tx per block lasts 15× longer than a busy one.

**Read this table carefully.** The ladder is deliberately steep at the top: a
fully-loaded Tier 3 (`15 RUNE`/block) exhausts the reserve by **≈ Y6.7**, while a
chain that only manages a single transaction per block lasts to **≈ Y44**. PoNA
therefore **does not lengthen** the busy-network lifetime — it **protects the
reserve on quiet chains** (out to ~Y84 when completely idle), which is precisely
the intended behaviour: emission is throttled by real usage.

The `single` band is what stops a barely-used chain from being paid like a *busy*
one: one transaction earns `single_activity_reward` (1 RUNE in tier 3), not the
5-RUNE low rate, and not the 0.5-RUNE empty rate. Because the `zero` rate is now
0.5 RUNE instead of 1, the completely-idle worst case doubled from ~Y44 to ~Y84.

**Fee-only end state:** when the reserve reaches zero (or the applicable reward
falls at/below the fee-only floor), the static reward stops. Transaction fees
continue to be split across the active validator set by voting power via
`x/distribution`, forever. PoNA becomes inactive at that point — there is no
reserve-funded reward left to scale.

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
| `tiers[].max_tx_count_threshold` | `Tier` | Txs at/above which the max rate is paid (`10`); `0` disables PoNA |
| `tiers[].mid_activity_reward` | `Tier` | Rate when `mid <= tx < max`; **required** |
| `tiers[].mid_tx_count_threshold` | `Tier` | Lower bound of the mid band (`5`); **required**, `low < mid < max` |
| `tiers[].low_activity_reward` | `Tier` | Rate when `low <= tx < mid`; **required** |
| `tiers[].low_tx_count_threshold` | `Tier` | Lower bound of the low band (`2`); **required**, at least 2 |
| `tiers[].single_activity_reward` | `Tier` | Rate when `tx == 1`; **required** |
| `tiers[].zero_activity_reward` | `Tier` | Rate when `tx == 0`; also the **floor** for every band |

The tx count itself is **not** a parameter: it is captured per block from
`len(req.Txs)` in the app's `PreBlocker` and stored under
`pona/types.KeyBlockTxCount`.

> **Field numbering.** The band fields are numbered and declared in the order
> they are evaluated: `low` = 5, `single` = 6, `zero` = 7. Declaration order is
> also the order the generated Go struct — and therefore the JSON emitted for
> genesis — lists them, which is why the genesis file reads
> `... low_activity_reward, single_activity_reward, zero_activity_reward`.
> Renumbering a field would swap what an already-stored value means, so treat
> these numbers as fixed once a chain exists.

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
| `types/pona.go` | **All PoNA**: bands, rates, `PoNAEnabled`, `BandForTxCount`, `RewardForBand`, `ApplyFloor`, `RewardForTxCount`, `RewardForHeightAndTxCount`, `BandForHeightAndTxCount`, `validatePoNA`, `DefaultTxCountThreshold`, `DefaultMidTxCountThreshold`, `DefaultLowTxCountThreshold`, `MinLowTxCountThreshold` |
| `types/schedule.go` | Generic tier lookup (`RewardForHeight`, `TierForHeight`, `tierForHeight`) |
| `types/params.go` | `DefaultTiers`/`DefaultParams`/`Validate` (schedule structure) + RUNE helpers (`RuneAmount`, `RuneString`) |
| `types/pona_test.go` | PoNA band/floor/validation tests |
| `types/schedule_test.go` | Schedule/boundary tests |
| `keeper/rewards.go` | `DistributeRuneBlockReward` + `ComputeBlockReward` (shared by payout and CLI) |
| `client/cli/query.go` | `runed query pona {params,tier,block-reward,tx-count,pool}` |

## 8. Interaction with the fee market

The two-gas-price model ([`../tx-fees.md`](../tx-fees.md)) is
**independent** of PoNA:

| Flow | Source | Recipient | Shaped by PoNA? |
|---|---|---|---|
| Static block reward | 1B reserve (`x/pona`) | Proposer | **Yes** |
| Transaction fees | Fee pool (`x/distribution`) | All active validators by power | No |

PoNA must **not** key off gas used or fee revenue — only the **transaction count**
— so the two mechanisms stay decoupled.

## 9. Querying the module

`x/pona` still has **no gRPC Query service**, but that is no longer a limitation
for operators: `runed query pona` reads the module's KV store directly through
the `store/pona/key` ABCI path, so every question about the reward system is
answerable without a new protobuf or a chain upgrade.

| Command | Answers |
|---|---|
| `runed query pona params` | the whole emission schedule + PoNA rates per tier |
| `runed query pona tier` | which tier is active **now**, and how far away the next one is |
| `runed query pona block-reward` | what a block pays **now**, which band, and why |
| `runed query pona block-reward --tx-count 25` | what a block with 25 txs *would* pay |
| `runed query pona tx-count` | the regular-tx count PoNA used for the last block, **plus** the block's raw entry count |
| `runed query pona pool` | RUNE left in `validator_reward_pool`, and how long it lasts |

Typical output:

```console
$ runed query pona block-reward
band: zero
band_explanation: the block contains no regular transactions, so the zero-activity
  rate of 0.5 RUNE applies
denom: arune
height: 32495
reward: "500000000000000000"
reward_rune: "0.5"
tier:
  max: {arune: "50000000000000000000", rune: "50"}
  mid: {arune: "25000000000000000000", rune: "25"}
  low: {arune: "15000000000000000000", rune: "15"}
  single: {arune: "1000000000000000000", rune: "1"}
  single_enabled: true
  zero: {arune: "500000000000000000", rune: "0.5"}
  floor_applied: false
  pona_enabled: true
  low_tx_count_threshold: 2
  mid_tx_count_threshold: 5
  max_tx_count_threshold: 10
  tier_index: 0
  tier_count: 3
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
height: 1304
note: regular transactions only; the injected vote-extension pseudo-tx is excluded
reward: "500000000000000000"
reward_rune: "0.5"
tier_index: 0
tx_count: 0
tx_count_threshold: 10
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
2. **Threshold configurability** — `TxCountThreshold` is a per-tier on-chain
   field (mutable via params), default `10`. Governance-adjustable in practice;
   no dedicated gov Msg exists yet.
3. **Zero-band floor** — **resolved**: `min_reward_per_block` was removed so the
   `1 RUNE` zero-activity reward is expressible. Emission now ends only when no
   tier covers the height or the pool is empty. The zero rate was later lowered to
   **0.5 RUNE** and now also acts as the **floor** of every band (see §2.1).
4. **Standalone `x/pona` module** — deferred. The policy is implemented inside
   `x/pona`; splitting it later is a refactor, not a behaviour change
   (see [§7](#7-module-boundary)).
5. **Genesis / first block** — before any `PreBlocker` has run, the stored tx
   count is absent and `BlockTxCount` returns `0` (the zero-activity band).
6. **gRPC Query service** — not implemented, and not needed for operators
   (`runed query pona` reads the store directly, see [§9](#9-querying-the-module)).
   A real `Query` service is only worth adding when a proto-breaking change is
   being made anyway.



##

#### Credits: PoNA Mechanism is designed and built by Evorz Labs
