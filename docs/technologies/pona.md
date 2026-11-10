# PoNA — Proof of Network Activity (RUNE rewards)

> **Status:** Implemented as **`x/pona`**. This is the authoritative document for
> both the **mechanism** and the **module**.
>
> **Code:** `x/pona/types/pona.go` (bands, rates, validation) ·
> `x/pona/keeper/rewards.go` (payout) · `app/app.go` (tx-count capture).
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
  low, or zero rate.
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

Each block is classified into exactly one of **three bands** by its transaction
count:

| Band | Condition | Reward paid |
|---|---|---|
| **Full** | `txCount >= TxCountThreshold` | `reward_per_block` (the tier's full base rate) |
| **Low** | `0 < txCount < TxCountThreshold` | `low_activity_reward` |
| **Zero** | `txCount == 0` | `zero_activity_reward` |

- **`TxCountThreshold` = 10** (the configured "active block" threshold).
- `txCount` is the **number of transactions actually included in the block being
  finalised** — *not* mempool size, and *not* a rolling average. This was
  confirmed as the intended semantics.

> **Note on "threshold 0":** a tier whose `TxCountThreshold` is **0** has PoNA
> *switched off* and always pays the full rate. Zero is a sentinel meaning
> "disabled" — it is **not** a claim that the threshold is 0 transactions. The
> live threshold is **10** (`DefaultTxCountThreshold`).

The distinction between **Low** and **Zero** is deliberate: a block with a small
number of real transactions is *some* activity and earns a meaningful reduced
reward; a **completely empty** block earns only a nominal amount, so an idle
chain drains the reserve at the slowest possible rate while still paying the
proposer something for keeping the chain alive.

## 3. Per-tier rates

PoNA rates are defined **per emission tier**. The full rate is the base tier
rate owned by `x/pona`; the Low and Zero rates are PoNA policy.

| Tier | Epoch | Full (`tx >= 10`) | Low (`0 < tx < 10`) | Zero (`tx == 0`) |
|---|---|---|---|---|
| 1 | Y0–2 | **50 RUNE** | **20 RUNE** | **1 RUNE** |
| 2 | Y2–4 | **25 RUNE** | **10 RUNE** | **1 RUNE** |
| 3 | Y4 → infinite | **10 RUNE** | **5 RUNE** | **1 RUNE** |

All values are in **RUNE** (stored on-chain as **arune**, `1 RUNE = 10^18 arune`).

Notes:

- Tier 3 runs **from year 4 forever** (open-ended). It is the terminal tier of the
  reserve-funded schedule; PoNA only shapes it for as long as the reserve lasts.
- The Low rate is **not** a fixed fraction of the full rate across tiers
  (50→20 is 40%, 25→10 is 40%, but 10→5 is 50%). The rates are therefore stored
  as **explicit per-tier fields**, not derived by a multiplier. This is
  intentional — a derived model could not express `1 RUNE` for the Zero band,
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
4. **Fails safe.** If the tx count is unavailable (e.g. genesis, or a store miss),
   the mechanism must **not** silently over- or under-pay; the reward falls back to
   the base schedule for that height.

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

Assume `TxCountThreshold = 10`, 6-second blocks (5,259,600 blocks/year), and the
three tiers of [§3](#3-per-tier-rates).

| Block | Height epoch | `txCount` | Band | Reward |
|---|---|---|---|---|
| A | Y1 | 42 | Full | **50 RUNE** |
| B | Y1 | 7 | Low | **20 RUNE** |
| C | Y1 | 0 | Zero | **1 RUNE** |
| D | Y3 | 15 | Full | **25 RUNE** |
| E | Y3 | 3 | Low | **10 RUNE** |
| F | Y5 | 128 | Full | **10 RUNE** |
| G | Y5 | 6 | Low | **5 RUNE** |
| H | Y5 | 0 | Zero | **1 RUNE** |

## 6. Economic effect on the 1B reserve

The reserve is a fixed **1,000,000,000 RUNE**. Tier 1 and Tier 2 are *not*
PoNA-dependent in the worst case — they always consume their full budget:

| Tier | Blocks | Rate | RUNE spent |
|---|---|---|---|
| Tier 1 (Y0–2) | 10,519,200 | 50 | 525,960,000 |
| Tier 2 (Y2–4) | 10,519,200 | 25 | 262,980,000 |
| **Remaining for Tier 3** | | | **211,060,000** |

Tier 3 (10 full / 5 low / 1 zero) then drains the remainder at a rate set by how
busy the chain is:

| Chain activity | Tier-3 rate | Tier-3 duration | Fee-only reached at |
|---|---|---|---|
| **Busy** — every block `tx >= 10` | 10 | 211.06M ÷ 52.596M ≈ **4.01 y** | **≈ Y8.0** |
| **Typical** — most blocks `0 < tx < 10` | 5 | 211.06M ÷ 26.298M ≈ **8.03 y** | **≈ Y12.0** |
| **Quiet** — every block `tx == 0` | 1 | 211.06M ÷ 5.2596M ≈ **40.1 y** | **≈ Y44.1** |

**Read this table carefully.** The often-quoted *"~12 years"* figure is the
**Typical** case (Tier 3 at the reduced rate). If the chain is genuinely busy and
Tier 3 pays its **full 10 RUNE**, the reserve is exhausted at **≈ Y8**, not Y12.
PoNA therefore **does not lengthen** the busy-network lifetime — it **protects
the reserve on quiet chains** (up to ~Y44), which is precisely the intended
behaviour: emission is throttled by real usage.

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
| `tiers[].start_height` | `Tier` | Tier boundary (block height) |
| `tiers[].end_height` | `Tier` | Tier boundary (`0` = open-ended) |
| `tiers[].reward_per_block` | `Tier` | **Full** rate for that tier |
| `tiers[].tx_count_threshold` | `Tier` | Txs at/above which the full rate is paid (`10`); `0` disables PoNA |
| `tiers[].low_activity_reward` | `Tier` | Rate when `0 < tx < threshold` |
| `tiers[].zero_activity_reward` | `Tier` | Rate when `tx == 0` |

The tx count itself is **not** a parameter: it is captured per block from
`len(req.Txs)` in the app's `PreBlocker` and stored under
`pona/types.KeyBlockTxCount`.

### 7.2 Code map

| Piece | Location |
|---|---|
| PoNA bands, rates, validation | `x/pona/types/pona.go` |
| Band selection | `Tier.RewardForTxCount` (`pona.go`) |
| Height + tx count → reward | `Params.RewardForHeightAndTxCount` (`pona.go`) |
| Tx-count capture | `app.Hoodium.PreBlocker` → `keeper.SetBlockTxCount` |
| Reward payout | `Keeper.DistributeRuneBlockReward` (`keeper/rewards.go`) |
| Consensus version | `pona/types.ConsensusVersion` = **2** |

### 7.3 File layout

PoNA lives in its own file, **`x/pona/types/pona.go`**, separate from the
generic schedule (`schedule.go`) and the default parameters (`params.go`). A
dedicated `x/pona` **module** is not used, but the code is isolated so the
mechanism has a single reference point:

| File | Responsibility |
|---|---|
| `types/pona.go` | **All PoNA**: bands, rates, `PoNAEnabled`, `RewardForTxCount`, `RewardForHeightAndTxCount`, `validatePoNA`, `DefaultTxCountThreshold` |
| `types/schedule.go` | Generic tier lookup (`RewardForHeight`, `tierForHeight`) |
| `types/params.go` | `DefaultTiers`/`DefaultParams`/`Validate` (schedule structure) |
| `types/pona_test.go` | PoNA band/validation tests |
| `types/schedule_test.go` | Schedule/boundary tests |

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
| `runed query pona tx-count` | the regular-tx count PoNA used for the last block |
| `runed query pona pool` | RUNE left in `validator_reward_pool`, and how long it lasts |

Typical output:

```console
$ runed query pona block-reward
band: zero
band_explanation: the block contains no regular transactions, so the zero-activity
  rate of 1 RUNE applies
denom: arune
height: 32495
reward: "1000000000000000000"
reward_rune: "1"
tier:
  full_reward: {arune: "50000000000000000000", rune: "50"}
  low_reward:  {arune: "20000000000000000000", rune: "20"}
  zero_reward: {arune: "1000000000000000000",  rune: "1"}
  pona_enabled: true
  tier_index: 0
  tier_count: 3
  tx_count_threshold: 10
tx_count: 0
tx_count_source: chain (recorded for the last finalized block)
```

Read it as: **the chain is idle, and it is paying 1 RUNE, not 20.** That is the
behaviour the zero band exists for.

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
   tier covers the height or the pool is empty.
4. **Standalone `x/pona` module** — deferred. The policy is implemented inside
   `x/pona`; splitting it later is a refactor, not a behaviour change
   (see [§7](#7-module-boundary)).
5. **Genesis / first block** — before any `PreBlocker` has run, the stored tx
   count is absent and `BlockTxCount` returns `0` (the zero-activity band).
6. **gRPC Query service** — not implemented, and not needed for operators
   (`runed query pona` reads the store directly, see [§9](#9-querying-the-module)).
   A real `Query` service is only worth adding when a proto-breaking change is
   being made anyway.
