You are an expert Go systems architect tasked with refactoring an AI agent codebase called **CodePicker**. This is a complex, multi-phase project that requires careful planning and incremental implementation.

## Context

CodePicker is a CLI tool that uses AI agents (via OpenRouter API) to autonomously read, analyze, and modify codebases. It has grown into a 20,000+ line monolith with severe architectural issues:

- **Scope creep**: Tries to be a context generator, AI agent framework, batch processor, MCP server, AND deployment tool
- **Tight coupling**: Circular dependencies between packages, everything imports everything
- **Inconsistent patterns**: Three different orchestration systems, mixed dependency injection strategies
- **Over-engineering**: Complex checkpoint system (26 fields), relevance scoring without validation, auto-recovery that's dangerous
- **Security confusion**: Three overlapping security layers with contradictory rules
- **Poor observability**: Emoji-based logging, no structured telemetry

**Your mission**: Rebuild this into a clean, maintainable 2000-line system following Go best practices.

---

## Core Requirements (Non-Negotiable)

1. **Shadow Filesystem**: Changes staged in `.codepicker/shadow/` before applying to source
2. **Policy System**: Strict security policies (no shell in batch mode, approval gates in interactive)
3. **Tool-Based Agent**: LLM calls tools (read_file, write_file, search_code, run_shell, delegate_task)
4. **Plan Execution**: Break tasks into steps, execute sequentially with checkpointing
5. **Cost Tracking**: Monitor and limit API costs per session/day
6. **Multi-Model Support**: Use cheaper "worker" model for grunt work, smarter "supervisor" for planning

---

## Architecture Principles

Apply these rigorously:

### 1. **Hexagonal Architecture (Ports & Adapters)**
```
domain/      (pure business logic, zero external deps)
├── agent/      (Agent, Tool, Plan interfaces)
├── policy/     (security rules, pure functions)
└── execution/  (state machine for plan execution)

infra/       (external dependencies)
├── llm/        (OpenRouter client)
├── storage/    (SQLite, file I/O)
└── shell/      (command execution)

adapters/    (implement domain interfaces)
├── tools/      (concrete tool implementations)
└── prompts/    (LLM system prompts)

app/         (composition root - wires everything)
cmd/         (CLI commands, thin layer)
```

### 2. **Dependency Rule**
- `domain/` imports NOTHING from other packages
- `infra/` can import `domain/` interfaces
- `adapters/` import both domain and infra
- `app/` imports everything to wire dependencies
- `cmd/` imports only `app/`

### 3. **Clear Interfaces**
```go
// domain/agent/agent.go
type Agent interface {
    Execute(ctx context.Context, task Task) (*Result, error)
}

type Tool interface {
    Name() string
    Execute(ctx context.Context, args json.RawMessage) (string, error)
    Capabilities() []Capability
}

type Repository interface {
    SaveExecution(exec *Execution) error
    LoadExecution(id string) (*Execution, error)
}
```

### 4. **Single Responsibility**
- Each package has ONE reason to change
- Functions do ONE thing (max 50 lines)
- Structs have max 7 fields

### 5. **Explicit Over Implicit**
- No global state
- No init() functions with side effects
- Constructor functions explicitly return errors
- Dependencies injected via constructors

---

## Refactor Roadmap

Execute in strict order. Each phase must pass before moving to next.

---

## **PHASE 1: Foundation & Cleanup** (Week 1)

**Goal**: Establish clean architecture, delete dangerous code, create skeleton

### Tasks:

#### 1.1 Delete Hazardous Code
**Files to DELETE entirely**:
- `internal/agent/recovery.go` (auto-recovery is unsafe)
- `internal/agent/orchestrator.go` (redundant, keep PlanExecutor)
- `internal/database/relevance.go` (unproven optimization)
- `internal/prompts/prompts.go` (will externalize)

