// Copyright 2022 Evmos Foundation
// This file is part of the Evmos Network packages.
//
// Evmos is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The Evmos packages are distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the Evmos packages. If not, see https://github.com/evmos/evmos/blob/main/LICENSE

package ante

import (
	errorsmod "cosmossdk.io/errors"
	storetypes "cosmossdk.io/store/types"
	"github.com/cosmos/cosmos-sdk/codec"
	sdk "github.com/cosmos/cosmos-sdk/types"
	errortypes "github.com/cosmos/cosmos-sdk/types/errors"
	"github.com/cosmos/cosmos-sdk/types/tx/signing"
	"github.com/cosmos/cosmos-sdk/x/auth/ante"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	sdkvesting "github.com/cosmos/cosmos-sdk/x/auth/vesting/types"
	anteutils "github.com/hoodium-io/hoodium/app/ante/utils"

	cosmosante "github.com/hoodium-io/hoodium/app/ante/cosmos"
	evmante "github.com/hoodium-io/hoodium/app/ante/evm"
	evmtypes "github.com/hoodium-io/hoodium/x/evm/types"

	txsigning "cosmossdk.io/x/tx/signing"
)

// HandlerOptions defines the list of module keepers required to run the Hoodium
// AnteHandler decorators.
type HandlerOptions struct {
	Cdc                    codec.BinaryCodec
	AccountKeeper          evmtypes.AccountKeeper
	BankKeeper             evmtypes.BankKeeper
	FeeMarketKeeper        evmante.FeeMarketKeeper
	EvmKeeper              evmante.EVMKeeper
	FeegrantKeeper         ante.FeegrantKeeper
	ExtensionOptionChecker ante.ExtensionOptionChecker
	SignModeHandler        *txsigning.HandlerMap
	SigGasConsumer         func(meter storetypes.GasMeter, sig signing.SignatureV2, params authtypes.Params) error
	MaxTxGasWanted         uint64
	TxFeeChecker           anteutils.TxFeeChecker
}

// Validate checks if the keepers are defined
func (options HandlerOptions) Validate() error {
	if options.Cdc == nil {
		return errorsmod.Wrap(errortypes.ErrLogic, "codec is required for AnteHandler")
	}
	if options.AccountKeeper == nil {
		return errorsmod.Wrap(errortypes.ErrLogic, "account keeper is required for AnteHandler")
	}
	if options.BankKeeper == nil {
		return errorsmod.Wrap(errortypes.ErrLogic, "bank keeper is required for AnteHandler")
	}
	if options.FeeMarketKeeper == nil {
		return errorsmod.Wrap(errortypes.ErrLogic, "fee market keeper is required for AnteHandler")
	}
	if options.EvmKeeper == nil {
		return errorsmod.Wrap(errortypes.ErrLogic, "evm keeper is required for AnteHandler")
	}
	if options.SigGasConsumer == nil {
		return errorsmod.Wrap(errortypes.ErrLogic, "signature gas consumer is required for AnteHandler")
	}
	if options.SignModeHandler == nil {
		return errorsmod.Wrap(errortypes.ErrLogic, "sign mode handler is required for AnteHandler")
	}
	if options.TxFeeChecker == nil {
		return errorsmod.Wrap(errortypes.ErrLogic, "tx fee checker is required for AnteHandler")
	}
	return nil
}

// newEVMAnteHandler creates the default ante handler for Ethereum transactions
func newEVMAnteHandler(options HandlerOptions) sdk.AnteHandler {
	return sdk.ChainAnteDecorators(
		// outermost AnteDecorator. SetUpContext must be called first
		evmante.NewEthSetUpContextDecorator(options.EvmKeeper),
		// Check eth effective gas price against the node's minimal-gas-prices config
		evmante.NewEthMempoolFeeDecorator(options.EvmKeeper),
		// Check eth effective gas price against the global MinGasPrice
		evmante.NewEthMinGasPriceDecorator(options.FeeMarketKeeper, options.EvmKeeper),
		// Enforce Hoodium's two-gas-price model: contract deployments must pay a
		// higher minimum gas price than regular transactions.
		evmante.NewEthDeploymentGasPriceDecorator(options.EvmKeeper, options.FeeMarketKeeper),
		evmante.NewEthValidateBasicDecorator(options.EvmKeeper),
		evmante.NewEthSigVerificationDecorator(options.EvmKeeper),
		evmante.NewEthAccountVerificationDecorator(options.AccountKeeper, options.EvmKeeper),
		evmante.NewCanTransferDecorator(options.EvmKeeper),
		evmante.NewEthGasConsumeDecorator(options.BankKeeper, options.EvmKeeper, options.MaxTxGasWanted),
		evmante.NewEthIncrementSenderSequenceDecorator(options.AccountKeeper),
		evmante.NewGasWantedDecorator(options.EvmKeeper, options.FeeMarketKeeper),
		// emit eth tx hash and index at the very last ante handler.
		evmante.NewEthEmitEventDecorator(options.EvmKeeper),
	)
}

