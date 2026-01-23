# Codepicker Improvement Plan

**Generated**: January 22, 2026  
**Priority Scoring**: Critical (P0) → High (P1) → Medium (P2) → Low (P3)  
**Estimated Total Effort**: ~8-12 hours of agent time

---

## 🔴 Critical Priority (P0) - Fix Now

### Race Conditions & Concurrency

- [ ] **[P0] Add mutex protection to shadow.Manager**
  - **File**: `internal/shadow/fs.go`
  - **Issue**: Concurrent writes to `Manifest.Changes` map cause data races
  - **Solution**: Add `sync.RWMutex` to Manager struct, lock all map operations
  - **Test**: Create `internal/shadow/fs_test.go` with concurrent write tests
  - **Estimated Impact**: Prevents data corruption in parallel agent execution

- [ ] **[P0] Fix memory leak in Engine.Run() message accumulation**
  - **File**: `internal/agent/engine.go`
  - **Issue**: `messages` slice grows unbounded in turn loop (can reach 1000+ messages)
  - **Solution**: Keep system message + last 50 messages, prune when len > 100
  - **Code Change**: Add `pruneMessages()` helper function in Run() loop
  - **Estimated Impact**: Reduces memory usage by 70-90% in long sessions

- [ ] **[P0] Thread-safe database operations**
  - **File**: `internal/database/store.go`
  - **Issue**: Concurrent reads/writes to working memory without proper locking
  - **Solution**: Use `sync.RWMutex` for all memory operations, SQLite connection pooling
  - **Test**: Add concurrent read/write tests in `internal/database/store_test.go`
  - **Estimated Impact**: Prevents SQLite "database locked" errors

### Context Window Management

- [ ] **[P0] Implement intelligent context eviction with relevance scoring**
  - **File**: `internal/database/store.go` (modify `GetWorkingMemory()`)
  - **Issue**: Simple LRU eviction loses critical context, agent forgets recent work
  - **Solution**: Score files by: (1) keyword match to current task, (2) recently modified in shadow, (3) explicitly read by agent
  - **Algorithm**: 
    ```
    score = (0.4 * relevanceMatch) + (0.3 * recencyBoost) + (0.3 * explicitRead)
    evict lowest scoring files first
    ```
  - **Estimated Impact**: 40% reduction in "I need to re-read that file" loops

---

## 🟡 High Priority (P1) - Important Improvements

### Reliability & Recovery

- [ ] **[P1] Implement checkpoint system for resumable agent sessions**
  - **Files**: 
    - `internal/agent/checkpoint.go` (NEW)
    - `internal/database/schema.go` (add migration for checkpoints table)
    - `cmd/agent.go` (add `--resume` flag)
  - **Issue**: Agent crashes lose all progress, must restart from scratch
  - **Solution**: Save checkpoint every 5 turns containing: stepID, memory snapshot, message history, timestamp
  - **Schema**:
    ```sql
    CREATE TABLE checkpoints (
      id TEXT PRIMARY KEY,
      session_id TEXT,
      turn_number INTEGER,
      memory_snapshot TEXT,
      messages_json TEXT,
      created_at DATETIME
    );
    ```
  - **Usage**: `codepicker agent run "task" --resume` auto-detects incomplete sessions
  - **Estimated Impact**: Saves hours of wasted work on crashes

- [ ] **[P1] Enhanced error recovery system**
  - **File**: `internal/agent/recovery.go` (extend `CommonFailures`)
  - **Issue**: Only handles 3 Go-specific errors, many common failures unhandled
  - **Solution**: Add recovery strategies for:
    - Python: `ModuleNotFoundError` → suggest `pip install <package>`
    - npm: `Cannot find module` → suggest `npm install`
    - Permission denied → suggest `chmod +x <file>`
    - Git conflicts → suggest resolution workflow
  - **Pattern**:
    ```go
    {
      Name: "PythonModuleMissing",
      Pattern: regexp.MustCompile(`ModuleNotFoundError: No module named '(\w+)'`),
      FixCommands: []CommandSequence{
        {Binary: "pip", Args: []string{"install", "$1"}},
      },
    }
    ```
  - **Estimated Impact**: Reduces manual intervention by 50%

### Testing & Quality

- [ ] **[P1] Add critical test coverage for concurrency scenarios**
  - **Files to Create**:
    - `internal/shadow/fs_test.go` - Concurrent WriteFile + ApplyAtomic tests
    - `internal/database/store_test.go` - Parallel UpdateWorkingMemory tests
    - `internal/agent/executor_test.go` - Malformed JSON recovery tests
    - `pkg/openrouter/client_test.go` - Network interruption + retry tests
  - **Test Coverage Goal**: 70% for critical paths (currently ~30%)
  - **Estimated Impact**: Catch bugs before production

