# CodePicker Checks Roadmap

## Mission

Implement all safety, verifier, apply, undo, pack, prove, and CLI checks so CodePicker can be trusted for daily agent-driven repo work.

Core rule:

```text
No real file write should happen unless the change is planned, previewed, verified, backed up, recorded, and undoable.
````

---

# Phase 1: Centralize Check Execution

## Goal

Create one internal check runner used by `prove`, `run --apply`, `apply`, and tests.

## Target Files

```text
adapters/verifier/pipeline.go
infra/shell/executor.go
infra/config/loader.go
domain/config/config.go
runtime/config.go
cmd/prove.go
cmd/run.go
cmd/apply.go
```

## Tasks

### 1. Define check result types

Add or update a result model:

```go
type CheckStatus string

const (
	CheckPass CheckStatus = "pass"
	CheckFail CheckStatus = "fail"
	CheckSkip CheckStatus = "skip"
)

type CheckResult struct {
	Name       string        `json:"name"`
	Command    string        `json:"command,omitempty"`
	Status     CheckStatus   `json:"status"`
	ExitCode   int           `json:"exit_code,omitempty"`
	DurationMS int64         `json:"duration_ms"`
	Stdout     string        `json:"stdout,omitempty"`
	Stderr     string        `json:"stderr,omitempty"`
	Error      string        `json:"error,omitempty"`
}

type CheckReport struct {
	Status string        `json:"status"`
	Checks []CheckResult `json:"checks"`
}
```

### 2. Add reusable command runner

Implement:

```go
RunCommandCheck(ctx, name, command, workingDir string) CheckResult
```

Rules:

```text
capture stdout
capture stderr
capture exit code
capture duration
never panic
return fail on non-zero exit
return fail on timeout/context cancel
```

### 3. Add check report writer

Write:

```text
.codepicker/runs/<run-id>/checks.json
.codepicker/runs/<run-id>/checks.log
.codepicker/runs/<run-id>/checks.md
```

## Acceptance Criteria

Any command can run checks and receive structured pass/fail results.

All check failures are recorded instead of crashing.

---

# Phase 2: Add Repository Health Checks

## Goal

Create baseline repo checks used by `codepicker prove`.

## Target Files

```text
cmd/prove.go
Makefile
adapters/verifier/pipeline.go
infra/shell/executor.go
```

## Required Checks

Run these in order:

```bash
go test ./...
go vet ./...
go build -o codepicker main.go
./codepicker --help
./codepicker init --help
./codepicker pack --help
./codepicker run --help
./codepicker apply --help
./codepicker undo --help
./codepicker prove --help
```

## Tasks

### 1. Implement `codepicker prove`

Command:

```bash
codepicker prove
```

Output:

```text
CODEPICKER PROOF REPORT

PASS go test ./...
PASS go vet ./...
PASS go build -o codepicker main.go
PASS codepicker --help
PASS codepicker init --help
PASS codepicker pack --help
PASS codepicker run --help
PASS codepicker apply --help
PASS codepicker undo --help

Artifacts:
.codepicker/runs/proof/<timestamp>/
```

### 2. Save proof artifacts

Create:

```text
.codepicker/runs/proof/<timestamp>/
  proof.json
  proof.log
  proof.md
```

### 3. Update Makefile

Add:

```makefile
test:
	go test ./...

vet:
	go vet ./...

smoke: build
	./$(BINARY_NAME) --help
	./$(BINARY_NAME) init --help
	./$(BINARY_NAME) pack --help
	./$(BINARY_NAME) run --help
	./$(BINARY_NAME) apply --help
	./$(BINARY_NAME) undo --help
	./$(BINARY_NAME) prove --help

prove: test vet smoke
	./$(BINARY_NAME) prove
```

## Acceptance Criteria

This works:

```bash
make prove
codepicker prove
```

Both produce clear pass/fail output and artifact files.

---

# Phase 3: Add Verifier Checks Before Apply

## Goal

Block unsafe real writes when verifier fails.

## Target Files

```text
adapters/verifier/pipeline.go
cmd/run.go
cmd/apply.go
infra/fs/shadow.go
infra/fs/sandbox.go
infra/shell/executor.go
domain/config/config.go
runtime/config.go
```

## Required Behavior

Before real apply:

```text
1. collect proposed changes
2. run verifier checks
3. save verifier result
4. block apply if verifier fails
5. allow bypass only with --force
```

## Verifier Config

Support:

```yaml
verify:
  fail_closed: true
  commands:
    - go test ./...
    - go vet ./...
    - go build ./...
