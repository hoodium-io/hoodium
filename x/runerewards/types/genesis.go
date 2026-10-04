package types

import (
	"fmt"
)

// GenesisState is defined in genesis.pb.go (generated from
// proto/rune/runerewards/v1/genesis.proto).

// DefaultGenesisState returns the default genesis state.
func DefaultGenesisState() *GenesisState {
	return &GenesisState{
		Params: DefaultParams("arune"),
	}
}

// Validate performs basic validation of the genesis state.
func (gs GenesisState) Validate() error {
	if err := gs.Params.Validate(); err != nil {
		return fmt.Errorf("invalid runerewards params: %w", err)
	}
	return nil
}
