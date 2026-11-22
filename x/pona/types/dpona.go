package types

import "fmt"

// Dynamic PoNA (dPona) — the difficulty controller.
//
// dPona raises or lowers every band's transaction threshold in response to
// sustained max-band activity. The shift is expressed in whole steps of
// DPonaTxCountStep transactions (10), and applied to the stored thresholds
// (low / mid / high / max) by Tier.WithDPonaOffset. The exact-count bands
// (quint..single) follow automatically because they are derived from `low`.
//
// # State
//
//	consecutive_max   blocks in a row that landed in the max band (saturating)
//	blocks_since_max  blocks since the last max-band block (saturating)
//	diff              difficulty in +10-tx steps; 0 means dPona is OFF
//
// There is deliberately no separate enabled flag: diff == 0 *is* "off", so the
// flag and the number can never disagree.
const (
	// DPonaTxCountStep is the number of transactions one difficulty step adds to
	// (or removes from) every band threshold.
	DPonaTxCountStep = uint64(10)

	// DPonaIncrementAfter is the number of consecutive max-band blocks that
	// raises the difficulty by one step.
	DPonaIncrementAfter = uint64(5)

	// DPonaDecrementAfter is the number of consecutive blocks without a max-band
	// block that lowers the difficulty by one step.
	DPonaDecrementAfter = uint64(100)

	// DPonaWarnDiff is the difficulty at which an operator warning is logged.
	//
	// It is NOT a cap. The controller is a negative-feedback loop: difficulty can
	// only rise while blocks are actually at max, and raising the threshold is
	// itself what makes max harder and starts the decay timer. In full flight the
	// decay side runs 20x faster per step (5 blocks up vs 100 blocks down), so a
	// runaway is self-correcting. The warning exists only so an operator can see
	// that the chain has been hot.
	DPonaWarnDiff = uint64(5)
)

// IsActive reports whether dPona is currently applying a difficulty offset.
func (s DPonaState) IsActive() bool { return s.Diff > 0 }

// TxCountOffset returns the number of transactions added to every band
// threshold at the current difficulty.
func (s DPonaState) TxCountOffset() uint64 { return s.Diff * DPonaTxCountStep }

// ObserveBlock advances the controller by one block.
//
// inMaxBand must report whether the block just finalised was classified into the
// max band. The returned state is what should be persisted; the receiver is not
// modified.
//
// Transitions, evaluated in this order:
//
//	in max band                              -> consecutive_max++, blocks_since_max = 0
//	                                             if consecutive_max == 5 -> diff++, consecutive_max = 0
//	otherwise                                -> consecutive_max = 0, blocks_since_max++
//	                                             if blocks_since_max == 100 -> diff-- (floor 0), blocks_since_max = 0
//
// The two branches are mutually exclusive, so an increment and a decrement can
// never happen in the same block. Both counters saturate at their trigger, which
// keeps the stored state small and avoids uint64 overflow on a long-lived chain.
func (s DPonaState) ObserveBlock(inMaxBand bool) DPonaState {
	if inMaxBand {
		s.BlocksSinceMax = 0
		if s.ConsecutiveMax < DPonaIncrementAfter {
			s.ConsecutiveMax++
		}
		if s.ConsecutiveMax == DPonaIncrementAfter {
			s.Diff++
			s.ConsecutiveMax = 0
		}
		return s
	}

	s.ConsecutiveMax = 0
	if s.BlocksSinceMax < DPonaDecrementAfter {
		s.BlocksSinceMax++
	}
	if s.BlocksSinceMax == DPonaDecrementAfter {
		// Floor at zero: diff never goes negative, and reaching zero is what
		// turns dPona off.
		if s.Diff > 0 {
			s.Diff--
		}
		s.BlocksSinceMax = 0
	}

	return s
}

// Validate checks the controller state is self-consistent.
func (s DPonaState) Validate() error {
	if s.ConsecutiveMax >= DPonaIncrementAfter {
		return fmt.Errorf(
			"dpona: consecutive_max %d must be below the increment threshold %d (it is reset on increment)",
			s.ConsecutiveMax, DPonaIncrementAfter,
		)
	}
	if s.BlocksSinceMax > DPonaDecrementAfter {
		return fmt.Errorf(
			"dpona: blocks_since_max %d must not exceed the decrement threshold %d (it is reset on decrement)",
			s.BlocksSinceMax, DPonaDecrementAfter,
		)
	}
	return nil
}
