// Copyright 2022 Evmos Foundation
// This file is part of the Evmos Network packages.
//
// Evmos is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The Evmos packages are distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the Evmos packages. If not, see https://github.com/evmos/evmos/blob/main/LICENSE

package app

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/cosmos/cosmos-sdk/runtime"
	authcodec "github.com/cosmos/cosmos-sdk/x/auth/codec"

	"github.com/spf13/cast"

	"cosmossdk.io/log"
	abci "github.com/cometbft/cometbft/abci/types"
	tmos "github.com/cometbft/cometbft/libs/os"
	dbm "github.com/cosmos/cosmos-db"

	"github.com/cosmos/cosmos-sdk/baseapp"
	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/client/grpc/cmtservice"
	"github.com/cosmos/cosmos-sdk/client/grpc/node"
	"github.com/cosmos/cosmos-sdk/codec"
	"github.com/cosmos/cosmos-sdk/codec/types"
	"github.com/cosmos/cosmos-sdk/server/api"
	"github.com/cosmos/cosmos-sdk/server/config"

	"cosmossdk.io/simapp"
	simappparams "cosmossdk.io/simapp/params"
	storetypes "cosmossdk.io/store/types"
	"cosmossdk.io/x/upgrade"
	upgradekeeper "cosmossdk.io/x/upgrade/keeper"
	upgradetypes "cosmossdk.io/x/upgrade/types"
	servertypes "github.com/cosmos/cosmos-sdk/server/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/module"
	"github.com/cosmos/cosmos-sdk/version"
	"github.com/cosmos/cosmos-sdk/x/auth"
	authkeeper "github.com/cosmos/cosmos-sdk/x/auth/keeper"
	"github.com/cosmos/cosmos-sdk/x/auth/posthandler"
	authsims "github.com/cosmos/cosmos-sdk/x/auth/simulation"
	authtx "github.com/cosmos/cosmos-sdk/x/auth/tx"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	"github.com/cosmos/cosmos-sdk/x/authz"
	authzkeeper "github.com/cosmos/cosmos-sdk/x/authz/keeper"
	authzmodule "github.com/cosmos/cosmos-sdk/x/authz/module"
	"github.com/cosmos/cosmos-sdk/x/bank"
	bankkeeper "github.com/cosmos/cosmos-sdk/x/bank/keeper"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	consensusparams "github.com/cosmos/cosmos-sdk/x/consensus"
	consensusparamskeeper "github.com/cosmos/cosmos-sdk/x/consensus/keeper"
	consensusparamstypes "github.com/cosmos/cosmos-sdk/x/consensus/types"
	"github.com/cosmos/cosmos-sdk/x/crisis"
	crisiskeeper "github.com/cosmos/cosmos-sdk/x/crisis/keeper"
	crisistypes "github.com/cosmos/cosmos-sdk/x/crisis/types"
	distrtypes "github.com/cosmos/cosmos-sdk/x/distribution/types"
	distrkeeper "github.com/cosmos/cosmos-sdk/x/distribution/keeper"
	distr "github.com/cosmos/cosmos-sdk/x/distribution"
	"github.com/cosmos/cosmos-sdk/x/genutil"
	genutiltypes "github.com/cosmos/cosmos-sdk/x/genutil/types"
	"github.com/cosmos/cosmos-sdk/x/params"
	paramskeeper "github.com/cosmos/cosmos-sdk/x/params/keeper"
	paramstypes "github.com/cosmos/cosmos-sdk/x/params/types"
	"github.com/cosmos/cosmos-sdk/x/staking"
	stakingkeeper "github.com/cosmos/cosmos-sdk/x/staking/keeper"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"

	oracleclient "github.com/skip-mev/connect/v2/service/clients/oracle"
	servicemetrics "github.com/skip-mev/connect/v2/service/metrics"
	"github.com/skip-mev/connect/v2/x/marketmap"
	marketmapkeeper "github.com/skip-mev/connect/v2/x/marketmap/keeper"
	marketmaptypes "github.com/skip-mev/connect/v2/x/marketmap/types"
	"github.com/skip-mev/connect/v2/x/oracle"
	oraclekeeper "github.com/skip-mev/connect/v2/x/oracle/keeper"
	oracletypes "github.com/skip-mev/connect/v2/x/oracle/types"

	appabci "github.com/hoodium-io/hoodium/app/abci"
	ethante "github.com/hoodium-io/hoodium/app/ante/evm"
	"github.com/hoodium-io/hoodium/encoding"
	"github.com/hoodium-io/hoodium/evm/eip712"
	"github.com/hoodium-io/hoodium/core"
	"github.com/hoodium-io/hoodium/core/runetoken"
	"github.com/hoodium-io/hoodium/core/hooditoken"
	"github.com/hoodium-io/hoodium/core/priceoracle"
	"github.com/hoodium-io/hoodium/core/testbed"
	srvflags "github.com/hoodium-io/hoodium/server/flags"
	runetypes "github.com/hoodium-io/hoodium/types"

	"github.com/hoodium-io/hoodium/x/evm"
	evmkeeper "github.com/hoodium-io/hoodium/x/evm/keeper"
	evmtypes "github.com/hoodium-io/hoodium/x/evm/types"
	"github.com/hoodium-io/hoodium/x/feemarket"
	feemarketkeeper "github.com/hoodium-io/hoodium/x/feemarket/keeper"
	feemarkettypes "github.com/hoodium-io/hoodium/x/feemarket/types"

	"github.com/hoodium-io/hoodium/app/ante"

	// Force-load the tracer engines to trigger registration due to Go-Ethereum v1.10.15 changes

	//nolint:revive
	_ "github.com/ethereum/go-ethereum/eth/tracers/js"

	//nolint:revive
	_ "github.com/ethereum/go-ethereum/eth/tracers/native"
)

