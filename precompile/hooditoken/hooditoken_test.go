package hooditoken_test

import (
	"math/big"
	"testing"
	"time"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/ethereum/go-ethereum/common"
	"github.com/hoodium-io/hoodium/app"
	"github.com/hoodium-io/hoodium/crypto/ethsecp256k1"
	"github.com/hoodium-io/hoodium/precompile"
	"github.com/hoodium-io/hoodium/precompile/erc20"
	erc20testsuite "github.com/hoodium-io/hoodium/precompile/erc20/testsuite"
	"github.com/hoodium-io/hoodium/precompile/hooditoken"
	"github.com/hoodium-io/hoodium/testutil"
	utiltx "github.com/hoodium-io/hoodium/testutil/tx"
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

type PrecompileTestSuite struct {
	suite.Suite

	app                 *app.Hoodium
	ctx                 sdk.Context
	mezoPrecompile      *precompile.Contract
	poaOwner            common.Address
	poaOwnerSDK         sdk.AccAddress
	minter              common.Address
	minterSDK           sdk.AccAddress
	recipient           common.Address
	recipientSDK        sdk.AccAddress
	unauthorizedAddr    common.Address
	unauthorizedAddrSDK sdk.AccAddress
}

func TestMEZOPrecompile(t *testing.T) {
	precompileFactoryFn := func(app *app.Hoodium) (*precompile.Contract, error) {
		return hooditoken.NewPrecompile(
			app.BankKeeper,
			app.AuthzKeeper,
			*app.EvmKeeper,
			app.PoaKeeper,
			"rune_6590-1",
			&hooditoken.Settings{
				Minting: false,
			},
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

	// Run the test suite for the common ERC20 functionality
	suite.Run(t, erc20TestSuite)
	// Run the test suite for custom HOODI functionality
	suite.Run(t, new(PrecompileTestSuite))
}

func (s *PrecompileTestSuite) SetupTest() {
	// Consensus key
	privCons, err := ethsecp256k1.GenerateKey()
	s.Require().NoError(err)
	consAddress := sdk.ConsAddress(privCons.PubKey().Address())

	// Init app
	s.app = app.Setup(false, nil)

	header := testutil.NewHeader(
		1, time.Now().UTC(), "rune_6590-1", consAddress, nil, nil,
	)
	s.ctx = s.app.BaseApp.NewContextLegacy(false, header)

	// Get POA owner from genesis (set by app.Setup)
	s.poaOwnerSDK = s.app.PoaKeeper.GetOwner(s.ctx)
	s.poaOwner = common.BytesToAddress(s.poaOwnerSDK.Bytes())

	// Create minter account
	minterAddr, _ := utiltx.NewAddrKey()
	s.minter = minterAddr
	s.minterSDK = sdk.AccAddress(minterAddr.Bytes())

	// Create recipient account
	recipientAddr, _ := utiltx.NewAddrKey()
	s.recipient = recipientAddr
	s.recipientSDK = sdk.AccAddress(recipientAddr.Bytes())

	// Create unauthorized account
	unauthorizedAddr, _ := utiltx.NewAddrKey()
	s.unauthorizedAddr = unauthorizedAddr
	s.unauthorizedAddrSDK = sdk.AccAddress(unauthorizedAddr.Bytes())

	// Create precompile
	s.mezoPrecompile, err = hooditoken.NewPrecompile(
		s.app.BankKeeper,
		s.app.AuthzKeeper,
		*s.app.EvmKeeper,
		s.app.PoaKeeper,
		"rune_6590-1",
		&hooditoken.Settings{
			Minting: true,
		},
	)
	s.Require().NoError(err)
}
