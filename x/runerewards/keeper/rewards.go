package keeper

import (
	"fmt"

	sdkmath "cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/hoodium-io/hoodium/x/runerewards/types"
)

// DistributeRuneBlockReward pays the block reward for the current block from the
// validator reward pool to the block proposer.
//
// Behaviour:
//   - The reward for the current height is looked up from the schedule with
//     Proof of Network Activity applied (see
//     types.Params.RewardForHeightAndTxCount): the tier's full rate is paid when
//     the block contains at least the tier's transaction threshold, the low
//     activity rate when it contains some but fewer, and the zero activity rate
//     when it is empty.
//   - The reward is paid to the proposer's operator address, taken from the
//     proposer consensus address reported by the ABCI request.
//   - If the pool cannot cover the reward (exhausted), emission simply stops for
//     this block (fee-only). The pool is never overdrawn.
//
// Transaction fees are NOT handled here: they follow the standard x/distribution
// flow and are split across the active validator set by voting power.
//
// It returns the amount actually paid (zero if nothing was paid).
func (k Keeper) DistributeRuneBlockReward(
	ctx sdk.Context, proposerConsAddr sdk.ConsAddress,
) (sdkmath.Int, error) {
	params := k.GetParams(ctx)

	height := uint64(ctx.BlockHeight()) //nolint:gosec // block height is always positive
	txCount := k.BlockTxCount(ctx)
	reward := params.RewardForHeightAndTxCount(height, txCount)
	if reward.IsZero() {
		// Fee-only: emission has ended (no tier covers this height).
		return sdkmath.ZeroInt(), nil
	}

	// Ensure the pool can cover the reward; otherwise emission stops.
	pool := k.RewardPoolBalance(ctx)
	if pool.Amount.LT(reward) {
		k.Logger(ctx).Info(
			"validator reward pool exhausted, switching to fee-only",
			"height", height,
			"pool_balance", pool.Amount.String(),
			"requested_reward", reward.String(),
		)
		return sdkmath.ZeroInt(), nil
	}

	// Resolve the proposer's operator address from its consensus address.
	validator, err := k.stakingKeeper.GetValidatorByConsAddr(ctx, proposerConsAddr)
	if err != nil {
		return sdkmath.ZeroInt(), err
	}

	valAddr, err := sdk.ValAddressFromBech32(validator.OperatorAddress)
	if err != nil {
		return sdkmath.ZeroInt(), err
	}

	coins := sdk.NewCoins(sdk.NewCoin(params.Denom, reward))
	if err := k.bankKeeper.SendCoinsFromModuleToAccount(
		ctx, types.ValidatorRewardPoolName, valAddr.Bytes(), coins,
	); err != nil {
		return sdkmath.ZeroInt(), err
	}

	ctx.EventManager().EmitEvent(
		sdk.NewEvent(
			types.EventTypeRuneBlockReward,
			sdk.NewAttribute(types.AttributeKeyHeight, fmt.Sprintf("%d", height)),
			sdk.NewAttribute(types.AttributeKeyValidator, valAddr.String()),
			sdk.NewAttribute(types.AttributeKeyAmount, reward.String()),
			sdk.NewAttribute(types.AttributeKeyTxCount, fmt.Sprintf("%d", txCount)),
		),
	)

	return reward, nil
}
