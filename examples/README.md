# CodePicker Examples

This directory contains example programs demonstrating various CodePicker features.

## Available Examples

### `context_eviction_demo.go`

Demonstrates the intelligent context eviction system with relevance scoring.

**What it does:**
- Creates a temporary database
- Simulates an AI agent session
- Adds files with different characteristics (critical configs, tests, vendor, etc.)
- Shows how files are scored based on recency, frequency, importance, and relationships
- Simulates repeated access to demonstrate frequency tracking
- Adds many large files to trigger eviction
- Shows which files survived and which were evicted

**Run it:**
```bash
cd examples
go run context_eviction_demo.go
```

**Expected output:**
```
=== CodePicker Context Eviction Demo ===

Phase 1: Initial file loading
--------------------------------
  Adding: go.mod                                   (Critical config file)
  Adding: internal/database/store.go               (Core implementation)
  Adding: internal/database/store_test.go          (Test file (lower priority))
  Adding: vendor/github.com/external/lib.go        (Vendor file (lowest priority))
  Adding: cmd/main.go                              (Entry point (high importance))

Phase 2: Initial relevance scores
--------------------------------

File                                              | Final  | Recency | Freq   | Import | Relat  | Access
----------------------------------------------------------------------------------------------------
go.mod                                            | 0.8910 | 0.8500 | 0.3200 | 1.0000 | 0.1500 |       1
internal/database/store.go                        | 0.7820 | 0.9200 | 0.6500 | 0.6500 | 0.4500 |       1
cmd/main.go                                       | 0.7430 | 0.8100 | 0.4500 | 0.8000 | 0.3200 |       1
internal/database/store_test.go                   | 0.4120 | 0.6500 | 0.2100 | 0.3500 | 0.1800 |       1
vendor/github.com/external/lib.go                 | 0.1560 | 0.1200 | 0.1000 | 0.1000 | 0.0000 |       1

...

✅ Files that survived eviction:
  • go.mod                                (score: 0.891)
  • internal/database/store.go            (score: 0.842)
  • cmd/main.go                           (score: 0.793)

❌ Files that were evicted:
  • internal/database/store_test.go
  • vendor/github.com/external/lib.go
```

**Key Observations:**
1. Critical files (go.mod) have maximum importance and survive
2. Frequently accessed files (store.go, main.go) survive due to high frequency
3. Vendor files and tests are first to be evicted (low importance)
4. Recent additions with low importance are evicted over critical old files

## Adding New Examples

To add a new example:

1. Create a new `.go` file in this directory
2. Use `package main` with a `func main()`
3. Import CodePicker packages as needed
4. Add documentation comments
5. Update this README with:
   - What the example demonstrates
   - How to run it
   - What output to expect

## Dependencies

All examples use the CodePicker module:

```go
import (
    "github.com/david22573/codepicker/internal/database"
    "github.com/david22573/codepicker/internal/agent"
    // ... etc
)
```

Make sure to run from the examples directory or adjust your go.mod accordingly.

## Tips

- Examples create temporary files/databases - they clean up automatically
- Use examples as learning tools to understand CodePicker internals
- Copy and modify examples for your own testing
- Run with `-v` for verbose output: `go run -v context_eviction_demo.go`
