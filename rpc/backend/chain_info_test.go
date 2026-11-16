package backend

import (
	"fmt"
	"math/big"

	sdkmath "cosmossdk.io/math"

	"github.com/ethereum/go-ethereum/common/hexutil"
	ethrpc "github.com/ethereum/go-ethereum/rpc"

	"google.golang.org/grpc/metadata"

	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/cometbft/cometbft/abci/types"
	tmrpctypes "github.com/cometbft/cometbft/rpc/core/types"

	"github.com/ethereum/go-ethereum/common/math"
	"github.com/hoodium-io/hoodium/rpc/backend/mocks"
	rpc "github.com/hoodium-io/hoodium/rpc/types"
	utiltx "github.com/hoodium-io/hoodium/testutil/tx"
	evmtypes "github.com/hoodium-io/hoodium/x/evm/types"
	feemarkettypes "github.com/hoodium-io/hoodium/x/feemarket/types"
)

func (suite *BackendTestSuite) TestBaseFee() {
	baseFee := sdkmath.NewInt(1)

	testCases := []struct {
		name         string
		blockRes     *tmrpctypes.ResultBlockResults
		registerMock func()
		expBaseFee   *big.Int
		expPass      bool
	}{
		{
			"fail - grpc BaseFee error",
			&tmrpctypes.ResultBlockResults{Height: 1},
			func() {
				queryClient := suite.backend.queryClient.QueryClient.(*mocks.EVMQueryClient)
				RegisterBaseFeeError(queryClient)
			},
			nil,
			false,
		},
		{
			"fail - grpc BaseFee error - with non feemarket block event",
			&tmrpctypes.ResultBlockResults{
				Height: 1,
				FinalizeBlockEvents: []types.Event{
					{
						Type: evmtypes.EventTypeBlockBloom,
					},
				},
			},
			func() {
				queryClient := suite.backend.queryClient.QueryClient.(*mocks.EVMQueryClient)
				RegisterBaseFeeError(queryClient)
			},
			nil,
			false,
		},
		{
			"fail - grpc BaseFee error - with feemarket block event",
			&tmrpctypes.ResultBlockResults{
				Height: 1,
				FinalizeBlockEvents: []types.Event{
					{
						Type: feemarkettypes.EventTypeFeeMarket,
					},
				},
			},
			func() {
				queryClient := suite.backend.queryClient.QueryClient.(*mocks.EVMQueryClient)
				RegisterBaseFeeError(queryClient)
			},
			nil,
			false,
		},
		{
			"fail - grpc BaseFee error - with feemarket block event with wrong attribute value",
			&tmrpctypes.ResultBlockResults{
				Height: 1,
				FinalizeBlockEvents: []types.Event{
					{
						Type: feemarkettypes.EventTypeFeeMarket,
						Attributes: []types.EventAttribute{
							{Value: string([]byte{0x1})},
						},
					},
				},
			},
			func() {
				queryClient := suite.backend.queryClient.QueryClient.(*mocks.EVMQueryClient)
				RegisterBaseFeeError(queryClient)
			},
			nil,
			false,
		},
		{
			"fail - grpc baseFee error - with feemarket block event with baseFee attribute value",
			&tmrpctypes.ResultBlockResults{
				Height: 1,
				FinalizeBlockEvents: []types.Event{
					{
						Type: feemarkettypes.EventTypeFeeMarket,
						Attributes: []types.EventAttribute{
							{Value: baseFee.String()},
						},
					},
				},
			},
			func() {
				queryClient := suite.backend.queryClient.QueryClient.(*mocks.EVMQueryClient)
				RegisterBaseFeeError(queryClient)
			},
			baseFee.BigInt(),
			true,
		},
		{
			"fail - base fee or london fork not enabled",
			&tmrpctypes.ResultBlockResults{Height: 1},
			func() {
				queryClient := suite.backend.queryClient.QueryClient.(*mocks.EVMQueryClient)
				RegisterBaseFeeDisabled(queryClient)
			},
			nil,
			true,
		},
		{
			"pass",
			&tmrpctypes.ResultBlockResults{Height: 1},
			func() {
				queryClient := suite.backend.queryClient.QueryClient.(*mocks.EVMQueryClient)
				RegisterBaseFee(queryClient, baseFee)
			},
			baseFee.BigInt(),
			true,
		},
	}
	for _, tc := range testCases {
		suite.Run(fmt.Sprintf("Case %s", tc.name), func() {
			suite.SetupTest() // reset test and queries
			tc.registerMock()

			baseFee, err := suite.backend.BaseFee(tc.blockRes)

			if tc.expPass {
				suite.Require().NoError(err)
				suite.Require().Equal(tc.expBaseFee, baseFee)
			} else {
				suite.Require().Error(err)
			}
		})
	}
}

