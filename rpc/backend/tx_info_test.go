package backend

import (
	"fmt"
	"math/big"

	sdkmath "cosmossdk.io/math"

	"cosmossdk.io/log"
	abci "github.com/cometbft/cometbft/abci/types"
	tmrpctypes "github.com/cometbft/cometbft/rpc/core/types"
	"github.com/cometbft/cometbft/types"
	dbm "github.com/cosmos/cosmos-db"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/holiman/uint256"
	"github.com/hoodium-io/hoodium/indexer"
	"github.com/hoodium-io/hoodium/rpc/backend/mocks"
	rpctypes "github.com/hoodium-io/hoodium/rpc/types"
	utiltx "github.com/hoodium-io/hoodium/testutil/tx"
	mezotypes "github.com/hoodium-io/hoodium/types"
	evmtypes "github.com/hoodium-io/hoodium/x/evm/types"
	"google.golang.org/grpc/metadata"
)

func (suite *BackendTestSuite) TestGetTransactionByHash() {
	msgEthereumTx, _ := suite.buildEthereumTx()
	ethTxHash := msgEthereumTx.AsTransaction().Hash()

	txBz := suite.signAndEncodeEthTx(msgEthereumTx)
	block := &types.Block{Header: types.Header{Height: 1, ChainID: "test"}, Data: types.Data{Txs: []types.Tx{txBz}}}
	responseDeliver := []*abci.ExecTxResult{
		{
			Code: 0,
			Events: []abci.Event{
				{Type: evmtypes.EventTypeEthereumTx, Attributes: []abci.EventAttribute{
					{Key: "ethereumTxHash", Value: ethTxHash.Hex()},
					{Key: "txIndex", Value: "0"},
					{Key: "amount", Value: "1000"},
					{Key: "txGasUsed", Value: "21000"},
					{Key: "txHash", Value: ""},
					{Key: "recipient", Value: ""},
				}},
			},
		},
	}
	txHash := common.HexToHash(msgEthereumTx.Hash)

	rpcTransaction, _ := rpctypes.NewRPCTransaction(msgEthereumTx.AsTransaction(), common.Hash{}, 0, 0, big.NewInt(1), suite.backend.chainID)

	testCases := []struct {
		name         string
		registerMock func()
		block        *types.Block
		txHash       common.Hash
		expRPCTx     *rpctypes.RPCTransaction
		expPass      bool
	}{
		{
			"fail - Block error",
			func() {
				client := suite.backend.clientCtx.Client.(*mocks.Client)
				RegisterBlockError(client, 1)
			},
			block,
			txHash,
			rpcTransaction,
			false,
		},
		{
			"fail - Block Result error",
			func() {
				client := suite.backend.clientCtx.Client.(*mocks.Client)
				_, err := RegisterBlock(client, 1, txBz)
				suite.Require().NoError(err)
				RegisterBlockResultsError(client, 1)
			},
			block,
			txHash,
			nil,
			true,
		},
		{
			"pass - Base fee error",
			func() {
				client := suite.backend.clientCtx.Client.(*mocks.Client)
				queryClient := suite.backend.queryClient.QueryClient.(*mocks.EVMQueryClient)
				_, err := RegisterBlock(client, 1, txBz)
				suite.Require().NoError(err)
				_, err = RegisterBlockResults(client, 1)
				suite.Require().NoError(err)
				RegisterBaseFeeError(queryClient)
			},
			block,
			txHash,
			rpcTransaction,
			true,
		},
		{
			"pass - Transaction found and returned",
			func() {
				client := suite.backend.clientCtx.Client.(*mocks.Client)
				queryClient := suite.backend.queryClient.QueryClient.(*mocks.EVMQueryClient)
				_, err := RegisterBlock(client, 1, txBz)
				suite.Require().NoError(err)
				_, err = RegisterBlockResults(client, 1)
				suite.Require().NoError(err)
				RegisterBaseFee(queryClient, sdkmath.NewInt(1))
			},
			block,
			txHash,
			rpcTransaction,
			true,
		},
	}
	for _, tc := range testCases {
		suite.Run(tc.name, func() {
			suite.SetupTest() // reset
			tc.registerMock()

			db := dbm.NewMemDB()
			suite.backend.indexer = indexer.NewKVIndexer(db, log.NewNopLogger(), suite.backend.clientCtx)
			err := suite.backend.indexer.IndexBlock(tc.block, responseDeliver)
			suite.Require().NoError(err)

			rpcTx, err := suite.backend.GetTransactionByHash(tc.txHash)

			if tc.expPass {
				suite.Require().NoError(err)
				suite.Require().Equal(rpcTx, tc.expRPCTx)
			} else {
				suite.Require().Error(err)
			}
		})
	}
}

