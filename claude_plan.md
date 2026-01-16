# Autonomous Agent Enhancement Plan for Codepicker

## Executive Summary

This document outlines a structured implementation plan to enhance the codepicker autonomous agent from its current 55% autonomy level to approximately 80-85% autonomy. The plan focuses on four core capabilities: error recovery, persistent memory, self-reflection, and validation layers.

## Current State Assessment

### Strengths
- Production-grade infrastructure with proper middleware, rate limiting, and error handling
- Robust path validation and security mechanisms preventing directory traversal and system file access
- Multi-threaded concurrent scanning with proper synchronization
- AI-powered smart file selection using LLM planning
- Shadow filesystem for safe code generation
- Cost tracking and rate limiting to prevent API abuse

### Critical Gaps
1. No automated error recovery when tool execution fails
2. Session-only memory with no persistence across restarts
3. No post-task analysis or learning from experience
4. Shadow filesystem writes without syntax validation or risk assessment
5. Planning system cannot self-correct when initial file selection is poor

## Implementation Plan

### Phase 1: Error Recovery System (Priority: Critical, Duration: 2-3 days)

#### Objective
Enable the agent to automatically diagnose and recover from common failure scenarios rather than simply reporting errors to the user.

#### Implementation Details

**1.1 Create Recovery Strategy Framework**

Create new file: `internal/agent/recovery.go`

```go
package agent

import (
    "fmt"
    "regexp"
    "strings"
)

type RecoveryStrategy struct {
    Name        string
    Pattern     *regexp.Regexp
    Diagnosis   string
    FixCommands []CommandSequence
    MaxRetries  int
}

type CommandSequence struct {
    Binary string
    Args   []string
}

type RecoveryResult struct {
    Attempted      bool
    Success        bool
    StrategyUsed   string
    ActionsToken   []string
    FinalOutput    string
    FinalError     error
}
```

**1.2 Define Common Recovery Patterns**

```go
var CommonFailures = []RecoveryStrategy{
    {
        Name:    "MissingGoMod",
        Pattern: regexp.MustCompile(`go: go\.mod file not found`),
        Diagnosis: "Missing go.mod file - module not initialized",
        FixCommands: []CommandSequence{
            {Binary: "go", Args: []string{"mod", "init", "temp-module"}},
        },
        MaxRetries: 1,
    },
    {
        Name:    "MissingDependencies",
        Pattern: regexp.MustCompile(`cannot find package|missing go\.sum entry`),
        Diagnosis: "Missing dependencies - running go mod tidy",
        FixCommands: []CommandSequence{
            {Binary: "go", Args: []string{"mod", "download"}},
            {Binary: "go", Args: []string{"mod", "tidy"}},
        },
        MaxRetries: 1,
    },
    {
        Name:    "BuildCacheProblem",
        Pattern: regexp.MustCompile(`build cache is disabled|cache verification failed`),
        Diagnosis: "Build cache issue - cleaning and retrying",
        FixCommands: []CommandSequence{
            {Binary: "go", Args: []string{"clean", "-cache"}},
        },
        MaxRetries: 1,
    },
    {
        Name:    "PermissionDenied",
        Pattern: regexp.MustCompile(`permission denied`),
        Diagnosis: "Permission denied - this may require manual intervention",
        FixCommands: []CommandSequence{},
        MaxRetries: 0,
    },
}
```

**1.3 Implement Recovery Execution Logic**

```go
func (e *Engine) ExecuteWithRecovery(binary string, args []string, maxAttempts int) RecoveryResult {
    result := RecoveryResult{
        Attempted: false,
        Success:   false,
    }
    
    // Initial execution attempt
    output, err := e.Sentinel.Execute(binary, args)
    result.FinalOutput = output
    result.FinalError = err
    
    if err == nil {
        result.Success = true
        return result
    }
    
    // Attempt recovery
    for _, strategy := range CommonFailures {
        if !strategy.Pattern.MatchString(output) {
            continue
        }
        
        result.Attempted = true
        result.StrategyUsed = strategy.Name
        
        e.Logger.Info(fmt.Sprintf("Auto-recovery triggered: %s", strategy.Diagnosis))
        
        // Execute fix commands
        for _, fixCmd := range strategy.FixCommands {
            result.ActionsTaken = append(result.ActionsTaken, 
                fmt.Sprintf("%s %s", fixCmd.Binary, strings.Join(fixCmd.Args, " ")))
            
            fixOutput, fixErr := e.Sentinel.Execute(fixCmd.Binary, fixCmd.Args)
            if fixErr != nil {
                e.Logger.Warn(fmt.Sprintf("Recovery step failed: %v", fixErr))
                e.Logger.Debug(fmt.Sprintf("Output: %s", fixOutput))
            }
        }
        
        // Retry original command
        if len(strategy.FixCommands) > 0 {
            output, err = e.Sentinel.Execute(binary, args)
            result.FinalOutput = output
            result.FinalError = err
            
            if err == nil {
                result.Success = true
                e.Logger.Info(fmt.Sprintf("Recovery successful using strategy: %s", strategy.Name))
                return result
            }
        }
        
        break // Only try one strategy per failure
    }
    
    return result
}
```

