package state

import (
	"fmt"
	"sync/atomic"
	"time"

	dbm "github.com/cometbft/cometbft-db"
	"github.com/cometbft/cometbft/libs/log"
)

type compactTask struct {
	start, limit []byte // nil,nil = full range
	label        string
	// Pour la compaction des deux bases de données
	stateStore  Store
	blockStore  BlockStore
	compactBoth bool
}

type Compactor struct {
	inFlight int32            // atomic
	queue    chan compactTask // taille 1 pour éviter le flood
	logger   log.Logger
}

func NewCompactor(logger log.Logger) *Compactor {
	c := &Compactor{
		queue:  make(chan compactTask, 1),
		logger: logger,
	}
	go c.worker()
	return c
}

func (c *Compactor) worker() {
	for task := range c.queue {
		if !atomic.CompareAndSwapInt32(&c.inFlight, 0, 1) {
			continue
		}
		start := time.Now()
		c.logger.Info("compaction_start", "label", task.label)

		var err error
		if task.compactBoth && task.stateStore != nil && task.blockStore != nil {
			// Compaction des deux bases de données
			err = c.CompactBothDatabases(task.stateStore, task.blockStore)
		} else {
			err = fmt.Errorf("compactBoth is false")
		}

		if err != nil {
			c.logger.Error("compaction_error", "label", task.label, "err", err)
		} else {
			c.logger.Info("compaction_done", "label", task.label, "ms", time.Since(start).Milliseconds())
		}
		atomic.StoreInt32(&c.inFlight, 0)
	}
}

// Non-bloquant : si worker occupé ou queue pleine, on skippe
func (c *Compactor) TryEnqueue(task compactTask) bool {
	if atomic.LoadInt32(&c.inFlight) == 1 {
		return false
	}
	select {
	case c.queue <- task:
		return true
	default:
		return false
	}
}

// forceCompact effectue une compaction forcée sur la base de données
func forceCompact(db dbm.DB, start, limit []byte) error {
	return db.Compact(start, limit)
}

// CompactStateDB compacte la base de données de l'état
func (store dbStore) CompactStateDB() error {
	return forceCompact(store.db, nil, nil)
}

// CompactBothDatabases compacte à la fois la state database et la block store database
// Cette fonction est utilisée par le compactor pour éviter les compactions concurrentes
func (c *Compactor) CompactBothDatabases(stateStore Store, blockStore BlockStore) error {
	// Compact state database
	if err := stateStore.CompactStateDB(); err != nil {
		return fmt.Errorf("failed to compact state db: %w", err)
	}

	// Compact block store database
	if err := blockStore.CompactBlockStore(); err != nil {
		return fmt.Errorf("failed to compact block store db: %w", err)
	}

	return nil
}
