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

// DONTCOVER

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"text/template"
	"time"

	"github.com/cosmos/cosmos-sdk/x/genutil/types"

	sdkmath "cosmossdk.io/math"

	"github.com/ethereum/go-ethereum/common"

	tmconfig "github.com/cometbft/cometbft/config"
	tmrand "github.com/cometbft/cometbft/libs/rand"
	tmtime "github.com/cometbft/cometbft/types/time"
	"github.com/spf13/cobra"

	"github.com/cosmos/cosmos-sdk/client"
	clientconfig "github.com/cosmos/cosmos-sdk/client/config"
	"github.com/cosmos/cosmos-sdk/client/flags"
	"github.com/cosmos/cosmos-sdk/crypto/keyring"
	cryptotypes "github.com/cosmos/cosmos-sdk/crypto/types"
	sdkserver "github.com/cosmos/cosmos-sdk/server"
	srvconfig "github.com/cosmos/cosmos-sdk/server/config"
	"github.com/cosmos/cosmos-sdk/testutil"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/module"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	crisistypes "github.com/cosmos/cosmos-sdk/x/crisis/types"
	distrtypes "github.com/cosmos/cosmos-sdk/x/distribution/types"
	"github.com/cosmos/cosmos-sdk/x/genutil"
	slashingtypes "github.com/cosmos/cosmos-sdk/x/slashing/types"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
	"github.com/hoodium-io/hoodium/crypto/hd"
	"github.com/hoodium-io/hoodium/server/config"
	srvflags "github.com/hoodium-io/hoodium/server/flags"
	"github.com/hoodium-io/hoodium/utils"

	runetypes "github.com/hoodium-io/hoodium/types"
	evmtypes "github.com/hoodium-io/hoodium/x/evm/types"
	ponatypes "github.com/hoodium-io/hoodium/x/pona/types"

	cmdcfg "github.com/hoodium-io/hoodium/cmd/config"
	runekr "github.com/hoodium-io/hoodium/crypto/keyring"
)

var (
	flagNodeDirPrefix     = "node-dir-prefix"
	flagNumValidators     = "val"
	flagNodeDaemonHome    = "node-daemon-home"
	flagStartingIPAddress = "starting-ip-address"
)

type initArgs struct {
	algo              string
	chainID           string
	keyringBackend    string
	minGasPrices      string
	nodeDaemonHome    string
	nodeDirPrefix     string
	numValidators     int
	homeDir           string
	startingIPAddress string
}

func addGenesisInitFlagsToCmd(cmd *cobra.Command, defaultNodeHome string) {
	cmd.Flags().String(flags.FlagHome, defaultNodeHome, "Node home directory (the node's home; in single-node mode this IS the node home)")
	cmd.Flags().Int(flagNumValidators, 1, "Number of nodes to generate. 1 (default) writes a single production node home; >1 generates a multi-node local network layout")
	cmd.Flags().String(flags.FlagChainID, "", "genesis file chain-id, if left blank will be randomly created")
	cmd.Flags().String(srvflags.MinGasPrices, fmt.Sprintf("0.0000025%s", cmdcfg.BaseDenom), "Minimum gas prices to accept for transactions; All fees in a tx must meet this minimum (e.g. 0.0000025arune)") //nolint:lll
	cmd.Flags().String(flags.FlagKeyType, string(hd.EthSecp256k1Type), "Key signing algorithm to generate keys for")
}

// NewGenesisInitCmd creates the `runed genesis init-files` command: the official
// Hoodium genesis generator.
//
// NOTE: this is a TEAM-ONLY tool. It assembles the full production genesis (RUNE
// premine, validator reward reserve, bonded pool, staking and EVM/fee-market
// params). It is deliberately NOT exposed as a `testnet`/`mainnet`/`devnet`
// subcommand so that operators never mistake it for the way to join a network —
// joining is done with `runed setup`.
func NewGenesisInitCmd(mbm module.BasicManager, defaultNodeHome string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "init-files",
		Short: "Generate the genesis and node config for a Hoodium network (TEAM ONLY)",
		Long: `init-files generates a complete, production-shaped genesis and node home for a
Hoodium network (mainnet, testnet or devnet), selected by --chain-id.

This is the official Hoodium genesis generator. It is intended for use by the
Hoodium team when launching a network. It is NOT the way to join an existing
network - use "runed setup" for that.

By default it generates a SINGLE node home in --home (default: the node's
home directory), ready to run with "runed start":

  runed genesis init-files --chain-id rune_6590-1

Passing --val N with N > 1 instead generates N separate node homes under
<home> (a local-network/Docker layout), which is NOT how a public network
is launched:

  runed genesis init-files --val 4 --home ./.localnet --starting-ip-address 192.168.10.2
`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			clientCtx, err := client.GetClientQueryContext(cmd)
			if err != nil {
				return err
			}

			serverCtx := sdkserver.GetServerContextFromCmd(cmd)

			args := initArgs{}
			args.homeDir, _ = cmd.Flags().GetString(flags.FlagHome)
			args.keyringBackend, _ = cmd.Flags().GetString(flags.FlagKeyringBackend)
			args.chainID, _ = cmd.Flags().GetString(flags.FlagChainID)
			args.minGasPrices, _ = cmd.Flags().GetString(srvflags.MinGasPrices)
			args.nodeDirPrefix, _ = cmd.Flags().GetString(flagNodeDirPrefix)
			args.nodeDaemonHome, _ = cmd.Flags().GetString(flagNodeDaemonHome)
			args.startingIPAddress, _ = cmd.Flags().GetString(flagStartingIPAddress)
			args.numValidators, _ = cmd.Flags().GetInt(flagNumValidators)
			args.algo, _ = cmd.Flags().GetString(flags.FlagKeyType)

			return initGenesisFilesForNetwork(clientCtx, cmd, serverCtx.Config, mbm, args)
		},
	}

	addGenesisInitFlagsToCmd(cmd, defaultNodeHome)
	cmd.Flags().String(flagNodeDirPrefix, "node", "Node directory prefix, used only when --val > 1 (node results in node0, node1, ...)")
	cmd.Flags().String(flagNodeDaemonHome, "runed", "Node daemon home directory name, used only when --val > 1")
	cmd.Flags().String(flagStartingIPAddress, localhost, "Starting IP address for the generated peer list; used only when --val > 1. Pass \"localhost\" for a single-host local network")
	cmd.Flags().String(flags.FlagKeyringBackend, keyring.BackendFile, "Keyring backend for the generated wallet key (file|os|test)")

	return cmd
}

