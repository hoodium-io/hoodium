package keeper_test

import (
	sdk "github.com/cosmos/cosmos-sdk/types"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	"github.com/ethereum/go-ethereum/common"
	evmtypes "github.com/hoodium-io/hoodium/x/evm/types"
)

func (suite *KeeperTestSuite) TestEndBlock() {
	em := suite.ctx.EventManager()
	suite.Require().Equal(0, len(em.Events()))

	err := suite.app.EvmKeeper.EndBlock(suite.ctx)
	suite.Require().NoError(err)

	// should emit 1 EventTypeBlockBloom event on EndBlock
	suite.Require().Equal(1, len(em.Events()))
	suite.Require().Equal(evmtypes.EventTypeBlockBloom, em.Events()[0].Type)
}

func (suite *KeeperTestSuite) TestEndBlockWithZeroBalance() {
	feeCollectorAddr := authtypes.NewModuleAddress(authtypes.FeeCollectorName)
	balance := suite.app.BankKeeper.GetBalance(suite.ctx, feeCollectorAddr, suite.app.EvmKeeper.GetParams(suite.ctx).EvmDenom)

	// Set balance to zero for FeeCollector
	zeroAddress := common.Address{}.Bytes()
	err := suite.app.BankKeeper.SendCoinsFromModuleToAccount(suite.ctx, authtypes.FeeCollectorName, zeroAddress, sdk.NewCoins(balance))
	suite.Require().NoError(err)

	err = suite.app.EvmKeeper.EndBlock(suite.ctx)
	suite.Require().NoError(err)
}
