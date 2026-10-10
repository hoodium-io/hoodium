package types_test

import (
	"testing"

	sdkmath "cosmossdk.io/math"
	"github.com/stretchr/testify/require"

	"github.com/hoodium-io/hoodium/x/runerewards/types"
)

// rune returns n RUNE expressed in arune.
func rune(n int64) sdkmath.Int {
	return sdkmath.NewIntWithDecimal(n, 18)
}

func TestDefaultParamsValidate(t *testing.T) {
	params := types.DefaultParams("arune")
	require.NoError(t, params.Validate())
	require.Equal(t, "arune", params.Denom)
	require.Len(t, params.Tiers, 3)
}

func TestDefaultTiersPoNA(t *testing.T) {
	tiers := types.DefaultTiers()
	require.Len(t, tiers, 3)

	testCases := []struct {
		name      string
		idx       int
		full      sdkmath.Int
		low       sdkmath.Int
		zero      sdkmath.Int
		threshold uint64
	}{
		{"tier 1 (Y0-2)", 0, rune(50), rune(20), rune(1), 10},
		{"tier 2 (Y2-4)", 1, rune(25), rune(10), rune(1), 10},
		{"tier 3 (Y4+)", 2, rune(10), rune(5), rune(1), 10},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tier := tiers[tc.idx]
			require.Equal(t, tc.full.String(), tier.RewardPerBlock.String(), "full reward")
			require.Equal(t, tc.low.String(), tier.LowActivityReward.String(), "low activity reward")
			require.Equal(t, tc.zero.String(), tier.ZeroActivityReward.String(), "zero activity reward")
			require.Equal(t, tc.threshold, tier.TxCountThreshold, "tx count threshold")
			require.True(t, tier.PoNAEnabled())
		})
	}
}

func TestRewardForHeight(t *testing.T) {
	params := types.DefaultParams("arune")

	oneYear := types.BlocksPerYearAt6s      // 5,259,600
	twoYears := types.BlocksPerTwoYearsAt6s // 10,519,200

	// RewardForHeight is the tx-count-agnostic (full) tier reward.
	testCases := []struct {
		name   string
		height uint64
		want   sdkmath.Int
	}{
		{"genesis", 0, rune(50)},
		{"last block of tier 1", twoYears - 1, rune(50)},
		{"first block of tier 2", twoYears, rune(25)},
		{"last block of tier 2", 4*oneYear - 1, rune(25)},
		{"first block of tier 3", 4 * oneYear, rune(10)},
		{"far in tier 3 (open-ended)", 100 * oneYear, rune(10)},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := params.RewardForHeight(tc.height)
			require.Equal(t, tc.want.String(), got.String(),
				"height %d: want %s got %s", tc.height, tc.want, got)
		})
	}
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
		name    string
		height  uint64
		txCount uint64
		want    sdkmath.Int
	}{
		// Tier 1: 50 / 20 / 1
		{"tier1 full (tx == threshold)", inTier1, threshold, rune(50)},
		{"tier1 full (tx > threshold)", inTier1, 1000, rune(50)},
		{"tier1 low (1 tx)", inTier1, 1, rune(20)},
		{"tier1 low (threshold-1)", inTier1, threshold - 1, rune(20)},
		{"tier1 zero (no txs)", inTier1, 0, rune(1)},

		// Tier 2: 25 / 10 / 1
		{"tier2 full", inTier2, threshold, rune(25)},
		{"tier2 low", inTier2, 3, rune(10)},
		{"tier2 zero", inTier2, 0, rune(1)},

		// Tier 3: 10 / 5 / 1
		{"tier3 full", inTier3, threshold, rune(10)},
		{"tier3 full (far future)", 100 * oneYear, 42, rune(10)},
		{"tier3 low", inTier3, 6, rune(5)},
		{"tier3 zero", inTier3, 0, rune(1)},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := params.RewardForHeightAndTxCount(tc.height, tc.txCount)
			require.Equal(t, tc.want.String(), got.String(),
				"height %d txCount %d: want %s got %s", tc.height, tc.txCount, tc.want, got)
		})
	}
}

// TestPoNADisabledWhenThresholdZero verifies that a tier with a zero threshold
// always pays its full reward, regardless of block activity.
func TestPoNADisabledWhenThresholdZero(t *testing.T) {
	tier := types.Tier{
		RewardPerBlock: rune(50),
		// TxCountThreshold intentionally 0 => PoNA off.
	}
	require.False(t, tier.PoNAEnabled())
	require.Equal(t, rune(50).String(), tier.RewardForTxCount(0).String())
	require.Equal(t, rune(50).String(), tier.RewardForTxCount(1).String())
	require.Equal(t, rune(50).String(), tier.RewardForTxCount(9999).String())
}

// TestRewardForHeightAndTxCountNoTier verifies that a height with no covering
// tier yields zero (fee-only, emission ended).
func TestRewardForHeightAndTxCountNoTier(t *testing.T) {
	params := types.Params{
		Denom: "arune",
		Tiers: []types.Tier{
			{StartHeight: 0, EndHeight: 100, RewardPerBlock: rune(50)},
		},
	}
	require.NoError(t, params.Validate())
	require.True(t, params.RewardForHeightAndTxCount(150, 20).IsZero())
}

func TestParamsValidateRejectsBadSchedules(t *testing.T) {
	base := types.DefaultParams("arune")

	// Empty denom.
	bad := base
	bad.Denom = ""
	require.Error(t, bad.Validate())

	// First tier not starting at 0.
	bad = base
	bad.Tiers = []types.Tier{{StartHeight: 1, EndHeight: 0, RewardPerBlock: rune(5)}}
	require.Error(t, bad.Validate())

	// Non-contiguous tiers.
	bad = base
	bad.Tiers = []types.Tier{
		{StartHeight: 0, EndHeight: 100, RewardPerBlock: rune(50)},
		{StartHeight: 101, EndHeight: 0, RewardPerBlock: rune(25)},
	}
	require.Error(t, bad.Validate())

	// Last tier not open-ended.
	bad = base
	bad.Tiers = []types.Tier{{StartHeight: 0, EndHeight: 100, RewardPerBlock: rune(50)}}
	require.Error(t, bad.Validate())
}

// TestTierBoundariesMatchYearMath pins the tier boundaries to the documented
// 2-year / 2-year / infinite schedule at a 6s block time.
func TestTierBoundariesMatchYearMath(t *testing.T) {
	require.Equal(t, uint64(5_259_600), types.BlocksPerYearAt6s)
	require.Equal(t, uint64(10_519_200), types.BlocksPerTwoYearsAt6s)
}

// TestParamsValidateRejectsBadPoNA verifies that an enabled tier with a missing
// or negative PoNA rate is rejected, while a disabled (threshold 0) tier is not.
func TestParamsValidateRejectsBadPoNA(t *testing.T) {
	mkTier := func(low, zero sdkmath.Int) types.Tier {
		return types.Tier{
			StartHeight:        0,
			EndHeight:          0,
			RewardPerBlock:     rune(50),
			TxCountThreshold:   10,
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
		Tiers: []types.Tier{{StartHeight: 0, EndHeight: 0, RewardPerBlock: rune(50)}},
	}
	require.NoError(t, disabled.Validate())
}
