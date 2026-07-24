# CodePicker Commands Reference

This reference details every command available in the CodePicker CLI.

## 1. Non-LLM Commands (Work Offline without API Key)

### `init`
Initializes CodePicker workspace directories and scaffolds configuration files.
```bash
codepicker init
```

### `pack`
Packs codebase targets into a high-density format optimized for LLMs.
```bash
# General full pack
codepicker pack --output ctx.txt

# Selective pack
codepicker pack ./cmd ./infra --output ctx.txt

# Pack only files changed in Git
codepicker pack --changed --output changed_ctx.txt

# Pack using a language profile
codepicker pack --profile go-cli --output ctx.txt
```

### `plans`
Lists plans or previews details of a specific plan.
```bash
# List plans
codepicker plans

# Preview plan details
codepicker plans <plan-id>
```

### `cost`
Displays estimated cumulative token counts and API usage costs.
```bash
codepicker cost
```

### `history`
Lists past execution sessions.
```bash
codepicker history
```

### `inspect`
Replays the thought-action-observation loop timeline of a past execution run.
```bash
codepicker inspect <run-id>
```

### `apply`
Applies verified patches or shadow files to the real project filesystem.
```bash
# Preview and apply all pending shadow writes
codepicker apply

# Apply a specific plan or run diff
codepicker apply <plan-id>
codepicker apply patch.diff
```

### `undo`
Reverts the edits made by a past run by restoring file backups.
```bash
# List undoable runs
codepicker undo --list

# Revert the single most recent run
codepicker undo --last

# Revert a specific run
codepicker undo <run-id>
```

---

## 2. LLM-Dependent Commands (Require `OPENROUTER_API_KEY`)

### `run`
Executes a natural language instruction using the unified agent engine.
```bash
# Generate plan (Safe Default)
codepicker run "add tests" --plan-only

# Sandbox dry-run
codepicker run "add tests" --dry-run

# Execute and apply
codepicker run "add tests" --apply --branch
```

### `fix`
Analyzes a single file and applies automated fixes.
```bash
codepicker fix cmd/pack.go --apply --branch
```

### `improve`
Scouts the codebase for structural areas of improvement, suggests 3 tasks, and lets the user pick one to run.
```bash
# Interactive selection
codepicker improve

# Non-interactive auto-pick and apply
codepicker improve --pick 1 --apply --branch
```

### `prove`
Runs tests, compiler builds, vets, help text verifications, and sandbox smokes to verify CodePicker health.
```bash
codepicker prove
```
