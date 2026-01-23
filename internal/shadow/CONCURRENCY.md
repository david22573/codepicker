# Shadow Manager Concurrency Guide

## Thread Safety Guarantees

The `shadow.Manager` is fully thread-safe as of the mutex implementation. All public methods can be called concurrently from multiple goroutines without additional synchronization.

## Usage Patterns

### ✅ Safe Patterns

```go
// Pattern 1: Concurrent writes to different files
var wg sync.WaitGroup
for i := 0; i < 100; i++ {
    wg.Add(1)
    go func(id int) {
        defer wg.Done()
        m.WriteFile(fmt.Sprintf("file%d.txt", id), content)
    }(i)
}
wg.Wait()

// Pattern 2: Concurrent reads
var wg sync.WaitGroup
for i := 0; i < 50; i++ {
    wg.Add(1)
    go func() {
        defer wg.Done()
        files, _ := m.ListShadowFiles()
        for _, f := range files {
            m.PreviewDiff(f)
        }
    }()
}
wg.Wait()

// Pattern 3: Mixed read/write operations
go func() {
    m.WriteFile("new.txt", []byte("data"))
    m.RecordAttribution("new.txt", "agent", "task")
}()
go func() {
    files, _ := m.ListShadowFiles()
    // Use files...
}()
```

### ⚠️ Patterns to Avoid

```go
// DON'T: Access Manager fields directly
// WRONG: m.Manifest.Changes["file.txt"] = meta
// RIGHT: m.RecordAttribution("file.txt", agent, task)

// DON'T: Manually lock the mutex (it's private)
// WRONG: m.mu.Lock() // Won't compile - mu is private
// RIGHT: Just call the method - it handles locking
```

## Lock Types Used

### Write Lock (`mu.Lock()`)
Methods that modify state:
- `WriteFile` - Writes shadow file
- `RecordAttribution` - Updates manifest
- `LoadManifest` - Reloads manifest from disk
- `ApplyAtomic` - Applies changes to source
- `Restore` - Restores from backup
- `Cleanup` - Removes shadow directory

### Read Lock (`mu.RLock()`)
Methods that only read state:
- `ListShadowFiles` - Lists shadow files
- `PreviewDiff` - Generates diff preview

### No Lock
Methods with no shared state access:
- `GetShadowPath` - Pure path computation

## Performance Tips

1. **Prefer Read Operations**: `ListShadowFiles` and `PreviewDiff` use read locks, allowing concurrent execution with other readers.

2. **Batch Operations**: Instead of many small writes, consider batching:
   ```go
   // Less efficient
   for _, file := range files {
       m.WriteFile(file, content)
   }
   
   // Better - still safe, but fewer lock acquisitions if processing is done outside
   for _, file := range files {
       processedContent := process(file) // Do work outside lock
       m.WriteFile(file, processedContent) // Quick lock/unlock
   }
   ```

3. **Don't Hold External Locks**: When calling Manager methods, don't hold external locks that might be acquired by other goroutines calling the same Manager:
   ```go
   // RISKY: Could deadlock if otherLock is used in Manager callbacks
   otherLock.Lock()
   m.WriteFile(path, content)
   otherLock.Unlock()
   ```

## Testing for Race Conditions

Always test with the race detector:
```bash
go test -race ./internal/shadow/...
```

## Internal Implementation Notes

### Lock Granularity
- Locks are held for the minimum necessary duration
- File I/O operations are included in the lock scope to ensure consistency
- Internal methods ending in `Locked` assume the caller holds the lock

### Deadlock Prevention
- No method acquires multiple locks
- No circular dependencies between Manager and other components
- Lock ordering is consistent (always Manager's lock first)

### Future Considerations

If performance becomes an issue with high concurrency:
1. Consider per-file locks for WriteFile
2. Use sync.Map for Manifest.Changes for lock-free reads
3. Add metrics to identify contention hotspots

Current implementation prioritizes correctness and simplicity over maximum concurrency.
