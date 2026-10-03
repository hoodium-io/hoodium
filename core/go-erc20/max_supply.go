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
// time (it does not depend on chain state).
//
// ENFORCEMENT: this method only *reports* the cap. The cap is actually enforced
// at the bank-keeper level via a minting restriction installed in app.go
// (see app/mint_cap.go), which rejects any mint that would push the denom's
// total supply above the value returned here. The two use the same constant
// (runetoken.MaxSupply / hooditoken.MaxSupply) so the reported cap and the
// enforced cap cannot diverge.
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
