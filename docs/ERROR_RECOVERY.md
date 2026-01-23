# Error Recovery System

## Overview

The Codepicker error recovery system provides automatic detection and recovery from common development environment errors. When a tool execution fails, the system analyzes the error output, identifies known failure patterns, and attempts to automatically fix the issue.

## How It Works

1. **Error Detection**: When a command fails, the system captures both the error message and output
2. **Pattern Matching**: Error text is matched against a library of known failure patterns using regex
3. **Strategy Selection**: The first matching recovery strategy is selected
4. **Automated Fix**: Fix commands are executed automatically (if available)
5. **Retry**: The original command is retried after applying fixes
6. **Result Reporting**: Success or failure is reported with detailed diagnostics

## Supported Error Categories

### Go Language Errors

| Error | Pattern | Auto-Fix | Description |
|-------|---------|----------|-------------|
| MissingGoMod | `go: go.mod file not found` | ✅ Yes | Initializes a new Go module with `go mod init` |
| MissingDependencies | `cannot find package` | ✅ Yes | Runs `go mod download` and `go mod tidy` |
| BuildCacheProblem | `build cache is disabled` | ✅ Yes | Cleans the build cache with `go clean -cache` |

### Python Errors

| Error | Pattern | Auto-Fix | Description |
|-------|---------|----------|-------------|
| PythonModuleMissing | `ModuleNotFoundError: No module named 'X'` | ✅ Yes | Installs the missing module with `pip install X` |
| PythonPipMissing | `pip: command not found` | ✅ Yes | Installs pip with `python3 -m ensurepip` |
| PythonVenvNotActivated | `No module named 'venv'` | ✅ Yes | Creates a virtual environment |
| PythonSyntaxError | `SyntaxError: invalid syntax` | ❌ No | Diagnostic only - requires manual fix |

### Node.js/npm Errors

| Error | Pattern | Auto-Fix | Description |
|-------|---------|----------|-------------|
| NodeModuleMissing | `Cannot find module 'X'` | ✅ Yes | Installs the missing module with `npm install X` |
| NpmNotInstalled | `npm: command not found` | ❌ No | Diagnostic only - requires Node.js installation |
| PackageJsonMissing | `no such file or directory.*package.json` | ✅ Yes | Initializes npm project with `npm init -y` |
| NodeModulesCorrupted | `integrity check failed` | ✅ Yes | Removes and reinstalls node_modules |

### File System Permission Errors

| Error | Pattern | Auto-Fix | Description |
|-------|---------|----------|-------------|
| PermissionDenied | `permission denied` | ✅ Yes | Attempts to fix file permissions |
| ScriptNotExecutable | `Permission denied` on script | ✅ Yes | Makes the script executable with `chmod +x` |

### Git Errors

| Error | Pattern | Auto-Fix | Description |
|-------|---------|----------|-------------|
| GitMergeConflict | `CONFLICT (content): Merge conflict` | ❌ No | Shows status - requires manual resolution |
| GitNotInitialized | `not a git repository` | ✅ Yes | Initializes git repository with `git init` |
| GitUnstagedChanges | `Your local changes would be overwritten` | ✅ Yes | Stashes changes with `git stash` |

### Docker Errors

| Error | Pattern | Auto-Fix | Description |
|-------|---------|----------|-------------|
| DockerDaemonNotRunning | `Cannot connect to the Docker daemon` | ❌ No | Diagnostic only - requires manual daemon start |
| DockerImageNotFound | `Unable to find image 'X' locally` | ✅ Yes | Pulls the image with `docker pull X` |

### Network Errors

| Error | Pattern | Auto-Fix | Description |
|-------|---------|----------|-------------|
| NetworkTimeout | `i/o timeout` | ✅ Yes | Automatic retry with backoff |
| DNSResolutionFailed | `no such host` | ❌ No | Diagnostic only - check network connectivity |

### Database Errors

| Error | Pattern | Auto-Fix | Description |
|-------|---------|----------|-------------|
| DatabaseLocked | `database is locked` | ✅ Yes | Retries operation up to 3 times |

### Build Tool Errors

| Error | Pattern | Auto-Fix | Description |
|-------|---------|----------|-------------|
| MakefileNotFound | `No targets specified and no makefile found` | ❌ No | Diagnostic only |
| MissingBuildTool | `gcc: command not found` | ❌ No | Diagnostic only - requires tool installation |

## Usage Examples

### Automatic Recovery

The recovery system is automatically invoked when using the agent's `ExecuteWithRecovery` method:

```go
result := engine.ExecuteWithRecovery("python", []string{"script.py"}, 3)

if result.Success {
    fmt.Println("Command succeeded after recovery")
} else if result.Attempted {
    fmt.Printf("Recovery attempted using %s but failed\n", result.StrategyUsed)
} else {
    fmt.Println("Command failed with no matching recovery strategy")
}
```

### Testing Error Patterns

You can test if an error matches any recovery pattern:

```go
errorText := "ModuleNotFoundError: No module named 'requests'"
matches := agent.TestPattern(errorText)
// Returns: ["PythonModuleMissing"]
```

### Finding a Specific Strategy

```go
strategy := agent.FindStrategy("PythonModuleMissing")
if strategy != nil {
    fmt.Printf("Diagnosis: %s\n", strategy.Diagnosis)
    fmt.Printf("Max Retries: %d\n", strategy.MaxRetries)
}
```

### Getting All Strategies

```go
strategies := agent.GetRecoveryStrategies()
for _, strategy := range strategies {
    fmt.Printf("- %s: %s\n", strategy.Name, strategy.Diagnosis)
}
```

## Configuration

Recovery behavior can be controlled through the Engine's configuration:

- **MaxRetries**: Number of retry attempts after applying fixes (default: varies by strategy)
- **Trace Mode**: Enable detailed logging of recovery attempts (set `Debug.Tools = true`)

## Adding New Recovery Strategies

To add a new recovery strategy, edit `internal/agent/recovery.go` and add an entry to `CommonFailures`:

```go
{
    Name:      "YourErrorName",
    Pattern:   regexp.MustCompile(`your error pattern here`),
    Diagnosis: "Human-readable explanation of the error",
    FixCommands: []CommandSequence{
        {Binary: "command", Args: []string{"arg1", "arg2"}},
    },
    MaxRetries: 1,
}
```

### Pattern Capture Groups

You can use regex capture groups to extract dynamic values from errors:

```go
{
    Name:      "PythonModuleMissing",
    Pattern:   regexp.MustCompile(`ModuleNotFoundError: No module named '(\w+)'`),
    Diagnosis: "Python module not installed",
    FixCommands: []CommandSequence{
        {Binary: "pip", Args: []string{"install", "$1"}}, // $1 is replaced with captured group
    },
    MaxRetries: 1,
}
```

### Special Placeholders

- `$1`, `$2`, etc.: Replaced with regex capture groups
- `$FILE`: Replaced with the first capture group (useful for file-related errors)

## Diagnostic-Only Strategies

Some errors cannot be automatically fixed (e.g., syntax errors, missing system dependencies). These strategies have empty `FixCommands` and provide helpful diagnostic messages instead:

```go
{
    Name:      "PythonSyntaxError",
    Pattern:   regexp.MustCompile(`SyntaxError: invalid syntax`),
    Diagnosis: "Python syntax error detected - please fix code manually",
    FixCommands: []CommandSequence{},
    MaxRetries: 0,
}
```

## Best Practices

1. **Specific Patterns First**: More specific error patterns should come before generic ones
2. **Safe Operations**: Only include fixes that are safe to run automatically
3. **Limited Retries**: Set reasonable `MaxRetries` values to avoid infinite loops
4. **Clear Diagnostics**: Write clear, actionable diagnosis messages
5. **Test Coverage**: Add test cases for new patterns in `recovery_test.go`

## Limitations

- Recovery strategies match on the first pattern found - order matters
- Some system-level issues (missing compilers, daemon not running) cannot be auto-fixed
- Recovery assumes commands are safe to retry - destructive operations are avoided
- Capture group substitution is simple text replacement - complex parsing not supported

## Security Considerations

- All fix commands go through the Sentinel security layer
- Commands are subject to the same policy enforcement as manual tool calls
- Dangerous patterns (like piped shell scripts) are blocked by Sentinel
- Recovery never executes arbitrary code from error messages

## Performance

- Pattern matching is fast (regex-based, O(n) on error text length)
- Recovery adds minimal overhead when no errors occur
- Failed recovery attempts are logged for debugging
- Benchmark tests ensure pattern matching stays efficient

## Future Enhancements

Potential improvements tracked in ARCHITECTURE_GOALS.md:

- Language-specific plugin system for custom recovery strategies
- Machine learning-based error classification
- Contextual recovery (use project metadata to inform fixes)
- Recovery strategy recommendations based on project type
- Automatic strategy generation from error logs