**1.4 Integrate Recovery into Agent Engine**

Modify `internal/agent/engine.go` in the `run_shell` case:

```go
case "run_shell":
    var args struct {
        Command string `json:"command"`
    }
    if err := json.Unmarshal([]byte(tool.Function.Arguments), &args); err != nil {
        e.Logger.Error(fmt.Sprintf("Failed to parse run_shell args: %v", err))
        resultStr = fmt.Sprintf("Error parsing arguments: %v", err)
        break
    }

    needsApproval, reason, binary, cmdArgs := e.Sentinel.CheckCommand(args.Command)
    if needsApproval {
        if !e.ApprovalCallback(args.Command, reason) {
            resultStr = "Command denied by user."
            break
        }
    }

    // Use recovery-enabled execution
    recoveryResult := e.ExecuteWithRecovery(binary, cmdArgs, 2)
    
    if recoveryResult.Success {
        resultStr = recoveryResult.FinalOutput
        if recoveryResult.Attempted {
            resultStr = fmt.Sprintf("[Auto-recovered using %s]\n%s", 
                recoveryResult.StrategyUsed, recoveryResult.FinalOutput)
        }
    } else {
        resultStr = fmt.Sprintf("Command failed: %v\nOutput:\n%s", 
            recoveryResult.FinalError, recoveryResult.FinalOutput)
        
        if recoveryResult.Attempted {
            resultStr += fmt.Sprintf("\n\nRecovery attempted (%s) but unsuccessful.\nActions taken: %v",
                recoveryResult.StrategyUsed, recoveryResult.ActionsTaken)
        }
    }
```

**1.5 Testing Plan**
- Test go mod initialization recovery
- Test dependency resolution recovery
- Test permission denied handling
- Verify recovery doesn't create infinite loops
- Ensure proper logging of all recovery attempts

---

### Phase 2: Persistent Memory System (Priority: High, Duration: 5-7 days)

#### Objective
Enable the agent to remember insights, patterns, and learnings across sessions, building institutional knowledge about the codebase over time.

#### Implementation Details

**2.1 Define Memory Schema**

Create new file: `internal/agent/persistence.go`

```go
package agent

import (
    "encoding/json"
    "os"
    "path/filepath"
    "sync"
    "time"
)

type CodebaseInsight struct {
    ID          string                 `json:"id"`
    Pattern     string                 `json:"pattern"`
    Description string                 `json:"description"`
    Category    string                 `json:"category"` // "error_pattern", "architecture", "dependency"
    Confidence  float64                `json:"confidence"`
    Examples    []string               `json:"examples"`
    FirstSeen   time.Time              `json:"first_seen"`
    LastSeen    time.Time              `json:"last_seen"`
    Occurrences int                    `json:"occurrences"`
    Metadata    map[string]interface{} `json:"metadata"`
}

type TaskHistory struct {
    ID            string    `json:"id"`
    Task          string    `json:"task"`
    FilesAccessed []string  `json:"files_accessed"`
    ToolsUsed     []string  `json:"tools_used"`
    Success       bool      `json:"success"`
    TurnCount     int       `json:"turn_count"`
    Timestamp     time.Time `json:"timestamp"`
    ErrorsEncountered []string `json:"errors_encountered"`
}

type PersistentMemory struct {
    StoragePath string
    Insights    map[string]*CodebaseInsight
    History     []TaskHistory
    mu          sync.RWMutex
}
```

**2.2 Implement Storage Layer**

```go
func NewPersistentMemory(srcRoot string) (*PersistentMemory, error) {
    storageDir := filepath.Join(srcRoot, ".codepicker", "memory")
    if err := os.MkdirAll(storageDir, 0755); err != nil {
        return nil, fmt.Errorf("failed to create memory storage: %w", err)
    }
    
    pm := &PersistentMemory{
        StoragePath: storageDir,
        Insights:    make(map[string]*CodebaseInsight),
        History:     make([]TaskHistory, 0),
    }
    
    // Load existing data
    if err := pm.Load(); err != nil {
        // Non-fatal - just start fresh
        return pm, nil
    }
    
    return pm, nil
}

func (pm *PersistentMemory) Load() error {
    pm.mu.Lock()
    defer pm.mu.Unlock()
    
    // Load insights
    insightsPath := filepath.Join(pm.StoragePath, "insights.json")
    if data, err := os.ReadFile(insightsPath); err == nil {
        json.Unmarshal(data, &pm.Insights)
    }
    
    // Load history
    historyPath := filepath.Join(pm.StoragePath, "history.json")
    if data, err := os.ReadFile(historyPath); err == nil {
        json.Unmarshal(data, &pm.History)
    }
    
    return nil
}

func (pm *PersistentMemory) Save() error {
    pm.mu.RLock()
    defer pm.mu.RUnlock()
    
    // Save insights
    insightsPath := filepath.Join(pm.StoragePath, "insights.json")
    if data, err := json.MarshalIndent(pm.Insights, "", "  "); err == nil {
        os.WriteFile(insightsPath, data, 0644)
    }
    
    // Save history (keep last 100 entries)
    historyPath := filepath.Join(pm.StoragePath, "history.json")
    history := pm.History
    if len(history) > 100 {
        history = history[len(history)-100:]
    }
    if data, err := json.MarshalIndent(history, "", "  "); err == nil {
        os.WriteFile(historyPath, data, 0644)
    }
    
    return nil
}
```