func init() {
	userHomeDir, err := os.UserHomeDir()
	if err != nil {
		panic(err)
	}

	DefaultNodeHome = filepath.Join(userHomeDir, ".runed")

	// manually update the power reduction by replacing micro (u) -> atto (a) btc
	sdk.DefaultPowerReduction = runetypes.PowerReduction
	// modify fee market parameter defaults through global
	feemarkettypes.DefaultMinGasPrice = MainnetMinGasPrices
	feemarkettypes.DefaultMinGasMultiplier = MainnetMinGasMultiplier
}

// Name defines the application binary name
const Name = "runed"

var (
	// DefaultNodeHome default home directories for the application daemon
	DefaultNodeHome string

	DefaultOracleTimeout = time.Second

	// ModuleBasics defines the module BasicManager is in charge of setting up basic,
	// non-dependant module elements, such as codec registration
	// and genesis verification.
	ModuleBasics = module.NewBasicManager(
		consensusparams.AppModuleBasic{},
		auth.AppModuleBasic{},
		bank.AppModuleBasic{},
		params.AppModuleBasic{},
		crisis.AppModuleBasic{},
		authzmodule.AppModuleBasic{},
		upgrade.AppModuleBasic{},
		evm.AppModuleBasic{},
		feemarket.AppModuleBasic{},
		marketmap.AppModuleBasic{},
		oracle.AppModuleBasic{},
		staking.AppModuleBasic{},
		distr.AppModuleBasic{},
		genutil.NewAppModuleBasic(genutiltypes.DefaultMessageValidator),
	)

	// module account permissions
	maccPerms = map[string][]string{
		authtypes.FeeCollectorName:       nil,
		evmtypes.ModuleName:              {authtypes.Minter, authtypes.Burner},
		stakingtypes.BondedPoolName:      {authtypes.Burner, authtypes.Staking},
		stakingtypes.NotBondedPoolName:   {authtypes.Burner, authtypes.Staking},
		distrtypes.ModuleName:            nil,
	}

	// module accounts that are allowed to receive tokens
	allowedReceivingModAcc = map[string]bool{}
)

var _ servertypes.Application = (*Hoodium)(nil)

// Hoodium implements an extended ABCI application. It is an application
// that may process transactions through Ethereum's EVM running atop of
// Tendermint consensus.
type Hoodium struct {
	*baseapp.BaseApp

	// encoding
	cdc               *codec.LegacyAmino
	appCodec          codec.Codec
	interfaceRegistry types.InterfaceRegistry

	invCheckPeriod uint

	// keys to access the substores
	keys  map[string]*storetypes.KVStoreKey
	tkeys map[string]*storetypes.TransientStoreKey

	// keepers
	ConsensusParamsKeeper consensusparamskeeper.Keeper
	AccountKeeper         authkeeper.AccountKeeper
	BankKeeper            bankkeeper.Keeper
	StakingKeeper         *stakingkeeper.Keeper
	DistributionKeeper    distrkeeper.Keeper
	CrisisKeeper          *crisiskeeper.Keeper
	UpgradeKeeper         *upgradekeeper.Keeper
	ParamsKeeper          paramskeeper.Keeper
	AuthzKeeper           authzkeeper.Keeper
	EvmKeeper             *evmkeeper.Keeper
	FeeMarketKeeper       feemarketkeeper.Keeper
	OracleKeeper          oraclekeeper.Keeper
	MarketMapKeeper       marketmapkeeper.Keeper

	// the module manager
	mm *module.Manager

	// the configurator
	configurator module.Configurator

	tpsCounter *tpsCounter

	// Connect client
	oracleClient  oracleclient.OracleClient
	oracleMetrics servicemetrics.Metrics

	preBlockHandler *appabci.PreBlockHandler
}

