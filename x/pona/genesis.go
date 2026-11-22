package pona

import (
	"encoding/json"

	"github.com/cosmos/cosmos-sdk/codec"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/hoodium-io/hoodium/x/pona/keeper"
	"github.com/hoodium-io/hoodium/x/pona/types"
)

// InitGenesis initializes the pona module's state.
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

	// Restore the dPona difficulty-controller state. A zero value means "dPona
	// off", which is what a fresh chain starts from; exporting and re-importing a
	// running chain therefore resumes at the same difficulty.
	if err := genState.DPona.Validate(); err != nil {
		panic(err)
	}
	k.SetDPona(ctx, genState.DPona)
}

// ExportGenesis returns the pona module's exported genesis state as raw
// JSON bytes.
func ExportGenesis(ctx sdk.Context, k keeper.Keeper, cdc codec.JSONCodec) json.RawMessage {
	genesis := types.GenesisState{
		Params: k.GetParams(ctx),
		DPona:  k.GetDPona(ctx),
	}
	return cdc.MustMarshalJSON(&genesis)
}
