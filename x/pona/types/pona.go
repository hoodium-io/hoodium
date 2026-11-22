package types

import (
	"fmt"

	sdkmath "cosmossdk.io/math"
)

// Proof of Network Activity (PoNA).
//
// PoNA scales the block reward by how many transactions the block actually
// contains. Each emission tier carries a graded ladder of rates, from a full
// (maximum) reward down to a nominal floor:
//
//	txCount >= MaxTxCountThreshold                    -> MaxActivityReward  (max)
//	MidTxCountThreshold  <= txCount < Max             -> MidActivityReward  (mid)
//	LowTxCountThreshold  <= txCount < Mid             -> LowActivityReward  (low)
//	txCount == 1                                      -> SingleActivityReward (single)
//	txCount == 0                                      -> ZeroActivityReward (zero)
//
// ZeroActivityReward is also the FLOOR: no band ever pays less than it, so a
// block is always worth at least that much.
//
// Every tier carries a complete ladder — all five rates and all three thresholds
// are required, and Validate rejects a tier that omits any of them. There is no
// "disabled" mode.
//
// PoNA never blocks a transaction, never changes block validity, and never
// changes who may propose: it only decides how much of the scheduled reward is
// paid for a given block. The transaction count is captured from the ABCI
// request in the application's PreBlock hook (see app.Hoodium.PreBlocker),
// stored via keeper.SetBlockTxCount, and read back in EndBlock.
//
// See docs/technologies/pona/pona.md for the full mechanism and economics.

// DefaultMaxTxCountThreshold is the default minimum number of transactions a
// block must contain for the MAXIMUM (base) reward to be paid under PoNA.
//
// The four stored thresholds of the shipped base ladder are 6 / 10 / 15 / 20, so
// with dPona off a block carrying:
//
//	tx_count >= 20   -> max    (MaxActivityReward)
//	15 <= tx <  20   -> high   (HighActivityReward)
//	10 <= tx <  15   -> mid    (MidActivityReward)
//	  6 <= tx <  10   -> low    (LowActivityReward)
//	tx_count == 5    -> quint  (QuintActivityReward)
//	tx_count == 4    -> quart  (QuartActivityReward)
//	tx_count == 3    -> trio   (TrioActivityReward)
//	tx_count == 2    -> duo    (DuoActivityReward)
//	tx_count == 1    -> single (SingleActivityReward)
//	tx_count == 0    -> zero   (ZeroActivityReward)
//
// The quint..single bands are derived from the low threshold, so they shift with
// it when dPona raises the difficulty.
const DefaultMaxTxCountThreshold = uint64(20)

// DefaultHighTxCountThreshold is the default lower bound of the HIGH band.
const DefaultHighTxCountThreshold = uint64(15)

// DefaultMidTxCountThreshold is the default lower bound of the MID band.
const DefaultMidTxCountThreshold = uint64(10)

// DefaultLowTxCountThreshold is the default lower bound of the LOW band, and the
// anchor the five exact-count bands are derived from (quint = low-1 ... single =
// low-5). It must stay >= MinLowTxCountThreshold.
const DefaultLowTxCountThreshold = uint64(6)

// MinLowTxCountThreshold is the smallest allowed low tx-count threshold.
//
// It is 6 because the five exact-count bands are derived from it as low-1 .. low-5
// (quint, quart, trio, duo, single). With low = 6 those land on 5, 4, 3, 2, 1 --
// all distinct and all above the zero band. A smaller low would push `single`
// onto 0 and collide with the zero band.
const MinLowTxCountThreshold = uint64(6)

// ActivityBand identifies which Proof of Network Activity band a block falls
// into. The band, not the raw transaction count, decides which of the tier's ten
// rates is paid, so the decision lives in exactly one place
// (Tier.BandForTxCount) and is shared by the payout path, the emitted event, the
// CLI and the block explorer.
//
// The constants are declared from MAX down to ZERO, matching the proto field
// declaration order and Tier.Ladder(), so the code, the genesis file and any UI
// all read the ladder the same way: busiest band first, empty block last.
type ActivityBand string

