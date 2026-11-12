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
// A tier with MaxTxCountThreshold == 0 has PoNA disabled and always pays its
// maximum reward.
//
// PoNA never blocks a transaction, never changes block validity, and never
// changes who may propose: it only decides how much of the scheduled reward is
// paid for a given block. The transaction count is captured from the ABCI
// request in the application's PreBlock hook (see app.Hoodium.PreBlocker),
// stored via keeper.SetBlockTxCount, and read back in EndBlock.
//
// See docs/technologies/pona.md for the full mechanism and economics.

// DefaultTxCountThreshold is the default minimum number of transactions a block
// must contain for the MAXIMUM (base) reward to be paid under PoNA.
//
// This is the value used by every tier of the default schedule. With the default
// thresholds (10 / 5 / 2) a block carrying:
//
//	tx_count >= 10  -> max    (MaxActivityReward)
//	5 <= tx < 10    -> mid    (MidActivityReward)
//	2 <= tx < 5     -> low    (LowActivityReward)
//	tx_count == 1   -> single (SingleActivityReward)
//	tx_count == 0   -> zero   (ZeroActivityReward)
//
// The ladder is graded so that a lightly-used chain is paid proportionally less
// than a busy one, instead of jumping straight from "one transaction" to "full
// block".
const DefaultTxCountThreshold = uint64(10)

// DefaultMidTxCountThreshold is the default lower bound of the MID band: a block
// with at least this many (and fewer than the max threshold) transactions earns
// MidActivityReward.
const DefaultMidTxCountThreshold = uint64(5)

// DefaultLowTxCountThreshold is the default lower bound of the LOW band: a block
// with at least this many (and fewer than the mid threshold) transactions earns
// LowActivityReward.
const DefaultLowTxCountThreshold = uint64(2)

// ActivityBand identifies which Proof of Network Activity band a block falls
// into. The band, not the raw transaction count, decides which of the tier's
// five rates is paid, so the decision lives in exactly one place
// (Tier.BandForTxCount) and is shared by the payout path, the emitted event and
// the CLI.
type ActivityBand string

const (
	// BandMax is the band of a busy block: txCount >= MaxTxCountThreshold.
	BandMax ActivityBand = "max"
	// BandMid is the band of a block with substantial activity:
	// MidTxCountThreshold <= txCount < MaxTxCountThreshold.
	BandMid ActivityBand = "mid"
	// BandLow is the band of a block with moderate activity:
	// LowTxCountThreshold <= txCount < MidTxCountThreshold.
	BandLow ActivityBand = "low"
	// BandSingle is the band of a block carrying exactly ONE transaction, when
	// the tier configures a single_activity_reward. It sits between the zero and
	// low bands: one transaction is more activity than none, but a single
	// transaction on its own is not a used chain.
	BandSingle ActivityBand = "single"
	// BandZero is the band of a block that contains no regular transactions.
	// It pays the tier's nominal ZeroActivityReward so an idle chain still
	// pays the proposer something while draining the reserve as slowly as
	// possible.
	BandZero ActivityBand = "zero"
	// BandNone means no tier covers the height: emission has ended and no
	// activity-scaled reward applies.
	BandNone ActivityBand = "none"
)

// PoNAEnabled reports whether the tier scales its reward by block activity.
//
// A TxCountThreshold of 0 is the "PoNA off" sentinel: it disables the activity
// scaling for that tier, so the full RewardPerBlock is always paid. This is NOT
// the threshold value — the schedule's live threshold is DefaultTxCountThreshold
// (10). Zero is used as the sentinel because a tier that required zero
// transactions to pay full would be meaningless.
func (t Tier) PoNAEnabled() bool {
	return t.MaxTxCountThreshold > 0
}

// SingleActivityEnabled reports whether the tier has a dedicated rate for a
// block carrying exactly one transaction.
//
// A zero (or unset) SingleActivityReward means "no single-transaction band": a
// block with one transaction then pays LowActivityReward, exactly as it did
// before this field existed. That keeps existing schedules working unchanged.
func (t Tier) SingleActivityEnabled() bool {
	return !t.SingleActivityReward.IsNil() && t.SingleActivityReward.IsPositive()
}

