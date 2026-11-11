package keeper

import (
	"fmt"

	sdkmath "cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"

	"github.com/hoodium-io/hoodium/x/pona/types"
)

// BlockRewardBreakdown describes how the block reward for a given height and
// transaction count was determined. It is the single source of truth shared by
// the payout path (DistributeRuneBlockReward), the emitted event and the
// `query pona` CLI commands, so the number a validator sees in a log line, in an
// event and in a query is always produced by the same computation.
type BlockRewardBreakdown struct {
	// Height is the block height the reward applies to.
	Height uint64
	// TxCount is the number of REGULAR transactions in the block (the injected
	// app-level pseudo-transaction is excluded).
	TxCount uint64
	// TierIndex is the index of the schedule tier covering Height, or -1 when
	// no tier covers it (emission ended).
	TierIndex int
	// Tier is the schedule tier covering Height.
	Tier types.Tier
	// Band is the PoNA band selected by TxCount.
	Band types.ActivityBand
	// Reward is the amount actually scheduled for payment.
	Reward sdkmath.Int
}

// HasTier reports whether a schedule tier covers the queried height.
func (b BlockRewardBreakdown) HasTier() bool {
	return b.TierIndex >= 0
}

// ComputeBlockReward resolves the PoNA reward for a height and transaction
// count without touching state.
//
// It is deterministic and side-effect free, which is what lets the CLI answer
// "what would this block pay?" for any height, including one that has not been
// produced yet.
func ComputeBlockReward(params types.Params, height, txCount uint64) BlockRewardBreakdown {
	idx, tier, ok := params.TierForHeight(height)

	breakdown := BlockRewardBreakdown{
		Height:    height,
		TxCount:   txCount,
		TierIndex: -1,
		Band:      types.BandNone,
		Reward:    sdkmath.ZeroInt(),
	}
	if !ok {
		return breakdown
	}

	breakdown.TierIndex = idx
	breakdown.Tier = tier
	breakdown.Band = tier.BandForTxCount(txCount)
	breakdown.Reward = tier.RewardForTxCount(txCount)

	return breakdown
}

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
	breakdown := ComputeBlockReward(params, height, txCount)
	reward := breakdown.Reward
	if reward.IsZero() {
		// Fee-only: emission has ended (no tier covers this height).
		k.Logger(ctx).Debug(
			"no block reward: emission has ended (fee-only)",
			"height", height,
			"tx_count", txCount,
			"band", string(breakdown.Band),
		)

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

	// Log what was paid and why.
	//
	// tx_count is the piece operators need: it is the number of REGULAR
	// transactions the block contained (the injected vote-extension
	// pseudo-transaction is excluded), and it is what selected the band. It is
	// deliberately NOT included in the CometBFT log lines -- those report
	// num_txs/num_txs_res, which count the pseudo-transaction and therefore do not
	// answer "why did this block pay the zero band?".
	//
	// Logged at Info once per block, which is the same rate as CometBFT's own
	// per-block lines.
	k.Logger(ctx).Info(
		"block reward paid",
		"height", height,
		"tx_count", txCount,
		"band", string(breakdown.Band),
		"amount", reward.String(),
		"reward_rune", types.RuneString(reward),
		"tier", breakdown.TierIndex,
	)

	k.Logger(ctx).Debug(
		"block reward breakdown",
		"height", height,
		"tx_count", txCount,
		"band", string(breakdown.Band),
		"tier_index", breakdown.TierIndex,
		"tx_count_threshold", breakdown.Tier.TxCountThreshold,
		"reward_per_block", breakdown.Tier.RewardPerBlock.String(),
		"low_activity_reward", breakdown.Tier.LowActivityReward.String(),
		"single_activity_reward", breakdown.Tier.SingleActivityReward.String(),
		"zero_activity_reward", breakdown.Tier.ZeroActivityReward.String(),
		"pool_balance", pool.Amount.String(),
	)

	event := sdk.NewEvent(
		types.EventTypeRuneBlockReward,
		sdk.NewAttribute(types.AttributeKeyHeight, fmt.Sprintf("%d", height)),
		sdk.NewAttribute(types.AttributeKeyAmount, reward.String()),
		sdk.NewAttribute(types.AttributeKeyTxCount, fmt.Sprintf("%d", txCount)),
		sdk.NewAttribute(types.AttributeKeyBand, string(breakdown.Band)),
		sdk.NewAttribute(types.AttributeKeyTier, fmt.Sprintf("%d", breakdown.TierIndex)),
	)
	if proposer.OperatorAddress != "" {
		event = event.AppendAttributes(
			sdk.NewAttribute(types.AttributeKeyValidator, proposer.OperatorAddress),
		)
	}
	ctx.EventManager().EmitEvent(event)

	return reward, nil
}
