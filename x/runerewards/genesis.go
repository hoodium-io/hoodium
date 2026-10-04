package runerewards

import (
	"github.com/cosmos/cosmos-sdk/codec"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/hoodium-io/hoodium/x/runerewards/keeper"
	"github.com/hoodium-io/hoodium/x/runerewards/types"
)

// InitGenesis initializes the runerewards module's state and returns the module's
// validator updates (none, always empty).
//
// NOTE: funding of the validator reward pool is done at genesis via the bank
// genesis balances (project premine + reserve), NOT here. The pool balance is
// supplied as a bank genesis balance and swept into the pool module account by
// the auth/bank module-account holder. See cmd/runed/testnet.go for the RUNE
// genesis (9B premine + 1B reserve).
func InitGenesis(ctx sdk.Context, k keeper.Keeper, cdc codec.JSONCodec, data []byte) {
	var genState types.GenesisState
	cdc.MustUnmarshalJSON(data, &genState)

	if err := k.SetParams(ctx, genState.Params); err != nil {
		panic(err)
	}
}

// ExportGenesis returns the runerewards module's exported genesis state.
func ExportGenesis(ctx sdk.Context, k keeper.Keeper, cdc codec.JSONCodec) []byte {
	genesis := types.GenesisState{
		Params: k.GetParams(ctx),
	}
	return cdc.MustMarshalJSON(&genesis)
}
