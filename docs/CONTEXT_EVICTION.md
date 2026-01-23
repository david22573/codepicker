# Intelligent Context Eviction with Relevance Scoring

## Overview

CodePicker implements an intelligent context eviction system that uses multi-factor relevance scoring to determine which files should remain in the agent's working memory when context limits are reached.

## Problem Statement

AI agents have limited context windows (token budgets). When the agent reads many files during a session, older or less important files must be evicted to make room for new ones. Simple strategies like "least recently used" (LRU) don't account for the inherent importance of different files.

## Solution

The intelligent eviction system calculates a **relevance score** for each file in working memory based on four key factors:

### 1. Recency (35% weight)
**How recently was the file accessed?**

- Uses exponential decay with a 10-minute half-life
- Recently accessed files (< 1 minute) receive maximum score (1.0)
- Score degrades over time, dropping ~50% every 10 minutes
- Ensures the agent keeps files it's actively working with

**Formula:**
```
score = 1.0 / (1.0 + minutes_since_access / 10.0)
```

### 2. Frequency (20% weight)
**How often has the file been accessed?**

- Tracks access count in database (`access_count` column)
- Uses logarithmic scaling to prevent old files from dominating
- Files accessed once get base score (0.1)
- Frequent access (10+ times) increases score to ~0.7
- Very frequent access (100+ times) approaches maximum (~0.9)

**Formula:**
```
score = 0.1 + 0.8 * (1.0 - 1.0/(1.0 + access_count/10.0))
```

### 3. Importance (30% weight)
**How inherently important is this file?**

Based on file type, path, and content analysis:

#### Critical Files (score = 0.9-1.0)
- `go.mod`, `package.json`, `Cargo.toml` (dependency manifests)
- `Dockerfile`, `docker-compose.yml` (container configs)

#### High Importance (score += 0.2-0.3)
- `main.go`, `main.py` (entry points)
- `schema.go`, `migration.go` (database structure)
- `types.go`, `interface.go` (type definitions)
- `config.go` (configuration files)

#### Core Packages (score += 0.15)
- Files in `internal/agent/`, `internal/database/`, `pkg/`

#### Low Priority (score -= 0.2-0.4)
- Test files (`*_test.go`)
- Generated files (containing "Code generated", "auto-generated")
- Vendor/node_modules files
- Deep nesting (>4 levels)

### 4. Relationships (15% weight)
**How connected is this file to other loaded files?**

- Scans for imports, package references, type usage
- Files referenced by many others get higher scores
- Files that reference many others get higher scores
- Uses logarithmic scaling: `0.8 * (1.0 - 1.0/(1.0 + relationships/5.0))`

## Architecture

### Core Components

```
internal/database/
├── relevance.go        # Scoring algorithms
├── relevance_test.go   # Comprehensive tests
├── schema.go          # Database schema (added access_count)
└── store.go           # Integration with memory management
```

### Key Types

```go
type RelevanceScore struct {
    Path              string
    Content           string
    TokenCount        int
    Score             float64  // Final weighted score
    RecencyScore      float64
    FrequencyScore    float64
    ImportanceScore   float64
    RelationshipScore float64
    LastAccessed      time.Time
    AccessCount       int
}
```

### Database Schema Changes

**Migration v5** adds access tracking:

```sql
ALTER TABLE memory_files ADD COLUMN access_count INTEGER DEFAULT 1;
```

## Usage

### Automatic Eviction

Eviction is **enabled by default** and happens automatically when:

1. A file is added via `UpdateWorkingMemory()`
2. Total tokens exceed `MaxContextTokens` (100,000)
3. System calculates relevance scores for all files
4. Lowest-scoring files are evicted until under budget

### Manual Inspection

View relevance scores for debugging:

```bash
codepicker context inspect
```

**Example output:**
```
=== RELEVANCE SCORES ===
File                                              | Final | Recen | Freq  | Imprt | Relat | Tokens
----------------------------------------------------------------------------------------------------
go.mod                                            | 0.891 | 0.850 | 0.320 | 1.000 | 0.150 |     45
internal/database/store.go                        | 0.782 | 0.920 | 0.650 | 0.650 | 0.450 |   1250
cmd/main.go                                       | 0.743 | 0.810 | 0.450 | 0.800 | 0.320 |    890
internal/database/store_test.go                   | 0.412 | 0.650 | 0.210 | 0.350 | 0.180 |    650
vendor/github.com/external/lib.go                 | 0.156 | 0.120 | 0.100 | 0.100 | 0.000 |    320

=== SUMMARY ===
Total Files:    5
Total Tokens:   3155 / 100000 (3.2% of max)
Score Range:    0.156 - 0.891
Average Score:  0.617
```

### Programmatic Control

```go
// Enable/disable intelligent eviction
store.EnableIntelligentEviction(true)

// Get relevance scorer for inspection
scorer := store.GetRelevanceScorer()

// Calculate scores manually
scores, err := scorer.CalculateRelevanceScores()

// Select files for eviction to meet budget
toEvict, err := scorer.SelectFilesForEviction(targetTokens)

// Get top N most relevant files
topFiles, err := scorer.GetTopFiles(10)

// Debug output
debugInfo, err := scorer.DebugScores()
```

## Implementation Details

### Eviction Flow

