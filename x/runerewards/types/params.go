package types

import (
	"fmt"

	sdkmath "cosmossdk.io/math"
)

// BlocksPerYearAt6s is the number of blocks in one year assuming the Hoodium
// target block time of 6 seconds. It is the unit used to express the reward
// schedule's tier boundaries in block height.
//
//	365.25 days * 24h * 60m * 60s / 6s = 5,259,600 blocks
const BlocksPerYearAt6s = uint64(5_259_600)

// BlocksPerTwoYearsAt6s is two schedule-years, the length of the first tier.
const BlocksPerTwoYearsAt6s = 2 * BlocksPerYearAt6s

// DefaultTiers returns the Hoodium emission schedule (user-confirmed):
//
//	Y0-2 (2 years): 50 RUNE/block
//	Y2-4 (2 years): 25 RUNE/block
//	Y4+  (open)   :  5 RUNE/block
//
// expressed as block-height boundaries at a 6s target block time.
func DefaultTiers() []Tier {
	oneRune := sdkmath.NewIntWithDecimal(1, 18)

	return []Tier{
		{
			StartHeight:    0,
			EndHeight:      BlocksPerTwoYearsAt6s, // 10,519,200
			RewardPerBlock: oneRune.MulRaw(50),
		},
		{
			StartHeight:    BlocksPerTwoYearsAt6s,
			EndHeight:      4 * BlocksPerYearAt6s, // 21,038,400
			RewardPerBlock: oneRune.MulRaw(25),
		},
		{
			StartHeight:    4 * BlocksPerYearAt6s, // 21,038,400
			EndHeight:      0,                     // open-ended (effectively infinite)
			RewardPerBlock: oneRune.MulRaw(5),
		},
	}
}

// DefaultParams returns the default runerewards module parameters.
func DefaultParams(runeDenom string) Params {
	return Params{
		Tiers:             DefaultTiers(),
		Denom:             runeDenom,
		MinRewardPerBlock: sdkmath.NewIntWithDecimal(5, 18), // 5 RUNE floor
	}
}

// Validate performs basic validation of the parameters.
func (p Params) Validate() error {
	if p.Denom == "" {
		return fmt.Errorf("runerewards: denom must not be empty")
	}
	if p.MinRewardPerBlock.IsNil() || p.MinRewardPerBlock.IsNegative() {
		return fmt.Errorf("runerewards: min reward per block must be non-negative")
	}
	if len(p.Tiers) == 0 {
		return fmt.Errorf("runerewards: at least one tier is required")
	}
	if p.Tiers[0].StartHeight != 0 {
		return fmt.Errorf("runerewards: first tier must start at height 0, got %d", p.Tiers[0].StartHeight)
	}

	for i, t := range p.Tiers {
		if t.RewardPerBlock.IsNil() || t.RewardPerBlock.IsNegative() {
			return fmt.Errorf("runerewards: tier %d reward must be non-negative", i)
		}
		if t.EndHeight != 0 && t.EndHeight <= t.StartHeight {
			return fmt.Errorf(
				"runerewards: tier %d end height %d must be greater than start height %d",
				i, t.EndHeight, t.StartHeight,
			)
		}
		// Contiguity: each tier (except the last) must end exactly where the
		// next one starts.
		if i < len(p.Tiers)-1 {
			next := p.Tiers[i+1]
			if t.EndHeight != next.StartHeight {
				return fmt.Errorf(
					"runerewards: tier %d end height %d must equal tier %d start height %d",
					i, t.EndHeight, i+1, next.StartHeight,
				)
			}
		}
	}

	// The last tier must be open-ended so every height maps to a tier.
	if last := p.Tiers[len(p.Tiers)-1]; last.EndHeight != 0 {
		return fmt.Errorf("runerewards: last tier must be open-ended (end height 0)")
	}

	return nil
}
