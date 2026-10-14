package runerewards

import (
	"encoding/json"

	"github.com/cosmos/cosmos-sdk/codec"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/hoodium-io/hoodium/x/runerewards/keeper"
	"github.com/hoodium-io/hoodium/x/runerewards/types"
)

// InitGenesis initializes the runerewards module's state.
//
// NOTE: funding of the validator reward pool is done at genesis via the bank
// genesis balances (9B premine + 1B reserve), NOT here. See cmd/runed/testnet.go
// for the RUNE genesis allocation.
func InitGenesis(ctx sdk.Context, k keeper.Keeper, cdc codec.JSONCodec, data json.RawMessage) {
	var genState types.GenesisState
	cdc.MustUnmarshalJSON(data, &genState)

	if err := k.SetParams(ctx, genState.Params); err != nil {
		panic(err)
	}
}

// ExportGenesis returns the runerewards module's exported genesis state as raw
// JSON bytes.
func ExportGenesis(ctx sdk.Context, k keeper.Keeper, cdc codec.JSONCodec) json.RawMessage {
	genesis := types.GenesisState{
		Params: k.GetParams(ctx),
	}
	return cdc.MustMarshalJSON(&genesis)
}
