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

// TestTierBoundariesMatchYearMath pins the tier boundaries to the documented
// 2-year / 2-year / infinite schedule at a 6s block time.
func TestTierBoundariesMatchYearMath(t *testing.T) {
	require.Equal(t, uint64(5_259_600), types.BlocksPerYearAt6s)
	require.Equal(t, uint64(10_519_200), types.BlocksPerTwoYearsAt6s)
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