const (
	nodeDirPerm = 0o755
	localhost   = "localhost"

	// UnbondingTime is the Hoodium staking unbonding period: 14 days. It is
	// written explicitly into the staking genesis params so the value is a
	// deliberate chain decision rather than an SDK default (the SDK default is
	// 21 days).
	//
	// Cosmos staking has a SINGLE unbonding param. It applies to BOTH validator
	// unbonding and delegator undelegation; the SDK offers no separate delegator
	// timeframe.
	UnbondingTime = 14 * 24 * time.Hour

	// DowntimeJailDuration is how long a validator stays jailed after missing
	// too many blocks: 24 hours. The SDK default is 10 minutes, which is far too
	// short for a real chain. This is the MAINNET value; the official
	// genesis.json is authoritative for devnet and mainnet alike.
	DowntimeJailDuration = 24 * time.Hour
)

// cleanupOnFailure removes partially generated output when generation fails.
//
// SAFETY: in single-node mode (the default) the output directory is the node's
// own home (often ~/.runed), which may already contain a live node. Removing it
// on failure would destroy operator state, so cleanup is SKIPPED in that case.
// It only ever removes a directory that this command created itself, which is
// the multi-node (val > 1) layout.
func cleanupOnFailure(homeDir string, numValidators int) {
	if numValidators <= 1 {
		// Single-node: output dir is the (possibly pre-existing) node home.
		// Never delete it.
		return
	}
	_ = os.RemoveAll(homeDir)
}

// resolveNodeDir returns the home directory for node `i` of the network being
// generated.
//
// Two layouts are supported:
//
//   - Single node (default, val == 1): the node home IS the output directory.
//     This is the production case - a team member generating a real network
//     gets a ready-to-run node home (e.g. ~/.runed), with no wrapper folders.
//
//   - Multi node (val > 1): a local-network / Docker layout, where each node
//     gets `<home>/<prefix><i>/<daemonHome>` so several node homes can be
//     generated side by side. This is NOT how a public network is launched.
func resolveNodeDir(homeDir, nodeDirPrefix, nodeDaemonHome string, i, numValidators int) string {
	if numValidators <= 1 {
		return homeDir
	}
	nodeDirName := fmt.Sprintf("%s%d", nodeDirPrefix, i)
	return filepath.Join(homeDir, nodeDirName, nodeDaemonHome)
}

// nodeDirName returns the display name for node `i`. A single node takes its
// name from the home directory so the genesis description stays meaningful,
// with any leading dot stripped: `~/.runed` yields `runed`, not `.runed`.
func nodeDirName(homeDir, nodeDirPrefix string, i, numValidators int) string {
	if numValidators <= 1 {
		return strings.TrimPrefix(filepath.Base(homeDir), ".")
	}
	return fmt.Sprintf("%s%d", nodeDirPrefix, i)
}

