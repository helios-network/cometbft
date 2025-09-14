package indexer

import (
	"context"

	"github.com/cometbft/cometbft/libs/log"
	"github.com/cometbft/cometbft/libs/pubsub/query"
	"github.com/cometbft/cometbft/types"
)

var _ BlockIndexer = (*NullBlockIndexer)(nil)

// NullBlockIndexer is a no-op block indexer.
type NullBlockIndexer struct{}

// Has returns false, indicating no blocks are indexed.
func (nbi *NullBlockIndexer) Has(height int64) (bool, error) {
	return false, nil
}

// Index is a no-op.
func (nbi *NullBlockIndexer) Index(types.EventDataNewBlockEvents) error {
	return nil
}

// Search returns empty results.
func (nbi *NullBlockIndexer) Search(ctx context.Context, q *query.Query) ([]int64, error) {
	return []int64{}, nil
}

// SetLogger is a no-op.
func (nbi *NullBlockIndexer) SetLogger(l log.Logger) {}
