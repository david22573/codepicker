# CodePicker Agent Workflow

This guide explains how future AI coders, CLI agents, and developer assistants can leverage CodePicker to perform safe, autonomous refactoring.

## 1. Context Preparation (Offline)

When an agent is loaded into a repository, it should first run the `pack` command to build a clean context payload without junk files:

```bash
codepicker pack --changed --task "describe your task" --output ctx.txt
```

If the agent needs Go-specific code, it can filter using the Go profile:
```bash
codepicker pack --profile go-cli --output ctx.txt
```

The agent should then read `ctx.txt` as its primary input to understand the codebase state.

## 2. Planning Phase

Before writing code or running terminal commands, the agent should generate an execution plan:

```bash
codepicker run "refactor task" --plan-only
```

This returns a Plan ID (e.g. `run-20260526-154419`) and lists all target files. The agent should verify that the targeted files match the intended scope.

## 3. Sandbox Validation (Dry Run)

Next, the agent must run the verification checks in the sandbox:

```bash
codepicker run "refactor task" --dry-run
```

During this step:
1. CodePicker executes the changes on the shadow filesystem.
2. CodePicker verifies that the code builds and tests pass in a safe sandbox.
3. CodePicker logs sandbox outputs to `.codepicker/runs/<run-id>/verifier.log`.

The agent should inspect `verifier.log` if any failures occur to refine its changes.

## 4. Applying Changes

Once the sandbox checks are green, the agent can commit the modifications on a new Git branch:

```bash
codepicker run "refactor task" --apply --branch --ci
```

The `--ci` flag runs in non-interactive mode (skipping prompts), while `--branch` creates an isolated Git branch to keep the main branch clean.

## 5. Proving Safety

At the end of the session, the agent should run prove checks to guarantee the overall integrity of the repository:

```bash
codepicker prove
```
This ensures that the repository compile state, tests, and vetting remain perfectly healthy after the autonomous refactoring turns!
