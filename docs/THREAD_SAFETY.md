# Thread-Safe Database Operations

## Overview

This document describes the thread-safety improvements implemented in the database layer to support concurrent operations in batch mode and other multi-threaded scenarios.

## Problem Statement

The original `Store` and `Queue` implementations used SQLite without proper synchronization, leading to potential race conditions when multiple goroutines accessed the database concurrently. This was particularly problematic in:

1. **Batch Runner**: Multiple workers processing jobs simultaneously
2. **Memory Operations**: Concurrent reads/writes to working memory
3. **Plan Management**: Simultaneous plan creation and updates

## Solution

### 1. Store Thread-Safety (`internal/database/store.go`)

Added `sync.RWMutex` to the `Store` struct to protect all database operations:

```go
type Store struct {
    db *sql.DB
    mu sync.RWMutex // Protects all database operations
}
```

#### Locking Strategy

- **Read operations** use `RLock()` for concurrent reads:
  - `GetContextAwareHistory()`
  - `GetWorkingMemory()`
  - `ListMemoryFiles()`
  - `CreateSnapshot()`
  - `GetPlan()`

- **Write operations** use `Lock()` for exclusive access:
  - `AddMessage()`
  - `ClearHistory()`
  - `UpdateWorkingMemory()`
  - `ClearWorkingMemory()`
  - `RemoveFromMemory()`
  - `RestoreSnapshot()`
  - `SavePlan()`
  - `UpdatePlanStatus()`
  - `Close()`

#### Benefits

1. **Concurrent Reads**: Multiple goroutines can read simultaneously without blocking
2. **Safe Writes**: Write operations are serialized to prevent data corruption
3. **Transaction Safety**: SQLite transactions are protected by the mutex
4. **Close Safety**: Proper synchronization during database closure

### 2. Queue Thread-Safety (`internal/batch/queue.go`)

Added `sync.RWMutex` to the `Queue` struct:

```go
type Queue struct {
    db *sql.DB
    mu sync.RWMutex // Protects all queue operations
}
```

#### Locking Strategy

- **Read operations** use `RLock()`:
  - `Next()` - Uses RLock as it's typically followed by UpdateStatus
  - `List()`

- **Write operations** use `Lock()`:
  - `Add()`
  - `UpdateStatus()`
  - `Clear()`

#### Special Considerations

The `Next()` method uses `RLock()` even though it reads data because:
1. It's typically immediately followed by `UpdateStatus()` in the same goroutine
2. Using RLock allows multiple workers to check for available jobs concurrently
3. The actual job claiming happens via `UpdateStatus()` which uses a full lock

### 3. WAL Mode Enhancement

The database initialization uses SQLite's Write-Ahead Logging (WAL) mode for better concurrent performance:

```go
dsn := fmt.Sprintf("%s?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)", dbPath)
```

Benefits:
- **Improved Concurrency**: Readers don't block writers and vice versa
- **Better Performance**: Reduced lock contention
- **Crash Safety**: Better recovery characteristics

## Testing

Comprehensive test suites verify thread-safety:

### Store Tests (`internal/database/store_test.go`)

1. **TestConcurrentAddMessage**: 10 workers, 20 messages each
2. **TestConcurrentMemoryOperations**: Mixed read/write operations
3. **TestConcurrentSnapshotOperations**: Concurrent snapshot/restore
4. **TestConcurrentPlanOperations**: Concurrent plan management
5. **TestMixedConcurrentOperations**: All operations simultaneously
6. **TestRaceDetection**: Rapid concurrent access (run with `-race`)
7. **TestHistoryConsistency**: Message ordering verification
8. **TestMemoryHashOptimization**: Content hash deduplication under load

### Queue Tests (`internal/batch/queue_test.go`)

1. **TestConcurrentAdd**: 10 workers adding 20 jobs each
2. **TestConcurrentNextAndUpdate**: 5 workers processing 50 jobs
3. **TestConcurrentListOperations**: Concurrent readers/writers
4. **TestConcurrentClear**: Concurrent cleanup operations
5. **TestMixedQueueOperations**: All operations simultaneously
6. **TestRaceDetection**: Rapid concurrent access (run with `-race`)
7. **TestJobPriorityOrdering**: Priority preservation under concurrency

### Running Tests

