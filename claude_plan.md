# Comprehensive Improvement Plan for Codepicker

## Executive Summary

After analyzing the codebase, I've identified several categories of improvements ranging from critical bug fixes to architectural enhancements. The implicit goals of this project are:

1. **Context Generation** - Efficiently scan and minify codebases for AI consumption
2. **AI Integration** - Provide multiple interaction modes (ask, chat, agent)
3. **Autonomous Agent** - Enable AI to read/write files and execute commands safely
4. **Security** - Prevent path traversal, command injection, and resource exhaustion

---

## 🚨 Critical Fixes (Priority 1)

<details>
<summary><strong>1.1 Dead Code & Unused Functions in serve.go</strong></summary>

**Issue:** `handleAsk` and `enableCORS` in `cmd/serve.go` are defined but never registered or used.

**File:** `cmd/serve.go` (lines 47-180)

**Fix:**
```go
// Remove these functions entirely OR wire them up properly
// Option A: Delete handleAsk and enableCORS (recommended - they're duplicates)
// Option B: Register them if needed:

func init() {
    rootCmd.AddCommand(serveCmd)
    serveCmd.Flags().StringVarP(&port, "port", "p", "22573", "Port to listen on")
}

// The server.go already has proper handlers, so delete the redundant code
```
</details>

<details>
<summary><strong>1.2 Race Condition in Approval System</strong></summary>

**Issue:** The approval callback system in `handlers.go` can deadlock or leak channels.

**File:** `internal/server/handlers.go`

**Fix:**
```go
func (s *AgentServer) handleAgentTask(w http.ResponseWriter, r *http.Request) {
    // ... existing code ...

    // Add timeout to approval callback
    s.Engine.ApprovalCallback = func(cmdStr, reason string) bool {
        ch := make(chan bool, 1) // Buffered channel to prevent leak
        
        s.approvalLock.Lock()
        s.approvalMap[reqID] = ch
        s.approvalLock.Unlock()
        
        defer func() {
            s.approvalLock.Lock()
            delete(s.approvalMap, reqID)
            s.approvalLock.Unlock()
        }()

        jsonMsg, _ := json.Marshal(map[string]interface{}{
            "type":    "approval_req",
            "id":      reqID,
            "command": cmdStr,
            "reason":  reason,
        })
        
        select {
        case eventStream <- string(jsonMsg):
        case <-r.Context().Done():
            return false
        }

        select {
        case approved := <-ch:
            return approved
        case <-r.Context().Done():
            return false
        case <-time.After(60 * time.Second):
            return false
        }
    }
    // ... rest of handler ...
}
```
</details>

<details>
<summary><strong>1.3 Memory Leak in Rate Limiter</strong></summary>

**Issue:** `RateLimiter.limiters` map grows indefinitely without cleanup.

**File:** `internal/server/ratelimit.go`

**Fix:**
```go
type RateLimiter struct {
    limiters  map[string]*rateLimiterEntry
    mu        sync.RWMutex
    rate      rate.Limit
    burst     int
    cleanupInterval time.Duration
}

type rateLimiterEntry struct {
    limiter  *rate.Limiter
    lastSeen time.Time
}

func NewRateLimiter(r rate.Limit, b int) *RateLimiter {
    rl := &RateLimiter{
        limiters:        make(map[string]*rateLimiterEntry),
        rate:            r,
        burst:           b,
        cleanupInterval: 10 * time.Minute,
    }
    go rl.cleanup()
    return rl
}

func (rl *RateLimiter) cleanup() {
    ticker := time.NewTicker(rl.cleanupInterval)
    defer ticker.Stop()
    
    for range ticker.C {
        rl.mu.Lock()
        threshold := time.Now().Add(-rl.cleanupInterval)
        for ip, entry := range rl.limiters {
            if entry.lastSeen.Before(threshold) {
                delete(rl.limiters, ip)
            }
        }
        rl.mu.Unlock()
    }
}

func (rl *RateLimiter) getLimiter(ip string) *rate.Limiter {
    rl.mu.Lock()
    defer rl.mu.Unlock()

    entry, exists := rl.limiters[ip]
    if !exists {
        entry = &rateLimiterEntry{
            limiter:  rate.NewLimiter(rl.rate, rl.burst),
            lastSeen: time.Now(),
        }
        rl.limiters[ip] = entry
    } else {
        entry.lastSeen = time.Now()
    }
    return entry.limiter
}
```
</details>

---

## 🔧 Architectural Improvements (Priority 2)

### 2.1 Unused Configuration Values

