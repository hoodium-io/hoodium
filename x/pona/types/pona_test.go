package types_test

import (
	"testing"

	sdkmath "cosmossdk.io/math"
	"github.com/stretchr/testify/require"

	"github.com/hoodium-io/hoodium/x/pona/types"
)

// TestDefaultTiersPoNA pins the max/mid/low/single/zero ladder and the thresholds
// of the default schedule.
func TestDefaultTiersPoNA(t *testing.T) {
	tiers := types.DefaultTiers()
	require.Len(t, tiers, 3)

	testCases := []struct {
		name          string
		idx           int
		max, mid, low sdkmath.Int
		single, zero  sdkmath.Int
		maxTx, midTx  uint64
		lowTx         uint64
	}{
		{"tier 1 (Y0-2)", 0, rune(50), rune(25), rune(15), rune(1), halfRune(), 10, 5, 2},
		{"tier 2 (Y2-4)", 1, rune(25), rune(15), rune(10), rune(1), halfRune(), 10, 5, 2},
		{"tier 3 (Y4+)", 2, rune(15), rune(10), rune(5), rune(1), halfRune(), 10, 5, 2},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tier := tiers[tc.idx]
			require.Equal(t, tc.max.String(), tier.MaxActivityReward.String(), "max reward")
			require.Equal(t, tc.mid.String(), tier.MidActivityReward.String(), "mid reward")
			require.Equal(t, tc.low.String(), tier.LowActivityReward.String(), "low reward")
			require.Equal(t, tc.single.String(), tier.SingleActivityReward.String(), "single reward")
			require.Equal(t, tc.zero.String(), tier.ZeroActivityReward.String(), "zero reward")
			require.Equal(t, tc.maxTx, tier.MaxTxCountThreshold, "max threshold")
			require.Equal(t, tc.midTx, tier.MidTxCountThreshold, "mid threshold")
			require.Equal(t, tc.lowTx, tier.LowTxCountThreshold, "low threshold")
			require.True(t, tier.PoNAEnabled())
			require.True(t, tier.SingleActivityEnabled())
		})
	}
}

// TestTierHeightBoundaries pins the tier windows. EndHeight is EXCLUSIVE and the
// tiers must be contiguous, starting at height 0.
func TestTierHeightBoundaries(t *testing.T) {
	tiers := types.DefaultTiers()
	by := types.BlocksPerYearAt6s

	require.Equal(t, uint64(0), tiers[0].StartHeight)
	require.Equal(t, 2*by, tiers[0].EndHeight)
	require.Equal(t, 2*by, tiers[1].StartHeight)
	require.Equal(t, 4*by, tiers[1].EndHeight)
	require.Equal(t, 4*by, tiers[2].StartHeight)
	require.Equal(t, uint64(0), tiers[2].EndHeight, "last tier is open-ended")

	// Both tiers are 2 schedule-years long, so the block counts are identical.
	require.Equal(t, tiers[0].EndHeight-tiers[0].StartHeight, tiers[1].EndHeight-tiers[1].StartHeight)
}

// halfRune returns 0.5 RUNE expressed in arune.
func halfRune() sdkmath.Int {
	return sdkmath.NewIntWithDecimal(5, 17)
}