// NewHoodium returns a reference to a new initialized Ethermint application.
func NewHoodium(
	logger log.Logger,
	db dbm.DB,
	traceStore io.Writer,
	loadLatest bool,
	skipUpgradeHeights map[int64]bool,
	homePath string,
	invCheckPeriod uint,
	encodingConfig simappparams.EncodingConfig,
	appOpts servertypes.AppOptions,
	baseAppOptions ...func(*baseapp.BaseApp),
) *Hoodium {
	appCodec := encodingConfig.Codec
	cdc := encodingConfig.Amino
	interfaceRegistry := encodingConfig.InterfaceRegistry

	eip712.SetEncodingConfig(encodingConfig)

	// NOTE we use custom transaction decoder that supports the sdk.Tx interface instead of sdk.StdTx
	bApp := baseapp.NewBaseApp(
		Name,
		logger,
		db,
		encodingConfig.TxConfig.TxDecoder(),
		baseAppOptions...,
	)
	bApp.SetCommitMultiStoreTracer(traceStore)
	bApp.SetVersion(version.Version)
	bApp.SetInterfaceRegistry(interfaceRegistry)

	keys := storetypes.NewKVStoreKeys(
		consensusparamstypes.StoreKey,
		authtypes.StoreKey,
		banktypes.StoreKey,
		crisistypes.StoreKey,
		paramstypes.StoreKey,
		authzkeeper.StoreKey,
		upgradetypes.StoreKey,
		evmtypes.StoreKey,
		feemarkettypes.StoreKey,
		marketmaptypes.StoreKey,
		oracletypes.StoreKey,
		stakingtypes.StoreKey,
		distrtypes.StoreKey,
	)

	tkeys := storetypes.NewTransientStoreKeys(
		paramstypes.TStoreKey,
		evmtypes.TransientKey,
		feemarkettypes.TransientKey,
	)

	app := &Hoodium{
		BaseApp:           bApp,
		cdc:               cdc,
		appCodec:          appCodec,
		interfaceRegistry: interfaceRegistry,
		invCheckPeriod:    invCheckPeriod,
		keys:              keys,
		tkeys:             tkeys,
	}

	if err := app.RegisterStreamingServices(appOpts, app.keys); err != nil {
		panic(fmt.Sprintf("failed to register streaming services: %s", err))
	}

	// Most of the modules require setting a Cosmos-level authority account
	// which has privileges to perform governance actions (e.g. parameters change).
	// Upon a governance action, the modules' keepers perform a check that
	// the operation is actually executed by the authority account. This is
	// necessary to ensure proper authorization of governance actions done
	// through native Cosmos transactions. For Hoodium, the actual authority will
	// be in hands of a multi-sig account deployed on EVM. Moreover, the governance
	// actions will be exposed through dedicated precompiled EVM contracts.
	// Those precompiles will validate the authority of the caller on EVM-level
	// and will execute state updates on specific modules keepers. However,
	// given the Cosmos-level authority check in keepers, the precompiles
	// will have to impersonate the Cosmos-level authority account.
	// As the precompiles live in the context of the `x/evm` module, using
	// the account of this module as authority seems to be a natural choice.
	authority := authtypes.NewModuleAddress(evmtypes.ModuleName)

	// init params keeper and subspaces
	app.ParamsKeeper = initParamsKeeper(appCodec, cdc, keys[paramstypes.StoreKey], tkeys[paramstypes.TStoreKey])
	// init consensus params keeper
	app.ConsensusParamsKeeper = consensusparamskeeper.NewKeeper(
		appCodec,
		runtime.NewKVStoreService(keys[consensusparamstypes.StoreKey]),
		authority.String(),
		runtime.EventService{},
	)
	// set the BaseApp's parameter store
	bApp.SetParamStore(app.ConsensusParamsKeeper.ParamsStore)

	bech32Prefix := sdk.GetConfig().GetBech32AccountAddrPrefix()
	addressCodec := authcodec.NewBech32Codec(bech32Prefix)

	app.AccountKeeper = authkeeper.NewAccountKeeper(
		appCodec,
		runtime.NewKVStoreService(keys[authtypes.StoreKey]),
		runetypes.ProtoAccount,
		maccPerms,
		addressCodec,
		bech32Prefix,
		authority.String(),
	)
	app.BankKeeper = bankkeeper.NewBaseKeeper(
		appCodec,
		runtime.NewKVStoreService(keys[banktypes.StoreKey]),
		app.AccountKeeper,
		app.BlockedAddrs(),
		authority.String(),
		logger,
	)
	app.StakingKeeper = stakingkeeper.NewKeeper(
		appCodec,
		runtime.NewKVStoreService(keys[stakingtypes.StoreKey]),
		app.AccountKeeper,
		app.BankKeeper,
		authority.String(),
		authcodec.NewBech32Codec(sdk.GetConfig().GetBech32ValidatorAddrPrefix()),
		authcodec.NewBech32Codec(sdk.GetConfig().GetBech32ConsensusAddrPrefix()),
	)
	app.DistributionKeeper = distrkeeper.NewKeeper(
		appCodec,
		runtime.NewKVStoreService(keys[distrtypes.StoreKey]),
		app.AccountKeeper,
		app.BankKeeper,
		app.StakingKeeper,
		authtypes.FeeCollectorName,
		authority.String(),
	)
	// Wire distribution's reward-tracking hooks into staking. This handles
	// validator-creation/delegation reward accounting. NOTE: reward allocation
	// (proposer/block rewards) is NOT wired here — that is handled by the custom
	// x/runerewards module later (Stage 4).
	app.StakingKeeper.SetHooks(stakingtypes.NewMultiStakingHooks(app.DistributionKeeper.Hooks()))
	app.CrisisKeeper = crisiskeeper.NewKeeper(
		appCodec,
		runtime.NewKVStoreService(keys[crisistypes.StoreKey]),
		invCheckPeriod,
		app.BankKeeper,
		authtypes.FeeCollectorName,
		authority.String(),
		app.AccountKeeper.AddressCodec(),
	)
	app.UpgradeKeeper = upgradekeeper.NewKeeper(
		skipUpgradeHeights,
		runtime.NewKVStoreService(keys[upgradetypes.StoreKey]),
		appCodec,
		homePath,
		app.BaseApp,
		authority.String(),
	)

	app.AuthzKeeper = authzkeeper.NewKeeper(
		runtime.NewKVStoreService(keys[authzkeeper.StoreKey]),
		appCodec,
		app.MsgServiceRouter(),
		app.AccountKeeper,
	)

	tracer := cast.ToString(appOpts.Get(srvflags.EVMTracer))
	ethCallGasCap := cast.ToUint64(appOpts.Get(srvflags.JSONRPCGasCap))
	ethCallTimeout := cast.ToDuration(appOpts.Get(srvflags.JSONRPCEVMTimeout))
	enableJSTracers := cast.ToBool(appOpts.Get(srvflags.JSONRPCEnableJSTracers))

	app.FeeMarketKeeper = feemarketkeeper.NewKeeper(
		appCodec, authority,
		keys[feemarkettypes.StoreKey],
		tkeys[feemarkettypes.TransientKey],
		app.GetSubspace(feemarkettypes.ModuleName),
	)

	app.MarketMapKeeper = *marketmapkeeper.NewKeeper(
		runtime.NewKVStoreService(keys[marketmaptypes.StoreKey]),
		appCodec,
		authority,
	)
	app.OracleKeeper = oraclekeeper.NewKeeper(
		runtime.NewKVStoreService(keys[oracletypes.StoreKey]),
		appCodec,
		&app.MarketMapKeeper,
		authority,
	)

	app.EvmKeeper = evmkeeper.NewKeeper(
		appCodec,
		keys[evmtypes.StoreKey],
		tkeys[evmtypes.TransientKey],
		authority,
		app.AccountKeeper,
		app.BankKeeper,
		newStakingKeeperAdapter(app.StakingKeeper),
		app.FeeMarketKeeper,
		&app.ConsensusParamsKeeper,
		tracer,
		ethCallGasCap,
		ethCallTimeout,
		enableJSTracers,
		app.GetSubspace(evmtypes.ModuleName),
	)

	precompiles, err := customEvmPrecompiles(
		logger,
		app.BankKeeper,
		app.AuthzKeeper,
		*app.EvmKeeper,
		*app.UpgradeKeeper,
		oraclekeeper.NewQueryServer(app.OracleKeeper),
		app.FeeMarketKeeper,
		bApp.ChainID(),
		cast.ToBool(appOpts.Get(srvflags.EnableTestbedPrecompile)),
	)
	if err != nil {
		panic(fmt.Sprintf("failed to build custom EVM precompiles: [%s]", err))
	}
	app.EvmKeeper.RegisterCustomPrecompiles(precompiles...)

	// NOTE: we may consider parsing `appOpts` inside module constructors. For the moment
	// we prefer to be more strict in what arguments the modules expect.
	skipGenesisInvariants := cast.ToBool(appOpts.Get(crisis.FlagSkipGenesisInvariants))

	// NOTE: Any module instantiated in the module manager that is later modified
	// must be passed by reference here.
	app.mm = module.NewManager(
		consensusparams.NewAppModule(appCodec, app.ConsensusParamsKeeper),
		auth.NewAppModule(appCodec, app.AccountKeeper, authsims.RandomGenesisAccounts, app.GetSubspace(authtypes.ModuleName)),
		bank.NewAppModule(appCodec, app.BankKeeper, app.AccountKeeper, app.GetSubspace(banktypes.ModuleName)),
		crisis.NewAppModule(app.CrisisKeeper, skipGenesisInvariants, app.GetSubspace(crisistypes.ModuleName)),
		upgrade.NewAppModule(app.UpgradeKeeper, addressCodec),
		params.NewAppModule(app.ParamsKeeper),
		authzmodule.NewAppModule(appCodec, app.AuthzKeeper, app.AccountKeeper, app.BankKeeper, app.interfaceRegistry),
		evm.NewAppModule(app.EvmKeeper, app.AccountKeeper, app.GetSubspace(evmtypes.ModuleName)),
		feemarket.NewAppModule(app.FeeMarketKeeper, app.GetSubspace(feemarkettypes.ModuleName)),
		marketmap.NewAppModule(appCodec, &app.MarketMapKeeper),
		oracle.NewAppModule(appCodec, app.OracleKeeper),
		staking.NewAppModule(appCodec, app.StakingKeeper, app.AccountKeeper, app.BankKeeper, app.GetSubspace(stakingtypes.ModuleName)),
		distr.NewAppModule(appCodec, app.DistributionKeeper, app.AccountKeeper, app.BankKeeper, app.StakingKeeper, app.GetSubspace(distrtypes.ModuleName)),
		genutil.NewAppModule(app.AccountKeeper, app.StakingKeeper, app.BaseApp, encodingConfig.TxConfig),
	)

	// NOTE: upgrade module must go first to handle software upgrades.
	app.mm.SetOrderPreBlockers(
		upgradetypes.ModuleName,
	)

	app.mm.SetOrderBeginBlockers(
		feemarkettypes.ModuleName,
		evmtypes.ModuleName,
		stakingtypes.ModuleName,
		distrtypes.ModuleName,
		oracletypes.ModuleName,
		// no-op modules
		authtypes.ModuleName,
		banktypes.ModuleName,
		crisistypes.ModuleName,
		authz.ModuleName,
		paramstypes.ModuleName,
		consensusparamstypes.ModuleName,
		marketmaptypes.ModuleName,
		genutiltypes.ModuleName,
	)

	// NOTE: fee market module must go last in order to retrieve the block gas used.
	app.mm.SetOrderEndBlockers(
		crisistypes.ModuleName,
		stakingtypes.ModuleName,
		distrtypes.ModuleName,
		evmtypes.ModuleName,
		authtypes.ModuleName,
		banktypes.ModuleName,
		authz.ModuleName,
		paramstypes.ModuleName,
		upgradetypes.ModuleName,
		consensusparamstypes.ModuleName,
		marketmaptypes.ModuleName,
		oracletypes.ModuleName,
		feemarkettypes.ModuleName,
		genutiltypes.ModuleName,
	)

	// NOTE: crisis module must go at the end to check for invariants on each module
	app.mm.SetOrderInitGenesis(
		authtypes.ModuleName,
		banktypes.ModuleName,
		genutiltypes.ModuleName,
		stakingtypes.ModuleName,
		distrtypes.ModuleName,
		evmtypes.ModuleName,
		feemarkettypes.ModuleName,
		authz.ModuleName,
		paramstypes.ModuleName,
		upgradetypes.ModuleName,
		oracletypes.ModuleName,
		marketmaptypes.ModuleName,
		crisistypes.ModuleName,
		consensusparamstypes.ModuleName,
	)

	app.mm.RegisterInvariants(app.CrisisKeeper)
	app.configurator = module.NewConfigurator(app.appCodec, app.MsgServiceRouter(), app.GRPCQueryRouter())
	err = app.mm.RegisterServices(app.configurator)
	if err != nil {
		panic(err)
	}

	// initialize stores
	app.MountKVStores(keys)
	app.MountTransientStores(tkeys)

	// initialize the BaseApp with markets in state.
	app.SetInitChainer(app.InitChainer)
	app.SetPreBlocker(app.PreBlocker)
	app.SetBeginBlocker(app.BeginBlocker)

	maxGasWanted := cast.ToUint64(appOpts.Get(srvflags.EVMMaxTxGasWanted))

	app.setAnteHandler(encodingConfig.TxConfig, maxGasWanted)
	app.setPostHandler()
	app.SetEndBlocker(app.EndBlocker)

	// Set the x/marketmap keeper hooks
	app.MarketMapKeeper.SetHooks(app.OracleKeeper.Hooks())
	// oracle initialization
	app.oracleClient, app.oracleMetrics, err = app.initializeOracle(appOpts)
	if err != nil {
		panic(fmt.Sprintf("failed to initialize oracle client and metrics: %s", err))
	}
	// Connect ABCI initialization requires the oracle client/metrics to be setup first.
	app.setABCIExtensions()

	app.setupUpgradeHandlers()

	if loadLatest {
		if err := app.LoadLatestVersion(); err != nil {
			tmos.Exit(err.Error())
		}
	}

	// Finally start the tpsCounter.
	app.tpsCounter = newTPSCounter(logger)
	go func() {
		// Unfortunately golangci-lint is so pedantic,
		// so we have to ignore this error explicitly.
		_ = app.tpsCounter.start(context.Background())
	}()

	return app
}

