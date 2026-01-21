# Codepicker Stabilization & Hardening Roadmap

## Phase 0 — Guardrails (½–1 day)

**Goal:** Prevent accidental data loss and undefined behavior.

### 0.1 Fix overwrite protection (blocking bug)

* Implement explicit `os.Stat` check in `runContextScan`
* Add unit test:

  * existing file + no `--yes` → error
  * existing file + `--yes` → success

📍 Files:

* `cmd/context.go`

---

### 0.2 Nil-safe lifecycle handling

* Guard `defer w.Close()`
* Add `Close()` no-op implementations if needed

📍 Files:

* `cmd/context.go`
* `internal/writer/*`

---

### 0.3 Fail loudly on invalid glob patterns

* Validate `acceptPattern` / `rejectPattern` once
* Log and disable invalid patterns

📍 Files:

* `cmd/apply.go`

---

## Phase 1 — Policy & Execution Safety (2–3 days)

**Goal:** Make it impossible for tools to bypass policy accidentally.

### 1.1 Centralize approval logic

Create a single approval interface:

```go
type ApprovalRequest struct {
	Tool   string
	Args   string
	Reason string
}
```

* Route **all** approvals through `PolicyEnforcer`
* Remove direct calls from tools to CLI prompts

📍 Files:

* `internal/agent/enforcer.go`
* `internal/tools/*`
* `internal/agent/executor.go`

---

### 1.2 Enforce policy at executor level

Before executing *any* tool:

```go
if !e.RuntimeContext.Enforcer.AllowTool(...) {
	return "Denied by policy"
}
```

This becomes the **last line of defense**.

📍 Files:

* `internal/agent/executor.go`

---

### 1.3 Add argument-level classification

Classify commands into:

* read-only
* filesystem write
* network
* dangerous

Use this classification in approval prompts.

📍 Files:

* `internal/agent/sentinel.go`
* `internal/tools/run_shell.go`

---

## Phase 2 — Agent State & Retry Correctness (2–3 days)

**Goal:** Eliminate “AI hallucinated failure” loops.

### 2.1 Snapshot / restore memory per retry

Before each retry:

* Snapshot working memory
* Restore on failure

📍 Files:

* `internal/agent/plan_executor.go`
* `internal/agent/memory.go`

---

### 2.2 Reset tool messages between retries

Prevent retry attempts from inheriting:

* failed tool outputs
* partial state

📍 Files:

* `internal/agent/engine.go`

---

### 2.3 Deduplicate working memory entries

* Hash file content before storing
* Skip identical content

📍 Files:

* `internal/agent/memory.go`
* `internal/database/*`

---

## Phase 3 — Tool & Engine Refactors (3–4 days)

**Goal:** Reduce duplication and future bug surface.

### 3.1 Extract tool rebuild logic

Create:

```go
func (e *Engine) rebuildTools(toolSet tools.ToolSet)
```

Used by:

* `NewEngine`
* `SetPolicy`

📍 Files:

* `internal/agent/engine.go`

---

### 3.2 Normalize AgentContext creation

Introduce a builder:

```go
func NewDefaultAgentContext(ctx context.Context, opts Options) (*AgentContext, error)
```

Replace duplicated setup in:

* `agent run`
* `agent improve`
* `serve`
* `batch`

📍 Files:

* `internal/app/context.go`
* `cmd/*`

---

### 3.3 Make tools capability-driven

Instead of relying on name checks:

* `ToolCapabilities() []Capability`
* Enforcer decides based on capability

📍 Files:

* `internal/tools/*`
* `internal/agent/enforcer.go`

---

## Phase 4 — Cost & Resource Controls (1–2 days)

**Goal:** Prevent silent overruns and runaway agents.

### 4.1 Enforce cost after each response

* Abort immediately when exceeded
* Surface clear error to user

📍 Files:

* `internal/agent/engine.go`

---

### 4.2 Token & output ceilings per step

* Max tokens per step
* Max tool output per step

📍 Files:

* `internal/agent/sentinel.go`
* `internal/config/limits.go`

---

## Phase 5 — Testing & Observability (ongoing)

**Goal:** Lock in correctness.

### 5.1 Add targeted tests

High ROI tests:

* policy denial paths
* retry recovery
* overwrite protection
* shell command rejection

📍 Files:

* `internal/agent/*_test.go`
* `cmd/*_test.go`

---

### 5.2 Debug modes

Add:

* `--debug-policy`
* `--trace-tools`
* `--trace-memory`

These flags **save hours** when agents misbehave.

---

## Suggested Execution Order (TL;DR)

**Week 1**

* Phase 0
* Phase 1.1–1.2

**Week 2**

* Phase 1.3
* Phase 2

**Week 3**

* Phase 3
* Phase 4

**Ongoing**

* Phase 5

---

## Architectural Outcome If You Finish This

You’ll end up with:

* A **formally enforced execution boundary**
* Deterministic retries
* Predictable policy behavior
* A tool system safe enough to expose via server mode


