package runetoken

import (
	"embed"
	"fmt"

	evmtypes "github.com/hoodium-io/hoodium/x/evm/types"

	authzkeeper "github.com/cosmos/cosmos-sdk/x/authz/keeper"
	bankkeeper "github.com/cosmos/cosmos-sdk/x/bank/keeper"
	"github.com/ethereum/go-ethereum/common"
	"github.com/hoodium-io/hoodium/precompile"
	"github.com/hoodium-io/hoodium/precompile/erc20"
	mezotypes "github.com/hoodium-io/hoodium/types"
	evmkeeper "github.com/hoodium-io/hoodium/x/evm/keeper"
)

//go:embed abi.json
var filesystem embed.FS

const (
	// EvmAddress is the EVM address of the RUNE token precompile. Token address is
	// prefixed with 0x19be which was used to derive Hoodium chain ID. This prefix is
	// used to avoid potential collisions with EVM native precompiles.
	EvmAddress = evmtypes.RUNETokenPrecompileAddress

	Decimals = uint8(18)
	Symbol   = "RUNE"
	Name     = "RUNE"
)

// NewPrecompileVersionMap creates a new version map for the RUNE token precompile.
func NewPrecompileVersionMap(
	bankKeeper bankkeeper.Keeper,
	authzkeeper authzkeeper.Keeper,
	evmkeeper evmkeeper.Keeper,
	id string,
) (*precompile.VersionMap, error) {
	contractV1, err := NewPrecompile(bankKeeper, authzkeeper, evmkeeper, id)
	if err != nil {
		return nil, err
	}

	return precompile.NewVersionMap(
		map[int]*precompile.Contract{
			0:                                        contractV1, // returning v1 as v0 is legacy to support this precompile before versioning was introduced
			evmtypes.RUNETokenPrecompileLatestVersion: contractV1,
		},
	), nil
}

// NewPrecompile creates a new RUNE token precompile.
func NewPrecompile(bankKeeper bankkeeper.Keeper, authzkeeper authzkeeper.Keeper, evmkeeper evmkeeper.Keeper, id string) (*precompile.Contract, error) {
	contractAbi, err := precompile.LoadAbiFile(filesystem, "abi.json")
	if err != nil {
		return nil, fmt.Errorf("failed to load abi file: [%w]", err)
	}

	chainID, err := mezotypes.ParseChainID(id)
	if err != nil {
		return nil, fmt.Errorf("failed to parse chain ID: [%w]", err)
	}

	evmAddress := common.HexToAddress(EvmAddress)
	denom := evmtypes.DefaultEVMDenom

	domainSeparator, err := erc20.BuildDomainSeparator(chainID, Name, "1", evmAddress)
	if err != nil {
		return nil, fmt.Errorf("failed to build domain separator: [%w]", err)
	}

	nonceKey := evmtypes.PrecompileRUNENonceKey()

	contract := precompile.NewContract(
		contractAbi,
		evmAddress,
		EvmByteCode,
		"btc-token",
	)

	methods := newPrecompileMethods(bankKeeper, authzkeeper, evmkeeper, denom, domainSeparator, nonceKey)
	contract.RegisterMethods(methods...)

	return contract, nil
}

// newPrecompileMethods builds the list of methods for the RUNE token precompile.
// All methods returned by this function are registered in the RUNE token precompile.
func newPrecompileMethods(
	bankKeeper bankkeeper.Keeper,
	authzkeeper authzkeeper.Keeper,
	evmkeeper evmkeeper.Keeper,
	denom string,
	domainSeparator []byte,
	nonceKey []byte,
) []precompile.Method {
	return []precompile.Method{
		erc20.NewBalanceOfMethod(bankKeeper, denom),
		erc20.NewTotalSupplyMethod(bankKeeper, denom),
		erc20.NewNameMethod(Name),
		erc20.NewSymbolMethod(Symbol),
		erc20.NewDecimalsMethod(Decimals),
		erc20.NewApproveMethod(bankKeeper, authzkeeper, denom),
		erc20.NewTransferMethod(bankKeeper, authzkeeper, evmkeeper, denom),
		erc20.NewTransferFromMethod(bankKeeper, authzkeeper, evmkeeper, denom),
		erc20.NewAllowanceMethod(authzkeeper, denom),
		erc20.NewPermitMethod(bankKeeper, authzkeeper, evmkeeper, denom, domainSeparator, nonceKey),
		erc20.NewNonceMethod(evmkeeper, nonceKey),
		erc20.NewNoncesMethod(evmkeeper, nonceKey),
		erc20.NewDomainSeparatorMethod(domainSeparator),
		erc20.NewPermitTypehashMethod(),
	}
}