**2.3 Add Insight Recording**

```go
func (pm *PersistentMemory) RecordInsight(pattern, description, category string, examples []string) {
    pm.mu.Lock()
    defer pm.mu.Unlock()
    
    id := fmt.Sprintf("%s_%s", category, hashString(pattern))
    
    if existing, found := pm.Insights[id]; found {
        existing.LastSeen = time.Now()
        existing.Occurrences++
        existing.Examples = append(existing.Examples, examples...)
        // Keep only most recent 5 examples
        if len(existing.Examples) > 5 {
            existing.Examples = existing.Examples[len(existing.Examples)-5:]
        }
    } else {
        pm.Insights[id] = &CodebaseInsight{
            ID:          id,
            Pattern:     pattern,
            Description: description,
            Category:    category,
            Confidence:  0.5,
            Examples:    examples,
            FirstSeen:   time.Now(),
            LastSeen:    time.Now(),
            Occurrences: 1,
            Metadata:    make(map[string]interface{}),
        }
    }
    
    pm.Save()
}

func (pm *PersistentMemory) GetRelevantInsights(query string, category string) []*CodebaseInsight {
    pm.mu.RLock()
    defer pm.mu.RUnlock()
    
    var relevant []*CodebaseInsight
    queryLower := strings.ToLower(query)
    
    for _, insight := range pm.Insights {
        if category != "" && insight.Category != category {
            continue
        }
        
        if strings.Contains(strings.ToLower(insight.Pattern), queryLower) ||
           strings.Contains(strings.ToLower(insight.Description), queryLower) {
            relevant = append(relevant, insight)
        }
    }
    
    // Sort by confidence and recency
    sort.Slice(relevant, func(i, j int) bool {
        return relevant[i].Confidence > relevant[j].Confidence
    })
    
    return relevant
}
```

**2.4 Integrate with Agent Engine**

Modify `internal/agent/engine.go`:

```go
type Engine struct {
    Client           *openrouter.Client
    Model            string
    Sentinel         *Sentinel
    Shadow           *shadow.Manager
    Memory           *WorkingMemory
    PersistentMemory *PersistentMemory  // Add this
    Logger           logger.Logger
    SrcRoot          string
    ApprovalCallback func(command string, reason string) bool
    CostTracker      *tracking.CostTracker
    Limits           *config.Limits
}

func NewEngine(client *openrouter.Client, model, srcRoot string, log logger.Logger, limits *config.Limits) (*Engine, error) {
    shadowMgr, err := shadow.NewManager(srcRoot)
    if err != nil {
        return nil, err
    }
    
    persistentMem, err := NewPersistentMemory(srcRoot)
    if err != nil {
        log.Warn(fmt.Sprintf("Failed to initialize persistent memory: %v", err))
    }

    return &Engine{
        Client:           client,
        Model:            model,
        Sentinel:         NewSentinel(limits),
        Shadow:           shadowMgr,
        Memory:           NewMemory(srcRoot),
        PersistentMemory: persistentMem,
        Logger:           log,
        SrcRoot:          srcRoot,
        ApprovalCallback: func(c, r string) bool { return false },
        CostTracker:      tracking.NewCostTracker(limits.DailyCostLimit),
        Limits:           limits,
    }, nil
}
```

**2.5 Record Task History**

At the end of `Engine.Run()`:

```go
func (e *Engine) Run(ctx context.Context, task string, updateHistory func(openrouter.ChatMessage)) (string, error) {
    // ... existing code ...
    
    // Record task in history
    if e.PersistentMemory != nil {
        taskRecord := TaskHistory{
            ID:            fmt.Sprintf("task_%d", time.Now().Unix()),
            Task:          task,
            FilesAccessed: e.Memory.List(),
            ToolsUsed:     extractToolsFromMessages(messages),
            Success:       err == nil,
            TurnCount:     i,
            Timestamp:     time.Now(),
        }
        
        e.PersistentMemory.RecordTask(taskRecord)
    }
    
    return result, err
}
```

---

### Phase 3: Self-Reflection and Learning (Priority: High, Duration: 3-4 days)

#### Objective
Enable the agent to analyze its own performance after completing tasks and extract learnable patterns.

#### Implementation Details

**3.1 Create Reflection Framework**

Create new file: `internal/agent/reflection.go`

