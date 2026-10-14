package types_test

import (
	"testing"

	sdkmath "cosmossdk.io/math"
	"github.com/stretchr/testify/require"

	"github.com/hoodium-io/hoodium/x/runerewards/types"
)

// TestDefaultTiersPoNA pins the full/low/zero rates of the default schedule.
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
//
// NOTE: a *valid* schedule always covers every height (it starts at 0 and its
// last tier is open-ended), so this defensive branch is only reachable with an
// explicit end height on the final tier — which Validate() rejects. The schedule
// is therefore left unvalidated on purpose, and the zero result must still hold.
func TestRewardForHeightAndTxCountNoTier(t *testing.T) {
	params := types.Params{
		Denom: "arune",
		Tiers: []types.Tier{
			{StartHeight: 0, EndHeight: 100, RewardPerBlock: rune(50)},
		},
	}
	// Deliberately NOT calling params.Validate(): the point is that a height
	// past the last tier is fee-only rather than panicking or paying.
	require.True(t, params.RewardForHeightAndTxCount(150, 20).IsZero())
	require.True(t, params.RewardForHeight(150).IsZero())
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
