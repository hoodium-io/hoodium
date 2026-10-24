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
	sdk "github.com/cosmos/cosmos-sdk/types"
	errortypes "github.com/cosmos/cosmos-sdk/types/errors"
	authante "github.com/cosmos/cosmos-sdk/x/auth/ante"
)

// NewAnteHandler returns an ante handler responsible for attempting to route an
// Ethereum or SDK transaction to an internal ante handler for performing
// transaction-level processing (e.g. fee payment, signature verification) before
// being passed onto it's respective handler.
func NewAnteHandler(options HandlerOptions) sdk.AnteHandler {
	return func(
		ctx sdk.Context, tx sdk.Tx, sim bool,
	) (newCtx sdk.Context, err error) {
		var anteHandler sdk.AnteHandler

		txWithExtensions, ok := tx.(authante.HasExtensionOptionsTx)
		if ok {
			opts := txWithExtensions.GetExtensionOptions()
			if len(opts) > 0 {
				switch typeURL := opts[0].GetTypeUrl(); typeURL {
				case "/ethermint.evm.v1.ExtensionOptionsEthereumTx":
					if len(tx.GetMsgs()) != 1 {
						return ctx, errorsmod.Wrapf(errortypes.ErrNotSupported, "cannot submit more than one transaction at a time")
					}
					// handle as *evmtypes.MsgEthereumTx
					anteHandler = newEVMAnteHandler(options)
				case "/ethermint.types.v1.ExtensionOptionDynamicFeeTx":
					// A Cosmos tx that opts into the EIP-1559 dynamic fee market.
					// Handled by the Cosmos chain; the fee decorator reads the
					// extension to derive the priority tip.
					anteHandler = newCosmosAnteHandler(options)
				default:
					return ctx, errorsmod.Wrapf(
						errortypes.ErrUnknownExtensionOptions,
						"rejecting tx with unsupported extension option: %s", typeURL,
					)
				}

				return anteHandler(ctx, tx, sim)
			}
		}

		// No extension options: a plain Cosmos SDK transaction (MsgSend,
		// MsgCreateValidator, MsgDelegate, MsgGrant, MsgUpdateParams, …). Route it
		// to the Cosmos ante chain — otherwise validator onboarding and every
		// other native message is unreachable. `tx` is already known to be a
		// valid sdk.Tx here, so no further type assertion is needed.
		return newCosmosAnteHandler(options)(ctx, tx, sim)
	}
}