---

## 🟢 Medium Priority (P2) - UX Enhancements

### Progress & Visibility

- [ ] **[P2] Add progress indicators to plan execution**
  - **Files**:
    - `internal/ui/progress.go` (NEW) - Create ProgressBar component
    - `internal/agent/plan_executor.go` - Integrate progress updates
  - **Issue**: Long-running plans show no progress, users don't know if it's working
  - **Solution**: Show `"Step 3/10: Refactoring database [=====>    ] 30% (2m elapsed)"`
  - **Implementation**:
    ```go
    type ProgressBar struct {
      total int
      current int
      startTime time.Time
    }
    
    func (p *ProgressBar) Update(step int, desc string) {
      percent := float64(step) / float64(p.total) * 100
      elapsed := time.Since(p.startTime)
      fmt.Printf("\r[%3.0f%%] %s | Elapsed: %s", percent, desc, elapsed)
    }
    ```
  - **Estimated Impact**: Better user confidence, fewer "is it frozen?" questions

- [ ] **[P2] Cost estimation and warnings**
  - **Files**:
    - `internal/agent/planner.go` - Add `EstimatePlanCost(plan, contextSize)` method
    - `cmd/agent.go` - Prompt before expensive operations
  - **Issue**: Users surprised by high bills, no cost transparency
  - **Solution**: 
    - Show cost estimate before plan execution
    - Prompt if estimate > $0.50
    - Display running cost every 5 turns
  - **Formula**:
    ```go
    estimatedCost := 0.0
    for _, step := range plan.Steps {
      avgTurns := 3
      tokensPerTurn := contextSize + 2000  // prompt + completion
      estimatedCost += calculateCost(tokensPerTurn, 1000, model) * avgTurns
    }
    ```
  - **Estimated Impact**: Prevents bill shock, builds trust

### Developer Experience

- [ ] **[P2] Improve error messages with actionable guidance**
  - **Files**: All tool executors in `internal/tools/*.go`
  - **Issue**: Generic "operation failed" errors don't help users fix problems
  - **Solution**: Replace all `fmt.Errorf("failed: %w", err)` with contextual help
  - **Template**:
    ```go
    return fmt.Errorf(`Tool '%s' failed: %v
    
    💡 Common fixes:
      1. Check if file exists: ls %s
      2. Verify permissions: ls -la %s
      3. Try absolute path instead of relative
    
    Debug: Run 'codepicker check' to validate environment`,
        toolName, err, path, path)
    ```
  - **Estimated Impact**: Reduces support requests by 30%

- [ ] **[P2] Enhanced diff viewer in TUI**
  - **File**: `internal/tui/review.go`
  - **Issue**: Basic diff display, hard to read large changes
  - **Solution**: 
    - Syntax highlighting via `github.com/alecthomas/chroma`
    - Side-by-side view toggle (press 's')
    - Jump to next/prev change (n/p keys)
    - Inline edit before apply (press 'e')
  - **Dependencies**: Add to `go.mod`: `github.com/alecthomas/chroma v0.10.0`
  - **Estimated Impact**: Faster review, fewer apply-undo cycles

---

## 🔵 Low Priority (P3) - Nice to Have

### Performance Optimizations

- [ ] **[P3] Implement streaming context updates (delta-based)**
  - **Files**: 
    - `internal/agent/memory.go` - Track deltas
    - `internal/agent/engine.go` - Build incremental context
  - **Issue**: Sending full 100k token context every turn wastes tokens/money
  - **Solution**: Send only changes since last turn
    ```go
    type ContextDelta struct {
      Added   []string  // New files in memory
      Removed []string  // Evicted files
      Updated []string  // Modified in shadow
    }
    
    // First turn: send full context
    // Subsequent turns: send "Added: [file1], Updated: [file2]"
    ```
  - **Estimated Impact**: 40% token reduction = 40% cost savings

- [ ] **[P3] Parallel file processing in context generation**
  - **File**: `internal/contextgen/generator.go`
  - **Issue**: Sequential processing slow for large codebases (5+ seconds for 1000 files)
  - **Solution**: Worker pool pattern
    ```go
    semaphore := make(chan struct{}, runtime.NumCPU())
    results := make(chan fileResult, len(files))
    
    for _, f := range files {
      go func(file string) {
        semaphore <- struct{}{}
        defer func() { <-semaphore }()
        // process file
        results <- result
      }(f)
    }
    ```
  - **Estimated Impact**: 3-5x faster context generation

