package app

import (
	"context"
	"testing"

	sdkmath "cosmossdk.io/math"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/stretchr/testify/require"

	"github.com/hoodium-io/hoodium/core/hooditoken"
	"github.com/hoodium-io/hoodium/core/runetoken"
	"github.com/hoodium-io/hoodium/utils"
)

// staticSupply returns a getSupply callback with a fixed supply per denom.
func staticSupply(supplies map[string]sdkmath.Int) func(ctx context.Context, denom string) sdkmath.Int {
	return func(_ context.Context, denom string) sdkmath.Int {
		if amt, ok := supplies[denom]; ok {
			return amt
		}
		return sdkmath.ZeroInt()
	}
}

func TestNewMintCapRestriction(t *testing.T) {
	runeCap := runetoken.MaxSupply   // 10,000,000,000 * 10^18
	hoodiCap := hooditoken.MaxSupply // 10,000,000 * 10^18

	testCases := []struct {
		name        string
		supply      map[string]sdkmath.Int
		mint        sdk.Coins
		expectError bool
	}{
		{
			name:        "rune: mint within cap (empty supply)",
			supply:      nil,
			mint:        sdk.NewCoins(sdk.NewCoin(utils.BaseDenom, sdkmath.NewInt(1_000_000))),
			expectError: false,
		},
		{
			name:        "rune: mint exactly reaches cap",
			supply:      map[string]sdkmath.Int{utils.BaseDenom: runeCap.SubRaw(1_000_000)},
			mint:        sdk.NewCoins(sdk.NewCoin(utils.BaseDenom, sdkmath.NewInt(1_000_000))),
			expectError: false,
		},
		{
			name:        "rune: mint one over cap is rejected",
			supply:      map[string]sdkmath.Int{utils.BaseDenom: runeCap},
			mint:        sdk.NewCoins(sdk.NewCoin(utils.BaseDenom, sdkmath.NewInt(1))),
			expectError: true,
		},
		{
			name:        "rune: mint far over cap is rejected",
			supply:      map[string]sdkmath.Int{utils.BaseDenom: runeCap},
			mint:        sdk.NewCoins(sdk.NewCoin(utils.BaseDenom, sdkmath.NewInt(1_000_000_000_000))),
			expectError: true,
		},
		{
			name:        "hoodi: mint within cap",
			supply:      nil,
			mint:        sdk.NewCoins(sdk.NewCoin(utils.HoodiDenom, sdkmath.NewInt(1_000_000))),
			expectError: false,
		},
		{
			name:        "hoodi: mint one over cap is rejected",
			supply:      map[string]sdkmath.Int{utils.HoodiDenom: hoodiCap},
			mint:        sdk.NewCoins(sdk.NewCoin(utils.HoodiDenom, sdkmath.NewInt(1))),
			expectError: true,
		},
		{
			name:        "uncapped denom is allowed regardless of amount",
			supply:      map[string]sdkmath.Int{"stake": sdkmath.NewInt(1)},
			mint:        sdk.NewCoins(sdk.NewCoin("stake", runeCap.MulRaw(100))),
			expectError: false,
		},
		{
			name:   "zero mint amount is rejected",
			supply: nil,
			// Constructed via a struct literal on purpose: sdk.NewCoin panics on
			// invalid amounts, but the restriction must still defend against a
			// malformed Coins value reaching it.
			mint:        sdk.Coins{{Denom: utils.BaseDenom, Amount: sdkmath.ZeroInt()}},
			expectError: true,
		},
		{
			name:        "negative mint amount is rejected",
			supply:      nil,
			mint:        sdk.Coins{{Denom: utils.BaseDenom, Amount: sdkmath.NewInt(-1)}},
			expectError: true,
		},
		{
			// MintCoins does not validate that denoms are unique, so a malformed
			// Coins value carrying the same denom twice must still be capped on
			// the SUM of the entries, not each entry independently.
			name:   "duplicate denom entries are summed before the cap check",
			supply: nil,
			mint: sdk.Coins{
				{Denom: utils.BaseDenom, Amount: runeCap.SubRaw(1)},
				{Denom: utils.BaseDenom, Amount: sdkmath.NewInt(2)},
			},
			expectError: true,
		},
		{
			name:   "duplicate denom entries exactly at cap are allowed",
			supply: nil,
			mint: sdk.Coins{
				{Denom: utils.BaseDenom, Amount: runeCap.SubRaw(1)},
				{Denom: utils.BaseDenom, Amount: sdkmath.NewInt(1)},
			},
			expectError: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			restriction := NewMintCapRestriction(staticSupply(tc.supply))
			err := restriction(context.Background(), tc.mint)
			if tc.expectError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// TestDenomMaxSupplies pins the caps to the token constants so the enforced cap
// can never silently diverge from the cap reported by the precompiles.
func TestDenomMaxSupplies(t *testing.T) {
	caps := DenomMaxSupplies()

	require.Equal(t, runetoken.MaxSupply, caps[utils.BaseDenom],
		"arune cap must equal runetoken.MaxSupply (the value maxSupply() reports)")
	require.Equal(t, hooditoken.MaxSupply, caps[utils.HoodiDenom],
		"ahoodi cap must equal hooditoken.MaxSupply (the value maxSupply() reports)")

	// Sanity: exactly the two production token denoms are capped.
	require.Len(t, caps, 2)
}

// TestValidateGenesisSupply covers the only supply-writing path that bypasses the
// mint restriction. Genesis writes supply directly (bank's InitGenesis calls
// setSupply, never MintCoins), so the cap has to be checked here or nowhere.
func TestValidateGenesisSupply(t *testing.T) {
	runeCap := runetoken.MaxSupply   // 10,000,000,000 * 10^18
	hoodiCap := hooditoken.MaxSupply // 10,000,000 * 10^18
	one := sdkmath.OneInt()

	testCases := []struct {
		name        string
		supply      sdk.Coins
		expectError bool
	}{
		{
			name:        "empty genesis supply is within cap",
			supply:      sdk.NewCoins(),
			expectError: false,
		},
		{
			name:        "rune exactly at cap is allowed (the shipped devnet shape)",
			supply:      sdk.NewCoins(sdk.NewCoin(utils.BaseDenom, runeCap)),
			expectError: false,
		},
		{
			name:        "rune one arune over cap is rejected",
			supply:      sdk.NewCoins(sdk.NewCoin(utils.BaseDenom, runeCap.Add(one))),
			expectError: true,
		},
		{
			name:        "rune far over cap is rejected",
			supply:      sdk.NewCoins(sdk.NewCoin(utils.BaseDenom, runeCap.MulRaw(2))),
			expectError: true,
		},
		{
			name:        "hoodi exactly at cap is allowed",
			supply:      sdk.NewCoins(sdk.NewCoin(utils.HoodiDenom, hoodiCap)),
			expectError: false,
		},
		{
			name:        "hoodi over cap is rejected",
			supply:      sdk.NewCoins(sdk.NewCoin(utils.HoodiDenom, hoodiCap.Add(one))),
			expectError: true,
		},
		{
			name: "both denoms exactly at cap is allowed (full emission)",
			supply: sdk.NewCoins(
				sdk.NewCoin(utils.BaseDenom, runeCap),
				sdk.NewCoin(utils.HoodiDenom, hoodiCap),
			),
			expectError: false,
		},
		{
			name: "one denom at cap and the other over is rejected",
			supply: sdk.NewCoins(
				sdk.NewCoin(utils.BaseDenom, runeCap),
				sdk.NewCoin(utils.HoodiDenom, hoodiCap.Add(one)),
			),
			expectError: true,
		},
		{
			name:        "uncapped denom is ignored no matter how large",
			supply:      sdk.NewCoins(sdk.NewCoin("uatom", sdkmath.NewIntWithDecimal(1, 30))),
			expectError: false,
		},
		{
			name: "an over-cap rune is caught even alongside an uncapped denom",
			supply: sdk.NewCoins(
				sdk.NewCoin("uatom", sdkmath.NewIntWithDecimal(1, 30)),
				sdk.NewCoin(utils.BaseDenom, runeCap.Add(one)),
			),
			expectError: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateGenesisSupply(tc.supply)
			if tc.expectError {
				require.Error(t, err)
				require.Contains(t, err.Error(), "exceeds max supply")
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// TestValidateGenesisSupplyDeterministicDenom pins the *order* of reporting when
// more than one capped denom is over its cap. The check sorts denom names so the
// panic message a node operator sees is identical on every machine — an
// unordered map iteration would make the same broken genesis report "arune" on
// one node and "ahoodi" on another.
func TestValidateGenesisSupplyDeterministicDenom(t *testing.T) {
	overCap := sdk.NewCoins(
		sdk.NewCoin(utils.BaseDenom, runetoken.MaxSupply.Add(sdkmath.OneInt())),
		sdk.NewCoin(utils.HoodiDenom, hooditoken.MaxSupply.Add(sdkmath.OneInt())),
	)

	// Repeat: Go randomises map iteration order, so a non-deterministic
	// implementation would eventually report the other denom.
	for i := 0; i < 50; i++ {
		err := ValidateGenesisSupply(overCap)
		require.Error(t, err)
		// "ahoodi" sorts before "arune".
		require.Contains(t, err.Error(), utils.HoodiDenom,
			"the alphabetically first over-cap denom must always be reported")
	}
}