**Rationale**: These are liabilities. Recovery auto-fixes can break builds, orchestrator duplicates PlanExecutor, relevance scoring adds complexity without proof of value.

#### 1.2 Create New Package Structure
```
codepicker/
├── domain/
│   ├── agent/
│   │   ├── agent.go          (Agent, Tool, Plan interfaces)
│   │   ├── execution.go      (Execution state machine)
│   │   └── policy.go         (Policy interface)
│   ├── task/
│   │   └── task.go           (Task, Step value objects)
│   └── errors/
│       └── errors.go         (domain-specific errors)
│
├── infra/
│   ├── llm/
│   │   ├── client.go         (OpenRouter HTTP client)
│   │   └── types.go          (API request/response types)
│   ├── storage/
│   │   ├── sqlite.go         (checkpoint persistence)
│   │   └── filesystem.go     (shadow FS implementation)
│   └── shell/
│       └── executor.go       (safe command execution)
│
├── adapters/
│   ├── tools/
│   │   ├── registry.go       (tool factory)
│   │   ├── read.go          (read_file tool)
│   │   ├── write.go         (write_shadow_file tool)
│   │   ├── search.go        (search_code tool)
│   │   └── shell.go         (run_shell tool)
│   ├── prompts/
│   │   └── loader.go         (load from ~/.codepicker/prompts/)
│   └── policy/
│       └── enforcer.go       (policy implementation)
│
├── app/
│   ├── runtime.go            (DI container, wire dependencies)
│   └── config.go             (load .codepicker.yml)
│
└── cmd/
    ├── root.go
    ├── agent.go              (agent run, plan, resume)
    ├── apply.go              (apply shadow changes)
    └── context.go            (generate context files)
```

#### 1.3 Define Core Domain Interfaces

**File: `domain/agent/agent.go`**
```go
package agent

import (
    "context"
    "encoding/json"
)

// Agent executes tasks by calling tools and managing state
type Agent interface {
    Execute(ctx context.Context, task *Task) (*Result, error)
}

// Tool represents a capability the agent can invoke
type Tool interface {
    Name() string
    Description() string
    Execute(ctx context.Context, args json.RawMessage) (string, error)
    Capabilities() []Capability
}

type Capability string

const (
    CapRead    Capability = "read"
    CapWrite   Capability = "write"
    CapExecute Capability = "execute"
)

// Policy controls what tools can execute
type Policy interface {
    Authorize(tool Tool, args json.RawMessage) error
}

// Repository persists execution state
type Repository interface {
    SaveExecution(exec *Execution) error
    LoadExecution(id string) (*Execution, error)
    SaveCheckpoint(checkpoint *Checkpoint) error
}
```

**File: `domain/agent/execution.go`**
```go
package agent

import "time"

// Execution represents a single agent run
type Execution struct {
    ID          string
    Task        string
    Plan        *Plan
    State       State
    Events      []Event
    Cost        Cost
    CreatedAt   time.Time
    CompletedAt *time.Time
}

type State string

const (
    StatePending   State = "pending"
    StateRunning   State = "running"
    StateCompleted State = "completed"
    StateFailed    State = "failed"
)

type Event struct {
    Timestamp time.Time
    Type      EventType
    Data      map[string]interface{}
}

type EventType string

const (
    EventToolCall     EventType = "tool_call"
    EventThought      EventType = "thought"
    EventStepComplete EventType = "step_complete"
)

type Cost struct {
    PromptTokens     int
    CompletionTokens int
    TotalUSD         float64
}
```

**File: `domain/task/task.go`**
```go
package task

// Task is what the user wants to accomplish
type Task struct {
    Description string
    Context     []string // File paths for context
}

// Plan breaks task into executable steps
type Plan struct {
    ID       string
    Steps    []Step
    Metadata map[string]string
}

type Step struct {
    ID          int
    Description string
    Instruction string // Detailed instruction for LLM
    Files       []string
    Status      StepStatus
    Result      string
}

type StepStatus string

const (
    StepPending   StepStatus = "pending"
    StepRunning   StepStatus = "running"
    StepCompleted StepStatus = "completed"
    StepFailed    StepStatus = "failed"
)
```

