package keeper

import (
	"context"
	"fmt"

	"cosmossdk.io/log"
	sdkmath "cosmossdk.io/math"
	storetypes "cosmossdk.io/store/types"
	"github.com/cosmos/cosmos-sdk/codec"
	sdk "github.com/cosmos/cosmos-sdk/types"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"

	"github.com/hoodium-io/hoodium/x/pona/types"
)

// BankKeeper is the subset of the bank keeper used by the pona module.
type BankKeeper interface {
	// GetBalance returns the balance of the given address for the denom.
	GetBalance(ctx context.Context, addr sdk.AccAddress, denom string) sdk.Coin
	// SendCoinsFromModuleToAccount transfers coins from a module account to an
	// account. It errors if the module account has insufficient funds.
	SendCoinsFromModuleToAccount(
		ctx context.Context, senderModule string, recipientAddr sdk.AccAddress, amt sdk.Coins,
	) error
	// SendCoinsFromModuleToModule transfers coins between module accounts. Used
	// to forward the block reward to the distribution fee collector for splitting
	// across the active validator set.
	SendCoinsFromModuleToModule(
		ctx context.Context, senderModule, recipientModule string, amt sdk.Coins,
	) error
	// MintCoins mints new coins from a module account.
	MintCoins(ctx context.Context, moduleName string, amt sdk.Coins) error
}

// StakingKeeper is the subset of the staking keeper used by pona.
type StakingKeeper interface {
	// GetValidatorByConsAddr returns the validator for the given consensus
	// address (the block proposer's consensus address).
	GetValidatorByConsAddr(ctx context.Context, consAddr sdk.ConsAddress) (stakingtypes.Validator, error)
}

// AccountKeeper is the subset of the auth keeper used by pona.
type AccountKeeper interface {
	GetModuleAddress(moduleName string) sdk.AccAddress
}

// Keeper grants access to the pona module state.
type Keeper struct {
	cdc      codec.BinaryCodec
	storeKey storetypes.StoreKey

	bankKeeper    BankKeeper
	stakingKeeper StakingKeeper
	accountKeeper AccountKeeper

	// authority is the gov/module authority allowed to update params.
	authority string
}

// NewKeeper creates a new pona keeper. It panics if the authority address
// is not correctly formatted or the reward-pool module account is missing.
func NewKeeper(
	cdc codec.BinaryCodec,
	storeKey storetypes.StoreKey,
	bankKeeper BankKeeper,
	stakingKeeper StakingKeeper,
	accountKeeper AccountKeeper,
	authority string,
) Keeper {
	if _, err := sdk.AccAddressFromBech32(authority); err != nil {
		panic(err)
	}
	if accountKeeper.GetModuleAddress(types.ValidatorRewardPoolName) == nil {
		panic(fmt.Sprintf("%s module account has not been set", types.ValidatorRewardPoolName))
	}

	return Keeper{
		cdc:           cdc,
		storeKey:      storeKey,
		bankKeeper:    bankKeeper,
		stakingKeeper: stakingKeeper,
		accountKeeper: accountKeeper,
		authority:     authority,
	}
}

// Logger returns a module-specific logger.
func (k Keeper) Logger(ctx sdk.Context) log.Logger {
	return ctx.Logger().With("module", types.ModuleName)
}

// GetAuthority returns the module's authority.
func (k Keeper) GetAuthority() string {
	return k.authority
}

// GetParams returns the current module parameters.
func (k Keeper) GetParams(ctx sdk.Context) types.Params {
	store := ctx.KVStore(k.storeKey)
	bz := store.Get(types.KeyPrefixParams)
	if len(bz) == 0 {
		return types.Params{}
	}

	var params types.Params
	k.cdc.MustUnmarshal(bz, &params)
	return params
}

// SetParams stores the module parameters.
func (k Keeper) SetParams(ctx sdk.Context, params types.Params) error {
	if err := params.Validate(); err != nil {
		return err
	}

	bz := k.cdc.MustMarshal(&params)

	store := ctx.KVStore(k.storeKey)
	store.Set(types.KeyPrefixParams, bz)
	return nil
}

