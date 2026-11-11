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

// RuneDecimals is the exponent separating RUNE from its base denom arune
// (1 RUNE = 10^18 arune, see BaseDenomUnit in the root types package).
const RuneDecimals = 18

// DefaultTiers returns the Hoodium emission schedule.
//
// Each tier carries a full (base) reward plus its Proof of Network Activity
// (PoNA) rates. The reward actually paid for a block depends on how many
// transactions the block contains:
//
//	tx >= TxCountThreshold   -> RewardPerBlock       (full)
//	tx == 1                  -> SingleActivityReward (single)
//	1 < tx < TxCountThreshold -> LowActivityReward    (low)
//	tx == 0                  -> ZeroActivityReward    (zero)
//
// ZeroActivityReward is also the FLOOR of the paid reward: no block is ever worth
// less than it. See Tier.RewardForTxCount.
//
// See docs/technologies/pona.md for the full description.
//
//	Y0-2 (2 years): full 50 | low 20 | single 1 | zero 0.5 RUNE/block
//	Y2-4 (2 years): full 25 | low 10 | single 1 | zero 0.5 RUNE/block
//	Y4+  (open)   : full 10 | low  5 | single 1 | zero 0.5 RUNE/block
//
// expressed as block-height boundaries at a 6s target block time.
func DefaultTiers() []Tier {
	// runeAmount(n) is n whole RUNE; runeAmountPrec(n, prec) is n * 10^-prec
	// RUNE (so half a RUNE is runeAmountPrec(5, 1)). Both are expressed in arune.
	runeAmount := RuneAmount
	runeAmountPrec := RuneAmountPrec

	return []Tier{
		{
			StartHeight:          0,
			EndHeight:            BlocksPerTwoYearsAt6s, // 10,519,200
			RewardPerBlock:       runeAmount(50),
			TxCountThreshold:     DefaultTxCountThreshold,
			LowActivityReward:    runeAmount(20),
			SingleActivityReward: runeAmount(1),
			ZeroActivityReward:   runeAmountPrec(5, 1), // 0.5 RUNE, also the floor
		},
		{
			StartHeight:          BlocksPerTwoYearsAt6s,
			EndHeight:            4 * BlocksPerYearAt6s, // 21,038,400
			RewardPerBlock:       runeAmount(25),
			TxCountThreshold:     DefaultTxCountThreshold,
			LowActivityReward:    runeAmount(10),
			SingleActivityReward: runeAmount(1),
			ZeroActivityReward:   runeAmountPrec(5, 1),
		},
		{
			StartHeight:          4 * BlocksPerYearAt6s, // 21,038,400
			EndHeight:            0,                     // open-ended (effectively infinite)
			RewardPerBlock:       runeAmount(10),
			TxCountThreshold:     DefaultTxCountThreshold,
			LowActivityReward:    runeAmount(5),
			SingleActivityReward: runeAmount(1),
			ZeroActivityReward:   runeAmountPrec(5, 1),
		},
	}
}

// RuneAmount returns n whole RUNE expressed in arune (the base denom).
//
// It exists so the schedule can be written in RUNE terms without repeating the
// 10^18 literal at every call site.
func RuneAmount(n int64) sdkmath.Int {
	return sdkmath.NewIntWithDecimal(1, RuneDecimals).MulRaw(n)
}

// RuneAmountPrec returns n * 10^-prec RUNE expressed in arune, so half a RUNE is
// RuneAmountPrec(5, 1).
func RuneAmountPrec(n, prec int64) sdkmath.Int {
	return sdkmath.NewIntWithDecimal(1, RuneDecimals).QuoRaw(pow10(prec)).MulRaw(n)
}

// RuneString formats an arune amount as a decimal RUNE string for human-readable
// output (logs, CLI). It never replaces the exact amount: callers that need the
// exact value keep the arune integer alongside.
//
// It rounds by truncation of the 18-decimal fraction's trailing zeros, so
// 500000000000000000 (0.5 RUNE) renders as "0.5".
func RuneString(amount sdkmath.Int) string {
	if amount.IsNil() || amount.IsZero() {
		return "0"
	}

	unit := sdkmath.NewIntWithDecimal(1, RuneDecimals)
	quo := amount.Quo(unit)
	rem := amount.Mod(unit)
	if rem.IsZero() {
		return quo.String()
	}

	fraction := fmt.Sprintf("%0*s", RuneDecimals, rem.String())

	return fmt.Sprintf("%s.%s", quo, trimTrailingZeros(fraction))
}

// trimTrailingZeros removes the redundant trailing zeros of a fixed-width
// fraction.
func trimTrailingZeros(s string) string {
	end := len(s)
	for end > 0 && s[end-1] == '0' {
		end--
	}

	if end == 0 {
		return "0"
	}

	return s[:end]
}

// pow10 returns 10^n for the small n used by the RUNE helpers.
func pow10(n int64) int64 {
	result := int64(1)
	for range n {
		result *= 10
	}

	return result
}

// DefaultParams returns the default pona module parameters.
func DefaultParams(runeDenom string) Params {
	return Params{
		Tiers: DefaultTiers(),
		Denom: runeDenom,
	}
}

// Validate performs basic validation of the parameters.
func (p Params) Validate() error {
	if p.Denom == "" {
		return fmt.Errorf("pona: denom must not be empty")
	}
	if len(p.Tiers) == 0 {
		return fmt.Errorf("pona: at least one tier is required")
	}
	if p.Tiers[0].StartHeight != 0 {
		return fmt.Errorf("pona: first tier must start at height 0, got %d", p.Tiers[0].StartHeight)
	}

	for i, t := range p.Tiers {
		if t.RewardPerBlock.IsNil() || t.RewardPerBlock.IsNegative() {
			return fmt.Errorf("pona: tier %d reward must be non-negative", i)
		}
		if err := t.validatePoNA(i); err != nil {
			return err
		}
		if t.EndHeight != 0 && t.EndHeight <= t.StartHeight {
			return fmt.Errorf(
				"pona: tier %d end height %d must be greater than start height %d",
				i, t.EndHeight, t.StartHeight,
			)
		}
		// Contiguity: each tier (except the last) must end exactly where the
		// next one starts.
		if i < len(p.Tiers)-1 {
			next := p.Tiers[i+1]
			if t.EndHeight != next.StartHeight {
				return fmt.Errorf(
					"pona: tier %d end height %d must equal tier %d start height %d",
					i, t.EndHeight, i+1, next.StartHeight,
				)
			}
		}
	}

	// The last tier must be open-ended so every height maps to a tier.
	if last := p.Tiers[len(p.Tiers)-1]; last.EndHeight != 0 {
		return fmt.Errorf("pona: last tier must be open-ended (end height 0)")
	}

	return nil
}
