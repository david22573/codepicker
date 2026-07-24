# CodePicker Safety Model

CodePicker is engineered from the ground up to prevent accidental destructive behaviors, directory escapes, and buggy AI refactoring.

## 1. Safety Modes

To protect your codebase, CodePicker enforces safe default modes during task execution:

- **Safe Defaults**: If neither `--apply` nor `--dry-run` nor `--plan-only` is provided, CodePicker automatically defaults to safe `--plan-only` mode. No writes ever happen by accident.
- **`--plan-only`**: Generates a complete step-by-step plan, prints plan ID, lists target files, but makes zero file writes.
- **`--dry-run`**: Generates the plan, copies repository files to a temporary sandbox, applies the generated diff inside the sandbox, and runs your test/build checks. Real files remain untouched.
- **`--apply`**: Real files are updated only after verification passes and the user confirms the change.

## 2. Shadow Filesystem

All file modifications during dry-runs or active agent turns are written to a **Shadow Filesystem** under `.codepicker/shadow/`.
Real filesystem files are never directly modified by the AI. When the user is ready, the `apply` command moves shadow files atomically to the real project structure.

## 3. Sandboxed Verification

Before code changes are committed, the verifier pipeline:
1. Copies the repository state to a sandboxed directory.
2. Applies the proposed patches/diffs.
3. Automatically runs checks based on your project language:
   - **Go**: `go test ./...`, `go vet ./...`, `go build ./...`
   - **Node**: `npm test`, `npm run build`
   - **pnpm**: `pnpm test`, `pnpm build`
   - **Python**: `pytest`, `python -m compileall .`
   - **Custom**: User-defined commands inside `codepicker.yaml` under `verify.commands`.

If any of the checks fail, CodePicker **fails closed**—refusing to apply the modifications unless the user explicitly forces the update.

## 4. Atomic Transactions & Rollbacks

Every apply is wrapped in a workspace transaction.
- **Backups**: Before applying any changes, CodePicker saves the original files to `.codepicker/runs/<run-id>/backups/`.
- **Atomic Rollback**: If you need to revert a run, executing `codepicker undo --last` (or `codepicker undo <run-id>`) restores the exact original files from the backups directory instantly.

## 5. Path Sanitization & Guardrails

Path traversal attempts are systematically blocked:
- Traversals (e.g. `../etc/passwd`) are blocked.
- Absolute paths (e.g. `/etc/passwd`) are blocked.
- Drive letters (e.g. `C:\Windows`) are blocked.
- Home directory shortcuts (e.g. `~/.ssh`) are blocked.
- Security whitelists block hazardous shell commands like `rm -rf`, `chmod`, `sudo`, and `curl | sh`.
