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

package main

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/hoodium-io/hoodium/cmd/runed/genesis"
	"github.com/hoodium-io/hoodium/cmd/runed/toml"

	storetypes "cosmossdk.io/store/types"

	"github.com/cosmos/cosmos-sdk/client/config"

	"github.com/spf13/viper"

	"github.com/spf13/cast"
	"github.com/spf13/cobra"

	"cosmossdk.io/log"
	confixcmd "cosmossdk.io/tools/confix/cmd"
	tmcfg "github.com/cometbft/cometbft/config"
	tmcli "github.com/cometbft/cometbft/libs/cli"
	dbm "github.com/cosmos/cosmos-db"

	autocliv1 "cosmossdk.io/api/cosmos/autocli/v1"
	autocli "cosmossdk.io/client/v2/autocli"
	"cosmossdk.io/simapp/params"
	"cosmossdk.io/store"
	"cosmossdk.io/store/snapshots"
	snapshottypes "cosmossdk.io/store/snapshots/types"
	upgradecli "cosmossdk.io/x/upgrade/client/cli"
	"github.com/cosmos/cosmos-sdk/baseapp"
	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/client/flags"
	"github.com/cosmos/cosmos-sdk/client/pruning"
	"github.com/cosmos/cosmos-sdk/client/rpc"
	sdkserver "github.com/cosmos/cosmos-sdk/server"
	servertypes "github.com/cosmos/cosmos-sdk/server/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	auth "github.com/cosmos/cosmos-sdk/x/auth"
	authcmd "github.com/cosmos/cosmos-sdk/x/auth/client/cli"
	authcodec "github.com/cosmos/cosmos-sdk/x/auth/codec"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	authzcli "github.com/cosmos/cosmos-sdk/x/authz/client/cli"
	bank "github.com/cosmos/cosmos-sdk/x/bank"
	bankcli "github.com/cosmos/cosmos-sdk/x/bank/client/cli"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	"github.com/cosmos/cosmos-sdk/x/crisis"
	distr "github.com/cosmos/cosmos-sdk/x/distribution"
	distrcli "github.com/cosmos/cosmos-sdk/x/distribution/client/cli"
	distrtypes "github.com/cosmos/cosmos-sdk/x/distribution/types"
	slashing "github.com/cosmos/cosmos-sdk/x/slashing"
	slashingtypes "github.com/cosmos/cosmos-sdk/x/slashing/types"
	staking "github.com/cosmos/cosmos-sdk/x/staking"
	stakingcli "github.com/cosmos/cosmos-sdk/x/staking/client/cli"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
	runeclient "github.com/hoodium-io/hoodium/client"
	"github.com/hoodium-io/hoodium/encoding"
	"github.com/hoodium-io/hoodium/evm/eip712"
	runeserver "github.com/hoodium-io/hoodium/server"
	servercfg "github.com/hoodium-io/hoodium/server/config"
	srvflags "github.com/hoodium-io/hoodium/server/flags"
	evmcli "github.com/hoodium-io/hoodium/x/evm/client/cli"

	"github.com/hoodium-io/hoodium/app"
	cmdcfg "github.com/hoodium-io/hoodium/cmd/config"
	runekr "github.com/hoodium-io/hoodium/crypto/keyring"
)

const (
	EnvPrefix = "RUNED"
)