**Issue:** The `AIConfig` from `configfile.go` is loaded but never applied.

**Files to modify:**

```go
// internal/config/configfile.go - Add getter
func (c *ConfigFile) GetModel() string {
    if c.AI.Model != "" {
        return c.AI.Model
    }
    return constants.DefaultModel
}

// cmd/ask.go - Apply config
func init() {
    // ... existing code ...
}

// In RunE function of askCmd:
if cfgFile != nil && !cmd.Flags().Changed("model") {
    askModel = cfgFile.GetModel()
}
```

### 2.2 Create Interface Abstractions for Testability

**New file:** `internal/ai/client.go`

```go
package ai

import (
    "context"
    "github.com/david22573/codepicker/pkg/openrouter"
)

// AIClient abstracts the OpenRouter client for testing
type AIClient interface {
    CreateChatCompletion(ctx context.Context, req openrouter.ChatCompletionRequest) (*openrouter.ChatCompletionResponse, error)
    CreateChatCompletionStream(ctx context.Context, req openrouter.ChatCompletionRequest) (*openrouter.ChatCompletionStream, error)
    GetModelInfo(ctx context.Context, modelID string) (*openrouter.Model, error)
}

// Verify openrouter.Client implements AIClient
var _ AIClient = (*openrouter.Client)(nil)
```

### 2.3 Consolidate Global State

**Current Problem:** Global variables scattered across `cmd/*.go`

**Solution:** Create a unified App context:

```go
// internal/app/context.go
package app

import (
    "github.com/david22573/codepicker/internal/config"
    "github.com/david22573/codepicker/internal/logger"
)

type Context struct {
    Logger  logger.Logger
    Config  *config.Config
    Limits  *config.Limits
    
    // Runtime options (set via flags)
    SrcDir      string
    OutPath     string
    Minify      bool
    ShowTokens  bool
    IncludeExts []string
    IgnoreDirs  []string
    Verbose     bool
}

func NewContext() *Context {
    return &Context{
        Limits: config.DefaultLimits(),
        Config: config.NewConfig(),
    }
}
```

---

## 🛡️ Security Enhancements (Priority 2)

### 3.1 Enhanced Command Sentinel

**File:** `internal/agent/sentinel.go`

```go
// Add dangerous patterns
var dangerousPatterns = []string{
    "curl.*\\|.*sh",     // Piped curl to shell
    "wget.*\\|.*sh",
    "eval",
    "base64.*-d",
    "> /dev/",
    "dd if=",
    "mkfs",
    ":(){ :|:& };:",    // Fork bomb
}

func (s *Sentinel) CheckCommand(cmdStr string) (bool, string, string, []string) {
    // Check dangerous patterns first
    for _, pattern := range dangerousPatterns {
        if matched, _ := regexp.MatchString(pattern, cmdStr); matched {
            return true, "Potentially dangerous command pattern detected", "", nil
        }
    }
    
    // ... existing logic ...
}
```

### 3.2 Add Basic Authentication for Server

**New file:** `internal/server/auth.go`

```go
package server

import (
    "crypto/subtle"
    "net/http"
    "os"
)

func BasicAuth() Middleware {
    return func(next http.HandlerFunc) http.HandlerFunc {
        return func(w http.ResponseWriter, r *http.Request) {
            expectedToken := os.Getenv("CODEPICKER_API_TOKEN")
            if expectedToken == "" {
                // No auth configured, allow through
                next(w, r)
                return
            }
            
            token := r.Header.Get("Authorization")
            if token == "" || subtle.ConstantTimeCompare(
                []byte(token), 
                []byte("Bearer "+expectedToken),
            ) != 1 {
                http.Error(w, "Unauthorized", http.StatusUnauthorized)
                return
            }
            
            next(w, r)
        }
    }
}
```

---

## ✨ Feature Additions (Priority 3)

### 4.1 Add Version Command

**New file:** `cmd/version.go`

```go
package cmd

import (
    "fmt"
    "runtime"
    
    "github.com/spf13/cobra"
)

var (
    Version   = "dev"
    GitCommit = "unknown"
    BuildDate = "unknown"
)

var versionCmd = &cobra.Command{
    Use:   "version",
    Short: "Print version information",
    Run: func(cmd *cobra.Command, args []string) {
        fmt.Printf("codepicker %s\n", Version)
        fmt.Printf("  Commit:   %s\n", GitCommit)
        fmt.Printf("  Built:    %s\n", BuildDate)
        fmt.Printf("  Go:       %s\n", runtime.Version())
        fmt.Printf("  OS/Arch:  %s/%s\n", runtime.GOOS, runtime.GOARCH)
    },
}

func init() {
    rootCmd.AddCommand(versionCmd)
}
```

