package staking

import (
	"context"
	"embed"
	"fmt"
	"math/big"
	"time"

	sdkmath "cosmossdk.io/math"

	addresscodec "cosmossdk.io/core/address"
	"github.com/cosmos/cosmos-sdk/crypto/keys/ed25519"
	sdk "github.com/cosmos/cosmos-sdk/types"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
	"github.com/ethereum/go-ethereum/common"
	"github.com/hoodium-io/hoodium/core"
	runetypes "github.com/hoodium-io/hoodium/types"
	"github.com/hoodium-io/hoodium/x/evm/statedb"
	evmtypes "github.com/hoodium-io/hoodium/x/evm/types"
)

//go:embed abi.json
var filesystem embed.FS

// EvmAddress is the EVM address of the Staking precompile. It lets EVM wallets
// delegate RUNE to a validator without switching to a Cosmos wallet.
const EvmAddress = evmtypes.StakingPrecompileAddress

// SlashingKeeper is the subset of the x/slashing keeper used by this precompile.
//
// It exists so EVM accounts can unjail a validator. `MsgUnjail` is a native
// Cosmos message and is UNREACHABLE on Hoodium (the ante handler routes only EVM
// transactions), so the precompile is the only path to recovery.
type SlashingKeeper interface {
	// Unjail unjails a validator if its jail period has elapsed. It errors if
	// the validator is tombstoned, still jailed, or below min self-delegation.
	Unjail(ctx context.Context, validatorAddr sdk.ValAddress) error
}

// MinDelegation is the minimum amount of RUNE that can be delegated in a single
// `delegate` call, in arune (1 RUNE = 10^18 arune). It is 0.1 RUNE.
//
// The value is defined in the shared `types` package because it is enforced at
// BOTH delegation entry points — this precompile (for EVM wallets) and the
// Cosmos ante chain (for `runed tx staking delegate`). Keeping one definition
// means the two can never drift apart.
//
// NOTE: this is the ENFORCEMENT point for EVM wallets; the Cosmos path is
// enforced by `app/ante/cosmos.MinDelegationDecorator`.
var MinDelegation = runetypes.MinDelegation

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
	// BondDenom returns the staking bond denomination (arune on Hoodium).
	BondDenom(ctx context.Context) (string, error)
}

// MsgServerKeeper is the subset of the x/staking keeper needed to create a
// validator through the module's own message server.
//
// Creating a validator by hand (NewValidator + SetValidator + Delegate) would
// silently skip the validations and hooks that MsgServer.CreateValidator
// performs: minimum-commission enforcement, duplicate operator/consensus-key
// detection, bond-denom checking, description length limits, consensus pubkey
// type allow-listing, power-index maintenance and the AfterValidatorCreated
// hook. Routing through the message server keeps the precompile behaviourally
// identical to the `MsgCreateValidator` transaction path.
type MsgServerKeeper interface {
	// CreateValidator runs the x/staking MsgServer's CreateValidator.
	CreateValidator(
		ctx context.Context,
		msg *stakingtypes.MsgCreateValidator,
	) (*stakingtypes.MsgCreateValidatorResponse, error)
	// ValidatorAddressCodec returns the codec used to encode/decode validator
	// operator addresses (bech32, HRP "runevaloper" on Hoodium).
	ValidatorAddressCodec() addresscodec.Codec
}

func NewPrecompileVersionMap(sk StakingKeeper, slk SlashingKeeper, msk MsgServerKeeper) (*core.VersionMap, error) {
	contractV1, err := NewPrecompile(sk, slk, msk)
	if err != nil {
		return nil, err
	}
	return core.NewVersionMap(map[int]*core.Contract{
		0:                                       contractV1,
		evmtypes.StakingPrecompileLatestVersion: contractV1,
	}), nil
}

// NewPrecompile creates the Staking precompile. The ABI and bytecode are
// embedded from the generated `abi.json` and `byte_code.go` (produced from the
// StakingCaller contract via `make precompile-gen`). Registering the ABI is what
// makes the precompile register a genesis account so eth_getCode returns its
// bytecode.
func NewPrecompile(sk StakingKeeper, slk SlashingKeeper, msk MsgServerKeeper) (*core.Contract, error) {
	contractAbi, err := core.LoadAbiFile(filesystem, "abi.json")
	if err != nil {
		return nil, fmt.Errorf("failed to load abi file: [%w]", err)
	}

	evmAddress := common.HexToAddress(EvmAddress)
	contract := core.NewContract(
		contractAbi,
		evmAddress,
		EvmByteCode,
		"staking",
	)
	contract.RegisterMethods(
		newCreateValidatorMethod(sk, msk),
		newDelegateMethod(sk),
		newUndelegateMethod(sk),
		newGetDelegationMethod(sk),
		newUnjailMethod(slk),
	)
	return contract, nil
}

