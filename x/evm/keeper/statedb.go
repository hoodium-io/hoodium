// Copyright 2022 Evmos Foundation
// This file is part of the Evmos Network packages.
//
// Evmos is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The Evmos packages are distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the Evmos packages. If not, see https://github.com/evmos/evmos/blob/main/LICENSE
package keeper

import (
	"fmt"
	"math/big"

	storetypes "cosmossdk.io/store/types"

	sdkmath "cosmossdk.io/math"

	errorsmod "cosmossdk.io/errors"
	"cosmossdk.io/store/prefix"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/ethereum/go-ethereum/common"
	"github.com/holiman/uint256"
	runetypes "github.com/hoodium-io/hoodium/types"
	"github.com/hoodium-io/hoodium/x/evm/statedb"
	"github.com/hoodium-io/hoodium/x/evm/types"
)

var _ statedb.Keeper = &Keeper{}

// ----------------------------------------------------------------------------
// StateDB Keeper implementation
// ----------------------------------------------------------------------------

// GetAccount returns the EVM account at addr, or nil if the address has neither
// an x/auth record nor a native balance. go-ethereum treats an address with a
// balance as existing, so GetAccount reports one even when x/auth has no record,
// keeping Exist and Empty aligned with go-ethereum semantics.
func (k *Keeper) GetAccount(ctx sdk.Context, addr common.Address) *statedb.Account {
	acct := k.GetAccountWithoutBalance(ctx, addr)

	balance := k.GetBalance(ctx, addr)

	if acct == nil {
		if balance.Sign() == 0 {
			return nil
		}

		acct = statedb.NewEmptyAccount()
	}

	// Use conversion to bytes rather than uint64 to avoid an overflow error.
	acct.Balance = new(uint256.Int).SetBytes(balance.Bytes())

	return acct
}

// GetState loads contract state from database, implements `statedb.Keeper` interface.
func (k *Keeper) GetState(ctx sdk.Context, addr common.Address, key common.Hash) common.Hash {
	store := prefix.NewStore(ctx.KVStore(k.storeKey), types.AddressStoragePrefix(addr))

	return stateValue(store, key)
}

// GetStateExtension loads contract state from database. This is used as a separate storage
// that is not part of the main EVM persistent store. One of the use cases is storage
// for RUNE precompile functionality.
func (k *Keeper) GetStateExtension(ctx sdk.Context, addr common.Address, key common.Hash) common.Hash {
	store := prefix.NewStore(ctx.KVStore(k.storeKey), types.AddressStorageExtensionPrefix(addr))

	return stateValue(store, key)
}

func stateValue(store prefix.Store, key common.Hash) common.Hash {
	value := store.Get(key.Bytes())
	if len(value) == 0 {
		return common.Hash{}
	}

	return common.BytesToHash(value)
}

// GetCode loads contract code from database, implements `statedb.Keeper` interface.
func (k *Keeper) GetCode(ctx sdk.Context, codeHash common.Hash) []byte {
	store := prefix.NewStore(ctx.KVStore(k.storeKey), types.KeyPrefixCode)
	return store.Get(codeHash.Bytes())
}

// ForEachStorage iterate contract storage, callback return false to break early
func (k *Keeper) ForEachStorage(ctx sdk.Context, addr common.Address, cb func(key, value common.Hash) bool) {
	store := ctx.KVStore(k.storeKey)
	prefix := types.AddressStoragePrefix(addr)

	iterator := storetypes.KVStorePrefixIterator(store, prefix)
	defer iterator.Close()

	for ; iterator.Valid(); iterator.Next() {
		key := common.BytesToHash(iterator.Key())
		value := common.BytesToHash(iterator.Value())

		// check if iteration stops
		if !cb(key, value) {
			return
		}
	}
}

// balanceDelta is one account's requested balance change within a single
// commit: the address, and the (always positive) magnitude of the change.
type balanceDelta struct {
	addr   sdk.AccAddress
	amount sdkmath.Int
}

// accountBalance returns the balance of an account, treating a nil balance as
// zero so a partially populated account cannot panic the commit path.
func accountBalance(account statedb.Account) *big.Int {
	if account.Balance == nil {
		return new(big.Int)
	}

	return account.Balance.ToBig()
}