```

## Language Defaults

Detect default verifier commands:

```text
go.mod -> Go
package.json -> Node
pnpm-lock.yaml -> pnpm
pyproject.toml -> Python
requirements.txt -> Python
Cargo.toml -> Rust
```

Defaults:

```yaml
go:
  - go test ./...
  - go vet ./...
  - go build ./...

node:
  - npm test
  - npm run build

pnpm:
  - pnpm test
  - pnpm build

python:
  - pytest
  - python -m compileall .

rust:
  - cargo test
  - cargo build
```

## Tasks

### 1. Implement verifier command selection

Priority:

```text
1. config verify.commands
2. language auto-detected defaults
3. fallback to no-op warning
```

If `verify.fail_closed` is true and no verifier commands exist, block apply.

### 2. Save verifier artifacts

Write:

```text
.codepicker/runs/<run-id>/verifier.json
.codepicker/runs/<run-id>/verifier.log
.codepicker/runs/<run-id>/verifier.md
```

### 3. Block apply on verifier fail

Default behavior:

```text
verifier fail -> no real file writes
```

Allow only:

```bash
--force
```

### 4. Show concise output

Example pass:

```text
Verifier:
  PASS go test ./...
  PASS go vet ./...
  PASS go build ./...

Result:
  Safe to apply.
```

Example fail:

```text
Verifier:
  PASS go test ./...
  FAIL go vet ./...

Result:
  Apply blocked.
  See: .codepicker/runs/<run-id>/verifier.log
```

## Acceptance Criteria

This blocks unsafe apply:

```bash
codepicker run "make a change" --apply
```

when checks fail.

This bypasses only explicitly:

```bash
codepicker run "make a change" --apply --force
```

Verifier artifacts are always saved.

---

# Phase 4: Add Dirty Repo Checks

## Goal

Prevent CodePicker from overwriting unrelated user work.

## Target Files

```text
infra/git/client.go
cmd/run.go
cmd/apply.go
cmd/undo.go
infra/fs/manager.go
```

## Required Behavior

Before apply or undo, inspect git status.

If repo has uncommitted changes unrelated to the current run, block unless:

```bash
--allow-dirty
```

## Tasks

### 1. Add git status helper

Implement:

```go
IsDirty(ctx, repoRoot string) (bool, []string, error)
```

Return changed files.

### 2. Add dirty repo guard

Before real writes:

```text
if dirty and not --allow-dirty:
    print changed files
    block operation
```

### 3. Allow bypass

Support:

```bash
codepicker apply <run-id> --allow-dirty
codepicker undo --last --allow-dirty
codepicker run "task" --apply --allow-dirty
```

## Output

```text
Dirty repo detected.

Uncommitted files:
  M cmd/pack.go
  ?? notes.md

Apply blocked.

Use --allow-dirty to continue anyway.
```

## Acceptance Criteria

Apply and undo refuse dirty repos unless `--allow-dirty` is passed.

Dirty file list is shown clearly.

---

# Phase 5: Add Apply Preview Checks

## Goal

Make every real write visible before it happens.

## Target Files

```text
cmd/apply.go
cmd/run.go
infra/fs/shadow.go
infra/fs/manager.go
domain/task/task.go
domain/audit/trail.go
```

## Required Behavior

Before writing:

```text
show run ID
show files to change
show operation type per file
show verifier status
require confirmation
```

## Preview Format

```text
Apply Preview

Run: run-20260526-153000
Task: Add token summary to pack output

Files:
  MODIFIED cmd/pack.go
  MODIFIED adapters/context/builder.go
  ADDED    docs/commands.md
  DELETED  old-notes.md

Verifier:
  PASS

Proceed? [y/N]
```

## Tasks

### 1. Detect file operation type

Classify each touched file:

```text
ADDED
MODIFIED
DELETED
RENAMED
```

### 2. Require confirmation

Interactive default:

```text
Proceed? [y/N]
```

Non-interactive:

```bash
--yes
```

### 3. Block empty apply

If no file changes exist:

```text
No changes to apply.
```

Exit cleanly.

## Acceptance Criteria

`apply` never silently writes files.

`--yes` is required for non-interactive apply.

Empty applies are handled cleanly.

---

# Phase 6: Add Backup Checks

## Goal

Ensure every modified file can be restored.

## Target Files

```text
infra/fs/manager.go
infra/fs/shadow.go
cmd/apply.go
cmd/run.go
domain/audit/trail.go
infra/audit/trail.go
infra/storage/sqlite.go
```

## Required Backup Layout

```text
.codepicker/runs/<run-id>/
  backups/
    cmd/
      pack.go
    adapters/
      context/
        builder.go
  backup_manifest.json