// NewRootCmd creates a new root command for runed. It is called once in the
// main function.
func NewRootCmd() (*cobra.Command, params.EncodingConfig) {
	encodingConfig := encoding.MakeConfig(app.ModuleBasics)
	initClientCtx := client.Context{}.
		WithCodec(encodingConfig.Codec).
		WithInterfaceRegistry(encodingConfig.InterfaceRegistry).
		WithTxConfig(encodingConfig.TxConfig).
		WithLegacyAmino(encodingConfig.Amino).
		WithInput(os.Stdin).
		WithAccountRetriever(authtypes.AccountRetriever{}).
		WithBroadcastMode(flags.BroadcastSync).
		WithHomeDir(app.DefaultNodeHome).
		WithKeyringOptions(runekr.Option()).
		WithViper(EnvPrefix).
		WithLedgerHasProtobuf(true)

	eip712.SetEncodingConfig(encodingConfig)

	rootCmd := &cobra.Command{
		Use:   app.Name,
		Short: "Hoodium Daemon",
		PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
			// set the default command outputs
			cmd.SetOut(cmd.OutOrStdout())
			cmd.SetErr(cmd.ErrOrStderr())

			initClientCtx, err := client.ReadPersistentCommandFlags(initClientCtx, cmd.Flags())
			if err != nil {
				return err
			}

			initClientCtx, err = config.ReadFromClientConfig(initClientCtx)
			if err != nil {
				return err
			}

			if err := client.SetCmdClientContextHandler(initClientCtx, cmd); err != nil {
				return err
			}

			// override the app and tendermint configuration
			customAppTemplate, customAppConfig := initAppConfig()
			customTMConfig := initTendermintConfig()

			return sdkserver.InterceptConfigsPreRunHandler(cmd, customAppTemplate, customAppConfig, customTMConfig)
		},
	}

	cfg := sdk.GetConfig()
	cfg.Seal()

	a := appCreator{encodingConfig}

	// Build the genesis utilities command and attach the team-only genesis
	// generator (`runed genesis init-files`) to it. It lives under `genesis`
	// rather than as a `testnet`/`mainnet` subcommand so it is never confused
	// with joining a network (`runed setup`).
	genesisCmd := genesis.NewCmd(app.ModuleBasics, encodingConfig.TxConfig, app.DefaultNodeHome)
	genesisCmd.AddCommand(NewGenesisInitCmd(app.ModuleBasics, app.DefaultNodeHome))

	rootCmd.AddCommand(
		genesisCmd,
		toml.NewCmd(),
		NewInitCmd(app.ModuleBasics),
		NewSetupCmd(setupDeps{
			mbm:               app.ModuleBasics,
			txConfig:          encodingConfig.TxConfig,
			codec:             encodingConfig.Codec,
			defaultHome:       app.DefaultNodeHome,
			validatorGasPrice: app.RecommendedValidatorGasPrice,
		}),
		tmcli.NewCompletionCmd(rootCmd, true),
		confixcmd.ConfigCommand(),
		pruning.Cmd(a.newApp, app.DefaultNodeHome),
	)

	runeserver.AddCommands(
		rootCmd,
		runeserver.NewDefaultStartOptions(a.newApp, app.DefaultNodeHome),
		a.appExport,
		addModuleInitFlags,
	)

	// add keybase, auxiliary RPC, query, and tx child commands
	rootCmd.AddCommand(
		sdkserver.StatusCommand(),
		queryCommand(encodingConfig),
		txCommand(encodingConfig),
		runeclient.KeyCommands(app.DefaultNodeHome),
	)
	rootCmd, err := srvflags.AddTxFlags(rootCmd)
	if err != nil {
		panic(err)
	}

	return rootCmd, encodingConfig
}

func addModuleInitFlags(startCmd *cobra.Command) {
	crisis.AddModuleInitFlags(startCmd)
}

