package main

import (
	"bufio"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/client/flags"
	"github.com/cosmos/cosmos-sdk/client/input"
	"github.com/cosmos/cosmos-sdk/types/module"

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

Modes:
  * automated - runs init and keys add for you (with prompts).
  * manual    - prints the exact commands for you to copy-paste and run yourself.

There is NO gentx step: Hoodium's genesis is generated in one pass by the team
via "runed genesis init-files", which writes validators straight into the staking
genesis. To join an already-running network, use create-validator instead.

Devnet is a FULL mainnet replica for accelerated testing; it is not a localnet.
Both validator and seed-only nodes are supported. Additional validators can join
an already-running chain later, so a single initial validator boots the network.

Non-interactive use is possible by passing the flags below.`

// setupDeps carries the app wiring the setup command needs to EXECUTE the
// bootstrap commands (init, keys add) in automated mode.
type setupDeps struct {
	mbm         module.BasicManager
	txConfig    client.TxConfig
	defaultHome string
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
			return runSetupAutomated(
				deps, cmd, home, moniker, chainID, nodeType, keyName, keyringBackend, stakeAmount, network,
			)
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
		fmt.Println("# 3. Replace the default genesis with the official one.")
		fmt.Println("#    Hoodium's genesis is authored by the team; the local `init` only")
		fmt.Println("#    writes a placeholder. Official genesis:")
		fmt.Println("#      git clone https://github.com/hoodium-io/chains.git")
		fmt.Printf("cp chains/mainchain/genesis.json %s/config/genesis.json\n\n", home)

		fmt.Println("# 4. Ensure the wallet above holds at least the stake amount.")
		fmt.Println("#    (mainnet/testnet: acquire RUNE first; devnet: use the premine account.)")
		fmt.Println()

		fmt.Println("# 5. Start the node and let it FULLY sync.")
		fmt.Printf("runed start %s\n\n", base)

		fmt.Println("# 6. Create the validator (in another terminal, once synced).")
		fmt.Println("#    create-validator takes a JSON file, not inline flags. Build it with")
		fmt.Println("#    your consensus pubkey, then submit the file:")
		fmt.Printf("runed tendermint show-validator %s   # -> the \"pubkey\" value\n", base)
		fmt.Println("#    validator.json:")
		fmt.Println("#      {")
		fmt.Println("#        \"pubkey\": <the JSON object printed above>,")
		fmt.Printf("#        \"amount\": \"%s\",\n", stakeAmount)
		fmt.Printf("#        \"moniker\": \"%s\",\n", moniker)
		fmt.Println("#        \"commission-rate\": \"0.05\",")
		fmt.Println("#        \"commission-max-rate\": \"0.25\",")
		fmt.Println("#        \"commission-max-change-rate\": \"0.01\",")
		fmt.Println("#        \"min-self-delegation\": \"500000000000000000000000\"")
		fmt.Println("#      }")
		fmt.Printf("runed tx staking create-validator validator.json --from %s --keyring-backend %s --chain-id %s %s --gas auto --gas-adjustment 1.3 --fees 1000arune -y\n\n", keyName, keyringBackend, chainID, base)
		fmt.Println("NOTE: there is no gentx step. Hoodium genesis is generated in one")
		fmt.Println("pass by the team, so a joining validator uses create-validator.")
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
// real child commands (init, keys add). This reuses the exact same logic the
// standalone commands use, so behaviour can never drift. Prompts (such as the
// keyring password) are surfaced to the user as usual.
//
// A confirmation is required before anything is written.
func runSetupAutomated(
	deps setupDeps,
	parent *cobra.Command,
	home, moniker, chainID, nodeType, keyName, keyringBackend, stakeAmount, network string,
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
	}

	// 3. Next steps ------------------------------------------------------------
	//
	// Hoodium has NO gentx step. Genesis is generated in one pass by the team
	// (`runed genesis init-files`), which writes validators straight into the
	// staking genesis. A joining validator uses `create-validator`, not a gentx.
	fmt.Println()
	fmt.Println("──────────────────────────────────────────────────────────────")
	fmt.Println("✅ Setup complete.")
	fmt.Println()

	switch {
	case nodeType == nodeTypeValidator && network != "":
		// Validator joining a live network.
		fmt.Println("  ⚠️  JOINING THE " + strings.ToUpper(network) + " NETWORK?")
		fmt.Println()
		fmt.Println("  If the " + network + " network is already running, you must:")
		fmt.Println()
		fmt.Println("    1. Replace the default genesis with the official one:")
		fmt.Printf("         cp <downloaded>/mainchain/genesis.json %s/config/genesis.json\n", home)
		fmt.Println("       (official genesis: https://github.com/hoodium-io/chains)")
		fmt.Println("    2. Set the published seeds / persistent_peers in config.toml.")
		fmt.Println("    3. Fund your wallet (>= 500,000 RUNE self-delegation).")
		fmt.Println("    4. Start the node and let it FULLY sync.")
		fmt.Printf("         runed start --home %s\n", home)
		fmt.Println("    5. In another terminal, create your validator.")
		fmt.Println("       create-validator takes a JSON file, not inline flags:")
		fmt.Println("         runed tendermint show-validator --home " + home)
		fmt.Println("         # put that JSON object in validator.json's \"pubkey\" field, then:")
		fmt.Println("         runed tx staking create-validator validator.json \\")
		fmt.Println("           --from " + keyName + " --keyring-backend " + keyringBackend + " \\")
		fmt.Println("           --gas auto --gas-adjustment 1.3 --fees 1000arune -y")
		fmt.Println()
		fmt.Println("  Full guide: docs/validator.md")
	default:
		fmt.Println("Next: start your node with:")
		fmt.Printf("  runed start --home %s\n", home)
	}
	fmt.Println("──────────────────────────────────────────────────────────────")
	return nil
}