```

## Manifest Format

```json
{
  "run_id": "run-20260526-153000",
  "created_at": "2026-05-26T15:30:00Z",
  "files": [
    {
      "path": "cmd/pack.go",
      "operation": "modified",
      "existed_before": true,
      "backup_path": "backups/cmd/pack.go"
    },
    {
      "path": "docs/new.md",
      "operation": "added",
      "existed_before": false,
      "backup_path": null
    }
  ]
}
```

## Tasks

### 1. Backup before write

Before applying any file change:

```text
if file exists:
    copy original to backups/
if file does not exist:
    record existed_before=false
```

### 2. Verify backup exists

Before writing a modified/deleted file:

```text
if file existed_before and backup missing:
    block apply
```

### 3. Save manifest

Always save:

```text
backup_manifest.json
```

### 4. Include backup status in run summary

Add to:

```text
summary.md
```

## Acceptance Criteria

Every apply creates backup metadata.

Modified/deleted files always have backups.

Apply is blocked if backup creation fails.

---

# Phase 7: Add Undo Checks

## Goal

Make undo safe and predictable.

## Target Files

```text
cmd/undo.go
infra/fs/manager.go
infra/audit/trail.go
domain/audit/trail.go
infra/storage/sqlite.go
```

## Required Commands

```bash
codepicker undo --list
codepicker undo --last
codepicker undo <run-id>
codepicker undo <run-id> --yes
```

## Undo Rules

```text
If file existed before:
    restore backup.

If file did not exist before:
    delete created file.

If current file has changed since apply:
    warn and block unless --force or --yes-confirmed behavior is explicit.

If backup manifest missing:
    block undo.

If backup file missing for modified/deleted file:
    block undo.
```

## Tasks

### 1. Implement undo list

Output:

```text
Undoable Runs

run-20260526-153000  3 files  Add pack token summary
run-20260526-160200  1 file   Fix verifier config
```

### 2. Implement undo last

Find latest applied run with backup manifest:

```bash
codepicker undo --last
```

### 3. Check current file drift

Before restore, compare current file hash against expected post-apply hash if available.

If no hash exists, use modified timestamp or warn conservatively.

### 4. Require confirmation

Output:

```text
Undo Preview

Run: run-20260526-153000

Files:
  RESTORE cmd/pack.go
  DELETE  docs/new.md

Proceed? [y/N]
```

Skip prompt only with:

```bash
--yes
```

## Acceptance Criteria

This works:

```bash
codepicker apply <run-id>
codepicker undo --last
```

Undo restores modified files and deletes created files.

Undo blocks if required backup data is missing.

---

# Phase 8: Add Path Safety Checks

## Goal

Prevent file operations outside the repo.

## Target Files

```text
infra/fs/safety.go
infra/pathutil/path.go
adapters/policy/enforcer.go
adapters/tools/fs.go
adapters/tools/fs_edit.go
adapters/tools/fs_read.go
cmd/apply.go
cmd/run.go
cmd/undo.go
```

## Block These Paths

```text
../outside.txt
../../etc/passwd
/etc/passwd
~/.ssh/id_rsa
C:\Windows\System32
\\server\share\file.txt
file:///etc/passwd
```

## Allow These Paths

```text
cmd/root.go
internal/app/app.go
docs/quickstart.md
README.md
Makefile
```

## Tasks

### 1. Centralize path validation

Create or enforce:

```go
ValidateRepoRelativePath(repoRoot, inputPath string) (string, error)
```

Rules:

```text
must be relative
must clean safely
must stay inside repo root
must reject home expansion
must reject absolute paths
must reject Windows drive paths
must reject UNC paths
must reject file URLs
```

### 2. Use validator everywhere

Apply to:

```text
read_file
write_file
edit_file
apply
undo
pack explicit paths
shadow commit
backup restore
```

### 3. Add fuzz tests

Keep or expand:

```text
adapters/policy/enforcer_fuzz_test.go
```

## Acceptance Criteria

Unsafe paths are blocked everywhere.

No file tool can read/write outside repo.

---

# Phase 9: Add Pack Checks

## Goal

Ensure context packs are deterministic, clean, and useful.

## Target Files

```text
cmd/pack.go
adapters/context/builder.go
adapters/context/primer.go
adapters/context/streaming.go
infra/indexer/repo_map.go
infra/llm/token_estimator.go
infra/git/client.go
```

## Required Checks

### 1. Deterministic output

Same input should produce same file order.

Sort all included files.

### 2. Default excludes

Exclude:

```text
.git
node_modules
vendor
dist
build
tmp
.cache
coverage
*.log
*.exe
*.dll
*.so
*.dylib
*.png
*.jpg
*.jpeg
*.gif
*.webp
*.mp4
*.zip
*.tar
*.gz
```

### 3. Header required

Every pack must include:

```markdown
# CodePicker Context Pack

