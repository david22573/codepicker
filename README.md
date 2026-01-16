# codepicker ⛏️

**Codepicker** is an autonomous developer agent and context harvester. It bridges your local filesystem with Large Language Models (LLMs) to analyze code, answer questions, and perform complex refactoring tasks safely.

## ✨ Features

### 🧠 Agentic Capabilities
* **Interactive Chat:** Chat with your codebase (`codepicker chat`) with persistent history.
* **Autonomous Tasks:** Ask the agent to "Refactor the logging" or "Write a unit test" (`codepicker do`).
* **Shadow Workspace:** The agent proposes changes in a sandboxed `.codepicker/shadow` directory. It **never** modifies your code without permission.
* **Review & Apply:** Use `codepicker apply` to review diffs and commit agent changes.

### 🛠️ Core Tools
* **Context Generation:** Scans and minifies code into a single Markdown file optimized for LLM context windows.
* **Smart Selection:** "Smart Mode" uses AI to pick only relevant files for a query, saving tokens.
* **Plugin System:** Extend the agent with custom shell commands defined in `.codepicker.yml`.
* **Git Aware:** Scan only changed files with `--diff` or `--diff staged`.

### ⚙️ Production Ready
* **Daemon Mode:** Run as a server with `codepicker serve`.
* **Observability:** Structured JSON logs and Prometheus metrics at `/metrics`.
* **Security:** Rate limiting, CORS configuration, and file size quotas.

## 🚀 Quick Start

### Installation
```bash
go install [github.com/david22573/codepicker@latest](https://github.com/david22573/codepicker@latest)

```

### Basic Usage

1. **Initialize** configuration:
```bash
codepicker init

```


2. **Generate Context** (for manual LLM pasting):
```bash
codepicker --out context.md

```


3. **Ask a Question**:
```bash
export OPENROUTER_API_KEY=sk-or-...
codepicker ask "Explain how the tokenizer works"

```


4. **Perform a Task**:
```bash
# 1. Agent plans and writes code to shadow dir
codepicker do "Create a new handler for the /health endpoint"

# 2. You review and apply the changes
codepicker apply

```



## 🔌 Configuration & Plugins

Configure behavior in `.codepicker.yml`. You can also define **Custom Tools** that the agent can execute!

```yaml
ai:
  model: xiaomi/mimo-v2-flash:free

# Define custom tools for the agent
tools:
  - name: run_tests
    description: Run the project unit tests
    command: go test ./...
  
  - name: deploy_staging
    description: Deploy the current build to staging
    command: ./scripts/deploy.sh
    args_schema: '{ "properties": { "env": { "type": "string" } } }'

```

## 🐳 Docker & CI/CD

### Run via Docker

```bash
docker run -p 22573:22573 -e OPENROUTER_API_KEY=... ghcr.io/david22573/codepicker serve

```

### GitHub Action

Use Codepicker in your workflow to review PRs or generate documentation:

```yaml
steps:
  - uses: david22573/codepicker@v1
    with:
      openai_key: ${{ secrets.OPENROUTER_KEY }}
      task: "Review this PR and check for security flaws"

```

## 🛡️ Server API

When running `codepicker serve`, the following endpoints are available:

| Method | Endpoint | Description |
| --- | --- | --- |
| `POST` | `/agent/task?q=...` | Stream agent thoughts and actions (SSE) |
| `POST` | `/agent/approve` | Approve/Deny sensitive commands |
| `GET` | `/metrics` | Prometheus metrics (Req count, Cost, Memory) |
| `GET` | `/health` | Health check |

## License

MIT
