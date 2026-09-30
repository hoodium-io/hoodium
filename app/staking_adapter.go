package app

import (
	tmproto "github.com/cometbft/cometbft/proto/tendermint/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	stakingkeeper "github.com/cosmos/cosmos-sdk/x/staking/keeper"
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
