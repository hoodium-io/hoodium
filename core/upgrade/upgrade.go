package upgrade

import (
	"context"
	"embed"
	"fmt"

	evmtypes "github.com/hoodium-io/hoodium/x/evm/types"

	upgradetypes "cosmossdk.io/x/upgrade/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/ethereum/go-ethereum/common"
	"github.com/hoodium-io/hoodium/core"
)

//go:embed abi.json
var filesystem embed.FS

// EvmAddress is the EVM address of the upgrade precompile. The address is
// prefixed with 0x19be which was used to derive Hoodium chain ID. This prefix is
// used to avoid potential collisions with EVM native precompiles.
const EvmAddress = evmtypes.UpgradePrecompileAddress

// NewPrecompileVersionMap creates a new version map for the upgrade precompile.
func NewPrecompileVersionMap(
	upgradeKeeper UpgradeKeeper,
	poaKeeper PoaKeeper,
) (*core.VersionMap, error) {
	contractV1, err := NewPrecompile(upgradeKeeper, poaKeeper)
	if err != nil {
		return nil, err
	}

	return core.NewVersionMap(
		map[int]*core.Contract{
			evmtypes.UpgradePrecompileLatestVersion: contractV1,
		},
	), nil
}

// NewPrecompile creates a new upgrade precompile.
func NewPrecompile(upgradeKeeper UpgradeKeeper, poaKeeper PoaKeeper) (*core.Contract, error) {
	contractAbi, err := core.LoadAbiFile(filesystem, "abi.json")
	if err != nil {
		return nil, fmt.Errorf("failed to load abi file: [%w]", err)
	}

	contract := core.NewContract(
		contractAbi,
		common.HexToAddress(EvmAddress),
		EvmByteCode,
		"upgrade",
	)

	methods := newPrecompileMethods(upgradeKeeper, poaKeeper)
	contract.RegisterMethods(methods...)

	return contract, nil
}

// newPrecompileMethods builds the list of methods for the upgrade precompile.
// All methods returned by this function are registered in the upgrade precompile.
func newPrecompileMethods(upgradeKeeper UpgradeKeeper, poaKeeper PoaKeeper) []core.Method {
	return []core.Method{
		newSubmitPlanMethod(upgradeKeeper, poaKeeper),
		newCancelPlanMethod(upgradeKeeper, poaKeeper),
		newPlanMethod(upgradeKeeper),
	}
}

type PoaKeeper interface {
	CheckOwner(ctx sdk.Context, sender sdk.AccAddress) error
}

//nolint:all
type UpgradeKeeper interface {
	ClearUpgradePlan(ctx context.Context) error
	GetUpgradePlan(ctx context.Context) (upgradetypes.Plan, error)
	ScheduleUpgrade(ctx context.Context, plan upgradetypes.Plan) error
}
