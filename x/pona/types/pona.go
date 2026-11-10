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
	// 0 < txCount < TxCountThreshold.
	BandLow ActivityBand = "low"
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

// BandForTxCount returns the PoNA band that applies to a block containing
// txCount regular transactions.
//
// A tier with PoNA disabled (TxCountThreshold == 0) always reports BandFull,
// because its full reward is paid regardless of the block's activity.
func (t Tier) BandForTxCount(txCount uint64) ActivityBand {
	if !t.PoNAEnabled() {
		return BandFull
	}

	switch {
	case txCount >= t.TxCountThreshold:
		return BandFull
	case txCount == 0:
		return BandZero
	default:
		return BandLow
	}
}

// RewardForTxCount returns the reward this tier pays for a block containing
// `txCount` transactions, applying its PoNA bands.
func (t Tier) RewardForTxCount(txCount uint64) sdkmath.Int {
	switch t.BandForTxCount(txCount) {
	case BandFull:
		return t.RewardPerBlock
	case BandZero:
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
// A tier either disables PoNA (threshold 0) or enables it with non-negative
// low/zero rewards.
func (t Tier) validatePoNA(idx int) error {
	if t.TxCountThreshold == 0 {
		// PoNA disabled: the low/zero fields are ignored, but must not be
		// negative if set.
		if !t.LowActivityReward.IsNil() && t.LowActivityReward.IsNegative() {
			return fmt.Errorf("pona: tier %d low activity reward must be non-negative", idx)
		}
		if !t.ZeroActivityReward.IsNil() && t.ZeroActivityReward.IsNegative() {
			return fmt.Errorf("pona: tier %d zero activity reward must be non-negative", idx)
		}
		return nil
	}

	if t.LowActivityReward.IsNil() || t.LowActivityReward.IsNegative() {
		return fmt.Errorf("pona: tier %d low activity reward must be non-negative", idx)
	}
	if t.ZeroActivityReward.IsNil() || t.ZeroActivityReward.IsNegative() {
		return fmt.Errorf("pona: tier %d zero activity reward must be non-negative", idx)
	}
	return nil
}
