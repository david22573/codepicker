# Refactor & Bug Fix Roadmap

## Critical Bugs

1. **Race Condition in Scanner** - `scanner.go` uses concurrent workers writing to shared `Writer` interface without guarantees that all implementations are thread-safe. Only `ConcatStrategy` and `TreeStrategy` have mutexes.

2. **Panic Risk in Agent Engine** - `engine.go` type assertions like `fmt.Sprintf("%v", msg.Content)` can panic if Content is unexpected type. Need safe type handling.

3. **Context Leak in Server** - `server.go` approval goroutine can leak if client disconnects before approval completes. Missing cleanup on context cancellation.

4. **Shadow File Validation Gap** - `shadow/fs.go` checks `strings.Contains(relPath, "..")` but doesn't normalize paths first, allowing traversal via symlinks or `.//` sequences.

## High-Priority Refactors

5. **Duplicate Code in Agents** - `agents/base_agent.go` and `agent/engine.go` have nearly identical tool execution logic. Extract to shared `ToolExecutor` interface.

6. **Config Loading Inconsistency** - Multiple places load config with silent failures (`config.LoadConfigFile("")` returns `nil, nil`). Should fail fast or use singleton pattern.

7. **Error Handling Inconsistency** - Mix of `fmt.Errorf`, custom `errors` package, and plain strings. Standardize on custom error types for better observability.

8. **Hard-coded Model Strings** - Model names scattered across codebase (`"deepseek/deepseek-chat"` appears 4+ times). Centralize in constants with fallback chain.

## Medium-Priority Issues

9. **Database Migration Missing** - `database/store.go` schema has no versioning. Adding columns will break existing DBs. Add migration system.

10. **Token Estimation Fragility** - `tokenizer.go` falls back to `len(text)/4` on error, which is wildly inaccurate for non-English. Log warning or fail explicitly.

11. **Planner Context Explosion** - `planner.go` loads all file paths into single prompt (capped at 1000). Should use embeddings or hierarchical planning for large repos.

12. **Approval Timeout Not Configurable** - Server hardcodes 60s approval timeout. Should respect `AgentTimeout` from limits config.

## Low-Priority/Tech Debt

13. **Test Coverage Gaps** - Core agent logic, planner, and server have zero tests. Add integration test suite.

14. **Logging Verbosity** - Agent emits token-by-token "thought" logs in interactive mode, flooding terminals. Add log level filtering.

15. **Unused Fields** - `Plan.EstimatedCost` calculated but never enforced. Either implement budget checks or remove.

16. **Magic Numbers** - Constants like `MaxShadowSize = 1MB`, `MaxFileSize = 100MB` should be configurable limits, not hardcoded.

---

**Suggested Priority Order:**  
#4 (security) → #1 (correctness) → #3 (stability) → #5 (maintainability) → #6 (reliability) → #9 (future-proofing)