func queryCommand(encodingConfig params.EncodingConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use:                        "query",
		Aliases:                    []string{"q"},
		Short:                      "Querying subcommands",
		DisableFlagParsing:         true,
		SuggestionsMinimumDistance: 2,
		RunE:                       client.ValidateCmd,
	}

	cmd.AddCommand(
		rpc.ValidatorCommand(),
		sdkserver.QueryBlockCmd(),
		authcmd.QueryTxsByEventsCmd(),
		authcmd.QueryTxCmd(),
	)

	// Module query commands.
	//
	// Two layers are needed, because SDK v0.50 split the old GetQueryCmd pattern:
	//
	//  1. AddQueryCommands enumerates ModuleBasics and calls GetQueryCmd. That
	//     still yields `query evm`, `query feemarket`, `query marketmap`,
	//     `query oracle`, `query params`, `query authz`, `query upgrade`, and
	//     `query pona` (whose commands read the module store directly, because
	//     x/pona exposes no gRPC Query service).
	//
	//  2. Writing the APP UP in queryCommand used to come from the modules we are
	//     not adding here.
	//
	// The SDK's own staking/distribution/slashing/bank no longer implement
	// GetQueryCmd AT ALL in v0.50 (their client/cli packages ship only tx.go), so
	// layer 1 silently omits them. Their query subtrees live in AutoCLIOptions and
	// are generated from the modules' gRPC Query services. Layer 2 below runs
	// autocli to materialise them.
	app.ModuleBasics.AddQueryCommands(cmd)
	cmd.PersistentFlags().String(flags.FlagChainID, "", "The network chain ID")

	// layer 2: autocli. EnhanceRootCommand only ADDS commands that are missing, so
	// anything registered above is left untouched. Failures are non-fatal: a node
	// whose gRPC surface is unreachable must still be able to run every command
	// registered in layer 1.
	if err := addAutoCLIQueryCommands(cmd, encodingConfig); err != nil {
		cmd.PrintErrf("warning: could not register autocli query commands: %v\n", err)
	}

	return cmd
}

// addAutoCLIQueryCommands materialises the SDK's autocli-generated query subtrees
// (staking, distribution, slashing, bank, …) onto queryCmd.
//
// Autocli builds each command from the module's protobuf Query service, which is
// why it can offer `query staking validator <valoper>` and `query slashing
// signing-info <consaddr>` even though those modules ship NO query CLI code in
// SDK v0.50 (their client/cli packages contain only tx.go).
//
// # Why zero-value AppModules are enough
//
// AutoCLIOptions() is pure static configuration in all four modules — it returns a
// descriptor referencing the gRPC service name and never touches keeper fields
// (verified: zero `am.` references in each autocli.go). So a zero-value
// `staking.AppModule{}` produces exactly the same options as a fully wired one.
// That matters because query commands run WITHOUT building the app, so no keeper
// instances exist at this point.
//
// # Why not ModuleBasics
//
// AutoCLIOptions is declared on AppModule, not AppModuleBasic. app.ModuleBasics
// holds zero-value AppModuleBasic structs whose only method is GetTxCmd, so
// passing it to autocli would register nothing at all — silently.
func addAutoCLIQueryCommands(queryCmd *cobra.Command, encodingConfig params.EncodingConfig) error {
	// AutoCLIOptions() is pure static data on these four modules (it never reads
	// keeper fields), so a zero-value AppModule can be asked for its options
	// directly. Call ONLY AutoCLIOptions here, never GetTxCmd: the full
	// AppModule.GetTxCmd dereferences nil codecs on a zero value and panics.
	//
	// The options are then handed to autocli as ModuleOptions, with Modules left
	// EMPTY. That is essential: EnhanceRootCommand iterates appOptions.Modules and
	// unconditionally calls GetTxCmd()/GetQueryCmd() on every entry, which would
	// re-introduce the nil-codec panic we are avoiding by not passing modules.
	sourceModules := map[string]interface{}{
		stakingtypes.ModuleName:  staking.AppModule{},
		distrtypes.ModuleName:    distr.AppModule{},
		slashingtypes.ModuleName: slashing.AppModule{},
		banktypes.ModuleName:     bank.AppModule{},
		authtypes.ModuleName:     auth.AppModule{},
	}

	moduleOptions := make(map[string]*autocliv1.ModuleOptions, len(sourceModules))
	for name, mod := range sourceModules {
		if autoCLIOpts, ok := mod.(interface {
			AutoCLIOptions() *autocliv1.ModuleOptions
		}); ok {
			if opts := autoCLIOpts.AutoCLIOptions(); opts != nil && opts.Query != nil {
				moduleOptions[name] = opts
			}
		}
	}

	if len(moduleOptions) == 0 {
		return nil
	}

	signingCtx := encodingConfig.InterfaceRegistry.SigningContext()

	opts := autocli.AppOptions{
		ModuleOptions:         moduleOptions,
		AddressCodec:          signingCtx.AddressCodec(),
		ValidatorAddressCodec: signingCtx.ValidatorAddressCodec(),
		// signing.Context exposes only the account and validator codecs, so the
		// consensus one is built the same way app.go does (app/app.go:387) from the
		// configured bech32 prefix. Autocli needs it to render consensus addresses
		// in `query slashing signing-info` output.
		ConsensusAddressCodec: authcodec.NewBech32Codec(
			sdk.GetConfig().GetBech32ConsensusAddrPrefix(),
		),
		// EnhanceRootCommand derives the proto FileResolver from
		// ClientCtx.InterfaceRegistry. Without it, builder validation fails with
		// "file resolver is required in flag builder" and NO commands are added.
		ClientCtx: client.Context{}.WithInterfaceRegistry(encodingConfig.InterfaceRegistry),
	}

	// EnhanceRootCommand looks for a "query" child on a root command, so build a
	// throwaway root. It only ADDS missing commands, leaving everything already
	// registered on queryCmd untouched.
	root := &cobra.Command{Use: app.Name}
	root.AddCommand(queryCmd)

	return opts.EnhanceRootCommand(root)
}