// TestRewardForHeightAndTxCount exercises the three PoNA bands on each tier.
func TestRewardForHeightAndTxCount(t *testing.T) {
	params := types.DefaultParams("arune")

	oneYear := types.BlocksPerYearAt6s
	twoYears := types.BlocksPerTwoYearsAt6s
	threshold := types.DefaultTxCountThreshold // 10

	// Heights chosen inside each tier.
	inTier1 := twoYears - 1
	inTier2 := twoYears + oneYear
	inTier3 := 4*oneYear + oneYear

	testCases := []struct {
		name     string
		height   uint64
		txCount  uint64
		want     sdkmath.Int
		wantBand types.ActivityBand
	}{
		// Tier 1: max 50 | mid 25 (5-9) | low 15 (2-4) | single 1 | zero 0.5
		{"tier1 max (tx == threshold)", inTier1, threshold, rune(50), types.BandMax},
		{"tier1 max (tx > threshold)", inTier1, 1000, rune(50), types.BandMax},
		{"tier1 mid (tx == mid threshold)", inTier1, 5, rune(25), types.BandMid},
		{"tier1 mid (threshold-1)", inTier1, threshold - 1, rune(25), types.BandMid},
		{"tier1 low (tx == low threshold)", inTier1, 2, rune(15), types.BandLow},
		{"tier1 low (mid-1)", inTier1, 4, rune(15), types.BandLow},
		{"tier1 single (exactly 1 tx)", inTier1, 1, rune(1), types.BandSingle},
		{"tier1 zero (no txs)", inTier1, 0, halfRune(), types.BandZero},

		// Tier 2: max 25 | mid 15 (5-9) | low 10 (2-4) | single 1 | zero 0.5
		{"tier2 max", inTier2, threshold, rune(25), types.BandMax},
		{"tier2 mid", inTier2, 7, rune(15), types.BandMid},
		{"tier2 low", inTier2, 3, rune(10), types.BandLow},
		{"tier2 single", inTier2, 1, rune(1), types.BandSingle},
		{"tier2 zero", inTier2, 0, halfRune(), types.BandZero},

		// Tier 3: max 15 | mid 10 (5-9) | low 5 (2-4) | single 1 | zero 0.5
		{"tier3 max", inTier3, threshold, rune(15), types.BandMax},
		{"tier3 max (far future)", 100 * oneYear, 42, rune(15), types.BandMax},
		{"tier3 mid", inTier3, 6, rune(10), types.BandMid},
		{"tier3 low", inTier3, 2, rune(5), types.BandLow},
		{"tier3 single", inTier3, 1, rune(1), types.BandSingle},
		{"tier3 zero", inTier3, 0, halfRune(), types.BandZero},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := params.RewardForHeightAndTxCount(tc.height, tc.txCount)
			require.Equal(t, tc.want.String(), got.String(),
				"height %d txCount %d: want %s got %s", tc.height, tc.txCount, tc.want, got)
			require.Equal(t, tc.wantBand, params.BandForHeightAndTxCount(tc.height, tc.txCount),
				"height %d txCount %d: band", tc.height, tc.txCount)
		})
	}
}

// TestPoNADisabledWhenThresholdZero verifies that a tier with a zero max
// threshold always pays its max reward, regardless of block activity.
func TestPoNADisabledWhenThresholdZero(t *testing.T) {
	tier := types.Tier{
		MaxActivityReward: rune(50),
		// MaxTxCountThreshold intentionally 0 => PoNA off.
	}
	require.False(t, tier.PoNAEnabled())
	require.Equal(t, rune(50).String(), tier.RewardForTxCount(0).String())
	require.Equal(t, rune(50).String(), tier.RewardForTxCount(1).String())
	require.Equal(t, rune(50).String(), tier.RewardForTxCount(9999).String())
}

// TestRewardForHeightAndTxCountNoTier verifies that a height with no covering
// tier yields zero (fee-only, emission ended).
//
// NOTE: a *valid* schedule always covers every height (it starts at 0 and its
// last tier is open-ended), so this defensive branch is only reachable with an
// explicit end height on the final tier — which Validate() rejects. The schedule
// is therefore left unvalidated on purpose, and the zero result must still hold.
func TestRewardForHeightAndTxCountNoTier(t *testing.T) {
	params := types.Params{
		Denom: "arune",
		Tiers: []types.Tier{
			{StartHeight: 0, EndHeight: 100, MaxActivityReward: rune(50)},
		},
	}
	// Deliberately NOT calling params.Validate(): the point is that a height
	// past the last tier is fee-only rather than panicking or paying.
	require.True(t, params.RewardForHeightAndTxCount(150, 20).IsZero())
	require.True(t, params.RewardForHeight(150).IsZero())
}

