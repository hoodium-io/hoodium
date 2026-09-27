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
package types

import (
	"math/big"

	sdkmath "cosmossdk.io/math"

	sdk "github.com/cosmos/cosmos-sdk/types"
)

const (
	// AttoBtc defines the default coin denomination used in Hoodium in:
	//
	// - Staking parameters: denomination used as stake in the dPoS chain
	// - Mint parameters: denomination minted due to fee distribution rewards
	// - Governance parameters: denomination used for spam prevention in proposal deposits
	// - Crisis parameters: constant fee denomination used for spam prevention to check broken invariant
	// - EVM parameters: denomination used for running EVM state transitions in Hoodium.
	AttoBtc string = "arune"

	// BaseDenomUnit defines the base denomination unit for Hoodium.
	// 1 RUNE = 1x10^{BaseDenomUnit} arune
	BaseDenomUnit = 18

	// DefaultGasPrice is default gas price for evm transactions
	DefaultGasPrice = 20
)

// PowerReduction defines the default power reduction value for staking
var PowerReduction = sdkmath.NewIntFromBigInt(new(big.Int).Exp(big.NewInt(10), big.NewInt(BaseDenomUnit), nil))

// NewHoodiumCoin is a utility function that returns an "arune" coin with the given sdkmath.Int amount.
// The function will panic if the provided amount is negative.
func NewHoodiumCoin(amount sdkmath.Int) sdk.Coin {
	return sdk.NewCoin(AttoBtc, amount)
}

// NewHoodiumDecCoin is a utility function that returns an "arune" decimal coin with the given sdkmath.Int amount.
// The function will panic if the provided amount is negative.
func NewHoodiumDecCoin(amount sdkmath.Int) sdk.DecCoin {
	return sdk.NewDecCoin(AttoBtc, amount)
}

// NewHoodiumCoinInt64 is a utility function that returns an "arune" coin with the given int64 amount.
// The function will panic if the provided amount is negative.
func NewHoodiumCoinInt64(amount int64) sdk.Coin {
	return sdk.NewInt64Coin(AttoBtc, amount)
}
