package app

import (
	"context"

	addresscodec "cosmossdk.io/core/address"
	tmproto "github.com/cometbft/cometbft/proto/tendermint/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	stakingkeeper "github.com/cosmos/cosmos-sdk/x/staking/keeper"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
)

// stakingKeeperAdapter adapts the standard *stakingkeeper.Keeper to the narrow
// StakingKeeper interface expected by x/evm (GetHistoricalHeader +
// GetValidatorOperator). The standard keeper exposes GetHistoricalInfo (returning
// stakingtypes.HistoricalInfo) and GetValidatorByConsAddr (returning a concrete
// Validator); this wrapper projects them onto the narrower surface x/evm needs.
type stakingKeeperAdapter struct {
	sk *stakingkeeper.Keeper
}

func newStakingKeeperAdapter(sk *stakingkeeper.Keeper) stakingKeeperAdapter {
	return stakingKeeperAdapter{sk: sk}
}

func (a stakingKeeperAdapter) GetHistoricalHeader(ctx sdk.Context, height int64) (tmproto.Header, bool) {
	histInfo, err := a.sk.GetHistoricalInfo(ctx, height)
	if err != nil {
		return tmproto.Header{}, false
	}

	return histInfo.Header, true
}

func (a stakingKeeperAdapter) GetValidatorOperator(ctx sdk.Context, consAddr sdk.ConsAddress) (sdk.ValAddress, bool) {
	validator, err := a.sk.GetValidatorByConsAddr(ctx, consAddr)
	if err != nil {
		return nil, false
	}

	// stakingtypes.Validator.GetOperator() returns a bech32 string; x/evm needs
	// an sdk.ValAddress.
	operator, err := sdk.ValAddressFromBech32(validator.GetOperator())
	if err != nil {
		return nil, false
	}

	return operator, true
}

// stakingMsgServerAdapter adapts the x/staking message server to the narrow
// MsgServerKeeper interface expected by the staking precompile
// (core/staking).
//
// The SDK's MsgServer interface exposes CreateValidator but not the validator
// address codec, which the precompile needs to encode the caller's EVM address
// into a bech32 operator address. Both come from the same underlying keeper, so
// this wrapper simply pairs them.
type stakingMsgServerAdapter struct {
	stakingtypes.MsgServer
	sk *stakingkeeper.Keeper
}

func newStakingMsgServerAdapter(sk *stakingkeeper.Keeper) stakingMsgServerAdapter {
	return stakingMsgServerAdapter{
		MsgServer: stakingkeeper.NewMsgServerImpl(sk),
		sk:        sk,
	}
}

func (a stakingMsgServerAdapter) ValidatorAddressCodec() addresscodec.Codec {
	return a.sk.ValidatorAddressCodec()
}

// Compile-time assertions: the adapter must satisfy the precompile's interface,
// and it must keep delegating CreateValidator to the SDK implementation.
var (
	_ interface {
		CreateValidator(context.Context, *stakingtypes.MsgCreateValidator) (*stakingtypes.MsgCreateValidatorResponse, error)
		ValidatorAddressCodec() addresscodec.Codec
	} = stakingMsgServerAdapter{}
)
