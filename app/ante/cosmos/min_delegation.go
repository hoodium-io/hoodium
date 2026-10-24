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
package cosmos

import (
	errorsmod "cosmossdk.io/errors"
	sdk "github.com/cosmos/cosmos-sdk/types"
	errortypes "github.com/cosmos/cosmos-sdk/types/errors"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"

	runetypes "github.com/hoodium-io/hoodium/types"
)

// MinDelegationDecorator enforces Hoodium's minimum delegation amount
// (0.1 RUNE) on native Cosmos delegation messages.
//
// Cosmos x/staking has no minimum-delegation parameter, so Hoodium must enforce
// the floor itself. There are two delegation entry points and both are covered:
//
//   - the EVM staking precompile (`core/staking`) for EVM wallets, and
//   - this decorator for `runed tx staking delegate`.
//
// Before the Cosmos ante chain existed, only the precompile checked it, so
// re-enabling native Cosmos transactions would have silently opened a path
// around the floor. Both now read the same `runetypes.MinDelegation` value.
//
// MsgUndelegate and MsgBeginRedelegate are deliberately NOT checked: they move
// stake that is already delegated, so they cannot introduce a dust delegation.
// Only MsgDelegate creates one.
type MinDelegationDecorator struct{}

// NewMinDelegationDecorator creates a MinDelegationDecorator.
func NewMinDelegationDecorator() MinDelegationDecorator {
	return MinDelegationDecorator{}
}

func (mdd MinDelegationDecorator) AnteHandle(
	ctx sdk.Context, tx sdk.Tx, simulate bool, next sdk.AnteHandler,
) (sdk.Context, error) {
	for _, msg := range tx.GetMsgs() {
		delegateMsg, ok := msg.(*stakingtypes.MsgDelegate)
		if !ok {
			continue
		}

		if delegateMsg.Amount.Amount.LT(runetypes.MinDelegation) {
			return ctx, errorsmod.Wrapf(
				errortypes.ErrInvalidRequest,
				"delegation amount %s is below the minimum of %s arune (0.1 RUNE)",
				delegateMsg.Amount.Amount, runetypes.MinDelegation,
			)
		}
	}

	return next(ctx, tx, simulate)
}