```go
package agent

import (
    "fmt"
    "strings"
    
    "github.com/david22573/codepicker/pkg/openrouter"
)

type PerformanceMetrics struct {
    TotalTurns          int
    ToolCallCount       int
    FileAccessCount     int
    ErrorCount          int
    RecoveryAttempts    int
    SuccessfulRecoveries int
}

type TaskReflection struct {
    Efficiency      string   // "optimal", "acceptable", "inefficient"
    PatternsFound   []string
    SuggestedOptimizations []string
    KeyInsights     []string
}

func (e *Engine) AnalyzePerformance(messages []openrouter.ChatMessage, metrics PerformanceMetrics) TaskReflection {
    reflection := TaskReflection{
        PatternsFound:   make([]string, 0),
        SuggestedOptimizations: make([]string, 0),
        KeyInsights:     make([]string, 0),
    }
    
    // Analyze efficiency
    if metrics.TotalTurns <= 3 {
        reflection.Efficiency = "optimal"
    } else if metrics.TotalTurns <= 7 {
        reflection.Efficiency = "acceptable"
    } else {
        reflection.Efficiency = "inefficient"
        reflection.SuggestedOptimizations = append(reflection.SuggestedOptimizations,
            "Task took many turns - consider broader initial file selection")
    }
    
    // Detect patterns in file access
    filesAccessed := e.Memory.List()
    if len(filesAccessed) > 0 {
        commonPatterns := detectFilePatterns(filesAccessed)
        for _, pattern := range commonPatterns {
            reflection.PatternsFound = append(reflection.PatternsFound, pattern)
        }
    }
    
    // Check for repeated errors
    errorPatterns := extractErrorPatterns(messages)
    if len(errorPatterns) > 0 {
        for pattern, count := range errorPatterns {
            if count > 1 {
                insight := fmt.Sprintf("Repeated error pattern: %s (occurred %d times)", pattern, count)
                reflection.KeyInsights = append(reflection.KeyInsights, insight)
            }
        }
    }
    
    return reflection
}

func detectFilePatterns(files []string) []string {
    patterns := make([]string, 0)
    
    dirFrequency := make(map[string]int)
    for _, file := range files {
        dir := filepath.Dir(file)
        dirFrequency[dir]++
    }
    
    for dir, count := range dirFrequency {
        if count >= 3 {
            patterns = append(patterns, fmt.Sprintf("Frequent access to %s/ directory", dir))
        }
    }
    
    return patterns
}

func extractErrorPatterns(messages []openrouter.ChatMessage) map[string]int {
    patterns := make(map[string]int)
    
    for _, msg := range messages {
        if msg.Role == "tool" {
            content, ok := msg.Content.(string)
            if !ok {
                continue
            }
            
            if strings.Contains(content, "Command failed") {
                // Extract error type
                if strings.Contains(content, "permission denied") {
                    patterns["permission_denied"]++
                } else if strings.Contains(content, "no such file") {
                    patterns["file_not_found"]++
                } else if strings.Contains(content, "syntax error") {
                    patterns["syntax_error"]++
                }
            }
        }
    }
    
    return patterns
}
```

**3.2 Integrate Reflection into Task Completion**

Modify the end of `Engine.Run()`:

```go
func (e *Engine) Run(ctx context.Context, task string, updateHistory func(openrouter.ChatMessage)) (string, error) {
    // ... existing execution logic ...
    
    // Calculate metrics
    metrics := PerformanceMetrics{
        TotalTurns:      i + 1,
        ToolCallCount:   countToolCalls(messages),
        FileAccessCount: len(e.Memory.List()),
        ErrorCount:      countErrors(messages),
    }
    
    // Perform self-reflection
    reflection := e.AnalyzePerformance(messages, metrics)
    
    // Log insights
    if reflection.Efficiency != "optimal" {
        e.Logger.Debug(fmt.Sprintf("Task efficiency: %s", reflection.Efficiency))
    }
    
    for _, insight := range reflection.KeyInsights {
        e.Logger.Info(fmt.Sprintf("Insight: %s", insight))
    }
    
    // Store learnings in persistent memory
    if e.PersistentMemory != nil {
        for _, pattern := range reflection.PatternsFound {
            e.PersistentMemory.RecordInsight(
                pattern,
                fmt.Sprintf("Observed during task: %s", truncateString(task, 50)),
                "file_access_pattern",
                e.Memory.List(),
            )
        }
        
        // Store error patterns
        errorPatterns := extractErrorPatterns(messages)
        for pattern, count := range errorPatterns {
            if count > 0 {
                e.PersistentMemory.RecordInsight(
                    pattern,
                    fmt.Sprintf("Error encountered %d times", count),
                    "error_pattern",
                    []string{task},
                )
            }
        }
    }
    
    return result, err
}
```

**3.3 Use Historical Insights for Future Tasks**

At the beginning of `Engine.Run()`, add context from previous learnings:

```go
func (e *Engine) Run(ctx context.Context, task string, updateHistory func(openrouter.ChatMessage)) (string, error) {
    ctx, cancel := context.WithTimeout(ctx, e.Limits.AgentTimeout)
    defer cancel()
    
    baseSystemPrompt := `You are an autonomous AI developer agent.
RULES:
1. Code context is provided in "ACTIVE SOURCE FILES".
2. If you need to see a file not listed there, use 'read_file' to add it to context.
3. DO NOT output code for files already in context unless you are changing them.
4. Use 'write_shadow_file' to propose changes.`

    // Add historical insights
    if e.PersistentMemory != nil {
        relevantInsights := e.PersistentMemory.GetRelevantInsights(task, "")
        if len(relevantInsights) > 0 {
            baseSystemPrompt += "\n\nHISTORICAL INSIGHTS (from previous tasks):\n"
            for i, insight := range relevantInsights {
                if i >= 3 {
                    break // Limit to top 3 most relevant
                }
                baseSystemPrompt += fmt.Sprintf("- %s (seen %d times)\n", 
                    insight.Description, insight.Occurrences)
            }
        }
    }
    
    messages := []openrouter.ChatMessage{
        {Role: "user", Content: task},
    }
    
    // ... rest of existing code ...
}
```

