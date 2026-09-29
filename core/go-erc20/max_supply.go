package erc20

import (
	sdkmath "cosmossdk.io/math"

	"github.com/hoodium-io/hoodium/core"
	"github.com/hoodium-io/hoodium/x/evm/statedb"
)

// MaxSupplyMethodName is the name of the maxSupply method. It matches the name
// of the method in the contract ABI.
const MaxSupplyMethodName = "maxSupply"

// MaxSupplyMethod is the implementation of the maxSupply method that returns
// the hard cap on the total supply of the ERC20 token.
//
// Unlike totalSupply, maxSupply is a fixed constant configured at construction
// time (it does not depend on chain state). The token cannot be minted past
// this amount.
type MaxSupplyMethod struct {
	maxSupply sdkmath.Int
}

func NewMaxSupplyMethod(maxSupply sdkmath.Int) *MaxSupplyMethod {
	return &MaxSupplyMethod{
		maxSupply: maxSupply,
	}
}

func (msm *MaxSupplyMethod) MethodName() string {
	return MaxSupplyMethodName
}

func (msm *MaxSupplyMethod) MethodType() core.MethodType {
	return core.Read
}

func (msm *MaxSupplyMethod) RequiredGas(_ []byte) (uint64, bool) {
	// Fallback to the default gas calculation.
	return 0, false
}

func (msm *MaxSupplyMethod) Payable() bool {
	return false
}

func (msm *MaxSupplyMethod) Run(
	_ *core.RunContext,
	inputs core.MethodInputs,
) (core.MethodOutputs, []statedb.StateChange, error) {
	if err := core.ValidateMethodInputsCount(inputs, 0); err != nil {
		return nil, nil, err
	}

	return core.MethodOutputs{
		core.TypesConverter.BigInt.FromSDK(msm.maxSupply),
	}, nil, nil
}