const (
	// BandMax is the band of a busy block: txCount >= MaxTxCountThreshold.
	BandMax ActivityBand = "max"
	// BandHigh is the band of a very active block:
	// HighTxCountThreshold <= txCount < MaxTxCountThreshold.
	BandHigh ActivityBand = "high"
	// BandMid is the band of a block with substantial activity:
	// MidTxCountThreshold <= txCount < HighTxCountThreshold.
	BandMid ActivityBand = "mid"
	// BandLow is the band of a block with moderate activity:
	// LowTxCountThreshold <= txCount < MidTxCountThreshold.
	BandLow ActivityBand = "low"
	// BandQuint is the band of a block with exactly LowTxCountThreshold-1
	// transactions (5 by default).
	BandQuint ActivityBand = "quint"
	// BandQuart is the band of a block with exactly LowTxCountThreshold-2
	// transactions (4 by default).
	BandQuart ActivityBand = "quart"
	// BandTrio is the band of a block with exactly LowTxCountThreshold-3
	// transactions (3 by default).
	BandTrio ActivityBand = "trio"
	// BandDuo is the band of a block with exactly LowTxCountThreshold-4
	// transactions (2 by default).
	BandDuo ActivityBand = "duo"
	// BandSingle is the clamped-floor band: exactly LowTxCountThreshold-5
	// transactions (1 by default) and, once the dPona offset is active, every
	// count below the duo band. It is its own band because one transaction is
	// more activity than none, but a single transaction on its own is not a used
	// chain.
	BandSingle ActivityBand = "single"
	// BandZero is the band of a block that contains no regular transactions.
	// It pays the tier's nominal ZeroActivityReward so an idle chain still pays
	// the proposer something while draining the reserve as slowly as possible.
	BandZero ActivityBand = "zero"
	// BandNone means no tier covers the height and the reserve cannot pay: no
	// activity-scaled reward applies from the emission schedule.
	BandNone ActivityBand = "none"
)

// BandForTxCount returns the PoNA band that applies to a block containing
// txCount regular transactions.
//
// The ten bands are selected by four stored thresholds (max/high/mid/low); the
// five exact-count bands (quint..single) are DERIVED from low, so the ladder has
// no gaps and no overlaps at any difficulty.
//
// The cases are evaluated MAX -> ZERO, matching the order the ladder is declared
// in the proto, the genesis file and Tier.Ladder(). Reading the switch is
// therefore the same as reading the ladder top-down:
//
//	txCount >= MaxThreshold    -> Max
//	txCount >= HighThreshold   -> High
//	txCount >= MidThreshold    -> Mid
//	txCount >= LowThreshold    -> Low
//	txCount == LowThreshold-1  -> Quint
//	txCount == LowThreshold-2  -> Quart
//	txCount == LowThreshold-3  -> Trio
//	txCount == LowThreshold-4  -> Duo
//	txCount == 0               -> Zero
//	otherwise                  -> Single  (clamped floor: 1 .. LowThreshold-5)
//
// # Why the exact bands are derived and not stored
//
// Storing five more thresholds would allow an inconsistent ladder (a quint
// threshold above the quart one, or a hole where a count matches nothing). The
// naive alternative — `txCount <= quint` for each exact band — is worse: the
// first `<=` swallows every lower count, making the rest unreachable. Deriving
// them from a single anchor (low) makes those states unrepresentable.
func (t Tier) BandForTxCount(txCount uint64) ActivityBand {
	low := t.LowTxCountThreshold

	switch {
	// Range bands, descending from the top.
	case txCount >= t.MaxTxCountThreshold:
		return BandMax
	case txCount >= t.HighTxCountThreshold:
		return BandHigh
	case txCount >= t.MidTxCountThreshold:
		return BandMid
	case txCount >= low:
		return BandLow

	// The five exact-count bands, still descending. `low` is validated to be >= 6,
	// so quint..duo are all >= 2 and never collide with the zero band.
	case txCount == low-1:
		return BandQuint
	case txCount == low-2:
		return BandQuart
	case txCount == low-3:
		return BandTrio
	case txCount == low-4:
		return BandDuo

	// The empty block is its own band and is NOT the floor of this ladder: the
	// floor is the single band, which absorbs every count below duo.
	case txCount == 0:
		return BandZero

	// Remaining counts (1 .. low-5) clamp into single, so no count is ever
	// unbanded at any difficulty.
	default:
		return BandSingle
	}
}

// QuintTxCount, QuartTxCount, TrioTxCount, DuoThreshold and SingleTxCount return
// the exact transaction counts of the five derived bands.
//
// They are exposed so the CLI, the event and the block explorer can display the
// real ladder for the current difficulty instead of recomputing low-1 .. low-5
// themselves (and getting it subtly wrong).
func (t Tier) QuintTxCount() uint64 { return t.LowTxCountThreshold - 1 }
func (t Tier) QuartTxCount() uint64 { return t.LowTxCountThreshold - 2 }
func (t Tier) TrioTxCount() uint64  { return t.LowTxCountThreshold - 3 }
func (t Tier) DuoTxCount() uint64   { return t.LowTxCountThreshold - 4 }

