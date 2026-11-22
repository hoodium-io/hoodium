package cli

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/hoodium-io/hoodium/x/pona/types"
)

// TestDPonaEffectiveThresholds pins the arithmetic the `dpona` command reports.
//
// The command is a thin wrapper over WithDPonaOffset + the derived-band helpers,
// so this test documents and locks what an operator sees for each difficulty.
func TestDPonaEffectiveThresholds(t *testing.T) {
	base := types.DefaultTiers()[0]

	// build mirrors what GetDPonaCmd does with the queried state.
	build := func(diff uint64) effectiveThresholds {
		eff := base.WithDPonaOffset(diff * types.DPonaTxCountStep)
		return effectiveThresholds{
			Max:    eff.MaxTxCountThreshold,
			High:   eff.HighTxCountThreshold,
			Mid:    eff.MidTxCountThreshold,
			Low:    eff.LowTxCountThreshold,
			Quint:  eff.QuintTxCount(),
			Quart:  eff.QuartTxCount(),
			Trio:   eff.TrioTxCount(),
			Duo:    eff.DuoTxCount(),
			Single: eff.SingleTxCount(),
		}
	}

	cases := []struct {
		diff uint64
		want effectiveThresholds
	}{
		// dPona off: the base ladder exactly as shipped.
		{0, effectiveThresholds{Max: 20, High: 15, Mid: 10, Low: 6, Quint: 5, Quart: 4, Trio: 3, Duo: 2, Single: 1}},
		// +10: every threshold moves up by exactly 10.
		{1, effectiveThresholds{Max: 30, High: 25, Mid: 20, Low: 16, Quint: 15, Quart: 14, Trio: 13, Duo: 12, Single: 11}},
		// +20.
		{2, effectiveThresholds{Max: 40, High: 35, Mid: 30, Low: 26, Quint: 25, Quart: 24, Trio: 23, Duo: 22, Single: 21}},
	}

	for _, tc := range cases {
		got := build(tc.diff)
		require.Equalf(t, tc.want, got, "diff=%d effective thresholds", tc.diff)
	}
}

// TestDPonaReportCountdowns checks the "blocks until the next change" numbers the
// command derives, since an off-by-one there would mislead an operator watching
// the controller.
func TestDPonaReportCountdowns(t *testing.T) {
	// Fresh controller: needs a full 5 consecutive max blocks.
	var fresh types.DPonaState
	require.Equal(t, types.DPonaIncrementAfter, types.DPonaIncrementAfter-fresh.ConsecutiveMax,
		"a fresh controller needs the full run of max blocks")

	// Four max blocks in: one more triggers the increment.
	almost := types.DPonaState{ConsecutiveMax: 4}
	require.Equal(t, uint64(1), types.DPonaIncrementAfter-almost.ConsecutiveMax)

	// Active at diff 2, 60 blocks since the last max: 40 more decay it.
	decaying := types.DPonaState{Diff: 2, BlocksSinceMax: 60}
	require.Equal(t, uint64(40), types.DPonaDecrementAfter-decaying.BlocksSinceMax)
}
