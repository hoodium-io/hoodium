package erc20

import (
	"github.com/hoodium-io/hoodium/core"
	"github.com/hoodium-io/hoodium/x/evm/statedb"
)

const (
	NameMethodName     = "name"
	SymbolMethodName   = "symbol"
	DecimalsMethodName = "decimals"
)

type (
	NameMethod struct {
		name string
	}
	SymbolMethod struct {
		symbol string
	}
	DecimalsMethod struct {
		decimals uint8
	}
)

// Name method returns the name of the token.
func NewNameMethod(name string) *NameMethod {
	return &NameMethod{name: name}
}

func (nm *NameMethod) MethodName() string {
	return NameMethodName
}

func (nm *NameMethod) MethodType() core.MethodType {
	return core.Read
}

func (nm *NameMethod) RequiredGas(_ []byte) (uint64, bool) {
	// Fallback to the default gas calculation.
	return 0, false
}

func (nm *NameMethod) Payable() bool {
	return false
}

func (nm *NameMethod) Run(
	_ *core.RunContext,
	inputs core.MethodInputs,
) (core.MethodOutputs, []statedb.StateChange, error) {
	if err := core.ValidateMethodInputsCount(inputs, 0); err != nil {
		return nil, nil, err
	}

	// Return stored name
	return core.MethodOutputs{
		nm.name,
	}, nil, nil
}

// Symbol method returns the symbol of the token.
func NewSymbolMethod(symbol string) *SymbolMethod {
	return &SymbolMethod{symbol: symbol}
}

func (sm *SymbolMethod) MethodName() string {
	return SymbolMethodName
}

func (sm *SymbolMethod) MethodType() core.MethodType {
	return core.Read
}

func (sm *SymbolMethod) RequiredGas(_ []byte) (uint64, bool) {
	// Fallback to the default gas calculation.
	return 0, false
}

func (sm *SymbolMethod) Payable() bool {
	return false
}

func (sm *SymbolMethod) Run(
	_ *core.RunContext,
	inputs core.MethodInputs,
) (core.MethodOutputs, []statedb.StateChange, error) {
	if err := core.ValidateMethodInputsCount(inputs, 0); err != nil {
		return nil, nil, err
	}

	// Return stored symbol
	return core.MethodOutputs{
		sm.symbol,
	}, nil, nil
}

// Decimals method returns the number of decimals used to represent the token.
func NewDecimalsMethod(decimals uint8) *DecimalsMethod {
	return &DecimalsMethod{decimals: decimals}
}

func (dm *DecimalsMethod) MethodName() string {
	return DecimalsMethodName
}

func (dm *DecimalsMethod) MethodType() core.MethodType {
	return core.Read
}

func (dm *DecimalsMethod) RequiredGas(_ []byte) (uint64, bool) {
	// Fallback to the default gas calculation.
	return 0, false
}

func (dm *DecimalsMethod) Payable() bool {
	return false
}

func (dm *DecimalsMethod) Run(
	_ *core.RunContext,
	inputs core.MethodInputs,
) (core.MethodOutputs, []statedb.StateChange, error) {
	if err := core.ValidateMethodInputsCount(inputs, 0); err != nil {
		return nil, nil, err
	}

	// Return stored decimals
	return core.MethodOutputs{
		dm.decimals,
	}, nil, nil
}
