# Codepicker

**Codepicker** is an AI-native developer tool that turns your codebase into context for LLMs and provides autonomous agents to perform complex refactoring, analysis, and architectural tasks.

It is designed for safety, predictability, and automation.

## 🚀 Key Features

* **Context Generation**: Turn your entire repo into a single, token-optimized Markdown file for LLM consumption.
* **Autonomous Agents**: Delegate complex tasks (refactoring, audits, bug fixes) to AI agents that plan and execute multi-step workflows.
* **Shadow Workspace**: Agents write code to a hidden `.codepicker/shadow` directory. No changes touch your live code until you review and `apply` them.
* **CI/CD Ready**: Strict headless modes, cost tracking, and deterministic behavior for automated pipelines.
* **Safety First**: Granular execution policies (Architect, Batch, Interactive) prevent unauthorized shell access or file destruction.
* **Extensible**: Connect external tools via [Model Context Protocol (MCP)](https://modelcontextprotocol.io/) or define custom scripts in YAML.

---

## 📦 Installation

```bash
go install [github.com/david22573/codepicker@latest](https://github.com/david22573/codepicker@latest)

```

Or build from source:

```bash
git clone [https://github.com/david22573/codepicker.git](https://github.com/david22573/codepicker.git)
cd codepicker
go build -o codepicker main.go

```

---

## ⚡ Quick Start

### 1. Initialize

Run the interactive setup wizard to configure your language, ignore patterns, and AI model.

```bash
codepicker init

```

### 2. Generate Context

Create a single file representation of your codebase to paste into ChatGPT or Claude.

```bash
codepicker context gen --out context.md --tokens

```

### 3. Run an Agent Task

Ask the agent to plan and execute a task.

```bash
codepicker agent run "Refactor the logging interface to use slog"

```

### 4. Review & Apply

The agent writes to a shadow filesystem. Review changes safely before applying them.

```bash
# Interactive TUI review
codepicker apply

# Batch apply all changes (safe for trusted outputs)
codepicker apply --yes

```

---

## 🛡️ Security & Policies

Codepicker enforces strict policies to ensure the AI behaves predictably.

| Policy | Shell Access | File Write | Use Case |
| --- | --- | --- | --- |
| **Interactive** (Default) | ✅ (Ask) | ✅ (Ask) | Local development, pair programming. |
| **Batch** | ❌ | ✅ | Background jobs, automated refactoring. |
| **Architect** | ❌ | ❌ | Code audits, read-only analysis. |
| **CI Mode** | ❌ | ✅ | GitHub Actions, headless pipelines. |

**CI Mode:**
To run safely in CI pipelines, use the `--ci` flag. This disables the TUI, enforces the Batch policy, and auto-approves safe file writes.

```bash
codepicker agent run "Lint and fix formatting" --ci

```

---

## 🤖 Advanced Usage

### Planning Mode

For complex tasks, generate a step-by-step plan before execution.

```bash
# Generate a plan
codepicker agent plan "Migrate database from SQLite to Postgres"

# Execute a specific plan ID
codepicker agent run --plan <plan-id>

```

### Batch Processing

Queue multiple tasks to run in the background with worker pools.

```bash
codepicker batch add "Refactor pkg/api"
codepicker batch add "Write tests for pkg/utils"
codepicker batch run --concurrent 2

```

### Architecture Audit

Have the agent perform a deep scan of your codebase and generate a prioritized improvement plan (`ARCHITECTURE_GOALS.md`).

```bash
codepicker agent plan --architect

```

---

## 🔌 Extensibility (MCP & Custom Tools)

Configure extensions in `.codepicker.yml`:

```yaml
# Connect to external data sources (e.g., GitHub, Postgres, Slack)
mcp_servers:
  - name: github
    command: npx
    args: ["-y", "@modelcontextprotocol/server-github"]
    env:
      - "GITHUB_PERSONAL_ACCESS_TOKEN=..."

# Define custom project-specific tools
tools:
  - name: run_linter
    description: "Run the project linter"
    command: "make lint"

```

Validate your configuration and connections:

```bash
codepicker check

```

---

## 📚 Documentation

For a full reference of all commands and flags, generate the CLI documentation:

```bash
codepicker doc --dir docs/cli

```

---

## 📄 License

MIT
