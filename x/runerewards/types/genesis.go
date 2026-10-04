package types

import (
	"fmt"
)

// GenesisState is the runerewards module genesis state.
type GenesisState struct {
	// Params is the module's initial parameters (includes the emission schedule).
	Params Params `json:"params"`
}

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
