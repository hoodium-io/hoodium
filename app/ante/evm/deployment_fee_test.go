package evm

import (
	"math/big"
	"testing"

	sdkmath "cosmossdk.io/math"
	"github.com/stretchr/testify/require"

	"cosmossdk.io/log"
	tmproto "github.com/cometbft/cometbft/proto/tendermint/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/ethereum/go-ethereum/common"

	evmtypes "github.com/hoodium-io/hoodium/x/evm/types"
	feemarkettypes "github.com/hoodium-io/hoodium/x/feemarket/types"
)

// MockFeeMarketKeeper is a minimal FeeMarketKeeper test double backing the
// Hoodium two-gas-price params.
type MockFeeMarketKeeper struct {
	Params    feemarkettypes.Params
	BaseFeeOn bool
}

func (m MockFeeMarketKeeper) GetParams(_ sdk.Context) feemarkettypes.Params { return m.Params }
func (m MockFeeMarketKeeper) AddTransientGasWanted(_ sdk.Context, _ uint64) (uint64, error) {
	return 0, nil
}
func (m MockFeeMarketKeeper) GetBaseFeeEnabled(_ sdk.Context) bool { return m.BaseFeeOn }

// buildMsgEthereumTx constructs a legacy MsgEthereumTx with the given gas price.
// When isDeploy is true the `to` address is nil (contract creation).
func buildMsgEthereumTx(t *testing.T, gasPrice int64, isDeploy bool) sdk.Tx {
	t.Helper()

	var to *common.Address
	if !isDeploy {
		addr := common.HexToAddress("0x1234567890123456789012345678901234567890")
		to = &addr
	}

	gp := big.NewInt(gasPrice)
	msg := evmtypes.NewTx(&evmtypes.EvmTxArgs{
		ChainID:  big.NewInt(6591),
		Nonce:    0,
		GasLimit: 21000,
		GasPrice: gp,
		To:       to,
		Amount:   big.NewInt(0),
	})

	return sdk.Tx(msg)
}

// twoGasPriceParams returns feemarket params with the Hoodium two-gas-price
// minimums set (in arune): regular 2.5e12, deployment 5e12.
func twoGasPriceParams() feemarkettypes.Params {
	p := feemarkettypes.DefaultParams()
	p.MinRegularGasPrice = sdkmath.LegacyNewDec(2_500_000_000_000) // 0.0000025 RUNE
	p.MinDeploymentGasPrice = sdkmath.LegacyNewDec(5_000_000_000_000)
	return p
}

func TestEthDeploymentGasPriceDecorator(t *testing.T) {
	ctx := sdk.NewContext(nil, tmproto.Header{Height: 1}, false, log.NewNopLogger())
	keeper := MockEVMKeeper{EnableLondonHF: false}
	feeMarket := MockFeeMarketKeeper{Params: twoGasPriceParams()}

	next := func(ctx sdk.Context, _ sdk.Tx, _ bool) (sdk.Context, error) { return ctx, nil }
	decorator := NewEthDeploymentGasPriceDecorator(keeper, feeMarket)

	// regular min = 2.5e12 arune/gas; deployment min = 5e12 arune/gas.
	const (
		belowRegular = int64(1e12) // < regular min
		atRegular    = int64(25e11)
		belowDeploy  = int64(4e12) // > regular min, < deploy min
		atDeploy     = int64(5e12)
	)

	testCases := []struct {
		name     string
		gasPrice int64
		isDeploy bool
		wantErr  bool
	}{
		{"regular below min rejected", belowRegular, false, true},
		{"regular at min accepted", atRegular, false, false},
		{"deployment below deploy min rejected", belowDeploy, true, true},
		{"deployment at deploy min accepted", atDeploy, true, false},
		{"regular at deploy price accepted", atDeploy, false, false},
		{"deployment at regular price rejected", atRegular, true, true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tx := buildMsgEthereumTx(t, tc.gasPrice, tc.isDeploy)
			_, err := decorator.AnteHandle(ctx, tx, false, next)
			if tc.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// TestEthDeploymentGasPriceDecoratorNilBaseFee is a regression test: with London
// disabled the base fee is nil, and the decorator must NOT panic (it previously
// called GetEffectiveFee(nil), which nil-dereferenced).
func TestEthDeploymentGasPriceDecoratorNilBaseFee(t *testing.T) {
	ctx := sdk.NewContext(nil, tmproto.Header{Height: 1}, false, log.NewNopLogger())
	keeper := MockEVMKeeper{EnableLondonHF: false}
	feeMarket := MockFeeMarketKeeper{Params: twoGasPriceParams()}
	decorator := NewEthDeploymentGasPriceDecorator(keeper, feeMarket)
	next := func(ctx sdk.Context, _ sdk.Tx, _ bool) (sdk.Context, error) { return ctx, nil }

	for _, isDeploy := range []bool{false, true} {
		tx := buildMsgEthereumTx(t, 5e12, isDeploy)
		require.NotPanics(t, func() {
			_, _ = decorator.AnteHandle(ctx, tx, false, next)
		})
	}
}

// TestEthDeploymentGasPriceDecoratorDisabled checks that zero minimums disable
// the check entirely.
func TestEthDeploymentGasPriceDecoratorDisabled(t *testing.T) {
	ctx := sdk.NewContext(nil, tmproto.Header{Height: 1}, false, log.NewNopLogger())
	keeper := MockEVMKeeper{EnableLondonHF: false}

	p := feemarkettypes.DefaultParams()
	p.MinRegularGasPrice = sdkmath.LegacyZeroDec()
	p.MinDeploymentGasPrice = sdkmath.LegacyZeroDec()
	decorator := NewEthDeploymentGasPriceDecorator(keeper, MockFeeMarketKeeper{Params: p})
	next := func(ctx sdk.Context, _ sdk.Tx, _ bool) (sdk.Context, error) { return ctx, nil }

	tx := buildMsgEthereumTx(t, 1, true)
	_, err := decorator.AnteHandle(ctx, tx, false, next)
	require.NoError(t, err)
}
