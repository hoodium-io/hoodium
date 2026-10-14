package types

import (
	sdkmath "cosmossdk.io/math"
)

// RewardForHeight returns the static block reward (in arune) that applies at the
// given block height, or zero when emission has ended (rewards are fee-only).
//
// It walks the tier schedule to find the tier covering `height` and returns its
// per-block reward. If no tier covers the height, it returns zero (fee-only).
//
// This function is pure and deterministic: given the same params and height it
// always returns the same reward on every node.
//
// It is the tx-count-agnostic view of the schedule (the tier's full base
// reward). The reward actually paid for a block is
// RewardForHeightAndTxCount (see pona.go), which applies Proof of Network
// Activity.
func (p Params) RewardForHeight(height uint64) sdkmath.Int {
	tier, ok := p.tierForHeight(height)
	if !ok {
		return sdkmath.ZeroInt()
	}
	return tier.RewardPerBlock
}

// tierForHeight returns the tier covering the given height and whether one was
// found.
func (p Params) tierForHeight(height uint64) (Tier, bool) {
	for _, t := range p.Tiers {
		if height < t.StartHeight {
			continue
		}
		if t.EndHeight != 0 && height >= t.EndHeight {
			continue
		}
		return t, true
	}
	return Tier{}, false
}