// txCommand builds the `tx` command tree: the auth helpers for signing and
// broadcasting, plus the module transaction commands (bank send, staking
// create-validator, distribution, authz, upgrade, evm).
//
// Without this the binary exposes no way to submit a transaction — not even a
// plain token transfer — because `srvflags.AddTxFlags` only attaches flags to
// the root command; it does not build the command tree.
//
// The module commands are constructed here with the address codecs from the
// encoding config, rather than via ModuleBasics.AddTxCommands. ModuleBasics
// holds zero-value AppModuleBasic structs whose codec fields are nil, and
// staking/distribution/authz/upgrade dereference that codec inside GetTxCmd,
// which panics. Building them from real codecs avoids that.
func txCommand(encodingConfig params.EncodingConfig) *cobra.Command {
	signingCtx := encodingConfig.InterfaceRegistry.SigningContext()
	valAddrCodec := signingCtx.ValidatorAddressCodec()
	addrCodec := signingCtx.AddressCodec()

	cmd := &cobra.Command{
		Use:                        "tx",
		Aliases:                    []string{"txs"},
		Short:                      "Transactions subcommands",
		DisableFlagParsing:         true,
		SuggestionsMinimumDistance: 2,
		RunE:                       client.ValidateCmd,
	}

	cmd.AddCommand(
		authcmd.GetSignCommand(),
		authcmd.GetSignBatchCommand(),
		authcmd.GetMultiSignCommand(),
		authcmd.GetMultiSignBatchCmd(),
		authcmd.GetValidateSignaturesCommand(),
		authcmd.GetBroadcastCommand(),
		authcmd.GetEncodeCommand(),
		authcmd.GetDecodeCommand(),
		authcmd.GetSimulateCmd(),
		bankcli.NewTxCmd(addrCodec),
		stakingcli.NewTxCmd(valAddrCodec, addrCodec),
		distrcli.NewTxCmd(valAddrCodec, addrCodec),
		authzcli.GetTxCmd(addrCodec),
		upgradecli.GetTxCmd(addrCodec),
		evmcli.GetTxCmd(),
	)
	cmd.PersistentFlags().String(flags.FlagChainID, "", "The network chain ID")

	return cmd
}

