# PoNA Enhancement 3 — Original Design Record

> **This is the original design document, preserved verbatim as a historical
> record.** It is the concept as first proposed. It is **not** the implementation
> spec — [`dpona.md`](./dpona.md) is. Where they differ, `dpona.md` wins.
>
> Corrections applied to this record are marked **[NOTE]** inline. They do not
> alter the original intent; they annotate the places where the raw text was
> ambiguous or arithmetically impossible. See the *Corrections log* at the end for
> the complete list and the reasoning.

**Credits:** original design concept by **RZ | ROD ZISSO (Muroko)**.

This is an original design, conceived from scratch. It is not derived from,
copied from, or based on any other project or protocol.

---

## 1. Base tiers (dPona off)

### Tier 1

```
"start_height": "0",
"end_height": "10519200"
txs_count >= 20        - band = max    - rewards = 50
txs_count >= 15 < 20   - band = high   - rewards = 40
txs_count >= 10 < 15   - band = mid    - rewards = 30
txs_count >= 6 < 10    - band = low    - rewards = 20
txs_count = 5          - band = quint  - rewards = 5
txs_count = 4          - band = quart  - rewards = 4
txs_count = 3          - band = trio   - rewards = 3
txs_count = 2          - band = duo    - rewards = 2
txs_count = 1          - band = single - rewards = 1
txs_count = 0          - band = zero   - rewards = 0.5
```

### Tier 2

```
"start_height": "10519200",
"end_height": "21038400"
txs_count >= 20        - band = max    - rewards = 40
txs_count >= 15 < 20   - band = high   - rewards = 30
txs_count >= 10 < 15   - band = mid    - rewards = 20
txs_count >= 6 < 10    - band = low    - rewards = 15
txs_count = 5          - band = quint  - rewards = 5
txs_count = 4          - band = quart  - rewards = 4
txs_count = 3          - band = trio   - rewards = 3
txs_count = 2          - band = duo    - rewards = 2
txs_count = 1          - band = single - rewards = 1
txs_count = 0          - band = zero   - rewards = 0.5
```

### Tier 3

```
"start_height": "21038400",
"end_height": "0"
txs_count >= 20        - band = max    - rewards = 30
txs_count >= 15 < 20   - band = high   - rewards = 20
txs_count >= 10 < 15   - band = mid    - rewards = 15
txs_count >= 6 < 10    - band = low    - rewards = 10
txs_count = 5          - band = quint  - rewards = 5
txs_count = 4          - band = quart  - rewards = 4
txs_count = 3          - band = trio   - rewards = 3
txs_count = 2          - band = duo    - rewards = 2
txs_count = 1          - band = single - rewards = 1
txs_count = 0          - band = zero   - rewards = 0.5
```

### Tier 4 — fees-only block rewards

> **TODO (original):** We need a 4th Tier for the fees-only block rewards.
> Can the fees get accumulated back in `x/distribution` or a similar
> `x/fees_collector` — or am I wrong?

**[NOTE]** The answer, verified in code: fees are **not** accumulated. `x/distribution`'s
`AllocateTokens` sweeps `fee_collector` into the distribution module account at the
start of **every block** and splits it across validators by voting power, so no
standing balance exists to fund a tier from. Tier 4 therefore **re-allocates fees
validators already receive**, rather than adding new income. It is also **not a
height tier** — see the corrections log.

```
"end_height": "0"
txs_count >= 20        - band = max    - rewards = 1.0
txs_count >= 15 < 20   - band = high   - rewards = 0.7
txs_count >= 10 < 15   - band = mid    - rewards = 0.5
txs_count >= 6 < 10    - band = low    - rewards = 0.1
txs_count = 5          - band = quint  - rewards = 0.05
txs_count = 4          - band = quart  - rewards = 0.04
txs_count = 3          - band = trio   - rewards = 0.03
txs_count = 2          - band = duo    - rewards = 0.02
txs_count = 1          - band = single - rewards = 0.01
txs_count = 0          - band = zero   - rewards = 0.00
```

**[NOTE]** The original text wrote `0.005` for the zero band. Corrected to `0.00`,
because a block with zero transactions has zero fees to pay from — that is what
makes Tier 4 self-describing: *fees only, so no fees means no reward*.

---

## 2. Make PoNA Dynamic (dPona)

Dynamically increase and decrease PoNA difficulty based on band-max activity.

