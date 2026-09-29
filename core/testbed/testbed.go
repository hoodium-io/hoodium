package testbed

import (
	"embed"
	"fmt"
	"math/big"

	authzkeeper "github.com/cosmos/cosmos-sdk/x/authz/keeper"
	bankkeeper "github.com/cosmos/cosmos-sdk/x/bank/keeper"
	"github.com/ethereum/go-ethereum/common"
	"github.com/hoodium-io/hoodium/core"
	runetypes "github.com/hoodium-io/hoodium/types"
	evmkeeper "github.com/hoodium-io/hoodium/x/evm/keeper"
	evmtypes "github.com/hoodium-io/hoodium/x/evm/types"
)

//go:embed abi.json
var filesystem embed.FS

const EvmAddress = evmtypes.TestBedPrecompileAddress

//nolint:unused
var chainID *big.Int

// NewPrecompileVersionMap creates a new version map for the TestBed token precompile.
func NewPrecompileVersionMap(
	bankKeeper bankkeeper.Keeper,
	authzkeeper authzkeeper.Keeper,
	evmkeeper evmkeeper.Keeper,
	id string,
) (*core.VersionMap, error) {
	contractV1, err := NewPrecompile(bankKeeper, authzkeeper, evmkeeper, id)
	if err != nil {
		return nil, err
	}

	return core.NewVersionMap(
		map[int]*core.Contract{
			evmtypes.TestBedPrecompileLatestVersion: contractV1,
		},
	), nil
}

// NewPrecompile creates a new TestBed token precompile.
func NewPrecompile(bankKeeper bankkeeper.Keeper, authzkeeper authzkeeper.Keeper, evmkeeper evmkeeper.Keeper, id string) (*core.Contract, error) {
	contractAbi, err := core.LoadAbiFile(filesystem, "abi.json")
	if err != nil {
		return nil, fmt.Errorf("failed to load abi file: [%w]", err)
	}
	chainID, err = runetypes.ParseChainID(id)
	if err != nil {
		return nil, fmt.Errorf("failed to parse chain ID: [%w]", err)
	}

	contract := core.NewContract(
		contractAbi,
		common.HexToAddress(EvmAddress),
		EvmByteCode,
		"testbed",
	)

	methods := newPrecompileMethods(bankKeeper, authzkeeper, evmkeeper)
	contract.RegisterMethods(methods...)

	return contract, nil
}

// newPrecompileMethods builds the list of methods for the TestBed token precompile.
// All methods returned by this function are registered in the TestBed token precompile.
func newPrecompileMethods(bankKeeper bankkeeper.Keeper, authzkeeper authzkeeper.Keeper, _ evmkeeper.Keeper) []core.Method {
	return []core.Method{
		newTransferWithRevertMethod(bankKeeper, authzkeeper),
	}
}
