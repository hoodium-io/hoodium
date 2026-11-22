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
// Each tier carries a graded Proof of Network Activity (PoNA) ladder. The reward
// actually paid for a block depends on how many REGULAR transactions it contains:
//
//	tx >= max     -> MaxActivityReward   (max)
//	mid <= tx < max -> MidActivityReward   (mid)
//	low <= tx < mid -> LowActivityReward   (low)
//	tx == 1       -> SingleActivityReward (single)
//	tx == 0       -> ZeroActivityReward  (zero)
//
// Default thresholds are max = 10, mid = 5, low = 2, so the bands are:
//
//	tx >= 10      -> max     | 5..9  -> mid | 2..4 -> low | 1 -> single | 0 -> zero
//
// ZeroActivityReward is also the FLOOR: no block is ever worth less than it. See
// Tier.RewardForTxCount.
//
// See docs/technologies/pona/pona.md for the full description.
//
//	Y0-2 (2 years): max 50 | mid 25 | low 15 | single 1 | zero 0.5 RUNE/block
//	Y2-4 (2 years): max 25 | mid 15 | low 10 | single 1 | zero 0.5 RUNE/block
//	Y4+  (open)   : max 15 | mid 10 | low  5 | single 1 | zero 0.5 RUNE/block
//
// expressed as block-height boundaries at a 6s target block time.
//
// NOTE ON THE HEIGHT BOUNDARIES: Tier.EndHeight is EXCLUSIVE and Validate requires
// tiers to be contiguous (tier[i].EndHeight == tier[i+1].StartHeight) with the
// first tier starting at 0. The boundaries below therefore use the natural
// "next tier starts where the previous ended" form (0, 10519200, 21038400) rather
// than an inclusive start at 1 / 10519201. That shifts each subsequent tier's
// first block by one height and keeps every tier's block COUNT identical.
func DefaultTiers() []Tier {
	// runeAmount(n) is n whole RUNE; runeAmountPrec(n, prec) is n * 10^-prec
	// RUNE (so half a RUNE is runeAmountPrec(5, 1)). Both are expressed in arune.
	runeAmount := RuneAmount
	runeAmountPrec := RuneAmountPrec

	// Every tier carries the same ten-band ladder shape; only the amounts change.
	// The thresholds are the dPona base (low 6 / mid 10 / high 15 / max 20).
	ladder := func(max, high, mid, low, quint, quart, trio, duo, single sdkmath.Int) Tier {
		return Tier{
			MaxActivityReward:    max,
			MaxTxCountThreshold:  DefaultMaxTxCountThreshold,
			HighActivityReward:   high,
			HighTxCountThreshold: DefaultHighTxCountThreshold,
			MidActivityReward:    mid,
			MidTxCountThreshold:  DefaultMidTxCountThreshold,
			LowActivityReward:    low,
			LowTxCountThreshold:  DefaultLowTxCountThreshold,
			QuintActivityReward:  quint,
			QuartActivityReward:  quart,
			TrioActivityReward:   trio,
			DuoActivityReward:    duo,
			SingleActivityReward: single,
			ZeroActivityReward:   runeAmountPrec(5, 1), // 0.5 RUNE, also the floor
		}
	}

	t1 := ladder(
		runeAmount(50), runeAmount(40), runeAmount(30), runeAmount(20),
		runeAmount(5), runeAmount(4), runeAmount(3), runeAmount(2), runeAmount(1),
	)
	t1.StartHeight, t1.EndHeight = 0, BlocksPerTwoYearsAt6s

	t2 := ladder(
		runeAmount(40), runeAmount(30), runeAmount(20), runeAmount(15),
		runeAmount(5), runeAmount(4), runeAmount(3), runeAmount(2), runeAmount(1),
	)
	t2.StartHeight, t2.EndHeight = BlocksPerTwoYearsAt6s, 4*BlocksPerYearAt6s

	t3 := ladder(
		runeAmount(30), runeAmount(20), runeAmount(15), runeAmount(10),
		runeAmount(5), runeAmount(4), runeAmount(3), runeAmount(2), runeAmount(1),
	)
	t3.StartHeight, t3.EndHeight = 4*BlocksPerYearAt6s, 0 // open-ended

	return []Tier{t1, t2, t3}
}

