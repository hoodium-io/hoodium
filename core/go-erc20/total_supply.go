package erc20

import (
	"fmt"

	bankkeeper "github.com/cosmos/cosmos-sdk/x/bank/keeper"
	"github.com/hoodium-io/hoodium/core"
	"github.com/hoodium-io/hoodium/x/evm/statedb"
)

// TotalSupplyMethodName is the name of the totalSupply method.
// It matches the name of the method in the contract ABI.
const TotalSupplyMethodName = "totalSupply"

// TotalSupplyMethod is the implementation of the totalSupply method that returns
// the total supply of the ERC20 tokens in existence.
type TotalSupplyMethod struct {
	bankKeeper bankkeeper.Keeper
	denom      string
}

func NewTotalSupplyMethod(
	bankKeeper bankkeeper.Keeper,
	denom string,
) *TotalSupplyMethod {
	return &TotalSupplyMethod{
		bankKeeper: bankKeeper,
		denom:      denom,
	}
}

func (tsm *TotalSupplyMethod) MethodName() string {
	return TotalSupplyMethodName
}

func (tsm *TotalSupplyMethod) MethodType() core.MethodType {
	return core.Read
}

func (tsm *TotalSupplyMethod) RequiredGas(_ []byte) (uint64, bool) {
	// Fallback to the default gas calculation.
	return 0, false
}

func (tsm *TotalSupplyMethod) Payable() bool {
	return false
}

func (tsm *TotalSupplyMethod) Run(
	context *core.RunContext,
	inputs core.MethodInputs,
) (core.MethodOutputs, []statedb.StateChange, error) {
	if err := core.ValidateMethodInputsCount(inputs, 0); err != nil {
		return nil, nil, err
	}

	supply := tsm.bankKeeper.GetSupply(context.SdkCtx(), tsm.denom)
	if supply.Amount.IsNil() {
		return nil, nil, fmt.Errorf("failed to get the supply amount of the %s token", tsm.denom)
	}

	return core.MethodOutputs{
		core.TypesConverter.BigInt.FromSDK(supply.Amount),
	}, nil, nil
}
