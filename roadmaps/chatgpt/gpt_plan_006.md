# Codepicker Actualization Roadmap

This roadmap translates the current codebase review into an execution-ready plan. It is structured to reduce risk first, then improve UX clarity, then harden security and operability, and finally enable growth and adoption.

---

## Guiding Principles

* **Safety before autonomy**: destructive actions must be explicit, reversible, and observable.
* **Predictable CLI semantics**: no surprises, no implicit behavior.
* **Policy ≠ implementation drift**: what the user is told must match what the system enforces.
* **Composable improvements**: each phase should be shippable independently.

---

## Phase 0 — Stabilization & Bug Fixes (Immediate)

**Goal:** Eliminate correctness bugs and unsafe defaults.

### Summary

This phase addresses real logic bugs and behaviors that can lead to unintended file changes or user confusion. Nothing new is added; risk is reduced.

### Action Plan

1. **Fix `apply --accept` logic (critical bug)**

   * Treat `--accept` as a whitelist, not a hint.
   * Ensure `--reject` always wins.
   * Add unit tests covering:

     * accept only
     * reject only
     * accept + reject

2. **Change root command default behavior**

   * Replace implicit `context gen` execution with `cmd.Help()`.
   * Add a short hint: `Run 'codepicker context gen' to generate context.`

3. **Make rollback default-on for batch apply failures**

   * Automatically rollback on first failure.
   * Only prompt if rollback itself fails.

4. **Explicit confirmation on `init --force`**

   * Require interactive confirmation unless `--yes` is also provided.

**Exit Criteria**

* No implicit file generation.
* No partial-apply states without rollback.
* All apply filters behave deterministically.

---

## Phase 1 — CLI Contract & UX Clarity

**Goal:** Make the CLI predictable, scriptable, and self-explanatory.

### Summary

This phase standardizes flags, defaults, and interaction modes. The system becomes easier to reason about without reading source code.

### Action Plan

1. **Normalize flag semantics**

   * Reserve `-y` exclusively for destructive confirmation.
   * Avoid reusing short flags across unrelated commands.
   * Prefer long flags for safety (`--auto-approve`, `--overwrite`).

2. **Fix TUI defaults**

   * Change `apply --tui` default to `false`.
   * Introduce `--interactive` instead.

3. **Clarify diff behavior**

   * Require explicit diff base OR print resolved base (e.g. `HEAD`).
   * Log which files are included before scanning.

4. **Context/minify flag unification**

   * Either:

     * Remove global `--minify`, or
     * Explicitly propagate it into context commands.

**Exit Criteria**

* Flags mean the same thing everywhere.
* CI usage requires zero special casing.
* Users can predict behavior from `--help` alone.

---

## Phase 2 — Safety & Policy Alignment

**Goal:** Ensure enforcement matches claims and failures are explicit.

### Summary

This phase tightens the boundary between agent autonomy and system guarantees. The system fails loudly and safely.

### Action Plan

1. **Align Batch policy with execution reality**

   * Explicitly document and enforce allowed subprocesses (e.g. `gofmt`).
   * Or defer formatting to `apply` time only.

2. **Structured tool results**

   * Replace string-based error signaling with structured responses:

     * `{ status, reason, policy_violation }`
   * Allow agents to branch intelligently on failure type.

3. **Fail-closed safety middleware**

   * Safety middleware errors abort execution.
   * Cosmetic middleware remains best-effort.

4. **Gate chain-of-thought output**

   * Hide raw thoughts by default.
   * Reveal only under `--trace` or debug flags.

**Exit Criteria**

* Policy violations are machine-detectable.
* Users never see misleading “success” states.
* Debug output is intentional, not accidental.

---

## Phase 3 — Batch & Agent Operability

**Goal:** Make long-running and autonomous behavior observable and controllable.

### Summary

This phase improves confidence when running unattended jobs or multi-step plans.

### Action Plan

1. **Graceful batch shutdown**

   * Handle SIGINT/SIGTERM.
   * Drain or checkpoint running jobs.
   * Report final state on exit.

2. **Plan schema validation & retry**

   * Validate agent-produced plans before execution.
   * On failure, retry once with corrective prompt.

3. **Pre-apply change summaries**

   * Show:

     * File counts by type
     * Net line changes
     * Creates / modifies / deletes

**Exit Criteria**

* Users trust batch mode unattended.
* Plan execution failures are explainable and recoverable.

---

## Phase 4 — Security Posture & CI Mode

**Goal:** Make Codepicker safe-by-default in automated environments.

### Summary

This phase formalizes non-interactive usage and reduces ambient authority.

### Action Plan

1. **Introduce explicit CI mode**

   * Disable TUI, thoughts, and prompts.
   * Require `--yes` for all writes.

2. **Serve-mode hardening**

   * Warn when starting without auth.
   * Optional token-based protection.

3. **Cost & rate visibility**

   * Print cost summaries to stdout (not logs).
   * Fail fast when limits approach thresholds.

**Exit Criteria**

* CI usage is safe and deterministic.
* Long-running services expose clear risk boundaries.

---

## Phase 5 — Adoption & Extensibility

**Goal:** Prepare the system for wider use and contribution.

### Summary

This phase is about polish, documentation, and extensibility—not new core behavior.

### Action Plan

1. **Document the CLI contract**

   * Explicit behavior guarantees per command.
   * Stability promises for flags.

2. **Security & policy documentation**

   * Explain what agents can and cannot do.
   * Map policies to enforcement points.

3. **Extension guidelines**

   * How to add tools, MCP servers, or agents safely.

**Exit Criteria**

* Contributors can extend without breaking safety.
* Users understand trust boundaries.

---

## Final Outcome

After completing this roadmap, Codepicker becomes:

* Predictable enough for CI
* Safe enough for unattended autonomy
* Transparent enough to trust
* Structured enough to scale

This is the transition from a powerful prototype to a production-grade developer system.