// initGenesisFilesForNetwork initializes the genesis and configuration files for
// the network being generated. It is the single entry point for the team-only
// `runed genesis init-files` command.
func initGenesisFilesForNetwork(
	clientCtx client.Context,
	cmd *cobra.Command,
	nodeConfig *tmconfig.Config,
	mbm module.BasicManager,
	args initArgs,
) error {
	if args.chainID == "" {
		args.chainID = fmt.Sprintf("rune_%d-1", tmrand.Int63n(9999999999999)+1)
	}

	var (
		genAccounts []authtypes.GenesisAccount
		genBalances []banktypes.Balance
	)

	genFiles := make([]string, args.numValidators)
	nodeIDs := make([]string, args.numValidators)
	valPubKeys := make([]cryptotypes.PubKey, args.numValidators)
	validators := make([]stakingtypes.Validator, args.numValidators)
	memos := make([]string, args.numValidators)
	nodeDirNames := make([]string, args.numValidators)
	nodeDirs := make([]string, args.numValidators)

	inBuf := bufio.NewReader(cmd.InOrStdin())

	for i := 0; i < args.numValidators; i++ {
		nodeDirNames[i] = nodeDirName(args.homeDir, args.nodeDirPrefix, i, args.numValidators)

		nodeDir := resolveNodeDir(
			args.homeDir, args.nodeDirPrefix, args.nodeDaemonHome, i, args.numValidators,
		)
		nodeDirs[i] = nodeDir

		nodeConfig.SetRoot(nodeDir)

		if err := os.MkdirAll(filepath.Join(nodeDir, "config"), nodeDirPerm); err != nil {
			cleanupOnFailure(args.homeDir, args.numValidators)
			return err
		}

		var err error
		nodeIDs[i], valPubKeys[i], err = genutil.InitializeNodeValidatorFiles(nodeConfig)
		if err != nil {
			cleanupOnFailure(args.homeDir, args.numValidators)
			return err
		}

		memo, err := getMemo(nodeIDs[i], i, args.startingIPAddress)
		if err != nil {
			cleanupOnFailure(args.homeDir, args.numValidators)
			return err
		}

		memos[i] = memo

		genFiles[i] = nodeConfig.GenesisFile()

		kr, err := keyring.New(
			sdk.KeyringServiceName(),
			args.keyringBackend,
			nodeDir,
			inBuf,
			clientCtx.Codec,
			runekr.Option(),
		)
		if err != nil {
			return err
		}

		keyringAlgos, _ := kr.SupportedAlgorithms()
		algo, err := keyring.NewSigningAlgoFromString(args.algo, keyringAlgos)
		if err != nil {
			return err
		}

		address, secret, err := testutil.GenerateSaveCoinKey(
			kr,
			nodeDirNames[i],
			"",
			true,
			algo,
		)
		if err != nil {
			cleanupOnFailure(args.homeDir, args.numValidators)
			return err
		}

		info := map[string]string{"secret": secret}
		infoBytes, err := json.Marshal(info)
		if err != nil {
			return err
		}

		// The mnemonic controls the premine, so write it owner-only. The
		// testutil WriteFile helper hardcodes 0644, which would leave it
		// world-readable.
		keySeedFile := filepath.Join(nodeDir, "key_seed.json")
		if err := os.WriteFile(keySeedFile, infoBytes, 0o600); err != nil {
			return err
		}

		// Per-node genesis balance.
		//
		// The genesis validator's self-delegation is funded from the premine
		// account (the initial validator's operator IS the premine owner), so no
		// separate genesis balance is emitted here. Additional validators join
		// later through their own nodes and self-fund their stake.
		//
		// NOTE: no bank balance is added for this account here; the premine
		// balance (see below) covers it. The account is still created so it can
		// sign as the genesis validator operator.
		genAccounts = append(
			genAccounts, &runetypes.EthAccount{
				BaseAccount: authtypes.NewBaseAccount(address, nil, 0, 0),
				CodeHash:    common.BytesToHash(evmtypes.EmptyCodeHash).Hex(),
			},
		)

		validator, err := stakingtypes.NewValidator(
			sdk.ValAddress(address).String(),
			valPubKeys[i],
			stakingtypes.Description{
				Moniker: nodeDirNames[i],
			},
		)
		if err != nil {
			return err
		}

		validators[i] = validator
	}

	// Persistent peers. A single node gets none: listing its own node ID makes
	// it dial itself, which CometBFT rejects and logs as an error on every
	// start. Multi-node generations peer every node with the others.
	persistentPeers := ""
	if args.numValidators > 1 {
		sort.Strings(memos)
		persistentPeers = strings.Join(memos, ",")
	}

	// Initialize config files for each validator node.
	for i := range validators {
		nodeDir := nodeDirs[i]

		nodeConfig.SetRoot(nodeDir)
		nodeConfig.Moniker = nodeDirNames[i]
		nodeConfig.RPC.ListenAddress = getRPCAddress(i, args.startingIPAddress)
		nodeConfig.P2P.ListenAddress = getP2PAddress(i, args.startingIPAddress)
		nodeConfig.RPC.PprofListenAddress = getPprofAddress(i, args.startingIPAddress)
		nodeConfig.P2P.PersistentPeers = persistentPeers
		nodeConfig.P2P.AddrBookStrict = getAddrBookStrict(args.startingIPAddress)
		nodeConfig.P2P.AllowDuplicateIP = getAllowDuplicateIP(args.startingIPAddress)
		tmconfig.WriteConfigFile(filepath.Join(nodeDir, "config/config.toml"), nodeConfig)

		appConfig := config.DefaultConfig()
		appConfig.MinGasPrices = args.minGasPrices
		appConfig.API.Enable = true
		appConfig.API.Address = getAPIAddress(i, args.startingIPAddress)
		appConfig.GRPC.Address = getGRPCAddress(i, args.startingIPAddress)
		appConfig.Telemetry.Enabled = true
		appConfig.Telemetry.PrometheusRetentionTime = 60
		appConfig.Telemetry.EnableHostnameLabel = false
		appConfig.Telemetry.GlobalLabels = [][]string{{"chain_id", args.chainID}}
		appConfig.JSONRPC.Address = getJSONRPCAddress(i, args.startingIPAddress)
		appConfig.JSONRPC.WsAddress = getJSONRPCWsAddress(i, args.startingIPAddress)
		srvconfig.WriteConfigFile(filepath.Join(nodeDir, "config/app.toml"), appConfig)

		// Write client.toml with the chain-id set. Without this, the node's
		// client.toml has an empty chain-id, and `runed start` reads it (via the
		// flag fallback in appCreator) to set baseapp's chain-id. An empty
		// chain-id breaks construction of the custom EVM precompiles (they parse
		// the chain-id for the EIP-712 domain separator).
		clientConfig := clientconfig.DefaultConfig()
		clientConfig.SetChainID(args.chainID)
		clientConfig.SetKeyringBackend(args.keyringBackend)
		if err := writeClientConfigFile(
			filepath.Join(nodeDir, "config/client.toml"), clientConfig,
		); err != nil {
			return err
		}
	}

	// ---------------------------------------------------------------------------
	// Hoodium genesis allocation
	//
	//   RUNE:  9,000,000,000 premine  -> genAccounts[0] (the genesis validator)
	//          1,000,000,000 reserve  -> validator_reward_pool (x/pona)
	//   HOODI: 10,000,000    premine  -> genAccounts[0] (the genesis validator)
	//
	// RUNE total  = 10,000,000,000 (== RUNE max supply cap)
	// HOODI total = 10,000,000     (== HOODI max supply cap)
	//
	// There are NO hardcoded/pre-computed addresses here: both destinations are
	// derived at runtime, so no source edit or recompile is needed.
	//
	//   * genAccounts[0] is the first account created by `keys add` during
	//     `runed genesis init-files`, so the premine lands on the operator's own
	//     key automatically.
	//
	//   * validator_reward_pool is a Cosmos *module account*: its address is
	//     always ripemd160(sha256("validator_reward_pool")) =
	//     0x4967715BC2Ba5CC2A262054A5a893e2E90FCD3f9
	//     (bech32 rune1f9nhzk7zhfwv9gnzq4994zf796g0e5lej732lk). It is created by
	//     the auth module from the name alone - nothing to configure, and it is
	//     NOT a precompile.
	//
	// NOTE: initial validator self-delegations are funded MANUALLY by the chain
	// operator (not from these premine balances). A genesis EVM address that
	// receives RUNE here is the SAME account as its rune1... bech32 twin
	// (sdk.AccAddress(evmAddr.Bytes()) == the cosmos address), so the premine is
	// usable directly from an EVM wallet.
	// ---------------------------------------------------------------------------
	oneRuneGenesis := sdkmath.NewIntWithDecimal(1, 18)

	// Premine destination: the genesis validator's own wallet account.
	//
	// Both premines (RUNE and HOODI) are placed on the first account created by
	// `keys add` / `init` (see genAccounts below), so the operator running
	// `runed genesis init-files` controls the premine by definition. Additional
	// validators join later and self-fund their stake from their own accounts.
	//
	// To control the premine from an EVM wallet, the operator copies the wallet's
	// 0x private key (runed keys unsafe-export-eth-key <key>) into the EVM wallet.

	// RUNE total is capped at 10,000,000,000 (9B premine + 1B reserve).
	//
	// The genesis validator's self-delegation (500,000 RUNE) is bonded out of the
	// premine owner's 9B balance: the bonded amount is moved into the
	// bonded_tokens_pool, so the wallet's liquid premine is reduced accordingly.
	// Totals: (9B - bonded) wallet + bonded pool + 1B reserve = exactly 10B.
	bondedTokensTotal := oneRuneGenesis.MulRaw(500_000).MulRaw(int64(args.numValidators))
	runePremineAmount := oneRuneGenesis.MulRaw(9_000_000_000).Sub(bondedTokensTotal) // 9B - bonded
	runeReserveAmount := oneRuneGenesis.MulRaw(1_000_000_000)                        // 1B RUNE (validator_reward_pool)
	hoodiPremineAmount := oneRuneGenesis.MulRaw(10_000_000)                          // 10M HOODI

	rewardPoolAddr := authtypes.NewModuleAddress(ponatypes.ValidatorRewardPoolName)

	genBalances = append(genBalances,
		// RUNE premine + HOODI premine -> first genesis validator's wallet (the
		// account created by keys add / init). Its operator is the premine owner.
		// The RUNE balance also backs that validator's 500,000 RUNE self-delegation.
		// The operator can copy this wallet's 0x private key
		// (runed keys unsafe-export-eth-key) into an EVM wallet to control the premine.
		//
		// NOTE: the bank genesis allows ONE balance entry per address, so both
		// denominations are combined into a single Balance here.
		banktypes.Balance{
			Address: genAccounts[0].GetAddress().String(),
			Coins: sdk.NewCoins(
				sdk.NewCoin(cmdcfg.BaseDenom, runePremineAmount),
				sdk.NewCoin(utils.HoodiDenom, hoodiPremineAmount),
			).Sort(),
		},
		// RUNE staking reserve -> validator_reward_pool (funds x/pona).
		banktypes.Balance{
			Address: rewardPoolAddr.String(),
			Coins:   sdk.NewCoins(sdk.NewCoin(cmdcfg.BaseDenom, runeReserveAmount)),
		},
	)

	// Fund the staking bonded_tokens_pool.
	//
	// Staking InitGenesis asserts `bonded_pool_balance == sum(bonded validator
	// tokens)`; every genesis validator is bonded with a self-delegation of
	// minSelfDelegation. Without this balance the chain panics at InitChain with
	// "bonded pool balance is different from bonded coins". This mirrors what the
	// app test helpers do when they build a staking genesis.
	bondedPoolAddr := authtypes.NewModuleAddress(stakingtypes.BondedPoolName)
	genBalances = append(genBalances, banktypes.Balance{
		Address: bondedPoolAddr.String(),
		Coins:   sdk.NewCoins(sdk.NewCoin(cmdcfg.BaseDenom, bondedTokensTotal)),
	})

	if err := initGenesisFiles(
		clientCtx,
		mbm,
		args.chainID,
		cmdcfg.BaseDenom,
		genAccounts,
		genBalances,
		genFiles,
		validators,
	); err != nil {
		return err
	}

	// Report what was generated, tailored to the layout.
	if args.numValidators <= 1 {
		cmd.PrintErrf(
			"Successfully initialized genesis and node config in %s\n",
			args.homeDir,
		)
	} else {
		cmd.PrintErrf(
			"Successfully initialized %d node directories under %s\n",
			args.numValidators, args.homeDir,
		)
	}

	return nil
}

