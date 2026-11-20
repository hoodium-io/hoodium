package statedb

import (
	"bytes"
	"errors"
	"math/big"

	"github.com/hoodium-io/hoodium/x/evm/types"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/ethereum/go-ethereum/common"
)

var (
	_          Keeper = &MockKeeper{}
	errAddress        = common.BigToAddress(big.NewInt(100))
)

type MockAccount struct {
	Account Account
	States  Storage
}

type MockKeeper struct {
	Accounts map[common.Address]MockAccount
	Codes    map[common.Hash][]byte
}

func NewMockKeeper() *MockKeeper {
	return &MockKeeper{
		Accounts: make(map[common.Address]MockAccount),
		Codes:    make(map[common.Hash][]byte),
	}
}

func (k MockKeeper) GetAccount(_ sdk.Context, addr common.Address) *Account {
	acct, ok := k.Accounts[addr]
	if !ok {
		return nil
	}
	return &acct.Account
}

func (k MockKeeper) GetState(_ sdk.Context, addr common.Address, key common.Hash) common.Hash {
	return k.Accounts[addr].States[key]
}

func (k MockKeeper) GetCode(_ sdk.Context, codeHash common.Hash) []byte {
	return k.Codes[codeHash]
}

func (k MockKeeper) ForEachStorage(_ sdk.Context, addr common.Address, cb func(key, value common.Hash) bool) {
	if acct, ok := k.Accounts[addr]; ok {
		for k, v := range acct.States {
			if !cb(k, v) {
				return
			}
		}
	}
}

// CommitAccounts applies a whole commit batch.
//
// Unlike the real keeper this mock has no supply to settle, so a balance is just
// a field: what it has to preserve is the *shape* of the batch — one validation
// pass over every account before any of them is written, survivors updated, and
// self-destructed accounts removed through DeleteAccount.
func (k MockKeeper) CommitAccounts(_ sdk.Context, updates []AccountUpdate) error {
	// Validate the whole batch first: a failure part-way through the writes would
	// leave the mock in a state the real keeper's cache-context rollback prevents.
	for _, update := range updates {
		if update.Address == errAddress {
			return errors.New("mock db error")
		}
	}

	for _, update := range updates {
		if update.SelfDestructed {
			if err := k.DeleteAccount(sdk.Context{}, update.Address, nil); err != nil {
				return err
			}
			continue
		}

		acct, exists := k.Accounts[update.Address]
		if exists {
			acct.Account = update.Account
			k.Accounts[update.Address] = acct
			continue
		}
		k.Accounts[update.Address] = MockAccount{Account: update.Account, States: make(Storage)}
	}

	return nil
}

func (k MockKeeper) SetAccount(_ sdk.Context, addr common.Address, account Account) error {
	if addr == errAddress {
		return errors.New("mock db error")
	}
	acct, exists := k.Accounts[addr]
	if exists {
		// update
		acct.Account = account
		k.Accounts[addr] = acct
	} else {
		k.Accounts[addr] = MockAccount{Account: account, States: make(Storage)}
	}
	return nil
}

// SetState writes a storage slot.
//
// It creates the mock account if it does not exist yet, mirroring the real
// keeper: Keeper.SetState writes straight to the KV store and has no account
// precondition. Without this, a commit that writes storage for a not-yet-known
// account would silently lose the write here while succeeding in production —
// which is exactly how CommitAccounts (metadata last) exposes the difference.
func (k MockKeeper) SetState(_ sdk.Context, addr common.Address, key common.Hash, value []byte) {
	acct, ok := k.Accounts[addr]
	if !ok {
		// Nothing to delete from a storage map that does not exist.
		if len(value) == 0 {
			return
		}
		acct = MockAccount{Account: *NewEmptyAccount(), States: make(Storage)}
	}

	if acct.States == nil {
		acct.States = make(Storage)
	}

	if len(value) == 0 {
		delete(acct.States, key)
	} else {
		acct.States[key] = common.BytesToHash(value)
	}

	k.Accounts[addr] = acct
}

func (k MockKeeper) SetCode(_ sdk.Context, codeHash []byte, code []byte) {
	k.Codes[common.BytesToHash(codeHash)] = code
}

func (k MockKeeper) DeleteAccount(_ sdk.Context, addr common.Address, _ *big.Int) error {
	if addr == errAddress {
		return errors.New("mock db error")
	}
	old := k.Accounts[addr]
	delete(k.Accounts, addr)
	if !bytes.Equal(old.Account.CodeHash, emptyCodeHash) {
		delete(k.Codes, common.BytesToHash(old.Account.CodeHash))
	}
	return nil
}

func (k MockKeeper) Clone() *MockKeeper {
	accounts := make(map[common.Address]MockAccount, len(k.Accounts))
	for k, v := range k.Accounts {
		accounts[k] = v
	}
	codes := make(map[common.Hash][]byte, len(k.Codes))
	for k, v := range k.Codes {
		codes[k] = v
	}
	return &MockKeeper{accounts, codes}
}

func (k MockKeeper) GetStorageRootStrategy(_ sdk.Context) types.StorageRootStrategy {
	return types.StorageRootStrategyEmptyHash
}

func (k MockKeeper) GetMaxPrecompilesCallsPerExecution(_ sdk.Context) uint {
	return 10
}
