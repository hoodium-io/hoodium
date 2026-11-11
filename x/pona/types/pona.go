package types

import (
	"fmt"

	sdkmath "cosmossdk.io/math"
)

// Proof of Network Activity (PoNA).
//
// PoNA scales the block reward by how many transactions the block actually
// contains. Each emission tier carries three rates:
//
//	txCount >= TxCountThreshold    -> RewardPerBlock       (full)
//	txCount > 0                    -> LowActivityReward    (low)
//	txCount == 0                   -> ZeroActivityReward   (zero, the floor)
//
// ZeroActivityReward is also the FLOOR: no band ever pays less than it, so a
// block is always worth at least that much. A tier with TxCountThreshold == 0
// has PoNA disabled and always pays its full RewardPerBlock.
//
// PoNA never blocks a transaction, never changes block validity, and never
// changes who may propose: it only decides how much of the scheduled reward is
// paid for a given block. The transaction count is captured from the ABCI
// request in the application's PreBlock hook (see app.Hoodium.PreBlocker),
// stored via keeper.SetBlockTxCount, and read back in EndBlock.
//
// See docs/technologies/pona.md for the full mechanism and economics.

// DefaultTxCountThreshold is the default minimum number of transactions a block
// must contain for the full (base) reward to be paid under PoNA.
//
// This is the value used by the default schedule (all three tiers). With the
// default threshold of 10, a block carrying:
//
//	tx_count >= 10   -> full activity  (RewardPerBlock)
//	tx_count > 0     -> low activity   (LowActivityReward), raised to the floor
//	tx_count == 0    -> zero activity  (ZeroActivityReward)
//
// A block with 1..9 transactions is therefore LOW activity: it takes at least
// DefaultTxCountThreshold transactions to earn the full rate.
const DefaultTxCountThreshold = uint64(10)

// ActivityBand identifies which Proof of Network Activity band a block falls
// into. The band, not the raw transaction count, decides which of the tier's
// three rates is paid, so the decision lives in exactly one place
// (Tier.BandForTxCount) and is shared by the payout path, the emitted event and
// the CLI.
type ActivityBand string

const (
	// BandFull is the band of a busy block: txCount >= TxCountThreshold.
	BandFull ActivityBand = "full"
	// BandLow is the band of a block with some, but not enough, activity:
	// 1 < txCount < TxCountThreshold.
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
	return t.TxCountThreshold > 0
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

// BandForTxCount returns the PoNA band that applies to a block containing
// txCount regular transactions.
//
// A tier with PoNA disabled (TxCountThreshold == 0) always reports BandFull,
// because its full reward is paid regardless of the block's activity.
func (t Tier) BandForTxCount(txCount uint64) ActivityBand {
	if !t.PoNAEnabled() {
		return BandFull
	}

	switch txCount {
	case 0:
		return BandZero
	case 1:
		if t.SingleActivityEnabled() {
			return BandSingle
		}

		return BandLow
	default:
		if txCount >= t.TxCountThreshold {
			return BandFull
		}

		return BandLow
	}
}

// RewardForTxCount returns the reward this tier pays for a block containing
// `txCount` transactions, applying its PoNA bands.
//
// The result is never below the tier's ZeroActivityReward: that value is the
// floor, so a block that qualifies for the low band still earns at least as much
// as an empty one. With the default schedule (zero = 1 RUNE) any block with
// 1..9 transactions is paid the low rate, which is >= 1 RUNE.
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
	case BandFull:
		return t.RewardPerBlock
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
// A tier either disables PoNA (threshold 0) or enables it with a well-ordered,
// non-negative rate set:
//
//	ZeroActivityReward <= SingleActivityReward <= LowActivityReward <= RewardPerBlock
//
// The zero rate is the FLOOR of the paid reward (see Tier.RewardForTxCount), so
// a rate below it would make the floor silently override the configured value —
// an operator mistake that is impossible to notice from the chain's behaviour
// alone. It is rejected here instead.
//
// SingleActivityReward may be zero, which means "no single-transaction band" and
// folds tx_count == 1 into the low band. If it is set, it must be ordered like
// any other rate.
func (t Tier) validatePoNA(idx int) error {
	if t.TxCountThreshold == 0 {
		// PoNA disabled: the activity fields are ignored, but must not be
		// negative if set.
		for _, r := range []struct {
			name   string
			amount sdkmath.Int
		}{
			{"low activity", t.LowActivityReward},
			{"single activity", t.SingleActivityReward},
			{"zero activity", t.ZeroActivityReward},
		} {
			if !r.amount.IsNil() && r.amount.IsNegative() {
				return fmt.Errorf("pona: tier %d %s reward must be non-negative", idx, r.name)
			}
		}

		return nil
	}

	if t.LowActivityReward.IsNil() || t.LowActivityReward.IsNegative() {
		return fmt.Errorf("pona: tier %d low activity reward must be non-negative", idx)
	}
	if t.ZeroActivityReward.IsNil() || t.ZeroActivityReward.IsNegative() {
		return fmt.Errorf("pona: tier %d zero activity reward must be non-negative", idx)
	}
	if !t.SingleActivityReward.IsNil() && t.SingleActivityReward.IsNegative() {
		return fmt.Errorf("pona: tier %d single activity reward must be non-negative", idx)
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
	if t.RewardPerBlock.LT(t.LowActivityReward) {
		return fmt.Errorf(
			"pona: tier %d full reward %s is below the low activity reward %s",
			idx, t.RewardPerBlock.String(), t.LowActivityReward.String(),
		)
	}

	return nil
}
