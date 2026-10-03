package app

import (
	"context"
	"fmt"

	sdkmath "cosmossdk.io/math"

	sdk "github.com/cosmos/cosmos-sdk/types"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"

	"github.com/hoodium-io/hoodium/core/hooditoken"
	"github.com/hoodium-io/hoodium/core/runetoken"
	"github.com/hoodium-io/hoodium/utils"
)

// denomMaxSupplies maps a denom to its hard supply cap, expressed in the base
// denomination (e.g. arune). A denom absent from this map is uncapped.
//
// IMPORTANT: this is the single source of truth for mint enforcement. It
// references the same values the precompiles expose via the `maxSupply()`
// method (runetoken.MaxSupply for arune, hooditoken.MaxSupply for ahoodi), so
// the cap reported on-chain and the cap enforced on-chain can never diverge.
//
// It is built once at package init (not per-mint) to keep the consensus-critical
// mint path allocation-free.
var denomMaxSupplies = map[string]sdkmath.Int{
	utils.BaseDenom:  runetoken.MaxSupply,
	utils.HoodiDenom: hooditoken.MaxSupply,
}

// DenomMaxSupplies returns the per-denom hard supply caps. The returned map must
// not be mutated by callers.
func DenomMaxSupplies() map[string]sdkmath.Int {
	return denomMaxSupplies
}

// NewMintCapRestriction returns a bank MintingRestrictionFn that rejects any
// mint which would push a capped denom's total supply above its cap.
//
// It is installed on the bank keeper via
// bankkeeper.Keeper.WithMintCoinsRestriction at construction time. The Cosmos
// SDK invokes this function at the very top of MintCoins, before any state is
// changed, so a rejection aborts the whole mint atomically.
//
// The `getSupply` callback is used (instead of capturing the bank keeper) to
// avoid a construction-order cycle: the bank keeper does not exist yet at the
// time this restriction is built. It is only ever called during a mint, which
// happens strictly after app construction, so the keeper is always populated by
// then.
func NewMintCapRestriction(getSupply func(ctx context.Context, denom string) sdkmath.Int) banktypes.MintingRestrictionFn {
	return func(ctx context.Context, coins sdk.Coins) error {
		for _, coin := range coins {
			cap, capped := denomMaxSupplies[coin.Denom]
			if !capped {
				// Uncapped denom (e.g. an unrelated token) - nothing to enforce.
				continue
			}

			// Guard against a degenerate/negative amount being minted. A mint
			// must be strictly positive; anything else is rejected before the
			// supply math.
			amount := coin.Amount
			if amount.IsNil() || !amount.IsPositive() {
				return fmt.Errorf(
					"invalid mint amount for denom %s: %s (must be positive)",
					coin.Denom, amount,
				)
			}

			currentSupply := getSupply(ctx, coin.Denom)

			// projected = currentSupply + amount. Reject if projected > cap.
			projected := currentSupply.Add(amount)
			if projected.GT(cap) {
				return fmt.Errorf(
					"mint would exceed max supply of denom %s: current supply %s + minted %s = %s > max supply %s",
					coin.Denom, currentSupply, amount, projected, cap,
				)
			}
		}

		return nil
	}
}
