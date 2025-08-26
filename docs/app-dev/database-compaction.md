# Database Compaction in CometBFT

This document describes the database compaction functionality that has been added to CometBFT to help manage disk space and improve performance.

## Overview

CometBFT now includes automatic database compaction capabilities that work similarly to the compactor used in Cosmos SDK. The compaction process helps reclaim disk space by removing deleted data and optimizing the database structure.

## Features

### 1. Automatic Asynchronous Compaction During Pruning (No Concurrent Compactions)

When blocks are pruned using the `pruneBlocks` function in the `BlockExecutor`, the system automatically compacts both the state database and block store database **asynchronously** in a separate goroutine. The system uses an atomic-based compactor that **prevents multiple compactions from running simultaneously**.

### 2. Manual Compaction Methods

The following methods are available for manual database compaction:

#### State Database Compaction
```go
// Compact the state database
err := stateStore.CompactStateDB()
if err != nil {
    // Handle error
}
```

#### Block Store Compaction
```go
// Compact the block store database
err := blockStore.CompactBlockStore()
if err != nil {
    // Handle error
}
```

#### Combined Compaction
```go
// Compact both state and block store databases
err := blockExecutor.compactStateAndBlockDB()
if err != nil {
    // Handle error
}
```

### 3. Asynchronous Compactor with Concurrency Control

A worker-based compactor is available for non-blocking compaction operations with built-in concurrency control:

```go
// Create a compactor
compactor := NewCompactor(db, logger)

// Enqueue a compaction task (non-blocking, prevents concurrent compactions)
task := compactTask{
    start: nil,  // nil = full range
    limit: nil,  // nil = full range
    label: "manual_compaction",
}

enqueued := compactor.TryEnqueue(task)
if !enqueued {
    // Compaction is already in progress or queue is full
}
```

## Usage Examples

### Example 1: Automatic Asynchronous Compaction During Pruning

The compaction is automatically triggered during the pruning process in `pruneBlocks` and runs asynchronously with concurrency control:

```go
func (blockExec *BlockExecutor) pruneBlocks(retainHeight int64, state State) (uint64, error) {
    // ... pruning logic ...
    
    // Compact databases asynchronously after pruning to reclaim disk space
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
    
    return amountPruned, nil
}
```

**Key Benefits:**
- The pruning process completes immediately without waiting for compaction
- Compaction runs in the background without blocking other operations
- **Only one compaction can run at a time** (atomic-based concurrency control)
- If compaction fails, it doesn't affect the pruning process
- Logs are generated to track compaction success/failure
- If a compaction is already in progress, new compaction requests are skipped

### Example 2: Manual Compaction

You can manually trigger compaction when needed:

```go
// Compact state database
if err := stateStore.CompactStateDB(); err != nil {
    log.Error("failed to compact state database", "err", err)
}

// Compact block store
if err := blockStore.CompactBlockStore(); err != nil {
    log.Error("failed to compact block store", "err", err)
}
```

### Example 3: Using the Asynchronous Compactor

For non-blocking compaction operations with concurrency control:

```go
// Create compactor with a database and logger
compactor := NewCompactor(db, logger)

// Try to enqueue a full database compaction
task := compactTask{
    start: nil,
    limit: nil,
    label: "full_compaction",
}

if compactor.TryEnqueue(task) {
    log.Info("compaction task enqueued successfully")
} else {
    log.Info("compaction skipped - already in progress or queue full")
}
```

### Example 4: Initializing the Compactor

To use the async compactor, you need to initialize it after creating the BlockExecutor:

```go
// After creating the BlockExecutor
blockExec := NewBlockExecutor(stateStore, logger, proxyApp, mempool, evpool, blockStore)

// Initialize the async compactor with the state database
blockExec.InitCompactor(stateDB)
```

## Implementation Details

### Database Interfaces

The compaction methods are implemented on the following interfaces:

- `Store.CompactStateDB()` - Compacts the state database
- `BlockStore.CompactBlockStore()` - Compacts the block store database

### Thread Safety and Concurrency Control

- The `CompactStateDB()` method is thread-safe and can be called concurrently
- The `CompactBlockStore()` method uses read locks to ensure thread safety
- **The asynchronous compactor uses atomic operations (`atomic.CompareAndSwapInt32`) to prevent multiple concurrent compactions**
- **Only one compaction task can be executed at a time across the entire system**
- **Asynchronous compaction during pruning runs in a separate goroutine to avoid blocking**

### Error Handling

- Compaction errors are logged but do not fail the overall pruning process
- The system continues to function even if compaction fails
- Manual compaction methods return errors that should be handled by the caller
- **Asynchronous compaction errors are logged but don't affect the main pruning flow**

## Best Practices

1. **Automatic Compaction**: Let the system handle compaction automatically during pruning operations
2. **Manual Compaction**: Use manual compaction sparingly, typically during maintenance windows
3. **Monitoring**: Monitor disk space and compaction logs to ensure the system is working correctly
4. **Error Handling**: Always handle compaction errors appropriately in your application
5. **Performance**: The asynchronous approach ensures that pruning performance is not impacted by compaction
6. **Concurrency**: The system automatically prevents concurrent compactions, so you don't need to worry about this

## Performance Considerations

- **Asynchronous compaction during pruning ensures no performance impact on the main pruning process**
- **Concurrency control prevents resource contention from multiple simultaneous compactions**
- Compaction can be I/O intensive but runs in the background
- The asynchronous compactor helps minimize performance impact by running in the background
- Compaction is most beneficial after pruning operations when significant data has been removed
- Consider the timing of manual compaction operations to avoid conflicts with normal operations

## Troubleshooting

### Common Issues

1. **Compaction Fails**: Check database permissions and available disk space
2. **Performance Impact**: The asynchronous approach minimizes performance impact
3. **Memory Usage**: Monitor memory usage during compaction operations
4. **Concurrent Compactions**: The system automatically prevents this, but you may see "skipped" messages in logs

### Log Messages

The system logs compaction activities with the following messages:
- `"compaction_start"` - Compaction operation started
- `"compaction_done"` - Compaction operation completed successfully
- `"compaction_error"` - Compaction operation failed
- `"enqueued async database compaction after pruning"` - Async compaction task enqueued
- `"skipped async compaction - already in progress or queue full"` - Compaction skipped due to concurrency control
- `"successfully compacted databases after pruning"` - Automatic compaction completed
- `"failed to compact databases after pruning"` - Automatic compaction failed

## Migration from Synchronous to Asynchronous

The compaction process has been updated to run asynchronously during pruning operations with concurrency control. This change:

1. **Improves performance** by not blocking the pruning process
2. **Maintains reliability** by logging compaction results
3. **Preserves functionality** while enhancing user experience
4. **Reduces latency** in block processing during pruning operations
5. **Prevents resource contention** by ensuring only one compaction runs at a time 