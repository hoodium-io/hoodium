package types

// Event types emitted by the pona module.
const (
	// EventTypeRuneBlockReward is emitted when a static block reward is paid
	// from the validator reward pool to the block proposer.
	EventTypeRuneBlockReward = "rune_block_reward"
)

// Attribute keys for pona events.
const (
	AttributeKeyHeight    = "height"
	AttributeKeyValidator = "validator"
	AttributeKeyAmount    = "amount"
	// AttributeKeyTxCount is the number of REGULAR transactions in the rewarded
	// block (the injected app-level pseudo-transaction is excluded), which
	// Proof of Network Activity used to select the reward.
	AttributeKeyTxCount = "tx_count"
	// AttributeKeyBand is the PoNA activity band that was paid: "full", "low"
	// or "zero".
	AttributeKeyBand = "band"
	// AttributeKeyTier is the index of the emission-schedule tier that covered
	// the rewarded height.
	AttributeKeyTier = "tier"
)