func (suite *BackendTestSuite) TestGetTransactionsByHashPending() {
	msgEthereumTx, bz := suite.buildEthereumTx()
	rpcTransaction, _ := rpctypes.NewRPCTransaction(msgEthereumTx.AsTransaction(), common.Hash{}, 0, 0, big.NewInt(1), suite.backend.chainID)

	testCases := []struct {
		name         string
		registerMock func()
		tx           *evmtypes.MsgEthereumTx
		expRPCTx     *rpctypes.RPCTransaction
		expPass      bool
	}{
		{
			"fail - Pending transactions returns error",
			func() {
				client := suite.backend.clientCtx.Client.(*mocks.Client)
				RegisterUnconfirmedTxsError(client, nil)
			},
			msgEthereumTx,
			nil,
			true,
		},
		{
			"fail - Tx not found return nil",
			func() {
				client := suite.backend.clientCtx.Client.(*mocks.Client)
				RegisterUnconfirmedTxs(client, nil, nil)
			},
			msgEthereumTx,
			nil,
			true,
		},
		{
			"pass - Tx found and returned",
			func() {
				client := suite.backend.clientCtx.Client.(*mocks.Client)
				RegisterUnconfirmedTxs(client, nil, types.Txs{bz})
			},
			msgEthereumTx,
			rpcTransaction,
			true,
		},
	}

	for _, tc := range testCases {
		suite.Run(tc.name, func() {
			suite.SetupTest() // reset
			tc.registerMock()

			rpcTx, err := suite.backend.getTransactionByHashPending(common.HexToHash(tc.tx.Hash))

			if tc.expPass {
				suite.Require().NoError(err)
				suite.Require().Equal(rpcTx, tc.expRPCTx)
			} else {
				suite.Require().Error(err)
			}
		})
	}
}

func (suite *BackendTestSuite) TestGetTxByEthHash() {
	msgEthereumTx, bz := suite.buildEthereumTx()
	rpcTransaction, _ := rpctypes.NewRPCTransaction(msgEthereumTx.AsTransaction(), common.Hash{}, 0, 0, big.NewInt(1), suite.backend.chainID)

	testCases := []struct {
		name         string
		registerMock func()
		tx           *evmtypes.MsgEthereumTx
		expRPCTx     *rpctypes.RPCTransaction
		expPass      bool
	}{
		{
			"fail - Indexer disabled can't find transaction",
			func() {
				suite.backend.indexer = nil
				client := suite.backend.clientCtx.Client.(*mocks.Client)
				query := fmt.Sprintf("%s.%s='%s'", evmtypes.TypeMsgEthereumTx, evmtypes.AttributeKeyEthereumTxHash, common.HexToHash(msgEthereumTx.Hash).Hex())
				RegisterTxSearch(client, query, bz)
			},
			msgEthereumTx,
			rpcTransaction,
			false,
		},
	}

	for _, tc := range testCases {
		suite.Run(tc.name, func() {
			suite.SetupTest() // reset
			tc.registerMock()

			rpcTx, err := suite.backend.GetTxByEthHash(common.HexToHash(tc.tx.Hash))

			if tc.expPass {
				suite.Require().NoError(err)
				suite.Require().Equal(rpcTx, tc.expRPCTx)
			} else {
				suite.Require().Error(err)
			}
		})
	}
}

