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
	// AttributeKeyFundingSource reports which pot paid the reward: "reserve"
	// (the pre-funded validator reward pool) or "fees" (re-allocated from the
	// block's own transaction fees once the reserve is exhausted).
	AttributeKeyFundingSource = "funding_source"
	// AttributeKeyDPona reports whether the dynamic-PoNA difficulty controller is
	// active ("true"/"false"). It is equivalent to diff > 0.
	AttributeKeyDPona = "dpona"
	// AttributeKeyDPonaDiff is the dPona difficulty in +10-transaction steps.
	// 0 means dPona is off, which is also the base ladder.
	AttributeKeyDPonaDiff = "pona_diff"
)
