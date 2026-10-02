package hooditoken_test

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/hoodium-io/hoodium/app"
	"github.com/hoodium-io/hoodium/core"
	"github.com/hoodium-io/hoodium/core/go-erc20"
	erc20testsuite "github.com/hoodium-io/hoodium/core/go-erc20/testsuite"
	"github.com/hoodium-io/hoodium/core/hooditoken"
	"github.com/stretchr/testify/suite"
)

const (
	Denom    = "ahoodi"
	Name     = "HOODI"
	Symbol   = "HOODI"
	Decimals = uint8(18)
)

// DomainSeparator is the EIP-712 domain separator for the HOODI token
// precompile, computed at runtime from the chain ID (6590) and the
// precompile's verifying-contract address.
var DomainSeparator = buildDomainSeparator()

func buildDomainSeparator() []byte {
	sep, err := erc20.BuildDomainSeparator(
		big.NewInt(6590),
		Name,
		"1",
		common.HexToAddress(hooditoken.EvmAddress),
	)
	if err != nil {
		panic(err)
	}
	return sep
}

// TestHOODIPrecompile runs the common ERC-20 test suite against the HOODI
// precompile. HOODI is a base ERC-20 token (no minter/mint) — the mint path
// was removed and is reserved for the POX Yieldchain (one-time full mint).
func TestHOODIPrecompile(t *testing.T) {
	precompileFactoryFn := func(app *app.Hoodium) (*core.Contract, error) {
		return hooditoken.NewPrecompile(
			app.BankKeeper,
			app.AuthzKeeper,
			*app.EvmKeeper,
			"rune_6590-1",
		)
	}

	erc20TestSuite := erc20testsuite.New(
		Denom,
		Name,
		Symbol,
		Decimals,
		DomainSeparator,
		precompileFactoryFn,
		false,
	)

	suite.Run(t, erc20TestSuite)
}