// TestBandForTxCount exercises the band selection that PoNA uses to decide
// which of a tier's rates is paid.
//
// The zero band is the important one: an idle chain must reach it, otherwise it
// pays a higher activity rate forever (see CountRegularTxs in app/abci). The
// single band is the newest: a block with exactly one transaction is more
// activity than none, but far less than a used chain.
func TestBandForTxCount(t *testing.T) {
	// A tier with the single band configured.
	withSingle := types.Tier{
		MaxActivityReward:   rune(50),
		MaxTxCountThreshold: 10, MidTxCountThreshold: 5, LowTxCountThreshold: 2,
		LowActivityReward:    rune(20),
		SingleActivityReward: rune(1),
		ZeroActivityReward:   halfRune(),
	}

	testCases := []struct {
		name    string
		tier    types.Tier
		txCount uint64
		want    types.ActivityBand
	}{
		{"no transactions is the zero band", withSingle, 0, types.BandZero},
		{"exactly one transaction is the single band", withSingle, 1, types.BandSingle},
		{"two transactions is the low band", withSingle, 2, types.BandLow},
		{"threshold-1 is still the low band", withSingle, 9, types.BandLow},
		{"exactly the threshold is the full band", withSingle, 10, types.BandMax},
		{"above the threshold is the full band", withSingle, 1000, types.BandMax},

		// Without a configured single rate, a 1-tx block folds into the low band:
		// this is what pre-existing on-chain schedules do, since the field was
		// added later and reads as zero until set.
		{
			name:    "single band disabled folds 1 tx into low",
			tier:    types.Tier{MaxActivityReward: rune(50), MaxTxCountThreshold: 10, MidTxCountThreshold: 5, LowTxCountThreshold: 2, LowActivityReward: rune(20), ZeroActivityReward: rune(1)},
			txCount: 1,
			want:    types.BandLow,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, tc.tier.BandForTxCount(tc.txCount))
			// The band must agree with the amount that is actually paid.
			require.Equal(
				t,
				tc.tier.RewardForTxCount(tc.txCount).String(),
				tc.tier.ApplyFloor(tc.tier.RewardForBand(tc.want)).String(),
			)
		})
	}
}

// TestBandForTxCountPoNADisabled verifies that a tier with PoNA switched off
// reports the full band for every transaction count.
func TestBandForTxCountPoNADisabled(t *testing.T) {
	tier := types.Tier{MaxActivityReward: rune(50)} // TxCountThreshold 0 => PoNA off

	require.False(t, tier.PoNAEnabled())
	for _, txCount := range []uint64{0, 1, 9, 10, 1000} {
		require.Equal(t, types.BandMax, tier.BandForTxCount(txCount), "txCount %d", txCount)
		require.Equal(t, rune(50).String(), tier.RewardForTxCount(txCount).String())
	}
}

// TestBandForHeightAndTxCount pins the height-aware band lookup, including the
// fee-only end state where no tier covers the height.
func TestBandForHeightAndTxCount(t *testing.T) {
	params := types.DefaultParams("arune")
	threshold := types.DefaultTxCountThreshold

	inTier1 := types.BlocksPerTwoYearsAt6s - 1
	inTier2 := types.BlocksPerTwoYearsAt6s
	inTier3 := 4 * types.BlocksPerYearAt6s

	require.Equal(t, types.BandZero, params.BandForHeightAndTxCount(inTier1, 0))
	require.Equal(t, types.BandSingle, params.BandForHeightAndTxCount(inTier1, 1))
	require.Equal(t, types.BandLow, params.BandForHeightAndTxCount(inTier1, 2))
	require.Equal(t, types.BandMax, params.BandForHeightAndTxCount(inTier1, threshold))
	require.Equal(t, types.BandZero, params.BandForHeightAndTxCount(inTier2, 0))
	require.Equal(t, types.BandSingle, params.BandForHeightAndTxCount(inTier2, 1))
	require.Equal(t, types.BandLow, params.BandForHeightAndTxCount(inTier3, 5))
	require.Equal(t, types.BandSingle, params.BandForHeightAndTxCount(inTier3, 1))
	require.Equal(t, types.BandMax, params.BandForHeightAndTxCount(inTier3, threshold))

	// A schedule with an explicit end height leaves later heights uncovered.
	gapped := types.Params{
		Denom: "arune",
		Tiers: []types.Tier{{StartHeight: 0, EndHeight: 100, MaxActivityReward: rune(50)}},
	}
	require.Equal(t, types.BandNone, gapped.BandForHeightAndTxCount(100, 5))
	require.Equal(t, types.BandMax, gapped.BandForHeightAndTxCount(99, 50))
}