// SingleTxCount is the highest count that pays the single-band rate. Below it,
// any count still pays single (the clamped floor), so there is never an unbanded
// transaction count.
func (t Tier) SingleTxCount() uint64 { return t.LowTxCountThreshold - 5 }

// BandLadderEntry describes one rung of the rendered ladder. It exists so every
// consumer (CLI, logs, blockdium) shows the same band/range/rate triple.
type BandLadderEntry struct {
	Band ActivityBand
	// Min and Max are the inclusive transaction-count range for this band.
	// Max is math.MaxUint64 for the top band.
	Min, Max uint64
	Reward   sdkmath.Int
}

// Ladder returns the tier's ten bands from MAX down to ZERO, with their resolved
// transaction-count ranges and configured rates.
//
// The order is deliberate and matches the proto field declaration order, so the
// genesis file, `runed query pona params` and any UI all read the ladder the same
// way: busiest band first, empty block last.
//
// The ranges are derived from the same thresholds BandForTxCount uses, so the
// rendered ladder cannot disagree with what is actually paid.
func (t Tier) Ladder() []BandLadderEntry {
	const top = ^uint64(0)
	low := t.LowTxCountThreshold

	return []BandLadderEntry{
		{BandMax, t.MaxTxCountThreshold, top, t.MaxActivityReward},
		{BandHigh, t.HighTxCountThreshold, t.MaxTxCountThreshold - 1, t.HighActivityReward},
		{BandMid, t.MidTxCountThreshold, t.HighTxCountThreshold - 1, t.MidActivityReward},
		{BandLow, low, t.MidTxCountThreshold - 1, t.LowActivityReward},
		{BandQuint, low - 1, low - 1, t.QuintActivityReward},
		{BandQuart, low - 2, low - 2, t.QuartActivityReward},
		{BandTrio, low - 3, low - 3, t.TrioActivityReward},
		{BandDuo, low - 4, low - 4, t.DuoActivityReward},
		{BandSingle, 1, low - 5, t.SingleActivityReward},
		{BandZero, 0, 0, t.ZeroActivityReward},
	}
}

// WithDPonaOffset returns a copy of the tier with every stored threshold raised
// by offset transactions.
//
// Only the four stored thresholds move (low / mid / high / max); the five
// exact-count bands are derived from `low` at classification time, so they follow
// automatically. That is the point of deriving them: one shift, one consistent
// ladder.
//
// The result is always a valid tier for classification purposes:
//
//   - every threshold is raised by the same offset, so ordering is preserved
//     (low < mid < high < max stays low+off < mid+off < ...);
//   - `low` stays >= MinLowTxCountThreshold, so the derived bands never collide
//     with the zero band;
//   - reward amounts are untouched -- difficulty changes *when* a band is earned,
//     never *how much* it pays.
//
// A zero offset returns the tier unchanged, which is the dPona-off case.
func (t Tier) WithDPonaOffset(offset uint64) Tier {
	if offset == 0 {
		return t
	}

	t.LowTxCountThreshold += offset
	t.MidTxCountThreshold += offset
	t.HighTxCountThreshold += offset
	t.MaxTxCountThreshold += offset

	return t
}

// IsReserveTier reports whether the tier is a height-scheduled emission tier.
//
// The fee-only ladder (Params.FeeOnlyTier, the "Tier 4" of the design) is not a
// height tier: it carries no start/end height and is selected by whether the
// reserve can still pay, not by the block height. It is therefore never part of
// Params.Tiers and must not be treated as one.
func (t Tier) IsReserveTier() bool {
	return t.StartHeight != 0 || t.EndHeight != 0
}

// `txCount` transactions, applying its PoNA bands.
//
// The result is never below the tier's ZeroActivityReward: that value is the
// floor, so an active block always earns at least as much as an empty one.
func (t Tier) RewardForTxCount(txCount uint64) sdkmath.Int {
	reward := t.RewardForBand(t.BandForTxCount(txCount))

	return t.ApplyFloor(reward)
}

// RewardForBand returns the tier's configured rate for a band, before the
// ZeroActivityReward floor is applied.
//
// Every tier carries all ten rates, and Params are validated on the way in (see
// validatePoNA), so each band maps to exactly one field with no fallbacks.
//
// Callers that display the configured rates (the CLI, the docs) want the raw
// value; callers that decide what is actually paid must use RewardForTxCount.
func (t Tier) RewardForBand(band ActivityBand) sdkmath.Int {
	switch band {
	case BandMax:
		return t.MaxActivityReward
	case BandHigh:
		return t.HighActivityReward
	case BandMid:
		return t.MidActivityReward
	case BandLow:
		return t.LowActivityReward
	case BandQuint:
		return t.QuintActivityReward
	case BandQuart:
		return t.QuartActivityReward
	case BandTrio:
		return t.TrioActivityReward
	case BandDuo:
		return t.DuoActivityReward
	case BandSingle:
		return t.SingleActivityReward
	case BandZero:
		return t.ZeroActivityReward
	default:
		return sdkmath.ZeroInt()
	}
}

