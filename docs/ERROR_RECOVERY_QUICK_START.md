# Error Recovery Quick Start Guide

## TL;DR

The error recovery system automatically fixes common development errors. It now supports **27 error patterns** across Go, Python, Node.js, Git, Docker, and more.

---

## Quick Examples

### 1. Python Module Missing

**Error**: `ModuleNotFoundError: No module named 'requests'`

**Auto-Fix**: `pip install requests`

**Result**: ✅ Module installed and command retried

---

### 2. npm Module Missing

**Error**: `Cannot find module 'express'`

**Auto-Fix**: `npm install express`

**Result**: ✅ Package installed and command retried

---

### 3. Permission Denied

**Error**: `bash: ./script.sh: Permission denied`

**Auto-Fix**: `chmod +x script.sh`

**Result**: ✅ Script made executable and retried

---

### 4. Go Dependencies Missing

**Error**: `cannot find package "github.com/pkg/errors"`

**Auto-Fix**: 
```bash
go mod download
go mod tidy
```

**Result**: ✅ Dependencies resolved and build retried

---

## For Developers

### Using Recovery in Code

```go
// Method 1: Use Engine's ExecuteWithRecovery
result := engine.ExecuteWithRecovery("python3", []string{"script.py"}, 3)

if result.Success {
    fmt.Println("Command succeeded!")
} else if result.Attempted {
    fmt.Printf("Tried to fix with: %s\n", result.StrategyUsed)
    fmt.Printf("Actions taken: %v\n", result.ActionsTaken)
}
```

### Testing Errors

```go
// Check what strategies match an error
errorText := "ModuleNotFoundError: No module named 'numpy'"
matches := agent.TestPattern(errorText)
// Returns: ["PythonModuleMissing"]
```

### Adding New Strategies

Edit `internal/agent/recovery.go`:

```go
{
    Name:      "YourNewError",
    Pattern:   regexp.MustCompile(`your error pattern`),
    Diagnosis: "What the error means",
    FixCommands: []CommandSequence{
        {Binary: "command", Args: []string{"arg1", "$1"}},
    },
    MaxRetries: 1,
}
```

**Capture Groups**: Use `$1`, `$2` in Args to substitute regex captures

---

## Supported Languages

| Language | Errors Covered | Auto-Fix |
|----------|----------------|----------|
| Go | 3 | ✅ Yes |
| Python | 4 | ✅ Yes (3/4) |
| Node.js | 5 | ✅ Yes (4/5) |
| Git | 3 | ⚠️ Partial |
| Docker | 2 | ⚠️ Partial |
| Shell | 2 | ✅ Yes |
| Database | 1 | ✅ Yes |

---

## When Recovery Doesn't Work

Some errors **cannot** be auto-fixed:

- Syntax errors (Python, Go, etc.)
- Missing system tools (npm, gcc)
- Docker daemon not running
- Git merge conflicts
- DNS/network failures

For these, the system provides **diagnostic messages** to help you fix them manually.

---

## Testing

```bash
# Run all tests
go test -v ./internal/agent/

# Run specific test
go test -run TestRecoveryPatternMatching ./internal/agent/

# Run benchmarks
go test -bench=. ./internal/agent/
```

---

## Performance

- Pattern matching: **~500 ns** per error
- Typical recovery time: **2-5 seconds** (network/install)
- Zero overhead when no errors occur

---

## Security

All recovery commands go through **Sentinel** security checks:

✅ Policy enforcement  
✅ Command whitelisting  
✅ No arbitrary code execution  
✅ Output size limits  
✅ Timeout protection

---

## Common Questions

**Q: Will it install packages without asking?**  
A: Yes, in batch mode. The system only installs packages needed to fix errors.

**Q: Can I disable recovery?**  
A: Don't use `ExecuteWithRecovery()` - use `Sentinel.Execute()` directly.

**Q: What if recovery fails?**  
A: The original error is returned with diagnostic info.

**Q: Can I see what it's doing?**  
A: Enable debug mode: `DebugConfig{Tools: true}`

**Q: How do I add recovery for my custom errors?**  
A: Add a pattern to `CommonFailures` in `internal/agent/recovery.go`

---

## Full Documentation

See `docs/ERROR_RECOVERY.md` for complete details.

---

## Examples

See `examples/error_recovery_demo.go` for working code examples.