const CreateValidatorMethodName = "createValidator"

type createValidatorMethod struct {
	sk  StakingKeeper
	msk MsgServerKeeper
}

func newCreateValidatorMethod(sk StakingKeeper, msk MsgServerKeeper) *createValidatorMethod {
	return &createValidatorMethod{sk: sk, msk: msk}
}

func (m *createValidatorMethod) MethodName() string          { return CreateValidatorMethodName }
func (m *createValidatorMethod) MethodType() core.MethodType { return core.Write }
func (m *createValidatorMethod) RequiredGas(_ []byte) (uint64, bool) {
	return 0, false
}
func (m *createValidatorMethod) Payable() bool { return false }

// legacyDecFromFixedPoint converts a fixed-point integer with 18 decimals into
// a Cosmos SDK LegacyDec. A nil or negative input becomes zero; the underlying
// commission validation then rejects the resulting rates with a clear error.
func legacyDecFromFixedPoint(value *big.Int) sdkmath.LegacyDec {
	if value == nil || value.Sign() < 0 {
		return sdkmath.LegacyZeroDec()
	}

	return sdkmath.LegacyNewDecFromBigIntWithPrec(value, sdkmath.LegacyPrecision)
}

// Run implements
//
//	createValidator(
//	    bytes   consensusPubKey,
//	    uint256 amount,
//	    string  moniker,
//	    uint256 commissionRate,
//	    uint256 commissionMaxRate,
//	    uint256 commissionMaxChangeRate,
//	    uint256 minSelfDelegation
//	) → bool
//
// The validator's OPERATOR address is the EVM msg.sender: the caller's own
// account becomes the operator, so an account can never create a validator it
// does not control. `amount` is the self-delegation in arune.
//
// Commission and min-self-delegation values are fixed-point integers with 18
// decimals (1e18 == 100%), matching the arune denomination. Passing the scale
// explicitly avoids any ambiguity in a Solidity `uint256` ratio.
//
// Every real precondition is enforced by x/staking's own message server
// (minimum commission, duplicate operator/consensus key, bond denom,
// description length, pubkey type, min-self-delegation ≤ amount), so this
// method cannot drift from `MsgCreateValidator` transaction semantics.
func (m *createValidatorMethod) Run(
	runCtx *core.RunContext,
	inputs core.MethodInputs,
) (core.MethodOutputs, []statedb.StateChange, error) {
	if err := core.ValidateMethodInputsCount(inputs, 7); err != nil {
		return nil, nil, err
	}

	consensusPubKeyBytes, ok := inputs[0].([]byte)
	if !ok {
		return nil, nil, fmt.Errorf("consensusPubKey argument must be []byte")
	}

	amount, ok := inputs[1].(*big.Int)
	if !ok {
		return nil, nil, fmt.Errorf("amount argument must be *big.Int")
	}

	moniker, ok := inputs[2].(string)
	if !ok {
		return nil, nil, fmt.Errorf("moniker argument must be string")
	}

	commissionRate, ok := inputs[3].(*big.Int)
	if !ok {
		return nil, nil, fmt.Errorf("commissionRate argument must be *big.Int")
	}

	commissionMaxRate, ok := inputs[4].(*big.Int)
	if !ok {
		return nil, nil, fmt.Errorf("commissionMaxRate argument must be *big.Int")
	}

	commissionMaxChangeRate, ok := inputs[5].(*big.Int)
	if !ok {
		return nil, nil, fmt.Errorf("commissionMaxChangeRate argument must be *big.Int")
	}

	minSelfDelegation, ok := inputs[6].(*big.Int)
	if !ok {
		return nil, nil, fmt.Errorf("minSelfDelegation argument must be *big.Int")
	}

	// The consensus key must be a 32-byte ed25519 public key — the only
	// validator key type this chain accepts.
	if len(consensusPubKeyBytes) != ed25519.PubKeySize {
		return nil, nil, fmt.Errorf(
			"consensus public key must be %d bytes (ed25519), got %d",
			ed25519.PubKeySize, len(consensusPubKeyBytes),
		)
	}

	// The operator is the caller. An EVM account and its Cosmos account share
	// the same 20 bytes, so this is an exact identity mapping.
	operator := core.TypesConverter.Address.ToSDK(runCtx.MsgSender())

	operatorAddr, err := m.msk.ValidatorAddressCodec().BytesToString(operator.Bytes())
	if err != nil {
		return nil, nil, fmt.Errorf("failed to encode validator operator address: %w", err)
	}

	denom, err := m.sk.BondDenom(runCtx.SdkCtx())
	if err != nil {
		return nil, nil, fmt.Errorf("failed to read staking bond denom: %w", err)
	}

	msg, err := stakingtypes.NewMsgCreateValidator(
		operatorAddr,
		&ed25519.PubKey{Key: consensusPubKeyBytes},
		sdk.NewCoin(denom, sdkmath.NewIntFromBigInt(amount)),
		stakingtypes.NewDescription(moniker, "", "", "", ""),
		stakingtypes.NewCommissionRates(
			legacyDecFromFixedPoint(commissionRate),
			legacyDecFromFixedPoint(commissionMaxRate),
			legacyDecFromFixedPoint(commissionMaxChangeRate),
		),
		sdkmath.NewIntFromBigInt(minSelfDelegation),
	)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to build create-validator message: %w", err)
	}

	if _, err := m.msk.CreateValidator(runCtx.SdkCtx(), msg); err != nil {
		return nil, nil, fmt.Errorf("failed to create validator: %w", err)
	}

	return core.MethodOutputs{true}, nil, nil
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

	// Enforce Hoodium's minimum delegation (0.1 RUNE). Cosmos staking has no
	// min-delegation param, so this precompile is the enforcement point.
	delegationAmount := sdkmath.NewIntFromBigInt(amount)
	if delegationAmount.LT(MinDelegation) {
		return nil, nil, fmt.Errorf(
			"delegation amount %s is below the minimum of %s arune (0.1 RUNE)",
			delegationAmount, MinDelegation,
		)
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
		delegationAmount,
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

// Run implements getDelegation(address delegator, address validator) → (uint256 amount).
// Returns the token-equivalent (RUNE amount in base denomination) of the
// delegator's shares, truncated to an integer.
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

	validator, err := m.sk.GetValidator(runCtx.SdkCtx(), valAddr)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get validator: %w", err)
	}

	// Convert shares → tokens using the validator's share exchange rate, then
	// truncate to an integer RUNE amount (base denomination).
	tokens := validator.TokensFromShares(delegation.Shares)
	return core.MethodOutputs{tokens.TruncateInt().BigInt()}, nil, nil
}

