package erc20

import (
	"bytes"
	"fmt"

	sdkmath "cosmossdk.io/math"
	authzkeeper "github.com/cosmos/cosmos-sdk/x/authz/keeper"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/hoodium-io/hoodium/core"
	"github.com/hoodium-io/hoodium/x/evm/statedb"
)

// AllowanceMethodName is the name of the Allowance method that should match the
// name in the contract ABI.
const (
	AllowanceMethodName = "allowance"
)

// AllowanceMethod is a precompile method that allows users to check
// how much a spender is allowed to spend on behalf of an owner.
type AllowanceMethod struct {
	authzkeeper authzkeeper.Keeper
	denom       string
}

func NewAllowanceMethod(
	authzkeeper authzkeeper.Keeper,
	denom string,
) *AllowanceMethod {
	return &AllowanceMethod{
		authzkeeper: authzkeeper,
		denom:       denom,
	}
}

func (am *AllowanceMethod) MethodName() string {
	return AllowanceMethodName
}

func (am *AllowanceMethod) MethodType() core.MethodType {
	return core.Read
}

func (am *AllowanceMethod) RequiredGas(_ []byte) (uint64, bool) {
	// Fallback to the default gas calculation.
	return 0, false
}

func (am *AllowanceMethod) Payable() bool {
	return false
}

// Run returns the allowance of the spender for the owner.
func (am *AllowanceMethod) Run(
	context *core.RunContext,
	inputs core.MethodInputs,
) (core.MethodOutputs, []statedb.StateChange, error) {
	if err := core.ValidateMethodInputsCount(inputs, 2); err != nil {
		return nil, nil, err
	}

	owner, ok := inputs[0].(common.Address)
	if !ok {
		return nil, nil, fmt.Errorf("invalid owner address: %v", inputs[0])
	}

	if isZeroAddress(owner) {
		return nil, nil, fmt.Errorf("owner address cannot be empty")
	}

	spender, ok := inputs[1].(common.Address)
	if !ok {
		return nil, nil, fmt.Errorf("invalid spender address: %v", inputs[1])
	}

	if isZeroAddress(spender) {
		return nil, nil, fmt.Errorf("spender address cannot be empty")
	}

	// Return the max uint256 when the owner and spender are the same.
	if bytes.Equal(owner.Bytes(), spender.Bytes()) {
		return core.MethodOutputs{abi.MaxUint256}, nil, nil
	}

	var allowance sdkmath.Int
	authorization, _ := am.authzkeeper.GetAuthorization(
		context.SdkCtx(),
		core.TypesConverter.Address.ToSDK(spender),
		core.TypesConverter.Address.ToSDK(owner),
		SendMsgURL,
	)
	if authorization == nil {
		allowance = sdkmath.ZeroInt()
	} else {
		sendAuth, ok := authorization.(*banktypes.SendAuthorization)
		if !ok {
			return nil, nil, fmt.Errorf(
				"expected authorization to be a %T", banktypes.SendAuthorization{},
			)
		}

		allowance = sendAuth.SpendLimit.AmountOfNoDenomValidation(am.denom)
	}

	return core.MethodOutputs{
		core.TypesConverter.BigInt.FromSDK(allowance),
	}, nil, nil
}