// CommitAccounts writes the accounts produced by one StateDB commit.
//
// # Why this is a batch and not a loop over SetAccount
//
// The EVM only reports the final balance of each account, so a commit arrives as
// a set of *absolute* balances and each account's change has to be recovered by
// comparing against the bank. Applying those changes one account at a time forces
// every credit to be minted and every debit to be burned, which turns a plain
// value transfer — one account up, one down, supply unchanged — into a mint/burn
// pair. That only works while supply has headroom: once total supply reaches the
// cap (which the devnet genesis does exactly, emitting the whole 10B RUNE supply
// up front) the mint half of every transfer is rejected by the bank's mint
// restriction and the whole transaction reverts. Transfers between existing
// holders must not depend on headroom, because they create no coins.
//
// So balances are settled as a set. Debits are collected into the EVM module
// account, credits are paid out of it, and only the *net* change of the batch
// touches supply:
//
//	net = Σ credits − Σ debits
//
// For a transfer, a fee payment, a contract payout, a self-destruct, or any
// combination of them within one transaction, net is zero and no coin is minted
// or burned. A genuinely inflationary write still mints and must still pass the
// supply cap; a deflationary one still burns. Neither is weakened here — the cap
// keeps guarding the only path that can create coins.
//
// NOTE: net is per commit, and a commit is not always a whole transaction — a
// custom precompile mid-execution calls CommitCacheContext, which commits the
// deltas accumulated so far. Such a partial commit can be unbalanced, and then it
// mints or burns the difference exactly as the per-account code would have; the
// surrounding cache context rolls it back if the precompile or the transaction
// fails. The guarantee this change adds is that a *balanced* set of deltas never
// touches supply, which is what a transfer always produces.
func (k *Keeper) CommitAccounts(ctx sdk.Context, updates []statedb.AccountUpdate) error {
	if len(updates) == 0 {
		return nil
	}

	denom := k.GetParams(ctx).EvmDenom
	if denom == "" {
		// A pruned node has no params; refuse rather than guessing a denom and
		// writing balances against it.
		return errorsmod.Wrap(types.ErrInvalidState, "cannot commit balances: evm denom is not set")
	}

	var (
		credits, debits []balanceDelta
		totalCredit     = sdkmath.ZeroInt()
		totalDebit      = sdkmath.ZeroInt()
		survivors       = make([]statedb.AccountUpdate, 0, len(updates))
		removals        = make([]common.Address, 0, len(updates))
	)

	for _, update := range updates {
		// A self-destructed account is removed after its balance is settled, so
		// its metadata is never written.
		if update.SelfDestructed {
			removals = append(removals, update.Address)
		} else {
			survivors = append(survivors, update)
		}

		// The baseline is the live bank balance, not the balance the state object
		// was loaded with: a precompile (e.g. the RUNE token) can move coins
		// through x/bank mid-execution, and such a movement is already in the bank
		// while the state object still holds the pre-call value. Comparing against
		// the bank makes the recovered delta the change this commit is actually
		// responsible for.
		addr := sdk.AccAddress(update.Address.Bytes())
		current := k.bankKeeper.GetBalance(ctx, addr, denom).Amount
		target := sdkmath.NewIntFromBigInt(accountBalance(update.Account))

		switch delta := target.Sub(current); delta.Sign() {
		case 1:
			credits = append(credits, balanceDelta{addr, delta})
			totalCredit = totalCredit.Add(delta)
		case -1:
			debits = append(debits, balanceDelta{addr, delta.Neg()})
			totalDebit = totalDebit.Add(delta.Neg())
		default:
			// Balance unchanged: nothing to move.
		}
	}

	// Fund the batch before paying anything out, so credits are always covered by
	// coins already collected. When net is positive this is the only mint.
	net := totalCredit.Sub(totalDebit)
	if net.IsPositive() {
		coins := sdk.NewCoins(sdk.NewCoin(denom, net))
		if err := k.bankKeeper.MintCoins(ctx, types.ModuleName, coins); err != nil {
			return err
		}
	}

	// Debits first: they fund the module account the credits are paid from.
	for _, d := range debits {
		coins := sdk.NewCoins(sdk.NewCoin(denom, d.amount))
		if err := k.bankKeeper.SendCoinsFromAccountToModule(ctx, d.addr, types.ModuleName, coins); err != nil {
			return err
		}
	}

	// Credits go module -> account rather than through a bare SendCoins so the
	// bank's blocked-address rule keeps applying to EVM recipients as before.
	for _, c := range credits {
		coins := sdk.NewCoins(sdk.NewCoin(denom, c.amount))
		if err := k.bankKeeper.SendCoinsFromModuleToAccount(ctx, types.ModuleName, c.addr, coins); err != nil {
			return err
		}
	}

	// Anything left over is a genuine supply reduction.
	if net.IsNegative() {
		coins := sdk.NewCoins(sdk.NewCoin(denom, net.Neg()))
		if err := k.bankKeeper.BurnCoins(ctx, types.ModuleName, coins); err != nil {
			return err
		}
	}

	// Metadata is written only once every balance movement has succeeded, so a
	// rejected balance cannot leave a half-updated account behind.
	for _, update := range survivors {
		if err := k.setAccountMeta(ctx, update.Address, update.Account); err != nil {
			return err
		}
	}

	for _, addr := range removals {
		if err := k.removeAccount(ctx, addr); err != nil {
			return err
		}
	}

	return nil
}