func (suite *BackendTestSuite) TestChainId() {
	expChainID := (*hexutil.Big)(big.NewInt(6591))
	testCases := []struct {
		name         string
		registerMock func()
		expChainID   *hexutil.Big
		expPass      bool
	}{
		{
			"pass - block is at or past the EIP-155 replay-protection fork block, return chainID from config ",
			func() {
				var header metadata.MD
				queryClient := suite.backend.queryClient.QueryClient.(*mocks.EVMQueryClient)
				RegisterParamsInvalidHeight(queryClient, &header, int64(1))
			},
			expChainID,
			true,
		},
	}

	for _, tc := range testCases {
		suite.Run(fmt.Sprintf("case %s", tc.name), func() {
			suite.SetupTest() // reset test and queries
			tc.registerMock()

			chainID, err := suite.backend.ChainID()
			if tc.expPass {
				suite.Require().NoError(err)
				suite.Require().Equal(tc.expChainID, chainID)
			} else {
				suite.Require().Error(err)
			}
		})
	}
}

func (suite *BackendTestSuite) TestGetCoinbase() {
	validatorAcc := sdk.AccAddress(utiltx.GenerateAddress().Bytes())
	testCases := []struct {
		name         string
		registerMock func()
		accAddr      sdk.AccAddress
		expPass      bool
	}{
		{
			"fail - Can't retrieve status from node",
			func() {
				client := suite.backend.clientCtx.Client.(*mocks.Client)
				RegisterStatusError(client)
			},
			validatorAcc,
			false,
		},
		{
			"fail - Can't query validator account",
			func() {
				client := suite.backend.clientCtx.Client.(*mocks.Client)
				queryClient := suite.backend.queryClient.QueryClient.(*mocks.EVMQueryClient)
				RegisterStatus(client)
				RegisterValidatorAccountError(queryClient)
			},
			validatorAcc,
			false,
		},
		{
			"pass - Gets coinbase account",
			func() {
				client := suite.backend.clientCtx.Client.(*mocks.Client)
				queryClient := suite.backend.queryClient.QueryClient.(*mocks.EVMQueryClient)
				RegisterStatus(client)
				RegisterValidatorAccount(queryClient, validatorAcc)
			},
			validatorAcc,
			true,
		},
	}

	for _, tc := range testCases {
		suite.Run(fmt.Sprintf("case %s", tc.name), func() {
			suite.SetupTest() // reset test and queries
			tc.registerMock()

			accAddr, err := suite.backend.GetCoinbase()

			if tc.expPass {
				suite.Require().Equal(tc.accAddr, accAddr)
			} else {
				suite.Require().Error(err)
			}
		})
	}
}

func (suite *BackendTestSuite) TestSuggestGasTipCap() {
	testCases := []struct {
		name         string
		registerMock func()
		baseFee      *big.Int
		expGasTipCap *big.Int
		expPass      bool
	}{
		{
			"pass - London hardfork not enabled or feemarket not enabled ",
			func() {},
			nil,
			big.NewInt(0),
			true,
		},
		{
			"pass - Gets the suggest gas tip cap ",
			func() {},
			nil,
			big.NewInt(0),
			true,
		},
	}

	for _, tc := range testCases {
		suite.Run(fmt.Sprintf("case %s", tc.name), func() {
			suite.SetupTest() // reset test and queries
			tc.registerMock()

			maxDelta, err := suite.backend.SuggestGasTipCap(tc.baseFee)

			if tc.expPass {
				suite.Require().Equal(tc.expGasTipCap, maxDelta)
			} else {
				suite.Require().Error(err)
			}
		})
	}
}

func (suite *BackendTestSuite) TestGlobalMinGasPrice() {
	testCases := []struct {
		name           string
		registerMock   func()
		expMinGasPrice sdkmath.LegacyDec
		expPass        bool
	}{
		{
			"fail - Can't get FeeMarket params",
			func() {
				feeMarketCleint := suite.backend.queryClient.FeeMarket.(*mocks.FeeMarketQueryClient)
				RegisterFeeMarketParamsError(feeMarketCleint, int64(1))
			},
			sdkmath.LegacyZeroDec(),
			false,
		},
	}

	for _, tc := range testCases {
		suite.Run(fmt.Sprintf("case %s", tc.name), func() {
			suite.SetupTest() // reset test and queries
			tc.registerMock()

			globalMinGasPrice, err := suite.backend.GlobalMinGasPrice()

			if tc.expPass {
				suite.Require().Equal(tc.expMinGasPrice, globalMinGasPrice)
			} else {
				suite.Require().Error(err)
			}
		})
	}
}