## Task

## Repo

## Generated

## Token Estimate

## Files Included

## File Tree
```

### 4. Token summary required

End with:

```markdown
## Token Summary

| File | Estimated Tokens |
|---|---:|
```

### 5. Changed mode check

`--changed` should only include changed/untracked files.

### 6. Max token check

If output exceeds `--max-tokens`, print warning or trim according to configured behavior.

## Acceptance Criteria

These work:

```bash
codepicker pack --output ctx.txt
codepicker pack --task "review repo" --output ctx.txt
codepicker pack --changed --output changed_ctx.txt
codepicker pack --profile go-cli --output go_ctx.txt
```

Pack output is deterministic and excludes junk.

---

# Phase 10: Add CLI Help Checks

## Goal

Make every command usable by humans and agents.

## Target Files

```text
cmd/root.go
cmd/init.go
cmd/pack.go
cmd/run.go
cmd/apply.go
cmd/undo.go
cmd/prove.go
cmd/plans.go
cmd/history.go
cmd/cost.go
cmd/fix.go
cmd/improve.go
cmd/context.go
cmd/agent.go
```

## Required Help Fields

Every command should include:

```text
short description
long description
examples
whether it writes files
whether it requires LLM
important flags
```

## Required Examples

### Root

```bash
codepicker init
codepicker pack --task "review repo" --output ctx.txt
codepicker run "add tests" --dry-run
codepicker run "add tests" --apply --branch
codepicker prove
codepicker undo --last
```

### Pack

```bash
codepicker pack --output ctx.txt
codepicker pack --changed --task "review diff" --output ctx.txt
codepicker pack --profile go-cli --max-tokens 80000 --output ctx.txt
```

### Run

```bash
codepicker run "add tests for pack" --plan-only
codepicker run "add tests for pack" --dry-run
codepicker run "add tests for pack" --apply --branch
```

### Apply

```bash
codepicker apply <run-id>
codepicker apply <run-id> --yes
```

### Undo

```bash
codepicker undo --list
codepicker undo --last
codepicker undo <run-id>
```

## Acceptance Criteria

This passes:

```bash
codepicker --help
codepicker init --help
codepicker pack --help
codepicker run --help
codepicker apply --help
codepicker undo --help
codepicker prove --help
```

All help output is clear and non-empty.

---

# Phase 11: Add JSON Output Checks

## Goal

Make CodePicker scriptable by CLI agents.

## Target Files

```text
cmd/run.go
cmd/pack.go
cmd/apply.go
cmd/undo.go
cmd/prove.go
cmd/history.go
cmd/plans.go
cmd/cost.go
infra/ui/tui.go
domain/task/task.go
domain/audit/report.go
```

## Required Flag

Support:

```bash
--json
```

For:

```text
run
pack
apply
undo
prove
history
plans
cost
```

## JSON Rules

```text
valid JSON only
no color codes
no extra logs mixed into stdout
human logs go to stderr
exit code reflects success/failure
```

## Example

```json
{
  "status": "pass",
  "run_id": "run-20260526-153000",
  "artifacts_dir": ".codepicker/runs/run-20260526-153000",
  "checks": [
    {
      "name": "go test",
      "status": "pass"
    }
  ]
}
```

## Acceptance Criteria

This emits valid JSON:

```bash
codepicker prove --json
codepicker run "task" --dry-run --json
codepicker apply <run-id> --json
codepicker undo --last --json
```

---

# Phase 12: Add Automated Tests for All Checks

## Goal

Lock safety behavior with tests.

## Target Files

```text
infra/fs/safety_test.go
infra/pathutil/path_test.go
infra/fs/shadow_test.go
infra/fs/manager_test.go
adapters/verifier/pipeline_test.go
cmd/pack_test.go
cmd/run_test.go
cmd/apply_test.go
cmd/undo_test.go
cmd/prove_test.go
```

## Required Test Groups

### 1. Path Safety Tests

Assert blocked:

```text
../outside.txt
../../etc/passwd
/etc/passwd
~/.ssh/id_rsa
C:\Windows\System32
\\server\share\file.txt
file:///etc/passwd
```

Assert allowed:

```text
cmd/root.go
internal/app/app.go
docs/quickstart.md
README.md
Makefile
```

### 2. Verifier Tests

Assert:

```text
passing command returns pass
failing command returns fail
stdout/stderr captured
duration captured
logs written
apply blocked on fail
--force bypasses fail
```

### 3. Dirty Repo Tests

Assert:

```text
dirty repo blocks apply
dirty repo blocks undo
--allow-dirty bypasses
dirty file list is shown
```

### 4. Apply Tests

Assert:

```text
preview lists files
confirmation required
--yes bypasses prompt
backup created before write
backup manifest written
empty apply exits cleanly
```

### 5. Undo Tests

Assert:

```text
undo --list shows runs
undo --last selects newest
modified files restored
created files deleted
missing manifest blocks undo
missing backup blocks undo
file drift blocks undo unless forced
```

### 6. Pack Tests

Assert:

```text
deterministic file order
default excludes applied
header exists
file tree exists
token summary exists
--changed only includes changed files
--profile go-cli works
```

### 7. Prove Tests

Assert:

```text
prove creates artifact dir
prove writes proof.json
prove writes proof.log
prove writes proof.md
prove fails when a required command fails
prove --json emits valid JSON
```

### 8. CLI Help Tests

Assert:

```text
all required commands have non-empty help
help includes examples
help states whether command writes files
```

## Acceptance Criteria

This passes:

```bash
go test ./...
make prove
```

Tests cover:

```text
path safety
verifier
dirty repo guard
apply preview
backups
undo
pack
prove
CLI help
JSON output
```

---

# Phase 13: Add Final Proof Sequence

## Goal

Create one end-to-end command sequence that proves CodePicker is ready.

## Target Files

```text
Makefile
cmd/prove.go
docs/agent-workflow.md
```

## Required Manual Proof Sequence

Run:

```bash
make prove