#### 1.4 Acceptance Criteria
- [ ] New package structure created
- [ ] All interfaces compile with zero dependencies on old code
- [ ] Old dangerous files deleted
- [ ] `go build ./...` succeeds (even if empty implementations)

---

## **PHASE 2: Infrastructure Layer** (Week 2)

**Goal**: Implement external dependencies that domain relies on

### Tasks:

#### 2.1 LLM Client (Pure HTTP, No Business Logic)

**File: `infra/llm/client.go`**
```go
package llm

import (
    "context"
    "encoding/json"
)

type Client struct {
    apiKey  string
    baseURL string
    client  *http.Client
}

func NewClient(apiKey string) *Client {
    return &Client{
        apiKey:  apiKey,
        baseURL: "https://openrouter.ai/api/v1",
        client:  &http.Client{Timeout: 5 * time.Minute},
    }
}

type Request struct {
    Model    string    `json:"model"`
    Messages []Message `json:"messages"`
    Tools    []Tool    `json:"tools,omitempty"`
}

type Response struct {
    Choices []Choice `json:"choices"`
    Usage   Usage    `json:"usage"`
}

func (c *Client) Complete(ctx context.Context, req Request) (*Response, error) {
    // Simple HTTP POST, retry logic, error handling
    // NO BUSINESS LOGIC (no cost tracking, no token limits here)
}
```

#### 2.2 Storage Layer (SQLite + Shadow FS)

**File: `infra/storage/repository.go`**
```go
package storage

import (
    "database/sql"
    "github.com/yourname/codepicker/domain/agent"
)

type SQLiteRepository struct {
    db *sql.DB
}

func NewRepository(dbPath string) (*SQLiteRepository, error) {
    db, err := sql.Open("sqlite3", dbPath)
    // ... migrations
    return &SQLiteRepository{db: db}, nil
}

func (r *SQLiteRepository) SaveExecution(exec *agent.Execution) error {
    // Simple INSERT/UPDATE, no fancy JSON in columns
    // Use separate tables: executions, steps, events
}
```

**Schema Design** (normalized, no JSON columns):
```sql
CREATE TABLE executions (
    id TEXT PRIMARY KEY,
    task TEXT NOT NULL,
    state TEXT NOT NULL,
    total_cost_usd REAL,
    created_at INTEGER,
    completed_at INTEGER
);

CREATE TABLE steps (
    id INTEGER PRIMARY KEY,
    execution_id TEXT,
    description TEXT,
    status TEXT,
    result TEXT,
    FOREIGN KEY(execution_id) REFERENCES executions(id)
);

CREATE TABLE events (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    execution_id TEXT,
    timestamp INTEGER,
    type TEXT,
    data TEXT,  -- Simple JSON for flexibility, but queried minimally
    FOREIGN KEY(execution_id) REFERENCES executions(id)
);

CREATE TABLE checkpoints (
    id TEXT PRIMARY KEY,
    execution_id TEXT,
    snapshot TEXT,  -- JSON of entire Execution state
    created_at INTEGER,
    FOREIGN KEY(execution_id) REFERENCES executions(id)
);
```

