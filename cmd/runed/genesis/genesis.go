package genesis

import (
	"github.com/spf13/cobra"

	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/types/module"
	genutilcli "github.com/cosmos/cosmos-sdk/x/genutil/client/cli"
)

// NewCmd builds the genesis utilities command: add-account, migrate, validate
// and the team-only init-files generator (attached in root.go).
//
// NOTE: the Cosmos-standard `gentx` / `collect-gentxs` commands are deliberately
// NOT registered. Hoodium's genesis is always generated in one pass by the team
// with `runed genesis init-files`, which writes the validators straight into the
// staking genesis — so there is no multi-party gentx flow and no gentx artifact
// to collect. Joining an existing network is done with `create-validator`, not a
// gentx. See docs/node-setup.md and docs/validator.md.
func NewCmd(mbm module.BasicManager, _ client.TxConfig, _ string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "genesis",
		Short: "Utilities for chain bootstrapping",
	}

	cmd.AddCommand(
		NewAddAccountCmd(),
		NewMigrateCmd(),
		genutilcli.ValidateGenesisCmd(mbm),
	)

	return cmd
}
