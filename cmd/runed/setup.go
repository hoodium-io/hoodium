package main

import (
	"bufio"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/client/flags"
	"github.com/cosmos/cosmos-sdk/client/input"
	"github.com/cosmos/cosmos-sdk/codec/address"
	"github.com/cosmos/cosmos-sdk/types/module"
	genutilcli "github.com/cosmos/cosmos-sdk/x/genutil/client/cli"
	genutiltypes "github.com/cosmos/cosmos-sdk/x/genutil/types"

	runeclient "github.com/hoodium-io/hoodium/client"
	"github.com/hoodium-io/hoodium/utils"
)

// Node types selectable in the guided setup.
const (
	nodeTypeValidator = "validator"
	nodeTypeSeed      = "seed"
)

// Setup modes: automated actually runs the bootstrap commands; manual prints them.
const (
	setupModeAutomated = "automated"
	setupModeManual    = "manual"
)

// setupNetworkChainID maps a friendly network name to its chain-id.
func setupNetworkChainID(network string) (string, bool) {
	switch strings.ToLower(network) {
	case "mainnet":
		return utils.MainnetChainID + "-1", true
	case "testnet":
		return utils.TestnetChainID + "-1", true
	case "devnet":
		return utils.DevnetChainID + "-1", true
	default:
		return "", false
	}
}

const SetupCmdLong = `Guided first-run setup for a Hoodium node.

This command walks you through the whole bootstrap interactively:

  1. Choose the mode          (automated | manual)
  2. Choose the network       (mainnet | testnet | devnet)
  3. Choose the node type     (validator | seed)
  4. Initialize the node      (genesis + config + node keys)
  5. Create your wallet key   (the account that holds/stakes RUNE)
  6. For validators only:     create the genesis transaction (gentx)

Modes:
  * automated - runs init, keys add and gentx for you (with prompts).
  * manual    - prints the exact commands for you to copy-paste and run yourself.

Devnet is a FULL mainnet replica for accelerated testing; it is not a localnet.
Both validator and seed-only nodes are supported. Additional validators can join
an already-running chain later, so a single initial validator boots the network.

Non-interactive use is possible by passing the flags below.`

// setupDeps carries the app wiring the setup command needs to EXECUTE the
// bootstrap commands (init, keys add, gentx) in automated mode.
type setupDeps struct {
	mbm           module.BasicManager
	txConfig      client.TxConfig
	defaultHome   string
	valAddrCodec  address.Codec
	genBalancesIt genutiltypes.GenesisBalancesIterator
}

// NewSetupCmd returns the guided first-run setup command.
func NewSetupCmd(deps setupDeps) *cobra.Command {
	defaultNodeHome := deps.defaultHome
	cmd := &cobra.Command{
		Use:   "setup",
		Short: "Guided first-run setup for a validator or seed node",
		Long:  SetupCmdLong,
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			inBuf := bufio.NewReader(cmd.InOrStdin())

			mode, _ := cmd.Flags().GetString(flagSetupMode)
			moniker, _ := cmd.Flags().GetString(flagSetupMoniker)
			network, _ := cmd.Flags().GetString(flagSetupNetwork)
			nodeType, _ := cmd.Flags().GetString(flagSetupNodeType)
			keyName, _ := cmd.Flags().GetString(flagSetupKeyName)
			chainIDOverride, _ := cmd.Flags().GetString(flags.FlagChainID)
			keyringBackend, _ := cmd.Flags().GetString(flags.FlagKeyringBackend)
			home, _ := cmd.Flags().GetString(flags.FlagHome)
			if home == "" {
				home = defaultNodeHome
			}
			stakeAmount, _ := cmd.Flags().GetString(flagSetupStakeAmount)

			// --- 0. Mode --------------------------------------------------------
			if mode == "" {
				var err error
				mode, err = input.GetString(
					"Do you want the setup to be [automated] (run it for you) or [manual] (print the steps)?",
					inBuf,
				)
				if err != nil {
					return err
				}
			}
			mode = strings.ToLower(strings.TrimSpace(mode))
			if mode != setupModeAutomated && mode != setupModeManual {
				return fmt.Errorf("invalid mode %q (expected automated or manual)", mode)
			}

			// --- 1. Network -----------------------------------------------------
			if network == "" {
				var err error
				network, err = input.GetString("Select the network [mainnet/testnet/devnet]", inBuf)
				if err != nil {
					return err
				}
			}
			chainID := chainIDOverride
			if chainID == "" {
				var ok bool
				chainID, ok = setupNetworkChainID(network)
				if !ok {
					return fmt.Errorf("unknown network %q (expected mainnet, testnet or devnet)", network)
				}
			}

			// --- 2. Node type ---------------------------------------------------
			if nodeType == "" {
				var err error
				nodeType, err = input.GetString(
					"Set up a [validator] node or a [seed] (non-validating) node?", inBuf,
				)
				if err != nil {
					return err
				}
			}
			nodeType = strings.ToLower(strings.TrimSpace(nodeType))
			if nodeType != nodeTypeValidator && nodeType != nodeTypeSeed {
				return fmt.Errorf("invalid node type %q (expected validator or seed)", nodeType)
			}

			// --- 3. Moniker -----------------------------------------------------
			if moniker == "" {
				var err error
				moniker, err = input.GetString("Enter a moniker for your node", inBuf)
				if err != nil {
					return err
				}
				if strings.TrimSpace(moniker) == "" {
					return fmt.Errorf("moniker must not be empty")
				}
			}

			fmt.Printf("\nSetting up a %q node on %q (chain-id %s)\n", nodeType, network, chainID)
			fmt.Printf("Home: %s\n\n", home)

			// --- 4. Wallet key name (validator only) ---------------------------
			if nodeType == nodeTypeValidator && keyName == "" {
				var err error
				keyName, err = input.GetString("Enter a name for your validator wallet key", inBuf)
				if err != nil {
					return err
				}
				if strings.TrimSpace(keyName) == "" {
					return fmt.Errorf("key name must not be empty")
				}
			}

			// Manual mode: print the copy-paste guide.
			if mode == setupModeManual {
				printSetupInstructions(home, moniker, chainID, nodeType, keyName, keyringBackend, stakeAmount)
				return nil
			}

			// Automated mode: run the bootstrap commands.
			return runSetupAutomated(deps, cmd, home, moniker, chainID, nodeType, keyName, keyringBackend, stakeAmount)
		},
	}

	addSetupFlags(cmd, defaultNodeHome)

	return cmd
}