// Name returns the name of the App
func (app *Hoodium) Name() string { return app.BaseApp.Name() }

func (app *Hoodium) setAnteHandler(txConfig client.TxConfig, maxGasWanted uint64) {
	options := ante.HandlerOptions{
		Cdc:                    app.appCodec,
		AccountKeeper:          app.AccountKeeper,
		BankKeeper:             app.BankKeeper,
		ExtensionOptionChecker: runetypes.HasDynamicFeeExtensionOption,
		EvmKeeper:              app.EvmKeeper,
		FeeMarketKeeper:        app.FeeMarketKeeper,
		SignModeHandler:        txConfig.SignModeHandler(),
		SigGasConsumer:         ante.SigVerificationGasConsumer,
		MaxTxGasWanted:         maxGasWanted,
		TxFeeChecker:           ethante.NewDynamicFeeChecker(app.EvmKeeper),
	}

	if err := options.Validate(); err != nil {
		panic(err)
	}

	app.SetAnteHandler(ante.NewAnteHandler(options))
}

func (app *Hoodium) setPostHandler() {
	postHandler, err := posthandler.NewPostHandler(
		posthandler.HandlerOptions{},
	)
	if err != nil {
		panic(err)
	}

	app.SetPostHandler(postHandler)
}

func (app *Hoodium) PreBlocker(
	ctx sdk.Context,
	req *abci.RequestFinalizeBlock,
) (*sdk.ResponsePreBlock, error) {
	return app.preBlockHandler.PreBlocker(app.mm)(ctx, req)
}