**File: `infra/storage/shadow.go`**
```go
package storage

type ShadowFS struct {
    root       string
    shadowRoot string
}

func NewShadowFS(root string) (*ShadowFS, error) {
    shadowRoot := filepath.Join(root, ".codepicker", "shadow")
    os.MkdirAll(shadowRoot, 0755)
    return &ShadowFS{root: root, shadowRoot: shadowRoot}, nil
}

func (fs *ShadowFS) ReadFile(path string) ([]byte, error) {
    // Check shadow first, fallback to source
    shadowPath := filepath.Join(fs.shadowRoot, path)
    if data, err := os.ReadFile(shadowPath); err == nil {
        return data, nil
    }
    return os.ReadFile(filepath.Join(fs.root, path))
}

func (fs *ShadowFS) WriteFile(path string, content []byte) error {
    // ALWAYS write to shadow, never to source directly
    shadowPath := filepath.Join(fs.shadowRoot, path)
    os.MkdirAll(filepath.Dir(shadowPath), 0755)
    return os.WriteFile(shadowPath, content, 0644)
}
```

#### 2.3 Shell Executor (Secure by Default)

**File: `infra/shell/executor.go`**
```go
package shell

import (
    "context"
    "os/exec"
    "time"
)

type Executor struct {
    timeout time.Duration
    maxOutput int
}

func NewExecutor() *Executor {
    return &Executor{
        timeout: 5 * time.Minute,
        maxOutput: 1024 * 100, // 100KB
    }
}

func (e *Executor) Run(ctx context.Context, command string, args []string) (string, error) {
    ctx, cancel := context.WithTimeout(ctx, e.timeout)
    defer cancel()
    
    cmd := exec.CommandContext(ctx, command, args...)
    
    // Bounded output buffer
    output := &bytes.Buffer{}
    cmd.Stdout = io.LimitReader(output, int64(e.maxOutput))
    cmd.Stderr = io.LimitReader(output, int64(e.maxOutput))
    
    err := cmd.Run()
    return output.String(), err
}
```

#### 2.4 Acceptance Criteria
- [ ] LLM client makes successful API calls
- [ ] Repository saves/loads executions from SQLite
- [ ] Shadow FS correctly reads/writes files
- [ ] Shell executor runs commands with timeout
- [ ] All infra packages have unit tests

---

## **PHASE 3: Tool Adapters** (Week 3)

**Goal**: Implement concrete tools that agents use

### Tasks:

#### 3.1 Tool Registry Pattern

**File: `adapters/tools/registry.go`**
```go
package tools

import (
    "github.com/yourname/codepicker/domain/agent"
)

type Registry struct {
    tools map[string]agent.Tool
}

func NewRegistry(fs FileSystem, shell ShellExecutor) *Registry {
    r := &Registry{tools: make(map[string]agent.Tool)}
    
    // Register all tools
    r.Register(&ReadFileTool{fs: fs})
    r.Register(&WriteFileTool{fs: fs})
    r.Register(&SearchCodeTool{fs: fs})
    r.Register(&ShellTool{shell: shell})
    
    return r
}

func (r *Registry) Register(tool agent.Tool) {
    r.tools[tool.Name()] = tool
}

func (r *Registry) GetTool(name string) (agent.Tool, error) {
    tool, ok := r.tools[name]
    if !ok {
        return nil, fmt.Errorf("tool not found: %s", name)
    }
    return tool, nil
}

func (r *Registry) All() []agent.Tool {
    tools := make([]agent.Tool, 0, len(r.tools))
    for _, t := range r.tools {
        tools = append(tools, t)
    }
    return tools
}
```

#### 3.2 Implement Core Tools

**File: `adapters/tools/read.go`**
```go
package tools

type ReadFileTool struct {
    fs FileSystem
}

func (t *ReadFileTool) Name() string { return "read_file" }

func (t *ReadFileTool) Description() string {
    return "Read the contents of a file from the project"
}

func (t *ReadFileTool) Capabilities() []agent.Capability {
    return []agent.Capability{agent.CapRead}
}

func (t *ReadFileTool) Execute(ctx context.Context, argsJSON json.RawMessage) (string, error) {
    var args struct {
        Path string `json:"path"`
    }
    if err := json.Unmarshal(argsJSON, &args); err != nil {
        return "", err
    }
    
    content, err := t.fs.ReadFile(args.Path)
    if err != nil {
        return "", fmt.Errorf("failed to read %s: %w", args.Path, err)
    }
    
    return string(content), nil
}
```

