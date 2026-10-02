package staking

import (
	"context"
	"fmt"
	"math/big"
	"time"

	sdkmath "cosmossdk.io/math"

	sdk "github.com/cosmos/cosmos-sdk/types"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
	"github.com/ethereum/go-ethereum/accounts/abi"
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
	// GetValidator returns the concrete validator for the given operator address.
	GetValidator(ctx context.Context, addr sdk.ValAddress) (stakingtypes.Validator, error)
	// Delegate bonds the given amount to the validator on behalf of delAddr.
	Delegate(
		ctx context.Context,
		delAddr sdk.AccAddress,
		bondAmt sdkmath.Int,
		tokenSrc stakingtypes.BondStatus,
		validator stakingtypes.Validator,
		subtractAccount bool,
	) (newShares sdkmath.LegacyDec, err error)
	// Undelegate unbonds sharesAmount from the validator on behalf of delAddr.
	Undelegate(
		ctx context.Context,
		delAddr sdk.AccAddress,
		valAddr sdk.ValAddress,
		sharesAmount sdkmath.LegacyDec,
	) (completionTime time.Time, amount sdkmath.Int, err error)
	// GetDelegation returns the delegation between delAddr and valAddr.
	GetDelegation(ctx context.Context, delAddr sdk.AccAddress, valAddr sdk.ValAddress) (stakingtypes.Delegation, error)
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
		abi.ABI{},
		evmAddress,
		"",
		"staking",
	)
	contract.RegisterMethods(
		newDelegateMethod(sk),
		newUndelegateMethod(sk),
		newGetDelegationMethod(sk),
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
	runCtx *core.RunContext,
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
	delegator := core.TypesConverter.Address.ToSDK(runCtx.MsgSender())

	validator, err := m.sk.GetValidator(runCtx.SdkCtx(), sdk.ValAddress(validatorAddr.Bytes()))
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get validator: %w", err)
	}

	_, err = m.sk.Delegate(
		runCtx.SdkCtx(),
		delegator,
		sdkmath.NewIntFromBigInt(amount),
		stakingtypes.Unbonded,
		validator,
		false, // let the keeper subtract from the account
	)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to delegate: %w", err)
	}

	return core.MethodOutputs{true}, nil, nil
}

const UndelegateMethodName = "undelegate"

type undelegateMethod struct {
	sk StakingKeeper
}

func newUndelegateMethod(sk StakingKeeper) *undelegateMethod {
	return &undelegateMethod{sk: sk}
}

func (m *undelegateMethod) MethodName() string          { return UndelegateMethodName }
func (m *undelegateMethod) MethodType() core.MethodType { return core.Write }
func (m *undelegateMethod) RequiredGas(_ []byte) (uint64, bool) {
	return 0, false
}
func (m *undelegateMethod) Payable() bool { return false }

// Run implements undelegate(address validator, uint256 amount) → bool.
// The amount is the number of shares (in RUNE base denomination) to unbond.
func (m *undelegateMethod) Run(
	runCtx *core.RunContext,
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

	delegator := core.TypesConverter.Address.ToSDK(runCtx.MsgSender())
	valAddr := sdk.ValAddress(validatorAddr.Bytes())

	_, _, err := m.sk.Undelegate(
		runCtx.SdkCtx(),
		delegator,
		valAddr,
		sdkmath.LegacyNewDecFromInt(sdkmath.NewIntFromBigInt(amount)),
	)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to undelegate: %w", err)
	}

	return core.MethodOutputs{true}, nil, nil
}

const GetDelegationMethodName = "getDelegation"

type getDelegationMethod struct {
	sk StakingKeeper
}

func newGetDelegationMethod(sk StakingKeeper) *getDelegationMethod {
	return &getDelegationMethod{sk: sk}
}

func (m *getDelegationMethod) MethodName() string          { return GetDelegationMethodName }
func (m *getDelegationMethod) MethodType() core.MethodType { return core.Read }
func (m *getDelegationMethod) RequiredGas(_ []byte) (uint64, bool) {
	return 0, false
}
func (m *getDelegationMethod) Payable() bool { return false }

// Run implements getDelegation(address delegator, address validator) → (uint256 shares).
func (m *getDelegationMethod) Run(
	runCtx *core.RunContext,
	inputs core.MethodInputs,
) (core.MethodOutputs, []statedb.StateChange, error) {
	if err := core.ValidateMethodInputsCount(inputs, 2); err != nil {
		return nil, nil, err
	}

	delegatorAddr, ok := inputs[0].(common.Address)
	if !ok {
		return nil, nil, fmt.Errorf("delegator argument must be common.Address")
	}

	validatorAddr, ok := inputs[1].(common.Address)
	if !ok {
		return nil, nil, fmt.Errorf("validator argument must be common.Address")
	}

	delAddr := sdk.AccAddress(delegatorAddr.Bytes())
	valAddr := sdk.ValAddress(validatorAddr.Bytes())

	delegation, err := m.sk.GetDelegation(runCtx.SdkCtx(), delAddr, valAddr)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get delegation: %w", err)
	}

	shares := delegation.Shares.BigInt()
	return core.MethodOutputs{shares}, nil, nil
}