func (suite *BackendTestSuite) TestGetTransactionByBlockHashAndIndex() {
	_, bz := suite.buildEthereumTx()

	testCases := []struct {
		name         string
		registerMock func()
		blockHash    common.Hash
		expRPCTx     *rpctypes.RPCTransaction
		expPass      bool
	}{
		{
			"pass - block not found",
			func() {
				client := suite.backend.clientCtx.Client.(*mocks.Client)
				RegisterBlockByHashError(client, common.Hash{}, bz)
			},
			common.Hash{},
			nil,
			true,
		},
		{
			"pass - Block results error",
			func() {
				client := suite.backend.clientCtx.Client.(*mocks.Client)
				_, err := RegisterBlockByHash(client, common.Hash{}, bz)
				suite.Require().NoError(err)
				RegisterBlockResultsError(client, 1)
			},
			common.Hash{},
			nil,
			true,
		},
	}

	for _, tc := range testCases {
		suite.Run(tc.name, func() {
			suite.SetupTest() // reset
			tc.registerMock()

			rpcTx, err := suite.backend.GetTransactionByBlockHashAndIndex(tc.blockHash, 1)

			if tc.expPass {
				suite.Require().NoError(err)
				suite.Require().Equal(rpcTx, tc.expRPCTx)
			} else {
				suite.Require().Error(err)
			}
		})
	}
}

func (suite *BackendTestSuite) TestGetTransactionByBlockAndIndex() {
	msgEthTx, bz := suite.buildEthereumTx()

	defaultBlock := types.MakeBlock(1, []types.Tx{bz}, nil, nil)
	defaultExecTxResult := []*abci.ExecTxResult{
		{
			Code: 0,
			Events: []abci.Event{
				{Type: evmtypes.EventTypeEthereumTx, Attributes: []abci.EventAttribute{
					{Key: "ethereumTxHash", Value: common.HexToHash(msgEthTx.Hash).Hex()},
					{Key: "txIndex", Value: "0"},
					{Key: "amount", Value: "1000"},
					{Key: "txGasUsed", Value: "21000"},
					{Key: "txHash", Value: ""},
					{Key: "recipient", Value: ""},
				}},
			},
		},
	}

	txFromMsg, _ := rpctypes.NewTransactionFromMsg(
		msgEthTx,
		common.BytesToHash(defaultBlock.Hash().Bytes()),
		1,
		0,
		big.NewInt(1),
		suite.backend.chainID,
	)

	testCases := []struct {
		name         string
		registerMock func()
		block        *tmrpctypes.ResultBlock
		idx          hexutil.Uint
		expRPCTx     *rpctypes.RPCTransaction
		expPass      bool
	}{
		{
			"pass - block txs index out of bound",
			func() {
				client := suite.backend.clientCtx.Client.(*mocks.Client)
				_, err := RegisterBlockResults(client, 1)
				suite.Require().NoError(err)
			},
			&tmrpctypes.ResultBlock{Block: types.MakeBlock(1, []types.Tx{bz}, nil, nil)},
			1,
			nil,
			true,
		},
		{
			"pass - Can't fetch base fee",
			func() {
				queryClient := suite.backend.queryClient.QueryClient.(*mocks.EVMQueryClient)
				client := suite.backend.clientCtx.Client.(*mocks.Client)
				_, err := RegisterBlockResults(client, 1)
				suite.Require().NoError(err)
				RegisterBaseFeeError(queryClient)
			},
			&tmrpctypes.ResultBlock{Block: defaultBlock},
			0,
			txFromMsg,
			true,
		},
		{
			"pass - Gets Tx by transaction index",
			func() {
				queryClient := suite.backend.queryClient.QueryClient.(*mocks.EVMQueryClient)
				client := suite.backend.clientCtx.Client.(*mocks.Client)
				db := dbm.NewMemDB()
				suite.backend.indexer = indexer.NewKVIndexer(db, log.NewNopLogger(), suite.backend.clientCtx)
				txBz := suite.signAndEncodeEthTx(msgEthTx)
				block := &types.Block{Header: types.Header{Height: 1, ChainID: "test"}, Data: types.Data{Txs: []types.Tx{txBz}}}
				err := suite.backend.indexer.IndexBlock(block, defaultExecTxResult)
				suite.Require().NoError(err)
				_, err = RegisterBlockResults(client, 1)
				suite.Require().NoError(err)
				RegisterBaseFee(queryClient, sdkmath.NewInt(1))
			},
			&tmrpctypes.ResultBlock{Block: defaultBlock},
			0,
			txFromMsg,
			true,
		},
		{
			"pass - returns the Ethereum format transaction by the Ethereum hash",
			func() {
				queryClient := suite.backend.queryClient.QueryClient.(*mocks.EVMQueryClient)
				client := suite.backend.clientCtx.Client.(*mocks.Client)
				_, err := RegisterBlockResults(client, 1)
				suite.Require().NoError(err)
				RegisterBaseFee(queryClient, sdkmath.NewInt(1))
			},
			&tmrpctypes.ResultBlock{Block: defaultBlock},
			0,
			txFromMsg,
			true,
		},
	}

	for _, tc := range testCases {
		suite.Run(tc.name, func() {
			suite.SetupTest() // reset
			tc.registerMock()

			rpcTx, err := suite.backend.GetTransactionByBlockAndIndex(tc.block, tc.idx)

			if tc.expPass {
				suite.Require().NoError(err)
				suite.Require().Equal(rpcTx, tc.expRPCTx)
			} else {
				suite.Require().Error(err)
			}
		})
	}
}

