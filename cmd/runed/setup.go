package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	pvm "github.com/cometbft/cometbft/privval"

	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/client/flags"
	"github.com/cosmos/cosmos-sdk/client/input"
	"github.com/cosmos/cosmos-sdk/codec"
	cryptocodec "github.com/cosmos/cosmos-sdk/crypto/codec"
	"github.com/cosmos/cosmos-sdk/types/module"

	runeclient "github.com/hoodium-io/hoodium/client"
	cmdcfg "github.com/hoodium-io/hoodium/cmd/config"
	"github.com/hoodium-io/hoodium/utils"
)

// ensureInheritedFlags registers the root's persistent flags on a child command
// when the child does not already define them.
//
// setup executes child commands standalone (child.ExecuteContext), bypassing the
// root, so flags the root normally injects — --home, --chain-id,
// --keyring-backend — are absent and would be rejected as unknown. Registering
// them here reproduces what the root would have done.
func ensureInheritedFlags(cmd *cobra.Command) {
	if cmd.Flags().Lookup(flags.FlagHome) == nil {
		cmd.Flags().String(flags.FlagHome, "", "The application home directory")
	}
	if cmd.Flags().Lookup(flags.FlagChainID) == nil {
		cmd.Flags().String(flags.FlagChainID, "", "Specify Chain ID for sending Tx")
	}
	if cmd.Flags().Lookup(flags.FlagKeyringBackend) == nil {
		cmd.Flags().String(flags.FlagKeyringBackend, cmdcfg.DefaultKeyringBackend, "Select keyring's backend")
	}
}

// minSelfDelegationAmount is the default self-delegation written into the
// generated validator.json: 500,000 RUNE, in the base denom.
const minSelfDelegationAmount = "500000000000000000000000"

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

// resolveChoice maps a prompt answer to one of the allowed options.
//
// It accepts the full option name ("automated") or its first letter ("a"), case
// insensitively, so users can answer these prompts with a single keystroke. The
// empty answer is returned unchanged so the caller's own validation reports it.
func resolveChoice(answer string, options ...string) (string, bool) {
	answer = strings.ToLower(strings.TrimSpace(answer))
	if answer == "" {
		return "", false
	}

	for _, opt := range options {
		if answer == opt {
			return opt, true
		}
	}

	// Single letter: match the option with that initial. Ambiguous initials are
	// rejected rather than guessed.
	if len(answer) == 1 {
		var match string
		for _, opt := range options {
			if strings.HasPrefix(opt, answer) {
				if match != "" {
					return "", false // ambiguous
				}
				match = opt
			}
		}
		if match != "" {
			return match, true
		}
	}

	return "", false
}

// setupDeps carries the app wiring the setup command needs to EXECUTE the
// bootstrap commands (init, keys add) in automated mode.
type setupDeps struct {
	mbm         module.BasicManager
	txConfig    client.TxConfig
	codec       codec.Codec
	defaultHome string
}

// validatorJSONName is the file `create-validator` reads. The SDK requires a JSON
// file rather than flags (see cosmos-sdk#14864), so setup writes one for the user.
const validatorJSONName = "validator.json"

// writeValidatorJSONFile writes a ready-to-submit create-validator input file
// into the node's home directory.
//
// It reads the node's consensus public key from priv_validator_key.json (written
// by `init`) and fills in the remaining terms, so the operator only has to edit
// the moniker/amount if they want something other than the defaults.
//
// The file is deliberately placed in the node home rather than a temp dir: it
// records the exact terms the validator was created under, and create-validator
// must be run after the node has synced (potentially hours later, in a different
// shell).
func writeValidatorJSONFile(
	cdc codec.Codec, home, moniker, stakeAmount string,
) (string, error) {
	privValPath := filepath.Join(home, "config", "priv_validator_key.json")

	privVal := pvm.LoadFilePVEmptyState(privValPath, "")
	pubKey, err := privVal.GetPubKey()
	if err != nil {
		return "", fmt.Errorf("failed to read consensus key from %s: %w", privValPath, err)
	}

	sdkPubKey, err := cryptocodec.FromCmtPubKeyInterface(pubKey)
	if err != nil {
		return "", fmt.Errorf("failed to convert consensus key: %w", err)
	}

	// Marshal through the app codec so the pubkey carries the "@type" envelope
	// create-validator expects. A bare Any.MarshalJSON panics without a resolver.
	pubKeyJSON, err := cdc.MarshalInterfaceJSON(sdkPubKey)
	if err != nil {
		return "", fmt.Errorf("failed to encode consensus key: %w", err)
	}

	contents, err := buildValidatorJSON(pubKeyJSON, moniker, stakeAmount)
	if err != nil {
		return "", err
	}

	path := filepath.Join(home, validatorJSONName)
	if err := os.WriteFile(path, contents, 0o600); err != nil {
		return "", err
	}

	return path, nil
}