---

### Phase 4: Shadow Filesystem Validation (Priority: Medium, Duration: 2-3 days)

#### Objective
Prevent the agent from writing syntactically invalid code or making risky changes without proper analysis.

#### Implementation Details

**4.1 Create Validation Framework**

Create new file: `internal/shadow/validation.go`

```go
package shadow

import (
    "bytes"
    "fmt"
    "go/ast"
    "go/parser"
    "go/token"
    "os"
    "path/filepath"
    "strings"
)

type ValidationResult struct {
    Valid          bool
    Errors         []string
    Warnings       []string
    RiskScore      float64  // 0.0 (safe) to 1.0 (very risky)
    Recommendation string
    Diff           string
}

type Validator interface {
    Validate(content []byte) ValidationResult
}

type GoValidator struct{}

func (v *GoValidator) Validate(content []byte) ValidationResult {
    result := ValidationResult{
        Valid:    true,
        Errors:   make([]string, 0),
        Warnings: make([]string, 0),
    }
    
    fset := token.NewFileSet()
    _, err := parser.ParseFile(fset, "", content, parser.AllErrors)
    
    if err != nil {
        result.Valid = false
        result.Errors = append(result.Errors, fmt.Sprintf("Syntax error: %v", err))
        result.RiskScore = 0.9
        result.Recommendation = "Fix syntax errors before applying"
        return result
    }
    
    // Additional checks
    contentStr := string(content)
    
    // Check for dangerous patterns
    if strings.Contains(contentStr, "os.RemoveAll") {
        result.Warnings = append(result.Warnings, "Contains os.RemoveAll - destructive operation")
        result.RiskScore += 0.3
    }
    
    if strings.Contains(contentStr, "exec.Command") {
        result.Warnings = append(result.Warnings, "Contains exec.Command - external execution")
        result.RiskScore += 0.2
    }
    
    // Normalize risk score
    if result.RiskScore > 1.0 {
        result.RiskScore = 1.0
    }
    
    if result.RiskScore > 0.5 {
        result.Recommendation = "Manual review recommended before applying"
    } else if result.RiskScore > 0.2 {
        result.Recommendation = "Safe to apply with caution"
    } else {
        result.Recommendation = "Safe to apply"
    }
    
    return result
}

type JSValidator struct{}

func (v *JSValidator) Validate(content []byte) ValidationResult {
    result := ValidationResult{
        Valid:    true,
        Errors:   make([]string, 0),
        Warnings: make([]string, 0),
    }
    
    // Basic syntax checks for JS/TS
    contentStr := string(content)
    
    // Check for unclosed braces
    braceCount := strings.Count(contentStr, "{") - strings.Count(contentStr, "}")
    if braceCount != 0 {
        result.Valid = false
        result.Errors = append(result.Errors, "Unclosed braces detected")
        result.RiskScore = 0.8
    }
    
    // Check for dangerous patterns
    if strings.Contains(contentStr, "eval(") {
        result.Warnings = append(result.Warnings, "Contains eval() - security risk")
        result.RiskScore += 0.4
    }
    
    if result.RiskScore > 1.0 {
        result.RiskScore = 1.0
    }
    
    if result.RiskScore > 0.5 {
        result.Recommendation = "Manual review recommended"
    } else {
        result.Recommendation = "Looks safe to apply"
    }
    
    return result
}
```

**4.2 Integrate Validation into Shadow Manager**

Modify `internal/shadow/fs.go`:

