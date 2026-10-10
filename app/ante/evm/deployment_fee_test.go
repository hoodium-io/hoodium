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
)

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

func TestEthDeploymentGasPriceDecorator(t *testing.T) {
	// Context with London disabled (base fee nil) to exercise the nil-guard too.
	ctx := sdk.NewContext(nil, tmproto.Header{Height: 1}, false, log.NewNopLogger())
	keeper := MockEVMKeeper{EnableLondonHF: false}

	// A no-op next handler so we only test this decorator.
	next := func(ctx sdk.Context, _ sdk.Tx, _ bool) (sdk.Context, error) { return ctx, nil }

	decorator := NewEthDeploymentGasPriceDecorator(keeper)

	// RegularGasPriceMin    = 0.0001 RUNE/gas = 1e14 arune/gas.
	// DeploymentGasPriceMin = 0.01   RUNE/gas = 1e16 arune/gas.
	const (
		belowRegular = int64(1e13) // 0.00001 RUNE/gas (< regular min)
		atRegular    = int64(1e14) // 0.0001  RUNE/gas (= regular min)
		belowDeploy  = int64(1e15) // 0.001   RUNE/gas (> regular min, < deploy min)
		atDeploy     = int64(1e16) // 0.01    RUNE/gas (= deploy min)
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
	decorator := NewEthDeploymentGasPriceDecorator(keeper)
	next := func(ctx sdk.Context, _ sdk.Tx, _ bool) (sdk.Context, error) { return ctx, nil }

	for _, isDeploy := range []bool{false, true} {
		tx := buildMsgEthereumTx(t, atDeployPrice(), isDeploy)
		require.NotPanics(t, func() {
			_, _ = decorator.AnteHandle(ctx, tx, false, next)
		})
	}
}

// atDeployPrice returns a gas price at/above the deployment minimum so the tx
// passes and the decorator reaches its fee math (the code path that used to
// panic).
func atDeployPrice() int64 { return 1e16 }

func TestValidateTwoGasPriceConfig(t *testing.T) {
	require.NoError(t, ValidateTwoGasPriceConfig())

	// Constants are in the base denomination (arune).
	require.Equal(t, sdkmath.LegacyNewDec(100_000_000_000_000), RegularGasPriceMin)       // 1e14 arune = 0.0001 RUNE
	require.Equal(t, sdkmath.LegacyNewDec(10_000_000_000_000_000), DeploymentGasPriceMin) // 1e16 arune = 0.01 RUNE
	require.True(t, DeploymentGasPriceMin.GT(RegularGasPriceMin))
}
