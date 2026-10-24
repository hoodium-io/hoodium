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

package app

import sdkmath "cosmossdk.io/math"

var (
	// MainnetMinGasPrices defines 20B arune as the minimum gas price value on the fee market module.
	// See https://commonwealth.im/evmos/discussion/5073-global-min-gas-price-value-for-cosmos-sdk-and-evm-transaction-choosing-a-value for reference
	MainnetMinGasPrices = sdkmath.LegacyNewDec(20_000_000_000)
	// MainnetMinGasMultiplier defines the min gas multiplier value on the fee market module.
	// 50% of the leftover gas will be refunded
	MainnetMinGasMultiplier = sdkmath.LegacyNewDecWithPrec(5, 1)
)

// RecommendedGasPrice is the gas price (in arune per gas) that CLI users should
// pass via `--gas-prices` when submitting a transaction.
//
// It is deliberately EQUAL to MainnetMinGasPrices: the chain rejects any fee
// below `MinGasPrice × gas`, and `--gas-prices` makes the SDK derive the fee
// from the *simulated* gas limit, so a value exactly at the floor is both
// sufficient and the cheapest valid choice. There is no headroom to add —
// paying above the floor only costs the user more.
//
// Using `--gas-prices` (rather than `--fees`) is what makes this correct with
// `--gas auto`: the SDK simulates the gas limit first and then multiplies it by
// this price, so the resulting fee always clears the floor without the user
// having to guess the gas. A fixed `--fees` amount cannot do that — it is
// compared against `MinGasPrice × gas`, so any amount smaller than the floor is
// rejected outright.
//
// Example: a ~300k-gas create-validator needs 300_000 × 20e9 = 6e15 arune
// (0.006 RUNE). A hardcoded `--fees 1000arune` is 1e-15 RUNE — roughly 6×10^12
// times too small, and is rejected before the message is ever executed.
const RecommendedGasPrice = "0.00000002arune"