**File: `adapters/tools/write.go`** (similar pattern)
**File: `adapters/tools/search.go`** (grep-based implementation)
**File: `adapters/tools/shell.go`** (delegates to infra/shell)

#### 3.3 Acceptance Criteria
- [ ] 5 core tools implemented: read, write, search, shell, list_files
- [ ] Each tool has unit tests with mocked dependencies
- [ ] Registry correctly maps tool names to implementations

---

## **PHASE 4: Policy & Security** (Week 3)

**Goal**: Single, clear security enforcement point

### Tasks:

#### 4.1 Policy Implementation

**File: `adapters/policy/enforcer.go`**
```go
package policy

import (
    "github.com/yourname/codepicker/domain/agent"
)

type Enforcer struct {
    mode       Mode
    allowShell bool
    allowWrite bool
}

type Mode string

const (
    ModeInteractive Mode = "interactive"  // Prompt user
    ModeBatch      Mode = "batch"         // Deny risky ops
    ModeReadOnly   Mode = "readonly"      // No writes/shell
)

func NewEnforcer(mode Mode) *Enforcer {
    switch mode {
    case ModeInteractive:
        return &Enforcer{mode: mode, allowShell: true, allowWrite: true}
    case ModeBatch:
        return &Enforcer{mode: mode, allowShell: false, allowWrite: true}
    case ModeReadOnly:
        return &Enforcer{mode: mode, allowShell: false, allowWrite: false}
    default:
        panic("invalid policy mode")
    }
}

func (e *Enforcer) Authorize(tool agent.Tool, args json.RawMessage) error {
    caps := tool.Capabilities()
    
    for _, cap := range caps {
        switch cap {
        case agent.CapExecute:
            if !e.allowShell {
                return fmt.Errorf("policy denies shell execution")
            }
            if e.mode == ModeInteractive {
                // Prompt user (implement separately)
                if !e.promptUser(tool, args) {
                    return fmt.Errorf("user denied execution")
                }
            }
            
        case agent.CapWrite:
            if !e.allowWrite {
                return fmt.Errorf("policy denies file writes")
            }
        }
    }
    
    return nil
}
```

#### 4.2 Acceptance Criteria
- [ ] Policy correctly blocks shell in batch mode
- [ ] Policy allows all tools in interactive mode
- [ ] Policy denies writes in readonly mode
- [ ] User prompts work in interactive mode

---

## **PHASE 5: Agent Implementation** (Week 4)

**Goal**: Core agent loop with tool calling

### Tasks:

#### 5.1 Agent Implementation

