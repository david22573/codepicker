# Codepicker Multi-Agent Architecture Implementation Summary

## Current State
Codepicker is a Go-based AI coding agent with a monolithic architecture. Single agent handles all tasks: code search, modification, shell execution, and quality checks.

## Proposed Architecture: Multi-Agent by Default

### Core Design Principle
**One Model (deepseek/deepseek-chat), Multiple Specialized Agents**
- All agents use the same LLM model
- Specialization via different system prompts + tool subsets
- No experimental flags - multi-agent is the default

### Five Specialized Agents

```
┌─────────────────────────────────────────┐
│         Orchestrator Agent              │
│  (Planning, delegation, synthesis)      │
└──────────┬──────────────────────────────┘
           │
    ┌──────┴───────┬──────────┬──────────┐
    │              │          │          │
┌───▼────┐  ┌─────▼────┐  ┌──▼───┐  ┌──▼──────┐
│Context │  │Code      │  │System│  │Quality  │
│Agent   │  │Modifier  │  │Agent │  │Agent    │
│(Read)  │  │(Write)   │  │(Run) │  │(Review) │
└────────┘  └──────────┘  └──────┘  └─────────┘
```

**1. Context Agent** (Read-Only)
- Tools: read_file, search_code, analyze_dependencies
- Purpose: Find and analyze code
- Policy: No writes allowed

**2. Code Modifier Agent** (Write-Only to Shadow FS)
- Tools: write_shadow_file, validate_syntax, format_code
- Purpose: Create/edit code safely
- Policy: All changes go to `.codepicker/shadow/`

**3. System Agent** (Command Execution)
- Tools: run_shell, run_tests, check_build
- Purpose: Execute commands, run tests
- Policy: Requires user approval for dangerous commands

**4. Quality Agent** (Review)
- Tools: run_linter, check_coverage, security_scan
- Purpose: Validate changes, ensure quality
- Policy: Read-only, automated checks

**5. Orchestrator Agent** (Coordinator)
- Tools: create_subtask, delegate_to_agent
- Purpose: Break down tasks, coordinate execution
- Policy: No direct code/file access

### Implementation Structure

```go
// Shared by all agents
type BaseAgent struct {
    agentType    AgentType
    client       *openrouter.Client
    model        string  // Same for all: "deepseek/deepseek-chat"
    systemPrompt string  // DIFFERENT per agent
    tools        []Tool  // DIFFERENT subset per agent
    
    // Shared resources
    memory    *WorkingMemory
    shadow    *ShadowManager
    sentinel  *Sentinel
}

// All agents use same Execute() method
func (a *BaseAgent) Execute(ctx, task) (*Result, error) {
    // Standard LLM chat loop with agent-specific tools
}
```

### Execution Flow Example
**User Task:** "Add JWT authentication to the API"

```
1. Orchestrator receives task
   ↓
2. Creates execution plan:
   - Step 1: Context Agent → "Find existing auth patterns"
   - Step 2: Code Modifier → "Create auth middleware"
   - Step 3: Code Modifier → "Add JWT validation"
   - Step 4: System Agent → "Run tests"
   - Step 5: Quality Agent → "Security review"
   ↓
3. Executes in dependency order (parallel where possible)
   ↓
4. Orchestrator synthesizes results
   ↓
5. User reviews changes in shadow filesystem
   ↓
6. User runs `codepicker apply` to accept/reject
```

### Key Technical Components

**Task Decomposition**
```go
type ExecutionPlan struct {
    Steps []PlanStep
    Graph *DependencyGraph  // For parallel execution
}

type PlanStep struct {
    Agent        AgentType
    Task         Task
    Dependencies []string  // Must complete before this
}
```

**Parallel Execution**
- Steps with no dependencies run concurrently
- Wave-based execution (topological sort)
- Shared context passed between steps

**Safety Mechanisms**
1. Shadow filesystem - all code changes isolated
2. Policy enforcement per agent
3. Sentinel for command validation
4. Quality gates before applying changes

### Migration from Current Codebase

**Phase 1: Extract Agent Logic**
- Move from `internal/agent/engine.go` → `internal/agents/base_agent.go`
- Split tool definitions by agent type
- Create agent registry

**Phase 2: Implement Orchestrator**
- Task planner with LLM-based decomposition
- Dependency graph builder
- Parallel executor

**Phase 3: Update CLI**
```go
// OLD: cmd/agent.go
engine.Run(task)

// NEW: cmd/agent.go  
orchestrator := agents.NewOrchestrator(...)
orchestrator.Execute(task)
```

**Phase 4: Backward Compatibility**
- Keep existing `codepicker agent run` command
- Transparently use multi-agent under the hood
- No breaking changes to user interface

### File Structure Changes

```
internal/
├── agent/           # LEGACY (keep for migration)
├── agents/          # NEW multi-agent system
│   ├── base_agent.go
│   ├── prompts.go   # System prompts per agent
│   ├── tools.go     # Tool definitions
│   ├── orchestrator.go
│   ├── planner.go
│   └── executor.go
├── policy/          # EXISTING (reuse)
└── shadow/          # EXISTING (reuse)
```

### Benefits Over Current System

1. **Separation of Concerns**: Each agent has single responsibility
2. **Better Safety**: Read/write permissions enforced at agent level
3. **Parallel Execution**: Independent tasks run concurrently
4. **Easier Testing**: Mock individual agents
5. **Cost Efficiency**: Same model, better specialization
6. **Transparency**: Clear audit trail of which agent did what

### Configuration (`.codepicker.yml`)

```yaml
ai:
  model: deepseek/deepseek-chat  # Shared by all agents
  temperature: 0.0

# Optional: Override model per agent
agents:
  orchestrator:
    model: anthropic/claude-sonnet-4  # Could use smarter model here
  # others use default
```

### Success Metrics
- Task completion rate
- Reduction in human approvals needed
- Code quality scores (linter pass rate)
- Cost per task (token usage)

---

