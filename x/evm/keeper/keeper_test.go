package keeper_test

import (
	_ "embed"
	"math/big"

	sdk "github.com/cosmos/cosmos-sdk/types"

	runetypes "github.com/hoodium-io/hoodium/types"
	"github.com/hoodium-io/hoodium/x/evm/keeper"
	"github.com/hoodium-io/hoodium/x/evm/statedb"
	evmtypes "github.com/hoodium-io/hoodium/x/evm/types"

	"github.com/ethereum/go-ethereum/common"
	"github.com/holiman/uint256"
)

func (suite *KeeperTestSuite) TestWithChainID() {
	testCases := []struct {
		name       string
		chainID    string
		expChainID int64
		expPanic   bool
	}{
		{
			"fail - chainID is empty",
			"",
			0,
			true,
		},
		{
			"fail - other chainID",
			"chain_7701-1",
			0,
			true,
		},
		{
			"success - Hoodium mainnet chain ID",
			"rune_6590-1",
			6590,
			false,
		},
		{
			"success - Hoodium testnet chain ID",
			"rune_6591-1",
			6591,
			false,
		},
	}

	for _, tc := range testCases {
		suite.Run(tc.name, func() {
			keeper := keeper.Keeper{}
			ctx := suite.ctx.WithChainID(tc.chainID)

			if tc.expPanic {
				suite.Require().Panics(func() {
					keeper.WithChainID(ctx)
				})
			} else {
				suite.Require().NotPanics(func() {
					keeper.WithChainID(ctx)
					suite.Require().Equal(tc.expChainID, keeper.ChainID().Int64())
				})
			}
		})
	}
}

func (suite *KeeperTestSuite) TestBaseFee() {
	testCases := []struct {
		name            string
		enableLondonHF  bool
		enableFeemarket bool
		expectBaseFee   *big.Int
	}{
		{"not enable london HF, not enable feemarket", false, false, nil},
		{"enable london HF, not enable feemarket", true, false, big.NewInt(0)},
		{"enable london HF, enable feemarket", true, true, big.NewInt(1000000000)},
		{"not enable london HF, enable feemarket", false, true, nil},
	}

	for _, tc := range testCases {
		suite.Run(tc.name, func() {
			suite.enableFeemarket = tc.enableFeemarket
			suite.enableLondonHF = tc.enableLondonHF
			suite.SetupTest()
			err := suite.app.EvmKeeper.BeginBlock(suite.ctx)
			suite.Require().NoError(err)
			params := suite.app.EvmKeeper.GetParams(suite.ctx)
			ethCfg := params.ChainConfig.EthereumConfig(suite.app.EvmKeeper.ChainID())
			baseFee := suite.app.EvmKeeper.GetBaseFee(suite.ctx, ethCfg)
			suite.Require().Equal(tc.expectBaseFee, baseFee)
		})
	}
	suite.enableFeemarket = false
	suite.enableLondonHF = true
}

func (suite *KeeperTestSuite) TestGetAccountStorage() {
	testCases := []struct {
		name       string
		malleate   func() common.Address // returns the contract address (or zero if none)
		expStorage []int                 // expected storage length for suite.address (wallet) then contract
	}{
		{
			"Only one account that's not a contract (no storage)",
			func() common.Address { return common.Address{} },
			[]int{0},
		},
		{
			"Two accounts - one contract (with storage), one wallet",
			func() common.Address {
				supply := big.NewInt(100)
				return suite.DeployTestContract(suite.T(), suite.address, supply)
			},
			[]int{0, 2},
		},
	}

	for _, tc := range testCases {
		suite.Run(tc.name, func() {
			suite.SetupTest()
			contract := tc.malleate()

			// Collect storage lengths keyed by EVM address (order-independent).
			storageByAddress := make(map[common.Address]int)
			suite.app.AccountKeeper.IterateAccounts(suite.ctx, func(account sdk.AccountI) bool {
				ethAccount, ok := account.(runetypes.EthAccountI)
				if !ok {
					return false
				}
				addr := ethAccount.EthAddress()
				storageByAddress[addr] = len(suite.app.EvmKeeper.GetAccountStorage(suite.ctx, addr))
				return false
			})

			// Wallet (suite.address) must have the first expected storage length.
			suite.Require().Equal(tc.expStorage[0], storageByAddress[suite.address])

			// If a contract was deployed, verify its storage length.
			if contract != (common.Address{}) {
				suite.Require().Equal(tc.expStorage[1], storageByAddress[contract])
			}
		})
	}
}

func (suite *KeeperTestSuite) TestGetAccountOrEmpty() {
	empty := statedb.Account{
		Balance:  new(uint256.Int),
		CodeHash: evmtypes.EmptyCodeHash,
	}

	supply := big.NewInt(100)
	contractAddr := suite.DeployTestContract(suite.T(), suite.address, supply)

	testCases := []struct {
		name     string
		addr     common.Address
		expEmpty bool
	}{
		{
			"unexisting account - get empty",
			common.Address{},
			true,
		},
		{
			"existing contract account",
			contractAddr,
			false,
		},
	}

	for _, tc := range testCases {
		suite.Run(tc.name, func() {
			res := suite.app.EvmKeeper.GetAccountOrEmpty(suite.ctx, tc.addr)
			if tc.expEmpty {
				suite.Require().Equal(empty, res)
			} else {
				suite.Require().NotEqual(empty, res)
			}
		})
	}
}

func (suite *KeeperTestSuite) TestCustomPrecompileGenesisAccounts() {
	accounts := suite.app.EvmKeeper.CustomPrecompileGenesisAccounts()
	suite.Require().Equal(len(accounts), 4)

	// Expected addresses are the precompile address constants, checksummed via
	// go-ethereum's EIP-55 (same as the keeper emits). Sorted case-insensitively
	// to match CustomPrecompileGenesisAccounts.
	// NOTE: live precompiles are RUNE, HOODI, PriceOracle and Staking (sequential
	// 0x19be...0000-0003). The validatorpool/maintenance/upgrade precompiles were
	// removed with PoA.
	expected := []string{
		common.HexToAddress(evmtypes.RUNETokenPrecompileAddress).String(),
		common.HexToAddress(evmtypes.HOODITokenPrecompileAddress).String(),
		common.HexToAddress(evmtypes.PriceOraclePrecompileAddress).String(),
		common.HexToAddress(evmtypes.StakingPrecompileAddress).String(),
	}

	for i, exp := range expected {
		suite.Require().Equal(exp, accounts[i].Address)
	}
}

func (suite *KeeperTestSuite) TestIsContract() {
	contract := suite.DeployTestContract(suite.T(), suite.address, big.NewInt(0))

	testCases := []struct {
		name           string
		address        common.Address
		expectedResult bool
	}{
		{
			"non-existing account",
			common.Address{},
			false,
		},
		{
			"existing account",
			common.HexToAddress("0x4ccA899acA68EC4E04408f5A582456D4165e7A8e"),
			false,
		},
		{
			"existing contract",
			contract,
			true,
		},
	}

	for _, tc := range testCases {
		suite.Run(tc.name, func() {
			result := suite.app.EvmKeeper.IsContract(suite.ctx, tc.address.Bytes())
			suite.Require().Equal(tc.expectedResult, result)
		})
	}
}