// asAnteTxFeeChecker adapts the Hoodium `anteutils.TxFeeChecker` to the
// Cosmos SDK's `ante.TxFeeChecker`.
//
// The two have the same shape except for the tx parameter type: the SDK passes
// an `sdk.Tx` and asserts `sdk.FeeTx` internally, whereas Hoodium's checker
// takes the already-asserted `sdk.FeeTx`. The EVM chain uses the Hoodium type;
// the SDK's DeductFeeDecorator (used by the Cosmos chain) requires its own.
//
// A nil checker is returned as nil so the decorator falls back to its built-in
// validator min-gas-price logic — the same behaviour as passing nil directly.
func asAnteTxFeeChecker(checker anteutils.TxFeeChecker) ante.TxFeeChecker {
	if checker == nil {
		return nil
	}

	return func(ctx sdk.Context, tx sdk.Tx) (sdk.Coins, int64, error) {
		feeTx, ok := tx.(sdk.FeeTx)
		if !ok {
			return nil, 0, errorsmod.Wrapf(
				errortypes.ErrInvalidType, "invalid transaction type %T, expected sdk.FeeTx", tx,
			)
		}

		return checker(ctx, feeTx)
	}
}

// newCosmosAnteHandler creates the ante handler for native Cosmos SDK
// transactions (everything that is not a MsgEthereumTx).
//
// Hoodium is an EVM chain, but it is still a Cosmos SDK application: validator
// onboarding (`MsgCreateValidator`), token transfers (`MsgSend`), distribution,
// authz, upgrades and governance are all native Cosmos messages. Without this
// chain the router in ante.go rejected every one of them, which made
// `runed tx staking create-validator` impossible to broadcast.
//
// The decorator order follows upstream Evmos. Deviations from Evmos, all
// deliberate:
//
//   - No `ibcante.NewRedundantRelayDecorator` — Hoodium wires no IBC keeper.
//   - No `VestingDelegationDecorator` — Hoodium has no vesting module.
//   - `ante.NewDeductFeeDecorator` (the SDK one) rather than the Evmos custom
//     fee decorator. Evmos' version exists to fall back to *unclaimed staking
//     rewards* when the balance cannot cover the fee, which it needs because
//     Evmos ships clawback/vesting accounts. Hoodium has no such accounts, so
//     the standard SDK decorator is the correct and better-audited choice.
//
// `NewMinGasPriceDecorator` is the Cosmos-side counterpart of the EVM
// `EthMinGasPriceDecorator`: both read `x/feemarket`'s `MinGasPrice` param, so
// native and EVM transactions are held to the same global floor.
func newCosmosAnteHandler(options HandlerOptions) sdk.AnteHandler {
	return sdk.ChainAnteDecorators(
		// Reject MsgEthereumTx on the Cosmos path: it must arrive wrapped in a tx
		// carrying the ExtensionOptionsEthereumTx option so the EVM chain (which
		// performs the gas refund) handles it.
		cosmosante.RejectMessagesDecorator{},
		// Block msg types that must never be nested inside an authz.MsgExec.
		//
		// MsgEthereumTx must arrive on the EVM path (it needs the Ethereum ante
		// chain to perform the gas refund). MsgCreateVestingAccount is included
		// for the same reason upstream Evmos includes it: Hoodium wires no
		// vesting module, so a vesting account can never be created here, and
		// granting authority to create one is a privilege that must not be
		// delegable through authz.
		cosmosante.NewAuthzLimiterDecorator(
			sdk.MsgTypeURL(&evmtypes.MsgEthereumTx{}),
			sdk.MsgTypeURL(&sdkvesting.MsgCreateVestingAccount{}),
		),
		// Outermost decorator: SetUpContext must run first.
		ante.NewSetUpContextDecorator(),
		// Reject unknown tx extension options. The checker accepts the dynamic
		// fee extension; plain Cosmos txs carry no options at all and pass.
		ante.NewExtensionOptionsDecorator(options.ExtensionOptionChecker),
		ante.NewValidateBasicDecorator(),
		ante.NewTxTimeoutHeightDecorator(),
		ante.NewValidateMemoDecorator(options.AccountKeeper),
		// Global minimum gas price floor (x/feemarket MinGasPrice param).
		cosmosante.NewMinGasPriceDecorator(options.FeeMarketKeeper, options.EvmKeeper),
		ante.NewConsumeGasForTxSizeDecorator(options.AccountKeeper),
		// Fee validation + deduction. FeegrantKeeper is nil on Hoodium (no
		// feegrant module); the decorator only consults it when a tx actually
		// sets a fee granter, in which case it correctly reports that fee grants
		// are not enabled.
		ante.NewDeductFeeDecorator(
			options.AccountKeeper,
			options.BankKeeper,
			options.FeegrantKeeper,
			asAnteTxFeeChecker(options.TxFeeChecker),
		),
		// SetPubKeyDecorator must be called before all signature verification
		// decorators.
		ante.NewSetPubKeyDecorator(options.AccountKeeper),
		ante.NewValidateSigCountDecorator(options.AccountKeeper),
		ante.NewSigGasConsumeDecorator(options.AccountKeeper, options.SigGasConsumer),
		ante.NewSigVerificationDecorator(options.AccountKeeper, options.SignModeHandler),
		ante.NewIncrementSequenceDecorator(options.AccountKeeper),
		// Track gas wanted for the EIP-1559 base-fee calculation, exactly as the
		// EVM chain does.
		evmante.NewGasWantedDecorator(options.EvmKeeper, options.FeeMarketKeeper),
	)
}
