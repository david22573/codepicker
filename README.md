# CodePicker

CodePicker is an AI-native codebase harvester and autonomous developer agent. It scans your source code, generates context-optimized prompts for LLMs, and runs a local agent daemon that can read files, plan changes, and execute safe shell commands.

## Features

- 🧠 **Context Generation**: intelligently scans and minifies code for LLM consumption.
- 💬 **Interactive Chat**: Chat with your codebase directly in the terminal.
- 🤖 **Agent Daemon**: Runs a local server that allows AI agents to read/write files and run commands.
- 🛡️ **Secure Sandbox**: Includes path traversal protection, command whitelisting, and cost tracking.

## Installation

```bash
go install [github.com/david22573/codepicker@latest](https://github.com/david22573/codepicker@latest)