// effectiveBandThresholds returns the (low, mid, max) thresholds actually used
// for band selection, after normalising the optional lower bounds.
//
// The lower thresholds are optional: a tier that leaves them at zero (or sets
// them inconsistently) still behaves sensibly, because they are clamped into
// [2, maxThreshold]. A value of 0 or 1 would make the band meaningless (every
// block has >= 0 or >= 1 transactions, so the band could never be selected), so
// the minimum of 2 keeps each band reachable.
func (t Tier) effectiveBandThresholds() (low, mid, maxThreshold uint64) {
	maxThreshold = t.MaxTxCountThreshold
	if maxThreshold == 0 {
		return 0, 0, 0
	}

	// Clamp the lower bounds into [2, maxThreshold]; a missing bound defaults to
	// the schedule-wide default.
	low = t.LowTxCountThreshold
	if low < DefaultLowTxCountThreshold || low > maxThreshold {
		low = min(DefaultLowTxCountThreshold, maxThreshold)
	}

	mid = t.MidTxCountThreshold
	if mid < low || mid > maxThreshold {
		mid = min(DefaultMidTxCountThreshold, maxThreshold)
	}

	// Keep low <= mid after the mid repair.
	if low > mid {
		low = mid
	}

	return low, mid, maxThreshold
}

// BandForTxCount returns the PoNA band that applies to a block containing
// txCount regular transactions.
//
// A tier with PoNA disabled (MaxTxCountThreshold == 0) always reports BandMax,
// because its maximum reward is paid regardless of the block's activity.
func (t Tier) BandForTxCount(txCount uint64) ActivityBand {
	if !t.PoNAEnabled() {
		return BandMax
	}

	switch txCount {
	case 0:
		return BandZero
	case 1:
		if t.SingleActivityEnabled() {
			return BandSingle
		}

		// No single band: a 1-tx block is the weakest active block, so it earns
		// the lowest active rate.
		return BandLow
	}

	low, mid, maxThreshold := t.effectiveBandThresholds()

	switch {
	case txCount >= maxThreshold:
		return BandMax
	case txCount >= mid:
		return BandMid
	case txCount >= low:
		return BandLow
	default:
		// 2 <= txCount < low. The low bound is clamped to >= 2, so this is only
		// reachable if a tier sets low above 2; such a block is still active, so
		// it earns the lowest active rate rather than nothing.
		return BandLow
	}
}

// RewardForTxCount returns the reward this tier pays for a block containing
// `txCount` transactions, applying its PoNA bands.
//
// The result is never below the tier's ZeroActivityReward: that value is the
// floor, so an active block always earns at least as much as an empty one.
func (t Tier) RewardForTxCount(txCount uint64) sdkmath.Int {
	reward := t.RewardForBand(t.BandForTxCount(txCount))

	return t.ApplyFloor(reward)
}

// RewardForBand returns the tier's raw rate for a band, before the
// ZeroActivityReward floor is applied.
//
// Callers that display the configured rates (the CLI, the docs) want the raw
// value; callers that decide what is actually paid must use RewardForTxCount.
func (t Tier) RewardForBand(band ActivityBand) sdkmath.Int {
	switch band {
	case BandMax:
		return t.maxRewardOrDefault()
	case BandMid:
		// mid_activity_reward is a later addition, so Params written by an older
		// binary decode it as nil. Falling back to the low rate keeps an upgraded
		// node paying a sane amount instead of a nil Int.
		if t.MidActivityReward.IsNil() {
			return t.LowActivityReward
		}

		return t.MidActivityReward
	case BandLow:
		return t.LowActivityReward
	case BandSingle:
		return t.SingleActivityReward
	case BandZero:
		return t.ZeroActivityReward
	default:
		return sdkmath.ZeroInt()
	}
}

