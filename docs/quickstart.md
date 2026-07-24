# CodePicker Quickstart Guide

Get up and running with CodePicker in less than 5 minutes.

## 1. Initialize CodePicker

Scaffold the CodePicker workspace directory structures and create your configuration files in your repository root:

```bash
codepicker init
```

This creates:
- `.codepicker/` (runs history, shadow layer, backups, proof logs)
- `codepicker.yaml` (default configuration)
- `policy.json` (security whitelists and command rules)
- `.codepickerignore` (ignore files matching default excludes)

## 2. Generate a Context Pack

Prepare clean repository context for LLM prompts or autonomous agent workflows:

```bash
codepicker pack --task "review this repo" --output ctx.txt
```

Options:
- `--changed`: Pack only modified/untracked files from Git.
- `--profile go-cli`: Filter context files matching predefined presets (e.g. Go, Node, Python, fullstack).

## 3. Safe Execution (Plan Only)

Generate a high-fidelity step-by-step plan for your task without modifying any files:

```bash
codepicker run "add unit tests for pack" --plan-only
```

Outputs a unique **Plan ID** (e.g., `run-20260526-154419`) and details the planned steps and file changes.

## 4. Sandbox Dry Run

Execute the generated plan against a temporary sandbox/shadow filesystem to verify code compilation and tests:

```bash
codepicker run "add unit tests for pack" --dry-run
```

All writes go to the shadow layer `.codepicker/shadow/` and a unified patch diff is verified in the sandbox. Real project files remain completely unchanged.

## 5. Safe Real Apply

Once the plan executes and passes verification in the sandbox, apply the verified changes to your real project:

```bash
codepicker run "add unit tests for pack" --apply --branch
```

This automatically:
1. Backups original files.
2. Switches to a new Git session branch (`codepicker/add-unit-tests-for-pack-<timestamp>`).
3. Integrates changes atomically to your project files.

## 6. Verification and Proofing

Verify that everything compiles, vets, and passes all tests successfully:

```bash
codepicker prove
```

Prints the complete `CODEPICKER PROOF REPORT` and logs report artifacts under `.codepicker/runs/proof/<timestamp>/`.