// buildValidatorJSON assembles the create-validator input, splicing in the
// already-marshalled pubkey so its @type envelope is preserved verbatim.
func buildValidatorJSON(pubKeyJSON json.RawMessage, moniker, stakeAmount string) ([]byte, error) {
	input := map[string]json.RawMessage{
		"pubkey":                     pubKeyJSON,
		"amount":                     mustJSON(stakeAmount),
		"moniker":                    mustJSON(moniker),
		"commission-rate":            mustJSON("0.05"),
		"commission-max-rate":        mustJSON("0.25"),
		"commission-max-change-rate": mustJSON("0.01"),
		"min-self-delegation":        mustJSON(minSelfDelegationAmount),
	}

	return json.MarshalIndent(input, "", "  ")
}

// mustJSON marshals a string; it can only fail on invalid UTF-8, which cannot
// occur for the literals and CLI input used here.
func mustJSON(s string) json.RawMessage {
	bz, err := json.Marshal(s)
	if err != nil {
		panic(err)
	}
	return bz
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
				var answer string
				answer, err = input.GetString(
					"Do you want the setup to be [a]utomated (run it for you) or [m]anual (print the steps)?",
					inBuf,
				)
				if err != nil {
					return err
				}

				resolved, ok := resolveChoice(answer, setupModeAutomated, setupModeManual)
				if !ok {
					return fmt.Errorf(
						"invalid mode %q (type a for automated or m for manual)", strings.TrimSpace(answer),
					)
				}
				mode = resolved
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
				var answer string
				answer, err = input.GetString(
					"Set up a [v]alidator node or a [s]eed (non-validating) node?", inBuf,
				)
				if err != nil {
					return err
				}

				resolved, ok := resolveChoice(answer, nodeTypeValidator, nodeTypeSeed)
				if !ok {
					return fmt.Errorf(
						"invalid node type %q (type v for validator or s for seed)", strings.TrimSpace(answer),
					)
				}
				nodeType = resolved
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
				fmt.Printf("\nNote: using your moniker (%q) as the key name is recommended —\n"+
					"keeping one name avoids confusion later.\n", moniker)
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
		flags.FlagKeyringBackend, cmdcfg.DefaultKeyringBackend,
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

	// run executes a child command directly, without going through the root.
	//
	// Commands like `init` do not declare --home / --chain-id / --keyring-backend
	// themselves; when run under the root they inherit them as persistent flags.
	// Executing the child standalone would therefore reject those arguments with
	// "unknown flag", so any inherited flag the caller relies on is registered
	// here when the child does not already define it.
	run := func(name string, child *cobra.Command, args []string) error {
		fmt.Printf("\n── %s ──\n", name)
		ensureInheritedFlags(child)
		child.SetArgs(args)

		// Seed the child's client context with the SAME buffered reader used for
		// the prompts above.
		//
		// Child commands read prompts from clientCtx.Input (see runAddCmd in
		// client/keys.go), not from cmd.InOrStdin, so SetIn alone does nothing.
		// And it must be this reader rather than parent.InOrStdin(): bufio reads
		// ahead, so the raw stream has already been drained by the prompts here,
		// and a child that prompts (`keys add` with the file backend asks for the
		// passphrase twice) would otherwise see EOF and abort.
		//
		// The parent's client context is copied rather than replaced so the child
		// keeps the codec, home directory and keyring options the root resolved;
		// only Input is overridden. SetCmdClientContext also dereferences
		// cmd.Context() without a nil check, so a context must be set first.
		child.SetContext(parent.Context())
		childCtx := client.GetClientContextFromCmd(parent).WithInput(inBuf)
		if err := client.SetCmdClientContext(child, childCtx); err != nil {
			return err
		}

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

		// 3. validator.json ---------------------------------------------------
		// create-validator takes a JSON file, not flags, so write one now using
		// the consensus key that `init` just generated.
		validatorPath, err := writeValidatorJSONFile(deps.codec, home, moniker, stakeAmount)
		if err != nil {
			return fmt.Errorf("failed to write %s: %w", validatorJSONName, err)
		}
		fmt.Printf("\n── %s ──\n", validatorJSONName)
		fmt.Printf("Wrote %s (edit it if you want a different moniker or amount).\n", validatorPath)
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
		// Validator joining a live network. The network is already running by
		// definition — setup is only ever run to join one.
		fmt.Println("  ⚠️  You are now ready to join Hoodium " + strings.ToUpper(network))
		fmt.Println()
		fmt.Println("  Next steps:")
		fmt.Println()
		fmt.Println("    1. Replace the default genesis with the official one:")
		fmt.Println("       (a) git clone https://github.com/hoodium-io/chains")
		fmt.Printf("       (b) cp chains/mainchain/%s/genesis.json %s/config/genesis.json\n", network, home)
		fmt.Printf("    2. Set the published seeds / persistent_peers from\n"+
			"       chains/mainchain/%s/seeds.md in %s/config/config.toml.\n", network, home)
		fmt.Println("    3. Fund your wallet (>= 500,000 RUNE self-delegation).")
		fmt.Println("       Your account has ONE balance reachable as both its 0x and rune1")
		fmt.Println("       address. Export the key once and import it into an EVM wallet,")
		fmt.Println("       then send RUNE to that address from anywhere:")
		fmt.Println("         runed keys unsafe-export-eth-key " + keyName + " \\")
		fmt.Printf("           --keyring-backend %s --home %s\n", keyringBackend, home)
		fmt.Println("    4. Start the node and let it FULLY sync.")
		fmt.Printf("         runed start --home %s\n", home)
		fmt.Println("    5. In another terminal, launch your validator node:")
		fmt.Println("         runed tx staking create-validator validator.json \\")
		fmt.Printf("           --from %s --keyring-backend %s \\\n", keyName, keyringBackend)
		fmt.Println("           --gas auto --gas-adjustment 1.3 --fees 1000arune -y")
		fmt.Println("    6. Secure the node and set up systemd + cron so it restarts on reboot.")
		fmt.Println()
		fmt.Println("  Full guide: docs/validator.md")
	case nodeType == nodeTypeSeed && network != "":
		// Seed node joining a live network. No validator is created, so there is
		// no stake, no validator.json and no create-validator step.
		fmt.Println("  ⚠️  You are now ready to join Hoodium " + strings.ToUpper(network) + " as a seed node")
		fmt.Println()
		fmt.Println("  Next steps:")
		fmt.Println()
		fmt.Println("    1. Replace the default genesis with the official one:")
		fmt.Println("       (a) git clone https://github.com/hoodium-io/chains")
		fmt.Printf("       (b) cp chains/mainchain/%s/genesis.json %s/config/genesis.json\n", network, home)
		fmt.Printf("    2. Set the published seeds / persistent_peers from\n"+
			"       chains/mainchain/%s/seeds.md in %s/config/config.toml.\n", network, home)
		fmt.Println("    3. Start the node and let it FULLY sync.")
		fmt.Printf("         runed start --home %s\n", home)
		fmt.Println("    4. Secure the node and set up systemd + cron so it restarts on reboot.")
		fmt.Println()
		fmt.Println("  A seed node does not stake or produce blocks — it relays p2p traffic.")
		fmt.Println()
		fmt.Println("  Full guide: docs/node-setup.md")
	default:
		fmt.Println("Next: start your node with:")
		fmt.Printf("  runed start --home %s\n", home)
	}
	fmt.Println("──────────────────────────────────────────────────────────────")
	return nil
}
