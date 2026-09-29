package maintenance

import (
	"fmt"
	"slices"

	"github.com/hoodium-io/hoodium/core"
	"github.com/hoodium-io/hoodium/x/evm/statedb"
	evmtypes "github.com/hoodium-io/hoodium/x/evm/types"
)

// SetSelfDestructDisabledMethodName is the name of the setSelfDestructDisabled
// method. It matches the name of the method in the contract ABI.
const SetSelfDestructDisabledMethodName = "setSelfDestructDisabled"

// setSelfDestructDisabledMethod toggles SelfdestructDisableEIP in ExtraEIPs.
type setSelfDestructDisabledMethod struct {
	poaKeeper PoaKeeper
	evmKeeper EvmKeeper
}

func newSetSelfDestructDisabledMethod(
	poaKeeper PoaKeeper,
	evmKeeper EvmKeeper,
) *setSelfDestructDisabledMethod {
	return &setSelfDestructDisabledMethod{
		poaKeeper: poaKeeper,
		evmKeeper: evmKeeper,
	}
}

func (m *setSelfDestructDisabledMethod) MethodName() string {
	return SetSelfDestructDisabledMethodName
}

func (m *setSelfDestructDisabledMethod) MethodType() core.MethodType {
	return core.Write
}

func (m *setSelfDestructDisabledMethod) RequiredGas(_ []byte) (uint64, bool) {
	return 0, false
}

func (m *setSelfDestructDisabledMethod) Payable() bool {
	return false
}

func (m *setSelfDestructDisabledMethod) Run(
	context *core.RunContext,
	inputs core.MethodInputs,
) (core.MethodOutputs, []statedb.StateChange, error) {
	if err := core.ValidateMethodInputsCount(inputs, 1); err != nil {
		return nil, nil, err
	}

	disabled, ok := inputs[0].(bool)
	if !ok {
		return nil, nil, fmt.Errorf("disabled argument must be a boolean")
	}

	// This method is restricted to the PoA owner.
	err := m.poaKeeper.CheckOwner(
		context.SdkCtx(),
		core.TypesConverter.Address.ToSDK(context.MsgSender()),
	)
	if err != nil {
		return nil, nil, err
	}

	params := m.evmKeeper.GetParams(context.SdkCtx())

	if disabled {
		if !slices.Contains(params.ExtraEIPs, evmtypes.SelfdestructDisableEIP) {
			params.ExtraEIPs = append(params.ExtraEIPs, evmtypes.SelfdestructDisableEIP)
		}
	} else {
		params.ExtraEIPs = slices.DeleteFunc(params.ExtraEIPs, func(eip int64) bool {
			return eip == evmtypes.SelfdestructDisableEIP
		})
	}

	if err := m.evmKeeper.SetParams(context.SdkCtx(), params); err != nil {
		return nil, nil, err
	}

	return core.MethodOutputs{true}, nil, nil
}

// GetSelfDestructDisabledMethodName is the name of the getSelfDestructDisabled
// method. It matches the name of the method in the contract ABI.
const GetSelfDestructDisabledMethodName = "getSelfDestructDisabled"

// getSelfDestructDisabledMethod reports whether SelfdestructDisableEIP is set.
type getSelfDestructDisabledMethod struct {
	evmKeeper EvmKeeper
}

func newGetSelfDestructDisabledMethod(
	evmKeeper EvmKeeper,
) *getSelfDestructDisabledMethod {
	return &getSelfDestructDisabledMethod{evmKeeper: evmKeeper}
}

func (m *getSelfDestructDisabledMethod) MethodName() string {
	return GetSelfDestructDisabledMethodName
}

func (m *getSelfDestructDisabledMethod) MethodType() core.MethodType {
	return core.Read
}

func (m *getSelfDestructDisabledMethod) RequiredGas(_ []byte) (uint64, bool) {
	return 0, false
}

func (m *getSelfDestructDisabledMethod) Payable() bool {
	return false
}

func (m *getSelfDestructDisabledMethod) Run(
	context *core.RunContext,
	inputs core.MethodInputs,
) (core.MethodOutputs, []statedb.StateChange, error) {
	if err := core.ValidateMethodInputsCount(inputs, 0); err != nil {
		return nil, nil, err
	}

	params := m.evmKeeper.GetParams(context.SdkCtx())
	disabled := slices.Contains(params.ExtraEIPs, evmtypes.SelfdestructDisableEIP)

	return core.MethodOutputs{disabled}, nil, nil
}
