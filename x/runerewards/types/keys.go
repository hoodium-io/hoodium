package types

const (
	// ModuleName is the name of the runerewards module.
	ModuleName = "runerewards"

	// StoreKey is the primary store key for the runerewards module.
	StoreKey = ModuleName

	// ValidatorRewardPoolName is the name of the module account holding the
	// pre-funded RUNE reserve that funds validator block rewards. When it is
	// exhausted, block rewards become fee-only.
	ValidatorRewardPoolName = "validator_reward_pool"
)

// KVStore key prefixes for the runerewards persistent store.
const (
	// prefixParams is the prefix under which module parameters are stored.
	prefixParams = iota + 1
)

// KeyPrefixParams is the KVStore prefix for module parameters.
var KeyPrefixParams = []byte{prefixParams}
