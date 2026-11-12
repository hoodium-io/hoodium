package types

import (
	sdkmath "cosmossdk.io/math"
)

// RewardForHeight returns the MAXIMUM block reward (in arune) that applies at the
// given block height, or zero when emission has ended (rewards are fee-only).
//
// It walks the tier schedule to find the tier covering `height` and returns its
// max activity reward. If no tier covers the height, it returns zero (fee-only).
//
// This function is pure and deterministic: given the same params and height it
// always returns the same reward on every node.
//
// It is the tx-count-agnostic view of the schedule (the tier's ceiling). The
// reward actually paid for a block is RewardForHeightAndTxCount (see pona.go),
// which applies Proof of Network Activity.
func (p Params) RewardForHeight(height uint64) sdkmath.Int {
	tier, ok := p.tierForHeight(height)
	if !ok {
		return sdkmath.ZeroInt()
	}
	return tier.MaxActivityReward
}

// TierForHeight returns the schedule tier covering the given height, together
// with its index in Params.Tiers, and reports whether one was found.
//
// It returns ok == false only when no tier covers the height, which for a valid
// schedule (first tier at height 0, last tier open-ended) can never happen. The
// index is returned so callers can report "tier N of M" without re-deriving it.
func (p Params) TierForHeight(height uint64) (int, Tier, bool) {
	for i, t := range p.Tiers {
		if height < t.StartHeight {
			continue
		}
		if t.EndHeight != 0 && height >= t.EndHeight {
			continue
		}
		return i, t, true
	}
	return -1, Tier{}, false
}

// tierForHeight returns the tier covering the given height and whether one was
// found.
func (p Params) tierForHeight(height uint64) (Tier, bool) {
	_, tier, ok := p.TierForHeight(height)
	return tier, ok
}