// printSetupInstructions prints the copy-pasteable bootstrap sequence for the
// chosen node type and network.
func printSetupInstructions(home, moniker, chainID, nodeType, keyName, keyringBackend, stakeAmount string) {
	base := fmt.Sprintf("--home %s", home)

	fmt.Println("──────────────────────────────────────────────────────────────")
	fmt.Println("Run the following commands in order:")
	fmt.Println()

	// 1. init
	fmt.Println("# 1. Initialize the node (writes genesis + config + node keys).")
	fmt.Printf("runed init %s --chain-id %s %s\n\n", moniker, chainID, base)

	// 2. wallet key
	fmt.Println("# 2. Create your wallet key (you will be prompted for a keyring password).")
	fmt.Printf("runed keys add %s --keyring-backend %s %s\n\n", keyName, keyringBackend, base)

	if nodeType == nodeTypeValidator {
		fmt.Println("# 3. Ensure the wallet above holds at least the stake amount.")
		fmt.Println("#    (mainnet/testnet: acquire RUNE first; devnet: use the premine account.)")
		fmt.Println()
		fmt.Println("# 4. Create the validator genesis transaction (gentx).")
		fmt.Printf("VOTE_PUBKEY=$(runed tendermint show-validator %s)\n", base)
		fmt.Printf(
			"runed genesis gentx %s %s --pubkey \"$VOTE_PUBKEY\" --chain-id %s "+
				"--keyring-backend %s %s\n\n",
			keyName, stakeAmount, chainID, keyringBackend, base,
		)
		fmt.Println("# 5. Joining an already-running chain? Submit create-validator instead:")
		fmt.Printf(
			"runed tx staking create-validator --amount %s --pubkey \"$VOTE_PUBKEY\" "+
				"--moniker %s --commission-rate 0.05 "+
				"--min-self-delegation 500000000000000000000000 "+
				"--from %s --keyring-backend %s %s\n\n",
			stakeAmount, moniker, keyName, keyringBackend, base,
		)
		fmt.Println("# 6. Start the node.")
		fmt.Printf("runed start %s\n", base)
	} else {
		fmt.Println("# 3. Start the node (seed / non-validating).")
		fmt.Printf("runed start %s\n\n", base)
		fmt.Println("A seed node does not stake or produce blocks; it relays p2p traffic.")
		fmt.Println("Point other nodes at it via --p2p.persistent_peers.")
	}

	fmt.Println()
	fmt.Println("──────────────────────────────────────────────────────────────")
	fmt.Println("Notes:")
	fmt.Printf("  * Network chain-id: %s\n", chainID)
	fmt.Println("  * Devnet is a full mainnet replica for accelerated testing (not a localnet).")
	fmt.Println("  * The wallet's 0x address is the same account as its rune1... bech32")
	fmt.Println("    address (shared balance) — usable from an EVM wallet directly.")
	if nodeType == nodeTypeValidator {
		fmt.Println("  * MinSelfDelegation is 500,000 RUNE; commission must be >= 5%.")
	}
	fmt.Println("──────────────────────────────────────────────────────────────")
}