func (app *Hoodium) BeginBlocker(ctx sdk.Context) (sdk.BeginBlock, error) {
	app.beginBlockForks(ctx)
	return app.mm.BeginBlock(ctx)
}

func (app *Hoodium) EndBlocker(ctx sdk.Context) (sdk.EndBlock, error) {
	return app.mm.EndBlock(ctx)
}

// FinalizeBlock method is intentionally decomposed to calculate the
// transactions per second.
func (app *Hoodium) FinalizeBlock(req *abci.RequestFinalizeBlock) (
	res *abci.ResponseFinalizeBlock,
	err error,
) {
	defer func() {
		// Check required to not panic during res.TxResults in case the
		// upstream FinalizeBlock errors out and returns a nil response.
		if res == nil {
			return
		}

		for _, txResult := range res.TxResults {
			if txResult.IsErr() {
				app.tpsCounter.incrementFailure()
			} else {
				app.tpsCounter.incrementSuccess()
			}
		}
	}()

	return app.BaseApp.FinalizeBlock(req)
}

// InitChainer updates at chain initialization
func (app *Hoodium) InitChainer(ctx sdk.Context, req *abci.RequestInitChain) (*abci.ResponseInitChain, error) {
	var genesisState simapp.GenesisState
	if err := json.Unmarshal(req.AppStateBytes, &genesisState); err != nil {
		panic(err)
	}

	err := app.UpgradeKeeper.SetModuleVersionMap(ctx, app.mm.GetVersionMap())
	if err != nil {
		panic(err)
	}
	// Set default markets
	oracleGenState, marketmapGenState := customMarketGenesis()
	genesisState[oracletypes.ModuleName] = app.appCodec.MustMarshalJSON(oracleGenState)
	genesisState[marketmaptypes.ModuleName] = app.appCodec.MustMarshalJSON(marketmapGenState)

	return app.mm.InitGenesis(ctx, app.appCodec, genesisState)
}

