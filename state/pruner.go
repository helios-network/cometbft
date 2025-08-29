package state

import (
	"fmt"
	"os"
	"os/signal"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/cometbft/cometbft/libs/log"
)

type pruneTask struct {
	base              int64
	retainHeight      int64
	currentHeight     int64
	state             State
	label             string
	pruneBlocks       bool
	pruneStates       bool
	pruneTransactions bool
	pruneBlockIndexer bool
}

// Callbacks pour les opérations de pruning
type PruneCallbacks struct {
	PruneBlockStore         func(retainHeight int64, state State) (uint64, [][]byte, int64, error)
	PruneStateStore         func(base, retainHeight, prunedHeaderHeight int64) error
	PruneTransactionIndexer func(base, retainHeight int64) error
	PruneBlockIndexer       func(base, retainHeight int64) error
}

type Pruner struct {
	inFlight  int32          // atomic
	queue     chan pruneTask // taille 1 pour éviter le flood
	logger    log.Logger
	stopCh    chan struct{}  // canal pour arrêter le worker
	wg        sync.WaitGroup // pour attendre la fin du worker
	callbacks PruneCallbacks // callbacks pour les opérations de pruning
}

func NewPruner(logger log.Logger, callbacks PruneCallbacks) *Pruner {
	p := &Pruner{
		queue:     make(chan pruneTask, 1),
		logger:    logger,
		stopCh:    make(chan struct{}),
		callbacks: callbacks,
	}

	// Démarrer le worker
	p.wg.Add(1)
	go p.worker()

	// Démarrer la gestion des signaux
	go p.handleSignals()

	return p
}

func (p *Pruner) handleSignals() {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGQUIT, syscall.SIGTERM, syscall.SIGINT)

	<-sigCh
	p.logger.Info("received shutdown signal, stopping pruner")

	// Fermer le canal d'arrêt
	close(p.stopCh)

	// Attendre que le worker se termine
	p.wg.Wait()

	p.logger.Info("pruner stopped gracefully")
}

func (p *Pruner) worker() {
	defer p.wg.Done()

	for {
		select {
		case task := <-p.queue:
			if !atomic.CompareAndSwapInt32(&p.inFlight, 0, 1) {
				continue
			}
			start := time.Now()
			p.logger.Info("pruning_start", "label", task.label, "base", task.base, "retain_height", task.retainHeight)

			err := p.executePruning(task)

			if err != nil {
				p.logger.Error("pruning_error", "label", task.label, "err", err)
			} else {
				p.logger.Info("pruning_done", "label", task.label, "ms", time.Since(start).Milliseconds())
			}
			atomic.StoreInt32(&p.inFlight, 0)

		case <-p.stopCh:
			p.logger.Info("worker stopping due to shutdown signal")
			return
		}
	}
}

func (p *Pruner) executePruning(task pruneTask) error {
	var totalPruned uint64

	// Prune blocks if requested
	if task.pruneBlocks && p.callbacks.PruneBlockStore != nil {
		amountPrunedBlocks, _, prunedHeaderHeight, err := p.callbacks.PruneBlockStore(task.retainHeight, task.state)
		if err != nil {
			p.logger.Error("failed to prune block store", "err", err)
			return fmt.Errorf("block store pruning failed: %w", err)
		}
		totalPruned = amountPrunedBlocks
		p.logger.Info("pruned blocks", "amount", amountPrunedBlocks, "from_height", task.base, "to_height", task.retainHeight)

		// Prune states if requested
		if task.pruneStates && p.callbacks.PruneStateStore != nil {
			err = p.callbacks.PruneStateStore(task.base, task.retainHeight, prunedHeaderHeight)
			if err != nil {
				p.logger.Error("failed to prune state store", "err", err)
				return fmt.Errorf("state store pruning failed: %w", err)
			}
			p.logger.Info("pruned states", "from_height", task.base, "to_height", task.retainHeight)
		}
	}

	// Prune transactions if requested
	if task.pruneTransactions && p.callbacks.PruneTransactionIndexer != nil {
		err := p.callbacks.PruneTransactionIndexer(task.base, task.retainHeight)
		if err != nil {
			p.logger.Error("failed to prune transaction indexer", "err", err)
			return fmt.Errorf("transaction indexer pruning failed: %w", err)
		}
		p.logger.Info("pruned transactions", "from_height", task.base, "to_height", task.retainHeight)
	}

	// Prune block indexer if requested
	if task.pruneBlockIndexer && p.callbacks.PruneBlockIndexer != nil {
		err := p.callbacks.PruneBlockIndexer(task.base, task.retainHeight)
		if err != nil {
			p.logger.Error("failed to prune block indexer", "err", err)
			return fmt.Errorf("block indexer pruning failed: %w", err)
		}
		p.logger.Info("pruned block indexer", "from_height", task.base, "to_height", task.retainHeight)
	}

	p.logger.Info("pruning completed successfully", "total_pruned", totalPruned)
	return nil
}

// Stop arrête proprement le pruner
func (p *Pruner) Stop() {
	p.logger.Info("stopping pruner")
	close(p.stopCh)
	p.wg.Wait()
	p.logger.Info("pruner stopped")
}

// Non-bloquant : si worker occupé ou queue pleine, on skippe
func (p *Pruner) TryEnqueue(task pruneTask) bool {
	select {
	case <-p.stopCh:
		// Pruner en cours d'arrêt
		return false
	default:
		// Continuer normalement
	}

	if atomic.LoadInt32(&p.inFlight) == 1 {
		return false
	}
	select {
	case p.queue <- task:
		return true
	default:
		return false
	}
}
