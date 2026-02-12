Below is a **development roadmap written as a drip-feed implementation guide**.

It is structured so you can:

* Paste one section at a time into Gemini Pro
* Implement incrementally
* Validate before moving forward
* Avoid overwhelming the model

---

# 🚀 CodePicker Evolution Roadmap

> Goal: Transform CodePicker from a capable AI agent into a production-grade autonomous coding runtime.

---

# 🧱 Phase 1 — Runtime Hardening (Stability First)

Focus: determinism, safety, reliability.

Do NOT move to Phase 2 until this phase is stable.

---

## ✅ Step 1 — Structured LLM Output Enforcement

### 🎯 Objective

Eliminate brittle regex parsing of JSON responses.

---

### 📌 Task 1.1 — Introduce Structured Response Layer

**Create:**

```
infra/llm/structured.go
```

Define:

```go
type StructuredLLM interface {
    ChatJSON(ctx context.Context, system string, user string, out interface{}) error
}
```

Implementation idea:

* Call existing `Chat`
* Extract first valid `{ ... }`
* Unmarshal into `out`
* If unmarshal fails:

  * Retry with “Your previous response was invalid JSON” repair prompt

---

### 📌 Task 1.2 — Refactor Planner to Use Structured Mode

Modify:

```
adapters/agent/planner.go
```

Replace:

```go
resp, err := p.model.Chat(...)
```

With:

```go
var planData PlanSchema
err := p.model.ChatJSON(ctx, systemPrompt, userMessage, &planData)
```

Remove markdown stripping logic.

---

### 📌 Deliverable Checklist

* [ ] JSON schema struct defined
* [ ] No manual markdown trimming
* [ ] Planner fully schema-driven
* [ ] Invalid JSON triggers repair retry

---

### 🚦 Stop Here and Test

Before continuing:

* Run integration tests
* Force malformed model output
* Confirm graceful recovery

---

# 🧠 Step 2 — Sliding Token Window in ReActAgent

### 🎯 Objective

Prevent context growth explosion.

---

### 📌 Task 2.1 — Introduce Turn Memory

Create:

```go
type Turn struct {
    Thought     string
    Observation string
}

type TurnMemory struct {
    MaxTokens int
    Turns     []Turn
}
```

Add method:

```go
func (m *TurnMemory) Add(turn Turn)
func (m *TurnMemory) Render() string
```

Evict oldest turns when token estimate exceeds limit.

Use:

```
len(text)/4
```

for rough token estimation.

---

### 📌 Task 2.2 — Integrate Into ReActAgent

Replace:

```go
currentContext += ...
```

With:

```go
memory.Add(Turn{...})
currentContext = memory.Render()
```

---

### 📌 Deliverable Checklist

* [ ] No unbounded string concatenation
* [ ] Context never exceeds configured limit
* [ ] Works across 20+ turns

---

### 🚦 Test

Simulate long task with forced many tool calls.

Confirm:

* Token count stabilizes
* No runaway growth

---

# 🔐 Step 3 — Harden run_cmd Execution

### 🎯 Objective

Prevent workspace escape and command injection escalation.

---

### 📌 Task 3.1 — Enforce Working Directory

Modify shell executor:

* Force execution root to shadow workspace
* Reject `-C` flag
* Reject `--work-tree`
* Reject absolute paths outside root

---

### 📌 Task 3.2 — Restrict Network Access (Optional Advanced)

If feasible:

* Add toggle to disable network
* Or wrap execution in containerized sandbox

---

### 📌 Deliverable Checklist

* [ ] Commands cannot escape workspace
* [ ] Path traversal impossible
* [ ] CI mode blocks shell entirely

---

# 🧪 Step 4 — Fuzz Testing Critical Parsers

### 🎯 Objective

Treat LLM output as adversarial input.

---

Add fuzz tests for:

* `ParseBatchActions`
* `parseReActResponse`
* `PolicyEnforcer.CanExecute`

Example:

```go
func FuzzParseBatchActions(f *testing.F) {
    f.Add("Thought: test\nAction: read_file\nInput: {\"path\":\"main.go\"}")
    f.Fuzz(func(t *testing.T, input string) {
        ParseBatchActions(input)
    })
}
```