// setABCIExtensions sets the ABCI++ extensions on the application.
func (app *Hoodium) setABCIExtensions() {
	// Create the Connect ABCI handlers.
	connectVEHandler, connectProposalHandler, connectPreBlocker := app.connectABCIHandlers()

	// Create and attach the app-level vote extension handler for
	// ExtendVote and VerifyVoteExtension ABCI requests.
	voteExtensionHandler := appabci.NewVoteExtensionHandler(
		app.Logger(),
		connectVEHandler,
	)
	voteExtensionHandler.SetHandlers(app.BaseApp)

	// Create and attach the app-level proposal handler for
	// PrepareProposal and ProcessProposal ABCI requests.
	proposalHandler := appabci.NewProposalHandler(
		app.Logger(),
		connectProposalHandler,
	)
	proposalHandler.SetHandlers(app.BaseApp)

	app.preBlockHandler = appabci.NewPreBlockHandler(
		app.Logger(),
		connectPreBlocker,
	)
}

// LoadHeight loads state at a particular height
func (app *Hoodium) LoadHeight(height int64) error {
	return app.LoadVersion(height)
}

// ModuleAccountAddrs returns all the app's module account addresses.
func (app *Hoodium) ModuleAccountAddrs() map[string]bool {
	modAccAddrs := make(map[string]bool)

	accs := make([]string, 0, len(maccPerms))
	for k := range maccPerms {
		accs = append(accs, k)
	}
	sort.Strings(accs)

	for _, acc := range accs {
		modAccAddrs[authtypes.NewModuleAddress(acc).String()] = true
	}

	return modAccAddrs
}

