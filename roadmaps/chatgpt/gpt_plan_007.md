# ROADMAP: Fully Automated, Guard-Railed Code Fixing

> **Outcome:**
> A single command that:
>
> 1. Builds LLM-optimized context
> 2. Asks Kimi 2.5 to analyze (read-only)
> 3. Asks Kimi 2.5 to propose safe fixes (diff-only)
> 4. Verifies locally
> 5. Applies + commits if safe

---

## PHASE 0 — Preconditions (1–2 hours)

### 0.1 Freeze the context format (non-negotiable)

* Lock the **single LLM-optimized context builder** we defined
* Add a regression test:

  * Same repo → same output hash

Why: If context shifts, autonomy becomes nondeterministic.

---

### 0.2 Introduce a workspace layout

```
.codepicker/
├── runs/
│   └── 2026-01-28T18-42-10/
│       ├── context.txt
│       ├── analysis.txt
│       ├── policy.json
│       ├── patch.diff
│       ├── verify.log
│       └── metadata.json
```

This is your **audit trail**.

---

## PHASE 1 — Machine-Enforced Policy (half day)

### 1.1 Define a minimal policy schema

`policy.json`

```json
{
  "allowed_globs": [
    "adapters/context/**",
    "infra/fs/**"
  ],
  "allowed_issue_types": [
    "compile_error",
    "nil_deref",
    "dead_code",
    "incorrect_import"
  ],
  "forbidden_keywords": [
    "refactor",
    "redesign",
    "architecture",
    "concurrency"
  ]
}
```

LLM **does not interpret policy** — your tool does.

---

### 1.2 Implement policy enforcement (Go)

Core rule:

> If LLM output violates policy → abort, no retry.

You only need:

* Path matching
* Keyword scanning
* Diff file target validation

---

## PHASE 2 — Two-Pass LLM Protocol (1 day)

### 2.1 Analysis pass (read-only)

**Hard rules:**

* No diffs
* No code blocks
* Structured plain text only

Required sections:

```
INVARIANTS
HIGH_CONFIDENCE_ISSUES
MEDIUM_CONFIDENCE_ISSUES
LOW_CONFIDENCE_ISSUES
FILES_AT_RISK
```

Reject output if:

* Any section missing
* Mentions forbidden keywords
* Mentions files outside policy

---

### 2.2 Patch pass (diff-only)

Rules:

* Unified diff only
* One logical patch
* No explanations
* No file creations unless allowed

Reject if:

* Non-diff text exists
* Touches forbidden paths
* Changes public APIs

---

## PHASE 3 — Shadow Execution + Verification (½ day)

### 3.1 Shadow FS staging

* Apply patch to shadow copy
* Never touch working tree yet

### 3.2 Verification pipeline

Run in order:

1. `go fmt ./...`
2. `go vet ./...`
3. `go test ./...`
4. `go build ./...`

Any failure → abort + log

---

## PHASE 4 — Commit + Provenance (1–2 hours)

### 4.1 Auto-commit format

```
fix: automated safe repairs (kimi-2.5)

Context-Hash: <sha256>
Analysis-Hash: <sha256>
Policy-Hash: <sha256>
Model: kimi-2.5
```

This gives you **forensic traceability**.

---

## PHASE 5 — Single Command UX (final polish)

### 5.1 CLI command

```bash
codepicker fix \
  --model kimi-2.5 \
  --policy policy.json \
  --dry-run=false
```

### 5.2 Output states

* ✅ Applied
* ⚠️ No safe fixes found
* ❌ Rejected (policy)
* ❌ Verification failed

Never silent.

---

## PHASE 6 — Hardening (optional but recommended)

### 6.1 Add “analysis-only” mode

```bash
codepicker fix --analyze-only
```

### 6.2 Add patch confidence scoring

* Based on:

  * Issue confidence
  * Diff size
  * File criticality

Block low-confidence patches automatically.

---

## TIMELINE SUMMARY

| Phase         | Time            |
| ------------- | --------------- |
| Preconditions | 2h              |
| Policy system | 4h              |
| LLM protocol  | 1 day           |
| Verification  | 4h              |
| CLI polish    | 2h              |
| **Total**     | **~2–2.5 days** |