const UnjailMethodName = "unjail"

type unjailMethod struct {
	slk SlashingKeeper
}

func newUnjailMethod(slk SlashingKeeper) *unjailMethod {
	return &unjailMethod{slk: slk}
}

func (m *unjailMethod) MethodName() string          { return UnjailMethodName }
func (m *unjailMethod) MethodType() core.MethodType { return core.Write }
func (m *unjailMethod) RequiredGas(_ []byte) (uint64, bool) {
	return 0, false
}
func (m *unjailMethod) Payable() bool { return false }

// Run implements unjail(address validator) → bool.
//
// Recovery path for a jailed validator. This exists because `MsgUnjail` is a
// native Cosmos message and is unreachable on Hoodium — the ante handler routes
// only EVM transactions — so without this method a jailed validator could never
// be unjailed.
//
// The caller must be the validator's own operator account: only the operator may
// unjail its validator. The keeper then enforces every real precondition:
//
//   - the validator must actually be jailed,
//   - the jail period (`JailedUntil`) must have elapsed,
//   - the validator must not be tombstoned (tombstoning is permanent — a
//     double-sign cannot be undone), and
//   - self-delegation must be at or above `MinSelfDelegation`.
//
// All of that lives in x/slashing's Unjail; this method only adds the
// operator-only check so one account cannot unjail another's validator.
func (m *unjailMethod) Run(
	runCtx *core.RunContext,
	inputs core.MethodInputs,
) (core.MethodOutputs, []statedb.StateChange, error) {
	if err := core.ValidateMethodInputsCount(inputs, 1); err != nil {
		return nil, nil, err
	}

	validatorAddr, ok := inputs[0].(common.Address)
	if !ok {
		return nil, nil, fmt.Errorf("validator argument must be common.Address")
	}

	// The caller must be the validator's own operator. The signer is the EVM
	// msg sender; a validator's operator address is the same 20 bytes, so
	// comparing them is an exact identity check.
	if runCtx.MsgSender() != validatorAddr {
		return nil, nil, fmt.Errorf(
			"only the validator's own operator may unjail it: caller %s, validator %s",
			runCtx.MsgSender().Hex(), validatorAddr.Hex(),
		)
	}

	if err := m.slk.Unjail(runCtx.SdkCtx(), sdk.ValAddress(validatorAddr.Bytes())); err != nil {
		return nil, nil, fmt.Errorf("failed to unjail: %w", err)
	}

	return core.MethodOutputs{true}, nil, nil
}