// BlockedAddrs returns all the app's module account addresses that are not
// allowed to receive external tokens.
func (app *Hoodium) BlockedAddrs() map[string]bool {
	blockedAddrs := make(map[string]bool)

	accs := make([]string, 0, len(maccPerms))
	for k := range maccPerms {
		accs = append(accs, k)
	}
	sort.Strings(accs)

	for _, acc := range accs {
		blockedAddrs[authtypes.NewModuleAddress(acc).String()] = !allowedReceivingModAcc[acc]
	}

	return blockedAddrs
}

// LegacyAmino returns Hoodium's amino codec.
//
// NOTE: This is solely to be used for testing purposes as it may be desirable
// for modules to register their own custom testing types.
func (app *Hoodium) LegacyAmino() *codec.LegacyAmino {
	return app.cdc
}

// AppCodec returns Hoodium's app codec.
//
// NOTE: This is solely to be used for testing purposes as it may be desirable
// for modules to register their own custom testing types.
func (app *Hoodium) AppCodec() codec.Codec {
	return app.appCodec
}

// InterfaceRegistry returns Hoodium's InterfaceRegistry
func (app *Hoodium) InterfaceRegistry() types.InterfaceRegistry {
	return app.interfaceRegistry
}

// GetKey returns the KVStoreKey for the provided store key.
//
// NOTE: This is solely to be used for testing purposes.
func (app *Hoodium) GetKey(storeKey string) *storetypes.KVStoreKey {
	return app.keys[storeKey]
}

// GetTKey returns the TransientStoreKey for the provided store key.
//
// NOTE: This is solely to be used for testing purposes.
func (app *Hoodium) GetTKey(storeKey string) *storetypes.TransientStoreKey {
	return app.tkeys[storeKey]
}

// GetSubspace returns a param subspace for a given module name.
//
// NOTE: This is solely to be used for testing purposes.
func (app *Hoodium) GetSubspace(moduleName string) paramstypes.Subspace {
	subspace, _ := app.ParamsKeeper.GetSubspace(moduleName)
	return subspace
}

// RegisterAPIRoutes registers all application module routes with the provided
// API server.
func (app *Hoodium) RegisterAPIRoutes(apiSvr *api.Server, apiConfig config.APIConfig) {
	clientCtx := apiSvr.ClientCtx

	// Register new tx routes from grpc-gateway.
	authtx.RegisterGRPCGatewayRoutes(clientCtx, apiSvr.GRPCGatewayRouter)
	// Register new tendermint queries routes from grpc-gateway.
	cmtservice.RegisterGRPCGatewayRoutes(clientCtx, apiSvr.GRPCGatewayRouter)
	// Register node gRPC service for grpc-gateway.
	node.RegisterGRPCGatewayRoutes(clientCtx, apiSvr.GRPCGatewayRouter)

	// Register legacy and grpc-gateway routes for all modules.
	ModuleBasics.RegisterGRPCGatewayRoutes(clientCtx, apiSvr.GRPCGatewayRouter)

	// register swagger API from root so that other applications can override easily
	if apiConfig.Swagger {
		app.Logger().Warn(
			"api.swagger config key is enabled but the mechanism is currently not supported",
		)
	}
}

func (app *Hoodium) RegisterTxService(clientCtx client.Context) {
	authtx.RegisterTxService(app.BaseApp.GRPCQueryRouter(), clientCtx, app.BaseApp.Simulate, app.interfaceRegistry)
}

