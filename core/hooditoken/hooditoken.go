package hooditoken

import (
	"embed"
	"fmt"
	"math/big"

	sdkmath "cosmossdk.io/math"
	"github.com/hoodium-io/hoodium/utils"
	evmtypes "github.com/hoodium-io/hoodium/x/evm/types"

	authzkeeper "github.com/cosmos/cosmos-sdk/x/authz/keeper"
	bankkeeper "github.com/cosmos/cosmos-sdk/x/bank/keeper"
	"github.com/ethereum/go-ethereum/common"
	"github.com/hoodium-io/hoodium/core"
	"github.com/hoodium-io/hoodium/core/go-erc20"
	runetypes "github.com/hoodium-io/hoodium/types"
	evmkeeper "github.com/hoodium-io/hoodium/x/evm/keeper"
)

//go:embed abi.json
var filesystem embed.FS

const (
	// EvmAddress is the EVM address of the HOODI token precompile. Token address is
	// prefixed with 0x19be which was used to derive Hoodium chain ID. This prefix is
	// used to avoid potential collisions with EVM native precompiles.
	EvmAddress = evmtypes.HOODITokenPrecompileAddress

	Decimals = uint8(18)
	Symbol   = "HOODI"
	Name     = "HOODI"
)

// maxSupplyWholeUnits is the hard cap on the total HOODI supply expressed in
// whole HOODI units (10 million). HOODI has 18 decimals, so this is multiplied
// by 10^18 to obtain the cap in the base denomination (ahoodi).
const maxSupplyWholeUnits = 10_000_000

// MaxSupply is the hard cap on the total HOODI supply expressed in the base
// denomination (ahoodi / "wei"). It is 10,000,000 HOODI, i.e. 10^25 ahoodi.
var MaxSupply = sdkmath.NewIntFromBigInt(
	new(big.Int).Mul(
		new(big.Int).Exp(big.NewInt(10), big.NewInt(18), nil),
		big.NewInt(maxSupplyWholeUnits),
	),
)

// NewPrecompileVersionMap creates a new version map for the HOODI token precompile.
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
			evmtypes.HOODITokenPrecompileLatestVersion: contractV1,
		},
	), nil
}

// NewPrecompile creates a new HOODI token precompile.
//
// NOTE: HOODI is a base ERC-20 token with no minter and no minting path. It is
// detached from any Mainchain module and reserved for the POX Yieldchain, where
// its one-time full max-supply mint will be implemented in a later stage.
func NewPrecompile(
	bankKeeper bankkeeper.Keeper,
	authzkeeper authzkeeper.Keeper,
	evmkeeper evmkeeper.Keeper,
	id string,
) (*core.Contract, error) {
	contractAbi, err := core.LoadAbiFile(filesystem, "abi.json")
	if err != nil {
		return nil, fmt.Errorf("failed to load abi file: [%w]", err)
	}

	chainID, err := runetypes.ParseChainID(id)
	if err != nil {
		return nil, fmt.Errorf("failed to parse chain ID: [%w]", err)
	}

	evmAddress := common.HexToAddress(EvmAddress)
	denom := utils.HoodiDenom

	domainSeparator, err := erc20.BuildDomainSeparator(chainID, Name, "1", evmAddress)
	if err != nil {
		return nil, fmt.Errorf("failed to build domain separator: [%w]", err)
	}

	nonceKey := evmtypes.PrecompileHOODINonceKey()

	contract := core.NewContract(
		contractAbi,
		evmAddress,
		EvmByteCode,
		"hoodi-token",
	)

	methods := newPrecompileMethods(
		bankKeeper,
		authzkeeper,
		evmkeeper,
		denom,
		domainSeparator,
		nonceKey,
	)
	contract.RegisterMethods(methods...)

	return contract, nil
}

// newPrecompileMethods builds the list of methods for the HOODI token precompile.
// All methods returned by this function are registered in the HOODI token precompile.
func newPrecompileMethods(
	bankKeeper bankkeeper.Keeper,
	authzkeeper authzkeeper.Keeper,
	evmkeeper evmkeeper.Keeper,
	denom string,
	domainSeparator []byte,
	nonceKey []byte,
) []core.Method {
	return []core.Method{
		erc20.NewBalanceOfMethod(bankKeeper, denom),
		erc20.NewTotalSupplyMethod(bankKeeper, denom),
		erc20.NewMaxSupplyMethod(MaxSupply),
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