// DefaultFeeOnlyTier returns the fee-funded ladder — "Tier 4" of the design.
//
// It is NOT a height tier: it has no position in Params.Tiers and no start/end
// height. It is paid once the validator reward reserve can no longer cover the
// scheduled reward, and its payout is CLAMPED to the block's actual fee income:
//
//	paid = min(scheduled, fee_skim_percent% * block_fees)
//
// The amounts are therefore much smaller than the reserve-funded tiers: at light
// traffic 20 transactions yield ~1.08 RUNE of fees, so a max band of 1.0 RUNE is
// already 92% of the block's fee income.
//
// The zero band is deliberately ZERO, and must stay zero: a block with no
// transactions has no fees, so there is nothing to pay from. That is what makes
// this ladder self-describing -- fees only, so no fees means no reward.
func DefaultFeeOnlyTier() Tier {
	return Tier{
		MaxActivityReward:    RuneAmount(1),
		MaxTxCountThreshold:  DefaultMaxTxCountThreshold,
		HighActivityReward:   RuneAmountPrec(7, 1), // 0.7
		HighTxCountThreshold: DefaultHighTxCountThreshold,
		MidActivityReward:    RuneAmountPrec(5, 1), // 0.5
		MidTxCountThreshold:  DefaultMidTxCountThreshold,
		LowActivityReward:    RuneAmountPrec(1, 1), // 0.1
		LowTxCountThreshold:  DefaultLowTxCountThreshold,
		QuintActivityReward:  RuneAmountPrec(5, 2), // 0.05
		QuartActivityReward:  RuneAmountPrec(4, 2), // 0.04
		TrioActivityReward:   RuneAmountPrec(3, 2), // 0.03
		DuoActivityReward:    RuneAmountPrec(2, 2), // 0.02
		SingleActivityReward: RuneAmountPrec(1, 2), // 0.01
		ZeroActivityReward:   sdkmath.ZeroInt(),    // fees only: no fees, no reward
	}
}

// DefaultFeeSkimPercent is the share of a block's collected fees that the
// fee-only ladder may pay out: 75%.
//
// It is a percentage of fees validators would otherwise receive through
// x/distribution anyway, so both paths pay validators; this only decides the
// split. At 75%, ordinary light traffic reaches the full scheduled reward for
// every band except max, which clamps to 0.81 of its 1.0 ceiling.
const DefaultFeeSkimPercent = uint64(75)

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
		Tiers:          DefaultTiers(),
		Denom:          runeDenom,
		FeeOnlyTier:    DefaultFeeOnlyTier(),
		FeeSkimPercent: DefaultFeeSkimPercent,
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

	// The fee-only ladder ("Tier 4") is validated like any other ladder, but it is
	// NOT a height tier: it must carry no position, and its zero band must be
	// exactly zero because a block with no transactions has no fees to pay from.
	if err := p.validateFeeOnlyTier(); err != nil {
		return err
	}

	if p.FeeSkimPercent > 100 {
		return fmt.Errorf("pona: fee skim percent %d must be at most 100", p.FeeSkimPercent)
	}

	return nil
}

// validateFeeOnlyTier checks the fee-funded fallback ladder.
func (p Params) validateFeeOnlyTier() error {
	t := p.FeeOnlyTier

	// Must be fully specified. An empty ladder (all zero rates) is only reachable
	// if the field was never set, which would silently pay nothing forever.
	if t.MaxActivityReward.IsNil() {
		return fmt.Errorf("pona: fee-only tier max activity reward must be set")
	}

	if err := t.validatePoNA(-1); err != nil {
		return fmt.Errorf("fee-only tier: %w", err)
	}

	// Not a height tier: no start, no end.
	if t.StartHeight != 0 {
		return fmt.Errorf(
			"pona: fee-only tier must not set start_height (got %d); it is selected by reserve exhaustion, not by height",
			t.StartHeight,
		)
	}
	if t.EndHeight != 0 {
		return fmt.Errorf(
			"pona: fee-only tier must not set end_height (got %d); it is selected by reserve exhaustion, not by height",
			t.EndHeight,
		)
	}

	// Fees only: no fees means no reward. A non-zero zero-band would have to be
	// paid from somewhere that is not the block's fees.
	if !t.ZeroActivityReward.IsZero() {
		return fmt.Errorf(
			"pona: fee-only tier zero activity reward must be 0 (got %s); a block with no transactions collects no fees",
			t.ZeroActivityReward,
		)
	}

	return nil
}