**File: `adapters/agent/simple_agent.go`**
```go
package agent

type SimpleAgent struct {
    llm      LLMClient
    tools    ToolRegistry
    policy   Policy
    repo     Repository
    model    string
    maxTurns int
}

func NewSimpleAgent(
    llm LLMClient,
    tools ToolRegistry,
    policy Policy,
    repo Repository,
    model string,
) *SimpleAgent {
    return &SimpleAgent{
        llm:      llm,
        tools:    tools,
        policy:   policy,
        repo:     repo,
        model:    model,
        maxTurns: 30,
    }
}

func (a *SimpleAgent) Execute(ctx context.Context, task *Task) (*Result, error) {
    exec := &Execution{
        ID:    uuid.New().String(),
        Task:  task.Description,
        State: StatePending,
    }
    
    messages := []Message{
        {Role: "system", Content: systemPrompt},
        {Role: "user", Content: task.Description},
    }
    
    for turn := 0; turn < a.maxTurns; turn++ {
        // Call LLM
        resp, err := a.llm.Complete(ctx, Request{
            Model:    a.model,
            Messages: messages,
            Tools:    a.toolDefinitions(),
        })
        if err != nil {
            return nil, err
        }
        
        // Track cost
        exec.Cost.PromptTokens += resp.Usage.PromptTokens
        exec.Cost.CompletionTokens += resp.Usage.CompletionTokens
        
        msg := resp.Choices[0].Message
        messages = append(messages, msg)
        
        // If no tool calls, we're done
        if len(msg.ToolCalls) == 0 {
            exec.State = StateCompleted
            a.repo.SaveExecution(exec)
            return &Result{Content: msg.Content}, nil
        }
        
        // Execute tools
        for _, tc := range msg.ToolCalls {
            tool, err := a.tools.GetTool(tc.Function.Name)
            if err != nil {
                return nil, err
            }
            
            // Policy check
            if err := a.policy.Authorize(tool, tc.Function.Arguments); err != nil {
                return nil, fmt.Errorf("policy denied %s: %w", tc.Function.Name, err)
            }
            
            // Execute
            result, err := tool.Execute(ctx, tc.Function.Arguments)
            if err != nil {
                result = fmt.Sprintf("Error: %v", err)
            }
            
            messages = append(messages, Message{
                Role:       "tool",
                ToolCallID: tc.ID,
                Content:    result,
            })
        }
    }
    
    return nil, fmt.Errorf("exceeded max turns")
}
```

#### 5.2 Acceptance Criteria
- [ ] Agent successfully executes simple tasks
- [ ] Tool calls work end-to-end
- [ ] Policy correctly gates dangerous operations
- [ ] Cost tracking accumulates correctly

---

## **PHASE 6: Plan Execution** (Week 5)

**Goal**: Multi-step task execution with checkpointing

### Tasks:

#### 6.1 Planner

**File: `adapters/agent/planner.go`**
```go
package agent

type Planner struct {
    llm   LLMClient
    model string
}

func (p *Planner) CreatePlan(ctx context.Context, task string) (*Plan, error) {
    prompt := fmt.Sprintf(`Break this task into 3-7 sequential steps:
Task: %s

Return JSON: {"steps": [{"description": "...", "instruction": "...", "files": [...]}]}`, task)
    
    resp, err := p.llm.Complete(ctx, Request{
        Model: p.model,
        Messages: []Message{
            {Role: "system", Content: plannerPrompt},
            {Role: "user", Content: prompt},
        },
        ResponseFormat: &ResponseFormat{Type: "json_object"},
    })
    
    var planData struct {
        Steps []Step `json:"steps"`
    }
    json.Unmarshal([]byte(resp.Choices[0].Message.Content), &planData)
    
    return &Plan{
        ID:    uuid.New().String(),
        Steps: planData.Steps,
    }, nil
}
```

#### 6.2 Plan Executor

**File: `adapters/agent/executor.go`**
```go
package agent

type PlanExecutor struct {
    agent Agent
    repo  Repository
}

func (e *PlanExecutor) Execute(ctx context.Context, plan *Plan) error {
    exec := &Execution{
        ID:    uuid.New().String(),
        Plan:  plan,
        State: StateRunning,
    }
    
    for i, step := range plan.Steps {
        if step.Status == StepCompleted {
            continue // Resume from checkpoint
        }
        
        step.Status = StepRunning
        
        task := &Task{
            Description: step.Instruction,
            Context:     step.Files,
        }
        
        result, err := e.agent.Execute(ctx, task)
        if err != nil {
            step.Status = StepFailed
            step.Result = err.Error()
            e.repo.SaveExecution(exec)
            return err
        }
        
        step.Status = StepCompleted
        step.Result = result.Content
        
        // Checkpoint after each step
        e.repo.SaveCheckpoint(&Checkpoint{
            ExecutionID: exec.ID,
            Snapshot:    exec,
        })
    }
    
    exec.State = StateCompleted
    e.repo.SaveExecution(exec)
    return nil
}
```

