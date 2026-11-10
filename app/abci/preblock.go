package abci

import (
	"fmt"
	"slices"

	"cosmossdk.io/log"

	"golang.org/x/exp/maps"

	cmtabci "github.com/cometbft/cometbft/abci/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/hoodium-io/hoodium/app/abci/types"
)

// TxDecoder decodes a raw transaction into an sdk.Tx.
//
// It is the single source of truth for "is this entry in the block a real
// transaction?". The application's own decoder (baseapp's txDecoder) is the
// right implementation, because it is exactly what BaseApp uses when it decides
// whether to deliver an entry (see BaseApp.FinalizeBlock).
type TxDecoder func(txBytes []byte) (sdk.Tx, error)

// SplitBlockTxs separates the entries of a finalized block into the app-level
// injected pseudo-transaction (if present) and the regular chain transactions.
//
// It returns a nil `injected` when the block carries no app-level pseudo-tx,
// and `regularTxs` always aliases rawTxs when nothing was stripped.
//
// # Why this is not just "skip the first element"
//
// The app-level pseudo-tx is injected at index 0 during PrepareProposal, but
// only when vote extensions are enabled AND at least one sub-handler succeeded
// (see ProposalHandler.PrepareProposalHandler). So index 0 is *not* reliably a
// pseudo-tx: when every sub-handler fails, or before vote extensions activate,
// the first entry is an ordinary transaction.
//
// # Two independent tests, and why both are needed
//
//  1. Vote extensions must be enabled at this height. Without them PrepareProposal
//     short-circuits and injects nothing at all.
//  2. The payload must actually be an InjectedTx. This is the decisive test:
//     `len(injectedTx.Parts) > 0`.
//
// Test 2 alone is not sufficient. gogoproto skips unknown fields, so the
// `InjectedTx.Unmarshal` of an arbitrary Cosmos transaction can succeed with
// an EMPTY Parts map — the pre-block handler must treat that as "no pseudo-tx"
// (see PreBlockHandler.PreBlocker, where the same check gates the sub-handlers).
// Requiring at least one part both mirrors that logic and keeps the answer
// consistent: a pseudo-tx that no sub-handler vouched for is not a pseudo-tx.
//
// A real transaction can never satisfy both tests, because a genuine Cosmos
// transaction does not carry a repeated message field at proto field number 1,
// which is what would be needed to populate Parts.
func SplitBlockTxs(ctx sdk.Context, height int64, rawTxs [][]byte) (injected *types.InjectedTx, regularTxs [][]byte) {
	if len(rawTxs) == 0 {
		return nil, rawTxs
	}

	if !isVoteExtensionsEnabled(ctx, height) {
		return nil, rawTxs
	}

	var candidate types.InjectedTx
	if err := candidate.Unmarshal(rawTxs[0]); err != nil {
		return nil, rawTxs
	}
	if len(candidate.Parts) == 0 {
		// Decoded into the right wire shape but carries nothing: this is not a
		// pseudo-tx (see the function comment).
		return nil, rawTxs
	}

	return &candidate, rawTxs[1:]
}

// CountRegularTxs returns the number of regular chain transactions in a
// finalized block, excluding the app-level injected pseudo-transaction.
//
// # Why the count matters
//
// Proof of Network Activity (x/pona) scales the RUNE block reward by how many
// transactions the block contains, and the zero-activity band pays a nominal
// 1 RUNE precisely so an idle chain does not drain the reserve at the full
// rate. That band is only reachable if a block containing nothing but the
// injected pseudo-tx counts as ZERO transactions.
//
// Hoodium activates vote extensions at height 1 (genesis consensus params), so
// every block after genesis carries the pseudo-tx. Counting the raw block
// length — len(req.Txs) — therefore reports at least 1 for every block ever
// produced: the zero band is unreachable and every quiet block is paid the
// *low activity* rate (20 RUNE in tier 1) instead of 1 RUNE.
//
// count is the number of regular transactions. The optional def decoder is used
// to make the identification of the pseudo-tx airtight: an entry that decodes
// as a real sdk.Tx can never be the pseudo-tx, so it is always counted.
func CountRegularTxs(
	ctx sdk.Context, height int64, rawTxs [][]byte, def TxDecoder,
) uint64 {
	first, regular := SplitBlockTxs(ctx, height, rawTxs)

	count := len(regular)
	if first != nil && def != nil {
		// Defence in depth: a payload that both looks like an InjectedTx and
		// decodes as a valid transaction is ambiguous, so treat it as a real
		// transaction (the conservative choice: it is counted rather than
		// silently free).
		if _, err := def(rawTxs[0]); err == nil {
			count++
		}
	}

	return uint64(count) //nolint:gosec // bounded by the block's transaction list
}

// IModuleManager is an interface representing the module manager.
type IModuleManager interface {
	// PreBlock returns the pre-block logic for the module manager.
	PreBlock(ctx sdk.Context) (*sdk.ResponsePreBlock, error)
}

// IPreBlockHandler is an interface representing the pre-block handler
// (part of the FinalizeBlock ABCI request).
type IPreBlockHandler interface {
	// PreBlocker returns the pre-block handler (part of the FinalizeBlock ABCI request).
	PreBlocker() sdk.PreBlocker
}

// PreBlockHandler is a pre-block handler (part of the FinalizeBlock ABCI request).
// It is designed to be used with multiple sub-handlers, each responsible for a
// specific part of the pre-block logic.
type PreBlockHandler struct {
	logger      log.Logger
	subHandlers map[VoteExtensionPart]IPreBlockHandler
}