```go
func (m *Manager) WriteFileWithValidation(relPath string, content []byte) (*ValidationResult, error) {
    // Path safety check
    if strings.Contains(relPath, "..") {
        return nil, fmt.Errorf("invalid path: cannot escape project root")
    }
    
    // Select validator based on file extension
    var validator Validator
    ext := strings.ToLower(filepath.Ext(relPath))
    
    switch ext {
    case ".go":
        validator = &GoValidator{}
    case ".js", ".ts", ".jsx", ".tsx":
        validator = &JSValidator{}
    default:
        // No validation for other file types, just write
        shadowPath, err := m.writeToShadow(relPath, content)
        if err != nil {
            return nil, err
        }
        return &ValidationResult{
            Valid:          true,
            RiskScore:      0.0,
            Recommendation: "No validation performed for this file type",
        }, nil
    }
    
    // Validate content
    validationResult := validator.Validate(content)
    
    if !validationResult.Valid {
        return &validationResult, fmt.Errorf("validation failed: %v", validationResult.Errors)
    }
    
    // Compute diff if original file exists
    realPath := filepath.Join(m.SrcRoot, relPath)
    if originalContent, err := os.ReadFile(realPath); err == nil {
        validationResult.Diff = computeSimpleDiff(originalContent, content)
    }
    
    // Write to shadow filesystem
    shadowPath, err := m.writeToShadow(relPath, content)
    if err != nil {
        return nil, err
    }
    
    return &validationResult, nil
}

func (m *Manager) writeToShadow(relPath string, content []byte) (string, error) {
    shadowPath := filepath.Join(m.ShadowRoot, relPath)
    
    if err := os.MkdirAll(filepath.Dir(shadowPath), 0755); err != nil {
        return "", err
    }
    
    if err := os.WriteFile(shadowPath, content, 0644); err != nil {
        return "", err
    }
    
    return shadowPath, nil
}

func computeSimpleDiff(original, new []byte) string {
    origLines := strings.Split(string(original), "\n")
    newLines := strings.Split(string(new), "\n")
    
    var diff strings.Builder
    diff.WriteString(fmt.Sprintf("Changes: ~%d lines\n", 
        abs(len(newLines)-len(origLines))))
    
    // Simple line-by-line comparison
    maxLines := max(len(origLines), len(newLines))
    changedLines := 0
    
    for i := 0; i < maxLines && i < 10; i++ { // Show first 10 changes
        var oldLine, newLine string
        if i < len(origLines) {
            oldLine = origLines[i]
        }
        if i < len(newLines) {
            newLine = newLines[i]
        }
        
        if oldLine != newLine {
            changedLines++
            diff.WriteString(fmt.Sprintf("Line %d:\n", i+1))
            diff.WriteString(fmt.Sprintf("  - %s\n", oldLine))
            diff.WriteString(fmt.Sprintf("  + %s\n", newLine))
        }
    }
    
    if changedLines > 10 {
        diff.WriteString(fmt.Sprintf("... and %d more changes\n", changedLines-10))
    }
    
    return diff.String()
}

func abs(x int) int {
    if x < 0 {
        return -x
    }
    return x
}

func max(a, b int) int {
    if a > b {
        return a
    }
    return b
}
```

**4.3 Update Agent Engine to Use Validation**

Modify `internal/agent/engine.go` in the `write_shadow_file` case:

```go
case "write_shadow_file":
    var args struct {
        Path    string `json:"path"`
        Content string `json:"content"`
    }
    if err := json.Unmarshal([]byte

(tool.Function.Arguments), &args); err != nil {
        e.Logger.Error(fmt.Sprintf("Failed to parse write_shadow_file args: %v", err))
        resultStr = fmt.Sprintf("Invalid arguments for write_shadow_file: %v", err)
        break
    }
    
    validationResult, err := e.Shadow.WriteFileWithValidation(args.Path, []byte(args.Content))
    if err != nil {
        e.Logger.Warn(fmt.Sprintf("Failed to write shadow file %s: %v", args.Path, err))
        resultStr = fmt.Sprintf("Error writing shadow file: %v", err)
    } else {
        e.Logger.Info(fmt.Sprintf("Written to shadow: %s", args.Path))
        
        // Build detailed result message
        var resultParts []string
        resultParts = append(resultParts, fmt.Sprintf("✓ File written to shadow: %s", args.Path))
        
        if !validationResult.Valid {
            resultParts = append(resultParts, fmt.Sprintf("⚠ Validation errors: %v", validationResult.Errors))
        }
        
        if len(validationResult.Warnings) > 0 {
            resultParts = append(resultParts, fmt.Sprintf("⚠ Warnings: %v", validationResult.Warnings))
        }
        
        resultParts = append(resultParts, fmt.Sprintf("Risk score: %.2f/1.0", validationResult.RiskScore))
        resultParts = append(resultParts, fmt.Sprintf("Recommendation: %s", validationResult.Recommendation))
        
        if validationResult.Diff != "" {
            resultParts = append(resultParts, fmt.Sprintf("\nDiff preview:\n%s", validationResult.Diff))
        }
        
        resultStr = strings.Join(resultParts, "\n")
        
        // Record validation failures as insights
        if !validationResult.Valid && e.PersistentMemory != nil {
            e.PersistentMemory.RecordInsight(
                "validation_failure",
                fmt.Sprintf("Generated invalid %s code", filepath.Ext(args.Path)),
                "error_pattern",
                validationResult.Errors,
            )
        }
    }
```

---

### Phase 5: Self-Correcting Planner (Priority: Medium, Duration: 2-3 days)

#### Objective
Enable the planning system to validate its file selections and retry with better criteria if the initial selection appears inadequate.

#### Implementation Details

**5.1 Add Relevance Scoring**

Modify `internal/planner/planner.go`:

```go
type RelevanceScore struct {
    Confidence      float64
    FilesSelected   int
    MissingAspects  []string
    Reasoning       string
}

func scoreRelevance(selectedFiles []string, query string) RelevanceScore {
    score := RelevanceScore{
        Confidence:     0.5,
        FilesSelected:  len(selectedFiles),
        MissingAspects: make([]string, 0),
    }
    
    // Heuristic checks
    queryLower := strings.ToLower(query)
    
    // Check if query mentions specific files/directories
    if strings.Contains(queryLower, "cmd/") && !hasPathPrefix(selectedFiles, "cmd/") {
        score.Confidence -= 0.2
        score.MissingAspects = append(score.MissingAspects, "cmd/ directory")
    }
    
    if strings.Contains(queryLower, "internal/") && !hasPathPrefix(selectedFiles, "internal/") {
        score.Confidence -= 0.2
        score.MissingAspects = append(score.MissingAspects, "internal/ directory")
    }
    
    // Check for suspicious empty selections
    if len(selectedFiles) == 0 {
        score.Confidence = 0.1
        score.MissingAspects = append(score.MissingAspects, "no files selected")
        score.Reasoning = "Empty selection likely indicates planning failure"
        return score
    }
    
    // Check for overly broad selections
    if len(selectedFiles) > 50 {
        score.Confidence -= 0.1
        score.Reasoning = "Very broad selection may indicate poor targeting"
    }
    
    // Boost confidence for reasonable selections
    if len(selectedFiles) >= 3 && len(selectedFiles) <= 20 {
        score.Confidence += 0.2
    }
    
    // Normalize
    if score.Confidence > 1.0 {
        score.Confidence = 1.0
    }
    if score.Confidence < 0.0 {
        score.Confidence = 0.0
    }
    
    return score
}

func hasPathPrefix(files []string, prefix string) bool {
    for _, f := range files {
        if strings.HasPrefix(f, prefix) {
            return true
        }
    }
    return false
}
```

**5.2 Implement Iterative Planning**

```go
func SelectRelevantFilesWithValidation(ctx context.Context, opts Options, log logger.Logger) ([]string, error) {
    maxAttempts := 3
    
    for attempt := 1; attempt <= maxAttempts; attempt++ {
        log.Info(fmt.Sprintf("Planning attempt %d/%d", attempt, maxAttempts))
        
        selectedFiles, err := selectFilesInternal(ctx, opts, log)
        if err != nil {
            return nil, err
        }
        
        // Validate selection
        score := scoreRelevance(selectedFiles, opts.Query)
        
        log.Debug(fmt.Sprintf("Relevance score: %.2f (confidence threshold: 0.7)", score.Confidence))
        
        if score.Confidence >= 0.7 {
            log.Info(fmt.Sprintf("✓ File selection validated (%.0f%% confidence)", score.Confidence*100))
            return selectedFiles, nil
        }
        
        // Self-correction for next attempt
        if attempt < maxAttempts {
            log.Warn(fmt.Sprintf("Low confidence (%.0f%%), retrying with adjusted criteria", score.Confidence*100))
            
            // Provide feedback to the LLM
            if len(score.MissingAspects) > 0 {
                opts.Query = fmt.Sprintf("%s\n\nIMPORTANT: Previous selection missed these aspects: %s. Please include relevant files.",
                    opts.Query, strings.Join(score.MissingAspects, ", "))
            }
        }
    }
    
    log.Warn("Planning completed with suboptimal file selection after max attempts")
    return selectedFiles, nil
}

func selectFilesInternal(ctx context.Context, opts Options, log logger.Logger) ([]string, error) {
    // This is the existing SelectRelevantFiles logic
    // Move the core file selection logic here
    
    absSrc, err := paths.Sanitize(opts.SrcDir)
    if err != nil {
        return nil, fmt.Errorf("invalid source directory")
    }

    collector := &PathCollector{}
    cfg := config.NewConfig()
    if opts.IncludeExts != "" {
        cfg.AddAllowedExtensions(strings.Split(opts.IncludeExts, ","))
    }
    if opts.IgnoreDirs != "" {
        cfg.AddIgnoredDirs(strings.Split(opts.IgnoreDirs, ","))
    }

    s := scanner.NewScanner(absSrc, collector, cfg, log)
    if err := s.Scan(ctx); err != nil {
        return nil, fmt.Errorf("scan failed during planning")
    }

    if len(collector.Paths) == 0 {
        return nil, nil
    }

    maxFiles := 1000
    if len(collector.Paths) > maxFiles {
        collector.Paths = collector.Paths[:maxFiles]
    }

    fileList := strings.Join(collector.Paths, "\n")

    sysMsg := `You are a senior developer.
You have a list of files in a codebase.
Based on the user's query, identify exactly which files contain the relevant code to answer the question.
Return ONLY a valid JSON object with a single key "files" containing the list of strings.
Example: { "files": ["cmd/main.go", "internal/utils.go"] }
If no specific code is needed, return { "files": [] }.`

    userMsg := fmt.Sprintf("Files:\n%s\n\nQuery: %s", fileList, opts.Query)

    selectedFiles := callLLM(ctx, opts.APIKey, opts.Model, sysMsg, userMsg, log)
    validFiles := validatePaths(selectedFiles, absSrc, log)

    return validFiles, nil
}
```

**5.3 Update Public API**

Update the exported function to use the new validation logic:

```go
func SelectRelevantFiles(ctx context.Context, opts Options, log logger.Logger) ([]string, error) {
    return SelectRelevantFilesWithValidation(ctx, opts, log)
}
```

---

## Testing Strategy

### Phase 1 Testing (Error Recovery)
1. Test go mod initialization scenarios
2. Test missing dependencies scenarios
3. Test permission denied handling
4. Verify no infinite recovery loops
5. Test recovery logging and metrics