// RegisterTendermintService implements the Application.RegisterTendermintService method.
func (app *Hoodium) RegisterTendermintService(clientCtx client.Context) {
	cmtservice.RegisterTendermintService(
		clientCtx,
		app.BaseApp.GRPCQueryRouter(),
		app.interfaceRegistry,
		app.Query,
	)
}

// RegisterNodeService registers the node gRPC service on the provided
// application gRPC query router.
func (app *Hoodium) RegisterNodeService(
	clientCtx client.Context,
	cfg config.Config,
) {
	node.RegisterNodeService(clientCtx, app.GRPCQueryRouter(), cfg)
}

// GetBaseApp implements the TestingApp interface.
func (app *Hoodium) GetBaseApp() *baseapp.BaseApp {
	return app.BaseApp
}

// GetTxConfig implements the TestingApp interface.
func (app *Hoodium) GetTxConfig() client.TxConfig {
	cfg := encoding.MakeConfig(ModuleBasics)
	return cfg.TxConfig
}

// initParamsKeeper init params keeper and its subspaces
func initParamsKeeper(
	appCodec codec.BinaryCodec, legacyAmino *codec.LegacyAmino, key, tkey storetypes.StoreKey,
) paramskeeper.Keeper {
	paramsKeeper := paramskeeper.NewKeeper(appCodec, legacyAmino, key, tkey)

	paramsKeeper.Subspace(authtypes.ModuleName)
	paramsKeeper.Subspace(banktypes.ModuleName)
	paramsKeeper.Subspace(crisistypes.ModuleName)
	paramsKeeper.Subspace(evmtypes.ModuleName).WithKeyTable(evmtypes.ParamKeyTable()) //nolint: staticcheck
	paramsKeeper.Subspace(feemarkettypes.ModuleName).WithKeyTable(feemarkettypes.ParamKeyTable())

	return paramsKeeper
}

// baseCustomEvmPrecompiles builds custom precompiles of the EVM module.
func customEvmPrecompiles(
	logger log.Logger,
	bankKeeper bankkeeper.Keeper,
	authzKeeper authzkeeper.Keeper,
	evmKeeper evmkeeper.Keeper,
	upgradeKeeper upgradekeeper.Keeper,
	oracleQueryServer oracletypes.QueryServer,
	feemarketKeeper feemarketkeeper.Keeper,
	chainID string,
	enableTestbedPrecompile bool,
) ([]*core.VersionMap, error) {
	// RUNE token precompile.
	runeTokenVersionMap, err := runetoken.NewPrecompileVersionMap(
		bankKeeper,
		authzKeeper,
		evmKeeper,
		chainID,
	)
	if err != nil {
		return nil, fmt.Errorf(
			"failed to create RUNE token precompile: [%w]",
			err,
		)
	}

	// HOODI token precompile. Detached from any Mainchain module; base ERC-20
	// only (no minter/mint). Reserved for the POX Yieldchain.
	hoodiTokenVersionMap, err := hooditoken.NewPrecompileVersionMap(
		bankKeeper,
		authzKeeper,
		evmKeeper,
		chainID,
	)
	if err != nil {
		return nil, fmt.Errorf(
			"failed to create HOODI token precompile: [%w]",
			err,
		)
	}

	// TODO(Stage-2): rewire these precompiles onto staking-based authority once
	// the validator/delegator staking migration is complete. The validatorpool,
	// maintenance, and upgrade precompiles were gated by PoA's CheckOwner /
	// emergency-team model, which no longer exists.
	//
	// validatorpool.NewPrecompileVersionMap(...)
	// maintenance.NewPrecompileVersionMap(...)
	// upgradelocal.NewPrecompileVersionMap(upgradeKeeper, ...)

	// Price Oracle precompile.
	priceOracleVersionMap, err := priceoracle.NewPrecompileVersionMap(oracleQueryServer)
	if err != nil {
		return nil, fmt.Errorf("failed to create price oracle precompile: [%w]", err)
	}

	pvmap := []*core.VersionMap{
		runeTokenVersionMap,
		hoodiTokenVersionMap,
		priceOracleVersionMap,
	}

	// This is  the localnet chainID, we will load this specific
	// precompile only when running system tests.
	if chainID == "rune_6591-10" && enableTestbedPrecompile {
		logger.Warn("loading testbed precompiles")

		testBedVersionMap, err := testbed.NewPrecompileVersionMap(
			bankKeeper,
			authzKeeper,
			evmKeeper,
			chainID,
		)
		if err != nil {
			return nil, fmt.Errorf(
				"failed to create testbed precompile: [%w]",
				err,
			)
		}

		pvmap = append(pvmap, testBedVersionMap)
	}

	return pvmap, nil
}