// NewPreBlockHandler returns a new PreBlockHandler.
func NewPreBlockHandler(
	logger log.Logger,
	connectSubHandler IPreBlockHandler,
) *PreBlockHandler {
	subHandlers := map[VoteExtensionPart]IPreBlockHandler{
		VoteExtensionPartConnect: connectSubHandler,
	}

	return &PreBlockHandler{
		logger:      logger,
		subHandlers: subHandlers,
	}
}

// PreBlocker returns the pre-block handler (part of the FinalizeBlock ABCI request).
// This function starts by invoking the module manager's PreBlock method to
// invoke module-level pre-block logic. Next, it iterates over app-level
// pre-block sub-handlers, invoking each sub-handler's PreBlocker method.
// The app-level pre-block sub-handlers (unlike the module-level pre-block handlers)
// have access to the corresponding pseudo-transactions injected during the
// proposal phase and can perform state changes based on these transactions.
//
// Dev note: Returning an error from this function will make the FinalizeBlock
// ABCI request fail and lead to an unrecoverable consensus failure.
// Make sure you know what you are doing before returning an error.
//
// Invariants summary:
//  1. Calls only the module manager's PreBlock if vote extensions are not enabled.
//  2. Returns error if the module manager's PreBlock call fails.
//  3. Returns error if any of the sub-handlers fails.
func (pbh *PreBlockHandler) PreBlocker(mm IModuleManager) sdk.PreBlocker {
	return func(
		ctx sdk.Context,
		req *cmtabci.RequestFinalizeBlock,
	) (*sdk.ResponsePreBlock, error) {
		// TODO: Consider changing logging to debug level once this code matures.

		res, err := mm.PreBlock(ctx)
		if err != nil {
			return nil, err
		}

		if !isVoteExtensionsEnabled(ctx, req.Height) {
			// Short-circuit if vote extensions are not enabled.
			return res, nil
		}

		// If the transaction vector for this block is not empty, the
		// first transaction MAY be the app-level pseudo-transaction injected
		// during PrepareProposal. If the first transaction does not unmarshal
		// as an injected pseudo-transaction, that means the PrepareProposal
		// handler did not inject it due to all proposal sub-handlers failing.
		// In this case, the first transaction is a regular application-specific
		// transaction. Moreover, an empty transaction vector is also a valid
		// case and indicates that no pseudo-transaction was injected and there
		// are no regular application-specific txs in the vector.
		var injectedTx types.InjectedTx
		var injectedTxOk bool
		var regularTxs [][]byte
		if len(req.Txs) > 0 {
			if err := injectedTx.Unmarshal(req.Txs[0]); err != nil {
				// If the first transaction does not unmarshal as an injected
				// pseudo-transaction, that means all transactions in the
				// vector are regular chain transactions.
				injectedTxOk = false
				regularTxs = req.Txs
			} else {
				// If the first transaction unmarshals as an injected pseudo-transaction,
				// regular transactions occur after the injected pseudo-transaction.
				injectedTxOk = true
				regularTxs = req.Txs[1:]
			}
		}

		// Unlike in app-level VoteExtensionHandler and ProposalHandler, we
		// cannot think about running the pre-block sub-handlers in parallel.
		// This is because pre-block sub-handlers may change the state and
		// state changes have to occur sequentially. To avoid race, we guarantee
		// the sub-handlers are executed in a deterministic order.
		subHandlerKeys := maps.Keys(pbh.subHandlers)
		slices.Sort(subHandlerKeys)

		for _, part := range subHandlerKeys {
			pbh.logger.Info(
				"running sub-handler to execute pre-block",
				"height", req.Height,
				"part", part,
			)

			// No need to check if the sub-handler exists. We know it exists
			// because we iterate over the keys of the sub-handler map.
			subHandler := pbh.subHandlers[part]

			// Replace the app-level pseudo-transaction with the part-specific
			// pseudo-transaction. Note that the part-specific pseudo-transaction
			// may be zero-length, which is a valid case. The sub-handler is
			// responsible for handling this case.
			var injectedTxPart []byte
			if injectedTxOk && injectedTx.Parts != nil {
				injectedTxPart = injectedTx.Parts[uint32(part)]
			}
			subTxs := append([][]byte{injectedTxPart}, regularTxs...)

			// We ignore the exact response from the sub-handler. Construction
			// of the sdk.ResponsePreBlock implies that it should come from
			// the module manager. We do not want to interfere with that
			// by including responses from sub-handlers. The only goal here is
			// that sub-handlers introduce their state changes, based on
			// the txs they have access to.
			_, err = subHandler.PreBlocker()(ctx, &cmtabci.RequestFinalizeBlock{
				Txs:                subTxs,
				DecidedLastCommit:  req.DecidedLastCommit,
				Misbehavior:        req.Misbehavior,
				Hash:               req.Hash,
				Height:             req.Height,
				Time:               req.Time,
				NextValidatorsHash: req.NextValidatorsHash,
				ProposerAddress:    req.ProposerAddress,
			})
			if err != nil {
				// The vote extension and proposal phases should guarantee the
				// pre-block stage does not encounter any errors. If it does,
				// something went really wrong unexpectedly. The error should
				// bubble up and cause an unrecoverable consensus failure.
				return nil, fmt.Errorf(
					"pre-blocker for part %v failed: %w",
					part,
					err,
				)
			}

			pbh.logger.Info(
				"sub-handler executed pre-block",
				"height", req.Height,
				"part", part,
			)
		}

		pbh.logger.Info(
			"pre-block executed",
			"height", req.Height,
		)

		return res, nil
	}
}