#### 6.3 Acceptance Criteria
- [ ] Planner generates valid multi-step plans
- [ ] Executor runs plans step-by-step
- [ ] Checkpoints saved after each step
- [ ] Resume works correctly from checkpoint

---

## **PHASE 7: CLI & Integration** (Week 6)

**Goal**: Wire everything together, expose via CLI

### Tasks:

#### 7.1 Composition Root

**File: `app/runtime.go`**
```go
package app

type Runtime struct {
    Agent    agent.Agent
    Planner  *agent.Planner
    Executor *agent.PlanExecutor
    Repo     storage.Repository
    Config   *Config
}

func NewRuntime(configPath string) (*Runtime, error) {
    // Load config
    cfg, err := LoadConfig(configPath)
    if err != nil {
        return nil, err
    }
    
    // Create infrastructure
    llmClient := llm.NewClient(cfg.APIKey)
    repo, err := storage.NewRepository(cfg.DBPath)
    if err != nil {
        return nil, err
    }
    shadowFS, err := storage.NewShadowFS(cfg.ProjectRoot)
    if err != nil {
        return nil, err
    }
    shellExec := shell.NewExecutor()
    
    // Create adapters
    toolRegistry := tools.NewRegistry(shadowFS, shellExec)
    policyEnforcer := policy.NewEnforcer(cfg.PolicyMode)
    
    // Create agent
    agentImpl := agent.NewSimpleAgent(
        llmClient,
        toolRegistry,
        policyEnforcer,
        repo,
        cfg.Model,
    )
    
    // Create planner & executor
    planner := agent.NewPlanner(llmClient, cfg.Model)
    executor := agent.NewPlanExecutor(agentImpl, repo)
    
    return &Runtime{
        Agent:    agentImpl,
        Planner:  planner,
        Executor: executor,
        Repo:     repo,
        Config:   cfg,
    }, nil
}
```

#### 7.2 CLI Commands

**File: `cmd/agent.go`**
```go
package cmd

var agentCmd = &cobra.Command{
    Use:   "agent",
    Short: "Run AI agent tasks",
}

var runCmd = &cobra.Command{
    Use:   "run [task]",
    Short: "Execute a task",
    Args:  cobra.MinimumNArgs(1),
    RunE: func(cmd *cobra.Command, args []string) error {
        rt, err := app.NewRuntime(".codepicker.yml")
        if err != nil {
            return err
        }
        
        task := strings.Join(args, " ")
        
        result, err := rt.Agent.Execute(cmd.Context(), &task.Task{
            Description: task,
        })
        if err != nil {
            return err
        }
        
        fmt.Println(result.Content)
        return nil
    },
}

var planCmd = &cobra.Command{
    Use:   "plan [task]",
    Short: "Generate execution plan",
    Args:  cobra.MinimumNArgs(1),
    RunE: func(cmd *cobra.Command, args []string) error {
        rt, err := app.NewRuntime(".codepicker.yml")
        if err != nil {
            return err
        }
        
        task := strings.Join(args, " ")
        plan, err := rt.Planner.CreatePlan(cmd.Context(), task)
        if err != nil {
            return err
        }
        
        // Print plan as table
        printPlan(plan)
        
        if executeFlag {
            return rt.Executor.Execute(cmd.Context(), plan)
        }
        return nil
    },
}
```

#### 7.3 Acceptance Criteria
- [ ] `codepicker agent run "add logging"` works end-to-end
- [ ] `codepicker agent plan "refactor X"` generates plan
- [ ] `codepicker apply` applies shadow changes to source
- [ ] All commands handle errors gracefully

---

## **PHASE 8: Observability & Polish** (Week 7)

**Goal**: Production-ready logging, metrics, error handling

### Tasks:

#### 8.1 Structured Logging

Replace all `fmt.Printf` with:
```go
logger.Info("agent.turn.complete",
    "turn", turnNum,
    "tool", toolName,
    "cost_usd", cost,
    "duration_ms", elapsed.Milliseconds(),
)
```