func initGenesisFiles(
	clientCtx client.Context,
	mbm module.BasicManager,
	chainID,
	coinDenom string,
	genAccounts []authtypes.GenesisAccount,
	genBalances []banktypes.Balance,
	genFiles []string,
	validators []stakingtypes.Validator,
) error {
	appGenState := mbm.DefaultGenesis(clientCtx.Codec)
	// set the accounts in the genesis state
	var authGenState authtypes.GenesisState
	clientCtx.Codec.MustUnmarshalJSON(appGenState[authtypes.ModuleName], &authGenState)

	accounts, err := authtypes.PackAccounts(genAccounts)
	if err != nil {
		return err
	}
	authGenState.Accounts = accounts
	appGenState[authtypes.ModuleName] = clientCtx.Codec.MustMarshalJSON(&authGenState)

	// set the balances in the genesis state
	var bankGenState banktypes.GenesisState
	clientCtx.Codec.MustUnmarshalJSON(appGenState[banktypes.ModuleName], &bankGenState)
	bankGenState.Balances = genBalances

	// Set the total supply explicitly so the genesis is self-documenting and
	// auditable. The bank module derives the supply from balances if this is
	// empty, but writing it makes the 10B RUNE / 10M HOODI cap visible in the
	// genesis file. Values MUST equal the sum of genBalances per denom.
	supplyMap := sdk.NewMapCoins(sdk.Coins{})
	for _, balance := range genBalances {
		supplyMap.Add(balance.Coins...)
	}
	bankGenState.Supply = supplyMap.ToCoins()

	appGenState[banktypes.ModuleName] = clientCtx.Codec.MustMarshalJSON(&bankGenState)

	var crisisGenState crisistypes.GenesisState
	clientCtx.Codec.MustUnmarshalJSON(appGenState[crisistypes.ModuleName], &crisisGenState)
	// Anti-spam fee for MsgVerifyInvariant. The SDK default is 1000; raised so a
	// manual invariant check is not free to spam. Denom must be the chain denom.
	crisisGenState.ConstantFee = sdk.NewCoin(coinDenom, sdkmath.NewInt(10_000))
	appGenState[crisistypes.ModuleName] = clientCtx.Codec.MustMarshalJSON(&crisisGenState)

	var distrGenState distrtypes.GenesisState
	clientCtx.Codec.MustUnmarshalJSON(appGenState[distrtypes.ModuleName], &distrGenState)
	// Community tax is set to 0 so no share of collected fees is diverted to the
	// community pool. The pool cannot be spent on Hoodium: spending requires
	// MsgCommunityPoolSpend signed by the authority, and the authority here is
	// the x/evm module account while governance is not wired. Any non-zero tax
	// would accumulate unreachable funds.
	distrGenState.Params.CommunityTax = sdkmath.LegacyZeroDec()
	appGenState[distrtypes.ModuleName] = clientCtx.Codec.MustMarshalJSON(&distrGenState)

	var slashingGenState slashingtypes.GenesisState
	clientCtx.Codec.MustUnmarshalJSON(appGenState[slashingtypes.ModuleName], &slashingGenState)
	// Hoodium slashing parameters. Set explicitly rather than relying on the
	// SDK defaults, which are tuned for a 1s-block chain.
	//
	//   - SlashFractionDoubleSign = 10% (SDK default 5%): equivocation is the one
	//     unrecoverable fault, so the penalty is doubled.
	//   - DowntimeJailDuration = 24h (SDK default 10 min): long enough to matter,
	//     short enough that a failed restart is recoverable within a day.
	//   - SignedBlocksWindow / MinSignedPerWindow = 10000 / 0.50:
	//     ~16.7h window at 6s blocks, requiring 50% liveness,
	//     i.e. ~8.3h of downtime tolerated before jailing. The window is PER
	//     VALIDATOR (each tracks its own signed-block bitmap).
	//   - SlashFractionDowntime = 1% (SDK default): small, since the real
	//     deterrent for downtime is the jail itself.
	//
	// Slashed funds are burned (removed from supply), the SDK default.
	// See docs/features/slashing.md for the supply implications.
	slashingGenState.Params = slashingtypes.Params{
		SignedBlocksWindow:      10_000,
		MinSignedPerWindow:      sdkmath.LegacyMustNewDecFromStr("0.50"),
		DowntimeJailDuration:    DowntimeJailDuration,
		SlashFractionDoubleSign: sdkmath.LegacyMustNewDecFromStr("0.10"),
		SlashFractionDowntime:   sdkmath.LegacyMustNewDecFromStr("0.01"),
	}
	appGenState[slashingtypes.ModuleName] = clientCtx.Codec.MustMarshalJSON(&slashingGenState)

	var evmGenState evmtypes.GenesisState
	clientCtx.Codec.MustUnmarshalJSON(appGenState[evmtypes.ModuleName], &evmGenState)
	evmGenState.Params.EvmDenom = coinDenom
	appGenState[evmtypes.ModuleName] = clientCtx.Codec.MustMarshalJSON(&evmGenState)

	// Build the staking genesis: bond each validator with a self-delegation.
	stakingGenState := stakingtypes.DefaultGenesisState()
	stakingGenState.Params.BondDenom = coinDenom

	// Hoodium staking parameters.
	//
	//   - UnbondingTime = 14 days. Set explicitly so the value is a deliberate
	//     chain decision rather than an inherited SDK default (21 days). Cosmos
	//     staking has a single `UnbondingTime` param applying to BOTH validator
	//     unbonding and delegator undelegation; there is no separate delegator
	//     timeframe at the params level.
	//   - MaxValidators = 101 (fresh start; raise later via governance).
	//   - MinCommissionRate = 5%.
	//
	// The SDK has no global "min self-delegation" or "min delegation" param.
	// MinSelfDelegation is a PER-VALIDATOR field (set below on each validator);
	// the Hoodium 0.1 RUNE minimum delegation is enforced in the staking
	// precompile (core/staking), the entry point delegation flows through.
	oneRune := sdkmath.NewIntWithDecimal(1, 18)
	minSelfDelegation := oneRune.MulRaw(500_000) // 500,000 RUNE
	stakingGenState.Params.UnbondingTime = UnbondingTime
	stakingGenState.Params.MaxValidators = 101
	stakingGenState.Params.MinCommissionRate = sdkmath.LegacyMustNewDecFromStr("0.05")
	// MaxEntries caps the OUTSTANDING unbonding/redelegation entries per pair:
	//   - (delegator, validator)              -> pending unbonding entries
	//   - (delegator, srcVal, dstVal)         -> pending redelegation entries
	// SDK default is 7; Hoodium uses 10 to leave headroom for programmatic
	// delegators (dApps / wallets that undelegate in several chunks). Each entry
	// is a queued record the chain must settle, so the cap still prevents spam.
	stakingGenState.Params.MaxEntries = 10

	// Genesis validator commission.
	//
	// NewValidator zeroes all three rates, and a zero MaxRate/MaxChangeRate makes
	// the commission permanently immutable: ValidateNewRate rejects any rate
	// above MaxRate, so the validator could never raise it afterwards. These
	// match the values documented for joining validators in docs/validator.md.
	//
	//   - rate            = 5%  (the chain minimum set by MinCommissionRate above)
	//   - max_rate        = 25% (headroom to raise later)
	//   - max_change_rate = 1%  per change (the SDK also enforces 24h spacing)
	//
	// NewCommission leaves UpdateTime at the zero sentinel (1970-01-01T00:00:00Z),
	// meaning "never changed". Setting a real time here would make the 24h guard
	// block the validator's first commission change until a day after genesis.
	genesisCommission := stakingtypes.NewCommission(
		sdkmath.LegacyMustNewDecFromStr("0.05"),
		sdkmath.LegacyMustNewDecFromStr("0.25"),
		sdkmath.LegacyMustNewDecFromStr("0.01"),
	)

	var (
		genDelegations []stakingtypes.Delegation
		lastPowers     []stakingtypes.LastValidatorPower
		lastTotalPower int64
	)
	for i := range validators {
		// Each validator self-delegates its MinSelfDelegation (500,000 RUNE) so
		// the genesis is valid (self-delegation must be >= MinSelfDelegation) and
		// the validator has consensus power.
		tokens := minSelfDelegation
		val := validators[i]
		val.Status = stakingtypes.Bonded
		val.Tokens = tokens
		val.DelegatorShares = sdkmath.LegacyNewDecFromInt(tokens)
		val.MinSelfDelegation = minSelfDelegation
		val.Commission = genesisCommission

		validators[i] = val

		// Self-delegation: the DELEGATOR is the validator's account (acc) address,
		// while the VALIDATOR address is the valoper. The SDK decodes
		// DelegatorAddress with the account address codec, so it MUST be an acc
		// bech32 address (not valoper).
		delegator := genAccounts[i].GetAddress().String()
		operator := val.OperatorAddress

		genDelegations = append(genDelegations, stakingtypes.Delegation{
			DelegatorAddress: delegator,
			ValidatorAddress: operator,
			Shares:           sdkmath.LegacyNewDecFromInt(tokens),
		})

		power := val.GetConsensusPower(runetypes.PowerReduction)
		lastPowers = append(lastPowers, stakingtypes.LastValidatorPower{
			Address: operator,
			Power:   power,
		})
		lastTotalPower += power
	}

	stakingGenState.Validators = validators
	stakingGenState.Delegations = genDelegations
	stakingGenState.LastValidatorPowers = lastPowers
	stakingGenState.LastTotalPower = sdkmath.NewInt(lastTotalPower)
	appGenState[stakingtypes.ModuleName] = clientCtx.Codec.MustMarshalJSON(stakingGenState)

	if err := mbm.ValidateGenesis(clientCtx.Codec, clientCtx.TxConfig, appGenState); err != nil {
		return fmt.Errorf("failed to validate genesis file: %w", err)
	}

	// Drop fields that carry no information for a genesis that has never run.
	// ProtoMarshalJSON sets EmitDefaults, so the SDK always writes them; removing
	// them here (rather than editing the file afterwards) keeps the output
	// reproducible across runs.
	//
	//   - distribution: the deprecated proposer-reward fields. They are inert —
	//     permanently zero, and MsgUpdateParams rejects any non-zero value.
	//   - staking: the validator `unbonding_time` and `commission.update_time`
	//     sentinels. Both are meaningless before the chain has produced a block:
	//     the validator is bonded (never unbonding), and its commission has never
	//     been changed. The SDK writes them as Unix epoch 0 (1970), which reads
	//     as a real date and invites confusion. Absent, they decode to the Go
	//     zero time and are overwritten on first use anyway.
	//
	// The genesis loader accepts absence: jsonpb only rejects *unknown* fields,
	// and both field types decode cleanly to their zero values.
	if err := omitInertGenesisFields(appGenState); err != nil {
		return err
	}

	appGenStateJSON, err := json.MarshalIndent(appGenState, "", "  ")
	if err != nil {
		return err
	}

	// generate genesis files for each validator and save
	genTime := tmtime.Now()
	for i := range validators {
		genFile := genFiles[i]

		// Create the genesis.json file.
		if err := genutil.ExportGenesisFileWithTime(
			genFile,
			chainID,
			nil,
			appGenStateJSON,
			genTime,
		); err != nil {
			return err
		}

		// Load the genesis.json file and update to update the consensus params.
		appGenesis, err := types.AppGenesisFromFile(genFile)
		if err != nil {
			return err
		}
		// app_version is left as stamped by the build (version.Version, from the
		// Makefile's `git describe --tags --always`), so the genesis records
		// which build produced it. It has no consensus effect: ToGenesisDoc()
		// drops AppName/AppVersion, so CometBFT never reads it.
		// Set the block gas limit to 10M.
		appGenesis.Consensus.Params.Block.MaxGas = 10_000_000
		// Enable vote extensions from block 1.
		appGenesis.Consensus.Params.ABCI.VoteExtensionsEnableHeight = 1
		// Export the updated genesis file.
		if err := genutil.ExportGenesisFile(appGenesis, genFile); err != nil {
			return err
		}
	}
	return nil
}

