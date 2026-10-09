package evm

import (
	"math/big"

	errorsmod "cosmossdk.io/errors"
	sdkmath "cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
	errortypes "github.com/cosmos/cosmos-sdk/types/errors"
	ethtypes "github.com/ethereum/go-ethereum/core/types"

	evmtypes "github.com/hoodium-io/hoodium/x/evm/types"
)

// EthDeploymentGasPriceDecorator enforces Hoodium's two-gas-price model: a
// contract-deployment transaction must pay a higher minimum gas price than a
// regular transaction.
//
// Both minimums are compile-time genesis constants (see deployment_fee_config.go):
//
//	RegularGasPriceMin       - normal EVM txs
//	DeploymentGasPriceMin    - txs that deploy a contract (empty `to`)
//
// The check is a floor: it rejects a tx whose effective gas price is below the
// applicable minimum. It does not cap fees; the EIP-1559 base fee may still push
// the effective price above the floor under congestion.
//
// A tx is treated as a contract deployment when its `to` address is nil
// (standard EVM CREATE semantics; EIP-7702 set-code txs carry a populated `to`
// and are therefore NOT treated as deployments).
type EthDeploymentGasPriceDecorator struct {
	evmKeeper DynamicFeeEVMKeeper
}

// NewEthDeploymentGasPriceDecorator creates a new
// EthDeploymentGasPriceDecorator.
func NewEthDeploymentGasPriceDecorator(ek DynamicFeeEVMKeeper) EthDeploymentGasPriceDecorator {
	return EthDeploymentGasPriceDecorator{evmKeeper: ek}
}

func (dgpd EthDeploymentGasPriceDecorator) AnteHandle(
	ctx sdk.Context, tx sdk.Tx, simulate bool, next sdk.AnteHandler,
) (newCtx sdk.Context, err error) {
	evmParams := dgpd.evmKeeper.GetParams(ctx)
	chainCfg := evmParams.GetChainConfig()
	ethCfg := chainCfg.EthereumConfig(dgpd.evmKeeper.ChainID())
	baseFee := dgpd.evmKeeper.GetBaseFee(ctx, ethCfg)

	for _, msg := range tx.GetMsgs() {
		ethMsg, ok := msg.(*evmtypes.MsgEthereumTx)
		if !ok {
			return ctx, errorsmod.Wrapf(
				errortypes.ErrUnknownRequest,
				"invalid message type %T, expected %T",
				msg, (*evmtypes.MsgEthereumTx)(nil),
			)
		}

		txData, err := evmtypes.UnpackTxData(ethMsg.Data)
		if err != nil {
			return ctx, errorsmod.Wrapf(err, "failed to unpack tx data %s", ethMsg.Hash)
		}

		// Effective fee the sender will actually pay (TOTAL, price × gas).
		//
		// For typed (non-legacy) txs the effective price depends on the EIP-1559
		// base fee. When the base fee is not active (London not enabled, or the
		// fee market has it disabled) `baseFee` is nil, in which case the
		// effective fee is simply the tx's static fee (gas price / fee cap × gas).
		feeAmt := ethMsg.GetFee()
		if txData.TxType() != ethtypes.LegacyTxType && baseFee != nil {
			feeAmt = ethMsg.GetEffectiveFee(baseFee)
		}

		// Contract deployment? (empty `to` address).
		minGasPrice := RegularGasPriceMin
		kind := "regular transaction"
		if to := txData.GetTo(); to == nil {
			minGasPrice = DeploymentGasPriceMin
			kind = "contract deployment"
		}

		// Compare TOTAL fees (minimum-per-gas × gas limit vs the tx's fee), which
		// is the same formulation the global min-gas-price decorator uses.
		gasLimit := sdkmath.LegacyNewDecFromBigInt(new(big.Int).SetUint64(ethMsg.GetGas()))
		requiredFee := minGasPrice.Mul(gasLimit)
		fee := sdkmath.LegacyNewDecFromBigInt(feeAmt)

		if fee.LT(requiredFee) {
			// Report the implied per-gas price for clarity.
			gasPrice := sdkmath.LegacyZeroDec()
			if ethMsg.GetGas() > 0 {
				gasPrice = fee.Quo(gasLimit)
			}
			return ctx, errorsmod.Wrapf(
				errortypes.ErrInsufficientFee,
				"%s requires a gas price of at least %s arune, got %s arune; "+
					"please increase the gas price (or the priority tip for EIP-1559 txs)",
				kind,
				minGasPrice.String(),
				gasPrice.String(),
			)
		}
	}

	return next(ctx, tx, simulate)
}
