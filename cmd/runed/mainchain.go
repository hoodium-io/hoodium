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
	"github.com/cosmos/cosmos-sdk/x/genutil"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
	"github.com/hoodium-io/hoodium/crypto/hd"
	"github.com/hoodium-io/hoodium/server/config"
	srvflags "github.com/hoodium-io/hoodium/server/flags"
	"github.com/hoodium-io/hoodium/utils"

	runetypes "github.com/hoodium-io/hoodium/types"
	evmtypes "github.com/hoodium-io/hoodium/x/evm/types"
	runerewardstypes "github.com/hoodium-io/hoodium/x/runerewards/types"

	cmdcfg "github.com/hoodium-io/hoodium/cmd/config"
	runekr "github.com/hoodium-io/hoodium/crypto/keyring"
	"github.com/hoodium-io/hoodium/testutil/network"
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
// joining is done with `runed setup`. See docs/team/mainchain/genesis-init.md.
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
	cmd.Flags().String(flags.FlagKeyringBackend, flags.DefaultKeyringBackend, "Select keyring's backend (os|file|test)")

	return cmd
}

const (
	nodeDirPerm = 0o755
	localhost   = "localhost"

	// UnbondingTime is the Hoodium staking unbonding period (user-confirmed):
	// 14 days. It is written explicitly into the staking genesis params so the
	// value is a deliberate chain decision rather than an SDK default (the SDK
	// default is 21 days).
	//
	// NOTE: Cosmos staking has a SINGLE unbonding param. It applies to BOTH
	// validator unbonding and delegator undelegation - the SDK offers no
	// separate delegator timeframe.
	UnbondingTime = 14 * 24 * time.Hour
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

// nodeDirName returns the display name for node `i`. For a single node this is
// the node's moniker-ish name derived from the home directory so the genesis
// description is still meaningful.
func nodeDirName(homeDir, nodeDirPrefix string, i, numValidators int) string {
	if numValidators <= 1 {
		return filepath.Base(homeDir)
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

		// save private key seed words
		if err := network.WriteFile(
			fmt.Sprintf("%v.json", "key_seed"),
			nodeDir,
			infoBytes,
		); err != nil {
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

	// Build persistent peers list based on validators' memos.
	sort.Strings(memos)
	persistentPeers := strings.Join(memos, ",")

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
	//          1,000,000,000 reserve  -> validator_reward_pool (x/runerewards)
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

	rewardPoolAddr := authtypes.NewModuleAddress(runerewardstypes.ValidatorRewardPoolName)

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
		// RUNE staking reserve -> validator_reward_pool (funds x/runerewards).
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
	crisisGenState.ConstantFee.Denom = coinDenom
	appGenState[crisistypes.ModuleName] = clientCtx.Codec.MustMarshalJSON(&crisisGenState)

	var evmGenState evmtypes.GenesisState
	clientCtx.Codec.MustUnmarshalJSON(appGenState[evmtypes.ModuleName], &evmGenState)
	evmGenState.Params.EvmDenom = coinDenom
	appGenState[evmtypes.ModuleName] = clientCtx.Codec.MustMarshalJSON(&evmGenState)

	// Build the staking genesis: bond each validator with a self-delegation.
	stakingGenState := stakingtypes.DefaultGenesisState()
	stakingGenState.Params.BondDenom = coinDenom

	// Hoodium staking parameters (user-confirmed).
	//
	//   - UnbondingTime = 14 days. Set EXPLICITLY so the value is a deliberate
	//     chain decision rather than an inherited SDK default (the SDK default is
	//     21 days). NOTE: Cosmos staking has a single `UnbondingTime` param that
	//     applies to BOTH validator unbonding and delegator undelegation - there
	//     is no separate delegator timeframe at the params level.
	//   - MaxValidators = 101 (fresh start; raise later via governance).
	//   - MinCommissionRate = 5%.
	//
	// NOTE: the SDK has NO global "min self-delegation" or "min delegation"
	// params. MinSelfDelegation is a PER-VALIDATOR field (set below on each
	// validator); the Hoodium 0.1 RUNE minimum delegation is enforced at the
	// msg/precompile level (separate task).
	oneRune := sdkmath.NewIntWithDecimal(1, 18)
	minSelfDelegation := oneRune.MulRaw(500_000) // 500,000 RUNE
	stakingGenState.Params.UnbondingTime = UnbondingTime
	stakingGenState.Params.MaxValidators = 101
	stakingGenState.Params.MinCommissionRate = sdkmath.LegacyMustNewDecFromStr("0.05")

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