func (suite *BackendTestSuite) TestGetTransactionByBlockNumberAndIndex() {
	msgEthTx, bz := suite.buildEthereumTx()
	defaultBlock := types.MakeBlock(1, []types.Tx{bz}, nil, nil)
	txFromMsg, _ := rpctypes.NewTransactionFromMsg(
		msgEthTx,
		common.BytesToHash(defaultBlock.Hash().Bytes()),
		1,
		0,
		big.NewInt(1),
		suite.backend.chainID,
	)

	testCases := []struct {
		name         string
		registerMock func()
		blockNum     rpctypes.BlockNumber
		idx          hexutil.Uint
		expRPCTx     *rpctypes.RPCTransaction
		expPass      bool
	}{
		{
			"fail -  block not found return nil",
			func() {
				client := suite.backend.clientCtx.Client.(*mocks.Client)
				RegisterBlockError(client, 1)
			},
			0,
			0,
			nil,
			true,
		},
		{
			"pass - returns the transaction identified by block number and index",
			func() {
				client := suite.backend.clientCtx.Client.(*mocks.Client)
				queryClient := suite.backend.queryClient.QueryClient.(*mocks.EVMQueryClient)
				_, err := RegisterBlock(client, 1, bz)
				suite.Require().NoError(err)
				_, err = RegisterBlockResults(client, 1)
				suite.Require().NoError(err)
				RegisterBaseFee(queryClient, sdkmath.NewInt(1))
			},
			0,
			0,
			txFromMsg,
			true,
		},
	}

	for _, tc := range testCases {
		suite.Run(tc.name, func() {
			suite.SetupTest() // reset
			tc.registerMock()

			rpcTx, err := suite.backend.GetTransactionByBlockNumberAndIndex(tc.blockNum, tc.idx)
			if tc.expPass {
				suite.Require().NoError(err)
				suite.Require().Equal(rpcTx, tc.expRPCTx)
			} else {
				suite.Require().Error(err)
			}
		})
	}
}

func (suite *BackendTestSuite) TestQueryTendermintTxIndexer() {
	testCases := []struct {
		name         string
		registerMock func()
		txGetter     func(*rpctypes.ParsedTxs) *rpctypes.ParsedTx
		query        string
		expTxResult  *mezotypes.TxResult
		expPass      bool
	}{
		{
			"fail - Ethereum tx with query not found",
			func() {
				client := suite.backend.clientCtx.Client.(*mocks.Client)
				RegisterTxSearchEmpty(client, "")
			},
			func(_ *rpctypes.ParsedTxs) *rpctypes.ParsedTx {
				return &rpctypes.ParsedTx{}
			},
			"",
			&mezotypes.TxResult{},
			false,
		},
	}

	for _, tc := range testCases {
		suite.Run(tc.name, func() {
			suite.SetupTest() // reset
			tc.registerMock()

			txResults, err := suite.backend.queryTendermintTxIndexer(tc.query, tc.txGetter)

			if tc.expPass {
				suite.Require().NoError(err)
				suite.Require().Equal(txResults, tc.expTxResult)
			} else {
				suite.Require().Error(err)
			}
		})
	}
}