// RewardPoolBalance returns the current balance of the validator reward pool in
// the module's denom.
func (k Keeper) RewardPoolBalance(ctx sdk.Context) sdk.Coin {
	params := k.GetParams(ctx)
	addr := k.accountKeeper.GetModuleAddress(types.ValidatorRewardPoolName)
	return k.bankKeeper.GetBalance(ctx, addr, params.Denom)
}

// SetBlockTxCount records the number of transactions in the block currently
// being finalised.
//
// It is called from the application's PreBlock hook (the only place the ABCI
// request, and therefore the block's transaction list, is visible) and read
// back in EndBlock by DistributeRuneBlockReward to apply Proof of Network
// Activity. The value is overwritten at the start of every block.
func (k Keeper) SetBlockTxCount(ctx sdk.Context, txCount uint64) {
	store := ctx.KVStore(k.storeKey)
	store.Set(types.KeyBlockTxCount, sdk.Uint64ToBigEndian(txCount))
}

// BlockTxCount returns the number of transactions in the block currently being
// finalised, as recorded by SetBlockTxCount.
//
// It returns 0 when no count has been recorded for this block (e.g. genesis).
// A zero count is the "quiet block" case under PoNA, which pays the tier's
// zero-activity reward.
func (k Keeper) BlockTxCount(ctx sdk.Context) uint64 {
	store := ctx.KVStore(k.storeKey)
	bz := store.Get(types.KeyBlockTxCount)
	if len(bz) == 0 {
		return 0
	}
	return sdk.BigEndianToUint64(bz)
}

// GetDPona returns the persisted dPona difficulty-controller state.
//
// A zero value means "dPona off", which is the intended starting condition: on a
// fresh chain no difficulty offset applies and every band uses its base
// thresholds.
func (k Keeper) GetDPona(ctx sdk.Context) types.DPonaState {
	store := ctx.KVStore(k.storeKey)
	bz := store.Get(types.KeyDPona)
	if len(bz) == 0 {
		return types.DPonaState{}
	}

	var state types.DPonaState
	k.cdc.MustUnmarshal(bz, &state)
	return state
}

// SetDPona persists the dPona difficulty-controller state.
func (k Keeper) SetDPona(ctx sdk.Context, state types.DPonaState) {
	store := ctx.KVStore(k.storeKey)
	store.Set(types.KeyDPona, k.cdc.MustMarshal(&state))
}

// UpdateDPona advances the controller by one block and persists the result.
//
// inMaxBand reports whether the block just finalised landed in the max band. The
// returned state is the one now stored, so the caller can log the transition
// without a second read.
//
// This is the ONLY place the difficulty changes. It is called from EndBlock after
// the block's reward has been computed, so a block is never classified by a
// difficulty it caused.
func (k Keeper) UpdateDPona(ctx sdk.Context, inMaxBand bool) types.DPonaState {
	next := k.GetDPona(ctx).ObserveBlock(inMaxBand)
	k.SetDPona(ctx, next)
	return next
}

// FeeOnlyReward returns the fee-funded ladder's scheduled reward for a block
// containing `txCount` transactions, CLAMPED to what the block's fees can pay.
//
//	feeOnlyReward = min(feeOnlyTier.RewardForTxCount(txCount),
//	                    feeSkimPercent% * feePool)
//
// `feePool` is the block's collected fees in the reward denom, captured BEFORE
// PoNA credits its own reward into the fee collector (otherwise the skim would
// pay itself).
//
// Returns zero when the ladder schedules nothing (the zero band, by design) or
// when the clamp reduces it to nothing.
func (k Keeper) FeeOnlyReward(ctx sdk.Context, txCount uint64, feePool sdkmath.Int) sdkmath.Int {
	params := k.GetParams(ctx)

	scheduled := params.FeeOnlyTier.RewardForTxCount(txCount)
	if scheduled.IsZero() || feePool.IsZero() {
		return sdkmath.ZeroInt()
	}

	// skim = feePool * percent / 100, in integer math. Multiply before dividing so
	// small fee pools do not truncate to zero.
	skim := feePool.MulRaw(int64(params.FeeSkimPercent)).QuoRaw(100)
	if skim.IsZero() {
		return sdkmath.ZeroInt()
	}

	if scheduled.GT(skim) {
		return skim
	}

	return scheduled
}

