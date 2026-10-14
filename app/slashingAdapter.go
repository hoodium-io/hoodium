package app

import (
	"context"
	"fmt"

	sdkmath "cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	stakingslash "github.com/cosmos/cosmos-sdk/x/staking/keeper"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"

	ponatypes "github.com/hoodium-io/hoodium/x/pona/types"
)

// runeStakingKeeper implements the x/slashing StakingKeeper interface on top of
// the standard staking keeper, redirecting SLASHED FUNDS.
//
// # Why this exists
//
// The SDK's staking keeper BURNS slashed tokens (x/staking/keeper/slash.go ->
// burnBondedTokens -> bankKeeper.BurnCoins). Hoodium does not want to burn: it
// wants slashed funds to benefit the active validator set.
//
// The burn itself is hardcoded inside x/staking, but x/slashing does NOT call
// the concrete keeper - it calls this interface
// (x/slashing/types/expected_keepers.go). So Hoodium can intercept a slash by
// wrapping the keeper and overriding only the two Slash methods. No fork of
// x/staking is required.
//
// # How it works
//
//  1. The real staking keeper performs the slash exactly as normal: bonded /
//     unbonded tokens are deducted from the validator and delegators, and the
//     deducted amount IS BURNED.
//  2. This wrapper then MINTS the same amount into the distribution
//     FEE COLLECTOR.
//  3. x/distribution sweeps the fee collector every block
//     (AllocateTokens -> GetAllBalances(feeCollector)) and splits it across the
//     active validator set by voting power - the same path transaction fees take.
//
// # Supply accounting
//
// Net supply is UNCHANGED: the slash burns -X and the wrapper mints +X. The 10B
// cap is never exceeded. The re-mint is exactly bounded by the amount the real
// keeper reports as slashed, so it cannot be used to inflate supply: causing a
// slash requires an offence that costs the offender more than they gain.
//
// # Why the fee collector (not validator_reward_pool)
//
//   - Fee collector funds reach the ACTIVE validator set every block,
//     proportional to voting power - a live, network-wide payout appropriate for
//     a network-level offence.
//   - It does NOT extend the 1B validator reward emission schedule, which
//     redirecting into validator_reward_pool would have done.
//   - It matches Hoodium's end state, where rewards are fees-only.
//
// The embedded *stakingslash.Keeper provides every other interface method
// (Validator, IterateValidators, Jail, Unjail, ...); only the slash paths are
// overridden.
type runeStakingKeeper struct {
	*stakingslash.Keeper

	bankKeeper RuneSlashBankKeeper
}

// RuneSlashBankKeeper is the subset of the bank keeper needed to redirect slashed
// funds. It is an interface so the adapter is easy to stub in tests.
type RuneSlashBankKeeper interface {
	// MintCoins mints coins from a module account.
	MintCoins(ctx context.Context, moduleName string, amt sdk.Coins) error
	// SendCoinsFromModuleToModule moves coins between module accounts.
	SendCoinsFromModuleToModule(ctx context.Context, senderModule, recipientModule string, amt sdk.Coins) error
}

// newRuneStakingKeeper wraps the standard staking keeper.
func newRuneStakingKeeper(sk *stakingslash.Keeper, bankKeeper RuneSlashBankKeeper) runeStakingKeeper {
	return runeStakingKeeper{
		Keeper:     sk,
		bankKeeper: bankKeeper,
	}
}

// Slash slashes a validator and redirects the slashed amount to the fee pool.
func (k runeStakingKeeper) Slash(
	ctx context.Context, consAddr sdk.ConsAddress, infractionHeight, power int64, slashFactor sdkmath.LegacyDec,
) (sdkmath.Int, error) {
	return k.slashAndRedirect(ctx, consAddr, infractionHeight, power, slashFactor)
}

// SlashWithInfractionReason is the entry point x/slashing actually uses.
// The infraction reason is not needed by the staking keeper itself (it exists
// for Interchain Security), so it is accepted and ignored.
func (k runeStakingKeeper) SlashWithInfractionReason(
	ctx context.Context, consAddr sdk.ConsAddress, infractionHeight, power int64,
	slashFactor sdkmath.LegacyDec, _ stakingtypes.Infraction,
) (sdkmath.Int, error) {
	return k.slashAndRedirect(ctx, consAddr, infractionHeight, power, slashFactor)
}

// slashAndRedirect runs the real slash, then re-mints the slashed amount into
// the fee collector so no RUNE is destroyed.
func (k runeStakingKeeper) slashAndRedirect(
	ctx context.Context, consAddr sdk.ConsAddress, infractionHeight, power int64, slashFactor sdkmath.LegacyDec,
) (sdkmath.Int, error) {
	// 1. Real slash: tokens are deducted and burned by x/staking.
	slashed, err := k.Keeper.Slash(ctx, consAddr, infractionHeight, power, slashFactor)
	if err != nil {
		return sdkmath.ZeroInt(), err
	}

	// Nothing was actually burned (e.g. the slash amount rounded to zero) -
	// there is nothing to redirect.
	if slashed.IsZero() {
		return slashed, nil
	}

	// 2. Re-mint the same amount and forward it to the fee collector, where
	// x/distribution sweeps it into the fee pool on the next block and splits it
	// across the active validator set by voting power.
	if err := k.redirectToFeeCollector(ctx, slashed); err != nil {
		return sdkmath.ZeroInt(), fmt.Errorf("slashing: redirecting slashed funds: %w", err)
	}

	return slashed, nil
}

// redirectToFeeCollector mints `amount` of the bond denom and sends it to the
// distribution fee collector module account.
func (k runeStakingKeeper) redirectToFeeCollector(ctx context.Context, amount sdkmath.Int) error {
	denom, err := k.Keeper.BondDenom(ctx)
	if err != nil {
		return fmt.Errorf("reading bond denom: %w", err)
	}

	coins := sdk.NewCoins(sdk.NewCoin(denom, amount))

	// Mint from the pona module account, which ALREADY holds the Minter
	// permission (see maccPerms in app.go). Minting from the staking module
	// instead would require granting staking an unnecessary new privilege.
	if err := k.bankKeeper.MintCoins(ctx, ponatypes.ModuleName, coins); err != nil {
		return fmt.Errorf("minting slashed funds: %w", err)
	}

	return k.bankKeeper.SendCoinsFromModuleToModule(
		ctx, ponatypes.ModuleName, authtypes.FeeCollectorName, coins,
	)
}
