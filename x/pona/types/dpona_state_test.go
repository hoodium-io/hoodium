package types_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/hoodium-io/hoodium/x/pona/types"
)

// TestDPonaStartsOff pins the initial condition: a zero state is "dPona off".
func TestDPonaStartsOff(t *testing.T) {
	var s types.DPonaState
	require.False(t, s.IsActive())
	require.Equal(t, uint64(0), s.Diff)
	require.Equal(t, uint64(0), s.TxCountOffset())
	require.NoError(t, s.Validate())
}

// TestDPonaIncrementAfterFiveConsecutiveMaxBlocks walks the documented rule:
// five consecutive max-band blocks raise the difficulty by one step.
func TestDPonaIncrementAfterFiveConsecutiveMaxBlocks(t *testing.T) {
	var s types.DPonaState

	// Four max blocks: not enough.
	for i := 1; i < int(types.DPonaIncrementAfter); i++ {
		s = s.ObserveBlock(true)
		require.Equalf(t, uint64(0), s.Diff, "after %d max blocks diff must still be 0", i)
	}

	// The fifth raises it.
	s = s.ObserveBlock(true)
	require.Equal(t, uint64(1), s.Diff, "5 consecutive max blocks must set diff=1")
	require.True(t, s.IsActive())
	require.Equal(t, types.DPonaTxCountStep, s.TxCountOffset())
	require.Equal(t, uint64(0), s.ConsecutiveMax, "counter resets on increment")
}

// TestDPonaDecrementAfterHundredBlocksWithoutMax walks the decay rule.
func TestDPonaDecrementAfterHundredBlocksWithoutMax(t *testing.T) {
	s := types.DPonaState{Diff: 1}

	// 99 blocks without max: difficulty holds.
	for i := 1; i < int(types.DPonaDecrementAfter); i++ {
		s = s.ObserveBlock(false)
	}
	require.Equal(t, uint64(1), s.Diff, "diff must hold until the 100th block")

	// The 100th decrements.
	s = s.ObserveBlock(false)
	require.Equal(t, uint64(0), s.Diff, "100 blocks without max must drop diff to 0")
	require.False(t, s.IsActive())
}

// TestDPonaNeverGoesNegative floors the decrement at zero.
func TestDPonaNeverGoesNegative(t *testing.T) {
	s := types.DPonaState{Diff: 0, BlocksSinceMax: types.DPonaDecrementAfter - 1}

	s = s.ObserveBlock(false)
	require.Equal(t, uint64(0), s.Diff, "diff must never go below zero")
}

// TestDPonaHoldsOnMixedActivity covers the "stays stable" rule: a max block that
// breaks neither a run of 5 nor a drought of 100 changes nothing.
func TestDPonaHoldsOnMixedActivity(t *testing.T) {
	s := types.DPonaState{Diff: 2, ConsecutiveMax: 2, BlocksSinceMax: 0}

	// A non-max block resets the run but is far from the decay threshold.
	s = s.ObserveBlock(false)
	require.Equal(t, uint64(2), s.Diff, "diff must hold during mixed activity")
	require.Equal(t, uint64(0), s.ConsecutiveMax)

	// A max block resets the drought but is far from the increment threshold.
	s = s.ObserveBlock(true)
	require.Equal(t, uint64(2), s.Diff, "diff must hold during mixed activity")
	require.Equal(t, uint64(1), s.ConsecutiveMax)
}

// TestDPonaRepeatedIncrements walks two full increments to prove the offset
// accumulates by one step each time.
func TestDPonaRepeatedIncrements(t *testing.T) {
	var s types.DPonaState

	for round := 1; round <= 2; round++ {
		for i := uint64(0); i < types.DPonaIncrementAfter; i++ {
			s = s.ObserveBlock(true)
		}
		require.Equalf(t, uint64(round), s.Diff, "round %d", round)
		require.Equalf(t, round*types.DPonaTxCountStep, s.TxCountOffset(), "round %d offset", round)
	}
}

// TestDPonaIncrementThenDecayRoundTrip proves the controller returns to the base
// state after an increment followed by sufficient quiet blocks.
func TestDPonaIncrementThenDecayRoundTrip(t *testing.T) {
	var s types.DPonaState

	for i := uint64(0); i < types.DPonaIncrementAfter; i++ {
		s = s.ObserveBlock(true)
	}
	require.Equal(t, uint64(1), s.Diff)

	for i := uint64(0); i < types.DPonaDecrementAfter; i++ {
		s = s.ObserveBlock(false)
	}

	require.Equal(t, uint64(0), s.Diff, "a quiet chain must return to dPona off")
	require.Equal(t, uint64(0), s.TxCountOffset())
	require.NoError(t, s.Validate())
}

// TestDPonaCountersSaturate proves the counters do not overflow on a long chain,
// which is what keeps the persisted state small.
func TestDPonaCountersSaturate(t *testing.T) {
	s := types.DPonaState{Diff: 1}

	// 10,000 quiet blocks: BlocksSinceMax must never exceed the trigger.
	for i := 0; i < 10_000; i++ {
		s = s.ObserveBlock(false)
	}
	require.LessOrEqual(t, s.BlocksSinceMax, types.DPonaDecrementAfter)

	// 10,000 max blocks: ConsecutiveMax must never exceed the trigger.
	for i := 0; i < 10_000; i++ {
		s = s.ObserveBlock(true)
	}
	require.Less(t, s.ConsecutiveMax, types.DPonaIncrementAfter)
}

// TestDPonaValidateRejectsImpossibleState guards the persisted-state invariants.
func TestDPonaValidateRejectsImpossibleState(t *testing.T) {
	require.Error(t, types.DPonaState{ConsecutiveMax: types.DPonaIncrementAfter}.Validate(),
		"consecutive_max at the trigger must be rejected (it is reset on increment)")
	require.Error(t, types.DPonaState{BlocksSinceMax: types.DPonaDecrementAfter + 1}.Validate(),
		"blocks_since_max past the trigger must be rejected (it is reset on decrement)")
	require.NoError(t, types.DPonaState{BlocksSinceMax: types.DPonaDecrementAfter}.Validate())
}
