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

	"github.com/hoodium-io/hoodium/x/runerewards/types"
)

// BankKeeper is the subset of the bank keeper used by the runerewards module.
type BankKeeper interface {
	// GetBalance returns the balance of the given address for the denom.
	GetBalance(ctx context.Context, addr sdk.AccAddress, denom string) sdk.Coin
	// SendCoinsFromModuleToAccount transfers coins from a module account to an
	// account. It errors if the module account has insufficient funds.
	SendCoinsFromModuleToAccount(
		ctx context.Context, senderModule string, recipientAddr sdk.AccAddress, amt sdk.Coins,
	) error
	// MintCoins mints new coins from a module account.
	MintCoins(ctx context.Context, moduleName string, amt sdk.Coins) error
}

// StakingKeeper is the subset of the staking keeper used by runerewards.
type StakingKeeper interface {
	// GetValidatorByConsAddr returns the validator for the given consensus
	// address (the block proposer's consensus address).
	GetValidatorByConsAddr(ctx context.Context, consAddr sdk.ConsAddress) (stakingtypes.Validator, error)
}

// AccountKeeper is the subset of the auth keeper used by runerewards.
type AccountKeeper interface {
	GetModuleAddress(moduleName string) sdk.AccAddress
}

// Keeper grants access to the runerewards module state.
type Keeper struct {
	cdc      codec.BinaryCodec
	storeKey storetypes.StoreKey

	bankKeeper    BankKeeper
	stakingKeeper StakingKeeper
	accountKeeper AccountKeeper

	// authority is the gov/module authority allowed to update params.
	authority string
}

// NewKeeper creates a new runerewards keeper. It panics if the authority address
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
