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
			name:        "zero mint amount is rejected",
			supply:      nil,
			mint:        sdk.NewCoins(sdk.NewCoin(utils.BaseDenom, sdkmath.ZeroInt())),
			expectError: true,
		},
		{
			name:        "negative mint amount is rejected",
			supply:      nil,
			mint:        sdk.NewCoins(sdk.NewCoin(utils.BaseDenom, sdkmath.NewInt(-1))),
			expectError: true,
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