// blockFeeIncome returns this block's collected fees in `denom`, read from the
// distribution fee collector.
//
// PoNA runs LAST in EndBlock, so at this point the collector holds exactly the
// fees taken during DeliverTx for this block -- it is swept into x/distribution
// at the START of the next block. Reading it here is therefore both accurate and
// safe: no reward has been credited yet, so the fee-only ladder cannot skim its
// own payout.
func (k Keeper) blockFeeIncome(ctx sdk.Context, denom string) sdkmath.Int {
	addr := k.accountKeeper.GetModuleAddress(authtypes.FeeCollectorName)
	if addr == nil {
		return sdkmath.ZeroInt()
	}

	return k.bankKeeper.GetBalance(ctx, addr, denom).Amount
}

// advanceDPona feeds this block's band into the difficulty controller.
//
// Called once per block from DistributeRuneBlockReward, on both the paid and
// unpaid paths: difficulty tracks activity, not whether a reward was affordable.
func (k Keeper) advanceDPona(ctx sdk.Context, state types.DPonaState, band types.ActivityBand) {
	next := k.UpdateDPona(ctx, band == types.BandMax)

	if next.Diff != state.Diff {
		k.Logger(ctx).Info(
			"dPona difficulty changed",
			"height", ctx.BlockHeight(),
			"band", string(band),
			"previous_diff", state.Diff,
			"new_diff", next.Diff,
			"txs_offset", next.TxCountOffset(),
			"active", next.IsActive(),
		)
	}
}

// logRewardPaid emits the per-block reward log line.
//
// tx_count is the piece operators need: it is the number of REGULAR transactions
// the block contained (the injected vote-extension pseudo-transaction is
// excluded), and it is what selected the band. It is deliberately NOT taken from
// the CometBFT log lines -- those report num_txs/num_txs_res, which count the
// pseudo-transaction and therefore do not answer "why did this block pay the zero
// band?".
//
// dpona / pona_diff / txs_offset are logged every block so an operator can see
// the difficulty actually in force without querying the module.
func (k Keeper) logRewardPaid(
	ctx sdk.Context,
	height, txCount uint64,
	feePool sdkmath.Int,
	breakdown types.BlockRewardBreakdown,
	reward sdkmath.Int,
	state types.DPonaState,
) {
	k.Logger(ctx).Info(
		"block reward paid",
		"height", height,
		"tx_count", txCount,
		"band", string(breakdown.Band),
		"amount", reward.String(),
		"reward_rune", types.RuneString(reward),
		"tier", breakdown.TierIndex,
		"funding_source", string(breakdown.FundingSource),
		"dpona", state.IsActive(),
		"pona_diff", state.Diff,
		"txs_offset", state.TxCountOffset(),
		"effective_max_txs", breakdown.Tier.MaxTxCountThreshold,
	)

	if state.Diff >= types.DPonaWarnDiff {
		k.Logger(ctx).Warn(
			"dPona difficulty elevated: chain has been at max activity for a sustained period",
			"height", height,
			"pona_diff", state.Diff,
			"txs_offset", state.TxCountOffset(),
		)
	}

	k.Logger(ctx).Debug(
		"block reward breakdown",
		"height", height,
		"tx_count", txCount,
		"band", string(breakdown.Band),
		"tier_index", breakdown.TierIndex,
		"funding_source", string(breakdown.FundingSource),
		"low_tx_count_threshold", breakdown.Tier.LowTxCountThreshold,
		"mid_tx_count_threshold", breakdown.Tier.MidTxCountThreshold,
		"high_tx_count_threshold", breakdown.Tier.HighTxCountThreshold,
		"max_tx_count_threshold", breakdown.Tier.MaxTxCountThreshold,
		"fee_pool", feePool.String(),
	)
}

// FundRewardPool mints `amount` of the module denom into the validator reward
// pool. It is intended to be called once from genesis.
func (k Keeper) FundRewardPool(ctx sdk.Context, amount sdkmath.Int) error {
	params := k.GetParams(ctx)
	coins := sdk.NewCoins(sdk.NewCoin(params.Denom, amount))
	// Minter permission lives on the evm module account (see maccPerms).
	if err := k.bankKeeper.MintCoins(ctx, types.ModuleName, coins); err != nil {
		return err
	}
	return k.bankKeeper.SendCoinsFromModuleToAccount(
		ctx, types.ModuleName, k.accountKeeper.GetModuleAddress(types.ValidatorRewardPoolName), coins,
	)
}
