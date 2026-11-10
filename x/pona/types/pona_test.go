package types_test

import (
	"testing"

	sdkmath "cosmossdk.io/math"
	"github.com/stretchr/testify/require"

	"github.com/hoodium-io/hoodium/x/pona/types"
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

// TestBandForTxCount exercises the band selection that PoNA uses to decide
// which of a tier's three rates is paid.
//
// The zero band is the important one: an idle chain must reach it, otherwise it
// pays the low-activity rate forever (see CountRegularTxs in app/abci).
func TestBandForTxCount(t *testing.T) {
	tier := types.Tier{
		RewardPerBlock:     rune(50),
		TxCountThreshold:   10,
		LowActivityReward:  rune(20),
		ZeroActivityReward: rune(1),
	}

	testCases := []struct {
		name    string
		txCount uint64
		want    types.ActivityBand
	}{
		{"no transactions is the zero band", 0, types.BandZero},
		{"a single transaction is the low band", 1, types.BandLow},
		{"threshold-1 is still the low band", 9, types.BandLow},
		{"exactly the threshold is the full band", 10, types.BandFull},
		{"above the threshold is the full band", 1000, types.BandFull},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, tier.BandForTxCount(tc.txCount))
			// The band must agree with the amount that is actually paid.
			require.Equal(
				t,
				tier.RewardForTxCount(tc.txCount).String(),
				wantRewardForBand(tier, tc.want).String(),
			)
		})
	}
}

// TestBandForTxCountPoNADisabled verifies that a tier with PoNA switched off
// reports the full band for every transaction count.
func TestBandForTxCountPoNADisabled(t *testing.T) {
	tier := types.Tier{RewardPerBlock: rune(50)} // TxCountThreshold 0 => PoNA off

	require.False(t, tier.PoNAEnabled())
	for _, txCount := range []uint64{0, 1, 9, 10, 1000} {
		require.Equal(t, types.BandFull, tier.BandForTxCount(txCount), "txCount %d", txCount)
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
	require.Equal(t, types.BandLow, params.BandForHeightAndTxCount(inTier1, 1))
	require.Equal(t, types.BandFull, params.BandForHeightAndTxCount(inTier1, threshold))
	require.Equal(t, types.BandZero, params.BandForHeightAndTxCount(inTier2, 0))
	require.Equal(t, types.BandLow, params.BandForHeightAndTxCount(inTier3, 5))
	require.Equal(t, types.BandFull, params.BandForHeightAndTxCount(inTier3, threshold))

	// A schedule with an explicit end height leaves later heights uncovered.
	gapped := types.Params{
		Denom: "arune",
		Tiers: []types.Tier{{StartHeight: 0, EndHeight: 100, RewardPerBlock: rune(50)}},
	}
	require.Equal(t, types.BandNone, gapped.BandForHeightAndTxCount(100, 5))
	require.Equal(t, types.BandFull, gapped.BandForHeightAndTxCount(99, 50))
}

// TestTierForHeight returns the index and the tier together, so callers can
// report "tier N of M" without re-deriving the position.
func TestTierForHeight(t *testing.T) {
	params := types.DefaultParams("arune")

	idx, tier, ok := params.TierForHeight(0)
	require.True(t, ok)
	require.Equal(t, 0, idx)
	require.Equal(t, params.Tiers[0].RewardPerBlock.String(), tier.RewardPerBlock.String())

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

// wantRewardForBand maps a band back to the tier rate it must select.
func wantRewardForBand(tier types.Tier, band types.ActivityBand) sdkmath.Int {
	switch band {
	case types.BandFull:
		return tier.RewardPerBlock
	case types.BandLow:
		return tier.LowActivityReward
	case types.BandZero:
		return tier.ZeroActivityReward
	default:
		return sdkmath.ZeroInt()
	}
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
