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
	// FundingSource reports which pot pays this reward: the reserve, the block's
	// own fees, or nothing at all.
	FundingSource FundingSource
}

// FundingSource reports which pot a block reward is paid from.
type FundingSource string

const (
	// FundingSourceReserve means the reward comes from the pre-funded validator
	// reward pool. This is the normal case while emission lasts, and it never
	// changes total supply.
	FundingSourceReserve FundingSource = "reserve"
	// FundingSourceFees means the reserve can no longer cover the scheduled
	// reward, so the fee-only ladder pays instead. The coins are re-allocated from
	// the block's own transaction fees -- no new money is created.
	FundingSourceFees FundingSource = "fees"
	// FundingSourceNone means no reward is paid at all for this block.
	FundingSourceNone FundingSource = "none"
)

// HasTier reports whether a schedule tier covers the queried height.
func (b BlockRewardBreakdown) HasTier() bool {
	return b.TierIndex >= 0
}

// ComputeBlockRewardWithDiff resolves the PoNA reward for a height, transaction
// count and dPona difficulty without touching state.
//
// It is deterministic and side-effect free, which is what lets the CLI answer
// "what would this block pay?" for any height, including one that has not been
// produced yet, and at any difficulty.
//
// The returned FundingSource is RESERVE for every scheduled tier. Whether the
// reserve can actually cover the reward is a state question answered in
// DistributeRuneBlockReward, which switches to FundingSourceFees when it cannot.
func ComputeBlockRewardWithDiff(
	params types.Params, height, txCount, diff uint64,
) types.BlockRewardBreakdown {
	idx, tier, ok := params.TierForHeightWithDiff(height, diff)

	breakdown := types.BlockRewardBreakdown{
		Height:        height,
		TxCount:       txCount,
		TierIndex:     -1,
		Band:          types.BandNone,
		Reward:        sdkmath.ZeroInt(),
		FundingSource: FundingSourceNone,
	}
	if !ok {
		return breakdown
	}

	breakdown.TierIndex = idx
	breakdown.Tier = tier
	breakdown.Band = tier.BandForTxCount(txCount)
	breakdown.Reward = tier.RewardForTxCount(txCount)
	breakdown.FundingSource = FundingSourceReserve

	return breakdown
}

// ComputeBlockReward resolves the PoNA reward at the base (dPona-off) difficulty.
//
// It is the dPona-off view, used by the CLI and by callers that do not have the
// controller state. The consensus path must use ComputeBlockRewardWithDiff.
func ComputeBlockReward(
	params types.Params, height, txCount uint64,
) types.BlockRewardBreakdown {
	return ComputeBlockRewardWithDiff(params, height, txCount, 0)
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
	state := k.GetDPona(ctx)
	breakdown := ComputeBlockRewardWithDiff(params, height, txCount, state.Diff)

	// Snapshot the block's fee intake BEFORE any reward is credited to the fee
	// collector. PoNA runs last in EndBlock, so fee_collector holds exactly this
	// block's fees collected during DeliverTx. Reading it now is what lets the
	// fee-only ladder be funded by fees without skimming its own reward.
	feePool := k.blockFeeIncome(ctx, params.Denom)

	reward := breakdown.Reward
	source := breakdown.FundingSource

	// Reserve-funded tiers stop paying when the reserve can no longer cover the
	// scheduled reward. From that block on, the fee-only ladder takes over.
	if source == types.FundingSourceReserve {
		pool := k.RewardPoolBalance(ctx)
		if pool.Amount.LT(reward) {
			k.Logger(ctx).Info(
				"validator reward pool exhausted, switching to fee-only",
				"height", height,
				"pool_balance", pool.Amount.String(),
				"requested_reward", reward.String(),
			)
			source = types.FundingSourceFees
			breakdown.FundingSource = source
		}
	}

	if source == types.FundingSourceFees {
		// The scheduled amount is a ceiling: clamp it to what this block's fees
		// can actually pay.
		reward = k.FeeOnlyReward(ctx, txCount, feePool)
		breakdown.Reward = reward
	}

	if reward.IsZero() {
		// Nothing to pay this block: no tier covers the height, the reserve is
		// exhausted and the block collected too little in fees, or the fee-only
		// ladder pays nothing for an empty block. Advance the controller anyway --
		// difficulty tracks activity, not payment.
		k.advanceDPona(ctx, state, breakdown.Band)
		k.Logger(ctx).Debug(
			"no block reward this block",
			"height", height,
			"tx_count", txCount,
			"band", string(breakdown.Band),
			"funding_source", string(source),
			"fee_pool", feePool.String(),
			"dpona_diff", state.Diff,
		)

		return sdkmath.ZeroInt(), nil
	}

	// Route the reward.
	//
	// Reserve-funded: a real transfer out of the pre-funded pool, which never
	// changes total supply.
	//
	// Fee-funded: NO bank call. The coins are already in fee_collector, so paying
	// a fee-funded reward is a re-allocation, not a transfer of new money -- the
	// amount stays in the collector and x/distribution splits it with everything
	// else. Moving fee_collector -> fee_collector would be a no-op, and moving it
	// anywhere else would remove it from the distribution pool, contradicting
	// "fees continue to pay validators".
	if source == types.FundingSourceReserve {
		coins := sdk.NewCoins(sdk.NewCoin(params.Denom, reward))
		if err := k.bankKeeper.SendCoinsFromModuleToModule(
			ctx, types.ValidatorRewardPoolName, authtypes.FeeCollectorName, coins,
		); err != nil {
			return sdkmath.ZeroInt(), err
		}
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

	k.logRewardPaid(ctx, height, txCount, feePool, breakdown, reward, state)

	// Advance the controller AFTER computing this block's reward, so a block is
	// never classified by a difficulty it caused.
	k.advanceDPona(ctx, state, breakdown.Band)

	event := sdk.NewEvent(
		types.EventTypeRuneBlockReward,
		sdk.NewAttribute(types.AttributeKeyHeight, fmt.Sprintf("%d", height)),
		sdk.NewAttribute(types.AttributeKeyAmount, reward.String()),
		sdk.NewAttribute(types.AttributeKeyTxCount, fmt.Sprintf("%d", txCount)),
		sdk.NewAttribute(types.AttributeKeyBand, string(breakdown.Band)),
		sdk.NewAttribute(types.AttributeKeyTier, fmt.Sprintf("%d", breakdown.TierIndex)),
		sdk.NewAttribute(types.AttributeKeyFundingSource, string(breakdown.FundingSource)),
		sdk.NewAttribute(types.AttributeKeyDPona, fmt.Sprintf("%t", state.IsActive())),
		sdk.NewAttribute(types.AttributeKeyDPonaDiff, fmt.Sprintf("%d", state.Diff)),
	)
	if proposer.OperatorAddress != "" {
		event = event.AppendAttributes(
			sdk.NewAttribute(types.AttributeKeyValidator, proposer.OperatorAddress),
		)
	}
	ctx.EventManager().EmitEvent(event)

	return reward, nil
}
