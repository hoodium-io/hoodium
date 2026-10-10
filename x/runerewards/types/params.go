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

// DefaultTiers returns the Hoodium emission schedule (user-confirmed).
//
// Each tier carries a full (base) reward plus its Proof of Network Activity
// (PoNA) rates. The reward actually paid for a block depends on how many
// transactions the block contains:
//
//	tx >= TxCountThreshold   -> RewardPerBlock    (full)
//	0 < tx < TxCountThreshold -> LowActivityReward
//	tx == 0                  -> ZeroActivityReward
//
// See docs/technologies/pona.md for the full description.
//
//	Y0-2 (2 years): full 50 | low 20 | zero 1 RUNE/block
//	Y2-4 (2 years): full 25 | low 10 | zero 1 RUNE/block
//	Y4+  (open)   : full 10 | low  5 | zero 1 RUNE/block
//
// expressed as block-height boundaries at a 6s target block time.
func DefaultTiers() []Tier {
	oneRune := sdkmath.NewIntWithDecimal(1, 18)
	runeAmount := func(n int64) sdkmath.Int { return oneRune.MulRaw(n) }

	return []Tier{
		{
			StartHeight:        0,
			EndHeight:          BlocksPerTwoYearsAt6s, // 10,519,200
			RewardPerBlock:     runeAmount(50),
			TxCountThreshold:   DefaultTxCountThreshold,
			LowActivityReward:  runeAmount(20),
			ZeroActivityReward: runeAmount(1),
		},
		{
			StartHeight:        BlocksPerTwoYearsAt6s,
			EndHeight:          4 * BlocksPerYearAt6s, // 21,038,400
			RewardPerBlock:     runeAmount(25),
			TxCountThreshold:   DefaultTxCountThreshold,
			LowActivityReward:  runeAmount(10),
			ZeroActivityReward: runeAmount(1),
		},
		{
			StartHeight:        4 * BlocksPerYearAt6s, // 21,038,400
			EndHeight:          0,                     // open-ended (effectively infinite)
			RewardPerBlock:     runeAmount(10),
			TxCountThreshold:   DefaultTxCountThreshold,
			LowActivityReward:  runeAmount(5),
			ZeroActivityReward: runeAmount(1),
		},
	}
}

// DefaultParams returns the default runerewards module parameters.
func DefaultParams(runeDenom string) Params {
	return Params{
		Tiers: DefaultTiers(),
		Denom: runeDenom,
	}
}

// Validate performs basic validation of the parameters.
func (p Params) Validate() error {
	if p.Denom == "" {
		return fmt.Errorf("runerewards: denom must not be empty")
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
		if err := t.validatePoNA(i); err != nil {
			return err
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