const (
	flagSetupMode        = "mode"
	flagSetupMoniker     = "moniker"
	flagSetupNetwork     = "network"
	flagSetupNodeType    = "node-type"
	flagSetupKeyName     = "key-name"
	flagSetupStakeAmount = "stake-amount"
)

func addSetupFlags(cmd *cobra.Command, defaultNodeHome string) {
	cmd.Flags().String(flagSetupMode, "", "Setup mode: automated | manual (skip the interactive prompt)")
	cmd.Flags().String(flagSetupMoniker, "", "Node moniker (skip the interactive prompt)")
	cmd.Flags().String(flagSetupNetwork, "", "Network: mainnet | testnet | devnet")
	cmd.Flags().String(flagSetupNodeType, "", "Node type: validator | seed")
	cmd.Flags().String(flagSetupKeyName, "", "Name of the wallet key to create (validator only)")
	cmd.Flags().String(
		flagSetupStakeAmount, "500000000000000000000000arune",
		"Self-delegation amount for the validator (default 500,000 RUNE)",
	)
	cmd.Flags().String(flags.FlagHome, defaultNodeHome, "The application home directory")
	cmd.Flags().String(flags.FlagChainID, "", "Override the chain-id derived from --network")
	cmd.Flags().String(
		flags.FlagKeyringBackend, flags.DefaultKeyringBackend,
		"Keyring backend for the wallet key (os|file|test)",
	)
}

// runSetupAutomated executes the bootstrap sequence by building and running the
// real child commands (init, keys add, gentx). This reuses the exact same logic
// the standalone commands use, so behaviour can never drift. Prompts (such as
// the keyring password) are surfaced to the user as usual.
//
// A confirmation is required before anything is written.
func runSetupAutomated(
	deps setupDeps,
	parent *cobra.Command,
	home, moniker, chainID, nodeType, keyName, keyringBackend, stakeAmount string,
) error {
	inBuf := bufio.NewReader(parent.InOrStdin())

	// Confirm before mutating anything.
	confirm, err := input.GetString(
		fmt.Sprintf("\nAutomated setup will write to %q. Continue? [y/N]", home), inBuf,
	)
	if err != nil {
		return err
	}
	if !strings.HasPrefix(strings.ToLower(strings.TrimSpace(confirm)), "y") {
		fmt.Println("Aborted. Nothing was changed.")
		return nil
	}

	commonArgs := []string{
		"--" + flags.FlagHome, home,
		"--" + flags.FlagKeyringBackend, keyringBackend,
	}

	run := func(name string, child *cobra.Command, args []string) error {
		fmt.Printf("\n── %s ──\n", name)
		child.SetArgs(args)
		child.SetIn(parent.InOrStdin())
		child.SetOut(parent.OutOrStdout())
		child.SetErr(parent.ErrOrStderr())
		return child.ExecuteContext(parent.Context())
	}

	// 1. init -----------------------------------------------------------------
	initArgs := append([]string{moniker, "--" + flags.FlagChainID, chainID}, commonArgs...)
	if err := run("runed init", NewInitCmd(deps.mbm), initArgs); err != nil {
		return fmt.Errorf("init failed: %w", err)
	}

	// 2. keys add (validator only) -------------------------------------------
	if nodeType == nodeTypeValidator {
		keyArgs := append([]string{"add", keyName}, commonArgs...)
		if err := run("runed keys add", runeclient.KeyCommands(home), keyArgs); err != nil {
			return fmt.Errorf("keys add failed: %w", err)
		}

		// 3. gentx ------------------------------------------------------------
		valAddrCodec := deps.valAddrCodec
		gentxCmd := genutilcli.GenTxCmd(
			deps.mbm, deps.txConfig, deps.genBalancesIt, deps.defaultHome, valAddrCodec,
		)
		gentxArgs := append([]string{keyName, stakeAmount}, commonArgs...)
		if err := run("runed genesis gentx", gentxCmd, gentxArgs); err != nil {
			return fmt.Errorf("gentx failed: %w", err)
		}
	}

	// 4. Next step --------------------------------------------------------------
	fmt.Println()
	fmt.Println("──────────────────────────────────────────────────────────────")
	fmt.Println("✅ Setup complete.")
	fmt.Printf("Next: start your node with:\n  runed start --home %s\n", home)
	fmt.Println("──────────────────────────────────────────────────────────────")
	return nil
}
