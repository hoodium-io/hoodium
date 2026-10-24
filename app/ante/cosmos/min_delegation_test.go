package cosmos_test

import (
	"testing"

	sdkmath "cosmossdk.io/math"
	"github.com/stretchr/testify/require"

	sdk "github.com/cosmos/cosmos-sdk/types"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"

	cosmosante "github.com/hoodium-io/hoodium/app/ante/cosmos"
	"github.com/hoodium-io/hoodium/testutil"
	runetypes "github.com/hoodium-io/hoodium/types"
)

// TestMinDelegationDecorator covers Hoodium's 0.1 RUNE minimum delegation on the
// NATIVE Cosmos path (`runed tx staking delegate`).
//
// The same floor is enforced by the EVM staking precompile for EVM wallets.
// Both read `runetypes.MinDelegation`, so this test also guards against the two
// entry points drifting apart.
func TestMinDelegationDecorator(t *testing.T) {
	delegator := sdk.AccAddress([]byte("delegator-address---"))
	validator := sdk.ValAddress([]byte("validator-address---"))

	delegate := func(amount sdkmath.Int) *stakingtypes.MsgDelegate {
		return &stakingtypes.MsgDelegate{
			DelegatorAddress: delegator.String(),
			ValidatorAddress: validator.String(),
			Amount:           sdk.NewCoin(runetypes.AttoBtc, amount),
		}
	}

	// One arune below the floor.
	belowFloor := runetypes.MinDelegation.Sub(sdkmath.OneInt())
	// Exactly at the floor.
	atFloor := runetypes.MinDelegation
	// Comfortably above it.
	aboveFloor := runetypes.MinDelegation.Mul(sdkmath.NewInt(10))

	testCases := []struct {
		name      string
		msgs      []sdk.Msg
		expErr    bool
		errSubstr string
	}{
		{
			name:   "delegation above the floor is allowed",
			msgs:   []sdk.Msg{delegate(aboveFloor)},
			expErr: false,
		},
		{
			name:   "delegation exactly at the floor is allowed",
			msgs:   []sdk.Msg{delegate(atFloor)},
			expErr: false,
		},
		{
			name:      "delegation below the floor is rejected",
			msgs:      []sdk.Msg{delegate(belowFloor)},
			expErr:    true,
			errSubstr: "below the minimum",
		},
		{
			name:      "dust delegation is rejected",
			msgs:      []sdk.Msg{delegate(sdkmath.OneInt())},
			expErr:    true,
			errSubstr: "below the minimum",
		},
		{
			name:   "unrelated messages are ignored",
			msgs:   []sdk.Msg{&stakingtypes.MsgUndelegate{DelegatorAddress: delegator.String(), ValidatorAddress: validator.String(), Amount: sdk.NewCoin(runetypes.AttoBtc, sdkmath.OneInt())}},
			expErr: false,
		},
	}

	dec := cosmosante.NewMinDelegationDecorator()

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := sdk.Context{}
			err := testutil.ValidateAnteForMsgs(ctx, dec, tc.msgs...)

			if tc.expErr {
				require.Error(t, err, tc.name)
				require.Contains(t, err.Error(), tc.errSubstr, tc.name)
				return
			}

			require.NoError(t, err, tc.name)
		})
	}
}

// TestMinDelegationValue pins the floor itself, so a change to it is always a
// deliberate, reviewed edit rather than an accident.
func TestMinDelegationValue(t *testing.T) {
	// 0.1 RUNE, expressed in arune (1 RUNE = 10^18 arune).
	require.Equal(t, "100000000000000000", runetypes.MinDelegation.String())
}
