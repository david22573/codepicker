# Codebase Improvement Plan (Phased)

## Phase 0 — Build Breakers & Silent Failures (🔥 Must Fix First)

**Goal:** Ensure the code compiles, fails safely, and does not hide errors.

### 0.1 Fix invalid API usage (`filepath.HasPrefix`)
- **Problem:** `filepath.HasPrefix` does not exist → compile-time failure.
- **Fix:** Replace with a safe path containment check using `filepath.Rel`.
- **Why:** Prevents accidental recursion into output directories and ensures correctness across platforms.

**Action**
- Update `CopyStrategy.ShouldSkip` to:
  - Normalize paths using `filepath.Abs`
  - Use `filepath.Rel`
  - Treat paths inside `OutputDir` as skipped

---

### 0.2 Stop ignoring filesystem errors
- **Problem:** `os.Open` and `os.Create` errors are ignored.
- **Impact:** Silent data loss, nil dereferences, undefined behavior.
- **Fix:** Propagate errors and ensure directories exist before file creation.

**Action**
- Handle errors explicitly in:
  - `CopyStrategy.Write`
  - Any other file IO with `_ =`

---

### 0.3 Ensure directories exist before writing
- **Problem:** Writing files without guaranteeing parent directories exist.
- **Fix:** `os.MkdirAll(filepath.Dir(targetPath), 0755)` before file creation.

---

## Phase 1 — Network Safety & Cancellation (Security + Reliability)

**Goal:** Prevent hangs, allow user cancellation, and improve API robustness.

### 1.1 Enforce HTTP timeouts
- **Problem:** Default `http.Client` has no timeout.
- **Risk:** Hanging CLI on bad networks.
- **Fix:** Set a sane default timeout (e.g. 30s).

**Action**
- In `NewClient`, ensure:
  &http.Client{ Timeout: 30 * time.Second }

---

### 1.2 Propagate context from CLI to API calls

* **Problem:** Some calls use `context.Background()`.
* **Impact:** Ctrl+C does nothing during streaming or long requests.
* **Fix:** Use `cmd.Context()` everywhere.

**Action**

* Replace all `context.Background()` in CLI commands with command context.
* Ensure OpenRouter client accepts and respects context.

---

### 1.3 Harden retry logic

* **Current:** Retry exists but is simplistic.
* **Improvement:** Add jittered exponential backoff.
* **Why:** Prevents retry storms and improves stability.

---

## Phase 2 — Performance & Resource Efficiency

**Goal:** Reduce unnecessary CPU, memory, and repeated initialization.

### 2.1 Cache tokenizer encoding

* **Problem:** `tiktoken.GetEncoding` called repeatedly.
* **Cost:** Expensive, unnecessary work.
* **Fix:** Cache encoder using `sync.Once`.

**Action**

* Create a singleton encoder in tokenizer package.
* Fail loudly (or log once) if encoding fails instead of silent fallback.

---

### 2.2 Make token fallback explicit

* **Problem:** Silent fallback to `len(text)/4` hides real errors.
* **Fix:** Add:

  * CLI flag or config
  * Warning log on fallback
* **Why:** Token counts affect pricing and truncation decisions.

---

### 2.3 Avoid double buffering large context files

* **Current:** Write temp file → read entire file → send to API.
* **Improvement (optional):**

  * Stream context directly into request body.
* **Tradeoff:** More complexity, better scalability.

---

## Phase 3 — Correctness & Edge-Case Hardening

**Goal:** Ensure predictable behavior across filesystems, languages, and inputs.

### 3.1 Normalize path output consistently

* **Fix:** Use `filepath.ToSlash` for all user-facing paths.
* **Why:** Improves cross-platform UX and diff readability.

---

### 3.2 Strengthen minifier safety guarantees

* **Current:** JS/Python minifiers are heuristic.
* **Fix:**

  * Explicitly document limitations.
  * Add tests for:

    * Template literals
    * Triple-quoted strings
    * Nested comments
* **Policy:** Prefer *under-minifying* over corrupting code.

---

### 3.3 Improve error wrapping

* **Fix:** Wrap errors using `%w` everywhere.
* **Benefit:** Enables `errors.Is` / `errors.As`.
* **Why:** Important for CLI exit codes and future automation.

---

## Phase 4 — Configuration & Extensibility

**Goal:** Prepare the codebase for growth without refactors.

### 4.1 Externalize model cost assumptions

* **Problem:** Token cost estimates are hardcoded.
* **Fix:** Move to:

  * Config file
  * Model metadata map
* **Why:** Providers change pricing frequently.

---

### 4.2 Centralize model & tokenizer metadata

* **Improvement:** One registry for:

  * Model name
  * Tokenizer
  * Context window
  * Pricing
* **Benefit:** Eliminates scattered assumptions.

---

## Phase 5 — Testing, CI, and Tooling

**Goal:** Lock in correctness and prevent regressions.

### 5.1 Add targeted unit tests

**High-value targets**

* `CopyStrategy.ShouldSkip`
* Scanner with `.gitignore` + custom ignore
* Tokenizer fallback behavior
* Minifier edge cases

---

### 5.2 Add CI pipeline

**Recommended**

* `go test ./...`
* `go vet`
* `golangci-lint`

---

### 5.3 Add Makefile targets

```makefile
test:
	go test ./...

lint:
	golangci-lint run

fmt:
	gofmt -w .
```

---

## Phase 6 — Optional UX & DX Enhancements

**Goal:** Improve developer and user experience.

* Warn before copying large directory trees to clipboard.
* Add `--dry-run` for copy mode.
* Add structured output (`--json`) for automation.
* Centralize emoji / CLI formatting for consistency.

---

## Execution Order (TL;DR)

1. **Phase 0** — build correctness & error handling
2. **Phase 1** — network safety & cancellation
3. **Phase 2** — performance & efficiency
4. **Phase 3** — correctness edge cases
5. **Phase 5** — tests & CI
6. **Phase 4 / 6** — extensibility & polish


