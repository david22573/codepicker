# Thread-Safety Quick Reference

## ✅ Safe Operations (Now Thread-Safe)

### Store Operations
```go
// All these are now safe for concurrent use:
store.AddMessage(role, content)
store.GetContextAwareHistory(tokenBudget)
store.ClearHistory()
store.UpdateWorkingMemory(path, content)
store.GetWorkingMemory()
store.ClearWorkingMemory()
store.RemoveFromMemory(path)
store.ListMemoryFiles()
store.CreateSnapshot()
store.RestoreSnapshot(snapshot)
store.SavePlan(id, task, steps, cost)
store.GetPlan(id)
store.UpdatePlanStatus(id, status)
store.Close()
```

### Queue Operations
```go
// All these are now safe for concurrent use:
queue.Add(task, priority)
queue.Next()
queue.UpdateStatus(id, status, result, error)
queue.List(limit)
queue.Clear(olderThan)
```

## 🔒 Locking Behavior

### Read Operations (RLock - Concurrent)
Multiple goroutines can execute these simultaneously:
- `GetContextAwareHistory()`
- `GetWorkingMemory()`
- `ListMemoryFiles()`
- `CreateSnapshot()`
- `GetPlan()`
- `Next()` (queue)
- `List()` (queue)

### Write Operations (Lock - Exclusive)
Only one goroutine at a time:
- `AddMessage()`
- `UpdateWorkingMemory()`
- `SavePlan()`
- `Add()` (queue)
- `UpdateStatus()` (queue)
- All Clear/Remove operations

## 📋 Common Patterns

### Pattern 1: Batch Job Processing
```go
func worker(queue *batch.Queue, store *database.Store) {
    for {
        job, err := queue.Next()  // Thread-safe
        if err != nil || job == nil {
            break
        }
        
        queue.UpdateStatus(job.ID, batch.StatusRunning, "", "")  // Thread-safe
        
        // Process job...
        result := doWork(job, store)
        
        queue.UpdateStatus(job.ID, batch.StatusCompleted, result, "")  // Thread-safe
    }
}

// Safe to run multiple workers:
for i := 0; i < numWorkers; i++ {
    go worker(queue, store)
}
```

### Pattern 2: Concurrent Memory Updates
```go
var wg sync.WaitGroup
for _, file := range files {
    wg.Add(1)
    go func(f string) {
        defer wg.Done()
        content := processFile(f)
        store.UpdateWorkingMemory(f, content)  // Thread-safe
    }(file)
}
wg.Wait()
```

### Pattern 3: Concurrent Reads
```go
// Multiple goroutines can read simultaneously
go func() {
    msgs, _ := store.GetContextAwareHistory(5000)  // Thread-safe
    // Use msgs...
}()

go func() {
    files, _ := store.ListMemoryFiles()  // Thread-safe
    // Use files...
}()

go func() {
    memory, _, _ := store.GetWorkingMemory()  // Thread-safe
    // Use memory...
}()
```

## ⚠️ Important Notes

### DO ✅
- Use Store/Queue methods directly - they're thread-safe
- Create separate Store instances for independent databases
- Run tests with `-race` flag
- Use defer to ensure locks are released

### DON'T ❌
- Don't use `store.DB()` directly in concurrent code
- Don't add external locks around Store/Queue methods
- Don't assume operation ordering without synchronization
- Don't hold Store references across long-running operations

## 🧪 Testing

### Run with Race Detector
```bash
# Always test concurrent code with race detector
go test -race ./internal/database
go test -race ./internal/batch

# Or all tests
go test -race ./...
```

### Verify Your Concurrent Code
```go
func TestMyConcurrentFeature(t *testing.T) {
    store, _ := database.New(t.TempDir())
    defer store.Close()
    
    var wg sync.WaitGroup
    const workers = 10
    
    for i := 0; i < workers; i++ {
        wg.Add(1)
        go func(id int) {
            defer wg.Done()
            // Your concurrent operations here
            store.AddMessage("user", fmt.Sprintf("msg %d", id))
        }(i)
    }
    
    wg.Wait()
    // Verify results
}
```

## 🔍 Debugging

### Check for Deadlocks
```go
// Always use defer to prevent deadlocks
func (s *Store) MyMethod() error {
    s.mu.Lock()
    defer s.mu.Unlock()  // ✅ Ensures unlock even on error
    
    // Your code here...
    return nil
}
```

### Profile Lock Contention
```bash
go test -bench=. -benchmem -cpuprofile=cpu.prof
go tool pprof cpu.prof
```

## 📊 Performance Tips

### Batch Operations
```go
// Instead of:
for _, msg := range messages {
    store.AddMessage(msg.Role, msg.Content)  // Acquires lock 100 times
}

// Consider batching if possible (future improvement):
store.AddMessages(messages)  // Acquires lock once
```

### Read Optimization
```go
// Multiple concurrent reads are efficient:
var wg sync.WaitGroup
for i := 0; i < 10; i++ {
    wg.Add(1)
    go func() {
        defer wg.Done()
        store.GetWorkingMemory()  // All execute concurrently
    }()
}
wg.Wait()
```

## 🎯 Real-World Example

```go
// Batch runner with multiple workers (internal/batch/runner.go)
func (r *Runner) Start(ctx context.Context) error {
    var wg sync.WaitGroup
    sem := make(chan struct{}, r.Concurrency)
    
    for {
        select {
        case sem <- struct{}{}:
            job, err := r.Queue.Next()  // ✅ Thread-safe
            if job == nil {
                <-sem
                continue
            }
            
            wg.Add(1)
            go func(j *Job) {
                defer wg.Done()
                defer func() { <-sem }()
                
                // All these operations are thread-safe:
                r.Queue.UpdateStatus(j.ID, StatusRunning, "", "")
                agentCtx.Store.UpdateWorkingMemory(...)
                r.Queue.UpdateStatus(j.ID, StatusCompleted, result, "")
            }(job)
        }
    }
    
    wg.Wait()
    return nil
}
```

## 📚 Further Reading

- Full documentation: [THREAD_SAFETY.md](./THREAD_SAFETY.md)
- SQLite WAL mode: https://www.sqlite.org/wal.html
- Go sync package: https://pkg.go.dev/sync
- Go race detector: https://go.dev/blog/race-detector
