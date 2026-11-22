package types_test

import (
	"testing"

	sdkmath "cosmossdk.io/math"
	"github.com/stretchr/testify/require"

	"github.com/hoodium-io/hoodium/x/pona/types"
)

// tenBandTier builds a tier with the shipped base thresholds and distinguishable
// reward amounts, so a mis-selected band shows up as the wrong number rather than
// coincidentally matching a neighbour.
func tenBandTier() types.Tier {
	return types.Tier{
		StartHeight:          0,
		EndHeight:            10_519_200,
		MaxActivityReward:    sdkmath.NewInt(50),
		MaxTxCountThreshold:  20,
		HighActivityReward:   sdkmath.NewInt(40),
		HighTxCountThreshold: 15,
		MidActivityReward:    sdkmath.NewInt(30),
		MidTxCountThreshold:  10,
		LowActivityReward:    sdkmath.NewInt(20),
		LowTxCountThreshold:  6,
		QuintActivityReward:  sdkmath.NewInt(5),
		QuartActivityReward:  sdkmath.NewInt(4),
		TrioActivityReward:   sdkmath.NewInt(3),
		DuoActivityReward:    sdkmath.NewInt(2),
		SingleActivityReward: sdkmath.NewInt(1),
		// NOTE: 0.5 RUNE is 5e17 arune, which is far above the 1-arune single rate
		// used by the *contract* tier above. This helper works in arune and the
		// ApplyFloor would clamp single up to zero, so the floor is set BELOW every
		// band to keep the raw rates observable.
		ZeroActivityReward: sdkmath.NewInt(0),
	}
}

// TestBandLadderBaseTableAtDiffZero pins the exact shipped base ladder, so a
// change to the derivation cannot silently reprice the chain.
func TestBandLadderBaseTableAtDiffZero(t *testing.T) {
	tier := tenBandTier()

	table := []struct {
		count uint64
		band  types.ActivityBand
		pay   int64
	}{
		{0, types.BandZero, 0}, // 0.5 RUNE is asserted below
		{1, types.BandSingle, 1},
		{2, types.BandDuo, 2},
		{3, types.BandTrio, 3},
		{4, types.BandQuart, 4},
		{5, types.BandQuint, 5},
		{6, types.BandLow, 20},
		{9, types.BandLow, 20},
		{10, types.BandMid, 30},
		{14, types.BandMid, 30},
		{15, types.BandHigh, 40},
		{19, types.BandHigh, 40},
		{20, types.BandMax, 50},
		{60, types.BandMax, 50},
	}

	for _, tc := range table {
		require.Equalf(t, tc.band, tier.BandForTxCount(tc.count), "txCount=%d band", tc.count)
		if tc.count == 0 {
			continue
		}
		require.Equalf(t, sdkmath.NewInt(tc.pay), tier.RewardForTxCount(tc.count),
			"txCount=%d reward", tc.count)
	}

	require.Equal(t, sdkmath.ZeroInt(), tier.RewardForTxCount(0),
		"this helper sets the floor to 0 so raw band rates stay observable")
}

// TestBandLadderExhaustive walks every transaction count from 0 to 60 at four
// difficulties and asserts the band is exactly the intended one.
//
// This is the test that would have caught two real bugs in the original design:
//
//   - `txCount <= quint` for each exact band, where the first `<=` swallows every
//     lower count and makes quart/trio/duo/single unreachable;
//   - `txCount == quint` with no clamped floor, which leaves a range of counts
//     matching no band at all once the difficulty offset shifts the thresholds.
func TestBandLadderExhaustive(t *testing.T) {
	const maxCount = 60

	expected := func(count, offset uint64) types.ActivityBand {
		low := 6 + offset
		maxT := 20 + offset
		highT := 15 + offset
		midT := 10 + offset

		switch {
		case count == 0:
			return types.BandZero
		case count <= low-5:
			return types.BandSingle
		case count == low-4:
			return types.BandDuo
		case count == low-3:
			return types.BandTrio
		case count == low-2:
			return types.BandQuart
		case count == low-1:
			return types.BandQuint
		case count < midT:
			return types.BandLow
		case count < highT:
			return types.BandMid
		case count < maxT:
			return types.BandHigh
		default:
			return types.BandMax
		}
	}

	for _, diff := range []uint64{0, 1, 2, 3} {
		offset := diff * types.DPonaTxCountStep
		tier := tenBandTier().WithDPonaOffset(offset)

		for count := uint64(0); count <= maxCount; count++ {
			require.Equalf(t, expected(count, offset), tier.BandForTxCount(count),
				"diff=%d txCount=%d", diff, count)
		}
	}
}