If within 5 blocks band `max` is elected 5 times in a row, dPona (dynamic PoNA)
activates and PoNA difficulty increases with an additional **+10 txs count** to
all bands.

If within 100 blocks band `max` has **not** been elected even once, dPona stays
active and PoNA difficulty **decreases** by **−10 txs count** from the previous
increased difficulty, applied to all bands, and does not go below the PoNA base
txs_count.

If within 5 blocks band `max` is again elected 5 times in a row on top of a
current increased difficulty, dPona stays active and difficulty increases again
by **+10 txs count** on top of the current difficulty, applied to all bands.

If within 5 blocks band `max` is elected only 1, 2, or 3 times — or mixed with
other elected bands — while on an increased difficulty, dPona stays active and
there is **no increase or decrease** of txs count. It stays stable until a new
condition is met on band-max activity.

The PoNA Diff stays at **1** on PoNA base txs_count for all bands when dPona is
**not** activated by band-max conditions.

**[NOTE — the off-by-one]:** the original text then gives two formulas that
disagree:

```
Pona Diff = 1 + N(additions diff)
Pona Diff = Total Additions Diff - 1
```

At `N = 1` these produce **2** and **0**. Resolved in `dpona.md` by making diff a
plain **count of +10 steps starting at 0**, where `diff = 0` *means* dPona is off:

| dPona | `diff` | txs offset |
|---|---|---|
| Off | **0** | 0 |
| On | 1 | +10 |
| On | 2 | +20 |

This keeps the base state (`diff = 0`) and the disabled state identical, so no
separate boolean can drift out of sync with the number.

Also note that PoNA difficulty is computed and surfaced on the blockdium panel
and in the terminal log. Difficulty stays at base when dPona is off.

### The blockdium PoNA panel (as specified)

dPona: on / off — displayed as On or Off based on band-max condition activity.

Pona Diff: 1 — stays at 1 on PoNA base txs_count, and increases by +1 / decreases
by −1 per each +10 / −10 txs_count change, based on dPona when enabled by
band-max condition activity.

---

## 3. dPona active — all tiers and bands

When band `max` is actively elected 5 times in a row within 5 blocks mined, every
band gains **+10 txs_count**; the same applies in reverse (−10) when band `max`
is not elected within 100 blocks.

### Tier 1

```
"start_height": "0",
"end_height": "10519200"
txs_count >= 20+10      - band = max    - rewards = 50
txs_count >= 15+10 < 20+10 - band = high - rewards = 40
txs_count >= 10+10 < 15+10 - band = mid  - rewards = 30
txs_count >= 6+10 < 10+10  - band = low  - rewards = 20
txs_count = 5+10        - band = quint  - rewards = 5
txs_count = 4+10        - band = quart  - rewards = 4
txs_count = 3+10        - band = trio   - rewards = 3
txs_count = 2+10        - band = duo    - rewards = 2
txs_count = 1+10        - band = single - rewards = 1
txs_count = 0           - band = zero   - rewards = 0.5
```

Blockdium PoNA panel displays:

```
dPona: ON
Pona diff: 2
```

**[NOTE — `<=` was wrong]:** the original used `txs_count <= 5+10` …
`txs_count <= 1+10` for the exact-count bands. Evaluated in order, `<= 15`
swallows 0–15, making quart/trio/duo/single unreachable and overpaying quiet
blocks. Switching to `==` opens a *gap* (1–10 unbanded). Resolved in `dpona.md`
§2 by **deriving** the exact bands from `low_tx_count_threshold` with a clamped
floor. The original `=`/`<=` spelling is preserved above only as the record of
intent; the implementation uses the derived form.

### Tier 2

```
"start_height": "10519200",
"end_height": "21038400"
txs_count >= 20+10      - band = max    - rewards = 40
txs_count >= 15+10 < 20+10 - band = high - rewards = 30
txs_count >= 10+10 < 15+10 - band = mid  - rewards = 20
txs_count >= 6+10 < 10+10  - band = low  - rewards = 15
txs_count = 5+10        - band = quint  - rewards = 5
txs_count = 4+10        - band = quart  - rewards = 4
txs_count = 3+10        - band = trio   - rewards = 3
txs_count = 2+10        - band = duo    - rewards = 2
txs_count = 1+10        - band = single - rewards = 1
txs_count = 0           - band = zero   - rewards = 0.5
```

Blockdium PoNA panel displays:

