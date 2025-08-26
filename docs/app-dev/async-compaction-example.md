# Asynchronous Database Compaction Example

This example demonstrates how the asynchronous database compaction works in CometBFT with concurrency control.

## How It Works

When blocks are pruned, the compaction process now runs asynchronously in a separate goroutine with atomic-based concurrency control:

```go
// In pruneBlocks function
func (blockExec *BlockExecutor) pruneBlocks(retainHeight int64, state State) (uint64, error) {
    // ... pruning logic happens here ...
    
    // Compact databases asynchronously after pruning
    // Use the async compactor to prevent concurrent compactions
    if blockExec.compactor != nil {
        task := compactTask{
            start:       nil,
            limit:       nil,
            label:       "post_pruning_compaction",
            stateStore:  blockExec.store,
            blockStore:  blockExec.blockStore,
            compactBoth: true,
        }
        
        if blockExec.compactor.TryEnqueue(task) {
            blockExec.logger.Info("enqueued async database compaction after pruning")
        } else {
            blockExec.logger.Debug("skipped async compaction - already in progress or queue full")
        }
    } else {
        // Fallback to synchronous compaction if no async compactor is available
        blockExec.logger.Error("no async compactor available, using synchronous compaction")
        if err := blockExec.compactStateAndBlockDB(); err != nil {
            blockExec.logger.Error("failed to compact databases after pruning", "err", err)
        } else {
            blockExec.logger.Info("successfully compacted databases after pruning")
        }
    }
    
    // This returns immediately, without waiting for compaction
    return amountPruned, nil
}
```

## Benefits

1. **Non-blocking**: The pruning process completes immediately
2. **Background processing**: Compaction runs in the background
3. **Error isolation**: Compaction failures don't affect pruning
4. **Logging**: Success/failure is logged for monitoring
5. **Concurrency control**: Only one compaction can run at a time
6. **Resource protection**: Prevents multiple simultaneous compactions

## Concurrency Control

The system uses atomic operations to ensure only one compaction runs at a time:

```go
// In the compactor worker
func (c *Compactor) worker() {
    for task := range c.queue {
        // Atomic check: only proceed if no compaction is in flight
        if !atomic.CompareAndSwapInt32(&c.inFlight, 0, 1) {
            continue // Skip if another compaction is already running
        }
        
        // Perform compaction...
        
        // Mark compaction as complete
        atomic.StoreInt32(&c.inFlight, 0)
    }
}
```

## Log Output

When pruning occurs, you'll see logs like:

```
INFO pruning blocks retainHeightNumber 1000 currentHeight 1500 base 900 isArchive false
INFO enqueued async database compaction after pruning
INFO compaction_start label=post_pruning_compaction
INFO compaction_done label=post_pruning_compaction ms=1500
```

If a compaction is already in progress:

```
INFO pruning blocks retainHeightNumber 1000 currentHeight 1500 base 900 isArchive false
DEBUG skipped async compaction - already in progress or queue full
```

Or if there's an error:

```
INFO pruning blocks retainHeightNumber 1000 currentHeight 1500 base 900 isArchive false
INFO enqueued async database compaction after pruning
INFO compaction_start label=post_pruning_compaction
ERROR compaction_error label=post_pruning_compaction err="database compaction failed"
```

## Manual Usage

You can also trigger compaction manually:

```go
// Synchronous compaction (blocks until complete)
err := stateStore.CompactStateDB()
err = blockStore.CompactBlockStore()

// Asynchronous compaction using the compactor (with concurrency control)
compactor := NewCompactor(db, logger)
task := compactTask{
    start: nil,
    limit: nil,
    label: "manual_compaction",
}

if compactor.TryEnqueue(task) {
    log.Info("compaction enqueued successfully")
} else {
    log.Info("compaction skipped - already in progress")
}
```

## Initialization

To use the async compactor, initialize it after creating the BlockExecutor:

```go
// After creating the BlockExecutor
blockExec := NewBlockExecutor(stateStore, logger, proxyApp, mempool, evpool, blockStore)

// Initialize the async compactor with the state database
blockExec.InitCompactor(stateDB)
```

## Performance Impact

- **Before**: Pruning was blocked by compaction (synchronous)
- **After**: Pruning completes immediately, compaction runs in background (asynchronous)
- **Concurrency Control**: Only one compaction can run at a time, preventing resource contention

This ensures that your CometBFT node maintains optimal performance during pruning operations while preventing multiple compactions from running simultaneously. 