# CodePicker 🤖

CodePicker is an autonomous, security-hardened coding agent designed for local refactoring and feature implementation. Unlike generic AI tools, CodePicker runs safely on your machine with strict guardrails.

## 🛡️ Production Features

- **Sandboxed Execution**: All file operations are staged in a shadow filesystem (`.codepicker/shadow`) and only applied after verification.
- **Path Traversal Protection**: Mathematically proven path sanitization preventing escapes to parent directories (e.g., `../../etc/shadow`).
- **Atomic Rollbacks**: Every execution runs in a transaction. If the agent crashes or fails verification, the repository state is instantly restored.
- **Cost Tracking**: Real-time token usage and cost estimation for OpenRouter/LLM calls.
- **Audit Trail**: Full JSON logs of every thought, action, and tool output are saved to `.codepicker/audit/`.

## 🚀 Installation

```bash
# Clone the repo
git clone [https://github.com/david22573/codepicker](https://github.com/david22573/codepicker)
cd codepicker

# Build
go build -o codepicker main.go

# Verify
./codepicker --help

```

## ⚡ Usage

### 1. Run a Task

The agent will plan, execute, and verify the task.

```bash
export OPENROUTER_API_KEY="sk-..."
./codepicker run "Refactor infra/fs/manager.go to use a singleton pattern"

```

### 2. Dry Run (Safe Mode)

Simulate the run without writing any files to disk.

```bash
./codepicker run "Delete all unused files" --dry-run

```

### 3. View History

Check past runs and their costs.

```bash
./codepicker history

```

## 🔒 Security Policy

CodePicker enforces a `policy.json` whitelist.

* **Blocked Commands**: `rm -rf`, `curl | sh`, `chmod`, `sudo`.
* **Blocked Paths**: `/etc/*`, `.git/*`, absolute paths.
* **Network**: The agent cannot open arbitrary network connections.

## 🧩 Architecture

* **Planner**: Decomposes tasks into sequential steps.
* **ReAct Agent**: Executes steps using a Thought-Action-Observation loop.
* **Verifier**: Runs `go test` and syntax checks in a temporary sandbox before applying changes.
