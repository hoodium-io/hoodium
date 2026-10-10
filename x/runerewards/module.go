package runerewards

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/gorilla/mux"
	"github.com/grpc-ecosystem/grpc-gateway/runtime"
	"github.com/spf13/cobra"

	abci "github.com/cometbft/cometbft/abci/types"

	"cosmossdk.io/core/appmodule"
	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/codec"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/module"

	"github.com/hoodium-io/hoodium/x/runerewards/keeper"
	"github.com/hoodium-io/hoodium/x/runerewards/types"
)

var (
	_ module.AppModule      = AppModule{}
	_ module.AppModuleBasic = AppModuleBasic{}
	_ appmodule.AppModule   = AppModule{}
)

// AppModuleBasic defines the basic application module used by the runerewards module.
type AppModuleBasic struct{}

// Name returns the runerewards module's name.
func (AppModuleBasic) Name() string {
	return types.ModuleName
}

// RegisterLegacyAminoCodec performs a no-op as the runerewards module doesn't support amino.
func (AppModuleBasic) RegisterLegacyAminoCodec(_ *codec.LegacyAmino) {}

// ConsensusVersion returns the consensus state-breaking version for the module.
//
// Bumped 1 -> 2 when the Tier proto gained the PoNA activity fields
// (tx_count_threshold, low_activity_reward, zero_activity_reward) and
// min_reward_per_block was removed: the on-disk Params encoding changed.
func (AppModuleBasic) ConsensusVersion() uint64 {
	return 2
}

// DefaultGenesis returns default genesis state as raw bytes for the runerewards module.
func (AppModuleBasic) DefaultGenesis(cdc codec.JSONCodec) json.RawMessage {
	return cdc.MustMarshalJSON(types.DefaultGenesisState())
}

// ValidateGenesis is the validation check of the genesis.
func (AppModuleBasic) ValidateGenesis(cdc codec.JSONCodec, _ client.TxEncodingConfig, bz json.RawMessage) error {
	var genState types.GenesisState
	if err := cdc.UnmarshalJSON(bz, &genState); err != nil {
		return fmt.Errorf("failed to unmarshal %s genesis state: %w", types.ModuleName, err)
	}
	return genState.Validate()
}

// RegisterRESTRoutes performs a no-op as the module doesn't expose REST endpoints.
func (AppModuleBasic) RegisterRESTRoutes(_ client.Context, _ *mux.Router) {}

// RegisterGRPCGatewayRoutes performs a no-op as the module exposes no gRPC gateway routes yet.
func (AppModuleBasic) RegisterGRPCGatewayRoutes(_ client.Context, _ *runtime.ServeMux) {}

// GetTxCmd returns no root tx command for the runerewards module.
func (AppModuleBasic) GetTxCmd() *cobra.Command { return nil }

// GetQueryCmd returns no root query command for the runerewards module.
func (AppModuleBasic) GetQueryCmd() *cobra.Command { return nil }

// RegisterInterfaces registers interfaces and implementations of the runerewards module.
func (AppModuleBasic) RegisterInterfaces(_ codectypes.InterfaceRegistry) {}

// AppModule implements an application module for the runerewards module.
type AppModule struct {
	AppModuleBasic

	keeper keeper.Keeper
}

// NewAppModule creates a new AppModule object.
func NewAppModule(k keeper.Keeper) AppModule {
	return AppModule{
		AppModuleBasic: AppModuleBasic{},
		keeper:         k,
	}
}

// IsOnePerModuleType implements the depinject.OnePerModuleType interface.
func (AppModule) IsOnePerModuleType() {}

// IsAppModule implements the appmodule.AppModule interface.
func (AppModule) IsAppModule() {}

// RegisterInvariants performs a no-op as the runerewards module exposes no invariants.
func (AppModule) RegisterInvariants(_ sdk.InvariantRegistry) {}

// RegisterServices performs a no-op; the module exposes no Msg or Query services yet.
func (AppModule) RegisterServices(_ module.Configurator) {}

// InitGenesis initializes the module's state from a provided genesis state.
func (am AppModule) InitGenesis(ctx sdk.Context, cdc codec.JSONCodec, data json.RawMessage) []abci.ValidatorUpdate {
	InitGenesis(ctx, am.keeper, cdc, data)
	return []abci.ValidatorUpdate{}
}

// ExportGenesis returns the module's exported genesis state as raw JSON bytes.
func (am AppModule) ExportGenesis(ctx sdk.Context, cdc codec.JSONCodec) json.RawMessage {
	return ExportGenesis(ctx, am.keeper, cdc)
}

// BeginBlock performs a no-op.
func (AppModule) BeginBlock(_ context.Context) error { return nil }

// EndBlock pays the block reward for the closing block from the validator
// reward pool to the block proposer.
//
// The reward is scaled by Proof of Network Activity: the number of transactions
// in this block, captured in the application's PreBlock hook (see
// keeper.SetBlockTxCount), selects the tier's full, low, or zero activity rate.
//
// The proposer consensus address is taken from the ABCI request (FinalizeBlock) /
// the configured proposer. Transaction fees are NOT handled here; they follow the
// standard x/distribution flow (fee pool split across the active validator set by
// voting power).
func (am AppModule) EndBlock(ctx context.Context) error {
	sdkCtx := sdk.UnwrapSDKContext(ctx)

	proposerConsAddr := sdk.ConsAddress(sdkCtx.BlockHeader().ProposerAddress)
	if len(proposerConsAddr) == 0 {
		// No proposer recorded for this block (e.g. genesis); nothing to pay.
		return nil
	}

	_, err := am.keeper.DistributeRuneBlockReward(sdkCtx, proposerConsAddr)
	return err
}
