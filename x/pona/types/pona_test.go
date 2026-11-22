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
		name              string
		idx               int
		max, high, mid    sdkmath.Int
		low, single, zero sdkmath.Int
		maxTx, highTx     uint64
		midTx, lowTx      uint64
	}{
		{"tier 1 (Y0-2)", 0, rune(50), rune(40), rune(30), rune(20), rune(1), halfRune(), 20, 15, 10, 6},
		{"tier 2 (Y2-4)", 1, rune(40), rune(30), rune(20), rune(15), rune(1), halfRune(), 20, 15, 10, 6},
		{"tier 3 (Y4+)", 2, rune(30), rune(20), rune(15), rune(10), rune(1), halfRune(), 20, 15, 10, 6},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tier := tiers[tc.idx]
			require.Equal(t, tc.max.String(), tier.MaxActivityReward.String(), "max reward")
			require.Equal(t, tc.high.String(), tier.HighActivityReward.String(), "high reward")
			require.Equal(t, tc.mid.String(), tier.MidActivityReward.String(), "mid reward")
			require.Equal(t, tc.low.String(), tier.LowActivityReward.String(), "low reward")
			require.Equal(t, tc.single.String(), tier.SingleActivityReward.String(), "single reward")
			require.Equal(t, tc.zero.String(), tier.ZeroActivityReward.String(), "zero reward")
			require.Equal(t, tc.maxTx, tier.MaxTxCountThreshold, "max threshold")
			require.Equal(t, tc.highTx, tier.HighTxCountThreshold, "high threshold")
			require.Equal(t, tc.midTx, tier.MidTxCountThreshold, "mid threshold")
			require.Equal(t, tc.lowTx, tier.LowTxCountThreshold, "low threshold")
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

// paramsWith builds a Params carrying a valid fee-only ladder, so tests that
// only care about the height tiers are not forced to spell it out. Validate now
// requires fee_only_tier to be fully specified.
func paramsWith(tiers ...types.Tier) types.Params {
	return types.Params{
		Denom:          "arune",
		Tiers:          tiers,
		FeeOnlyTier:    types.DefaultFeeOnlyTier(),
		FeeSkimPercent: types.DefaultFeeSkimPercent,
	}
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
	threshold := types.DefaultMaxTxCountThreshold // 20

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
		// Tier 1: max 50 (>=20) | high 40 (15-19) | mid 30 (10-14) | low 20 (6-9)
		//         | quint 5 | quart 4 | trio 3 | duo 2 | single 1 | zero 0.5
		{"tier1 max (tx == threshold)", inTier1, threshold, rune(50), types.BandMax},
		{"tier1 max (tx > threshold)", inTier1, 1000, rune(50), types.BandMax},
		{"tier1 high (tx == high threshold)", inTier1, 15, rune(40), types.BandHigh},
		{"tier1 high (max-1)", inTier1, threshold - 1, rune(40), types.BandHigh},
		{"tier1 mid (tx == mid threshold)", inTier1, 10, rune(30), types.BandMid},
		{"tier1 mid (high-1)", inTier1, 14, rune(30), types.BandMid},
		{"tier1 low (tx == low threshold)", inTier1, 6, rune(20), types.BandLow},
		{"tier1 low (mid-1)", inTier1, 9, rune(20), types.BandLow},
		{"tier1 quint", inTier1, 5, rune(5), types.BandQuint},
		{"tier1 quart", inTier1, 4, rune(4), types.BandQuart},
		{"tier1 trio", inTier1, 3, rune(3), types.BandTrio},
		{"tier1 duo", inTier1, 2, rune(2), types.BandDuo},
		{"tier1 single (exactly 1 tx)", inTier1, 1, rune(1), types.BandSingle},
		{"tier1 zero (no txs)", inTier1, 0, halfRune(), types.BandZero},

		// Tier 2: max 40 | high 30 | mid 20 | low 15
		{"tier2 max", inTier2, threshold, rune(40), types.BandMax},
		{"tier2 high", inTier2, 17, rune(30), types.BandHigh},
		{"tier2 mid", inTier2, 12, rune(20), types.BandMid},
		{"tier2 low", inTier2, 7, rune(15), types.BandLow},
		{"tier2 quint", inTier2, 5, rune(5), types.BandQuint},
		{"tier2 single", inTier2, 1, rune(1), types.BandSingle},
		{"tier2 zero", inTier2, 0, halfRune(), types.BandZero},

		// Tier 3: max 30 | high 20 | mid 15 | low 10
		{"tier3 max", inTier3, threshold, rune(30), types.BandMax},
		{"tier3 max (far future)", 100 * oneYear, 42, rune(30), types.BandMax},
		{"tier3 high", inTier3, 16, rune(20), types.BandHigh},
		{"tier3 mid", inTier3, 11, rune(15), types.BandMid},
		{"tier3 low", inTier3, 6, rune(10), types.BandLow},
		{"tier3 trio", inTier3, 3, rune(3), types.BandTrio},
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

// fullTier builds a complete ten-band tier. Every rate and threshold is set,
// because the module requires all of them — a partial Tier has nil math.Int
// fields and will panic when a band is evaluated. Tests must always use this
// helper (or set every field) rather than hand-rolling a Tier literal.
//
// The four activity bands (max/high/mid/low) take explicit amounts; the five
// exact-count bands are given fixed, descending values so a mis-selected band
// shows up as the wrong number.
func fullTier(mx, high, mid, low, zero sdkmath.Int) types.Tier {
	return types.Tier{
		StartHeight:          0,
		EndHeight:            0,
		MaxActivityReward:    mx,
		MaxTxCountThreshold:  types.DefaultMaxTxCountThreshold,
		HighActivityReward:   high,
		HighTxCountThreshold: types.DefaultHighTxCountThreshold,
		MidActivityReward:    mid,
		MidTxCountThreshold:  types.DefaultMidTxCountThreshold,
		LowActivityReward:    low,
		LowTxCountThreshold:  types.DefaultLowTxCountThreshold,
		QuintActivityReward:  rune(5),
		QuartActivityReward:  rune(4),
		TrioActivityReward:   rune(3),
		DuoActivityReward:    rune(2),
		SingleActivityReward: rune(1),
		ZeroActivityReward:   zero,
	}
}

// tier1 mirrors the shipped tier-1 ladder: 50 / 40 / 30 / 20 / … / 0.5.
func tier1() types.Tier {
	return fullTier(rune(50), rune(40), rune(30), rune(20), halfRune())
}

// TestBandForTxCount exercises the band selection that PoNA uses to decide
// which of a tier's rates is paid.
//
// The zero band is the important one: an idle chain must reach it, otherwise it
// pays a higher activity rate forever (see CountRegularTxs in app/abci).
func TestBandForTxCount(t *testing.T) {
	tier := tier1()

	testCases := []struct {
		name    string
		txCount uint64
		want    types.ActivityBand
	}{
		{"no transactions is the zero band", 0, types.BandZero},
		{"exactly one transaction is the single band", 1, types.BandSingle},
		{"two transactions is the duo band", 2, types.BandDuo},
		{"three transactions is the trio band", 3, types.BandTrio},
		{"four transactions is the quart band", 4, types.BandQuart},
		{"five transactions is the quint band", 5, types.BandQuint},
		{"low threshold is the low band", 6, types.BandLow},
		{"mid-1 is still the low band", 9, types.BandLow},
		{"mid threshold is the mid band", 10, types.BandMid},
		{"high-1 is still the mid band", 14, types.BandMid},
		{"high threshold is the high band", 15, types.BandHigh},
		{"max-1 is still the high band", 19, types.BandHigh},
		{"exactly the max threshold is the max band", 20, types.BandMax},
		{"above the max threshold is the max band", 1000, types.BandMax},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, tier.BandForTxCount(tc.txCount))

			// The band must agree with the amount that is actually paid.
			want := tier.ApplyFloor(tier.RewardForBand(tc.want))

			require.Equal(t, want.String(), tier.RewardForTxCount(tc.txCount).String())
		})
	}
}