// initAppConfig helps to override default appConfig template and configs.
// return "", nil if no custom configuration is required for the application.
//
// The config itself comes from DefaultAppConfig so that a home created by
// `runed init` / `runed setup` (which never writes app.toml itself — the SDK
// creates it from this template+config) is byte-identical to one written
// directly by `runed genesis init-files`. Keeping the two in sync matters:
// a divergence made setup nodes accept zero-fee transactions.
//
// PROFILE: DefaultNodeProfile (validator-safe). At this point the root pre-run
// handler runs before the command body, so the node type is not known yet; the
// safe default is the correct fallback. `runed init` overwrites app.toml with
// the resolved profile (see --node-type), so this template value only survives
// for commands that never call init.
func initAppConfig() (string, interface{}) {
	customAppTemplate, _ := servercfg.AppConfig(cmdcfg.BaseDenom)

	srvCfg := DefaultAppConfig(cmdcfg.BaseDenom, DefaultNodeProfile)

	return customAppTemplate, *srvCfg
}

type appCreator struct {
	encCfg params.EncodingConfig
}

// newApp is an appCreator
func (a appCreator) newApp(logger log.Logger, db dbm.DB, traceStore io.Writer, appOpts servertypes.AppOptions) servertypes.Application {
	var cache storetypes.MultiStorePersistentCache

	if cast.ToBool(appOpts.Get(sdkserver.FlagInterBlockCache)) {
		cache = store.NewCommitKVStoreCacheManager()
	}

	skipUpgradeHeights := make(map[int64]bool)
	for _, h := range cast.ToIntSlice(appOpts.Get(sdkserver.FlagUnsafeSkipUpgrades)) {
		skipUpgradeHeights[int64(h)] = true
	}

	pruningOpts, err := sdkserver.GetPruningOptionsFromFlags(appOpts)
	if err != nil {
		panic(err)
	}

	home := cast.ToString(appOpts.Get(flags.FlagHome))

	snapshotDir := filepath.Join(home, "data", "snapshots")
	snapshotDB, err := dbm.NewDB("metadata", sdkserver.GetAppDBBackend(appOpts), snapshotDir)
	if err != nil {
		panic(err)
	}

	snapshotStore, err := snapshots.NewStore(snapshotDB, snapshotDir)
	if err != nil {
		panic(err)
	}

	snapshotOptions := snapshottypes.NewSnapshotOptions(
		cast.ToUint64(appOpts.Get(sdkserver.FlagStateSyncSnapshotInterval)),
		cast.ToUint32(appOpts.Get(sdkserver.FlagStateSyncSnapshotKeepRecent)),
	)

	configDir := filepath.Join(home, "config")

	chainID := cast.ToString(appOpts.Get(flags.FlagChainID))
	if len(chainID) == 0 {
		v := viper.New()
		v.AddConfigPath(configDir)
		v.SetConfigName("client")
		v.SetConfigType("toml")

		if err := v.ReadInConfig(); err != nil {
			panic(err)
		}

		clientConfig := new(config.ClientConfig)
		if err := v.Unmarshal(clientConfig); err != nil {
			panic(err)
		}

		chainID = clientConfig.ChainID
	}

	runeApp := app.NewHoodium(
		logger,
		db,
		traceStore,
		true,
		skipUpgradeHeights,
		home,
		cast.ToUint(appOpts.Get(sdkserver.FlagInvCheckPeriod)),
		a.encCfg,
		appOpts,
		baseapp.SetPruning(pruningOpts),
		baseapp.SetMinGasPrices(cast.ToString(appOpts.Get(srvflags.MinGasPrices))),
		baseapp.SetHaltHeight(cast.ToUint64(appOpts.Get(sdkserver.FlagHaltHeight))),
		baseapp.SetHaltTime(cast.ToUint64(appOpts.Get(sdkserver.FlagHaltTime))),
		baseapp.SetMinRetainBlocks(cast.ToUint64(appOpts.Get(sdkserver.FlagMinRetainBlocks))),
		baseapp.SetInterBlockCache(cache),
		baseapp.SetTrace(cast.ToBool(appOpts.Get(sdkserver.FlagTrace))),
		baseapp.SetIndexEvents(cast.ToStringSlice(appOpts.Get(sdkserver.FlagIndexEvents))),
		baseapp.SetSnapshot(snapshotStore, snapshotOptions),
		baseapp.SetIAVLCacheSize(cast.ToInt(appOpts.Get(sdkserver.FlagIAVLCacheSize))),
		baseapp.SetIAVLDisableFastNode(cast.ToBool(appOpts.Get(sdkserver.FlagDisableIAVLFastNode))),
		baseapp.SetChainID(chainID),
	)

	return runeApp
}

