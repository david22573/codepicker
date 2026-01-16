# Codepicker

A CLI tool for harvesting codebase context for AI consumption.

## Installation

```bash
go install [github.com/david22573/codepicker@latest](https://github.com/david22573/codepicker@latest)

```

## Quick Start

```bash
# Initialize configuration
codepicker init

# Generate context for current directory
codepicker

# Ask a question about your codebase
export OPENROUTER_API_KEY=your_key
codepicker ask "How does the authentication work?"

# Start interactive chat
codepicker chat

# Use autonomous agent mode
codepicker do "Add error handling to the API endpoint"

```

## Configuration

Create `.codepicker.yml` in your project root:

```yaml
src: .
output: context.md
minify: true
tokens: false

include:
  - .go
  - .ts
  - .js

exclude:
  - .git
  - node_modules
  - vendor

ai:
  model: xiaomi/mimo-v2-flash:free
  temperature: 0.7

```

## Commands

| Command | Description |
| --- | --- |
| `codepicker` | Generate context file |
| `codepicker ask [query]` | Ask AI about your codebase |
| `codepicker chat` | Interactive chat session |
| `codepicker do [task]` | Autonomous agent mode |
| `codepicker tree` | Print project structure |
| `codepicker copy` | Copy files preserving structure |
| `codepicker serve` | Start agent daemon |
| `codepicker init` | Generate default config |
| `codepicker version` | Print version information |

## Flags

* `-s, --src` - Source directory (default: `.`)
* `-o, --out` - Output file path
* `-m, --minify` - Enable minification (default: `true`)
* `-i, --include` - Comma-separated extensions to include
* `-e, --exclude` - Comma-separated directories to exclude
* `-v, --verbose` - Enable verbose logging

## License

MIT
