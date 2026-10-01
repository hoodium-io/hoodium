package genesis

import (
	"github.com/spf13/cobra"

	"github.com/cosmos/cosmos-sdk/types/module"
	genutilcli "github.com/cosmos/cosmos-sdk/x/genutil/client/cli"
)

// NewCmd builds the genesis utilities command (add-account, migrate, validate).
//
// TODO(Stage-3): wire the standard Cosmos SDK staking gentx/collect-gentxs flow
// here. This requires registering the genutil module in app.ModuleBasics (not
// currently present — the chain previously used PoA's own genval/collect-genvals
// path, which is now removed). The PoA genval/collect-genvals commands are gone.
func NewCmd(mbm module.BasicManager) *cobra.Command {
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