### Phase 2 Testing (Persistent Memory)
1. Test insight creation and storage
2. Test insight retrieval and relevance filtering
3. Test task history recording
4. Verify persistence across restarts
5. Test storage limits (100 history entries)

### Phase 3 Testing (Self-Reflection)
1. Test performance metric calculation
2. Test pattern detection in file access
3. Test error pattern extraction
4. Verify insights are recorded correctly
5. Test insight injection into future tasks

### Phase 4 Testing (Validation)
1. Test Go syntax validation
2. Test JavaScript syntax validation
3. Test risk scoring algorithm
4. Test diff generation
5. Test validation error handling

### Phase 5 Testing (Self-Correcting Planner)
1. Test relevance scoring with various queries
2. Test multi-attempt planning
3. Test feedback loop to LLM
4. Verify confidence thresholds
5. Test with edge cases (empty selections, over-selections)

---

## Success Metrics

### Quantitative Metrics
- **Error Recovery Rate**: Percentage of failures automatically recovered (target: >70%)
- **Task Efficiency**: Average turns per task completion (target: reduce by 30%)
- **Validation Accuracy**: Percentage of generated code passing syntax validation (target: >95%)
- **Planning Accuracy**: Percentage of file selections validated on first attempt (target: >80%)
- **Memory Growth**: Number of useful insights accumulated over 100 tasks (target: >50)

### Qualitative Metrics
- Agent can complete multi-step tasks with minimal human intervention
- Agent learns from mistakes and doesn't repeat the same errors
- Agent provides useful feedback about code quality and risks
- Agent makes informed decisions based on historical patterns

---

## Rollout Plan

### Week 1
- Implement Phase 1 (Error Recovery) - Days 1-3
- Write tests for error recovery - Days 4-5

### Week 2
- Implement Phase 2 (Persistent Memory) - Days 1-5
- Write tests for memory system - Days 6-7

### Week 3
- Implement Phase 3 (Self-Reflection) - Days 1-4
- Write tests for reflection system - Days 5-7

### Week 4
- Implement Phase 4 (Validation) - Days 1-3
- Implement Phase 5 (Self-Correcting Planner) - Days 4-6
- Integration testing and bug fixes - Day 7

### Week 5
- End-to-end testing with real-world scenarios
- Performance optimization
- Documentation updates
- Final validation

---

## Risk Mitigation

### Technical Risks
1. **Risk**: Persistent memory grows too large
   - **Mitigation**: Implement automatic pruning of old insights, keep only top N by confidence

2. **Risk**: Error recovery creates infinite loops
   - **Mitigation**: Hard limit on recovery attempts per command, timeout mechanisms

3. **Risk**: Validation false positives block valid code
   - **Mitigation**: Make validation warnings informational, only block on syntax errors

4. **Risk**: Self-reflection adds significant latency
   - **Mitigation**: Make reflection async, run after response is returned to user

### Operational Risks
1. **Risk**: Breaking changes to existing functionality
   - **Mitigation**: Comprehensive test coverage, feature flags for new capabilities

2. **Risk**: Increased API costs from additional LLM calls
   - **Mitigation**: Cache validation results, use cheaper models for reflection

---

## Dependencies and Prerequisites

### Required Libraries
- No new external dependencies required
- All implementations use standard library and existing dependencies

### Environment Requirements
- Go 1.21+ (current requirement)
- Write access to `.codepicker/` directory for persistent memory
- Sufficient disk space for insight storage (~1-10MB typical)

### Configuration Updates
Add to `internal/config/limits.go`:

```go
type Limits struct {
    // ... existing fields ...
    
    // New fields for enhanced capabilities
    MaxRecoveryAttempts   int
    MaxInsightStorage     int  // Max number of insights to keep
    ReflectionEnabled     bool
    ValidationEnabled     bool
    MaxPlanningAttempts   int
}

func DefaultLimits() *Limits {
    return &Limits{
        // ... existing defaults ...
        
        MaxRecoveryAttempts:  2,
        MaxInsightStorage:    500,
        ReflectionEnabled:    true,
        ValidationEnabled:    true,
        MaxPlanningAttempts:  3,
    }
}
```

---

## Future Enhancements (Beyond This Plan)

These are potential improvements to consider after completing the core enhancement plan:

1. **Multi-Agent Collaboration**: Multiple specialized agents working together
2. **Benchmark Suite**: Standardized task set for measuring improvement
3. **Explainability Dashboard**: UI for visualizing agent decision-making
4. **Code Quality Metrics**: Integration with linters and static analysis tools
5. **Learned Tool Sequences**: Automatically discover common tool usage patterns
6. **Proactive Suggestions**: Agent suggests improvements without being asked

---

## Conclusion

This plan provides a structured path to significantly enhance the codepicker agent's autonomy. Each phase builds upon the previous, creating a foundation for truly autonomous operation. The estimated 3-4 weeks of implementation time will result in an agent capable of:

- Automatically recovering from common failures
- Learning from experience across sessions
- Analyzing its own performance and improving over time
- Validating generated code before writing
- Self-correcting when initial plans are inadequate

The key to success is implementing these phases sequentially, with thorough testing at each stage, and maintaining the existing production-grade quality standards already present in the codebase.