// omitInertGenesisFields removes keys from the marshalled app state that carry
// no information for a genesis which has never run.
//
// Two groups are removed:
//
//   - distribution params: the deprecated proposer-reward fields. They are
//     permanently zero and MsgUpdateParams rejects any non-zero value.
//   - staking validators: the `unbonding_time` and `commission.update_time`
//     sentinels. A genesis validator is bonded (never unbonding) and its
//     commission has never been changed, so both are placeholders. The SDK
//     writes them as Unix epoch 0, which reads as a real date.
//
// The modules never read these values at genesis — staking's ValidateGenesis
// checks only consensus-key duplicates, jailed-and-bonded, and delegator shares,
// and its InitGenesis consults unbonding_time only for validators already
// unbonding. Omitted, each decodes to its type's zero value, and both are
// overwritten the first time they become meaningful.
func omitInertGenesisFields(appGenState map[string]json.RawMessage) error {
	if err := omitKeys(
		appGenState,
		distrtypes.ModuleName,
		[]string{"params"},
		"base_proposer_reward",
		"bonus_proposer_reward",
	); err != nil {
		return err
	}

	// Each validator entry: the unbonding and commission sentinels live at
	// different depths, so they are removed as two passes over the same array.
	if err := omitKeys(
		appGenState,
		stakingtypes.ModuleName,
		[]string{"validators"},
		"unbonding_time",
	); err != nil {
		return err
	}

	return omitKeys(
		appGenState,
		stakingtypes.ModuleName,
		[]string{"validators", "commission"},
		"update_time",
	)
}