// TestTierForHeight returns the index and the tier together, so callers can
// report "tier N of M" without re-deriving the position.
func TestTierForHeight(t *testing.T) {
	params := types.DefaultParams("arune")

	idx, tier, ok := params.TierForHeight(0)
	require.True(t, ok)
	require.Equal(t, 0, idx)
	require.Equal(t, params.Tiers[0].MaxActivityReward.String(), tier.MaxActivityReward.String())

	idx, _, ok = params.TierForHeight(types.BlocksPerTwoYearsAt6s)
	require.True(t, ok)
	require.Equal(t, 1, idx, "the first tier's end height is exclusive")

	idx, _, ok = params.TierForHeight(4 * types.BlocksPerYearAt6s)
	require.True(t, ok)
	require.Equal(t, 2, idx)

	idx, _, ok = params.TierForHeight(100 * types.BlocksPerYearAt6s)
	require.True(t, ok)
	require.Equal(t, 2, idx, "the last tier is open-ended")

	// No tier covers this height.
	idx, _, ok = types.Params{Tiers: []types.Tier{{StartHeight: 10, EndHeight: 20}}}.TierForHeight(20)
	require.False(t, ok)
	require.Equal(t, -1, idx)
}

// TestZeroActivityRewardIsTheFloor pins the rule that no block is ever paid less
// than the tier's zero-activity rate.
//
// The default schedule (zero = 0.5 RUNE) never triggers the floor, so the test
// also builds a tier whose low rate sits BELOW its zero rate: the precondition
// that validatePoNA rejects on chain, and the reason the floor exists as a
// safety net.
func TestZeroActivityRewardIsTheFloor(t *testing.T) {
	// A low rate below the floor: RewardForTxCount must clamp up to the floor.
	belowFloor := types.Tier{
		MaxActivityReward:   rune(50),
		MaxTxCountThreshold: 10, MidTxCountThreshold: 5, LowTxCountThreshold: 2,
		LowActivityReward:  sdkmath.NewInt(1), // 1 arune, far below 1 RUNE
		ZeroActivityReward: rune(1),
	}

	for _, txCount := range []uint64{0, 2, 5, 9} {
		got := belowFloor.RewardForTxCount(txCount)
		require.Equal(t, rune(1).String(), got.String(),
			"txCount %d must be clamped to the %s floor, got %s",
			txCount, rune(1), got)
	}
	// The full band is unaffected: it is far above the floor.
	require.Equal(t, rune(50).String(), belowFloor.RewardForTxCount(10).String())

	// The raw configured rate is still visible for reporting.
	require.Equal(t, "1", belowFloor.RewardForBand(types.BandLow).String(),
		"RewardForBand must return the raw rate, before the floor")

	// The default schedule never needs the floor: each configured rate sits
	// above the 0.5 RUNE zero rate, so every band pays exactly its own amount.
	for i, tier := range types.DefaultTiers() {
		for _, band := range []types.ActivityBand{
			types.BandMax, types.BandLow, types.BandSingle,
		} {
			require.False(t, tier.RewardForBand(band).LT(tier.ZeroActivityReward),
				"default tier %d band %s must not need the floor", i, band)
		}
		require.Equal(t, tier.ZeroActivityReward.String(),
			tier.RewardForTxCount(0).String(),
			"default tier %d zero block pays the zero rate", i)
	}
}

// TestValidateRejectsInvertedRates verifies the ordering validation: the zero
// rate is the floor, so nothing may sit below it, and the rates above it must be
// non-decreasing up to the full rate.
func TestValidateRejectsInvertedRates(t *testing.T) {
	mkTier := func(full, low, single, zero sdkmath.Int) types.Tier {
		return types.Tier{
			StartHeight:         0,
			EndHeight:           0,
			MaxActivityReward:   full,
			MaxTxCountThreshold: 10, MidTxCountThreshold: 5, LowTxCountThreshold: 2,
			LowActivityReward:    low,
			SingleActivityReward: single,
			ZeroActivityReward:   zero,
		}
	}
	noSingle := sdkmath.Int{} // unset => single band disabled

	testCases := []struct {
		name    string
		tier    types.Tier
		wantErr string
	}{
		{
			name:    "low below the zero floor",
			tier:    mkTier(rune(50), rune(1), noSingle, rune(20)),
			wantErr: "below the zero activity floor",
		},
		{
			name:    "full below the low rate",
			tier:    mkTier(rune(5), rune(20), noSingle, rune(1)),
			wantErr: "below the low activity reward",
		},
		{
			name:    "single below the zero floor",
			tier:    mkTier(rune(50), rune(20), rune(1), rune(5)),
			wantErr: "single activity reward",
		},
		{
			name:    "low below the single rate",
			tier:    mkTier(rune(50), rune(3), rune(10), rune(1)),
			wantErr: "below the single activity reward",
		},
		{
			name: "well ordered rates are accepted",
			tier: mkTier(rune(50), rune(20), rune(1), halfRune()),
		},
		{
			name: "single band may be omitted",
			tier: mkTier(rune(50), rune(20), noSingle, halfRune()),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := types.Params{Denom: "arune", Tiers: []types.Tier{tc.tier}}.Validate()
			if tc.wantErr == "" {
				require.NoError(t, err)

				return
			}
			require.ErrorContains(t, err, tc.wantErr)
		})
	}
}

