# Findings

## Discoveries & Architecture
- CodePicker is built in Go, using SPF13 Cobra for CLI command definitions.
- Commands are defined under `cmd/`.
- Logic for agent planning/execution is structured under `adapters/` and `infra/` / `domain/`.
- Safe writes are orchestrated via sandbox/shadow layer environments.

## External Resources & References
- Roadmap: [roadmap.md](file:///home/davidmiguel22573/Github/codepicker/roadmap.md)