// maxRewardOrDefault returns the max activity reward.
//
// It exists so the whole paid-reward path never dereferences a nil amount: a
// schedule authored before a field existed decodes that field as nil.
func (t Tier) maxRewardOrDefault() sdkmath.Int {
	if t.MaxActivityReward.IsNil() {
		return sdkmath.ZeroInt()
	}

	return t.MaxActivityReward
}

// ApplyFloor raises reward to the tier's ZeroActivityReward when it is lower.
//
// ZeroActivityReward is the minimum value of a block, which is what makes the
// name accurate: a block is never worth less than the zero-activity rate. This
// also keeps the schedule self-consistent if a tier is ever configured with a
// low rate below its zero rate.
func (t Tier) ApplyFloor(reward sdkmath.Int) sdkmath.Int {
	if t.ZeroActivityReward.IsNil() {
		return reward
	}

	if reward.LT(t.ZeroActivityReward) {
		return t.ZeroActivityReward
	}

	return reward
}

// RewardForHeightAndTxCount returns the static block reward (in arune) that
// applies at the given block height for a block containing `txCount`
// transactions.
//
// This is the Proof of Network Activity (PoNA) reward. The tier covering the
// height provides three rates, selected by the block's transaction count (see
// RewardForTxCount).
//
// It returns zero when no tier covers the height (emission has ended).
//
// This function is pure and deterministic: the transaction count is a property
// of the block, identical on every node, so every node computes the same reward.
func (p Params) RewardForHeightAndTxCount(height, txCount uint64) sdkmath.Int {
	tier, ok := p.tierForHeight(height)
	if !ok {
		return sdkmath.ZeroInt()
	}
	return tier.RewardForTxCount(txCount)
}

// BandForHeightAndTxCount returns the PoNA band that applies at the given block
// height for a block containing `txCount` regular transactions.
//
// It returns BandNone when no tier covers the height, which is the fee-only
// end state (emission has ended). The transaction count must be the number of
// REGULAR chain transactions: the app-level injected pseudo-transaction that
// vote extensions put in every block must not be counted, otherwise the zero
// band is unreachable (see app.Hoodium.PreBlocker).
func (p Params) BandForHeightAndTxCount(height, txCount uint64) ActivityBand {
	tier, ok := p.tierForHeight(height)
	if !ok {
		return BandNone
	}
	return tier.BandForTxCount(txCount)
}

