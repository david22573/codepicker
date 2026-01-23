# Codepicker Architecture

**Version**: 1.0  
**Last Updated**: January 2026  
**Purpose**: This document describes the architecture of Codepicker and serves as a guide for AI agents to understand and improve the codebase.

---

## Table of Contents

1. [Overview](#overview)
2. [Core Concepts](#core-concepts)
3. [System Architecture](#system-architecture)
4. [Component Map](#component-map)
5. [Data Flow](#data-flow)
6. [Key Subsystems](#key-subsystems)
7. [Known Limitations & Improvement Opportunities](#known-limitations--improvement-opportunities)
8. [Development Guidelines](#development-guidelines)

---

## Overview

Codepicker is an **AI-powered development assistant** that:
- Generates semantic codebase context for LLMs
- Executes autonomous coding tasks via multi-agent systems
- Provides safe, policy-controlled file modifications through a "shadow filesystem"
- Integrates with external tools via MCP (Model Context Protocol)

### Design Philosophy

- **Safety First**: Shadow filesystem + atomic backups prevent data loss
- **Policy-Driven**: Execution policies control what agents can do (Interactive, Batch, Architect modes)
- **Extensible**: Tool registry, MCP integration, custom tools
- **Transparent**: All changes reviewable before application

---

## Core Concepts

### 1. Shadow Filesystem
**Location**: `internal/shadow/`

Changes are written to `.codepicker/shadow/` directory, not directly to source files.

```
src/
├── main.go               # Original file
└── .codepicker/
    ├── shadow/
    │   └── main.go       # Proposed changes (staged here)
    └── backups/
        └── 20260122-143000/
            └── main.go   # Atomic backup before apply
```

**Key Operations**:
- `WriteFile(relPath, content)`: Stage changes
- `ApplyAtomic(relPath)`: Backup original → Apply changes
- `Restore(relPath, backupPath)`: Rollback on failure

### 2. Agent Engine
**Location**: `internal/agent/engine.go`

The brain of the system. Coordinates:
- **Supervisor Model**: Plans and orchestrates (e.g., `deepseek-chat`)
- **Worker Model**: Executes delegated tasks (cheaper model for bulk work)
- **Tool Execution**: Via `ToolExecutor` with policy enforcement
- **Memory Management**: Context window optimization via `WorkingMemory`

**Turn-Based Loop**:
```
1. User provides task
2. Agent thinks (LLM generates response)
3. Agent selects tools to call
4. Tools execute (enforced by policy)
5. Results fed back to agent
6. Repeat until task complete or turn limit
```

### 3. Policy System
**Location**: `internal/policy/`

Controls what agents can do:

| Policy | Shell Access | File Write | Use Case |
|--------|--------------|------------|----------|
| **Interactive** | ✅ (with approval) | ✅ (with approval) | Human-supervised sessions |
| **Batch** | ❌ | ✅ | Headless, CI/CD |
| **Architect** | ❌ | ❌ | Read-only audits |
| **Server** | ❌ | ✅ | Daemon mode |

### 4. Tool Registry
**Location**: `internal/tools/registry.go`

Tools are categorized into sets:
- **ReadOnly**: `read_file`, `search_code`, `list_files`, `scan_package`
- **Standard**: ReadOnly + `write_shadow_file`, `delegate_task`
- **Admin**: Standard + `run_shell`

**Custom Tools** can be defined in `.codepicker.yml`:
```yaml
tools:
  - name: deploy
    description: Deploy to staging
    command: ./scripts/deploy.sh
```

### 5. MCP Integration
**Location**: `internal/mcp/`

Connects to external services (GitHub, databases, APIs) via Model Context Protocol.

Example: GitHub MCP server provides tools like `create_issue`, `list_pull_requests`.

---

## System Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                         CLI Layer                            │
│  (cmd/root.go, cmd/agent.go, cmd/context.go, etc.)         │
└───────────────────────┬─────────────────────────────────────┘
                        │
                        ▼
┌─────────────────────────────────────────────────────────────┐
│                    Application Layer                         │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐      │
│  │ Agent Engine │  │   Scanner    │  │   Server     │      │
│  │ (Planning &  │  │ (Context Gen)│  │  (Daemon)    │      │
│  │  Execution)  │  │              │  │              │      │
│  └──────────────┘  └──────────────┘  └──────────────┘      │
└───────────────────────┬─────────────────────────────────────┘
                        │
                        ▼
┌─────────────────────────────────────────────────────────────┐
│                      Core Services                           │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐      │
│  │   Executor   │  │    Memory    │  │   Enforcer   │      │
│  │  (Tool Exec) │  │  (Context)   │  │  (Policy)    │      │
│  └──────────────┘  └──────────────┘  └──────────────┘      │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐      │
│  │   Sentinel   │  │     VFS      │  │  MCP Manager │      │
│  │  (Security)  │  │  (Overlay)   │  │ (Ext Tools)  │      │
│  └──────────────┘  └──────────────┘  └──────────────┘      │
└───────────────────────┬─────────────────────────────────────┘
                        │
                        ▼
┌─────────────────────────────────────────────────────────────┐
│                   Infrastructure Layer                       │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐      │
│  │   Database   │  │ Shadow FS    │  │  OpenRouter  │      │
│  │  (SQLite)    │  │  Manager     │  │    Client    │      │
│  └──────────────┘  └──────────────┘  └──────────────┘      │
└─────────────────────────────────────────────────────────────┘
```

---

## Component Map

### Entry Points
| Command | File | Purpose |
|---------|------|---------|
| `ctx gen` | `cmd/context.go` | Generate markdown context |
| `agent run` | `cmd/agent.go` | Execute autonomous task |
| `agent plan` | `cmd/agent_plan.go` | Create execution plan |
| `agent improve` | `cmd/agent_improve.go` | Execute from ARCHITECTURE_GOALS.md |
| `apply` | `cmd/apply.go` | Review & apply shadow changes |
| `batch` | `cmd/batch.go` | Background job queue |
| `serve` | `cmd/serve.go` | Start HTTP daemon |

### Core Modules

#### Agent System
| File | Responsibility |
|------|----------------|
| `agent/engine.go` | Main reasoning loop, model coordination |
| `agent/executor.go` | Tool execution with middleware |
| `agent/enforcer.go` | Policy enforcement & approval handling |
| `agent/sentinel.go` | Command safety classification |
| `agent/memory.go` | Working memory (context window) |
| `agent/planner.go` | Multi-step plan generation |
| `agent/plan_executor.go` | Execute saved plans |
| `agent/orchestrator.go` | Multi-agent coordination |
| `agent/worker.go` | Delegated task execution |
| `agent/recovery.go` | Auto-fix common errors (go mod, etc.) |

#### Tools
| File | Tools Provided |
|------|----------------|
| `tools/filesystem.go` | `read_file`, `write_shadow_file` |
| `tools/search.go` | `search_code` |
| `tools/scan.go` | `scan_package` (bulk context) |
| `tools/list_files.go` | `list_files` |
| `tools/run_shell.go` | `run_shell` |
| `tools/delegate.go` | `delegate_task` |
| `tools/skeleton_tool.go` | `read_skeleton` (Go AST parsing) |
| `tools/mcp_bridge.go` | MCP tool adapter |
| `tools/custom.go` | User-defined custom tools |

#### Storage
| File | Purpose |
|------|---------|
| `database/store.go` | SQLite persistence (memory, plans, history) |
| `database/schema.go` | Migration system |
| `shadow/fs.go` | Shadow filesystem manager |
| `vfs/vfs.go` | Virtual FS overlay (shadow + source) |

#### Context Generation
| File | Purpose |
|------|---------|
| `contextgen/generator.go` | Main context generation logic |
| `contextgen/tree.go` | Directory tree visualization |
| `scanner/scanner.go` | Concurrent file scanning |
| `writer/writer.go` | Output strategies (concat, tree, copy) |
| `minifier/` | Language-specific code minification |

---

## Data Flow

### Context Generation Flow
```
User Command
    ↓
cmd/context.go
    ↓
scanner.Scan() ──→ Parallel Workers
    ↓                    ↓
Filter by            Read Files
.gitignore,              ↓
extensions           Minify (optional)
    ↓                    ↓
writer.Write() ←────  Concatenate
    ↓
Output File (markdown)
```

### Agent Execution Flow
```
User Task
    ↓
cmd/agent.go
    ↓
AgentContext.New() ──→ Initialize:
    │                  - Engine
    │                  - Database
    │                  - Shadow Manager
    │                  - Policy Enforcer
    ↓
Engine.Run() ──→ Loop:
    │            1. Build context (memory + tools)
    │            2. Call LLM
    │            3. Parse tool calls
    │            4. Check policy ──→ Enforcer.AllowTool()
    │            5. Execute ──→ Executor.Execute()
    │            6. Append results to messages
    │            7. Repeat (max 30 turns)
    ↓
Shadow Changes Written
    ↓
User runs: codepicker apply
    ↓
Review TUI ──→ Approve/Reject
    ↓
ShadowManager.ApplyAtomic() ──→ Backup → Write
```

### Plan Execution Flow
```
codepicker agent plan "refactor X"
    ↓
Planner.CreatePlan()
    │
    ├──→ Generate project tree
    ├──→ Call LLM with structured prompt
    └──→ Parse JSON plan { steps: [...] }
    ↓
Store.SavePlan(planID, steps)
    ↓
codepicker agent run --plan <planID>
    ↓
PlanExecutor.Execute()
    │
    └──→ For each step:
         ├──→ Load step.Files into memory
         ├──→ Engine.Run(step.Instruction)
         ├──→ Retry on failure (with context)
         └──→ Mark complete
    ↓
All steps done → Apply changes
```

---

## Key Subsystems

### 1. Memory Management
**File**: `internal/database/store.go`

**Token Budget**: 100,000 tokens max

**Strategy**:
- Files are added to `memory_files` table via `read_file` tool
- LRU eviction when over budget
- Content hashing to avoid redundant re-reads
- Snapshot/Restore for plan execution rollback

**Issue**: Large codebases exceed limit quickly.

**Improvement Opportunity**:
```go
// Add semantic relevance scoring
type MemoryEntry struct {
    Path          string
    Content       string
    TokenCount    int
    RelevanceScore float64  // NEW: Cosine similarity to current task
    LastAccessed  time.Time
    Priority      int        // NEW: Manual boost for critical files
}

// Evict low-relevance files first, not just oldest
```

### 2. Cost Tracking
**File**: `internal/tracking/costs.go`

Tracks token usage and estimates cost:
```go
inputCost  = (promptTokens / 1M) * $5.00
outputCost = (completionTokens / 1M) * $15.00
```

**Daily Limit**: $5.00 (configurable via `DAILY_COST_LIMIT`)

**Issue**: No per-session limits, only daily.

**Improvement Opportunity**:
```go
type CostTracker struct {
    sessionCost  float64  // NEW: Track current session
    dailyCost    float64
    sessionLimit float64  // NEW: Warn at 80%
}
```

### 3. Security Sentinel
**File**: `internal/agent/sentinel.go`

Classifies commands into:
- `read-only`: ls, cat, grep
- `filesystem-write`: mv, rm, chmod
- `network`: curl, wget
- `dangerous`: rm -rf, eval, dd

**Command Validation**:
```go
func (s *Sentinel) CheckCommand(cmdStr string) (bool, string, string, []string)
```

**Improvement Opportunity**: Add ML-based anomaly detection for unusual command patterns.

### 4. Plan System
**Files**: `agent/planner.go`, `agent/plan_executor.go`

Plans are JSON structures:
```json
{
  "id": "uuid",
  "reasoning": "Why this approach",
  "steps": [
    {
      "id": 1,
      "description": "Add User interface",
      "instruction": "Create internal/user.go with...",
      "files": ["internal/user.go"],
      "critical": true
    }
  ],
  "estimated_cost": 0.15
}
```

Stored in SQLite, resumable via `--plan <id>`.

### 5. Orchestrator (Multi-Agent)
**File**: `agent/orchestrator.go`

Coordinates specialist agents:
- **ContextSpecialist**: Finding files
- **CodeModifier**: Writing code
- **SystemAgent**: Running tests
- **QualityAgent**: Linting, review

**Current Issue**: Not fully wired up (prototype).

**Improvement Path**: Wire to main `agent run` flow with flag `--orchestrate`.

---

## Known Limitations & Improvement Opportunities

### Critical Issues

#### 1. Race Conditions (PRIORITY: HIGH)
**Files**: `internal/shadow/fs.go`, `internal/database/store.go`

**Problem**: Concurrent writes to manifest/database without locks.

**Task for Agent**:
```
Add sync.RWMutex to:
- shadow.Manager (protect Manifest map)
- database.Store (protect concurrent transactions)

Files to modify:
- internal/shadow/fs.go
- internal/database/store.go
```

#### 2. Memory Leaks in Long Sessions (PRIORITY: MEDIUM)
**File**: `internal/agent/engine.go`

**Problem**: `messages` slice grows unbounded in `Run()` loop.

**Task for Agent**:
```
Implement sliding window in Engine.Run():
- Keep system prompt + last 50 messages
- Prune older messages when len > 100

File to modify:
- internal/agent/engine.go (Run function)
```

#### 3. Context Window Overflow (PRIORITY: HIGH)
**File**: `internal/database/store.go`

**Problem**: Simple LRU eviction loses important context.

**Task for Agent**:
```
Enhance GetWorkingMemory() with:
1. Relevance scoring (files matching current task keywords)
2. Recency boost (recently written shadow files)
3. Explicit priority (files agent explicitly read)

Files to modify:
- internal/database/store.go
- internal/agent/memory.go
```

### UX Improvements

#### 4. Progress Visibility (PRIORITY: MEDIUM)
**Files**: `internal/agent/plan_executor.go`, `internal/ui/`

**Task for Agent**:
```
Add progress indicators:
1. Create ui.ProgressBar component
2. Integrate into PlanExecutor.Execute()
3. Show: "Step 3/10: Refactoring database layer [=====>    ] 30%"

Files to create/modify:
- internal/ui/progress.go (NEW)
- internal/agent/plan_executor.go
```

#### 5. Cost Warnings (PRIORITY: LOW)
**File**: `cmd/agent.go`

**Task for Agent**:
```
Before executing expensive operations:
1. Estimate cost using Planner.EstimatePlanCost()
2. Prompt user if > $0.50
3. Show running cost every 5 turns

Files to modify:
- cmd/agent.go (runStandardPlan, runOrchestrator)
- internal/agent/planner.go (add EstimatePlanCost method)
```

#### 6. Better Diff Viewer (PRIORITY: LOW)
**File**: `internal/tui/review.go`

**Task for Agent**:
```
Enhance colorizeDiff():
1. Add syntax highlighting via chroma library
2. Implement side-by-side view toggle
3. Add jump-to-change navigation (n/p keys)

Files to modify:
- internal/tui/review.go
- go.mod (add github.com/alecthomas/chroma)
```

### Architecture Improvements

#### 7. Checkpoint System (PRIORITY: HIGH)
**Files**: `internal/agent/engine.go`, `internal/database/store.go`

**Task for Agent**:
```
Implement resumable sessions:
1. Save checkpoint every 5 turns: {stepID, memory snapshot, messages}
2. On crash, detect incomplete session
3. Prompt to resume from last checkpoint

Files to create/modify:
- internal/agent/checkpoint.go (NEW)
- internal/database/schema.go (add checkpoints table)
- cmd/agent.go (add --resume flag)
```

#### 8. Streaming Context (PRIORITY: MEDIUM)
**File**: `internal/agent/engine.go`

**Task for Agent**:
```
Instead of full context each turn, send deltas:
1. Track what changed: ContextDelta{Added, Removed, Updated}
2. Build incremental context string
3. Reduces token usage by ~40%

Files to modify:
- internal/agent/memory.go
- internal/agent/engine.go (Run function)
```

### Testing Gaps

#### 9. Critical Test Coverage (PRIORITY: HIGH)
**Task for Agent**:
```
Add tests for:
1. Concurrent shadow file writes (shadow/fs_test.go)
2. Context overflow scenarios (database/store_test.go)
3. Malformed LLM responses (agent/executor_test.go)
4. Network interruption during streaming (pkg/openrouter/client_test.go)

Files to create:
- internal/shadow/fs_test.go (concurrency tests)
- internal/database/store_test.go (overflow tests)
- internal/agent/executor_test.go (malformed JSON tests)
- pkg/openrouter/client_test.go (retry tests)
```

---

## Development Guidelines

### For AI Agents Working on This Codebase

#### Step 1: Understand the Component
```bash
# Use scan_package to understand a module
codepicker agent run "Use scan_package on internal/agent to understand the agent system"
```

#### Step 2: Search for Related Code
```bash
# Find all usages of a function/type
codepicker agent run "Search for all usages of 'Engine.Run' to understand the execution flow"
```

#### Step 3: Make Targeted Changes
```bash
# Focus on specific files
codepicker agent run "Add mutex protection to shadow.Manager in internal/shadow/fs.go"
```

#### Step 4: Test Changes
```bash
# Run existing tests
go test ./internal/shadow/... -v

# Or ask agent to write tests
codepicker agent run "Write unit tests for the new mutex locks in shadow.Manager"
```

#### Step 5: Review & Apply
```bash
# Review changes in TUI
codepicker apply -t

# Or batch apply
codepicker apply --yes
```

### Code Style Guidelines

1. **Error Handling**: Always wrap errors with context
   ```go
   return fmt.Errorf("failed to write shadow file: %w", err)
   ```

2. **Logging**: Use structured logging
   ```go
   log.Info(fmt.Sprintf("Starting scan of %s", path))
   ```

3. **Concurrency**: Always protect shared state
   ```go
   m.mu.Lock()
   defer m.mu.Unlock()
   ```

4. **Comments**: Explain WHY, not WHAT
   ```go
   // Prefill forces the model to start with specific text, preventing refusals
   req.Prefill = "```go\n"
   ```

### Testing Strategy

- **Unit Tests**: Pure functions, utilities (minifier, tokenizer)
- **Integration Tests**: Agent execution, tool calling
- **E2E Tests**: Full workflows (plan → execute → apply)

### Performance Targets

- Context generation: < 5 seconds for 1000 files
- Agent turn latency: < 10 seconds (excluding LLM call)
- Memory footprint: < 500MB for typical sessions

---

## Self-Improvement Workflow

### Using Codepicker to Improve Itself

#### Option 1: Architect Mode (Discovery)
```bash
# Generate improvement plan
codepicker agent plan --architect

# Review the generated ARCHITECTURE_GOALS.md
cat ARCHITECTURE_GOALS.md

# Execute improvements one by one
codepicker agent improve --loop
```

#### Option 2: Direct Task Execution
```bash
# Fix a specific issue
codepicker agent run "Add mutex protection to shadow.Manager to fix race conditions"

# Review changes
codepicker apply -t

# Apply if looks good
codepicker apply --yes
```

#### Option 3: Batch Queue (Background)
```bash
# Queue multiple improvements
codepicker batch add "Add progress bar to plan executor"
codepicker batch add "Implement checkpoint system"
codepicker batch add "Write tests for concurrent shadow writes"

# Run queue
codepicker batch run --concurrent 1
```

### Iterative Improvement Pattern

1. **Audit**: `codepicker agent plan --architect`
2. **Prioritize**: Edit `ARCHITECTURE_GOALS.md` manually
3. **Execute**: `codepicker agent improve --loop`
4. **Test**: `go test ./...`
5. **Apply**: `codepicker apply`
6. **Commit**: `git add . && git commit -m "AI improvements"`
7. **Repeat**: Go to step 1

---

## Metrics & Monitoring

### Key Metrics (Not Yet Implemented)

**Task for Agent**:
```
Add metrics collection:
1. Agent success rate (completed vs failed tasks)
2. Average turns per task
3. Tool usage frequency
4. Cost per task type
5. Shadow apply rate (% of changes accepted)

Files to create:
- internal/metrics/collector.go
- internal/metrics/reporter.go
```

### Logging Levels

- **ERROR**: Failures that prevent operation
- **WARN**: Recoverable issues (file not found, etc.)
- **INFO**: Normal operation (file scanned, tool executed)
- **DEBUG**: Detailed internals (turn-by-turn, tool args)

Enable debug: `codepicker agent run --verbose "your task"`

---

## FAQ for AI Agents

**Q: Where do I write code changes?**  
A: Always use `write_shadow_file`. Never modify source directly.

**Q: How do I understand a large module?**  
A: Use `scan_package` tool. Example: `scan_package(target_dir="internal/agent")`

**Q: I need to run tests. Can I?**  
A: Yes, if policy allows. Use `run_shell(command="go test ./...")`. Batch mode blocks this.

**Q: How do I add a file to context?**  
A: Use `read_file(path="...")`. It auto-adds to working memory.

**Q: I'm stuck in a loop, calling the same tool repeatedly.**  
A: The engine detects this and will warn you. Try a different approach or use `scan_package` for bulk context.

**Q: Can I modify this ARCHITECTURE.md file?**  
A: Yes! Keep it updated as you improve the system.

---

## Project Structure Reference

```
codepicker/
├── cmd/                    # CLI commands
│   ├── root.go            # Main entrypoint
│   ├── agent.go           # Agent subcommands
│   ├── context.go         # Context generation
│   ├── apply.go           # Review & apply changes
│   └── ...
├── internal/
│   ├── agent/             # 🧠 Core agent system
│   │   ├── engine.go      # Main reasoning loop
│   │   ├── executor.go    # Tool execution
│   │   ├── enforcer.go    # Policy enforcement
│   │   └── ...
│   ├── tools/             # 🔧 Agent tools
│   ├── database/          # 💾 SQLite persistence
│   ├── shadow/            # 📂 Shadow filesystem
│   ├── vfs/               # 🗂️  Virtual FS overlay
│   ├── policy/            # 🛡️  Security policies
│   ├── contextgen/        # 📄 Context generation
│   ├── scanner/           # 🔍 File scanning
│   ├── mcp/               # 🔌 External integrations
│   ├── ui/                # 🎨 User interface
│   └── ...
├── pkg/
│   └── openrouter/        # 🤖 LLM API client
├── .codepicker.yml        # Configuration
└── main.go                # Entry point
```

---

## Conclusion

This architecture is designed to be **self-improving**. The agent can read this document, understand the system, and make improvements autonomously.

**Next Steps**:
1. Run `codepicker agent plan --architect` to generate an improvement roadmap
2. Execute tasks with `codepicker agent improve --loop`
3. Review changes with `codepicker apply -t`
4. Iterate and improve this document as you learn

**Remember**: The shadow filesystem protects you. All changes are staged and reviewable before applying. Experiment boldly! 🚀

---

**Last Revised By**: Claude (Anthropic)  
**Revision Date**: January 22, 2026  
**Agent Prompt**: "Analyze the codepicker codebase and write a comprehensive ARCHITECTURE.md that enables the tool to improve itself."
