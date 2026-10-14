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
//	0 < txCount < TxCountThreshold -> LowActivityReward
//	txCount == 0                   -> ZeroActivityReward
//
// A tier with TxCountThreshold == 0 has PoNA disabled and always pays its full
// RewardPerBlock.
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
// This is the value used by the default schedule (all three tiers). A block with
// fewer transactions earns a reduced reward:
//
//	tx_count >= DefaultTxCountThreshold    -> full reward      (>= 10)
//	0 < tx_count < DefaultTxCountThreshold -> low activity      (1..9)
//	tx_count == 0                          -> zero activity     (0)
const DefaultTxCountThreshold = uint64(10)

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

// RewardForTxCount returns the reward this tier pays for a block containing
// `txCount` transactions, applying its PoNA bands.
func (t Tier) RewardForTxCount(txCount uint64) sdkmath.Int {
	// PoNA disabled: always the full reward.
	if !t.PoNAEnabled() {
		return t.RewardPerBlock
	}

	switch {
	case txCount >= t.TxCountThreshold:
		return t.RewardPerBlock
	case txCount == 0:
		return t.ZeroActivityReward
	default:
		return t.LowActivityReward
	}
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

// validatePoNA checks the activity-scaled reward fields of a single tier.
//
// A tier either disables PoNA (threshold 0) or enables it with non-negative
// low/zero rewards.
func (t Tier) validatePoNA(idx int) error {
	if t.TxCountThreshold == 0 {
		// PoNA disabled: the low/zero fields are ignored, but must not be
		// negative if set.
		if !t.LowActivityReward.IsNil() && t.LowActivityReward.IsNegative() {
			return fmt.Errorf("runerewards: tier %d low activity reward must be non-negative", idx)
		}
		if !t.ZeroActivityReward.IsNil() && t.ZeroActivityReward.IsNegative() {
			return fmt.Errorf("runerewards: tier %d zero activity reward must be non-negative", idx)
		}
		return nil
	}

	if t.LowActivityReward.IsNil() || t.LowActivityReward.IsNegative() {
		return fmt.Errorf("runerewards: tier %d low activity reward must be non-negative", idx)
	}
	if t.ZeroActivityReward.IsNil() || t.ZeroActivityReward.IsNegative() {
		return fmt.Errorf("runerewards: tier %d zero activity reward must be non-negative", idx)
	}
	return nil
}
