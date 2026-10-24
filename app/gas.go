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

const (
	// RecommendedGasPrice is the `--gas-prices` value for ordinary transactions.
	//
	// ⚠️ TWO things are easy to get wrong here, and both were:
	//
	//  1. THE DENOM MUST BE `arune`, NOT `rune`. `--gas-prices` is parsed with
	//     `sdk.ParseDecCoins`, which does NOT normalize denominations (unlike
	//     `--fees`, which uses `ParseCoinsNormalized`). A `rune`-denominated gas
	//     price therefore keeps `denom="rune"`, while the ante decorator builds
	//     its required fee with `evmDenom = "arune"`. The denominations never
	//     match, the comparison yields a 1-arune fee, and the transaction is
	//     rejected. A `rune` value cannot work — it must be `arune`.
	//
	//  2. THE MAGNITUDE IS IN THE BASE DENOM. `MinGasPrice` is
	//     `20_000_000_000 arune/gas`, which is `0.00000002 RUNE/gas` — the same
	//     number in a different unit. Writing `0.00000002arune` means 2e-8 arune
	//     per gas, i.e. 10^18 too small.
	//
	// The value below is 25_000_000_000 arune/gas = 0.000000025 RUNE/gas, which
	// is 1.25x the floor — a small margin so a tx still clears the floor if the
	// simulated gas is slightly under the final gas used.
	RecommendedGasPrice = "25000000000arune"

	// RecommendedValidatorGasPrice is the `--gas-prices` value for
	// `create-validator`, which pays a 2x margin (50_000_000_000 arune/gas
	// = 0.00000005 RUNE/gas) because it is the single most important
	// transaction an operator sends and a failure costs a full sync cycle.
	//
	// At ~300,000 gas that is 15_000_000_000_000_000 arune = 0.015 RUNE.
	RecommendedValidatorGasPrice = "50000000000arune"
)
