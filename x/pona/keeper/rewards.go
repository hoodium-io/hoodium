package keeper

import (
	"fmt"

	sdkmath "cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"

	"github.com/hoodium-io/hoodium/x/pona/types"
)

// DistributeRuneBlockReward pays the block reward for the current block from the
// validator reward pool into the distribution fee pool.
//
// Behaviour:
//   - The reward for the current height is looked up from the schedule with
//     Proof of Network Activity applied (see
//     types.Params.RewardForHeightAndTxCount): the tier's full rate is paid when
//     the block contains at least the tier's transaction threshold, the low
//     activity rate when it contains some but fewer, and the zero activity rate
//     when it is empty.
//   - The reward is transferred from the validator reward pool to the
//     distribution FEE COLLECTOR. x/distribution sweeps the fee collector every
//     block (AllocateTokens) and splits the funds across the ACTIVE validator set
//     by voting power — the same path transaction fees take. Delegators receive
//     their share through the validator's commission.
//   - If the pool cannot cover the reward (exhausted), emission simply stops for
//     this block (fee-only). The pool is never overdrawn.
//
// NOTE: the reward is deliberately NOT sent proposer-only. Paying only the
// proposer would concentrate reward on whoever produces the block and risk
// block-skipping / MEV centralisation; routing through x/distribution keeps the
// incentive aligned across the whole active set.
//
// The proposer consensus address is still used to identify the block and is
// recorded in the emitted event.
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

	// Forward the reward to the distribution fee collector. x/distribution picks
	// it up in its BeginBlock and allocates it across the active validator set by
	// voting power.
	coins := sdk.NewCoins(sdk.NewCoin(params.Denom, reward))
	if err := k.bankKeeper.SendCoinsFromModuleToModule(
		ctx, types.ValidatorRewardPoolName, authtypes.FeeCollectorName, coins,
	); err != nil {
		return sdkmath.ZeroInt(), err
	}

	// Record the proposer for observability. The payout is network-wide, so this
	// is informational only.
	proposer, err := k.stakingKeeper.GetValidatorByConsAddr(ctx, proposerConsAddr)
	if err != nil {
		// Non-fatal: the reward has already been routed correctly.
		k.Logger(ctx).Debug(
			"could not resolve proposer for reward event",
			"height", height,
			"error", err.Error(),
		)
	}

	event := sdk.NewEvent(
		types.EventTypeRuneBlockReward,
		sdk.NewAttribute(types.AttributeKeyHeight, fmt.Sprintf("%d", height)),
		sdk.NewAttribute(types.AttributeKeyAmount, reward.String()),
		sdk.NewAttribute(types.AttributeKeyTxCount, fmt.Sprintf("%d", txCount)),
	)
	if proposer.OperatorAddress != "" {
		event = event.AppendAttributes(
			sdk.NewAttribute(types.AttributeKeyValidator, proposer.OperatorAddress),
		)
	}
	ctx.EventManager().EmitEvent(event)

	return reward, nil
}
