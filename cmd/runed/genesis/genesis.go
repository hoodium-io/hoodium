package genesis

import (
	"github.com/spf13/cobra"

	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/types/module"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	"github.com/cosmos/cosmos-sdk/x/genutil"
	genutilcli "github.com/cosmos/cosmos-sdk/x/genutil/client/cli"
	genutiltypes "github.com/cosmos/cosmos-sdk/x/genutil/types"
)

// NewCmd builds the genesis utilities command: add-account, migrate, gentx,
// collect-gentxs and validate. It uses the standard Cosmos SDK staking gentx flow
// (replacing the removed PoA genval/collect-genvals commands).
func NewCmd(mbm module.BasicManager, txConfig client.TxConfig, defaultNodeHome string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "genesis",
		Short: "Utilities for chain bootstrapping",
	}

	gentxModule := mbm[genutiltypes.ModuleName].(genutil.AppModuleBasic)
	valAddrCodec := txConfig.SigningContext().ValidatorAddressCodec()

	cmd.AddCommand(
		NewAddAccountCmd(),
		NewMigrateCmd(),
		genutilcli.GenTxCmd(mbm, txConfig, banktypes.GenesisBalancesIterator{}, defaultNodeHome, valAddrCodec),
		genutilcli.CollectGenTxsCmd(banktypes.GenesisBalancesIterator{}, defaultNodeHome, gentxModule.GenTxValidator, valAddrCodec),
		genutilcli.ValidateGenesisCmd(mbm),
	)

	return cmd
}
