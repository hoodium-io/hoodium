package backend

import (
	sdkmath "cosmossdk.io/math"

	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"
	"github.com/hoodium-io/hoodium/rpc/backend/mocks"
	rpc "github.com/hoodium-io/hoodium/rpc/types"
	feemarkettypes "github.com/hoodium-io/hoodium/x/feemarket/types"
)

var _ feemarkettypes.QueryClient = &mocks.FeeMarketQueryClient{}

// Params
func RegisterFeeMarketParams(feeMarketClient *mocks.FeeMarketQueryClient, height int64) {
	params := feemarkettypes.DefaultParams()
	// These tests cover gas-price derivation, not Hoodium's fee policy, so the
	// chain-wide minimums are disabled here. Otherwise GlobalMinGasPrice clamps
	// every computed price up to the 20 arune mainnet floor and the expected
	// values below would be meaningless. The policy itself is covered by
	// app/ante/evm/deployment_fee_test.go.
	params.MinGasPrice = sdkmath.LegacyZeroDec()
	params.MinRegularGasPrice = sdkmath.LegacyZeroDec()
	params.MinDeploymentGasPrice = sdkmath.LegacyZeroDec()

	feeMarketClient.On("Params", rpc.ContextWithHeight(height), &feemarkettypes.QueryParamsRequest{}).
		Return(&feemarkettypes.QueryParamsResponse{Params: params}, nil)
}

func RegisterFeeMarketParamsError(feeMarketClient *mocks.FeeMarketQueryClient, height int64) {
	feeMarketClient.On("Params", rpc.ContextWithHeight(height), &feemarkettypes.QueryParamsRequest{}).
		Return(nil, sdkerrors.ErrInvalidRequest)
}
