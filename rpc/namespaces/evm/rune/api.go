package rune

import (
	"cosmossdk.io/log"

	"github.com/hoodium-io/hoodium/rpc/backend"
	rpctypes "github.com/hoodium-io/hoodium/rpc/types"
	evmtypes "github.com/hoodium-io/hoodium/x/evm/types"
)

// PublicAPI is the custom set of methods prefixed with rune_ in the EVM JSON-RPC API.
type PublicAPI struct {
	logger  log.Logger
	backend backend.EVMBackend
}

func NewPublicAPI(logger log.Logger, backend backend.EVMBackend) *PublicAPI {
	api := &PublicAPI{
		logger:  logger.With("api", "rune"),
		backend: backend,
	}

	return api
}

// EstimateCost returns the estimated cost of a transaction.
func (e *PublicAPI) EstimateCost(args evmtypes.TransactionArgs, blockNrOptional *rpctypes.BlockNumber) (*rpctypes.EstimateCostResult, error) {
	e.logger.Debug("rune_estimateCost")
	return e.backend.EstimateCost(args, blockNrOptional)
}