// SetBalance sets an account's balance to amount by comparing with the current
// balance, then minting or burning the difference.
//
// NOTE: being single-account, this cannot recognise a value movement: a caller
// that credits one account and debits another must use CommitAccounts, or it
// will mint and burn coins for a transfer that creates none. Commits go through
// CommitAccounts for exactly that reason.
func (k *Keeper) SetBalance(ctx sdk.Context, addr common.Address, amount *big.Int) error {
	cosmosAddr := sdk.AccAddress(addr.Bytes())

	params := k.GetParams(ctx)
	coin := k.bankKeeper.GetBalance(ctx, cosmosAddr, params.EvmDenom)
	balance := coin.Amount.BigInt()
	delta := new(big.Int).Sub(amount, balance)
	switch delta.Sign() {
	case 1:
		// mint
		coins := sdk.NewCoins(sdk.NewCoin(params.EvmDenom, sdkmath.NewIntFromBigInt(delta)))
		if err := k.bankKeeper.MintCoins(ctx, types.ModuleName, coins); err != nil {
			return err
		}
		if err := k.bankKeeper.SendCoinsFromModuleToAccount(ctx, types.ModuleName, cosmosAddr, coins); err != nil {
			return err
		}
	case -1:
		// burn
		coins := sdk.NewCoins(sdk.NewCoin(params.EvmDenom, sdkmath.NewIntFromBigInt(new(big.Int).Neg(delta))))
		if err := k.bankKeeper.SendCoinsFromAccountToModule(ctx, cosmosAddr, types.ModuleName, coins); err != nil {
			return err
		}
		if err := k.bankKeeper.BurnCoins(ctx, types.ModuleName, coins); err != nil {
			return err
		}
	default:
		// not changed
	}
	return nil
}

// bumpAccNonce increments the account-keeper sequence at addr by one,
// mirroring the consensus-path bump that
// EthIncrementSenderSequenceDecorator performs in ante. Query-only
// entry points (eth_call, eth_estimateGas, debug_trace*,
// eth_simulateV1) bypass ante and would otherwise read a pre-bump
// sender nonce while consensus reads the post-bump one, so
// applyMessageWithConfig stays a single geth-pure path regardless of
// caller.
//
// One concrete consequence is EIP-7702: a self-sponsored authorization
// signs its tuple against state_nonce + 1, and applyMessageWithConfig
// validates against the post-bump value — without this bump every
// self-sponsored auth on a query path is silently nonce-mismatched.
//
// Mutations land on the supplied ctx; callers that need a stable
// baseline across iterations must pass a CacheContext.
func (k *Keeper) bumpAccNonce(ctx sdk.Context, addr common.Address) error {
	acct := k.GetAccount(ctx, addr)
	if acct == nil {
		acc := k.accountKeeper.NewAccountWithAddress(ctx, addr[:])
		k.accountKeeper.SetAccount(ctx, acc)
		acct = statedb.NewEmptyAccount()
	}
	acct.Nonce = k.GetNonce(ctx, addr) + 1
	return k.SetAccount(ctx, addr, *acct)
}

// setAccountMeta writes the nonce and code hash of an account, creating the auth
// account if it does not exist yet, and leaves the balance to the caller.
//
// Balance is deliberately out of scope here: it is not a per-account fact during
// a commit, it is one half of a movement that CommitAccounts settles across the
// whole batch.
func (k *Keeper) setAccountMeta(ctx sdk.Context, addr common.Address, account statedb.Account) error {
	cosmosAddr := sdk.AccAddress(addr.Bytes())
	acct := k.accountKeeper.GetAccount(ctx, cosmosAddr)
	if acct == nil {
		acct = k.accountKeeper.NewAccountWithAddress(ctx, cosmosAddr)
	}

	if err := acct.SetSequence(account.Nonce); err != nil {
		return err
	}

	codeHash := common.BytesToHash(account.CodeHash)

	if ethAcct, ok := acct.(runetypes.EthAccountI); ok {
		if err := ethAcct.SetCodeHash(codeHash); err != nil {
			return err
		}
	}

	k.accountKeeper.SetAccount(ctx, acct)

	k.Logger(ctx).Debug(
		"account updated",
		"ethereum-address", addr.Hex(),
		"nonce", account.Nonce,
		"codeHash", codeHash.Hex(),
		"balance", accountBalance(account),
	)
	return nil
}

