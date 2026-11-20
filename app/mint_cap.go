package app

import (
	"context"
	"fmt"
	"sort"

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

// ValidateGenesisSupply returns an error when the genesis bank supply of a
// capped denom exceeds that denom's hard cap.
//
// # Why genesis needs its own check
//
// The mint restriction installed by NewMintCapRestriction guards MintCoins, and
// MintCoins is the only way supply can grow at runtime. Genesis is the one
// exception: x/bank's InitGenesis writes balances and supply directly
// (bank/keeper/genesis.go: it calls k.setSupply, never MintCoins), so a genesis
// file bypasses the restriction entirely. That makes this the only place the cap
// can be enforced for a chain's initial state.
//
// An over-cap genesis is not a recoverable misconfiguration:
//   - totalSupply() would permanently report more than maxSupply(), an
//     incoherent token, and
//   - headroom would be negative, so *no* mint could ever succeed again —
//     including PoNA's and any future module's.
//
// It must therefore fail the chain's first block rather than limp along. Sitting
// exactly AT the cap is legal and is the intended fully-emitted state (the devnet
// genesis emits the whole 10B RUNE emission up front), so this rejects only a
// strict excess.
func ValidateGenesisSupply(supply sdk.Coins) error {
	// Iterate the capped denoms in sorted order so the reported denom (and the
	// resulting panic message) is deterministic when more than one denom is
	// over its cap.
	denoms := make([]string, 0, len(denomMaxSupplies))
	for denom := range denomMaxSupplies {
		denoms = append(denoms, denom)
	}
	sort.Strings(denoms)

	for _, denom := range denoms {
		cap := denomMaxSupplies[denom]
		amount := supply.AmountOf(denom)
		if amount.GT(cap) {
			return fmt.Errorf(
				"genesis supply of denom %s is %s, which exceeds max supply %s; "+
					"a chain above its cap can never mint again (including PoNA rewards) "+
					"and would report totalSupply > maxSupply",
				denom, amount, cap,
			)
		}
	}

	return nil
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
		// Aggregate the requested mint amounts per capped denom BEFORE comparing
		// against the cap. MintCoins adds each entry of `coins` to the supply
		// independently (and does NOT validate that denoms are unique), so a
		// malformed Coins value carrying the same denom twice must not be able
		// to slip past a per-entry check. Summing first makes that impossible.
		requested := make(map[string]sdkmath.Int, len(denomMaxSupplies))
		for _, coin := range coins {
			if _, capped := denomMaxSupplies[coin.Denom]; !capped {
				// Uncapped denom (e.g. an unrelated token) - nothing to enforce.
				continue
			}

			// Guard against a degenerate/negative amount being minted. A mint
			// must be strictly positive; anything else is rejected before the
			// supply math. (sdk.NewCoin already panics on invalid amounts, but
			// MintCoins runs its restriction before any validation, so a
			// malformed Coins value can still reach us.)
			amount := coin.Amount
			if amount.IsNil() || !amount.IsPositive() {
				return fmt.Errorf(
					"mint rejected: invalid mint amount for denom %s (got %s, but the amount must be a positive integer)",
					coin.Denom, amount,
				)
			}

			acc, ok := requested[coin.Denom]
			if !ok {
				acc = sdkmath.ZeroInt()
			}
			requested[coin.Denom] = acc.Add(amount)
		}

		// Now compare the aggregated amount per denom against its cap.
		for denom, amount := range requested {
			cap := denomMaxSupplies[denom]
			currentSupply := getSupply(ctx, denom)

			// projected = currentSupply + amount. Reject if projected > cap.
			projected := currentSupply.Add(amount)
			if projected.GT(cap) {
				return fmt.Errorf(
					"mint rejected: projected supply of denom %s would exceed max supply (calculation: current supply %s + minted %s = projected %s, which is greater than max supply %s)",
					denom, currentSupply, amount, projected, cap,
				)
			}
		}

		return nil
	}
}