// omitKeys deletes the given key paths from a module's JSON object in appGenState.
//
// `path` is the chain of nested object keys leading to the target object (e.g.
// ["params"], or ["validators"]). A path segment may address a JSON array, in
// which case the leaf keys are deleted from every element — which is how the
// per-validator staking fields are reached.
//
// Missing modules, paths, or keys are not an error: the genesis simply does not
// contain them.
func omitKeys(appGenState map[string]json.RawMessage, module string, path []string, keys ...string) error {
	raw, ok := appGenState[module]
	if !ok {
		return nil
	}

	var moduleState map[string]json.RawMessage
	if err := json.Unmarshal(raw, &moduleState); err != nil {
		return fmt.Errorf("failed to decode %s genesis: %w", module, err)
	}

	if len(path) == 0 {
		return nil
	}

	encoded, err := omitFromRaw(moduleState[path[0]], path[1:], keys)
	if err != nil {
		return fmt.Errorf("failed to process %s.%s: %w", module, path[0], err)
	}
	if encoded == nil {
		return nil
	}
	moduleState[path[0]] = encoded

	encodedModule, err := json.Marshal(moduleState)
	if err != nil {
		return fmt.Errorf("failed to encode %s genesis: %w", module, err)
	}
	appGenState[module] = encodedModule

	return nil
}