// ApplyFloor raises reward to the tier's ZeroActivityReward when it is lower.
//
// ZeroActivityReward is the minimum value of a block, which is what makes the
// name accurate: a block is never worth less than the zero-activity rate. This
// also keeps the schedule self-consistent if a tier is ever configured with a
// low rate below its zero rate.
func (t Tier) ApplyFloor(reward sdkmath.Int) sdkmath.Int {
	if reward.LT(t.ZeroActivityReward) {
		return t.ZeroActivityReward
	}

	return reward
}

// RewardForHeightAndTxCount returns the static block reward (in arune) that
// applies at the given block height for a block containing `txCount`
// transactions, at the base (dPona-off) difficulty.
//
// This is the Proof of Network Activity (PoNA) reward. The tier covering the
// height provides ten rates, selected by the block's transaction count (see
// RewardForTxCount).
//
// It returns zero when no tier covers the height (emission has ended).
//
// This function is pure and deterministic: the transaction count is a property
// of the block, identical on every node, so every node computes the same reward.
//
// NOTE: this is the dPona-off view. The consensus path must call
// RewardForHeightTxCountAndDiff, which applies the stored difficulty offset.
func (p Params) RewardForHeightAndTxCount(height, txCount uint64) sdkmath.Int {
	return p.RewardForHeightTxCountAndDiff(height, txCount, 0)
}

// RewardForHeightTxCountAndDiff returns the static block reward at the given
// height for a block with `txCount` transactions, with the dPona difficulty
// applied.
//
// `diff` is the controller's difficulty in +DPonaTxCountStep steps (0 = off).
// The tier's thresholds are shifted before classification, so a busier chain has
// to be busier still to earn each band. Reward AMOUNTS never change with
// difficulty -- only the threshold needed to reach them.
//
// Returns zero when no tier covers the height (emission has ended).
func (p Params) RewardForHeightTxCountAndDiff(height, txCount, diff uint64) sdkmath.Int {
	tier, ok := p.tierForHeight(height)
	if !ok {
		return sdkmath.ZeroInt()
	}

	return tier.WithDPonaOffset(diff * DPonaTxCountStep).RewardForTxCount(txCount)
}

// BandForHeightAndTxCount returns the PoNA band that applies at the given block
// height for a block containing `txCount` regular transactions, at the base
// (dPona-off) difficulty.
//
// It returns BandNone when no tier covers the height, which is the fee-only
// end state (emission has ended). The transaction count must be the number of
// REGULAR chain transactions: the app-level injected pseudo-transaction that
// vote extensions put in every block must not be counted, otherwise the zero
// band is unreachable (see app.Hoodium.PreBlocker).
func (p Params) BandForHeightAndTxCount(height, txCount uint64) ActivityBand {
	return p.BandForHeightTxCountAndDiff(height, txCount, 0)
}

// BandForHeightTxCountAndDiff returns the PoNA band at the given height and
// transaction count, with the dPona difficulty applied.
func (p Params) BandForHeightTxCountAndDiff(height, txCount, diff uint64) ActivityBand {
	tier, ok := p.tierForHeight(height)
	if !ok {
		return BandNone
	}

	return tier.WithDPonaOffset(diff * DPonaTxCountStep).BandForTxCount(txCount)
}

// TierForHeightWithDiff returns the tier covering `height` with the dPona offset
// applied, and whether one was found.
//
// Display code (the CLI, the block explorer) should use this so the ladder it
// renders is the one actually in force at the current difficulty.
func (p Params) TierForHeightWithDiff(height, diff uint64) (int, Tier, bool) {
	idx, tier, ok := p.TierForHeight(height)
	if !ok {
		return idx, tier, false
	}

	return idx, tier.WithDPonaOffset(diff * DPonaTxCountStep), true
}