#### 8.2 Metrics
```go
type Metrics struct {
    TasksCompleted   int
    ToolCallsTotal   int
    PolicyDenials    int
    AverageCostUSD   float64
}

func (m *Metrics) Export() map[string]interface{} {
    return map[string]interface{}{
        "tasks_completed": m.TasksCompleted,
        "tools_called":    m.ToolCallsTotal,
        // ...
    }
}
```

#### 8.3 Error Wrapping
```go
if err != nil {
    return fmt.Errorf("failed to execute step %d: %w", stepID, err)
}
```

#### 8.4 Acceptance Criteria
- [ ] All logs are structured JSON (use `slog`)
- [ ] Errors include full context chain
- [ ] Metrics exported to `~/.codepicker/metrics.json`

---

## **PHASE 9: Testing & Documentation** (Week 8)

### Tasks:

#### 9.1 Integration Tests
```go
func TestAgentExecutesSimpleTask(t *testing.T) {
    rt := setupTestRuntime(t)
    
    result, err := rt.Agent.Execute(context.Background(), &task.Task{
        Description: "Create a hello.txt file with 'Hello World'",
    })

    require.NoError(t, err)
    
    content, err := os.ReadFile(".codepicker/shadow/hello.txt")
    require.NoError(t, err)
    assert.Contains(t, string(content), "Hello World")
}
```

#### 9.2 Security Tests
```go
func TestBatchModeDeniesShell(t *testing.T) {
    rt := setupTestRuntime(t, WithPolicy(policy.ModeBatch))
    
    _, err := rt.Agent.Execute(context.Background(), &task.Task{
        Description: "Run 'rm -rf /' command",
    })
    
    assert.Error(t, err)
    assert.Contains(t, err.Error(), "policy denies")
}
```

#### 9.3 Documentation
- `README.md`: Quick start, architecture diagram
- `ARCHITECTURE.md`: Explain hexagonal design, dependency flow
- `CONTRIBUTING.md`: How to add new tools

---

## Success Metrics

After all phases:

1. **Lines of Code**: ~2000 (down from 20,000)
2. **Test Coverage**: >80% for domain + adapters
3. **Build Time**: <5 seconds
4. **Dependency Graph**: No cycles, clear layers
5. **Performance**: Agent executes simple tasks in <10s

---

## Implementation Guidelines

### Do:
- ✅ Write tests BEFORE implementation
- ✅ Keep functions under 50 lines
- ✅ Use constructor injection for dependencies
- ✅ Return errors, don't panic
- ✅ Use interfaces for external dependencies

### Don't:
- ❌ Skip phases (each builds on previous)
- ❌ Add features not in spec
- ❌ Use global variables or init()
- ❌ Mix business logic with infrastructure
- ❌ Store JSON in SQL columns (use proper tables)

---

## Deliverables Per Phase

Each phase should produce:
1. **Code**: Fully working implementation
2. **Tests**: Unit tests for all new code
3. **Migration Guide**: How to port old features if needed
4. **Documentation**: Update architecture docs

---

## Emergency Escape Hatches

If you get stuck:

1. **Can't delete old code yet?** Create `legacy/` package, isolate it
2. **Phase taking too long?** Split into smaller sub-phases
3. **Integration failing?** Add adapter layer to bridge old/new
4. **Tests too hard?** You're coupling too tightly, add interfaces

---

## Final Notes for Gemini

- **Think before coding**: Design the interface signature before implementation
- **Favor composition**: Small focused types over large ones
- **Be boring**: Use standard library, avoid clever tricks
- **Ask questions**: If requirements unclear, propose alternatives
- **Incremental is key**: Every commit should compile and pass tests

**Start with Phase 1, Task 1.1. Show me the files you're deleting and the new package structure you're creating. Get approval before proceeding to implementation.**