```bash
# Run all database tests
go test ./internal/database -v

# Run with race detector
go test ./internal/database -race -v

# Run all batch tests
go test ./internal/batch -v

# Run with race detector
go test ./internal/batch -race -v

# Run all tests with race detector
go test ./... -race -v
```

## Performance Considerations

### Lock Granularity

The current implementation uses a single mutex per store/queue. This is appropriate because:

1. SQLite itself serializes writes
2. Database operations are relatively fast
3. The complexity of fine-grained locking would outweigh benefits
4. WAL mode already provides good concurrent read performance

### Potential Improvements

For high-throughput scenarios, consider:

1. **Connection Pooling**: Though SQLite has limitations here
2. **Batch Operations**: Group multiple operations into transactions
3. **Read Replicas**: For read-heavy workloads (requires WAL mode)
4. **Caching Layer**: In-memory cache for frequently accessed data

## Migration Notes

### Breaking Changes

None. The API remains unchanged, only the internal synchronization improved.

### Backward Compatibility

Fully compatible with existing code. The changes are internal to the `Store` and `Queue` implementations.

### Upgrade Path

No special migration needed. Simply update the code and existing databases will work with the new thread-safe implementation.

## Best Practices

### Do's

✅ Use the provided Store and Queue methods directly  
✅ Create separate Store instances for truly independent operations  
✅ Run tests with `-race` flag during development  
✅ Use batch operations when possible to reduce lock contention  
✅ Let the mutex handle synchronization - don't add external locks  

### Don'ts

❌ Don't use `Store.DB()` directly for concurrent operations without external locking  
❌ Don't hold locks while performing expensive I/O operations  
❌ Don't create multiple Store instances pointing to the same database file  
❌ Don't bypass Store methods to access the database directly  
❌ Don't assume operation ordering across goroutines without explicit synchronization  

## Example Usage

### Batch Processing with Multiple Workers

```go
func processJobsConcurrently(queue *batch.Queue, store *database.Store, numWorkers int) {
    var wg sync.WaitGroup
    
    for i := 0; i < numWorkers; i++ {
        wg.Add(1)
        go func(workerID int) {
            defer wg.Done()
            
            for {
                // Thread-safe: Get next job
                job, err := queue.Next()
                if err != nil || job == nil {
                    return
                }
                
                // Thread-safe: Update status
                queue.UpdateStatus(job.ID, batch.StatusRunning, "", "")
                
                // Thread-safe: Update working memory
                store.UpdateWorkingMemory("current_job.txt", job.Task)
                
                // Process job...
                result := processJob(job)
                
                // Thread-safe: Mark complete
                queue.UpdateStatus(job.ID, batch.StatusCompleted, result, "")
            }
        }(i)
    }
    
    wg.Wait()
}
```

### Concurrent Memory Operations

```go
func updateMemoryConcurrently(store *database.Store, files []string) {
    var wg sync.WaitGroup
    
    for _, file := range files {
        wg.Add(1)
        go func(path string) {
            defer wg.Done()
            
            content, err := readFile(path)
            if err != nil {
                return
            }
            
            // Thread-safe: Update working memory
            store.UpdateWorkingMemory(path, content)
        }(file)
    }
    
    wg.Wait()
    
    // Thread-safe: Get consolidated memory
    memory, tokens, _ := store.GetWorkingMemory()
    fmt.Printf("Memory size: %d tokens\n", tokens)
}
```

## Monitoring and Debugging

### Race Detection

Always run tests with the race detector enabled during development:

```bash
go test -race ./...
```

### Deadlock Detection

The implementation uses `RWMutex` which can help identify potential deadlocks:

1. Ensure locks are always released (use `defer`)
2. Avoid nested lock acquisitions
3. Keep critical sections short
4. Use timeouts for long-running operations

### Performance Profiling

Profile lock contention:

```bash
go test -bench=. -benchmem -cpuprofile=cpu.prof ./internal/database
go tool pprof cpu.prof
```

## References

- [SQLite WAL Mode](https://www.sqlite.org/wal.html)
- [Go sync.RWMutex Documentation](https://pkg.go.dev/sync#RWMutex)
- [Go Race Detector](https://go.dev/blog/race-detector)
- [SQLite Threading Modes](https://www.sqlite.org/threadsafe.html)