func (suite *BackendTestSuite) TestGetTransactionReceipt() {
	msgEthereumTx, _ := suite.buildEthereumTx()
	txHash := msgEthereumTx.AsTransaction().Hash()

	txBz := suite.signAndEncodeEthTx(msgEthereumTx)

	testCases := []struct {
		name         string
		registerMock func()
		txHash       common.Hash
		block        *types.Block
		blockResult  []*abci.ExecTxResult
		expTxReceipt map[string]interface{}
		expPass      bool
	}{
		{
			"fail - Receipts do not match",
			func() {
				var header metadata.MD
				queryClient := suite.backend.queryClient.QueryClient.(*mocks.EVMQueryClient)
				client := suite.backend.clientCtx.Client.(*mocks.Client)
				RegisterParams(queryClient, &header, 1)
				RegisterParamsWithoutHeader(queryClient, 1)
				_, err := RegisterBlock(client, 1, txBz)
				suite.Require().NoError(err)
				_, err = RegisterBlockResults(client, 1)
				suite.Require().NoError(err)
			},
			common.HexToHash(msgEthereumTx.Hash),
			&types.Block{Header: types.Header{Height: 1}, Data: types.Data{Txs: []types.Tx{txBz}}},
			[]*abci.ExecTxResult{
				{
					Code: 0,
					Events: []abci.Event{
						{Type: evmtypes.EventTypeEthereumTx, Attributes: []abci.EventAttribute{
							{Key: "ethereumTxHash", Value: txHash.Hex()},
							{Key: "txIndex", Value: "0"},
							{Key: "amount", Value: "1000"},
							{Key: "txGasUsed", Value: "21000"},
							{Key: "txHash", Value: ""},
							{Key: "recipient", Value: "0x775b87ef5D82ca211811C1a02CE0fE0CA3a455d7"},
						}},
					},
				},
			},
			map[string]interface{}(nil),
			false,
		},
	}

	for _, tc := range testCases {
		suite.Run(tc.name, func() {
			suite.SetupTest() // reset
			tc.registerMock()

			db := dbm.NewMemDB()
			suite.backend.indexer = indexer.NewKVIndexer(db, log.NewNopLogger(), suite.backend.clientCtx)
			err := suite.backend.indexer.IndexBlock(tc.block, tc.blockResult)
			suite.Require().NoError(err)

			txReceipt, err := suite.backend.GetTransactionReceipt(tc.txHash)
			if tc.expPass {
				suite.Require().NoError(err)
				suite.Require().Equal(txReceipt, tc.expTxReceipt)
			} else {
				suite.Require().NotEqual(txReceipt, tc.expTxReceipt)
			}
		})
	}
}

func (suite *BackendTestSuite) TestGetGasUsed() {
	origin := suite.backend.cfg.JSONRPC.FixRevertGasRefundHeight
	testCases := []struct {
		name                     string
		fixRevertGasRefundHeight int64
		txResult                 *mezotypes.TxResult
		price                    *big.Int
		gas                      uint64
		exp                      uint64
	}{
		{
			"success txResult",
			1,
			&mezotypes.TxResult{
				Height:  1,
				Failed:  false,
				GasUsed: 53026,
			},
			new(big.Int).SetUint64(0),
			0,
			53026,
		},
		{
			"fail txResult before cap",
			2,
			&mezotypes.TxResult{
				Height:  1,
				Failed:  true,
				GasUsed: 53026,
			},
			new(big.Int).SetUint64(200000),
			5000000000000,
			1000000000000000000,
		},
		{
			"fail txResult after cap",
			2,
			&mezotypes.TxResult{
				Height:  3,
				Failed:  true,
				GasUsed: 53026,
			},
			new(big.Int).SetUint64(200000),
			5000000000000,
			53026,
		},
	}
	for _, tc := range testCases {
		suite.Run(fmt.Sprintf("Case %s", tc.name), func() {
			suite.backend.cfg.JSONRPC.FixRevertGasRefundHeight = tc.fixRevertGasRefundHeight
			suite.Require().Equal(tc.exp, suite.backend.GetGasUsed(tc.txResult, tc.price, tc.gas))
			suite.backend.cfg.JSONRPC.FixRevertGasRefundHeight = origin
		})
	}
}

