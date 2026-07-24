# CodePicker 🤖

CodePicker is a highly secure, autonomous refactoring CLI and AI agent assistant designed to safely execute codebase refactoring with strict sandboxing and atomic rollbacks.

## 🚀 Core Workflow

```text
pack ──> plan ──> verify ──> apply ──> undo ──> prove
```

1. **Pack**: Optimize codebase context into a high-density format for LLMs.
2. **Plan**: Generate sequential refactoring tasks without file changes.
3. **Verify**: Execute patches in a sandboxed, isolated shadow filesystem.
4. **Apply**: Move verified changes atomically to the real project.
5. **Undo**: Rollback edits instantly using automatic file backups.
6. **Prove**: Run compilation, tests, vet, and smoke checks to ensure repository health.

---

## 🛡️ Key Features

- **Sandboxed Execution**: Proposed patches are validated inside a safe sandbox prior to applying.
- **Configurable Verification**: Supports custom verifier commands in `codepicker.yaml` and language defaults (Go, Node, Python, pnpm).
- **Atomic Rollbacks**: Automatic transaction backup copies stored under `.codepicker/runs/<run-id>/backups/` allow complete rollbacks via `codepicker undo`.
- **Path Sanitization**: Restricts path traversals (`..`), absolute paths (`/`), home directory symbols (`~`), and drive letters (`C:`).
- **Unified Engine**: Single DRY orchestrator engine powers `run`, `fix`, and `improve` pipelines identically.
- **Proof Report**: System-wide check via `codepicker prove` and `make prove` with detailed reports.

---

## ⚡ Quickstart

```bash
# 1. Initialize CodePicker configurations
codepicker init

# 2. Pack repo context for the LLM
codepicker pack --output ctx.txt

# 3. Create execution plan (Safe Default)
codepicker run "refactor database client" --plan-only

# 4. Execute and test in sandbox
codepicker run "refactor database client" --dry-run

# 5. Apply verified changes on a new branch
codepicker run "refactor database client" --apply --branch

# 6. Verify overall repository health
codepicker prove
```

---

## 📖 Documentation

Detailed guides are available in the [docs/](codepicker/docs/) directory:
- [Quickstart Guide](codepicker/docs/quickstart.md)
- [Safety Model](codepicker/docs/safety.md)
- [Commands Reference](codepicker/docs/commands.md)
- [Agent Workflow Guide](codepicker/docs/agent-workflow.md)
