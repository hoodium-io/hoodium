package types

// Event types emitted by the runerewards module.
const (
	// EventTypeRuneBlockReward is emitted when a static block reward is paid
	// from the validator reward pool to the block proposer.
	EventTypeRuneBlockReward = "rune_block_reward"
)

// Attribute keys for runerewards events.
const (
	AttributeKeyHeight    = "height"
	AttributeKeyValidator = "validator"
	AttributeKeyAmount    = "amount"
)
