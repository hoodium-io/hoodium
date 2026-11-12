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

// MinLowTxCountThreshold is the smallest allowed low tx-count threshold.
//
// It is 2 because tx_count values 0 and 1 already have their own bands (zero and
// single), so a low band starting at 0 or 1 could never be selected.
const MinLowTxCountThreshold = uint64(2)

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

// BandForTxCount returns the PoNA band that applies to a block containing
// txCount regular transactions.
//
// The five bands are selected by the tier's three thresholds, which Validate
// guarantees are ordered 2 <= Low < Mid < Max, so every band is reachable and no
// clamping or fallback is needed:
//
//	txCount == 0             -> Zero
//	txCount == 1             -> Single
//	Low  <= txCount < Mid    -> Low
//	Mid  <= txCount < Max    -> Mid
//	txCount >= Max           -> Max
func (t Tier) BandForTxCount(txCount uint64) ActivityBand {
	switch {
	case txCount == 0:
		return BandZero
	case txCount == 1:
		return BandSingle
	case txCount >= t.MaxTxCountThreshold:
		return BandMax
	case txCount >= t.MidTxCountThreshold:
		return BandMid
	default:
		// LowTxCountThreshold is validated to be >= 2, so every count reaching
		// here (2 .. Mid-1) is in the low band.
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

// RewardForBand returns the tier's configured rate for a band, before the
// ZeroActivityReward floor is applied.
//
// Every tier carries all five rates, and Params are validated on the way in
// (see validatePoNA), so each band maps to exactly one field with no fallbacks.
//
// Callers that display the configured rates (the CLI, the docs) want the raw
// value; callers that decide what is actually paid must use RewardForTxCount.
func (t Tier) RewardForBand(band ActivityBand) sdkmath.Int {
	switch band {
	case BandMax:
		return t.MaxActivityReward
	case BandMid:
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
	// Every rate is required and must be non-negative.
	for _, r := range []struct {
		name   string
		amount sdkmath.Int
	}{
		{"max activity", t.MaxActivityReward},
		{"mid activity", t.MidActivityReward},
		{"low activity", t.LowActivityReward},
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

	// Every threshold is required, in the order the bands are evaluated:
	// 2 <= low < mid < max.
	//
	// There is no "PoNA disabled" mode: a chain whose economics depend on
	// transaction activity must always have a complete ladder, so an incomplete
	// tier is rejected here instead of silently paying a flat rate.
	if t.LowTxCountThreshold < MinLowTxCountThreshold {
		return fmt.Errorf(
			"pona: tier %d low tx count threshold %d must be at least %d (tx counts 0 and 1 are the zero and single bands)",
			idx, t.LowTxCountThreshold, MinLowTxCountThreshold,
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

	// Rate ordering: zero <= single <= low <= mid <= max.
	for _, pair := range []struct {
		name     string
		lower    sdkmath.Int
		lowerKey string
		upper    sdkmath.Int
	}{
		{"single activity", t.ZeroActivityReward, "zero activity", t.SingleActivityReward},
		{"low activity", t.SingleActivityReward, "single activity", t.LowActivityReward},
		{"mid activity", t.LowActivityReward, "low activity", t.MidActivityReward},
		{"max", t.MidActivityReward, "mid activity", t.MaxActivityReward},
	} {
		if pair.upper.LT(pair.lower) {
			return fmt.Errorf(
				"pona: tier %d %s reward %s is below the %s reward %s",
				idx, pair.name, pair.upper.String(), pair.lowerKey, pair.lower.String(),
			)
		}
	}

	return nil
}