// TestGasPriceMeetsRegularFloor is a regression test for the bug where the RPC
// advertised MinGasPrice (2.25e10) while the ante chain enforced
// MinRegularGasPrice (2.5e12), so every wallet-produced transaction was rejected
// with "requires a gas price of at least 2500000000000 arune".
//
// It asserts the two properties a wallet depends on:
//
//  1. eth_gasPrice >= MinRegularGasPrice          (legacy txs)
//  2. tip + baseFee >= MinRegularGasPrice         (EIP-1559 txs, where the
//     effective price is min(feeCap, tipCap+baseFee) and therefore CANNOT be
//     rescued by raising maxFeePerGas alone)
func (suite *BackendTestSuite) TestGasPriceMeetsRegularFloor() {
	testCases := []struct {
		name          string
		minRegular    sdkmath.LegacyDec
		minDeployment sdkmath.LegacyDec
	}{
		{
			name:          "hoodium defaults 2.5e12 / 5e12",
			minRegular:    sdkmath.LegacyNewDec(2_500_000_000_000),
			minDeployment: sdkmath.LegacyNewDec(5_000_000_000_000),
		},
		{
			name:          "equal floors 2e10 / 4e10",
			minRegular:    sdkmath.LegacyNewDec(20_000_000_000),
			minDeployment: sdkmath.LegacyNewDec(40_000_000_000),
		},
	}

	for _, tc := range testCases {
		suite.Run(fmt.Sprintf("case %s", tc.name), func() {
			suite.SetupTest() // reset test and queries

			// Mirror TestGasPrice's wiring: CurrentHeader() resolves through
			// HeaderByNumber, which needs the block and block-results mocks.
			var header metadata.MD
			client := suite.backend.clientCtx.Client.(*mocks.Client)
			queryClient := suite.backend.queryClient.QueryClient.(*mocks.EVMQueryClient)
			feeMarketClient := suite.backend.queryClient.FeeMarket.(*mocks.FeeMarketQueryClient)

			RegisterFeeMarketParamsWithFloors(feeMarketClient, 1, tc.minRegular, tc.minDeployment)
			RegisterParams(queryClient, &header, 1)
			_, err := RegisterBlock(client, 1, nil)
			suite.Require().NoError(err)
			_, err = RegisterBlockResults(client, 1)
			suite.Require().NoError(err)
			RegisterBaseFee(queryClient, sdkmath.NewInt(1))

			hdr := suite.backend.CurrentHeader()
			suite.Require().NotNil(hdr)

			floor := tc.minRegular.TruncateInt().BigInt()

			// 1. Legacy price: eth_gasPrice must clear the regular floor.
			gasPrice, err := suite.backend.GasPrice()
			suite.Require().NoError(err)
			suite.Require().GreaterOrEqual(
				(*big.Int)(gasPrice).Cmp(floor), 0,
				"eth_gasPrice %s is below MinRegularGasPrice %s", gasPrice.ToInt(), floor,
			)

			// 2. EIP-1559: the effective price the chain will actually charge is
			//    min(feeCap, tipCap+baseFee), so tipCap+baseFee alone must clear
			//    the floor — raising maxFeePerGas cannot rescue it.
			baseFee := hdr.BaseFee
			if baseFee == nil {
				baseFee = big.NewInt(0)
			}

			tip, err := suite.backend.SuggestGasTipCap(baseFee)
			suite.Require().NoError(err)

			effective := new(big.Int).Add(tip, baseFee)
			suite.Require().GreaterOrEqual(
				effective.Cmp(floor), 0,
				"tip %s + baseFee %s = %s is below MinRegularGasPrice %s",
				tip, baseFee, effective, floor,
			)
		})
	}
}