// omitFromRaw removes keys from a raw JSON value, descending the remaining path.
// It returns nil when the value is absent or is not the expected JSON kind, so
// callers leave the genesis untouched.
func omitFromRaw(raw json.RawMessage, path []string, keys []string) (json.RawMessage, error) {
	if len(raw) == 0 {
		return nil, nil
	}

	trimmed := bytes.TrimLeft(raw, " \t\r\n")
	if len(trimmed) == 0 {
		return nil, nil
	}

	// Arrays: apply the deletion to every element.
	if trimmed[0] == '[' {
		var items []json.RawMessage
		if err := json.Unmarshal(raw, &items); err != nil {
			return nil, err
		}
		for i := range items {
			item, err := omitFromRaw(items[i], path, keys)
			if err != nil {
				return nil, err
			}
			if item != nil {
				items[i] = item
			}
		}
		return json.Marshal(items)
	}

	// Objects: descend, or delete the leaf keys at the end of the path.
	if trimmed[0] != '{' {
		return nil, nil
	}

	var object map[string]json.RawMessage
	if err := json.Unmarshal(raw, &object); err != nil {
		return nil, err
	}

	if len(path) == 0 {
		for _, key := range keys {
			delete(object, key)
		}
		return json.Marshal(object)
	}

	child, err := omitFromRaw(object[path[0]], path[1:], keys)
	if err != nil {
		return nil, err
	}
	if child == nil {
		return nil, nil
	}
	object[path[0]] = child

	return json.Marshal(object)
}