// validatePoNA checks the activity-scaled fields of a single tier.
//
// A tier either disables PoNA (max threshold 0) or enables it with a complete,
// well-ordered ladder. Every rate and every threshold is REQUIRED — there are no
// optional fields and no fallbacks, so a schedule that omits one is rejected here
// rather than silently paying a different amount at runtime.
//
// Rate ordering follows the bands, zero being the floor:
//
//	Zero <= Single <= Low <= Mid <= Max
//
// Threshold ordering follows the bands, each one gating the next:
//
//	2 <= Low < Mid < Max
//
// The lower bound of 2 is not arbitrary: a band that required 0 or 1 transactions
// could never be selected, because tx_count 0 and 1 are the zero and single
// bands. Such a tier would be dead configuration.
func (t Tier) validatePoNA(idx int) error {
	// Every band's rate is required and must be non-negative. Declared in the
	// same max-to-zero order as Tier.Ladder, so a validation failure names the
	// band in the order an operator reads the ladder on the panel.
	for _, r := range []struct {
		name   string
		amount sdkmath.Int
	}{
		{"max activity", t.MaxActivityReward},
		{"high activity", t.HighActivityReward},
		{"mid activity", t.MidActivityReward},
		{"low activity", t.LowActivityReward},
		{"quint activity", t.QuintActivityReward},
		{"quart activity", t.QuartActivityReward},
		{"trio activity", t.TrioActivityReward},
		{"duo activity", t.DuoActivityReward},
		{"single activity", t.SingleActivityReward},
		{"zero activity", t.ZeroActivityReward},
	} {
		if r.amount.IsNil() {
			return fmt.Errorf("pona: tier %d %s reward must be set", idx, r.name)
		}
		if r.amount.IsNegative() {
			return fmt.Errorf("pona: tier %d %s reward must be non-negative", idx, r.name)
		}
	}

	// Threshold ordering follows the bands, each one gating the next:
	// MinLow <= low < mid < high < max.
	//
	// The lower bound is MinLowTxCountThreshold (6), not 2: the five exact bands
	// are derived as low-1 .. low-5, so a smaller low would push `single` onto
	// tx_count 0 and collide with the zero band.
	if t.LowTxCountThreshold < MinLowTxCountThreshold {
		return fmt.Errorf(
			"pona: tier %d low tx count threshold %d must be at least %d (quint..single are derived from it as low-1..low-5)",
			idx, t.LowTxCountThreshold, MinLowTxCountThreshold,
		)
	}
	if t.MidTxCountThreshold <= t.LowTxCountThreshold {
		return fmt.Errorf(
			"pona: tier %d mid tx count threshold %d must be greater than the low threshold %d",
			idx, t.MidTxCountThreshold, t.LowTxCountThreshold,
		)
	}
	if t.HighTxCountThreshold <= t.MidTxCountThreshold {
		return fmt.Errorf(
			"pona: tier %d high tx count threshold %d must be greater than the mid threshold %d",
			idx, t.HighTxCountThreshold, t.MidTxCountThreshold,
		)
	}
	if t.MaxTxCountThreshold <= t.HighTxCountThreshold {
		return fmt.Errorf(
			"pona: tier %d max tx count threshold %d must be greater than the high threshold %d",
			idx, t.MaxTxCountThreshold, t.HighTxCountThreshold,
		)
	}

	// Rate ordering: max >= high >= mid >= low >= quint >= quart >= trio >= duo
	// >= single >= zero.
	//
	// Evaluated MAX -> ZERO, matching the order the ladder is declared everywhere
	// else (proto, genesis, Tier.Ladder, the switch above). Each entry asserts
	// "this band must not pay LESS than the band below it".
	for _, pair := range []struct {
		name     string
		upper    sdkmath.Int
		upperKey string
		lower    sdkmath.Int
	}{
		{"max", t.MaxActivityReward, "high activity", t.HighActivityReward},
		{"high activity", t.HighActivityReward, "mid activity", t.MidActivityReward},
		{"mid activity", t.MidActivityReward, "low activity", t.LowActivityReward},
		{"low activity", t.LowActivityReward, "quint activity", t.QuintActivityReward},
		{"quint activity", t.QuintActivityReward, "quart activity", t.QuartActivityReward},
		{"quart activity", t.QuartActivityReward, "trio activity", t.TrioActivityReward},
		{"trio activity", t.TrioActivityReward, "duo activity", t.DuoActivityReward},
		{"duo activity", t.DuoActivityReward, "single activity", t.SingleActivityReward},
		{"single activity", t.SingleActivityReward, "zero activity", t.ZeroActivityReward},
	} {
		// Violation: the band BELOW pays more than the band above it, i.e. the
		// upper band's reward is below the lower band's. Reported that way round so
		// the message names the band an operator must raise.
		if pair.lower.GT(pair.upper) {
			return fmt.Errorf(
				"pona: tier %d %s reward %s is below the %s reward %s",
				idx, pair.name, pair.upper.String(), pair.lowerKey, pair.lower.String(),
			)
		}
	}

	return nil
}