1. **File Added**: `store.UpdateWorkingMemory(path, content)`
2. **Hash Check**: If content unchanged, only update `last_accessed` and `access_count++`
3. **Token Check**: Calculate total tokens in memory
4. **Eviction Trigger**: If total > MaxContextTokens:
   - Call `performIntelligentEviction()`
   - Calculate relevance scores for all files
   - Sort by score (ascending)
   - Delete lowest-scoring files until under budget

### Thread Safety

All database operations use `sync.RWMutex`:
- Read operations: `s.mu.RLock()` / `s.mu.RUnlock()`
- Write operations: `s.mu.Lock()` / `s.mu.Unlock()`

Eviction is called within the write lock of `UpdateWorkingMemory()`, ensuring atomic updates.

### Performance Considerations

**Scoring Complexity**: O(n²) due to relationship analysis
- Acceptable for typical usage (< 100 files in memory)
- Relationship scoring can be optimized with caching if needed

**Database Queries**: 
- Scoring requires one `SELECT` query
- Eviction requires N `DELETE` queries (transactional)
- Consider batching deletes in future optimization

## Testing

Comprehensive test suite in `relevance_test.go`:

```bash
go test ./internal/database -v -run TestRelevance
```

**Test Coverage:**
- ✅ Recency scoring at different time intervals
- ✅ Frequency scoring with varying access counts
- ✅ Importance scoring for different file types
- ✅ Relationship scoring with interconnected files
- ✅ Final score calculation and weighting
- ✅ Sorting algorithms (ascending/descending)
- ✅ Integration test with real database
- ✅ Eviction selection with token budgets
- ✅ Top-N file selection

## Future Enhancements

### Potential Improvements

1. **User-Defined Weights**: Allow customization of scoring weights via config
2. **Semantic Analysis**: Use embedding similarity for better relationship scoring
3. **Project-Aware Scoring**: Boost files from the current working module
4. **Manual Pinning**: Allow users to pin critical files that should never be evicted
5. **Performance Optimization**: Cache relationship graph for O(n) scoring
6. **Adaptive Weights**: Learn optimal weights based on user behavior
7. **Diff-Aware Scoring**: Boost files with recent uncommitted changes

### Configuration Example (Future)

```yaml
# .codepicker.yml
context:
  eviction:
    enabled: true
    max_tokens: 100000
    scoring_weights:
      recency: 0.35
      frequency: 0.20
      importance: 0.30
      relationships: 0.15
    pinned_files:
      - go.mod
      - internal/database/schema.go
    importance_rules:
      - pattern: "cmd/.*main\\.go"
        score: 0.9
      - pattern: ".*_test\\.go"
        score: 0.3
```

## Comparison with Simple LRU

| Aspect | Simple LRU | Intelligent Scoring |
|--------|-----------|---------------------|
| **Eviction Basis** | Last access time only | Multi-factor scoring |
| **File Type Awareness** | ❌ No | ✅ Yes |
| **Access Frequency** | ❌ Ignored | ✅ Tracked |
| **Relationship Analysis** | ❌ No | ✅ Yes |
| **Preserves Critical Files** | ❌ No guarantee | ✅ Yes (go.mod, schema, etc.) |
| **Performance** | O(1) with heap | O(n²) with relationship analysis |
| **Complexity** | Simple | Moderate |

## Example Scenarios

### Scenario 1: Refactoring Session
Agent reads:
1. `internal/database/store.go` (multiple times) → High frequency
2. `internal/database/schema.go` (once) → High importance
3. `internal/database/store_test.go` (once) → Lower importance
4. `vendor/github.com/lib/sqlite/driver.go` (once) → Very low importance

**Result**: Vendor file evicted first, test file second. Core files retained.

### Scenario 2: Debugging
Agent repeatedly accesses:
1. `cmd/main.go` (10 times in 5 minutes) → High recency + frequency
2. `internal/logger/logger.go` (8 times) → High frequency
3. Various utility files (1-2 times each) → Lower scores

**Result**: Main and logger stay in memory. Utilities evicted as needed.

### Scenario 3: New Feature Development
Agent starts with:
1. `go.mod` (read at start) → Max importance, aging recency
2. `internal/agent/engine.go` (read 20 mins ago) → Low recency
3. `internal/tools/new_tool.go` (just created) → Max recency

**Result**: Despite aging, `go.mod` stays (importance). `engine.go` may be evicted (low recency). `new_tool.go` stays (high recency).

## Monitoring & Debugging

### Enable Debug Logging

```bash
codepicker agent --trace-memory "Implement feature X"
```

Shows:
```
[MEMORY] + Adding: internal/database/store.go
[MEMORY] Context: 3250 tokens
[MEMORY] 📸 Snapshotting...
[MEMORY] ⏪ Restoring...
```

### Check Current State

```bash
# View all files in memory
codepicker context list

# View with relevance scores
codepicker context inspect

# Clear memory
codepicker context clear
```

## References

- **Code**: `internal/database/relevance.go`
- **Tests**: `internal/database/relevance_test.go`
- **Schema**: `internal/database/schema.go` (Migration v5)
- **Integration**: `internal/database/store.go` (`UpdateWorkingMemory`, `GetWorkingMemory`)
- **CLI**: `cmd/context_inspect.go`

## Summary

The intelligent context eviction system ensures that CodePicker's AI agents maintain the most relevant files in their working memory, preserving critical configuration files, frequently accessed code, and interconnected modules while evicting low-priority vendor files, tests, and rarely-used utilities. This results in more coherent, context-aware agent behavior over long-running sessions.
