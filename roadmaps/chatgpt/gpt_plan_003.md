Got it 👍
Below is a **focused, technical review** of the codebase with a **prioritized roadmap**. I’ll concentrate on **logic cleanup, architectural weaknesses, security posture, UX, and feature evolution**, grounded in Go CLI best-practices and agent safety.

---

## High-Level Assessment

**Strengths**

* Clear separation between CLI (`cmd/`) and core logic (`internal/`)
* Strong safety model: shadow filesystem, sentinel, policies
* Well-designed agent lifecycle (Supervisor → Planner → Executor → Worker)
* Good use of Cobra, database-backed state, and explicit policies
* Thoughtful UX details (diff preview, interactive approvals, dry-run)

**Main Issues**

* Some **logic duplication and responsibility leakage**
* **State and config flow is implicit**, sometimes fragile
* **Policy enforcement is scattered**
* UX can feel verbose and inconsistent across commands
* Several features are “almost there” but need tightening to feel solid

---

## 1. Code Logic & Architecture Cleanup (Highest Priority)

### 1.1 Engine Responsibility Creep

`internal/agent/engine.go` is doing too much:

* Policy enforcement
* Approval UI
* Worker orchestration
* Tool execution wiring
* Cost tracking
* Prompt management

**Suggestion**
Split into:

* `Engine` → orchestration + lifecycle
* `PolicyEnforcer` → command validation + approval
* `PromptManager` → system + worker prompts
* `WorkerRunner` → isolated worker execution logic

This reduces future risk when adding:

* non-interactive UIs
* remote execution
* web-based approval

---

### 1.2 Policy Enforcement Is Spread Out

Policy logic currently exists in:

* `Engine.SetPolicy`
* `Sentinel`
* `ToolExecutor`
* CLI-level mode decisions

This creates subtle inconsistencies.

**Improve by**

* Making **Policy the single source of truth**
* Sentinel only classifies commands
* Engine asks `policy.CanExecute(cmd, context)`
* UI layer decides *how* to ask, not *whether*

---

### 1.3 Shadow vs Source Ambiguity

You correctly prefer shadow files, but logic is duplicated:

* `WorkingMemory.Add`
* Worker reads source directly
* Executor writes to shadow

**Risk**
Worker reads stale source while memory reads shadow → inconsistent context.

**Fix**
Introduce a single abstraction:

```go
type VirtualFS interface {
    Read(path string) ([]byte, error)
    WriteShadow(path string, []byte) error
}
```

Everything (memory, worker, tools) uses it.

---

### 1.4 Config Loading Is Implicit

`config.GetOrLoadConfig("")` silently loads global state.

**Problems**

* Hard to test
* Hard to override
* Hidden coupling

**Fix**

* Load config once in `app.NewAgentContext`
* Pass explicitly everywhere
* Remove hidden globals

---

## 2. UX & CLI Improvements (High Impact, Low Risk)

### 2.1 CLI Output Consistency

Some commands:

* print emojis
* some log via logger
* some use fmt.Println directly

**Suggestion**

* One output abstraction:

  * `UI.Info()`
  * `UI.Warn()`
  * `UI.Table()`
* Emojis enabled via `--fancy`

This will matter if you add:

* JSON output
* TUI / Web UI

---

### 2.2 `apply` Command UX

Current UX is good but can be better.

**Improvements**

* Show file summary before diff:

  ```
  +12 -4 lines
  ```
* Add:

  * `--accept-pattern "*.go"`
  * `--reject-pattern "vendor/*"`
* Remember last choice per file type

---

### 2.3 Agent Feedback Noise

Agent prints *thoughts* inline, which is helpful but noisy.

**Improve**

* Levels:

  * `--agent-verbose`
  * `--agent-thoughts`
* Default: only milestones
* Optional live streaming

---

## 3. Agent System Improvements

### 3.1 Plan → Execution Coupling

Planner outputs steps, executor mutates them in place.

**Problem**

* Hard to replay
* Hard to audit
* Hard to diff runs

**Suggestion**

* Treat plans as immutable
* Execution produces a `Run` record
* Steps update status in run, not plan

This unlocks:

* retries
* resume
* diffing executions

---

### 3.2 Tool Interface Is Stringly-Typed

Tools rely on JSON unmarshalling per call.

**Risk**

* Easy to break
* Hard to refactor

**Improve**

* Define typed tool structs
* Central registry:

```go
type Tool interface {
    Name() string
    Execute(ctx ToolContext) (string, error)
}
```

---

### 3.3 Worker Context Explosion

Worker prompt concatenates raw files.

**Issues**

* Token inefficiency
* No relevance ranking

**Improve**

* File chunking
* Summarize large files
* Rank by step relevance
* Cache embeddings later (optional)

---

## 4. Security & Safety Hardening

### 4.1 Shell Command Review

Good start with Sentinel, but:

* Reason strings are free-form
* Approval UI is embedded in engine

**Improve**

* Structured risk levels:

  * `Low`, `Medium`, `High`, `Critical`
* Require explicit user confirmation for Critical
* Log all approvals with hash

---

### 4.2 Path Safety

`paths.Sanitize` helps, but:

* Shadow paths and worker reads should be strictly jailed
* Prevent `../` escapes even inside shadow

---

### 4.3 Secrets Handling

Right now:

* API keys via env only
* Agent could accidentally print secrets

**Add**

* Secret masking in logs
* `.codepicker/secrets.ignore`
* Automatic redaction patterns

---

## 5. Feature Roadmap (Strategic)

### Phase 1 – Stabilization (Now)

* Engine responsibility split
* Unified VirtualFS
* Explicit config propagation
* Policy centralization

### Phase 2 – UX & Trust

* Better apply UX
* Output abstraction
* Cleaner agent verbosity controls
* Execution/run history

### Phase 3 – Power Features

* Resume interrupted plans
* Step retry
* Dry-run agent execution
* File-level change attribution

### Phase 4 – Advanced

* Embedding-backed context selection
* Web dashboard
* Remote agent runners
* GitHub PR integration

---

## Final Take

This is **already a strong, well-thought-out system**.
Your biggest risks are not features — they’re **implicit coupling and growing complexity**.

If you clean up:

* Engine responsibilities
* Policy flow
* Shadow/source abstraction

…you’ll have a **very solid foundation** for turning this into a serious long-lived tool rather than a clever prototype.

If you want, next we can:

* Do a **file-by-file refactor plan**
* Design a **VirtualFS interface**
* Or draft a **v2 architecture diagram**