- [ ] **[P3] Cache scan_package results**
  - **File**: `internal/tools/scan.go`
  - **Issue**: Re-scanning unchanged directories wastes time
  - **Solution**: Hash directory contents, cache results for 5 minutes
    ```go
    type ScanCache struct {
      Hash   string    // SHA256 of file mtimes
      Result string    // Cached markdown output
      Expiry time.Time
    }
    ```
  - **Estimated Impact**: Instant re-scans for unchanged code

### Feature Additions

- [ ] **[P3] Add --estimate flag to preview cost without executing**
  - **File**: `cmd/agent.go`
  - **Usage**: `codepicker agent run --estimate "complex task"`
  - **Output**: Show plan + estimated cost, exit without execution
  - **Estimated Impact**: Helps users budget expensive operations

- [ ] **[P3] Implement codepicker undo command**
  - **Files**:
    - `cmd/undo.go` (NEW)
    - `internal/shadow/fs.go` - Track last N applies
  - **Usage**: `codepicker undo` reverts last applied change
  - **Solution**: Keep backup history, restore from latest
  - **Estimated Impact**: Faster experimentation, less fear

- [ ] **[P3] Add shell completion (bash/zsh/fish)**
  - **File**: `cmd/completion.go` (NEW)
  - **Generate with**: `cobra.GenBashCompletion()`, etc.
  - **Install**: `codepicker completion bash > /etc/bash_completion.d/codepicker`
  - **Estimated Impact**: Better CLI discoverability

- [ ] **[P3] Create .codepickerignore templates**
  - **File**: `templates/` directory (NEW)
  - **Templates**: golang, nodejs, python, rust, generic
  - **Usage**: `codepicker init --template golang` copies template
  - **Estimated Impact**: Faster onboarding

---

## 📋 Implementation Order Recommendation

For `codepicker agent improve --loop`, tackle in this sequence:

**Phase 1: Stability (Week 1)**
1. P0-1: Add mutex to shadow.Manager
2. P0-2: Fix memory leak in Engine.Run()
3. P0-3: Thread-safe database operations
4. P1-3: Add critical test coverage

**Phase 2: Intelligence (Week 2)**
5. P0-4: Intelligent context eviction
6. P1-1: Checkpoint system
7. P1-2: Enhanced error recovery

**Phase 3: User Experience (Week 3)**
8. P2-1: Progress indicators
9. P2-2: Cost estimation
10. P2-3: Better error messages
11. P2-4: Enhanced diff viewer

**Phase 4: Optimization (Week 4+)**
12. P3-1: Streaming context
13. P3-2: Parallel file processing
14. P3-3: Scan caching
15. P3 features: undo, completion, templates

---

## 📊 Success Metrics

Track these after improvements:

- **Stability**: Zero race condition errors in batch mode
- **Memory**: < 200MB usage for 10+ turn sessions (currently can hit 500MB+)
- **Cost**: 30-40% reduction via context streaming + smart eviction
- **UX**: 80% of users complete tasks without manual intervention
- **Speed**: Context generation < 3 seconds for 1000 files

---

## 🚀 How to Execute This Plan

### Option 1: Automatic Loop (Recommended)
```bash
# Save this file as ARCHITECTURE_GOALS.md in repo root
codepicker agent improve --loop

# Agent will:
# 1. Read unchecked items
# 2. Execute first task
# 3. Check it off
# 4. Repeat until all done
```

### Option 2: Manual Step-by-Step
```bash
# Execute one task
codepicker agent improve

# Review changes
codepicker apply -t

# Apply if good
codepicker apply --yes

# Repeat
```

### Option 3: Batch Queue
```bash
# Queue all P0 tasks
codepicker batch add "Add mutex protection to shadow.Manager"
codepicker batch add "Fix memory leak in Engine.Run()"
codepicker batch add "Thread-safe database operations"

# Run queue
codepicker batch run --concurrent 1
```

---

## 📝 Notes for Agent

When executing these tasks:

1. **Always use scan_package first** to understand the module
2. **Search for existing patterns** before adding new code
3. **Write tests alongside fixes** (don't skip this)
4. **Check ARCHITECTURE.md** for context on components
5. **Use write_shadow_file** for all changes
6. **Verify with run_shell** when policy allows (e.g., `go test`)

Example workflow for task "Add mutex to shadow.Manager":
```
1. scan_package(target_dir="internal/shadow")
2. read_file(path="internal/shadow/fs.go")
3. search_code(query="Manifest.Changes") 
4. write_shadow_file(path="internal/shadow/fs.go", content="...with mutex...")
5. write_shadow_file(path="internal/shadow/fs_test.go", content="...concurrency test...")
6. run_shell(command="go test ./internal/shadow/...")
```

---

**Last Updated**: January 22, 2026  
**Agent Ready**: ✅ This file is formatted for `codepicker agent improve`  
**Estimated Completion**: 4-6 weeks of iterative agent work