**Update Makefile:**
```makefile
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
DATE ?= $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")

LDFLAGS = -ldflags "-X github.com/david22573/codepicker/cmd.Version=$(VERSION) \
    -X github.com/david22573/codepicker/cmd.GitCommit=$(COMMIT) \
    -X github.com/david22573/codepicker/cmd.BuildDate=$(DATE)"

build:
	go build $(LDFLAGS) -o $(BINARY_NAME) main.go
```

### 4.2 Progress Indicators for Long Operations

**New file:** `internal/progress/spinner.go`

```go
package progress

import (
    "fmt"
    "sync"
    "time"
)

type Spinner struct {
    message string
    done    chan struct{}
    wg      sync.WaitGroup
}

var spinnerChars = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

func NewSpinner(message string) *Spinner {
    return &Spinner{
        message: message,
        done:    make(chan struct{}),
    }
}

func (s *Spinner) Start() {
    s.wg.Add(1)
    go func() {
        defer s.wg.Done()
        i := 0
        for {
            select {
            case <-s.done:
                fmt.Print("\r\033[K") // Clear line
                return
            default:
                fmt.Printf("\r%s %s", spinnerChars[i%len(spinnerChars)], s.message)
                i++
                time.Sleep(100 * time.Millisecond)
            }
        }
    }()
}

func (s *Spinner) Stop() {
    close(s.done)
    s.wg.Wait()
}
```

### 4.3 Shadow Filesystem Cleanup

**File:** `internal/shadow/fs.go`

```go
// Add cleanup method
func (m *Manager) Cleanup() error {
    return os.RemoveAll(m.ShadowRoot)
}

// Add list method for shadow files
func (m *Manager) ListShadowFiles() ([]string, error) {
    var files []string
    err := filepath.WalkDir(m.ShadowRoot, func(path string, d os.DirEntry, err error) error {
        if err != nil || d.IsDir() {
            return err
        }
        rel, _ := filepath.Rel(m.ShadowRoot, path)
        files = append(files, rel)
        return nil
    })
    return files, err
}

// Add diff preview
func (m *Manager) PreviewDiff(relPath string) (string, error) {
    shadowPath := filepath.Join(m.ShadowRoot, relPath)
    realPath := filepath.Join(m.SrcRoot, relPath)
    
    shadowContent, err := os.ReadFile(shadowPath)
    if err != nil {
        return "", err
    }
    
    realContent, err := os.ReadFile(realPath)
    if os.IsNotExist(err) {
        return fmt.Sprintf("+++ NEW FILE: %s\n%s", relPath, string(shadowContent)), nil
    } else if err != nil {
        return "", err
    }
    
    // Use a diff library here or simple comparison
    if string(shadowContent) == string(realContent) {
        return "No changes", nil
    }
    
    return fmt.Sprintf("--- %s (original)\n+++ %s (shadow)\n[diff content here]", relPath, relPath), nil
}
```

---

## 📊 Testing Improvements (Priority 3)

### 5.1 Add Missing Tests

**New file:** `internal/agent/engine_test.go`

```go
package agent

import (
    "context"
    "testing"
    
    "github.com/david22573/codepicker/internal/config"
    "github.com/david22573/codepicker/internal/logger"
)

type MockClient struct {
    responses []string
    callCount int
}

// Implement mock methods...

func TestEngineBasicTask(t *testing.T) {
    tmpDir := t.TempDir()
    log := &logger.NoOpLogger{}
    limits := config.DefaultLimits()
    
    // Create mock client
    mockClient := &MockClient{
        responses: []string{`{"choices":[{"message":{"content":"Done"}}]}`},
    }
    
    eng, err := NewEngine(nil, "test-model", tmpDir, log, limits)
    if err != nil {
        t.Fatalf("Failed to create engine: %v", err)
    }
    
    // Test basic execution
    result, err := eng.Run(context.Background(), "test task", nil)
    if err != nil {
        t.Errorf("Unexpected error: %v", err)
    }
    
    if result == "" {
        t.Error("Expected non-empty result")
    }
}
```

**New file:** `internal/contextgen/generator_test.go`

