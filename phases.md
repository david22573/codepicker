Nice — this is a well-thought, practical tool and I like how pragmatic the architecture is (pluggable writers/minifiers/scanner + small CLI surface). Below I give a prioritized, actionable code review with **critical fixes**, **quality & safety improvements**, and **nice-to-have upgrades**. I include concrete code snippets/patches you can copy. I kept things terse and focused on correctness, security, and maintainability.

````markdown
# Codebase review — prioritized phases

## Executive summary
- Good modular structure: `scanner`, `writer`, `minifier`, `openrouter` client, CLI with cobra.
- Strong ideas: AST-based Go minifier, streaming client with retry/backoff, token counting.
- Main risks/targets: a few correctness bugs, fragile parsing, error handling gaps, small security/HTTP header mistakes, and some missing test coverage & CI checks.

---

## Phase 1 — **Critical fixes (apply immediately)**

### 1. `pkg/openrouter/client.go` — `Referer` header is wrong
**Problem:** `newRequest` sets header `HTTP-Referer` which is not standard. The HTTP header name should be `Referer`.  
**Fix:** change header name and make sure the header is only set if non-empty.

Patch:
```diff
-    if c.httpReferer != "" {
-        req.Header.Set("HTTP-Referer", c.httpReferer)
-    }
+    if c.httpReferer != "" {
+        req.Header.Set("Referer", c.httpReferer)
+    }
````

### 2. `internal/git/git.go` — staged diff handling incorrect

**Problem:** You treat `diffRef == "staged"` the same as empty; `git diff --name-only` lists unstaged diffs. For staged, you should use `--cached` (or `--staged` alias).
**Fix:** handle "staged" explicitly and prefer `git diff --name-only --cached` for staged changes.

Patch:

```diff
-    if diffRef == "" || diffRef == "staged" {
-        cmd = exec.Command("git", "diff", "--name-only")
-    } else {
-        cmd = exec.Command("git", "diff", "--name-only", diffRef)
-    }
+    if diffRef == "" {
+        cmd = exec.Command("git", "diff", "--name-only")
+    } else if diffRef == "staged" {
+        cmd = exec.Command("git", "diff", "--name-only", "--cached")
+    } else {
+        cmd = exec.Command("git", "diff", "--name-only", diffRef)
+    }
```

Also: prefer `exec.CommandContext(ctx, ...)` with a timeout in callers to avoid long-hanging git calls.

### 3. `pkg/openrouter/chat.go` — retry logic should treat network errors as retryable

**Problem:** `CreateChatCompletionStream` checks `errs.IsRetryable(err)` for network/http.Do errors, but `IsRetryable` expects `*APIError` or `*ScannerError`. Network errors (temporary TCP failures) won't be retried.
**Fix:** detect `net.Error` (temporary) and retry those too.

Example snippet to add before using `errs.IsRetryable`:

```go
if err != nil {
    lastErr = err
    var netErr net.Error
    if errors.As(err, &netErr) && netErr.Temporary() {
        continue
    }
    if errs.IsRetryable(err) {
        continue
    }
    return nil, fmt.Errorf("failed to execute stream request: %w", err)
}
```

(remember to `import "net"`)

### 4. `cmd/ask.go` — fragile JSON parsing of LLM file selection

**Problem:** `callLLMForPaths` strips code fences crudely and then tries two unmarshals; if the model returns text with stray commentary you log a warning but silently fall back. This is brittle.
**Fix:** be more defensive: attempt to extract first JSON object with a regex and return structured error when parsing fails. Log the response at debug level (not warn) to avoid noisy logs.

Suggested improvement (conceptual):

* Use a regex to find first `{...}` block, try to unmarshal that.
* If that fails, attempt to unmarshal an array.
* If still fails, include the raw response in debug logs and return nil.

(You already mostly do this; tighten the fence-stripping and add regex-based extraction.)

### 5. `internal/paths/validator.go` — `Sanitize` returns absolute path; `ValidateOutput` forbids some paths but compares with provided value (should use abs)

**Problem:** `ValidateOutput` compares `out` to system paths but `out` might be relative; ensure `ValidateOutput` receives an absolute path or normalize at start.
**Fix:** either call `Sanitize(out)` before validation or `filepath.Abs` inside `ValidateOutput`. Make check robust.

---

## Phase 2 — **Stability / correctness / maintainability**

### 6. Use `exec.CommandContext` in `git.GetChangedFiles` and other external calls

* Replace `exec.Command` with `exec.CommandContext(ctx, ...)` and pass a context with timeout where appropriate to avoid stuck processes.

Example:

```go
func GetChangedFiles(ctx context.Context, diffRef string) (map[string]bool, error) {
    var cmd *exec.Cmd
    // ...
    cmd = exec.CommandContext(ctx, "git", "diff", "--name-only")
    // ...
}
```

### 7. Normalize path separators consistently

* `writer.TreeStrategy.Write` splits by `os.PathSeparator` while scan code may produce slash-forward paths. Normalize with `filepath.ToSlash` when appropriate to avoid platform-specific bugs.

Small change:

```go
parts := strings.Split(filepath.ToSlash(relPath), "/")
```

### 8. `internal/minifier` — regex & newline handling

* Ensure regex uses `(?m)` multi-line mode when squeezing vertical whitespace: use `regexp.MustCompile("(?m)\\n{3,}")`.
* Avoid relying on platform-specific line endings by normalizing to `\n` early.

Patch example in `SqueezeVerticalWhitespace`:

```go
re := regexp.MustCompile(`(?m)\n{3,}`)
```

### 9. `internal/minifier/js_minifier.go` — comment stripping edge cases

* Current logic may remove `//` inside strings or template literals. Consider a more robust approach (token-based parsing) or at least ignore lines where `//` is inside quotes. If you prefer pragmatic approach, improve detection by scanning for quotes before comment index.