```
dPona: ON
Pona diff: 2
```

### Tier 3

```
"start_height": "21038400",
"end_height": "0"
txs_count >= 20+10      - band = max    - rewards = 30
txs_count >= 15+10 < 20+10 - band = high - rewards = 20
txs_count >= 10+10 < 15+10 - band = mid  - rewards = 15
txs_count >= 6+10 < 10+10  - band = low  - rewards = 10
txs_count = 5+10        - band = quint  - rewards = 5
txs_count = 4+10        - band = quart  - rewards = 4
txs_count = 3+10        - band = trio   - rewards = 3
txs_count = 2+10        - band = duo    - rewards = 2
txs_count = 1+10        - band = single - rewards = 1
txs_count = 0           - band = zero   - rewards = 0.5
```

Blockdium PoNA panel displays:

```
dPona: ON
Pona diff: 2
```

### Tier 4 — fees only

> **TODO (original, repeated):** We need a 4th Tier for the fees-only block
> rewards. Can the fees get accumulated back in `x/distribution` or a similar
> `x/fees_collector` — or am I wrong?

```
"end_height": "0"
txs_count >= 20+10      - band = max    - rewards = 1.0
txs_count >= 15+10 < 20+10 - band = high - rewards = 0.7
txs_count >= 10+10 < 15+10 - band = mid  - rewards = 0.5
txs_count >= 6+10 < 10+10  - band = low  - rewards = 0.1
txs_count = 5+10        - band = quint  - rewards = 0.05
txs_count = 4+10        - band = quart  - rewards = 0.04
txs_count = 3+10        - band = trio   - rewards = 0.03
txs_count = 2+10        - band = duo    - rewards = 0.02
txs_count = 1+10        - band = single - rewards = 0.01
txs_count = 0           - band = zero   - rewards = 0.00
```

Blockdium PoNA panel displays:

```
dPona: ON
Pona diff: 2
```

---

## 4. Corrections log

Every deviation between this record and the implemented design, with the reason.
The original intent is unchanged in each case.

| # | Original | Corrected | Why |
|---|---|---|---|
| 1 | Exact bands spelled `txs_count = 5+10`, `<= 4+10`, `<= 3+10`, `<= 2+10`, `<= 1+10` | Derived from `low_tx_count_threshold` with a clamped `single` floor | `<=` made quint swallow 0–15 (quart/trio/duo/single unreachable, quiet blocks overpaid 2.5×); `==` left 1–10 unbanded |
| 2 | `Pona Diff = 1 + N` **or** `= Total Additions Diff − 1` | `diff` = count of +10 steps, **starting at 0**; `diff = 0` means off | The two formulas disagreed at `N = 1` (produced 2 vs 0). Base 0 also removes the separate on/off boolean |
| 3 | Tier 4 as a height tier with `end_height: 0` | Tier 4 is the `pool < reward` **fallback branch**, not a height tier | Two open-ended tiers fail `Params.Validate` (contiguity + exactly one open tier). A fixed start height stranded up to **52,384,134 RUNE** (98% of Tier 3's budget) on a quiet chain, because Tier 3 spend is activity-scaled |
| 4 | Tier 4 zero band pays `0.005` | Pays **`0`** | 0 transactions ⇒ 0 fees. There is nothing to pay from |
| 5 | Fees "accumulate back in x/distribution" (open question) | Confirmed **they do not** — `AllocateTokens` sweeps `fee_collector` **every block** | No standing balance exists; Tier 4 must re-allocate fees, not add income |
| 6 | No difficulty cap specified | **No cap**, but warn at `diff >= 5` | The controller is negative feedback with 20:1 decay asymmetry; unreachable max *causes* decay. A cap would be dead configuration |
| 7 | dPona scope unspecified | **Global**, all three reserve tiers | `band max` is conditioned chain-wide |
| 8 | Fee-funded reward amount not constrained | `paid = min(scheduled, fee_skim_percent × block_fees)`, skim = 75% | A fixed schedule is insolvent: at 20 light txs there is only 1.083 RUNE of fees, so 100% skim is needed for the `max` band to reach its 1.0 |

---

## 5. See also

| Doc | Contents |
|---|---|
| [`dpona.md`](./dpona.md) | The implementation specification — ten-band ladder, dPona controller, Tier 4 fee fallback |
| [`pona.md`](./pona.md) | PoNA as it exists on chain today (five bands) |
