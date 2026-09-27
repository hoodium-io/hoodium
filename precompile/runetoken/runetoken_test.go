package runetoken_test

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/mezo-org/mezod/app"
	"github.com/mezo-org/mezod/precompile"
	"github.com/mezo-org/mezod/precompile/runetoken"
	"github.com/mezo-org/mezod/precompile/erc20"
	erc20testsuite "github.com/mezo-org/mezod/precompile/erc20/testsuite"
	"github.com/stretchr/testify/suite"
)

const (
	Denom    = "arune"
	Name     = "RUNE"
	Symbol   = "RUNE"
	Decimals = uint8(18)
)

// DomainSeparator is the EIP-712 domain separator for the RUNE token
// precompile, computed at runtime from the chain ID (6590) and the
// precompile's verifying-contract address.
var DomainSeparator = buildDomainSeparator()

func buildDomainSeparator() []byte {
	sep, err := erc20.BuildDomainSeparator(
		big.NewInt(6590),
		Name,
		"1",
		common.HexToAddress(runetoken.EvmAddress),
	)
	if err != nil {
		panic(err)
	}
	return sep
}

func TestBTCPrecompile(t *testing.T) {
	precompileFactoryFn := func(app *app.Hoodium) (*precompile.Contract, error) {
		return runetoken.NewPrecompile(app.BankKeeper, app.AuthzKeeper, *app.EvmKeeper, "rune_6590-1")
	}

	suiteInstance := erc20testsuite.New(
		Denom,
		Name,
		Symbol,
		Decimals,
		DomainSeparator,
		precompileFactoryFn,
		true,
	)

	suite.Run(t, suiteInstance)
}