// TestBandLadderNoGapsOrOverlaps asserts the Ladder() ranges tile the whole count
// space: every count from 0 upward belongs to exactly one rung.
//
// The ladder is ordered MAX -> ZERO (busiest first), so contiguity is checked by
// walking it and verifying each rung ends exactly one below the previous one, and
// that the last rung is the zero band at count 0.
func TestBandLadderNoGapsOrOverlaps(t *testing.T) {
	for _, diff := range []uint64{0, 1, 2, 5} {
		tier := tenBandTier().WithDPonaOffset(diff * types.DPonaTxCountStep)
		ladder := tier.Ladder()

		require.Len(t, ladder, 10, "ladder must have exactly ten bands")
		require.Equal(t, types.BandMax, ladder[0].Band, "ladder must start at max")
		require.Equal(t, types.BandZero, ladder[len(ladder)-1].Band, "ladder must end at zero")
		require.Equal(t, uint64(0), ladder[len(ladder)-1].Min, "the last rung must be count 0")

		// Walking MAX -> ZERO, each rung's Max must be exactly one below the
		// previous rung's Min: no gap, no overlap.
		for i := 1; i < len(ladder); i++ {
			require.Equalf(t, ladder[i-1].Min-1, ladder[i].Max,
				"diff=%d: gap or overlap between %q and %q",
				diff, ladder[i-1].Band, ladder[i].Band)
		}

		for _, rung := range ladder {
			require.Equalf(t, rung.Band, tier.BandForTxCount(rung.Min),
				"diff=%d: rung %q Min=%d classifies differently", diff, rung.Band, rung.Min)
		}
	}
}

// TestBandLadderOffsetShiftsThresholds proves the dPona offset moves every band
// by exactly the offset, and that the reward AMOUNTS are untouched.
func TestBandLadderOffsetShiftsThresholds(t *testing.T) {
	base := tenBandTier()
	shifted := base.WithDPonaOffset(10)

	// With offset +10 the thresholds become low=16, mid=20, high=25, max=30.
	// So 20 is exactly the MID threshold and 30 is exactly MAX.
	require.Equal(t, types.BandMid, shifted.BandForTxCount(20))
	require.Equal(t, types.BandHigh, shifted.BandForTxCount(25))
	require.Equal(t, types.BandHigh, shifted.BandForTxCount(29))
	require.Equal(t, types.BandMax, shifted.BandForTxCount(30))

	// quint is now 15, quart 14.
	require.Equal(t, types.BandQuint, shifted.BandForTxCount(15))
	require.Equal(t, types.BandQuart, shifted.BandForTxCount(14))

	// Counts below the shifted duo (16-4=12) still clamp into single: no gap.
	for _, c := range []uint64{1, 5, 9, 10, 11} {
		require.Equalf(t, types.BandSingle, shifted.BandForTxCount(c),
			"txCount=%d must clamp into single at offset +10", c)
	}

	// Amounts are identical to the base tier: difficulty never reprices.
	require.Equal(t, base.MaxActivityReward, shifted.MaxActivityReward)
	require.Equal(t, base.SingleActivityReward, shifted.SingleActivityReward)
}

// TestBandLadderZeroOffsetIsNoOp guards the dPona-off case.
func TestBandLadderZeroOffsetIsNoOp(t *testing.T) {
	base := tenBandTier()
	require.Equal(t, base, base.WithDPonaOffset(0))
}