// TestShippedDefaultScheduleIsValid validates the complete default schedule.
//
// It must be validated as a WHOLE, never tier by tier. Validate applies its
// checks in order and returns on the first failure:
//
//  1. denom non-empty
//  2. at least one tier
//  3. Tiers[0].StartHeight == 0
//  4. per-tier rate checks
//  5. last tier open-ended
//
// Because check 3 runs before check 5, extracting ANY default tier that does not
// start at height 0 fails on the height rule, not the open-ended rule. Only tier
// 0 starts at height 0, and being closed it then fails check 5. So no single
// default tier is a valid Params on its own -- which is exactly why the schedule
// is only ever validated whole.
func TestShippedDefaultScheduleIsValid(t *testing.T) {
	require.NoError(t, types.DefaultParams("arune").Validate())

	tiers := types.DefaultTiers()
	require.Len(t, tiers, 3)

	// Tiers 1 and 2 do not start at height 0, so they fail check 3 first.
	for i := 1; i < len(tiers); i++ {
		require.NotZero(t, tiers[i].StartHeight, "tier %d should not start at 0", i)

		single := types.Params{Denom: "arune", Tiers: []types.Tier{tiers[i]}}
		require.ErrorContains(t, single.Validate(), "first tier must start at height 0",
			"tier %d starts above 0, so it fails the height rule before the open-ended rule", i)
	}

	// Tier 0 starts at height 0 but is closed, so it passes check 3 and fails
	// check 5 (the open-ended rule).
	require.Zero(t, tiers[0].StartHeight)
	require.NotZero(t, tiers[0].EndHeight, "tier 0 must be closed for this to be the failure")

	single := types.Params{Denom: "arune", Tiers: []types.Tier{tiers[0]}}
	require.ErrorContains(t, single.Validate(), "last tier must be open-ended",
		"tier 0 starts at 0, so it reaches the open-ended rule and fails there")

	// Consequence: no single tier validates on its own, so a caller must validate
	// the whole slice. Asserting it directly keeps the invariant obvious.
	for i, tier := range tiers {
		require.Error(t,
			types.Params{Denom: "arune", Tiers: []types.Tier{tier}}.Validate(),
			"tier %d must not validate in isolation", i)
	}
}

// TestParamsValidateRejectsBadPoNA verifies that an enabled tier with a missing
// or negative PoNA rate is rejected, while a disabled (threshold 0) tier is not.
func TestParamsValidateRejectsBadPoNA(t *testing.T) {
	mkTier := func(low, zero sdkmath.Int) types.Tier {
		return types.Tier{
			StartHeight:         0,
			EndHeight:           0,
			MaxActivityReward:   rune(50),
			MaxTxCountThreshold: 10, MidTxCountThreshold: 5, LowTxCountThreshold: 2,
			LowActivityReward:  low,
			ZeroActivityReward: zero,
		}
	}

	// Negative low activity reward.
	bad := types.Params{Denom: "arune", Tiers: []types.Tier{mkTier(rune(-20), rune(1))}}
	require.Error(t, bad.Validate())

	// Nil zero activity reward (PoNA enabled).
	bad = types.Params{Denom: "arune", Tiers: []types.Tier{mkTier(rune(20), sdkmath.Int{})}}
	require.Error(t, bad.Validate())

	// Valid PoNA tier.
	good := types.Params{Denom: "arune", Tiers: []types.Tier{mkTier(rune(20), rune(1))}}
	require.NoError(t, good.Validate())

	// PoNA disabled (threshold 0): nil low/zero fields are fine.
	disabled := types.Params{
		Denom: "arune",
		Tiers: []types.Tier{{StartHeight: 0, EndHeight: 0, MaxActivityReward: rune(50)}},
	}
	require.NoError(t, disabled.Validate())
}