func getMemo(nodeID string, i int, startingIPAddr string) (string, error) {
	if startingIPAddr == localhost {
		return fmt.Sprintf("%s@localhost:%d", nodeID, 26656+2*i), nil
	}

	ip, err := getIP(i, startingIPAddr)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("%s@%s:26656", nodeID, ip), nil
}

func getRPCAddress(i int, startingIPAddr string) string {
	port := 26657

	if startingIPAddr == localhost {
		port += 2 * i
	}

	return fmt.Sprintf("tcp://0.0.0.0:%d", port)
}

func getP2PAddress(i int, startingIPAddr string) string {
	port := 26656

	if startingIPAddr == localhost {
		port += 2 * i
	}

	return fmt.Sprintf("tcp://0.0.0.0:%d", port)
}

func getPprofAddress(i int, startingIPAddr string) string {
	port := 6060

	if startingIPAddr == localhost {
		port += i
	}

	return fmt.Sprintf("localhost:%d", port)
}

func getAPIAddress(i int, startingIPAddr string) string {
	port := 1317

	if startingIPAddr == localhost {
		port += i
	}

	return fmt.Sprintf("tcp://0.0.0.0:%d", port)
}

func getGRPCAddress(i int, startingIPAddr string) string {
	port := 9090

	if startingIPAddr == localhost {
		port += 2 * i
	}

	return fmt.Sprintf("0.0.0.0:%d", port)
}

func getJSONRPCAddress(i int, startingIPAddr string) string {
	port := 8545

	if startingIPAddr == localhost {
		port += 2 * i
	}

	return fmt.Sprintf("0.0.0.0:%d", port)
}

func getJSONRPCWsAddress(i int, startingIPAddr string) string {
	port := 8546

	if startingIPAddr == localhost {
		port += 2 * i
	}

	return fmt.Sprintf("0.0.0.0:%d", port)
}

func getAddrBookStrict(startingIPAddr string) bool {
	return startingIPAddr != localhost
}

func getAllowDuplicateIP(startingIPAddr string) bool {
	return startingIPAddr == localhost
}

func getIP(i int, startingIPAddr string) (ip string, err error) {
	if len(startingIPAddr) == 0 {
		ip, err = sdkserver.ExternalIP()
		if err != nil {
			return "", err
		}
		return ip, nil
	}
	return calculateIP(startingIPAddr, i)
}

func calculateIP(ip string, i int) (string, error) {
	ipv4 := net.ParseIP(ip).To4()
	if ipv4 == nil {
		return "", fmt.Errorf("%v: non ipv4 address", ip)
	}

	for j := 0; j < i; j++ {
		ipv4[3]++
	}

	return ipv4.String(), nil
}

// clientConfigTemplate mirrors the Cosmos SDK's client.toml template so the file
// we write is byte-compatible with what the SDK expects to read.
const clientConfigTemplate = `# This is a TOML config file.
# For more information, see https://github.com/toml-lang/toml

###############################################################################
###                           Client Configuration                            ###
###############################################################################

# The network chain ID
chain-id = "{{ .ChainID }}"
# The keyring's backend, where the keys are stored (os|file|kwallet|pass|test|memory)
keyring-backend = "{{ .KeyringBackend }}"
# CLI output format (text|json)
output = "{{ .Output }}"
# <host>:<port> to CometBFT RPC interface for this chain
node = "{{ .Node }}"
# Transaction broadcasting mode (sync|async)
broadcast-mode = "{{ .BroadcastMode }}"
`

// writeClientConfigFile renders the client.toml template with the given config
// and writes it to configFilePath.
func writeClientConfigFile(configFilePath string, config *clientconfig.ClientConfig) error {
	tmpl, err := template.New("clientConfigFileTemplate").Parse(clientConfigTemplate)
	if err != nil {
		return err
	}

	var buffer bytes.Buffer
	if err := tmpl.Execute(&buffer, config); err != nil {
		return err
	}

	return os.WriteFile(configFilePath, buffer.Bytes(), 0o600)
}
