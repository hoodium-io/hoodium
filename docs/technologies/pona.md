# Proof of Network Activity (PoNA)

> **Status:** Designed, **not yet implemented**. This document is the authoritative
> description of the mechanism and its parameters. It supersedes the earlier
> design note that lived in `docs/features/README.md`.
>
> **Module:** `x/pona` (see [§7 Module boundary](#7-module-boundary)).

## 1. Why PoNA exists

Hoodium pays a **static RUNE block reward** from a pre-funded 1B RUNE reserve
(see [`runerewards.md`](./runerewards.md)). A naive static reward pays the same
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

The distinction between **Low** and **Zero** is deliberate: a block with a small
number of real transactions is *some* activity and earns a meaningful reduced
reward; a **completely empty** block earns only a nominal amount, so an idle
chain drains the reserve at the slowest possible rate while still paying the
proposer something for keeping the chain alive.

## 3. Per-tier rates

PoNA rates are defined **per emission tier**. The full rate is the base tier
rate owned by `x/runerewards`; the Low and Zero rates are PoNA policy.

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
├─ PreBlock        ── x/pona captures len(req.Txs) ──► store: "current block tx count"
│                     (txs are visible here; EndBlock cannot see them)
│
├─ DeliverTx …     ── transactions execute as normal (PoNA does not interfere)
│
└─ EndBlock
   ├─ x/pona       ── reads the stored tx count, classifies the band,
   │                  computes the *effective* reward for this block
   └─ x/runerewards ── pays the effective reward from validator_reward_pool
                        → to the block proposer's operator address
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

For the current (pre-pseudo-tx) implementation the count is the raw block length.
If Hoodium re-introduces an **app-level injected pseudo-transaction** during
`PrepareProposal`, PoNA must **exclude it** and count **only regular chain
transactions** — otherwise every block would look like it had ≥1 tx and the Zero
band could never be reached. The existing `app/abci/preblock.go` already
distinguishes an injected first tx from regular txs and provides the pattern to
reuse.

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

## 7. Module boundary

PoNA is split from the base reward machinery along a **money / policy** line:

| Module | Owns | Does not own |
|---|---|---|
| **`x/runerewards`** | The 1B `validator_reward_pool` (genesis funding, balance, top-up), the **base emission schedule** (`Tier` heights + `reward_per_block` full rates), and the coin-transfer plumbing to the proposer | The activity banding or how much of the base rate is actually paid |
| **`x/pona`** | The **activity policy**: capturing the block tx count, classifying the band, and computing the **effective** reward from the tier's full/low/zero rates | The reserve, the payout transfer, or the tier boundaries |

In short: **`runerewards` is the wallet; `pona` is the rule that decides how much
comes out of it each block.**

### 7.1 Parameter placement

| Parameter | Module | Meaning |
|---|---|---|
| `tiers[].start_height` | `runerewards` | Tier boundary (block height) |
| `tiers[].end_height` | `runerewards` | Tier boundary (`0` = open-ended) |
| `tiers[].reward_per_block` | `runerewards` | **Full** rate for that tier |
| `tiers[].tx_count_threshold` | `pona` | Txs at/above which the full rate is paid (default `10`) |
| `tiers[].low_activity_reward` | `pona` | Rate when `0 < tx < threshold` |
| `tiers[].zero_activity_reward` | `pona` | Rate when `tx == 0` |

> **Implementation note.** Because the three activity rates are properties of the
> same tier, the fields may live on a single shared `Tier` proto message. If
> `x/pona` is a separate module, `pona` holds its own `Params` carrying the
> low/zero rates keyed by tier boundary, and `runerewards` keeps only the full
> rate. The economic table in [§3](#3-per-tier-rates) is the contract between them.

## 8. Interaction with the fee market

The two-gas-price model ([`../features/tx-fees.md`](../features/tx-fees.md)) is
**independent** of PoNA:

| Flow | Source | Recipient | Shaped by PoNA? |
|---|---|---|---|
| Static block reward | 1B reserve (`x/runerewards`) | Proposer | **Yes** |
| Transaction fees | Fee pool (`x/distribution`) | All active validators by power | No |

PoNA must **not** key off gas used or fee revenue — only the **transaction count**
— so the two mechanisms stay decoupled.

## 9. Open questions / follow-ups

1. **Pseudo-tx exclusion** — confirm whether Hoodium's `PrepareProposal` injects a
   pseudo-tx; if so, PoNA counts only the regular txs (see [§4.1](#41-which-transactions-are-counted)).
2. **Threshold configurability** — should `TxCountThreshold = 10` be a governance
   parameter, or a constant? (Currently designed as a parameter.)
3. **Zero-band floor** — the `1 RUNE` zero-band reward sits below the legacy
   5-RUNE fee-only floor; the floor must be removed or redefined so `1` is
   expressible (see [`runerewards.md`](./runerewards.md)).
4. **Genesis** — at the first block there is no meaningful tx count; the fallback
   in [§4](#4-how-it-works--block-lifecycle) applies.
