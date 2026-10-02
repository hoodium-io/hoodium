package staking

import (
	"fmt"
	"math/big"

	sdkmath "cosmossdk.io/math"

	sdk "github.com/cosmos/cosmos-sdk/types"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
	"github.com/ethereum/go-ethereum/common"
	"github.com/hoodium-io/hoodium/core"
	"github.com/hoodium-io/hoodium/x/evm/statedb"
	evmtypes "github.com/hoodium-io/hoodium/x/evm/types"
)

// EvmAddress is the EVM address of the Staking precompile. It lets EVM wallets
// delegate RUNE to a validator without switching to a Cosmos wallet.
const EvmAddress = evmtypes.StakingPrecompileAddress

// StakingKeeper is the subset of the x/staking keeper used by this precompile.
type StakingKeeper interface {
	// Validator returns the validator for the given consensus address.
	Validator(ctx sdk.Context, addr sdk.ValAddress) (stakingtypes.Validator, error)
	// Delegate bonds the given amount to the validator on behalf of delAddr.
	Delegate(
		ctx sdk.Context,
		delAddr sdk.AccAddress,
		bondAmt sdkmath.Int,
		validator stakingtypes.Validator,
		subtractAccount bool,
	) (newShares sdkmath.LegacyDec, err error)
}

func NewPrecompileVersionMap(sk StakingKeeper) (*core.VersionMap, error) {
	contractV1, err := NewPrecompile(sk)
	if err != nil {
		return nil, err
	}
	return core.NewVersionMap(map[int]*core.Contract{
		0: contractV1,
		evmtypes.StakingPrecompileLatestVersion: contractV1,
	}), nil
}

// NewPrecompile creates the Staking precompile. The ABI/bytecode are wired once
// the Hardhat ABI pass generates them; for now the method is registered
// directly against a minimal empty-name contract (consistent with how
// max_supply was staged).
func NewPrecompile(sk StakingKeeper) (*core.Contract, error) {
	evmAddress := common.HexToAddress(EvmAddress)
	contract := core.NewContract(
		nil,
		evmAddress,
		"",
		"staking",
	)
	contract.RegisterMethods(
		newDelegateMethod(sk),
	)
	return contract, nil
}

const DelegateMethodName = "delegate"

type delegateMethod struct {
	sk StakingKeeper
}

func newDelegateMethod(sk StakingKeeper) *delegateMethod {
	return &delegateMethod{sk: sk}
}

func (m *delegateMethod) MethodName() string          { return DelegateMethodName }
func (m *delegateMethod) MethodType() core.MethodType { return core.Write }
func (m *delegateMethod) RequiredGas(_ []byte) (uint64, bool) {
	return 0, false
}
func (m *delegateMethod) Payable() bool { return false }

// Run implements delegate(address validator, uint256 amount) → bool.
// The validator input is the EVM address of the validator operator; the amount
// is in the RUNE base denomination (arune).
func (m *delegateMethod) Run(
	context *core.RunContext,
	inputs core.MethodInputs,
) (core.MethodOutputs, []statedb.StateChange, error) {
	if err := core.ValidateMethodInputsCount(inputs, 2); err != nil {
		return nil, nil, err
	}

	validatorAddr, ok := inputs[0].(common.Address)
	if !ok {
		return nil, nil, fmt.Errorf("validator argument must be common.Address")
	}

	amount, ok := inputs[1].(*big.Int)
	if !ok {
		return nil, nil, fmt.Errorf("amount argument must be *big.Int")
	}

	// The msg sender is the delegating EVM account.
	delegator := core.TypesConverter.Address.ToSDK(context.MsgSender())

	validator, err := m.sk.Validator(context.SdkCtx(), sdk.ValAddress(validatorAddr.Bytes()))
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get validator: %w", err)
	}

	_, err = m.sk.Delegate(
		context.SdkCtx(),
		delegator,
		sdkmath.NewIntFromBigInt(amount),
		validator,
		false, // let the keeper subtract from the account
	)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to delegate: %w", err)
	}

	return core.MethodOutputs{true}, nil, nil
}