---

### 📌 Deliverable Checklist

* [ ] Fuzz tests added
* [ ] No panics under random input
* [ ] Parser resilient to malformed output

---

# 🧱 Phase 2 — Reliability & Agent Intelligence

Move here only after Phase 1 stabilizes.

---

# 🔁 Step 5 — Global Self-Repair Layer

### 🎯 Objective

If output parsing fails, auto-retry with corrective prompt.

Add:

```
infra/llm/retry.go
```

Flow:

1. Call model
2. Validate output
3. If invalid:

   * Send correction prompt with exact error
4. Retry up to N times

---

# 🧠 Step 6 — Native Tool Calling Mode

### 🎯 Objective

Remove regex tool parsing entirely.

If Gemini Pro supports function calling:

* Define tool schemas
* Let model return structured tool call
* Execute directly

Benefits:

* Eliminates injection
* No regex parsing
* Deterministic behavior

---

# 📂 Phase 3 — UX Enhancements

Once runtime is solid, enhance developer experience.

---

# ✨ Feature 1 — Plan Preview Mode

Command:

```bash
codepicker plan --preview
```

Displays:

* Reasoning
* Step list
* Estimated cost
* Files impacted

Before execution.

---

# ✨ Feature 2 — Interactive Step Approval

```
🔹 STEP 2/5: Modify planner.go
Approve? (y/n/edit)
```

Allows:

* Step skip
* Manual edit of instruction
* Re-plan from step

---

# ✨ Feature 3 — Execution Timeline View

Expose:

```bash
codepicker history <execution-id>
```

Show:

* Thought chain
* Tool calls
* Cost usage
* Time per step

---

# ✨ Feature 4 — Cost Dashboard

Expose:

```bash
codepicker cost
```

Display:

* Total spend
* Avg per execution
* Most expensive tool
* Turn distribution

---

# ✨ Feature 5 — Dry Run Mode

```
codepicker run --dry
```

* Simulates execution
* No filesystem writes
* Shows intended changes

---

# ✨ Feature 6 — Patch Confidence Score

After patch generation:

* Ask model to critique its own diff
* Output:

  * Risk score
  * Confidence score
  * Test suggestions

---

# 📈 Phase 4 — Advanced Differentiators

These separate you from typical coding agents.

---

## 🔍 1. Deterministic Mode

```
codepicker run --deterministic
```

* Temperature = 0
* Strict JSON schema
* Single-shot planning

Useful for CI pipelines.

---

## 🧠 2. Project Memory Store

Persist:

* Known architecture notes
* Past execution summaries
* Known pitfalls

Stored in SQLite.

Agent references this automatically.

---

## 🔄 3. Multi-Model Strategy

Use:

* Fast model for planning
* Strong model for patching
* Cheap model for explanation

Controlled by cost heuristics.

---

# 📊 Suggested Implementation Order

| Priority | Feature                     |
| -------- | --------------------------- |
| 🔴       | Structured JSON enforcement |
| 🔴       | Sliding window memory       |
| 🔴       | Harden run_cmd              |
| 🟡       | Fuzz tests                  |
| 🟡       | Self-repair retry           |
| 🟡       | Native tool calling         |
| 🟢       | Plan preview UX             |
| 🟢       | Interactive step approval   |
| 🟢       | Cost dashboard              |
| 🔵       | Persistent project memory   |
| 🔵       | Multi-model routing         |

---

# 🧠 Development Strategy with Gemini Pro

When drip-feeding:

1. Provide one section only.
2. Include current file content.
3. Ask for:

   * Refactoring suggestions
   * Safety improvements
   * Edge case analysis
4. Validate manually.
5. Commit.
6. Move to next step.

Never ask it to implement 3+ architectural changes at once.

---

# 🎯 Final Strategic Note

Your system already has:

* Transactional safety
* Policy enforcement
* Observability
* Adaptive budgeting

The biggest gains now come from:

* Deterministic structure
* Token discipline
* Sandboxed execution
* Better UX transparency

If you'd like, I can next generate:

* A CI/CD maturity roadmap
* A scalability roadmap (multi-tenant server mode)
* Or a threat model document for production deployment