func (suite *BackendTestSuite) TestFeeHistory() {
	testCases := []struct {
		name           string
		registerMock   func(validator sdk.AccAddress)
		userBlockCount math.HexOrDecimal64
		latestBlock    ethrpc.BlockNumber
		expFeeHistory  *rpc.FeeHistoryResult
		validator      sdk.AccAddress
		expPass        bool
	}{
		{
			"fail - can't get params ",
			func(_ sdk.AccAddress) {
				var header metadata.MD
				queryClient := suite.backend.queryClient.QueryClient.(*mocks.EVMQueryClient)
				suite.backend.cfg.JSONRPC.FeeHistoryCap = 0
				RegisterParamsError(queryClient, &header, ethrpc.BlockNumber(1).Int64())
			},
			1,
			-1,
			nil,
			nil,
			false,
		},
		{
			"fail - user block count higher than max block count ",
			func(_ sdk.AccAddress) {
				var header metadata.MD
				queryClient := suite.backend.queryClient.QueryClient.(*mocks.EVMQueryClient)
				suite.backend.cfg.JSONRPC.FeeHistoryCap = 0
				RegisterParams(queryClient, &header, ethrpc.BlockNumber(1).Int64())
			},
			1,
			-1,
			nil,
			nil,
			false,
		},
		{
			"fail - Tendermint block fetching error ",
			func(_ sdk.AccAddress) {
				client := suite.backend.clientCtx.Client.(*mocks.Client)
				suite.backend.cfg.JSONRPC.FeeHistoryCap = 2
				RegisterBlockError(client, ethrpc.BlockNumber(1).Int64())
			},
			1,
			1,
			nil,
			nil,
			false,
		},
		{
			"fail - Eth block fetching error",
			func(_ sdk.AccAddress) {
				client := suite.backend.clientCtx.Client.(*mocks.Client)
				suite.backend.cfg.JSONRPC.FeeHistoryCap = 2
				_, err := RegisterBlock(client, ethrpc.BlockNumber(1).Int64(), nil)
				suite.Require().NoError(err)
				RegisterBlockResultsError(client, 1)
			},
			1,
			1,
			nil,
			nil,
			true,
		},
		{
			"fail - Invalid base fee",
			func(validator sdk.AccAddress) {
				// baseFee := sdkmath.NewInt(1)
				queryClient := suite.backend.queryClient.QueryClient.(*mocks.EVMQueryClient)
				client := suite.backend.clientCtx.Client.(*mocks.Client)
				suite.backend.cfg.JSONRPC.FeeHistoryCap = 2
				_, err := RegisterBlock(client, ethrpc.BlockNumber(1).Int64(), nil)
				suite.Require().NoError(err)
				_, err = RegisterBlockResults(client, 1)
				suite.Require().NoError(err)
				RegisterBaseFeeError(queryClient)
				RegisterValidatorAccount(queryClient, validator)
				RegisterConsensusParams(client, 1)
			},
			1,
			1,
			nil,
			sdk.AccAddress(utiltx.GenerateAddress().Bytes()),
			false,
		},
		{
			"pass - Valid FeeHistoryResults object",
			func(validator sdk.AccAddress) {
				var header metadata.MD
				baseFee := sdkmath.NewInt(1)
				queryClient := suite.backend.queryClient.QueryClient.(*mocks.EVMQueryClient)
				client := suite.backend.clientCtx.Client.(*mocks.Client)
				suite.backend.cfg.JSONRPC.FeeHistoryCap = 2
				_, err := RegisterBlock(client, ethrpc.BlockNumber(1).Int64(), nil)
				suite.Require().NoError(err)
				_, err = RegisterBlockResults(client, 1)
				suite.Require().NoError(err)
				RegisterBaseFee(queryClient, baseFee)
				RegisterValidatorAccount(queryClient, validator)
				RegisterConsensusParams(client, 1)
				RegisterParams(queryClient, &header, 1)
				RegisterParamsWithoutHeader(queryClient, 1)
			},
			1,
			1,
			&rpc.FeeHistoryResult{
				OldestBlock:  (*hexutil.Big)(big.NewInt(1)),
				BaseFee:      []*hexutil.Big{(*hexutil.Big)(big.NewInt(1)), (*hexutil.Big)(big.NewInt(1))},
				GasUsedRatio: []float64{0},
				Reward:       [][]*hexutil.Big{{(*hexutil.Big)(big.NewInt(0)), (*hexutil.Big)(big.NewInt(0)), (*hexutil.Big)(big.NewInt(0)), (*hexutil.Big)(big.NewInt(0))}},
			},
			sdk.AccAddress(utiltx.GenerateAddress().Bytes()),
			true,
		},
	}

	for _, tc := range testCases {
		suite.Run(fmt.Sprintf("case %s", tc.name), func() {
			suite.SetupTest() // reset test and queries
			tc.registerMock(tc.validator)

			feeHistory, err := suite.backend.FeeHistory(tc.userBlockCount, tc.latestBlock, []float64{25, 50, 75, 100})
			if tc.expPass {
				suite.Require().NoError(err)
				suite.Require().Equal(feeHistory, tc.expFeeHistory)
			} else {
				suite.Require().Error(err)
			}
		})
	}
}