codepicker init

codepicker pack \
  --task "review current repo" \
  --output ctx.txt

codepicker pack \
  --changed \
  --task "review current diff" \
  --output changed_ctx.txt

codepicker run \
  "make a tiny safe test change" \
  --plan-only

codepicker run \
  "make a tiny safe test change" \
  --dry-run

codepicker run \
  "make a tiny safe test change" \
  --apply \
  --branch \
  --yes

codepicker prove

codepicker undo --last --yes

codepicker prove
```

## Required Result

All commands complete successfully.

After undo:

```bash
git diff
```

should not show leftover changes from the applied run.

## Acceptance Criteria

The full sequence passes on a clean repo.

If any command fails, the artifact path clearly explains why.

---

# Final Implementation Order

Do this in order:

```text
1. Centralize check execution
2. Add repository health checks
3. Add verifier checks before apply
4. Add dirty repo checks
5. Add apply preview checks
6. Add backup checks
7. Add undo checks
8. Add path safety checks
9. Add pack checks
10. Add CLI help checks
11. Add JSON output checks
12. Add automated tests for all checks
13. Add final proof sequence
```

---

# Definition of Done

CodePicker is ready for heavier CLI-agent use when this full command set passes:

```bash
go test ./...
make prove
codepicker prove
codepicker pack --changed --task "review current diff" --output ctx.txt
codepicker run "tiny safe test change" --dry-run
codepicker run "tiny safe test change" --apply --branch --yes
codepicker undo --last --yes
codepicker prove
```

And these safety guarantees hold:

```text
unsafe paths are blocked
dirty repos are protected
apply requires preview
apply requires verifier pass unless --force
apply creates backups
undo restores from manifest
pack output is deterministic
prove creates artifacts
--json emits valid JSON
commands fail cleanly
```