Example heuristic:

* Scan char-by-char, track quote state, escape sequences, and only treat `//` as comment when not inside quotes nor regex literal.

### 10. `internal/tokenizer` — tiktoken fallback

* `GetEncoding("cl100k_base")` may fail in some environments. Make fallback behavior configurable via environment var, and document the approximate fallback used (`len / 4`). Consider returning (tokens, error) so callers can choose fallback vs abort.

### 11. Logging: avoid emojis in machine logs; use stderr for errors

* App logger uses emojis; ok for CLI UX, but consider separate structured logging for machine/CI use (or a --json flag). At minimum, don't log secrets; ensure OpenRouter API keys are never printed.

---

## Phase 3 — **Quality, testing, and UX improvements**

### 12. Add CI and linters

* Add GitHub Actions (or other CI) with:

  * `go vet`, `staticcheck`, `golangci-lint run`
  * `go test ./...`
  * `go fmt` / `gofmt -s`
* This will catch common issues and keep quality high.

### 13. Increase unit test coverage

* Add tests for:

  * `git.GetChangedFiles` behavior for staged/unstaged/diffRef.
  * `minifier` edge cases: JS with strings, templates; Python docstrings with indentation.
  * `openrouter` client: simulate non-2xx responses and stream parsing (use httptest server).
  * `writer` strategies: ensure TreeStrategy path normalization and ConcatStrategy token counting behave as expected.

### 14. Add graceful shutdown and context propagation

* You mostly use contexts, good. Ensure creation of temp files and cleanup uses proper `defer` and log errors on failure (already present). Add signal handler in `main` if you want to ensure long-running ops stop gracefully.

### 15. Improve CLI UX / config

* Allow API key via config file `.codepicker.yml` ai.model and ai.key (if user opts in).
* Support `--dry-run` to preview selected files without making API call.
* Add `--no-emoji` or `--json` flags for machine-friendly output.

### 16. Security: secret scanning & safe defaults

* Add `.gitignore` guidance to prevent `.env` / config with keys being committed (init template already warns—good). Consider integrating `git-secrets` optionally.
* Consider encrypting persisted outputs that may include secrets (e.g., if context includes `.env` accidentally).

---

## Concrete code suggestions / snippets

### Improve `CreateChatCompletionStream` network retry

```go
// pkg/openrouter/chat.go (inside CreateChatCompletionStream)
import "net"

// after httpClient.Do(httpReq) err check:
if err != nil {
    lastErr = err
    var netErr net.Error
    if errors.As(err, &netErr) {
        if netErr.Timeout() || netErr.Temporary() {
            // transient network error - retry
            continue
        }
    }
    if errs.IsRetryable(err) {
        continue
    }
    return nil, fmt.Errorf("failed to execute stream request: %w", err)
}
```

### Safer JSON extract in `callLLMForPaths`

```go
// inside callLLMForPaths, after obtaining content (string)
re := regexp.MustCompile(`(?s)\{.*?\}`)
firstObj := re.FindString(content)
if firstObj != "" {
    var resultObj struct{ Files []string `json:"files"` }
    if err := json.Unmarshal([]byte(firstObj), &resultObj); err == nil && len(resultObj.Files) > 0 {
        return resultObj.Files
    }
}
// fallback to previous array attempt
var paths []string
if err := json.Unmarshal([]byte(content), &paths); err == nil {
    return paths
}
appLogger.Debug(fmt.Sprintf("LLM file-selection response (unparsed): %s", content))
appLogger.Warn("Failed to parse AI planning JSON.")
return nil
```

(remember to `import "regexp"`)

### Normalize relPath use in writer TreeStrategy

```go
// internal/writer/writer.go -> TreeStrategy.Write
rel := filepath.ToSlash(relPath)
parts := strings.Split(rel, "/")
depth := len(parts) - 1
// ...
```

---

## Architectural / design suggestions (higher-level)

* **Pluggable network provider abstraction:** `pkg/openrouter` is good — consider abstracting provider behind interface so you can add other providers later without caller changes.
* **Throttling / rate-limiting:** when scanning and sending large context, provide a way to chunk queries and respect model limits automatically.
* **Deterministic minification pipeline:** steps: read → normalize line endings → minify per-ext → final normalization to `\n`, so tests are consistent across OSes.
* **Security sandboxing:** when scanning large codebases, sensitive files may be included. Provide a default denylist and an explicit `--include-env` or similar flag for risky file types.

---

## Low-effort wins

* Add `go:generate` for embedding sample `.codepicker.yml` into binaries for `--init` templates.
* Add `Makefile` targets for `fmt`, `lint`, `test`.
* Bump `DefaultTimeout` on HTTP client if streaming large responses often; make it configurable via env/flag.
* Add version command/subcommand and embed build info.

---

## Final notes & priorities (one-liner roadmap)

1. Fix `Referer` header, staged `git` diff, and network retry handling (Phase 1).
2. Normalize paths, improve regex/newline handling, and tighten JSON extraction/parsing (Phase 2).
3. Add CI, more tests, and some UX flags (Phase 3).

---

If you want I can:

* produce a small PR patch for the top 5 critical fixes (all ready-to-apply diffs), or
* generate a GitHub Actions workflow for CI plus `golangci-lint` config.

Which of those should I produce next? (I can create the patch or workflow immediately.)

```

Would you like the ready-to-apply patch for the Phase 1 fixes (I’ll include full file diffs)?
```

