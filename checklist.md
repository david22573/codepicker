# Codepicker Feature Reintroduction Roadmap

This roadmap defines **what features to reintroduce**, **why they matter**, **how to implement them safely**, and **how to measure success**. It is designed to be executable by another AI (e.g., Gemini 3 Pro) or a human contributor without additional context.

The intent is to **add power without regressing safety, determinism, or trust**.

---

## Guiding Constraints (Non-Negotiable)

These constraints apply to *all* phases:

* The agent **never writes directly to the real filesystem**
* All writes go through the **shadow filesystem**
* All real writes require **explicit user action** (`apply`)
* The agent is treated as **untrusted input**
* No feature may introduce implicit behavior

---

## Phase 1 — Plan-Centric Execution (Foundational)

**Objective:** Separate intent from execution.

### Problem

Agent behavior is currently implicit and action-oriented. There is no formal declaration of intent before file changes occur.

### Solution

Introduce a **first-class Plan lifecycle** that governs all agent actions.

### Actionable Goals

1. Modify the agent to produce a `Plan` as its *first output*

   * Plan contains:

     * Original task
     * High-level reasoning
     * Ordered steps
     * Files involved per step

2. Introduce a `PlanExecutor`

   * Executes steps sequentially
   * Validates each step before execution
   * Records step-level results

3. Prevent tools from executing unless attached to an active plan step

### Success Metrics

* No tool executes outside a plan step
* Plans can be serialized, inspected, and replayed
* Execution history maps cleanly to plan steps

---

## Phase 2 — Read-Only / Dry Execution Mode

**Objective:** Allow full reasoning without side effects.

### Problem

Users cannot safely explore agent behavior without risking file changes.

### Solution

Introduce a **read-only execution mode** enforced by policy.

### Actionable Goals

1. Add a policy mode: `readonly`

   * `write_file` is denied
   * `run_cmd` is denied

2. Allow agent to fully reason and produce plans in read-only mode

3. Surface clear messaging when actions are blocked by policy

### Success Metrics

* Agent completes tasks without modifying shadow or real FS
* Policy blocks are explicit and traceable
* Same task behaves identically except for side effects

---

## Phase 3 — Pre-Apply Change Summaries

**Objective:** Make consequences visible before commitment.

### Problem

Users must inspect shadow files manually to understand impact.

### Solution

Generate a deterministic summary of pending changes.

### Actionable Goals

1. Add a `change summary` generator

   * Files added / modified
   * Line counts (added / removed)
   * File types affected

2. Display summary before any apply action

3. Persist summary alongside execution history

### Success Metrics

* Users can understand changes without opening files
* Summary output is stable and deterministic
* Summary matches applied changes exactly

---

## Phase 4 — Explicit Context Builder

**Objective:** Reintroduce context generation without hidden cost.

### Problem

Automatic context generation is expensive and opaque.

### Solution

Make context building **explicit, inspectable, and bounded**.

### Actionable Goals

1. Add `codepicker context build`

   * Deterministic file selection
   * Explicit include / exclude rules

2. Introduce early token budgeting

   * Allocate tokens before reading file content

3. Allow users to inspect generated context

### Success Metrics

* Context generation performs no unnecessary IO
* Token usage is predictable
* Context output is reproducible

---

## Phase 5 — CI / Batch Mode

**Objective:** Enable safe, unattended execution.

### Problem

Interactive assumptions make automation unsafe.

### Solution

Introduce a hardened CI mode enforced by policy.

### Actionable Goals

1. Add policy mode: `ci`

   * No prompts
   * No shell execution
   * No writes

2. Require explicit flags for all destructive actions

3. Produce machine-readable output

### Success Metrics

* CI runs never hang or prompt
* No filesystem writes occur in CI mode
* Output is parseable and stable

---

## Phase 6 — Optional Formatting & Tooling Hooks

**Objective:** Add quality-of-life improvements without agent authority.

### Problem

Formatting and tooling are useful but risky if agent-controlled.

### Solution

Expose formatting as **post-apply, user-controlled actions**.

### Actionable Goals

1. Add `codepicker fmt` or post-apply hook

2. Ensure formatting is never invoked by the agent

3. Make tooling opt-in and reversible

### Success Metrics

* Formatting never modifies shadow implicitly
* Users retain full control
* Formatting failures do not affect plan execution

---

## Phase 7 — Auditing & Replay

**Objective:** Make behavior explainable and repeatable.

### Problem

Past executions are recorded but not easily replayed or analyzed.

### Solution

Leverage stored plans and executions for auditing.

### Actionable Goals

1. Add execution replay support

2. Allow diffing between planned vs applied changes

3. Expose execution timelines

### Success Metrics

* Past executions can be replayed deterministically
* Debugging does not require rerunning the agent

---

## Final Definition of Success

After completing this roadmap, Codepicker will:

* Execute plans, not impulses
* Make all consequences visible before action
* Be safe in CI and autonomous environments
* Treat AI as a collaborator, not an authority

This roadmap defines the transition from **capable tool** to **trustworthy system**.