// validatePoNA checks the activity-scaled reward fields of a single tier.
//
// A tier either disables PoNA (max threshold 0) or enables it with a well-ordered,
// non-negative rate ladder:
//
//	Zero <= Single <= Low <= Mid <= Max
//
// The zero rate is the FLOOR of the paid reward (see Tier.RewardForTxCount), so a
// rate below it would make the floor silently override the configured value — an
// operator mistake that is impossible to notice from the chain's behaviour alone.
// It is rejected here instead.
//
// SingleActivityReward may be zero, which means "no single-transaction band" and
// folds tx_count == 1 into the low band. If it is set, it must be ordered like any
// other rate.
//
// The tx-count thresholds must be strictly increasing:
//
//	2 <= low < mid < max
//
// A lower bound below 2 is meaningless (every block has >= 1 transaction, so the
// band could never be selected) and is rejected rather than silently clamped, so a
// misconfigured on-chain param fails loudly at the point it is set.
func (t Tier) validatePoNA(idx int) error {
	if t.MaxTxCountThreshold == 0 {
		// PoNA disabled: the activity fields are ignored, but must not be
		// negative if set.
		for _, r := range []struct {
			name   string
			amount sdkmath.Int
		}{
			{"low activity", t.LowActivityReward},
			{"mid activity", t.MidActivityReward},
			{"single activity", t.SingleActivityReward},
			{"zero activity", t.ZeroActivityReward},
		} {
			if !r.amount.IsNil() && r.amount.IsNegative() {
				return fmt.Errorf("pona: tier %d %s reward must be non-negative", idx, r.name)
			}
		}

		return nil
	}

	// Every active rate must be present and non-negative.
	//
	// MidActivityReward is the exception: it is optional and falls back to the low
	// rate (see RewardForBand), so a tier may omit it. Requiring it here would
	// contradict that fallback and make a valid three-step schedule impossible to
	// configure.
	for _, r := range []struct {
		name   string
		amount sdkmath.Int
	}{
		{"low activity", t.LowActivityReward},
		{"zero activity", t.ZeroActivityReward},
	} {
		if r.amount.IsNil() || r.amount.IsNegative() {
			return fmt.Errorf("pona: tier %d %s reward must be non-negative", idx, r.name)
		}
	}
	if !t.MidActivityReward.IsNil() && t.MidActivityReward.IsNegative() {
		return fmt.Errorf("pona: tier %d mid activity reward must be non-negative", idx)
	}
	if !t.SingleActivityReward.IsNil() && t.SingleActivityReward.IsNegative() {
		return fmt.Errorf("pona: tier %d single activity reward must be non-negative", idx)
	}

	// Threshold ordering.
	if t.LowTxCountThreshold < DefaultLowTxCountThreshold {
		return fmt.Errorf(
			"pona: tier %d low tx count threshold %d must be at least %d",
			idx, t.LowTxCountThreshold, DefaultLowTxCountThreshold,
		)
	}
	if t.MidTxCountThreshold <= t.LowTxCountThreshold {
		return fmt.Errorf(
			"pona: tier %d mid tx count threshold %d must be greater than the low threshold %d",
			idx, t.MidTxCountThreshold, t.LowTxCountThreshold,
		)
	}
	if t.MaxTxCountThreshold <= t.MidTxCountThreshold {
		return fmt.Errorf(
			"pona: tier %d max tx count threshold %d must be greater than the mid threshold %d",
			idx, t.MaxTxCountThreshold, t.MidTxCountThreshold,
		)
	}

	// Rate ordering. ZeroActivityReward is the floor applied to every band, so
	// nothing may sit below it.
	if t.LowActivityReward.LT(t.ZeroActivityReward) {
		return fmt.Errorf(
			"pona: tier %d low activity reward %s is below the zero activity floor %s",
			idx, t.LowActivityReward.String(), t.ZeroActivityReward.String(),
		)
	}
	if t.SingleActivityEnabled() {
		if t.SingleActivityReward.LT(t.ZeroActivityReward) {
			return fmt.Errorf(
				"pona: tier %d single activity reward %s is below the zero activity floor %s",
				idx, t.SingleActivityReward.String(), t.ZeroActivityReward.String(),
			)
		}
		if t.LowActivityReward.LT(t.SingleActivityReward) {
			return fmt.Errorf(
				"pona: tier %d low activity reward %s is below the single activity reward %s",
				idx, t.LowActivityReward.String(), t.SingleActivityReward.String(),
			)
		}
	}

	// MidActivityReward is optional; when omitted the low rate is used instead
	// (RewardForBand), so there is nothing to order. When set, it must sit above
	// low and at or below max.
	if !t.MidActivityReward.IsNil() {
		if t.MidActivityReward.LT(t.LowActivityReward) {
			return fmt.Errorf(
				"pona: tier %d mid activity reward %s is below the low activity reward %s",
				idx, t.MidActivityReward.String(), t.LowActivityReward.String(),
			)
		}
		if t.MaxActivityReward.LT(t.MidActivityReward) {
			return fmt.Errorf(
				"pona: tier %d max reward %s is below the mid activity reward %s",
				idx, t.MaxActivityReward.String(), t.MidActivityReward.String(),
			)
		}

		return nil
	}

	// No mid rate: max must still be at least the low rate it falls back to.
	if t.MaxActivityReward.LT(t.LowActivityReward) {
		return fmt.Errorf(
			"pona: tier %d max reward %s is below the low activity reward %s",
			idx, t.MaxActivityReward.String(), t.LowActivityReward.String(),
		)
	}

	return nil
}