// appExport creates a new simapp (optionally at a given height)
// and exports state.
func (a appCreator) appExport(
	logger log.Logger,
	db dbm.DB,
	traceStore io.Writer,
	height int64,
	forZeroHeight bool,
	jailAllowedAddrs []string,
	appOpts servertypes.AppOptions,
	_ []string,
) (servertypes.ExportedApp, error) {
	var runeApp *app.Hoodium
	homePath, ok := appOpts.Get(flags.FlagHome).(string)
	if !ok || homePath == "" {
		return servertypes.ExportedApp{}, errors.New("application home not set")
	}

	chainID := cast.ToString(appOpts.Get(flags.FlagChainID))
	if len(chainID) == 0 {
		v := viper.New()
		v.AddConfigPath(filepath.Join(homePath, "config"))
		v.SetConfigName("client")
		v.SetConfigType("toml")

		if err := v.ReadInConfig(); err != nil {
			panic(err)
		}

		clientConfig := new(config.ClientConfig)
		if err := v.Unmarshal(clientConfig); err != nil {
			panic(err)
		}

		chainID = clientConfig.ChainID
	}

	if height != -1 {
		runeApp = app.NewHoodium(
			logger,
			db,
			traceStore,
			false,
			map[int64]bool{},
			"",
			uint(1),
			a.encCfg,
			appOpts,
			baseapp.SetChainID(chainID),
		)

		if err := runeApp.LoadHeight(height); err != nil {
			return servertypes.ExportedApp{}, err
		}
	} else {
		runeApp = app.NewHoodium(
			logger,
			db,
			traceStore,
			true,
			map[int64]bool{},
			"",
			uint(1),
			a.encCfg,
			appOpts,
			baseapp.SetChainID(chainID),
		)
	}

	return runeApp.ExportAppStateAndValidators(forZeroHeight, jailAllowedAddrs)
}

// initTendermintConfig helps to override default Tendermint Config values.
// return tmcfg.DefaultConfig if no custom configuration is required for the application.
func initTendermintConfig() *tmcfg.Config {
	cfg := tmcfg.DefaultConfig()
	// Hoodium target block time: 6s. This is a node-local consensus setting;
	// CometBFT removed time_iota_ms from the genesis consensus params, so block
	// time is not part of genesis.json.
	//
	// It must stay in sync with x/pona/types.BlocksPerYearAt6s (5,259,600
	// blocks/year), which sets the emission tier boundaries in block height. If
	// the two drift apart, every tier completes in the wrong wall-clock time.
	cfg.Consensus.TimeoutCommit = 6 * time.Second
	// Allow empty blocks to be produced at most every 6 seconds. Combined with
	// the 6s commit timeout this keeps the chain cadence uniform (a block every
	// ~6s) instead of blocking on empty-block intervals of 0 (no interval).
	cfg.Consensus.CreateEmptyBlocksInterval = 6 * time.Second
	// to put a higher strain on node memory, use these values:
	// cfg.P2P.MaxNumInboundPeers = 100
	// cfg.P2P.MaxNumOutboundPeers = 40

	return cfg
}