func (suite *BackendTestSuite) TestGetTransactionReceiptSetCodeTxEffectiveGasPrice() {
	suite.SetupTest()

	priv, err := crypto.GenerateKey()
	suite.Require().NoError(err)
	from := crypto.PubkeyToAddress(priv.PublicKey)

	to := utiltx.GenerateAddress()
	chainIDBig := suite.backend.chainID
	signer := ethtypes.NewPragueSigner(chainIDBig)

	auth, err := ethtypes.SignSetCode(priv, ethtypes.SetCodeAuthorization{
		ChainID: *uint256.MustFromBig(chainIDBig),
		Address: to,
		Nonce:   1,
	})
	suite.Require().NoError(err)

	gasFeeCap := uint256.NewInt(10)
	gasTipCap := uint256.NewInt(2)
	inner := &ethtypes.SetCodeTx{
		ChainID:   uint256.MustFromBig(chainIDBig),
		Nonce:     0,
		GasTipCap: gasTipCap,
		GasFeeCap: gasFeeCap,
		Gas:       100_000,
		To:        to,
		Value:     uint256.NewInt(0),
		AuthList:  []ethtypes.SetCodeAuthorization{auth},
	}
	ethTx := ethtypes.MustSignNewTx(priv, signer, inner)

	msgEthTx := &evmtypes.MsgEthereumTx{}
	suite.Require().NoError(msgEthTx.FromEthereumTx(ethTx))
	cosmosTx, err := msgEthTx.BuildTx(
		suite.backend.clientCtx.TxConfig.NewTxBuilder(),
		evmtypes.DefaultEVMDenom,
	)
	suite.Require().NoError(err)
	txBz, err := suite.backend.clientCtx.TxConfig.TxEncoder()(cosmosTx)
	suite.Require().NoError(err)

	const height int64 = 1
	const txGasUsed = uint64(50_000)
	ethTxHash := ethTx.Hash()

	client := suite.backend.clientCtx.Client.(*mocks.Client)
	queryClient := suite.backend.queryClient.QueryClient.(*mocks.EVMQueryClient)
	var header metadata.MD
	RegisterParams(queryClient, &header, height)
	RegisterParamsWithoutHeader(queryClient, height)
	_, err = RegisterBlock(client, height, txBz)
	suite.Require().NoError(err)

	// EthereumTx event so the indexer treats this as a regular eth tx,
	// not a pseudo-tx.
	txResult := &abci.ExecTxResult{
		Code:    0,
		GasUsed: int64(txGasUsed), //nolint:gosec
		Events: []abci.Event{
			{Type: evmtypes.EventTypeEthereumTx, Attributes: []abci.EventAttribute{
				{Key: "ethereumTxHash", Value: ethTxHash.Hex()},
				{Key: "txIndex", Value: "0"},
				{Key: "amount", Value: "0"},
				{Key: "txGasUsed", Value: fmt.Sprintf("%d", txGasUsed)},
				{Key: "txHash", Value: ""},
				{Key: "recipient", Value: to.Hex()},
			}},
		},
	}
	_, err = RegisterBlockResultsWithTxResults(client, height, []*abci.ExecTxResult{txResult})
	suite.Require().NoError(err)

	baseFee := sdkmath.NewInt(1)
	RegisterBaseFee(queryClient, baseFee)

	db := dbm.NewMemDB()
	suite.backend.indexer = indexer.NewKVIndexer(db, log.NewNopLogger(), suite.backend.clientCtx)
	block := &types.Block{
		Header: types.Header{Height: height, ChainID: ChainID},
		Data:   types.Data{Txs: []types.Tx{txBz}},
	}
	suite.Require().NoError(
		suite.backend.indexer.IndexBlock(block, []*abci.ExecTxResult{txResult}),
	)

	receipt, err := suite.backend.GetTransactionReceipt(ethTxHash)
	suite.Require().NoError(err)
	suite.Require().NotNil(receipt)

	got, ok := receipt["effectiveGasPrice"]
	suite.Require().True(ok, "effectiveGasPrice missing from SetCodeTx receipt")

	txData, err := evmtypes.UnpackTxData(msgEthTx.Data)
	suite.Require().NoError(err)
	setCodeTxData, ok := txData.(*evmtypes.SetCodeTx)
	suite.Require().True(ok, "unpacked tx data is not *evmtypes.SetCodeTx")
	wantEffective := setCodeTxData.EffectiveGasPrice(baseFee.BigInt())

	suite.Require().Equal(hexutil.Big(*wantEffective), got)
	suite.Require().Equal(hexutil.Uint(ethtypes.SetCodeTxType), receipt["type"])
	suite.Require().Equal(from, receipt["from"])
}