// SetAccount updates nonce/balance/codeHash together.
//
// NOTE: this is the single-account form, so it writes the balance with
// SetBalance and therefore mints or burns the difference. Batch commits must use
// CommitAccounts instead — see its doc comment for why.
func (k *Keeper) SetAccount(ctx sdk.Context, addr common.Address, account statedb.Account) error {
	if err := k.setAccountMeta(ctx, addr, account); err != nil {
		return err
	}

	return k.SetBalance(ctx, addr, accountBalance(account))
}

// deletableAccount returns the auth account at addr, or nil when there is none,
// and refuses account types that must never be destroyed.
//
// NOTE: only Ethereum accounts (contracts) can be selfdestructed.
func (k *Keeper) deletableAccount(ctx sdk.Context, addr common.Address) (sdk.AccountI, error) {
	acct := k.accountKeeper.GetAccount(ctx, sdk.AccAddress(addr.Bytes()))
	if acct == nil {
		return nil, nil
	}

	if _, ok := acct.(runetypes.EthAccountI); !ok {
		return nil, errorsmod.Wrapf(types.ErrInvalidAccount, "type %T, address %s", acct, addr)
	}

	return acct, nil
}

// removeAccount deletes an account's stored contract state, its x/evm account
// record and its auth account. It does not touch balances — the caller settles
// those first, because the coins of a destroyed account are transferred to the
// beneficiary rather than disappearing.
//
// Storage is cleared even when the account has no x/auth record: an address can
// hold contract state and a bank balance without an auth account, and a destroyed
// contract must not leave its storage readable.
func (k *Keeper) removeAccount(ctx sdk.Context, addr common.Address) error {
	acct, err := k.deletableAccount(ctx, addr)
	if err != nil {
		return err
	}

	// clear storage
	store := ctx.KVStore(k.storeKey)
	prefix := types.AddressStoragePrefix(addr)
	iterator := storetypes.KVStorePrefixIterator(store, prefix)

	var keys []common.Hash
	for ; iterator.Valid(); iterator.Next() {
		key := common.BytesToHash(iterator.Key())
		keys = append(keys, key)
	}
	iterator.Close()

	for _, key := range keys {
		k.SetState(ctx, addr, key, nil)
	}

	if acct == nil {
		return nil
	}

	// remove auth account
	k.accountKeeper.RemoveAccount(ctx, acct)

	k.Logger(ctx).Debug(
		"account suicided",
		"ethereum-address", addr.Hex(),
		"cosmos-address", sdk.AccAddress(addr.Bytes()).String(),
	)

	return nil
}

// SetState update contract storage, delete if value is empty.
func (k *Keeper) SetState(ctx sdk.Context, addr common.Address, key common.Hash, value []byte) {
	store := prefix.NewStore(ctx.KVStore(k.storeKey), types.AddressStoragePrefix(addr))
	k.stateAction(store, ctx, addr, key, value)
}

// SetStateExtension update contract extension storage, delete if value is empty. This is used as
// a separate storage that is not part of the main EVM persistent store. One of the use cases is storage
// for RUNE precompile functionality.
func (k *Keeper) SetStateExtension(ctx sdk.Context, addr common.Address, key common.Hash, value []byte) {
	store := prefix.NewStore(ctx.KVStore(k.storeKey), types.AddressStorageExtensionPrefix(addr))
	k.stateAction(store, ctx, addr, key, value)
}

func (k *Keeper) stateAction(store prefix.Store, ctx sdk.Context, addr common.Address, key common.Hash, value []byte) {
	action := "updated"
	if len(value) == 0 {
		store.Delete(key.Bytes())
		action = "deleted"
	} else {
		store.Set(key.Bytes(), value)
	}
	k.Logger(ctx).Debug(
		fmt.Sprintf("state %s", action),
		"ethereum-address", addr.Hex(),
		"key", key.Hex(),
	)
}

// SetCode set contract code, delete if code is empty.
func (k *Keeper) SetCode(ctx sdk.Context, codeHash, code []byte) {
	store := prefix.NewStore(ctx.KVStore(k.storeKey), types.KeyPrefixCode)

	// store or delete code
	action := "updated"
	if len(code) == 0 {
		store.Delete(codeHash)
		action = "deleted"
	} else {
		store.Set(codeHash, code)
	}
	k.Logger(ctx).Debug(
		fmt.Sprintf("code %s", action),
		"code-hash", common.BytesToHash(codeHash).Hex(),
	)
}

// DeleteAccount handles contract's suicide call:
// - clear balance
// - remove code
// - remove states
// - remove auth account
func (k *Keeper) DeleteAccount(ctx sdk.Context, addr common.Address, balance *big.Int) error {
	acct, err := k.deletableAccount(ctx, addr)
	if err != nil || acct == nil {
		return err
	}

	// clear balance
	if err := k.SetBalance(ctx, addr, balance); err != nil {
		return err
	}

	return k.removeAccount(ctx, addr)
}
