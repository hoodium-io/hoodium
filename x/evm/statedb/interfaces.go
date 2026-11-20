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
package statedb

import (
	"math/big"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/vm"
	"github.com/hoodium-io/hoodium/x/evm/types"
)

// ExtStateDB defines an extension to the interface provided by the go-ethereum
// codebase to support additional state transition functionalities. In particular
// it supports appending a new entry to the state journal through
// AppendJournalEntry so that the state can be reverted after running
// stateful precompiled contracts.
type ExtStateDB interface {
	vm.StateDB
	AppendJournalEntry(JournalEntry)
}

// AccountUpdate is the final state of one dirty account produced by a single
// StateDB commit.
//
// A commit hands the whole slice of these to the keeper in one call, rather
// than writing account by account, because a balance change is only meaningful
// relative to the other balance changes of the same transaction: an EVM value
// transfer credits one account while debiting another, and settling the two
// halves independently would have to create and then destroy supply for a
// movement that does not change supply at all. See Keeper.CommitAccounts.
type AccountUpdate struct {
	// Address is the Ethereum address of the account.
	Address common.Address
	// Account carries the final nonce, balance and code hash of the account.
	Account Account
	// SelfDestructed is true for accounts destroyed during execution. The
	// keeper removes the auth account (and its storage) instead of writing
	// Account's metadata.
	SelfDestructed bool
}

// Keeper provide underlying storage of StateDB
type Keeper interface {
	// Read methods
	GetAccount(ctx sdk.Context, addr common.Address) *Account
	GetState(ctx sdk.Context, addr common.Address, key common.Hash) common.Hash
	GetCode(ctx sdk.Context, codeHash common.Hash) []byte
	// the callback returns false to break early
	ForEachStorage(ctx sdk.Context, addr common.Address, cb func(key, value common.Hash) bool)
	GetStorageRootStrategy(ctx sdk.Context) types.StorageRootStrategy
	GetMaxPrecompilesCallsPerExecution(ctx sdk.Context) uint

	// Write methods, only called by `StateDB.Commit()`
	//
	// CommitAccounts writes the whole batch produced by one commit. Balances are
	// settled as a set so that value movements between accounts never need to
	// mint; only the net supply change of the transaction does. Implementations
	// must therefore not be reduced to a loop over SetAccount.
	CommitAccounts(ctx sdk.Context, updates []AccountUpdate) error
	SetAccount(ctx sdk.Context, addr common.Address, account Account) error
	SetState(ctx sdk.Context, addr common.Address, key common.Hash, value []byte)
	SetCode(ctx sdk.Context, codeHash []byte, code []byte)
	DeleteAccount(ctx sdk.Context, addr common.Address, balance *big.Int) error
}
