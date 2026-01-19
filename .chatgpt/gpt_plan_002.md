## 1. Engine is becoming a God object

`internal/agent/engine.go` currently owns:

* Model selection
* Policy enforcement
* Sentinel execution
* Shadow management
* Memory
* Cost tracking
* Tool dispatch
* Worker orchestration

This will become brittle as features grow.

### Recommended refactor (incremental)

Split `Engine` into **composed services**:

```go
type Engine struct {
    Orchestrator *Orchestrator
    Tools        *ToolRunner
    Safety       *SafetyManager
    Memory       *WorkingMemory
    Shadow       *shadow.Manager
    Policy       policy.ExecutionPolicy
}
```

Where:

* **SafetyManager** wraps Sentinel + Policy
* **ToolRunner** handles tool calls and worker delegation
* **Orchestrator** manages LLM loops, streaming, retries, cost tracking

This keeps `Engine.Run()` readable and testable.

---

## 2. Policy vs Sentinel overlap (important)

Right now:

* **Sentinel** detects dangerous commands
* **Policy** decides whether they are allowed
* **ApprovalCallback** sits in between

This creates **ambiguous authority**.

### Concrete improvement

Make Sentinel **purely diagnostic**, not decision-making:

```go
type CommandRisk struct {
    Severity RiskLevel
    Reason   string
    Binary   string
    Args     []string
}
```

Then Policy decides:

```go
risk := sentinel.Analyze(cmd)
err := policy.Evaluate(risk)
```

Benefits:

* Policies become testable rule sets
* Sentinel remains deterministic
* Easier to add future policies (CI, readonly, audit-only)

---

## 3. Shell execution hardening (security)

You already block most dangerous cases, but a few improvements matter.

### Issues

* `shlex.Split` still allows odd quoting edge cases
* `run_shell` tool is powerful even in Batch mode
* Custom tools bypass Sentinel entirely

### Improvements

#### A. Explicit allowlist by mode

```go
AllowedBinariesByMode map[policy.Level]map[string]bool
```

Example:

* Batch: `go test`, `go vet`, `ls`
* Interactive: limited `git`, `go`, `npm`
* Server: **none**

#### B. Custom tools must go through Sentinel

In `ExecuteCustomTool`:

* Run the command string through Sentinel first
* Enforce policy limits (timeout, output size)

Right now custom tools are a privilege escalation vector.

---

## 4. Shadow filesystem: excellent, but add integrity checks

Shadow FS is one of your best ideas 👍

### Improvements

1. **Hash original file before shadow write**
2. Store hash alongside shadow file
3. Refuse to apply if source changed since shadow creation

This prevents:

* Race conditions
* Silent overwrites
* Conflicts in long-running sessions

Minimal implementation:

* `.codepicker/shadow/.meta.json`

---

## 5. Agent memory vs context generator duplication

You have:

* `contextgen.GenerateTree`
* `scanner + writer`
* `WorkingMemory.FormatContext`

These overlap conceptually.

### Suggested convergence

Create a **single context pipeline** with modes:

```go
ContextProvider {
    Tree()
    Snapshot(files []string)
    Diff(ref string)
}
```

Then:

* CLI context gen
* Agent working memory
* Architect audit

…all use the same source of truth.

This reduces bugs where the agent “sees” a different project than the user.

---

## 6. Planner & Executor: solid, but missing failure semantics

Plans are good, but execution is optimistic.

### Add explicit failure policy

Per step:

```go
OnFailure: Abort | Skip | Continue | Retry(n)
```

Then enforce:

* Critical steps abort immediately
* Non-critical steps continue with warning
* Retry with exponential backoff (cheap for LLM calls)

This makes plans safer and more predictable.

---

## 7. Batch mode is well designed — tighten isolation

Batch mode is already safer than most tools.

### Recommended additions

* Disable `write_shadow_file` unless explicitly allowed
* Enforce max plan steps
* Enforce max tokens per job
* Disallow custom tools in batch entirely

Batch jobs should be *deterministic, auditable, and boring*.

---

## 8. Testing gaps (important)

Good start, but critical paths lack coverage.

### High-value tests to add

1. **Sentinel fuzz tests**

   * Random shell strings
   * Ensure no panics
2. **Policy enforcement tests**

   * Each policy level vs risky commands
3. **Shadow apply tests**

   * File modified after shadow write
4. **Plan executor**

   * Failure propagation
5. **Context diff scan**

   * Git diff edge cases

These tests will catch regressions that humans won’t.

---

## 9. CLI UX polish (minor but worthwhile)

* `agent run` prints “Agent Thought” — consider `--verbose`
* `apply` should show file size + diff stats
* `context gen` silently ignores overwrite case (bug)

This line is a no-op:

```go
if _, err := filepath.Abs(outPath); err == nil && !ctxDryRun && !ctxOverwrite {}
```

That’s a bug worth fixing.