```go
package contextgen

import (
    "context"
    "os"
    "path/filepath"
    "strings"
    "testing"
    
    "github.com/david22573/codepicker/internal/logger"
)

func TestGenerate(t *testing.T) {
    tmpDir := t.TempDir()
    
    // Create test files
    os.WriteFile(filepath.Join(tmpDir, "main.go"), []byte("package main"), 0644)
    os.WriteFile(filepath.Join(tmpDir, "utils.go"), []byte("package main"), 0644)
    
    opts := Options{
        SrcDir: tmpDir,
        Minify: false,
    }
    
    result, err := Generate(context.Background(), opts, &logger.NoOpLogger{})
    if err != nil {
        t.Fatalf("Generate failed: %v", err)
    }
    
    if !strings.Contains(result, "main.go") {
        t.Error("Expected result to contain main.go")
    }
    
    if !strings.Contains(result, "utils.go") {
        t.Error("Expected result to contain utils.go")
    }
}

func TestGenerateFocusMode(t *testing.T) {
    tmpDir := t.TempDir()
    
    mainFile := filepath.Join(tmpDir, "main.go")
    os.WriteFile(mainFile, []byte("package main\nfunc main() {}"), 0644)
    os.WriteFile(filepath.Join(tmpDir, "other.go"), []byte("package other"), 0644)
    
    opts := Options{
        SrcDir:     tmpDir,
        FocusFiles: []string{mainFile},
        Minify:     false,
    }
    
    result, err := Generate(context.Background(), opts, &logger.NoOpLogger{})
    if err != nil {
        t.Fatalf("Generate failed: %v", err)
    }
    
    if !strings.Contains(result, "main.go") {
        t.Error("Expected result to contain main.go")
    }
    
    if strings.Contains(result, "other.go") {
        t.Error("Expected result to NOT contain other.go in focus mode")
    }
}
```

---

## 📝 Documentation (Priority 3)

### 6.1 Create README.md

```markdown
# Codepicker

A CLI tool for harvesting codebase context for AI consumption.

## Installation

```bash
go install github.com/david22573/codepicker@latest
```

## Quick Start

```bash
# Initialize configuration
codepicker init

# Generate context for current directory
codepicker

# Ask a question about your codebase
export OPENROUTER_API_KEY=your_key
codepicker ask "How does the authentication work?"

# Start interactive chat
codepicker chat

# Use autonomous agent mode
codepicker do "Add error handling to the API endpoint"
```

## Configuration

Create `.codepicker.yml` in your project root:

```yaml
src: .
output: context.md
minify: true
tokens: false

include:
  - .go
  - .ts
  - .js

exclude:
  - .git
  - node_modules
  - vendor

ai:
  model: anthropic/claude-3-sonnet
  temperature: 0.7
```

## Commands

| Command | Description |
|---------|-------------|
| `codepicker` | Generate context file |
| `codepicker ask [query]` | Ask AI about your codebase |
| `codepicker chat` | Interactive chat session |
| `codepicker do [task]` | Autonomous agent mode |
| `codepicker tree` | Print project structure |
| `codepicker copy` | Copy files preserving structure |
| `codepicker serve` | Start agent daemon |
| `codepicker init` | Generate default config |

## Flags

- `-s, --src` - Source directory (default: `.`)
- `-o, --out` - Output file path
- `-m, --minify` - Enable minification (default: `true`)
- `-i, --include` - Comma-separated extensions to include
- `-e, --exclude` - Comma-separated directories to exclude
- `-v, --verbose` - Enable verbose logging

## License

MIT
```

---

## 🗓️ Implementation Roadmap

| Phase | Items | Effort |
|-------|-------|--------|
| **Week 1** | Critical fixes (1.1-1.3) | 2 days |
| **Week 1** | Testing improvements (5.1) | 2 days |
| **Week 2** | Architecture improvements (2.1-2.3) | 3 days |
| **Week 2** | Security enhancements (3.1-3.2) | 2 days |
| **Week 3** | Feature additions (4.1-4.3) | 3 days |
| **Week 3** | Documentation (6.1) | 1 day |

---

## 📋 Checklist Summary

### Must Fix (Before Release)
- [ ] Remove dead code in `serve.go`
- [ ] Fix race condition in approval system
- [ ] Fix memory leak in rate limiter
- [ ] Apply AI config values from YAML

### Should Fix (Quality)
- [ ] Add interface abstractions
- [ ] Consolidate global state
- [ ] Add comprehensive tests
- [ ] Add README documentation

### Nice to Have
- [ ] Version command with build info
- [ ] Progress spinners
- [ ] Shadow filesystem cleanup/diff
- [ ] Server authentication
- [ ] Enhanced command validation