// TestBandForHeightAndTxCount pins the height-aware band lookup, including the
// fee-only end state where no tier covers the height.
func TestBandForHeightAndTxCount(t *testing.T) {
	params := types.DefaultParams("arune")
	threshold := types.DefaultMaxTxCountThreshold

	inTier1 := types.BlocksPerTwoYearsAt6s - 1
	inTier2 := types.BlocksPerTwoYearsAt6s
	inTier3 := 4 * types.BlocksPerYearAt6s

	require.Equal(t, types.BandZero, params.BandForHeightAndTxCount(inTier1, 0))
	require.Equal(t, types.BandSingle, params.BandForHeightAndTxCount(inTier1, 1))
	require.Equal(t, types.BandDuo, params.BandForHeightAndTxCount(inTier1, 2))
	require.Equal(t, types.BandQuint, params.BandForHeightAndTxCount(inTier1, 5))
	require.Equal(t, types.BandMax, params.BandForHeightAndTxCount(inTier1, threshold))
	require.Equal(t, types.BandZero, params.BandForHeightAndTxCount(inTier2, 0))
	require.Equal(t, types.BandSingle, params.BandForHeightAndTxCount(inTier2, 1))
	require.Equal(t, types.BandQuint, params.BandForHeightAndTxCount(inTier3, 5))
	require.Equal(t, types.BandSingle, params.BandForHeightAndTxCount(inTier3, 1))
	require.Equal(t, types.BandMax, params.BandForHeightAndTxCount(inTier3, threshold))

	// A schedule with an explicit end height leaves later heights uncovered.
	gt := fullTier(rune(50), rune(40), rune(30), rune(20), halfRune())
	gt.StartHeight, gt.EndHeight = 0, 100
	gapped := types.Params{Denom: "arune", Tiers: []types.Tier{gt}}
	require.Equal(t, types.BandNone, gapped.BandForHeightAndTxCount(100, 5))
	require.Equal(t, types.BandMax, gapped.BandForHeightAndTxCount(99, 20))
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
// The shipped schedule never needs the floor (every rate sits above the 0.5 RUNE
// zero rate and validatePoNA rejects an inverted ladder), so this test builds a
// tier directly with a rate BELOW the floor to exercise Tier.ApplyFloor itself.
func TestZeroActivityRewardIsTheFloor(t *testing.T) {
	// Every rate below the 1 RUNE zero rate, so the floor must clamp them all.
	// All fields are set (via fullTier) — a partial Tier would panic.
	belowFloor := fullTier(
		sdkmath.NewInt(3), // max:  3 arune
		sdkmath.NewInt(3), // high: 3 arune
		sdkmath.NewInt(2), // mid:  2 arune
		sdkmath.NewInt(1), // low:  1 arune
		rune(1),           // zero: 1 RUNE — the floor
	)

	// Every band that pays below the floor is clamped up to it.
	for _, txCount := range []uint64{0, 1, 6, 9, 10, 15, 20, 25} {
		require.Equal(t, rune(1).String(), belowFloor.RewardForTxCount(txCount).String(),
			"txCount %d must be clamped to the floor", txCount)
	}

	// A rate ABOVE the floor is paid unchanged.
	aboveFloor := fullTier(rune(50), rune(40), rune(30), rune(20), halfRune())
	require.Equal(t, rune(50).String(), aboveFloor.RewardForTxCount(20).String(), "max band")
	require.Equal(t, rune(40).String(), aboveFloor.RewardForTxCount(15).String(), "high band")
	require.Equal(t, rune(30).String(), aboveFloor.RewardForTxCount(10).String(), "mid band")
	require.Equal(t, rune(20).String(), aboveFloor.RewardForTxCount(6).String(), "low band")
	require.Equal(t, rune(5).String(), aboveFloor.RewardForTxCount(5).String(), "quint band")
	require.Equal(t, rune(1).String(), aboveFloor.RewardForTxCount(1).String(), "single band")
	require.Equal(t, halfRune().String(), aboveFloor.RewardForTxCount(0).String(), "zero band")

	// RewardForBand returns the configured rate, before the floor is applied.
	require.Equal(t, "1", belowFloor.RewardForBand(types.BandLow).String(),
		"RewardForBand must return the raw rate, before the floor")

	// The default schedule never needs the floor: each configured rate sits
	// above the 0.5 RUNE zero rate, so every band pays exactly its own amount.
	for i, tier := range types.DefaultTiers() {
		for _, band := range []types.ActivityBand{
			types.BandMax, types.BandHigh, types.BandMid, types.BandLow,
			types.BandQuint, types.BandQuart, types.BandTrio, types.BandDuo, types.BandSingle,
		} {
			require.False(t, tier.RewardForBand(band).LT(tier.ZeroActivityReward),
				"default tier %d band %s must not need the floor", i, band)
		}
		require.Equal(t, tier.ZeroActivityReward.String(),
			tier.RewardForTxCount(0).String(),
			"default tier %d zero block pays the zero rate", i)
	}
}

// TestValidateRejectsInvertedRates verifies the ordering validation.
//
// Every rate and every threshold is REQUIRED — there are no optional fields and
// no fallbacks — so a schedule that omits one is rejected rather than silently
// paying a different amount at runtime.
func TestValidateRejectsInvertedRates(t *testing.T) {
	mkTier := func(mx, mid, low, single, zero sdkmath.Int) types.Tier {
		t := fullTier(mx, rune(45), mid, low, zero)
		t.SingleActivityReward = single
		return t
	}
	missing := sdkmath.Int{} // nil Int

	testCases := []struct {
		name    string
		tier    types.Tier
		wantErr string
	}{
		// Rate ordering: zero <= single <= low <= mid <= max.
		{
			name:    "single below the zero floor",
			tier:    mkTier(rune(50), rune(25), rune(20), rune(1), rune(5)),
			wantErr: "single activity reward",
		},
		{
			// Only ONE inversion, so the reported band is unambiguous: single (10)
			// exceeds duo (2). Everything else stays correctly ordered.
			name:    "single above the duo rate",
			tier:    mkTier(rune(50), rune(25), rune(20), rune(10), rune(1)),
			wantErr: "duo activity reward",
		},
		{
			name:    "mid below the low rate",
			tier:    mkTier(rune(50), rune(3), rune(20), rune(1), sdkmath.ZeroInt()),
			wantErr: "mid activity reward",
		},
		{
			// max (10) is below the tier's fixed high rate (45). The descending walk
			// reports the topmost inversion, so max is named first.
			name:    "max below the high rate",
			tier:    mkTier(rune(10), rune(25), rune(20), rune(1), sdkmath.ZeroInt()),
			wantErr: "max activity reward",
		},

		// Every rate is required: there are no optional fields.
		{
			name:    "mid omitted is rejected",
			tier:    mkTier(rune(50), missing, rune(15), rune(1), halfRune()),
			wantErr: "mid activity reward must be set",
		},
		{
			name:    "single omitted is rejected",
			tier:    mkTier(rune(50), rune(25), rune(15), missing, halfRune()),
			wantErr: "single activity reward must be set",
		},
		{
			name:    "max omitted is rejected",
			tier:    mkTier(missing, rune(25), rune(15), rune(1), halfRune()),
			wantErr: "max activity reward must be set",
		},

		// The shipped ladder is accepted.
		{
			name: "tier 1 of the shipped schedule is accepted",
			tier: mkTier(rune(50), rune(30), rune(20), rune(1), halfRune()),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := paramsWith(tc.tier).Validate()
			if tc.wantErr == "" {
				require.NoError(t, err)

				return
			}
			require.ErrorContains(t, err, tc.wantErr)
		})
	}
}

