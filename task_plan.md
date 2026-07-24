# Task Plan: Fully Implement CodePicker Next Roadmap

## Goal
Make CodePicker a reliable CLI for LLM-assisted repository work by fully implementing all 13 roadmap phases.

## Current Phase
Completed

## Phases

### Phase 1: Lock Down the CLI Contract
- [x] Add standard Makefile targets (`test`, `vet`, `build`, `smoke`, `prove`)
- [x] Ensure every command has useful help text
- [x] Split commands into LLM and non-LLM categories (so non-LLM don't require `OPENROUTER_API_KEY`)
- [x] Fail cleanly when config or API key is missing (no panics, useful error message/hint)
- **Status:** complete

### Phase 2: Make `pack` the Core Product
- [x] Add structured output header with task, repo, generated timestamp, token estimate, files included/excluded
- [x] Add alphabetical file tree before file contents
- [x] Add deterministic file ordering (sort alphabetically)
- [x] Add default excludes (.git, node_modules, vendor, dist, build, tmp, .cache, coverage, images, logs, binaries, archives)
- [x] Add `--changed` flag
- [x] Add `--profile` support
- [x] Add token summary per file table
- **Status:** complete

### Phase 3: Make `run` Safe by Default
- [x] Default mode must be safe (default to `--plan-only` or `--dry-run`)
- [x] Implement `--plan-only` (make zero changes, save plan, print plan ID, list target files)
- [x] Implement `--dry-run` (execute against shadow/sandbox only, show changes, run verifier, zero real file changes)
- [x] Implement `--apply` (require explicit flag, summary before write, transaction backup/rollback, save run artifacts)
- [x] Implement `--branch` to switch to a task branch format `codepicker/<slug>-<timestamp>`
- [x] Implement `--ci` (run test, vet, build commands after patching; fail closed if any fails)
- **Status:** complete

### Phase 4: Store Every Run as an Artifact
- [x] Generate run ID format `run-YYYYMMDD-HHMMSS` for `run`, `fix`, `improve`
- [x] Save task input to `.codepicker/runs/<run-id>/task.md`
- [x] Save plan.json and plan.md
- [x] Save patch.diff
- [x] Save verifier.log
- [x] Save cost.json
- [x] Save human-readable summary.md
- **Status:** complete

### Phase 5: Unify `fix`, `improve`, and `run`
- [x] Extract shared run orchestration into a reusable internal function `RunTask`
- [x] Remove duplicated execution logic from `fix` and `improve` commands
- [x] Align safe modes (`--plan-only`, `--dry-run`, `--apply`, `--branch`) across all three commands
- **Status:** complete

### Phase 6: Add `codepicker prove`
- [x] Implement `codepicker prove` command running build, test, vet, help checks, and smoke checks
- [x] Save proof artifacts under `.codepicker/runs/proof/<timestamp>/` (`proof.log`, `proof.json`, `summary.md`)
- **Status:** complete

### Phase 7: Strengthen the Verifier
- [x] Always verify in sandbox first before real apply
- [x] Support configurable verifier commands in `codepicker.yaml`
- [x] Add language defaults (Go, Node, Python, pnpm)
- [x] Fail closed on verifier failure unless `--force` is passed
- **Status:** complete

### Phase 8: Improve `apply` and `undo`
- [x] `apply` should support plan IDs, patch files, and run diff paths
- [x] `apply` should preview changed files and ask for confirmation `Proceed? [y/N]` (skip with `--yes`)
- [x] `undo` should list undoable runs (`codepicker undo --list`, `<run-id>`, `--last`)
- [x] Store enough backup info under `.codepicker/runs/<run-id>/backups/` to allow full rollback
- **Status:** complete

### Phase 9: Add Minimal Tests Around Critical Safety
- [x] Path safety tests blocking outside paths (e.g. `../etc/passwd`, `~/.ssh/id_rsa`)
- [x] Pack output tests verifying excludes, file tree, determinism, token summaries
- [x] Shadow write tests verifying sandbox, shadow layer, rollback restores original file
- [x] Verifier tests checking pass/fail commands and log capturing
- [x] CLI smoke tests running `--help` options
- **Status:** complete

### Phase 10: Documentation Pass
- [x] README updates
- [x] Quickstart documentation
- [x] Commands reference
- [x] Safety model explanation
- [x] Agent workflow guidelines
- **Status:** complete

### Phase 11: Add JSON Output Checks
- [x] Add persistent global flag `--json` to `rootCmd`
- [x] Implement `prove --json`
- [x] Implement `run --json`
- [x] Implement `pack --json`
- [x] Implement `apply --json`
- [x] Implement `undo --json`
- [x] Implement `history --json`
- [x] Implement `plans --json`
- [x] Implement `cost --json`
- **Status:** complete

### Phase 12: Add Automated Tests for All Checks
- [x] Write comprehensive Go tests covering path safety, verifier, dirty repo guard, apply/undo backup flows, pack determinism, and JSON output formatting
- **Status:** complete

### Phase 13: Add Final Proof Sequence
- [x] Run automated build, test, vet, and CLI smoke checks
- [x] Verify the full end-to-end command sequence with a clean git diff
- **Status:** complete
