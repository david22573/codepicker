# Roadmap: High-Productivity Agent UX for Codepicker

## Phase 0 — Define the Target UX (1 day)

Before changing code, lock the behavioral contract.

### Target Interaction Model

| Agent Action         | User Interaction           |
| -------------------- | -------------------------- |
| Code search / read   | **Silent**                 |
| Planning / reasoning | Streamed thoughts          |
| Writing code         | Staged in shadow workspace |
| Applying changes     | Single review flow         |
| Shell / network      | Explicit approval          |
| Destructive ops      | Explicit + confirm         |

This aligns perfectly with how your **shadow filesystem** already works — you’re halfway there.

---

## Phase 1 — Tool Capability Classification (Core Fix) (1–2 days)

### Problem

All tools are treated as “approval required”.

### Solution

Add **capability-based trust tiers** to tools.

---

### 1.1 Extend tool metadata

In `internal/agent/tools` (or equivalent), ensure every tool declares:

```go
type ToolCapability int

const (
    CapReadOnly ToolCapability = iota
    CapWriteFS
    CapExec
    CapNetwork
)
```

Each tool exposes:

```go
Capabilities() []ToolCapability
```

Example:

```go
search_code → CapReadOnly
read_file   → CapReadOnly
write_file  → CapWriteFS
exec_shell  → CapExec
```

---

### 1.2 Policy default behavior (critical)

Modify your approval gate:

```go
func (p *Policy) RequiresApproval(tool Tool) bool {
    if p.Mode == ModeInteractive {
        return tool.Has(CapExec) || tool.Has(CapNetwork)
    }
    return false
}
```

Result:

* **Read tools never ask**
* Write tools are allowed but staged
* Exec/network always gated

This single change removes 80% of UX pain.

---

## Phase 2 — Replace Per-Tool Prompts with Session Consent (2 days)

### Problem

Repeated confirmations destroy cognitive flow.

### Solution

Introduce **session-scoped approvals**.

---

### 2.1 Approval cache

```go
type ApprovalScope struct {
    AllowRead   bool
    AllowWrite  bool
    AllowExec   bool
}
```

At session start:

```text
This task may:
✓ Read files
✓ Propose code changes
✗ Execute shell commands

Approve? [Y/n]
```

Store in context:

```go
ctx.Approvals.AllowRead = true
```

Tools check **capability vs scope**, not prompt every time.

---

### 2.2 Escalation path

If agent unexpectedly needs exec:

```text
⚠️ Agent requests shell execution:
Command: go test ./...

Approve once / always / deny
```

This keeps surprises visible without harassment.

---

## Phase 3 — Planning First, Acting Second (UX Multiplier) (2–3 days)

### Problem

Agent explores reactively, causing tool spam.

### Solution

Enforce a **Plan → Execute** contract.

---

### 3.1 Mandatory planning step

Before any tool usage:

```text
PLAN:
1. Search for X
2. Read Y
3. Modify Z
4. Write tests
```

User sees **one plan**, not 50 prompts.

---

### 3.2 Tool budget per step

Each plan step gets:

* Tool quota
* File scope

Example:

```json
Step 2:
  allowed_tools: [read_file]
  files: ["internal/services/drive/*"]
```

This also improves safety.

---

## Phase 4 — Silent Observability + Explicit Mutation (1–2 days)

### Problem

Users don’t know *what* the agent did without interruptions.

### Solution

Log silently, summarize explicitly.

---

### 4.1 Silent tool log

Maintain:

```go
[]ToolCall{
  Tool: "read_file",
  Target: "handler.go",
}
```

---

### 4.2 End-of-step summary

```text
Step Complete:
• Read 4 files
• Identified 2 handlers
• No code changes yet
```

This replaces dozens of approval prompts.

---

## Phase 5 — UX Power Features (High ROI) (3–5 days)

### 5.1 “Trust this agent for this repo”

Persist approval scope in:

```
.codepicker/trust.json
```

```json
{
  "repo": "envspace",
  "allow_read": true,
  "allow_write": true,
  "allow_exec": false
}
```

---

### 5.2 Intent-based modes

CLI flags:

```bash
codepicker agent run --analyze
codepicker agent run --refactor
codepicker agent run --implement
```

Each mode preloads a policy:

* Analyze → read-only
* Refactor → read + write
* Implement → full (with exec approval)

---

### 5.3 “Why did you touch this file?”

Every change carries provenance:

```go
// Changed because: handler duplication detected in drive + archive
```

Trust increases, review time drops.

---

## Phase 6 — Hard Security Without UX Cost (Ongoing)

### Keep these **non-negotiable**

✔ Shadow filesystem (already excellent)
✔ No direct writes to source
✔ Exec sandbox + allowlist
✔ Deterministic diff review

### Add later

* Tool anomaly detection
* Rate limits per capability
* Replayable execution logs

---

## Expected Results

After Phase 2:

* **~90% fewer prompts**
* Agent feels autonomous, not timid

After Phase 4:

* Users understand what happened without babysitting

After Phase 5:

* Codepicker feels *professional*, not experimental