// TestValidateRejectsBadThresholds pins the threshold ordering: 2 <= low < mid < max.
func TestValidateRejectsBadThresholds(t *testing.T) {
	mkTier := func(low, mid, high, max uint64) types.Tier {
		t := fullTier(rune(50), rune(40), rune(30), rune(20), halfRune())
		t.MaxTxCountThreshold = max
		t.HighTxCountThreshold = high
		t.MidTxCountThreshold = mid
		t.LowTxCountThreshold = low
		return t
	}

	testCases := []struct {
		name    string
		tier    types.Tier
		wantErr string
	}{
		{"low below the derived minimum", mkTier(5, 10, 15, 20), "must be at least 6"},
		{"mid not above low", mkTier(6, 6, 15, 20), "greater than the low threshold"},
		{"high not above mid", mkTier(6, 12, 12, 20), "greater than the mid threshold"},
		{"max not above high", mkTier(6, 10, 20, 20), "greater than the high threshold"},
		{"valid 6/10/15/20", mkTier(6, 10, 15, 20), ""},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := paramsWith(tc.tier).Validate()
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

		single := paramsWith(tiers[i])
		require.ErrorContains(t, single.Validate(), "first tier must start at height 0",
			"tier %d starts above 0, so it fails the height rule before the open-ended rule", i)
	}

	// Tier 0 starts at height 0 but is closed, so it passes check 3 and fails
	// check 5 (the open-ended rule).
	require.Zero(t, tiers[0].StartHeight)
	require.NotZero(t, tiers[0].EndHeight, "tier 0 must be closed for this to be the failure")

	single := paramsWith(tiers[0])
	require.ErrorContains(t, single.Validate(), "last tier must be open-ended",
		"tier 0 starts at 0, so it reaches the open-ended rule and fails there")

	// Consequence: no single tier validates on its own, so a caller must validate
	// the whole slice. Asserting it directly keeps the invariant obvious.
	for i, tier := range tiers {
		require.Error(t,
			paramsWith(tier).Validate(),
			"tier %d must not validate in isolation", i)
	}
}

// TestParamsValidateRejectsBadPoNA verifies that a missing or negative rate is
// rejected. Every rate is required — there is no partially-configured tier.
func TestParamsValidateRejectsBadPoNA(t *testing.T) {
	mkTier := func(mid, low, zero sdkmath.Int) types.Tier {
		t := fullTier(rune(50), rune(40), mid, low, zero)
		return t
	}

	// Negative low activity reward.
	bad := paramsWith(mkTier(rune(25), rune(-20), rune(1)))
	require.Error(t, bad.Validate())

	// Nil zero activity reward.
	bad = paramsWith(mkTier(rune(25), rune(15), sdkmath.Int{}))
	require.Error(t, bad.Validate())

	// Nil mid activity reward: required, so rejected.
	bad = paramsWith(mkTier(sdkmath.Int{}, rune(15), rune(1)))
	require.Error(t, bad.Validate())

	// Fully specified tier is accepted.
	good := paramsWith(mkTier(rune(25), rune(15), halfRune()))
	require.NoError(t, good.Validate())
}
