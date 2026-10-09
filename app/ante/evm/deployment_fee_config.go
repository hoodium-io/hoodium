package evm

import (
	"fmt"

	sdkmath "cosmossdk.io/math"
)

// Hoodium two-gas-price model (compile-time genesis constants).
//
// Hoodium charges a higher minimum gas price for CONTRACT-DEPLOYMENT
// transactions than for regular transactions, to deter deployment spam while
// keeping ordinary transfers cheap. These are fixed constants (not governance
// params) — changing them requires a software upgrade.
//
// Values are expressed in the base denomination (arune) per unit of gas:
//
//	RegularGasPriceMin    = 0.0001 RUNE / gas
//	DeploymentGasPriceMin = 0.01   RUNE / gas  (100x the regular minimum)
//
// NOTE: these are FLOORS. The EIP-1559 base fee may push the effective gas price
// above them under congestion; it never lowers them.
var (
	// RegularGasPriceMin is the minimum gas price for regular (non-deployment)
	// transactions: 0.0001 RUNE per gas.
	RegularGasPriceMin = sdkmath.LegacyMustNewDecFromStr("0.0001")

	// DeploymentGasPriceMin is the minimum gas price for contract-deployment
	// transactions (empty `to` address): 0.01 RUNE per gas.
	DeploymentGasPriceMin = sdkmath.LegacyMustNewDecFromStr("0.01")
)

// ValidateTwoGasPriceConfig sanity-checks the compile-time gas-price constants.
// It panics if the values are invalid (e.g. deployment price below the regular
// price), so a misconfiguration fails fast at startup rather than silently
// weakening the fee policy.
func ValidateTwoGasPriceConfig() error {
	if RegularGasPriceMin.IsNil() || !RegularGasPriceMin.IsPositive() {
		return fmt.Errorf("regular gas price minimum must be positive, got %s", RegularGasPriceMin)
	}
	if DeploymentGasPriceMin.IsNil() || !DeploymentGasPriceMin.IsPositive() {
		return fmt.Errorf("deployment gas price minimum must be positive, got %s", DeploymentGasPriceMin)
	}
	if DeploymentGasPriceMin.LT(RegularGasPriceMin) {
		return fmt.Errorf(
			"deployment gas price minimum (%s) must be >= regular gas price minimum (%s)",
			DeploymentGasPriceMin, RegularGasPriceMin,
		)
	}
	return nil
}
