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

	"github.com/hoodium-io/hoodium/utils"
)

// Node types selectable in the guided setup.
const (
	nodeTypeValidator = "validator"
	nodeTypeSeed      = "seed"
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

  1. Choose the network       (mainnet | testnet | devnet)
  2. Choose the node type     (validator | seed)
  3. Initialize the node      (genesis + config + node keys)
  4. Create your wallet key   (the account that holds/stakes RUNE)
  5. For validators only:     create the genesis transaction (gentx)

Devnet is a FULL mainnet replica for accelerated testing; it is not a localnet.
Both validator and seed-only nodes are supported. Additional validators can join
an already-running chain later, so a single initial validator boots the network.

Non-interactive use is possible by passing the flags below.`

// NewSetupCmd returns the guided first-run setup command.
func NewSetupCmd(_ module.BasicManager, defaultNodeHome string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "setup",
		Short: "Guided first-run setup for a validator or seed node",
		Long:  SetupCmdLong,
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			inBuf := bufio.NewReader(cmd.InOrStdin())

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

			printSetupInstructions(home, moniker, chainID, nodeType, keyName, keyringBackend, stakeAmount)

			return nil
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
	flagSetupMoniker     = "moniker"
	flagSetupNetwork     = "network"
	flagSetupNodeType    = "node-type"
	flagSetupKeyName     = "key-name"
	flagSetupStakeAmount = "stake-amount"
)

func addSetupFlags(cmd *cobra.Command, defaultNodeHome string) {
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
