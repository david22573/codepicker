# Project Dump Thu Feb  5 22:02:10 PST 2026
## File: adapters/agent/auditor.go
```go
package agent

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/david22573/codepicker/domain/agent"
	"github.com/david22573/codepicker/domain/audit"
)

type Auditor struct {
	model  agent.LLMClient
	repo   agent.Repository
	tools  []agent.Tool
	policy agent.Policy
}

func NewAuditor(model agent.LLMClient, repo agent.Repository, tools []agent.Tool, policy agent.Policy) *Auditor {
	return &Auditor{
		model:  model,
		repo:   repo,
		tools:  tools,
		policy: policy,
	}
}

// SuggestImprovements scans the codebase and returns a list of actionable tasks.
func (a *Auditor) SuggestImprovements(ctx context.Context) ([]string, error) {
	// 1. Construct Read-Only Tools
	toolDescs := ""
	toolMap := make(map[string]agent.Tool)
	for _, t := range a.tools {
		toolMap[t.Name()] = t
		toolDescs += fmt.Sprintf("- %s: %s\n", t.Name(), t.Description())
	}

	// 2. Strict System Prompt (FIXED: Added strict One-Shot Example)
	systemPrompt := fmt.Sprintf(`You are the CodePicker Scout.
Your goal is to scan the codebase and identify 3 SAFE, ISOLATED improvements.
Focus on: Error handling, unused variables, simple refactors, or documentation.

AVAILABLE TOOLS:
%s

RULES:
1. You MUST use tools to see the code. Do not guess.
2. You MUST follow the ReAct format exactly.
3. Your Final Answer must ONLY be the list of tasks.

FORMAT EXAMPLE (Follow this exactly):
Thought: I need to see the file structure.
Action: list_files
Input: {"path": "."}
(System adds Observation...)
Thought: I see main.go. I should check it for errors.
Action: read_file
Input: {"path": "main.go"}
...
Final Answer:
TASK: Fix unhandled error in main.go
TASK: Remove unused import in adapters/parser.go

Begin.`, toolDescs)

	// 3. Run the Agent
	scout := &ReActAgent{
		model:   a.model,
		tools:   toolMap,
		policy:  a.policy, // Strict Read-Only
		repo:    a.repo,
		sysMsg:  systemPrompt,
		maxTurn: 8,
	}

	fmt.Println("📡 [SCOUT] Scanning for improvements...")
	result, err := scout.Run(ctx, "Find 3 safe improvements in the current directory.")
	if err != nil {
		return nil, err
	}

	// 4. Parse the output into a slice
	var tasks []string
	lines := strings.Split(result, "\n")
	for _, line := range lines {
		clean := strings.TrimSpace(line)
		// We look for lines starting with TASK: (case insensitive to be safe)
		if strings.HasPrefix(strings.ToUpper(clean), "TASK:") {
			// Extract the text after "TASK:"
			parts := strings.SplitN(clean, ":", 2)
			if len(parts) == 2 {
				tasks = append(tasks, strings.TrimSpace(parts[1]))
			}
		}
	}

	return tasks, nil
}

// RunAudit performs the detailed security/quality analysis.
func (a *Auditor) RunAudit(ctx context.Context, input string) (*audit.Report, error) {
	// 1. Construct the Auditor Persona
	toolDescs := ""
	toolMap := make(map[string]agent.Tool)
	for _, t := range a.tools {
		toolMap[t.Name()] = t
		toolDescs += fmt.Sprintf("- %s: %s\n", t.Name(), t.Description())
	}

	systemPrompt := fmt.Sprintf(`You are CodePicker-Auditor, a senior security and code quality specialist.
Your goal is to AUDIT the codebase based on the user's request.
You are running in STRICT READ-ONLY MODE. You cannot modify files.

PROCESS:
1. Explore the codebase using available read tools to understand the context.
2. Identify bugs, security vulnerabilities, or architectural issues.
3. Provide a detailed Markdown report as your Final Answer.

AVAILABLE TOOLS:
%s

FORMAT:
Thought: <reasoning>
Action: <tool_name>
Input: <json_args>

Begin.`, toolDescs)

	// 2. Create an Ephemeral Agent for this Audit
	auditAgent := &ReActAgent{
		model:   a.model,
		tools:   toolMap,
		policy:  a.policy, // This must be the ReadOnly policy
		repo:    a.repo,
		sysMsg:  systemPrompt,
		maxTurn: 10,
	}

	// 3. Run the Agent
	fmt.Println("🔍 Auditor starting analysis...")
	result, err := auditAgent.Run(ctx, input)
	if err != nil {
		return nil, err
	}

	// 4. Generate Artifact
	reportID := fmt.Sprintf("audit-%d", time.Now().Unix())
	fileName := fmt.Sprintf("audit_report_%s.md", reportID)
	if err := os.WriteFile(fileName, []byte(result), 0644); err != nil {
		return nil, fmt.Errorf("failed to save audit artifact: %w", err)
	}

	return &audit.Report{
		ID:        reportID,
		Timestamp: time.Now(),
		Content:   result,
		Artifact:  fileName,
	}, nil
}
```

---

## File: adapters/agent/executor.go
```go
package agent

import (
	"context"
	"fmt"

	"github.com/david22573/codepicker/domain/agent"
	"github.com/david22573/codepicker/domain/task"
)

type PlanExecutor struct {
	worker agent.Agent
	repo   agent.Repository
}

func NewPlanExecutor(worker agent.Agent, repo agent.Repository) *PlanExecutor {
	return &PlanExecutor{
		worker: worker,
		repo:   repo,
	}
}

func (e *PlanExecutor) Execute(ctx context.Context, plan *task.Plan) error {
	plan.Status = task.StatusRunning
	_ = e.repo.SavePlan(ctx, plan)

	fmt.Println("\n---------------------------------------------------")
	fmt.Printf("📋 [PLANNER] Plan ID: %s (%d steps)\n", plan.ID, len(plan.Steps))
	fmt.Printf("🎯 [PLANNER] Goal: %s\n", plan.OriginalTask)
	fmt.Println("---------------------------------------------------")

	for _, step := range plan.Steps {
		if step.Status == task.StatusCompleted {
			fmt.Printf("⏭️  [PLANNER] Skipping completed step %d\n", step.ID)
			continue
		}

		fmt.Printf("\n🔹 [PLANNER] STEP %d/%d: %s\n", step.ID, len(plan.Steps), step.Description)

		workerInput := fmt.Sprintf("%s\n\nFocus on these files: %v", step.Instruction, step.Files)

		// The worker now handles its own verbose logging (Agent/System output)
		result, err := e.worker.Run(ctx, workerInput)

		if err != nil {
			fmt.Printf("\n❌ [PLANNER] Step %d Failed.\n", step.ID)
			plan.MarkStepFailed(step.ID, err)
			plan.Status = task.StatusFailed
			_ = e.repo.SavePlan(ctx, plan)
			return err
		}

		fmt.Printf("\n✨ [PLANNER] Step %d Complete.\n", step.ID)
		plan.MarkStepComplete(step.ID, result)
		_ = e.repo.SavePlan(ctx, plan)
	}

	plan.Status = task.StatusCompleted
	return e.repo.SavePlan(ctx, plan)
}
```

---

## File: adapters/agent/explainer.go
```go
package agent

import (
	"context"
	"fmt"
	"strings"

	"github.com/david22573/codepicker/domain/agent"
)

type Explainer struct {
	model agent.LLMClient
	repo  agent.Repository
}

func NewExplainer(model agent.LLMClient, repo agent.Repository) *Explainer {
	return &Explainer{
		model: model,
		repo:  repo,
	}
}

// Explain analyzes a specific execution ID and returns a natural language summary
func (e *Explainer) Explain(ctx context.Context, executionID string) (string, error) {
	// 1. Fetch the raw history
	exec, err := e.repo.GetExecution(ctx, executionID)
	if err != nil {
		return "", fmt.Errorf("failed to load execution: %w", err)
	}

	// 2. Format the history into a readable trace for the LLM
	var trace strings.Builder
	trace.WriteString(fmt.Sprintf("Execution ID: %s\n", exec.ID))
	trace.WriteString(fmt.Sprintf("Status: %s\n\n", exec.Status))

	for _, turn := range exec.History {
		trace.WriteString(fmt.Sprintf("TURN %d:\n", turn.TurnID))
		trace.WriteString(fmt.Sprintf("Thought: %s\n", turn.Thought))
		trace.WriteString(fmt.Sprintf("Action: %s(%s)\n", turn.ToolName, turn.ToolArgs))
		// Truncate output to save tokens, we care about decision flow
		out := turn.ToolOut
		if len(out) > 200 {
			out = out[:200] + "...(truncated)"
		}
		trace.WriteString(fmt.Sprintf("Result: %s\n\n", out))
	}

	// 3. Prompt the LLM for analysis
	systemPrompt := `You are an AI Explainability Specialist. 
Your goal is to analyze the execution trace of an autonomous coding agent.
Explain the agent's strategy, identify any errors in reasoning, and summarize the outcome.
Be concise and objective.`

	userPrompt := fmt.Sprintf("Analyze this execution trace:\n\n%s", trace.String())

	fmt.Println("🤔 Analyzing execution trace...")
	return e.model.Chat(ctx, systemPrompt, userPrompt)
}
```

---

## File: adapters/agent/parser.go
```go
package agent

import (
	"regexp"
	"strings"
)

// Regex to catch XML-style tool usage seen in logs (e.g., <invoke name="...">)
var xmlToolRegex = regexp.MustCompile(`<invoke name="(.*?)">(.*?)</invoke>`)
var xmlArgRegex = regexp.MustCompile(`<parameter name="(.*?)">(.*?)</parameter>`)

// parseReActResponse extracts structured components from various LLM output formats.
func parseReActResponse(resp string) (thought, tool, args string) {
	// STRATEGY 1: Check for XML format
	// This takes priority if the model has drifted into XML mode.
	if xmlToolRegex.MatchString(resp) {
		matches := xmlToolRegex.FindStringSubmatch(resp)
		if len(matches) > 1 {
			tool = matches[1]
			// Parse inner parameters into JSON-like string for the tool executor
			rawArgs := matches[2]
			argMatches := xmlArgRegex.FindAllStringSubmatch(rawArgs, -1)

			// Reconstruct simplistic JSON for the existing tool interface
			jsonBuilder := new(strings.Builder)
			jsonBuilder.WriteString("{")
			for i, m := range argMatches {
				if i > 0 {
					jsonBuilder.WriteString(", ")
				}
				// key: m[1], value: m[2]
				jsonBuilder.WriteString(`"`)
				jsonBuilder.WriteString(m[1])
				jsonBuilder.WriteString(`": "`)
				jsonBuilder.WriteString(m[2])
				jsonBuilder.WriteString(`"`)
			}
			jsonBuilder.WriteString("}")
			args = jsonBuilder.String()

			// Everything before the tag is considered thought
			loc := xmlToolRegex.FindStringIndex(resp)
			if loc != nil {
				thought = strings.TrimSpace(resp[:loc[0]])
			}
			return
		}
	}

	// STRATEGY 2: Standard ReAct Parsing (Original Logic)
	lines := strings.Split(resp, "\n")
	inInput := false
	var inputBuilder strings.Builder

	for _, line := range lines {
		cleanLine := strings.TrimSpace(line)

		// 1. Capture Input (Multi-line handling)
		if inInput {
			inputBuilder.WriteString(line + "\n")
			continue
		}

		// 2. Detect Keywords
		if strings.HasPrefix(cleanLine, "Thought:") {
			val := strings.TrimPrefix(cleanLine, "Thought:")
			thought = strings.TrimSpace(val)
		} else if strings.HasPrefix(cleanLine, "Action:") {
			val := strings.TrimPrefix(cleanLine, "Action:")
			// Remove backticks if model adds them (e.g. `read_file`)
			val = strings.Trim(val, "` ")
			tool = strings.TrimSpace(val)
		} else if strings.HasPrefix(cleanLine, "Input:") {
			val := strings.TrimPrefix(cleanLine, "Input:")
			inInput = true
			inputBuilder.WriteString(val + "\n")
		} else if !inInput && thought == "" && tool == "" {
			// If model forgets "Thought:", treat early text as thought
			thought = cleanLine
		}
	}

	if tool != "" {
		args = cleanInput(inputBuilder.String())
	}
	return
}

// cleanInput removes markdown code blocks and extra whitespace
func cleanInput(raw string) string {
	raw = strings.TrimSpace(raw)
	// Strip markdown code blocks ```json ... ```
	raw = strings.TrimPrefix(raw, "```json")
	raw = strings.TrimPrefix(raw, "```")
	raw = strings.TrimSuffix(raw, "```")
	return strings.TrimSpace(raw)
}
```

---

## File: adapters/agent/planner.go
```go
package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/david22573/codepicker/domain/agent"
	"github.com/david22573/codepicker/domain/errors"
	"github.com/david22573/codepicker/domain/task"
)

type Planner struct {
	model agent.LLMClient
	repo  agent.Repository
}

func NewPlanner(model agent.LLMClient, repo agent.Repository) *Planner {
	return &Planner{
		model: model,
		repo:  repo,
	}
}

// CreatePlan generates a new plan based on the task and file context
// FIX: Updated signature to accept 'fileContext string' instead of '*domain.LLMContext'
func (p *Planner) CreatePlan(ctx context.Context, taskInput, fileContext string) (*task.Plan, error) {
	systemPrompt := `You are the Lead Architect.
Break down the task into sequential steps.
STRICTLY only reference files that exist in the PROJECT CONTEXT.

OUTPUT FORMAT:
Return ONLY raw JSON. Do not include markdown formatting.

EXAMPLE:
{
  "reasoning": "I need to read X to understand Y...",
  "steps": [
    {
      "id": 1,
      "description": "Analyze main.go",
      "instruction": "Read main.go and check imports",
      "files": ["main.go"]
    }
  ],
  "estimated_cost": 0.1
}`

	// Combine the markdown context and the user task into a single string prompt
	userMessage := fmt.Sprintf("PROJECT CONTEXT:\n%s\n\nTASK: %s", fileContext, taskInput)

	resp, err := p.model.Chat(ctx, systemPrompt, userMessage)
	if err != nil {
		return nil, errors.NewLLM("planner.CreatePlan", err)
	}

	cleanResp := strings.TrimSpace(resp)
	cleanResp = strings.TrimPrefix(cleanResp, "```json")
	cleanResp = strings.TrimPrefix(cleanResp, "```")
	cleanResp = strings.TrimSuffix(cleanResp, "```")

	var planData struct {
		Reasoning string      `json:"reasoning"`
		Steps     []task.Step `json:"steps"`
	}

	if err := json.Unmarshal([]byte(cleanResp), &planData); err != nil {
		return nil, errors.NewSystem("planner.CreatePlan", "failed to parse plan JSON: "+cleanResp, err)
	}

	planID := fmt.Sprintf("plan-%d", time.Now().UnixNano())
	domainPlan := task.NewPlan(planID, taskInput, planData.Reasoning)
	domainPlan.Steps = planData.Steps

	for i := range domainPlan.Steps {
		domainPlan.Steps[i].Status = task.StatusPending
	}

	if err := p.repo.SavePlan(ctx, domainPlan); err != nil {
		return nil, fmt.Errorf("failed to persist plan: %w", err)
	}

	return domainPlan, nil
}

// OptimizePlan uses AI to refine an existing plan based on feedback
func (p *Planner) OptimizePlan(ctx context.Context, plan *task.Plan, feedback string) (*task.Plan, error) {
	systemPrompt := `You are the Lead Architect. Refine the plan based on feedback.
OUTPUT FORMAT: Return ONLY raw JSON matching the original structure.`

	planBytes, _ := json.Marshal(plan)

	userMessage := fmt.Sprintf("CURRENT PLAN:\n%s\n\nUSER FEEDBACK: %s\n\nRefine the plan.", string(planBytes), feedback)

	resp, err := p.model.Chat(ctx, systemPrompt, userMessage)
	if err != nil {
		return nil, errors.NewLLM("planner.OptimizePlan", err)
	}

	cleanResp := strings.TrimSpace(resp)
	cleanResp = strings.TrimPrefix(cleanResp, "```json")
	cleanResp = strings.TrimPrefix(cleanResp, "```")
	cleanResp = strings.TrimSuffix(cleanResp, "```")

	var planData struct {
		Reasoning string      `json:"reasoning"`
		Steps     []task.Step `json:"steps"`
	}

	if err := json.Unmarshal([]byte(cleanResp), &planData); err != nil {
		return nil, errors.NewSystem("planner.OptimizePlan", "failed to parse optimized plan", err)
	}

	plan.Reasoning = planData.Reasoning
	plan.Steps = planData.Steps

	plan.Status = task.StatusPending
	for i := range plan.Steps {
		plan.Steps[i].Status = task.StatusPending
	}

	if err := p.repo.SavePlan(ctx, plan); err != nil {
		return nil, err
	}

	return plan, nil
}
```

---

## File: adapters/agent/react.go
```go
package agent

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/david22573/codepicker/domain/agent"
	"github.com/david22573/codepicker/domain/errors"
	"github.com/david22573/codepicker/infra/logging"
	"go.uber.org/zap"
)

// ReActAgent implements domain.agent.Agent using the ReAct pattern
type ReActAgent struct {
	model   agent.LLMClient
	tools   map[string]agent.Tool
	policy  agent.Policy
	repo    agent.Repository
	logger  *logging.Logger
	sysMsg  string
	maxTurn int
}

func NewReActAgent(
	model agent.LLMClient,
	tools []agent.Tool,
	policy agent.Policy,
	repo agent.Repository,
	logger *logging.Logger,
) *ReActAgent {
	toolMap := make(map[string]agent.Tool)
	var toolDescs strings.Builder

	for _, t := range tools {
		toolMap[t.Name()] = t
		toolDescs.WriteString(fmt.Sprintf("- %s: %s\n", t.Name(), t.Description()))
	}

	systemPrompt := fmt.Sprintf(`You are CodePicker, an autonomous coding agent.
You verify every step. You never hallucinate filenames.
You operate in a loop: THOUGHT -> ACTION -> OBSERVATION.

AVAILABLE TOOLS:
%s

FORMAT RULES:
1. Output "Thought:", then "Action:", then "Input:".
2. "Action" must be a single tool name (e.g. read_file).
3. "Input" must be valid JSON single-line.
4. STRICTLY FORBIDDEN: Do not use XML tags like <invoke> or <function_calls>.
5. Do NOT output Markdown code blocks for the whole response.
6. Wait for the [SYSTEM] Observation before proceeding.

EXAMPLE INTERACTION:
Thought: I need to read the main file to understand the entry point.
Action: read_file
Input: {"path": "main.go"}

Begin.`, toolDescs.String())

	return &ReActAgent{
		model:   model,
		tools:   toolMap,
		policy:  policy,
		repo:    repo,
		logger:  logger,
		sysMsg:  systemPrompt,
		maxTurn: 15,
	}
}

func (a *ReActAgent) Name() string {
	return "CodePicker-ReAct"
}

func (a *ReActAgent) Run(ctx context.Context, taskInput string) (string, error) {
	// Create a context-aware logger for this run
	logger := a.logger.WithContext(ctx)

	execID := fmt.Sprintf("exec-%d", time.Now().Unix())
	execution := agent.NewExecution(execID, "adhoc-plan")

	// Log the start of the run
	logger.Info("Agent Run Started",
		zap.String("task", taskInput),
		zap.String("execution_id", execID))

	if err := a.repo.SaveExecution(ctx, execution); err != nil {
		logger.Error("Failed to persist execution start", zap.Error(err))
		return "", err
	}

	currentContext := fmt.Sprintf("TASK: %s\n", taskInput)

	for i := 0; i < a.maxTurn; i++ {

		// Log the turn start
		logger.Debug("Starting Turn", zap.Int("turn", i+1))

		response, err := a.model.Chat(ctx, a.sysMsg, currentContext)
		if err != nil {
			logger.Error("LLM Chat Failed", zap.Error(err))
			return "", errors.NewLLM("agent.Run", err)
		}

		thought, toolName, toolArgs := parseReActResponse(response)

		// Log the Agent's reasoning
		logger.Info("Agent Thought", zap.String("thought", thought))

		if toolName == "" {
			logger.Info("Agent Finished", zap.String("response", response))
			execution.Finish()
			_ = a.repo.SaveExecution(ctx, execution)
			return response, nil
		}

		// Log the intent to act
		logger.Info("Tool Request",
			zap.String("tool", toolName),
			zap.String("args", toolArgs))

		// 1. Policy Check
		allowed, reason := a.policy.CanExecute(toolName, toolArgs)
		if !allowed {
			toolOut := fmt.Sprintf("Error: Policy Violation: %s", reason)

			// Critical Security Log
			logger.Warn("Guardrail Blocked Action",
				zap.String("tool", toolName),
				zap.String("reason", reason))

			currentContext += fmt.Sprintf("\nThought: %s\nAction: %s\nInput: %s\nObservation: %s\n", thought, toolName, toolArgs, toolOut)
			continue
		}

		// 2. Execution
		tool, exists := a.tools[toolName]
		var toolOut string
		startTime := time.Now()

		if !exists {
			toolOut = fmt.Sprintf("Error: Tool '%s' not found.", toolName)
			logger.Warn("Tool Not Found", zap.String("tool", toolName))
		} else {
			// Execute and measure
			toolOut, err = tool.Execute(ctx, toolArgs)
			duration := time.Since(startTime)

			// Log the result using the standardized helper
			logger.LogToolExecution(toolName, toolArgs, duration, err)

			if err != nil {
				toolOut = fmt.Sprintf("Error: %v", err)
			}
		}

		execution.RecordTurn(thought, toolName, toolArgs, toolOut)
		_ = a.repo.SaveExecution(ctx, execution)

		currentContext += fmt.Sprintf("\nThought: %s\nAction: %s\nInput: %s\nObservation: %s\n", thought, toolName, toolArgs, toolOut)
	}

	logger.Error("Max Turns Exceeded")
	return "", errors.NewSystem("agent.Run", fmt.Sprintf("Max turns (%d) exceeded without final answer", a.maxTurn), nil)
}
```

---

## File: adapters/agent/two_pass.go
```go
package agent

import (
	"context"
	"fmt"
	"strings"

	"github.com/david22573/codepicker/domain/agent"
	"github.com/david22573/codepicker/domain/interaction"
)

type TwoPassEngine struct {
	model  agent.LLMClient
	repo   agent.Repository
	tools  []agent.Tool
	policy agent.Policy
}

func NewTwoPassEngine(model agent.LLMClient, repo agent.Repository, tools []agent.Tool, policy agent.Policy) *TwoPassEngine {
	return &TwoPassEngine{
		model:  model,
		repo:   repo,
		tools:  tools,
		policy: policy,
	}
}

// RunAnalysis performs Phase 2.1: The Analyst (Read-Only)
// FIX: Signature now accepts (task, contextFile string) to match cmd/fix.go usage
func (e *TwoPassEngine) RunAnalysis(ctx context.Context, task, contextFile string) (*interaction.Analysis, error) {
	readMap := make(map[string]agent.Tool)
	var toolDescs strings.Builder

	for _, t := range e.tools {
		name := t.Name()
		if name != "write_file" && name != "run_cmd" {
			readMap[name] = t
			toolDescs.WriteString(fmt.Sprintf("- %s: %s\n", name, t.Description()))
		}
	}

	systemPrompt := fmt.Sprintf(`You are the CodePicker Analyst.
Your goal is to diagnose the issue described in the TASK.
You have READ-ONLY access.
Locate the specific lines of code that need changing.

AVAILABLE TOOLS:
%s

FORMAT:
Thought: ...
Action: ...
Input: ...

When you have found the issue, output your findings as the Final Answer.`, toolDescs.String())

	analyst := &ReActAgent{
		model:   e.model,
		tools:   readMap,
		policy:  e.policy,
		repo:    e.repo,
		sysMsg:  systemPrompt,
		maxTurn: 8,
	}

	input := fmt.Sprintf("TASK: %s\n\nStart by reading: %s", task, contextFile)

	fmt.Println("🔍 [PHASE 1] Analyzing context...")
	summary, err := analyst.Run(ctx, input)
	if err != nil {
		return nil, err
	}

	return &interaction.Analysis{
		Markdown: summary,
		Files:    []string{contextFile},
	}, nil
}

// GeneratePatch performs Phase 2.2: The Engineer
func (e *TwoPassEngine) GeneratePatch(ctx context.Context, task string, analysis *interaction.Analysis) (*interaction.Patch, error) {
	systemPrompt := `You are the CodePicker Engineer.
Your goal is to write a Git Unified Diff to fix the issue described.

RULES FOR DIFF GENERATION:
1. Start with 'diff --git a/file b/file'.
2. Use standard '--- a/file' and '+++ b/file' headers.
3. INCLUDE 3 LINES OF CONTEXT around every change. Git apply will fail without context.
4. Do not omit lines with "...".
5. Ensure the indentation matches the original file exactly.

Output ONLY the raw diff content. No markdown wrappers.`

	userPrompt := fmt.Sprintf("TASK: %s\n\nANALYSIS:\n%s", task, analysis.Markdown)

	fmt.Println("📝 [PHASE 2] Generating Patch (Diff-Only)...")
	patchContent, err := e.model.Chat(ctx, systemPrompt, userPrompt)
	if err != nil {
		return nil, err
	}

	return &interaction.Patch{
		Diff: cleanPatch(patchContent),
	}, nil
}

// RefinePatch performs Phase 5: Self-Correction
func (e *TwoPassEngine) RefinePatch(ctx context.Context, task string, analysis *interaction.Analysis, originalDiff string, feedback string) (*interaction.Patch, error) {
	systemPrompt := `You are the CodePicker Repair Engineer.
Your previous patch failed to apply.
Analyze the error message and correct the patch.

RULES:
1. Keep the standard Git Unified Diff format.
2. Fix context lines or indentation based on the error.
3. Output ONLY the corrected raw diff.`

	userPrompt := fmt.Sprintf("TASK: %s\n\nFAILED PATCH:\n%s\n\nERROR:\n%s\n\nCorrected Patch:", task, originalDiff, feedback)

	fmt.Println("🩹 [PHASE 5] Refining Patch...")
	patchContent, err := e.model.Chat(ctx, systemPrompt, userPrompt)
	if err != nil {
		return nil, err
	}

	return &interaction.Patch{
		Diff: cleanPatch(patchContent),
	}, nil
}

func cleanPatch(raw string) string {
	raw = strings.TrimSpace(raw)
	raw = strings.TrimPrefix(raw, "```diff")
	raw = strings.TrimPrefix(raw, "```")
	raw = strings.TrimSuffix(raw, "```")
	return strings.TrimSpace(raw)
}
```

---

## File: adapters/context/builder.go
```go
package context

import (
	"fmt"
	"sort"
	"strings"

	"github.com/david22573/codepicker/domain/context"
)

// SliceBasedBuilder finds the most relevant code chunks for a specific prompt
type SliceBasedBuilder struct {
	store     context.SliceStore
	maxTokens int
}

// NewSliceBasedBuilder initializes the builder with a persistence store and token limit
func NewSliceBasedBuilder(store context.SliceStore, maxTokens int) *SliceBasedBuilder {
	return &SliceBasedBuilder{
		store:     store,
		maxTokens: maxTokens,
	}
}

// BuildForTask extracts keywords from the task and retrieves relevant slices from the store
func (b *SliceBasedBuilder) BuildForTask(taskDescription string) (string, error) {
	keywords := b.extractKeywords(taskDescription)

	// Increased MaxResults to give the ranker more to work with
	query := context.SliceQuery{
		Keywords:   keywords,
		MaxResults: 60,
	}

	slices, err := b.store.Query(query)
	if err != nil {
		return "", fmt.Errorf("failed to query slices: %w", err)
	}

	ranked := b.rankSlices(slices, keywords)
	selected := b.packSlices(ranked, b.maxTokens)

	return b.formatContext(selected), nil
}

// rankSlices scores code chunks based on keyword matches in symbols and content
func (b *SliceBasedBuilder) rankSlices(slices []context.CodeSlice, keywords []string) []context.CodeSlice {
	type scoredSlice struct {
		slice context.CodeSlice
		score int
	}

	scored := make([]scoredSlice, len(slices))
	for i, s := range slices {
		score := 0
		for _, kw := range keywords {
			lowerKw := strings.ToLower(kw)

			// Symbols (func/struct names) are highest priority
			for _, sym := range s.Symbols {
				if strings.Contains(strings.ToLower(sym), lowerKw) {
					score += 20
				}
			}

			// Content matches
			if strings.Contains(strings.ToLower(s.Content), lowerKw) {
				score += 5
			}

			// File path matches
			if strings.Contains(strings.ToLower(s.FilePath), lowerKw) {
				score += 10
			}
		}
		scored[i] = scoredSlice{s, score}
	}

	sort.Slice(scored, func(i, j int) bool {
		return scored[i].score > scored[j].score
	})

	result := make([]context.CodeSlice, len(scored))
	for i, s := range scored {
		result[i] = s.slice
	}
	return result
}

// packSlices fits as many high-scoring slices as possible into the token limit
func (b *SliceBasedBuilder) packSlices(slices []context.CodeSlice, maxTokens int) []context.CodeSlice {
	var selected []context.CodeSlice
	totalTokens := 0

	for _, s := range slices {
		// More conservative estimation: ~3 characters per token for code
		estTokens := len(s.Content) / 3
		if totalTokens+estTokens > maxTokens {
			continue
		}
		selected = append(selected, s)
		totalTokens += estTokens
	}

	return selected
}

// formatContext renders the selected slices into a single Markdown block
func (b *SliceBasedBuilder) formatContext(slices []context.CodeSlice) string {
	var sb strings.Builder
	sb.WriteString("# RELEVANT CODE CONTEXT\n")
	sb.WriteString("The following code units were selected based on your current task.\n\n")

	byFile := make(map[string][]context.CodeSlice)
	for _, s := range slices {
		byFile[s.FilePath] = append(byFile[s.FilePath], s)
	}

	for path, fileSlices := range byFile {
		sb.WriteString(fmt.Sprintf("## File: %s\n", path))
		for _, s := range fileSlices {
			sb.WriteString(fmt.Sprintf("### %s (Lines %d-%d)\n", s.SliceType, s.StartLine, s.EndLine))
			sb.WriteString("```go\n")
			sb.WriteString(s.Content)
			sb.WriteString("\n```\n")
		}
		sb.WriteString("\n---\n")
	}

	return sb.String()
}

// extractKeywords cleans the task input for better search matching
func (b *SliceBasedBuilder) extractKeywords(text string) []string {
	stopWords := map[string]bool{"the": true, "for": true, "fix": true, "add": true, "and": true, "with": true, "how": true}
	words := strings.Fields(strings.ToLower(text))
	var keywords []string
	for _, w := range words {
		w = strings.Trim(w, ".,!?;:\"'")
		if len(w) > 2 && !stopWords[w] {
			keywords = append(keywords, w)
		}
	}
	return keywords
}
```

---

## File: adapters/context/builder_test.go
```go
package context

import (
	"strings"
	"testing"

	"github.com/david22573/codepicker/domain/context"
)

// MockSliceStore allows us to test the builder without a real DB
type MockSliceStore struct {
	slices []context.CodeSlice
}

func (m *MockSliceStore) IndexFile(path string, slices []context.CodeSlice) error { return nil }
func (m *MockSliceStore) InvalidateFile(path string) error                        { return nil }
func (m *MockSliceStore) GetByID(id string) (*context.CodeSlice, error)           { return nil, nil }
func (m *MockSliceStore) GetByFile(path string) ([]context.CodeSlice, error)      { return nil, nil }
func (m *MockSliceStore) GetStats() (*context.IndexStats, error)                  { return nil, nil }

func (m *MockSliceStore) Query(q context.SliceQuery) ([]context.CodeSlice, error) {
	var results []context.CodeSlice
	for _, s := range m.slices {
		for _, kw := range q.Keywords {
			if strings.Contains(strings.ToLower(s.Content), strings.ToLower(kw)) ||
				strings.Contains(strings.ToLower(s.FilePath), strings.ToLower(kw)) {
				results = append(results, s)
				break
			}
		}
	}
	return results, nil
}

func (m *MockSliceStore) GetBySymbol(symbol string) ([]context.CodeSlice, error) {
	var results []context.CodeSlice
	for _, s := range m.slices {
		for _, sym := range s.Symbols {
			if sym == symbol {
				results = append(results, s)
			}
		}
	}
	return results, nil
}

func TestBuildForTask(t *testing.T) {
	// 1. Setup mock data
	mockSlices := []context.CodeSlice{
		{
			ID:        "1",
			FilePath:  "cmd/run.go",
			Content:   "func RunAgent() { fmt.Println(\"running\") }",
			Symbols:   []string{"RunAgent"},
			SliceType: context.SliceTypeFunction,
		},
		{
			ID:        "2",
			FilePath:  "infra/llm/client.go",
			Content:   "type LLMClient struct { APIKey string }",
			Symbols:   []string{"LLMClient"},
			SliceType: context.SliceTypeStruct,
		},
	}

	store := &MockSliceStore{slices: mockSlices}
	// Test with a 1000 token budget (plenty for these small slices)
	builder := NewSliceBasedBuilder(store, 1000)

	t.Run("Should include relevant slices based on task keywords", func(t *testing.T) {
		ctx, err := builder.BuildForTask("Fix the RunAgent function")
		if err != nil {
			t.Fatalf("BuildForTask failed: %v", err)
		}

		if !strings.Contains(ctx, "RunAgent") {
			t.Error("Expected context to contain 'RunAgent' slice")
		}
		if strings.Contains(ctx, "LLMClient") {
			t.Error("Did not expect context to contain unrelated 'LLMClient' slice")
		}
	})

	t.Run("Should pack slices within token budget", func(t *testing.T) {
		// Set a very tiny budget that only fits one slice
		smallBuilder := NewSliceBasedBuilder(store, 5)
		ctx, err := smallBuilder.BuildForTask("RunAgent and LLMClient")
		if err != nil {
			t.Fatalf("BuildForTask failed: %v", err)
		}

		// Grouping by file adds text, but we verify packing logic doesn't crash
		// and at least one relevant piece is attempted.
		if ctx == "" {
			t.Log("Context was empty due to very small budget (expected)")
		}
	})
}

func TestKeywordExtraction(t *testing.T) {
	builder := &SliceBasedBuilder{}
	input := "Fix the broken error handling in executor.go!"
	keywords := builder.extractKeywords(input)

	expected := []string{"broken", "error", "handling", "executor.go"}

	for _, exp := range expected {
		found := false
		for _, kw := range keywords {
			if kw == exp {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Keyword extraction failed: expected to find %s in %v", exp, keywords)
		}
	}
}
```

---

## File: adapters/context/config.go
```go
package context

type Config struct {
	ProjectRoot     string
	MaxTokens       int
	IncludePatterns []string
	ExcludePatterns []string
	TaskDescription string
}
```

---

## File: adapters/policy/config.go
```go
package policy

import (
	"encoding/json"
	"os"
)

// PolicySchema defines the structure of policy.json
type PolicySchema struct {
	AllowedGlobs      []string `json:"allowed_globs"`
	AllowedIssueTypes []string `json:"allowed_issue_types"`
	ForbiddenKeywords []string `json:"forbidden_keywords"`
}

// DefaultPolicy returns a safe baseline if no file exists
func DefaultPolicy() PolicySchema {
	return PolicySchema{
		// Default to allowing everything in src, but protecting sensitive dirs
		AllowedGlobs: []string{
			"**/*.go",
			"**/*.md",
			"Makefile",
		},
		ForbiddenKeywords: []string{
			"rm -rf",
			"drop table",
		},
	}
}

// LoadPolicy loads policy.json from the path, or returns default if missing
func LoadPolicy(path string) (*PolicySchema, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		def := DefaultPolicy()
		return &def, nil
	}
	if err != nil {
		return nil, err
	}

	var schema PolicySchema
	if err := json.Unmarshal(data, &schema); err != nil {
		return nil, err
	}
	return &schema, nil
}
```

---

## File: adapters/policy/enforcer.go
```go
package policy

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
)

// Enforcer implements agent.Policy with robust regex-based rules.
type Enforcer struct {
	config           PolicySchema
	readOnly         bool
	forbiddenRegex   []*regexp.Regexp
	commandWhitelist map[string]bool
}

// NewEnforcer creates a production-hardened policy engine.
func NewEnforcer(config PolicySchema, readOnly bool) *Enforcer {
	// Compile forbidden patterns into regex for robust matching
	var regexList []*regexp.Regexp
	for _, keyword := range config.ForbiddenKeywords {
		// Make pattern whitespace-insensitive (e.g., "rm -rf" matches "rm    -rf")
		pattern := strings.ReplaceAll(regexp.QuoteMeta(keyword), " ", `\s+`)
		regex, err := regexp.Compile(`(?i)` + pattern)
		if err == nil {
			regexList = append(regexList, regex)
		}
	}

	// Initialize the command whitelist from roadmap recommendations
	whitelist := map[string]bool{
		"go fmt":     true,
		"go test":    true,
		"go build":   true,
		"go mod":     true,
		"git status": true,
		"git diff":   true,
		"ls":         true,
	}

	return &Enforcer{
		config:           config,
		readOnly:         readOnly,
		forbiddenRegex:   regexList,
		commandWhitelist: whitelist,
	}
}

func (e *Enforcer) Mode() string {
	if e.readOnly {
		return "guarded-readonly"
	}
	return "guarded-active"
}

// CanExecute enforces the JSON policy rules on tool usage.
func (e *Enforcer) CanExecute(toolName string, args string) (bool, string) {
	// 1. Check Global Read-Only Mode
	if e.readOnly {
		if toolName == "write_file" || toolName == "run_cmd" {
			return false, "BLOCKED: Running in READ-ONLY mode."
		}
	}

	// 2. Regex-based Forbidden Pattern Matching
	for _, regex := range e.forbiddenRegex {
		if regex.MatchString(args) {
			return false, fmt.Sprintf("BLOCKED: Forbidden pattern detected: %s", regex.String())
		}
	}

	// 3. Tool-Specific Validation
	switch toolName {
	case "run_cmd":
		return e.validateCommand(args)
	case "write_file", "read_file":
		return e.validateFileSystemAccess(toolName, args)
	}

	return true, ""
}

func (e *Enforcer) validateCommand(args string) (bool, string) {
	var input struct {
		Command string `json:"command"`
	}
	if err := json.Unmarshal([]byte(args), &input); err != nil {
		return false, "BLOCKED: Invalid JSON for run_cmd"
	}

	cleanCmd := strings.TrimSpace(input.Command)
	if cleanCmd == "" {
		return false, "BLOCKED: Command cannot be empty"
	}

	// Extract base command (e.g., "go test" from "go test ./...")
	parts := strings.Fields(cleanCmd)
	if len(parts) == 0 {
		return false, "BLOCKED: Malformed command"
	}

	baseCmd := parts[0]
	if len(parts) > 1 && (baseCmd == "go" || baseCmd == "git") {
		baseCmd = baseCmd + " " + parts[1]
	}

	if !e.commandWhitelist[baseCmd] {
		return false, fmt.Sprintf("BLOCKED: Command '%s' is not in the whitelist", baseCmd)
	}

	return true, ""
}

func (e *Enforcer) validateFileSystemAccess(toolName, args string) (bool, string) {
	var input struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal([]byte(args), &input); err != nil {
		return false, fmt.Sprintf("BLOCKED: Invalid JSON for %s", toolName)
	}

	if input.Path == "" {
		return false, "BLOCKED: Path argument is missing"
	}

	// Existing Path Traversal Check
	if strings.Contains(input.Path, "..") {
		return false, "BLOCKED: Path traversal (..) detected"
	}

	if !e.isPathAllowed(input.Path) {
		return false, fmt.Sprintf("BLOCKED: Path '%s' is not in allowed_globs.", input.Path)
	}

	return true, ""
}

func (e *Enforcer) isPathAllowed(path string) bool {
	cleanPath := filepath.ToSlash(filepath.Clean(path))
	for _, pattern := range e.config.AllowedGlobs {
		if strings.HasSuffix(pattern, "/**") {
			prefix := strings.TrimSuffix(pattern, "**")
			if strings.HasPrefix(cleanPath, prefix) {
				return true
			}
			continue
		}
		matched, _ := filepath.Match(pattern, cleanPath)
		if matched {
			return true
		}
	}
	return false
}
```

---

## File: adapters/policy/strict.go
```go
package policy

import (
	"strings"
)

type StrictPolicy struct {
	blockedCmds []string
	readOnly    bool
	ciMode      bool
}

// NewStrictPolicy creates a policy instance.
func NewStrictPolicy(readOnly, ciMode bool) *StrictPolicy {
	return &StrictPolicy{
		readOnly: readOnly,
		ciMode:   ciMode,
		blockedCmds: []string{
			"rm -rf /", "rm -rf ~", "sudo", "su ", ":(){ :|:& };:", // Fork bomb
			"mkfs", "dd if=/dev",
		},
	}
}

func (p *StrictPolicy) Mode() string {
	if p.ciMode {
		return "ci-hardened"
	}
	if p.readOnly {
		return "strict-readonly"
	}
	return "strict"
}

func (p *StrictPolicy) CanExecute(toolName string, args string) (bool, string) {
	// 0. Global CI / Read-Only Check
	if p.ciMode || p.readOnly {
		// Strictly block side effects
		if toolName == "write_file" {
			return false, "BLOCKED (CI/Read-Only): File writes are disabled in this mode."
		}
		if toolName == "run_cmd" {
			return false, "BLOCKED (CI/Read-Only): Shell commands are disabled in this mode."
		}
	}

	// 1. Policy on Shell Commands
	if toolName == "run_cmd" {
		for _, blocked := range p.blockedCmds {
			if strings.Contains(args, blocked) {
				return false, "Command contains blocked pattern: " + blocked
			}
		}
	}

	// 2. Policy on File Writes
	if toolName == "write_file" {
		if strings.Contains(args, "..") {
			return false, "Path traversal (..) is not allowed"
		}
		if strings.Contains(args, "/.git/") {
			return false, "Modifying .git internals is prohibited"
		}
	}

	return true, ""
}
```

---

## File: adapters/tools/fs.go
```go
package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/david22573/codepicker/domain/errors"
	"github.com/david22573/codepicker/infra/fs"
)

// --- WriteFileTool ---

// WriteFileTool allows the agent to write content to the shadow filesystem
type WriteFileTool struct {
	shadow *fs.ShadowManager
}

func NewWriteFileTool(s *fs.ShadowManager) *WriteFileTool {
	return &WriteFileTool{shadow: s}
}

func (t *WriteFileTool) Name() string { return "write_file" }

func (t *WriteFileTool) Description() string {
	return `Write content to a file. Input JSON: {"path": "string", "content": "string"}`
}

func (t *WriteFileTool) Execute(ctx context.Context, args string) (string, error) {
	var input struct {
		Path    string `json:"path"`
		Content string `json:"content"`
	}
	if err := json.Unmarshal([]byte(args), &input); err != nil {
		return "", errors.NewValidation("tool.write_file", "invalid JSON arguments")
	}

	if input.Path == "" || input.Content == "" {
		return "", errors.NewValidation("tool.write_file", "path and content are required")
	}

	// Writes to the shadow directory to prevent direct unverified modification
	savedPath, err := t.shadow.Write(input.Path, []byte(input.Content))
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("Success. File written to shadow path: %s", savedPath), nil
}

// --- ReadFileTool ---

// ReadFileTool allows the agent to read content, prioritizing the shadow filesystem
type ReadFileTool struct {
	shadow *fs.ShadowManager
}

func NewReadFileTool(s *fs.ShadowManager) *ReadFileTool {
	return &ReadFileTool{shadow: s}
}

func (t *ReadFileTool) Name() string { return "read_file" }

func (t *ReadFileTool) Description() string {
	return `Read file content. Input JSON: {"path": "string"}`
}

func (t *ReadFileTool) Execute(ctx context.Context, args string) (string, error) {
	var input struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal([]byte(args), &input); err != nil {
		return "", errors.NewValidation("tool.read_file", "invalid JSON arguments")
	}

	// Reads from shadow if it exists, otherwise falls back to real file
	content, err := t.shadow.Read(input.Path)
	if err != nil {
		return "", err
	}

	return string(content), nil
}

// --- ListFilesTool ---

// ListFilesTool provides a recursive directory listing
type ListFilesTool struct {
	projectRoot string
}

func NewListFilesTool(root string) *ListFilesTool {
	return &ListFilesTool{projectRoot: root}
}

func (t *ListFilesTool) Name() string { return "list_files" }

func (t *ListFilesTool) Description() string {
	return `List files in a directory (recursive, ignores .git). Input JSON: {"path": "string"}`
}

func (t *ListFilesTool) Execute(ctx context.Context, args string) (string, error) {
	var input struct {
		Path string `json:"path"`
	}
	// Default to project root if no path is provided
	if err := json.Unmarshal([]byte(args), &input); err != nil || input.Path == "" {
		input.Path = "."
	}

	targetDir := filepath.Join(t.projectRoot, input.Path)
	var files []string

	// Walk the directory and capture all non-hidden files
	err := filepath.Walk(targetDir, func(path string, info os.FileInfo, err error) error {
		// FIX: We now propagate the error so the agent knows if a directory is inaccessible
		if err != nil {
			return fmt.Errorf("access error at %s: %v", path, err)
		}

		// Skip hidden files and directories like .git
		if strings.Contains(path, "/.") || strings.Contains(path, "\\.") {
			if info.IsDir() && info.Name() != "." {
				return filepath.SkipDir
			}
			return nil
		}

		if !info.IsDir() {
			rel, _ := filepath.Rel(t.projectRoot, path)
			files = append(files, rel)
		}
		return nil
	})

	if err != nil {
		return "", errors.NewSystem("tool.list_files", "failed to walk directory", err)
	}

	if len(files) == 0 {
		return "No files found.", nil
	}

	// Truncate output for very large projects to avoid token overflow
	if len(files) > 100 {
		return fmt.Sprintf("Found %d files. Showing first 100:\n%s\n... (truncated)", len(files), strings.Join(files[:100], "\n")), nil
	}

	return strings.Join(files, "\n"), nil
}
```

---

## File: adapters/tools/registry.go
```go
package tools

import (
	"github.com/david22573/codepicker/domain/agent"
	"github.com/david22573/codepicker/infra/fs"
	"github.com/david22573/codepicker/infra/shell"
)

// DefaultSet returns the standard set of tools for the coding agent
func DefaultSet(
	fsTools *fs.ShadowManager,
	shellExec *shell.Executor,
	root string,
) []agent.Tool {
	return []agent.Tool{
		NewReadFileTool(fsTools),
		NewWriteFileTool(fsTools),
		NewListFilesTool(root),
		NewSearchTool(root),
		NewSkeletonTool(root),
		NewDefinitionSearchTool(root),
		NewShellTool(shellExec),
	}
}
```

---

## File: adapters/tools/search.go
```go
package tools

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/david22573/codepicker/domain/errors"
)

type SearchTool struct {
	projectRoot string
}

func NewSearchTool(root string) *SearchTool {
	return &SearchTool{projectRoot: root}
}

func (t *SearchTool) Name() string { return "search_code" }
func (t *SearchTool) Description() string {
	return `Search for a string in non-binary files. Input JSON: {"query": "string", "path": "string"}`
}

func (t *SearchTool) Execute(ctx context.Context, args string) (string, error) {
	var input struct {
		Query string `json:"query"`
		Path  string `json:"path"`
	}
	if err := json.Unmarshal([]byte(args), &input); err != nil {
		return "", errors.NewValidation("tool.search_code", "invalid JSON arguments")
	}

	if input.Query == "" {
		return "", errors.NewValidation("tool.search_code", "query cannot be empty")
	}
	if input.Path == "" {
		input.Path = "."
	}

	targetDir := filepath.Join(t.projectRoot, input.Path)
	var results strings.Builder
	matchCount := 0

	err := filepath.Walk(targetDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Skip unreadable
		}
		if info.IsDir() {
			// Skip hidden dirs
			if strings.HasPrefix(info.Name(), ".") && info.Name() != "." {
				return filepath.SkipDir
			}
			return nil
		}
		// Skip hidden files
		if strings.HasPrefix(info.Name(), ".") {
			return nil
		}

		// Simple binary check (skip known binary extensions or huge files)
		ext := strings.ToLower(filepath.Ext(path))
		if isBinaryExt(ext) {
			return nil
		}

		// Read file
		file, err := os.Open(path)
		if err != nil {
			return nil
		}
		defer file.Close()

		scanner := bufio.NewScanner(file)
		lineNum := 1

		relPath, _ := filepath.Rel(t.projectRoot, path)

		for scanner.Scan() {
			line := scanner.Text()
			if strings.Contains(line, input.Query) {
				results.WriteString(fmt.Sprintf("%s:%d: %s\n", relPath, lineNum, strings.TrimSpace(line)))
				matchCount++
				if matchCount > 100 {
					results.WriteString("... (limit reached)\n")
					return filepath.SkipDir // Stop searching to save token limit
				}
			}
			lineNum++
		}
		return nil
	})

	if err != nil {
		return "", errors.NewSystem("tool.search_code", "walk failed", err)
	}

	if matchCount == 0 {
		return "No matches found.", nil
	}
	return results.String(), nil
}

func isBinaryExt(ext string) bool {
	switch ext {
	case ".exe", ".dll", ".so", ".dylib", ".bin", ".png", ".jpg", ".jpeg", ".gif", ".ico", ".pdf", ".zip", ".tar", ".gz":
		return true
	default:
		return false
	}
}
```

---

## File: adapters/tools/search_ast.go
```go
package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"

	"github.com/david22573/codepicker/domain/errors"
)

type DefinitionSearchTool struct {
	projectRoot string
}

func NewDefinitionSearchTool(root string) *DefinitionSearchTool {
	return &DefinitionSearchTool{projectRoot: root}
}

func (t *DefinitionSearchTool) Name() string { return "search_definition" }
func (t *DefinitionSearchTool) Description() string {
	return `Find the definition of a specific Go symbol (function, struct, interface). 
Input JSON: {"name": "SymbolName"}`
}

func (t *DefinitionSearchTool) Execute(ctx context.Context, args string) (string, error) {
	var input struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal([]byte(args), &input); err != nil {
		return "", errors.NewValidation("tool.search_definition", "invalid JSON arguments")
	}

	if input.Name == "" {
		return "", errors.NewValidation("tool.search_definition", "symbol name is required")
	}

	fset := token.NewFileSet()
	var results strings.Builder
	foundCount := 0

	err := filepath.Walk(t.projectRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		// Skip non-Go files and hidden directories
		if info.IsDir() {
			if strings.HasPrefix(info.Name(), ".") && info.Name() != "." {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}

		// Parse the file
		node, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
		if err != nil {
			// Skip unparsable files (tolerant of syntax errors in work-in-progress)
			return nil
		}

		// Inspect the AST for the symbol
		ast.Inspect(node, func(n ast.Node) bool {
			var matchType string
			var matchName string
			var matchPos token.Pos

			switch x := n.(type) {
			case *ast.FuncDecl:
				if x.Name.Name == input.Name {
					matchName = x.Name.Name
					matchPos = x.Pos()
					if x.Recv != nil {
						matchType = "Method"
					} else {
						matchType = "Function"
					}
				}
			case *ast.TypeSpec:
				if x.Name.Name == input.Name {
					matchName = x.Name.Name
					matchPos = x.Pos()
					matchType = "Type"
				}
			}

			if matchName != "" {
				relPath, _ := filepath.Rel(t.projectRoot, path)
				position := fset.Position(matchPos)

				// Capture the exact line of code
				lineContent := getLine(path, position.Line)

				results.WriteString(fmt.Sprintf("[%s] %s:%d\n%s\n\n", matchType, relPath, position.Line, lineContent))
				foundCount++
			}
			return true
		})

		return nil
	})

	if err != nil {
		return "", errors.NewSystem("tool.search_definition", "walk failed", err)
	}

	if foundCount == 0 {
		return fmt.Sprintf("No definitions found for symbol '%s'.", input.Name), nil
	}

	return results.String(), nil
}

// Helper to read a specific line for context
func getLine(path string, lineNum int) string {
	content, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	lines := strings.Split(string(content), "\n")
	if lineNum > 0 && lineNum <= len(lines) {
		return strings.TrimSpace(lines[lineNum-1])
	}
	return ""
}
```

---

## File: adapters/tools/skeleton.go
```go
package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"

	"github.com/david22573/codepicker/domain/errors"
)

type SkeletonTool struct {
	projectRoot string
}

func NewSkeletonTool(root string) *SkeletonTool {
	return &SkeletonTool{projectRoot: root}
}

func (t *SkeletonTool) Name() string { return "read_skeleton" }
func (t *SkeletonTool) Description() string {
	return `Read the structure (skeleton) of a Go file or directory without implementation details. 
Input JSON: {"path": "string"}`
}

func (t *SkeletonTool) Execute(ctx context.Context, args string) (string, error) {
	var input struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal([]byte(args), &input); err != nil {
		return "", errors.NewValidation("tool.read_skeleton", "invalid JSON arguments")
	}

	if input.Path == "" {
		input.Path = "."
	}

	targetPath := filepath.Join(t.projectRoot, input.Path)
	info, err := os.Stat(targetPath)
	if err != nil {
		return "", errors.NewValidation("tool.read_skeleton", "path not found")
	}

	var results strings.Builder
	fset := token.NewFileSet()

	// Handler for a single file
	processFile := func(currPath string) error {
		if !strings.HasSuffix(currPath, ".go") {
			return nil
		}

		node, err := parser.ParseFile(fset, currPath, nil, parser.ParseComments)
		if err != nil {
			return nil // Skip unparsable
		}

		// Prune the AST: Remove function bodies
		ast.Inspect(node, func(n ast.Node) bool {
			if fn, ok := n.(*ast.FuncDecl); ok {
				fn.Body = nil // Remove implementation
			}
			return true
		})

		// Render the skeleton back to source code
		var buf bytes.Buffer
		if err := format.Node(&buf, fset, node); err != nil {
			return err
		}

		relPath, _ := filepath.Rel(t.projectRoot, currPath)
		results.WriteString(fmt.Sprintf("## Skeleton: %s\n```go\n%s\n```\n\n", relPath, buf.String()))
		return nil
	}

	// Logic for Dir vs File
	if !info.IsDir() {
		if err := processFile(targetPath); err != nil {
			return "", err
		}
	} else {
		// Walk the directory (non-recursive to keep it focused, or shallow recursive)
		files, err := os.ReadDir(targetPath)
		if err != nil {
			return "", err
		}
		for _, f := range files {
			if !f.IsDir() {
				_ = processFile(filepath.Join(targetPath, f.Name()))
			}
		}
	}

	if results.Len() == 0 {
		return "No Go files found to skeletonize.", nil
	}

	return results.String(), nil
}
```

---

## File: adapters/tools/terminal.go
```go
package tools

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/david22573/codepicker/domain/errors"
	"github.com/david22573/codepicker/infra/shell"
)

type ShellTool struct {
	exec *shell.Executor
}

func NewShellTool(e *shell.Executor) *ShellTool {
	return &ShellTool{exec: e}
}

func (t *ShellTool) Name() string { return "run_cmd" }
func (t *ShellTool) Description() string {
	return `Execute a shell command. Input JSON: {"command": "string"}`
}

func (t *ShellTool) Execute(ctx context.Context, args string) (string, error) {
	var input struct {
		Command string `json:"command"`
	}
	if err := json.Unmarshal([]byte(args), &input); err != nil {
		return "", errors.NewValidation("tool.run_cmd", "invalid JSON arguments")
	}

	// Basic safety check is handled by Policy, but we can have a fallback here
	if strings.TrimSpace(input.Command) == "" {
		return "", errors.NewValidation("tool.run_cmd", "command cannot be empty")
	}

	// We pass the command to bash to allow pipes and simple logic
	return t.exec.Run(ctx, "bash", "-c", input.Command)
}
```

---

## File: adapters/verifier/pipeline.go
```go
package verifier

import (
	"context"
	"fmt"
	"os/exec"

	"github.com/david22573/codepicker/infra/fs"
)

// Pipeline manages the verification steps for a proposed patch.
type Pipeline struct {
	ProjectRoot string
}

func NewPipeline(root string) *Pipeline {
	return &Pipeline{ProjectRoot: root}
}

// VerifyResult holds the outcome of the verification process.
type VerifyResult struct {
	Success bool
	Stage   string // which stage failed (e.g., "go test")
	Logs    string // output from the failed command
}

// Verify creates a sandbox, applies the patch, and runs the standard Go checks.
func (p *Pipeline) Verify(ctx context.Context, patchDiff string) (*VerifyResult, error) {
	fmt.Println("🧪 [VERIFY] Creating Sandbox Environment...")

	sandbox, err := fs.NewSandbox(p.ProjectRoot)
	if err != nil {
		return nil, fmt.Errorf("failed to init sandbox: %w", err)
	}
	defer sandbox.Cleanup()

	// 1. Apply Patch
	fmt.Println("🧪 [VERIFY] Applying Patch to Sandbox...")
	if err := sandbox.ApplyPatch([]byte(patchDiff)); err != nil {
		return &VerifyResult{
			Success: false,
			Stage:   "git apply",
			Logs:    err.Error(),
		}, nil
	}

	// 2. Run Checks
	// We define the standard pipeline: Vet -> Test -> Build
	checks := []struct {
		Name string
		Args []string
	}{
		{"go vet", []string{"vet", "./..."}},
		{"go test", []string{"test", "./..."}},
		{"go build", []string{"build", "./..."}},
	}

	for _, check := range checks {
		fmt.Printf("🧪 [VERIFY] Running '%s'...\n", check.Name)
		out, err := sandbox.RunGoCommand(ctx, check.Args...)
		if err != nil {
			// Verification Failed
			return &VerifyResult{
				Success: false,
				Stage:   check.Name,
				Logs:    fmt.Sprintf("Error: %v\nOutput:\n%s", err, out),
			}, nil
		}
	}

	return &VerifyResult{Success: true}, nil
}

// ApplyToReal effectively "merges" the verified patch to the real codebase.
func (p *Pipeline) ApplyToReal(patchPath string) error {
	cmd := exec.Command("git", "apply", patchPath)
	cmd.Dir = p.ProjectRoot
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("apply failed: %s", string(out))
	}
	return nil
}
```

---

## File: app/container.go
```go
package app

import (
	"path/filepath"
	"time"

	"github.com/david22573/codepicker/adapters/agent"
	"github.com/david22573/codepicker/adapters/context"
	"github.com/david22573/codepicker/adapters/policy"
	"github.com/david22573/codepicker/adapters/tools"
	"github.com/david22573/codepicker/adapters/verifier"
	contextDomain "github.com/david22573/codepicker/domain/context"
	"github.com/david22573/codepicker/infra/fs"
	"github.com/david22573/codepicker/infra/git"
	"github.com/david22573/codepicker/infra/llm"
	"github.com/david22573/codepicker/infra/logging"
	"github.com/david22573/codepicker/infra/shell"
	"github.com/david22573/codepicker/infra/storage"
	"go.uber.org/zap"
)

type Container struct {
	Planner          *agent.Planner
	PlanExecutor     *agent.PlanExecutor
	Auditor          *agent.Auditor
	Explainer        *agent.Explainer
	TwoPassEngine    *agent.TwoPassEngine
	Verifier         *verifier.Pipeline
	Git              *git.Client
	ContextBuilder   *context.SliceBasedBuilder
	WorkspaceManager *fs.WorkspaceManager
	Repository       *storage.SQLiteRepository // Changed to pointer to avoid lock copy
	SliceStore       contextDomain.SliceStore
	Logger           *logging.Logger
}

func NewContainer(apiKey, projectRoot, llmModel string, isDryRun, isCI bool) (*Container, error) {
	// 1. Initialize Logger
	logEnv := "development"
	if isCI {
		logEnv = "production"
	}

	logger, err := logging.NewLogger(logEnv)
	if err != nil {
		return nil, err
	}

	// Fixed: Use zap fields instead of map
	logger.Info("Initializing CodePicker Container",
		zap.String("mode", logEnv),
		zap.String("root", projectRoot))

	hiddenDir := filepath.Join(projectRoot, ".codepicker")
	dbPath := filepath.Join(hiddenDir, "state.db")

	repo, err := storage.NewSQLiteRepository(dbPath)
	if err != nil {
		// Fixed: Use zap fields
		logger.Error("Database initialization failed", zap.Error(err))
		return nil, err
	}

	workspaceMgr := fs.NewWorkspaceManager(projectRoot)
	_ = git.NewClient(projectRoot)

	selectedModel := "moonshotai/kimi-k2.5"
	if llmModel != "" {
		selectedModel = llmModel
	}
	llmClient := llm.NewOpenRouterAdapter(apiKey, selectedModel)

	shadowMgr := fs.NewShadowManager(projectRoot)
	shellExec := shell.NewExecutor(30*time.Second, 5000)
	allTools := tools.DefaultSet(shadowMgr, shellExec, projectRoot)

	policyPath := filepath.Join(projectRoot, "policy.json")
	policyConfig, _ := policy.LoadPolicy(policyPath)
	guardRail := policy.NewEnforcer(*policyConfig, isDryRun)

	ctxBuilder := context.NewSliceBasedBuilder(repo, 16000)

	// Updated to pass logger
	worker := agent.NewReActAgent(llmClient, allTools, guardRail, repo, logger)
	planner := agent.NewPlanner(llmClient, repo)
	executor := agent.NewPlanExecutor(worker, repo)

	return &Container{
		Planner:          planner,
		PlanExecutor:     executor,
		ContextBuilder:   ctxBuilder,
		WorkspaceManager: workspaceMgr,
		Repository:       repo, // Pass pointer, do not dereference
		SliceStore:       repo,
		Logger:           logger,
	}, nil
}

// Close ensures all resources (DB, Logger) are flushed and closed properly
func (c *Container) Close() {
	if c.Logger != nil {
		c.Logger.Info("Shutting down container...")
		c.Logger.Sync()
	}
	if c.Repository != nil {
		c.Repository.Close()
	}
}
```

---

## File: cmd/agent.go
```go
package cmd

import (
	"context"
	"fmt"
	"os"

	"github.com/david22573/codepicker/app"
	"github.com/spf13/cobra"
)

var (
	agentModel  string
	executionID string
)

var agentCmd = &cobra.Command{
	Use:   "agent",
	Short: "Interact with specific agent personas and history",
}

// --- Audit Command ---
var auditCmd = &cobra.Command{
	Use:   "audit [query]",
	Short: "Perform a semantic read-only audit of the codebase",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runAgentTask(func(c *app.Container) error {
			query := args[0]
			fmt.Printf("🛡️  Starting Audit (Model: %s)\n", getModelDisplay())

			fmt.Println("🧠 Gathering relevant code slices for audit...")

			// FIX: Use BuildForTask(query) instead of Build()
			// This returns a markdown string we can prepend to the prompt
			contextStr, err := c.ContextBuilder.BuildForTask(query)
			if err != nil {
				fmt.Printf("⚠️  Warning: Context building failed: %v\n", err)
			}

			// Combine context and query for the Auditor
			fullInput := fmt.Sprintf("CONTEXT:\n%s\n\nTASK: %s", contextStr, query)

			report, err := c.Auditor.RunAudit(context.Background(), fullInput)
			if err != nil {
				return err
			}

			fmt.Println("\n================ REPORT ================")
			fmt.Println(report.Content)
			fmt.Println("========================================")
			fmt.Printf("📄 Saved to: %s\n", report.Artifact)
			return nil
		}, dryRunFlag)
	},
}

// --- Plan Command ---
var planCmd = &cobra.Command{
	Use:   "plan [task]",
	Short: "Generate a plan without executing it",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runAgentTask(func(c *app.Container) error {
			taskInput := args[0]
			fmt.Println("🧠 Generating Plan...")

			// FIX: Use BuildForTask here too
			contextStr, err := c.ContextBuilder.BuildForTask(taskInput)
			if err != nil {
				fmt.Printf("⚠️  Warning: Context building failed: %v\n", err)
			}

			plan, err := c.Planner.CreatePlan(context.Background(), taskInput, contextStr)
			if err != nil {
				return err
			}

			fmt.Printf("✅ Plan Created (ID: %s)\n", plan.ID)
			fmt.Printf("   Reasoning: %s\n", plan.Reasoning)
			for _, step := range plan.Steps {
				fmt.Printf("   - [ ] %s\n", step.Description)
			}
			return nil
		}, true) // Plan generation is always "dry run" safe
	},
}

// --- Replay Command ---
var replayCmd = &cobra.Command{
	Use:   "replay",
	Short: "Replay a past execution from the database",
	RunE: func(cmd *cobra.Command, args []string) error {
		return runAgentTask(func(c *app.Container) error {
			if executionID == "" {
				return fmt.Errorf("must specify --execution ID")
			}
			exec, err := c.Repository.GetExecution(context.Background(), executionID)
			if err != nil {
				return err
			}

			fmt.Printf("Replaying Execution: %s\n", exec.ID)
			for _, turn := range exec.History {
				fmt.Printf("\n--- Turn %d ---\n", turn.TurnID)
				fmt.Printf("Thought: %s\n", turn.Thought)
				fmt.Printf("Action: %s (%s)\n", turn.ToolName, turn.ToolArgs)
				fmt.Printf("Result: %s\n", turn.ToolOut)
			}
			return nil
		}, true)
	},
}

// --- Explain Command ---
var explainCmd = &cobra.Command{
	Use:   "explain",
	Short: "Use AI to explain a past execution",
	RunE: func(cmd *cobra.Command, args []string) error {
		return runAgentTask(func(c *app.Container) error {
			if executionID == "" {
				return fmt.Errorf("must specify --execution ID")
			}

			// FIX: c.Explainer is now valid
			summary, err := c.Explainer.Explain(context.Background(), executionID)
			if err != nil {
				return err
			}

			fmt.Println("\n🤖 EXECUTION EXPLANATION:")
			fmt.Println(summary)
			return nil
		}, true)
	},
}

// --- Helpers ---

func runAgentTask(action func(*app.Container) error, dryRun bool) error {
	apiKey := os.Getenv("OPENROUTER_API_KEY")
	if apiKey == "" {
		return fmt.Errorf("OPENROUTER_API_KEY required")
	}
	cwd, _ := os.Getwd()
	// Update NewContainer to match signature (empty model string uses default)
	container, err := app.NewContainer(apiKey, cwd, agentModel, dryRun, false)
	if err != nil {
		return err
	}
	return action(container)
}

func getModelDisplay() string {
	if agentModel == "" {
		return "Default"
	}
	return agentModel
}

func init() {
	agentCmd.PersistentFlags().StringVar(&agentModel, "model", "", "Override LLM model")
	replayCmd.Flags().StringVar(&executionID, "execution", "", "Execution ID to replay")
	explainCmd.Flags().StringVar(&executionID, "execution", "", "Execution ID to explain")

	agentCmd.AddCommand(auditCmd)
	agentCmd.AddCommand(planCmd)
	agentCmd.AddCommand(replayCmd)
	agentCmd.AddCommand(explainCmd)
	rootCmd.AddCommand(agentCmd)
}
```

---

## File: cmd/apply.go
```go
package cmd

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/david22573/codepicker/infra/fs"
	"github.com/david22573/codepicker/infra/hooks"
	"github.com/spf13/cobra"
)

var (
	formatFlag bool
)

var applyCmd = &cobra.Command{
	Use:   "apply [file_path]",
	Short: "Apply a file from shadow storage to the real filesystem",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		relPath := args[0]
		cwd, _ := os.Getwd()

		shadowMgr := fs.NewShadowManager(cwd)

		// 1. Validation & Summary (Existing Logic)
		shadowPath := filepath.Join(cwd, ".codepicker/shadow", relPath)
		if _, err := os.Stat(shadowPath); os.IsNotExist(err) {
			return fmt.Errorf("no shadow file found for %s", relPath)
		}

		summary, err := shadowMgr.Diff(relPath)
		if err != nil {
			return fmt.Errorf("failed to calculate diff: %w", err)
		}

		fmt.Println("---------------------------------------------------")
		fmt.Println("📝 PRE-APPLY CHANGE SUMMARY")
		fmt.Println(summary.String())
		fmt.Println("---------------------------------------------------")

		if summary.Type == fs.ChangeNoOp {
			fmt.Println("⚠️  No changes detected.")
			return nil
		}

		// 2. User Confirmation
		fmt.Print("Are you sure you want to apply these changes? [y/N]: ")
		reader := bufio.NewReader(os.Stdin)
		input, _ := reader.ReadString('\n')
		input = strings.TrimSpace(strings.ToLower(input))

		if input != "y" && input != "yes" {
			fmt.Println("❌ Apply aborted.")
			return nil
		}

		// 3. Apply Changes
		fmt.Printf("Applying changes to %s...\n", relPath)
		if err := shadowMgr.Apply(relPath); err != nil {
			return fmt.Errorf("failed to apply: %w", err)
		}

		// 4. Post-Apply Hook: Formatting
		if formatFlag {
			realPath := filepath.Join(cwd, relPath)
			// Run formatting in background context
			if err := hooks.RunFormatter(context.Background(), realPath); err != nil {
				// Warn but do not fail the command, as the file is already applied
				fmt.Printf("⚠️  %v\n", err)
			}
		}

		fmt.Println("✅ Changes applied successfully.")
		return nil
	},
}

func init() {
	// Add opt-out flag for formatting
	applyCmd.Flags().BoolVar(&formatFlag, "fmt", true, "Run code formatter after applying changes")
	rootCmd.AddCommand(applyCmd)
}
```

---

## File: cmd/context.go
```go
package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/david22573/codepicker/app"
	ctxDomain "github.com/david22573/codepicker/domain/context"
	"github.com/david22573/codepicker/infra/indexer"
	"github.com/spf13/cobra"
)

var (
	ctxInclude []string
	ctxExclude []string
)

// contextCmd represents the base command for context management
var contextCmd = &cobra.Command{
	Use:   "context",
	Short: "Manage the semantic code index and execution context",
}

// contextIndexCmd triggers the semantic indexing with your ignore logic
var contextIndexCmd = &cobra.Command{
	Use:   "index [directory]",
	Short: "Scan and index the codebase using ignore patterns",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		targetDir := "."
		if len(args) > 0 {
			targetDir = args[0]
		}

		apiKey := os.Getenv("OPENROUTER_API_KEY")
		cwd, _ := os.Getwd()

		container, err := app.NewContainer(apiKey, cwd, "", false, false)
		if err != nil {
			return fmt.Errorf("failed to initialize container: %w", err)
		}

		// --- Your Original Ignore Logic ---
		finalExcludes := []string{".git", ".git/*", ".codepicker/*"}

		if gitIgnore, err := readIgnoreFile(".gitignore"); err == nil {
			finalExcludes = append(finalExcludes, gitIgnore...)
			fmt.Printf("🛡️  Loaded %d patterns from .gitignore\n", len(gitIgnore))
		}

		if cpIgnore, err := readIgnoreFile(".codepickerignore"); err == nil {
			finalExcludes = append(finalExcludes, cpIgnore...)
			fmt.Printf("👁️  Loaded %d patterns from .codepickerignore\n", len(cpIgnore))
		}
		finalExcludes = append(finalExcludes, ctxExclude...)

		fmt.Printf("🔍 Indexing codebase at: %s (respecting ignore patterns)\n", targetDir)

		// --- Semantic Indexing (Phase 2 & 5) ---
		slicer := indexer.NewCodeSlicer()
		manager := indexer.NewIndexManager(slicer, container.SliceStore)

		if err := manager.IndexDirectory(targetDir); err != nil {
			return fmt.Errorf("indexing failed: %w", err)
		}

		stats, _ := container.SliceStore.GetStats()
		fmt.Printf("\n✅ Indexing complete! Total Slices: %d across %d files.\n",
			stats.TotalSlices, stats.TotalFiles)

		return nil
	},
}

// contextExportCmd generates a full project markdown like you wanted
var contextExportCmd = &cobra.Command{
	Use:   "export [output_file]",
	Short: "Export the entire semantic index as a single Markdown file",
	RunE: func(cmd *cobra.Command, args []string) error {
		outFile := "codepicker_context.md"
		if len(args) > 0 {
			outFile = args[0]
		}

		cwd, _ := os.Getwd()
		apiKey := os.Getenv("OPENROUTER_API_KEY")
		container, err := app.NewContainer(apiKey, cwd, "", false, false)
		if err != nil {
			return err
		}

		fmt.Printf("📄 Exporting full index to %s...\n", outFile)

		// Fixed: Pulling everything without FTS5 MATCH syntax
		allSlices, err := container.Repository.GetAllSlices()
		if err != nil {
			return err
		}

		var sb strings.Builder
		sb.WriteString("# CODEPICKER FULL PROJECT CONTEXT\n\n")

		byFile := make(map[string][]ctxDomain.CodeSlice)
		for _, s := range allSlices {
			byFile[s.FilePath] = append(byFile[s.FilePath], s)
		}

		for path, slices := range byFile {
			sb.WriteString(fmt.Sprintf("## File: %s\n", path))
			for _, s := range slices {
				sb.WriteString(fmt.Sprintf("### %s (Lines %d-%d)\n```go\n%s\n```\n\n",
					s.SliceType, s.StartLine, s.EndLine, s.Content))
			}
			sb.WriteString("---\n")
		}

		return os.WriteFile(outFile, []byte(sb.String()), 0644)
	},
}

// contextBuildCmd previews semantic slices for a specific task
var contextBuildCmd = &cobra.Command{
	Use:   "build [task]",
	Short: "Preview semantic slices that would be sent for a task",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		taskInput := args[0]
		apiKey := os.Getenv("OPENROUTER_API_KEY")
		cwd, _ := os.Getwd()

		container, err := app.NewContainer(apiKey, cwd, "", false, false)
		if err != nil {
			return err
		}

		// Uses Phase 3 ranking logic
		output, err := container.ContextBuilder.BuildForTask(taskInput)
		if err != nil {
			return err
		}

		fmt.Println("\n--- SEMANTIC CONTEXT PREVIEW ---")
		fmt.Println(output)
		return nil
	},
}

// readIgnoreFile is your original helper to parse ignore-style files
func readIgnoreFile(filename string) ([]string, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var patterns []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		patterns = append(patterns, line)
	}
	return patterns, scanner.Err()
}

func init() {
	contextIndexCmd.Flags().StringSliceVar(&ctxInclude, "include", []string{}, "Include patterns")
	contextIndexCmd.Flags().StringSliceVar(&ctxExclude, "exclude", []string{}, "Exclude patterns")

	contextCmd.AddCommand(contextIndexCmd)
	contextCmd.AddCommand(contextBuildCmd)
	contextCmd.AddCommand(contextExportCmd)
	rootCmd.AddCommand(contextCmd)
}
```

---

## File: cmd/fix.go
```go
package cmd

import (
	"context"
	"fmt"
	"os"

	"github.com/david22573/codepicker/app"
	"github.com/spf13/cobra"
)

var fixTargetFile string

var fixCmd = &cobra.Command{
	Use:   "fix [task]",
	Short: "Auto-fix a bug using the Two-Pass (Analyst/Engineer) workflow",
	Long: `The fix command uses a specialized two-stage process:
1. Analyst: Reads the code and diagnoses the root cause.
2. Engineer: Writes a patch to fix the issue.

Requires a task description and a target file.`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		taskInput := args[0]
		apiKey := os.Getenv("OPENROUTER_API_KEY")
		if apiKey == "" {
			fmt.Println("❌ Error: OPENROUTER_API_KEY environment variable is required.")
			os.Exit(1)
		}

		cwd, _ := os.Getwd()

		// FIX: Update NewContainer call signature
		container, err := app.NewContainer(apiKey, cwd, "", dryRunFlag, ciFlag)
		if err != nil {
			fmt.Printf("❌ Container Init Failed: %v\n", err)
			os.Exit(1)
		}

		ctx := context.Background()

		if fixTargetFile == "" {
			fmt.Println("❌ Error: You must specify a target file with --file or -f")
			return
		}

		// --- Phase 1: The Analyst ---
		fmt.Printf("🧐 [ANALYST] Diagnosing issue in %s...\n", fixTargetFile)

		// The Analyst reads the specific file to understand the context
		analysis, err := container.TwoPassEngine.RunAnalysis(ctx, taskInput, fixTargetFile)
		if err != nil {
			fmt.Printf("❌ Analysis Failed: %v\n", err)
			os.Exit(1)
		}

		fmt.Println("\n📄 [REPORT] Findings:")
		fmt.Println(analysis.Markdown)

		// --- Phase 2: The Engineer ---
		fmt.Println("\n👷 [ENGINEER] Generating patch...")

		// The Engineer generates a Git patch based on the Analyst's report
		patch, err := container.TwoPassEngine.GeneratePatch(ctx, taskInput, analysis)
		if err != nil {
			fmt.Printf("❌ Patch Generation Failed: %v\n", err)
			os.Exit(1)
		}

		fmt.Println("\n📝 [PATCH] Generated Diff:")
		fmt.Println(patch.Diff)

		// --- Phase 3: Verification (Optional) ---
		if !dryRunFlag {
			fmt.Println("\n🧪 [VERIFIER] Verifying patch in sandbox...")
			result, err := container.Verifier.Verify(ctx, patch.Diff)
			if err != nil {
				fmt.Printf("⚠️  Verification Error: %v\n", err)
			} else if !result.Success {
				fmt.Printf("❌ Verification Failed during %s:\n%s\n", result.Stage, result.Logs)

				// Optional: Trigger Self-Correction (RefinePatch) here if desired
				// patch, err = container.TwoPassEngine.RefinePatch(ctx, taskInput, analysis, patch.Diff, result.Logs)
			} else {
				fmt.Println("✅ Verification Passed! Applying to real codebase...")
				if err := container.Verifier.ApplyToReal("patch.diff"); err != nil {
					// Note: You might need to save patch.diff to disk first depending on implementation
					fmt.Printf("⚠️  Could not auto-apply. Save the diff manually.\n")
				}
			}
		}
	},
}

func init() {
	fixCmd.Flags().StringVarP(&fixTargetFile, "file", "f", "", "Target file to analyze and fix")
	rootCmd.AddCommand(fixCmd)
}
```

---

## File: cmd/fmt.go
```go
package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/david22573/codepicker/infra/hooks"
	"github.com/spf13/cobra"
)

var fmtCmd = &cobra.Command{
	Use:   "fmt [file]",
	Short: "Run the configured formatter on a file (manual trigger)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		path := args[0]
		absPath, _ := filepath.Abs(path)

		// Verify file exists
		if _, err := os.Stat(absPath); os.IsNotExist(err) {
			return fmt.Errorf("file not found: %s", path)
		}

		fmt.Printf("Running formatter on %s...\n", path)
		return hooks.RunFormatter(context.Background(), absPath)
	},
}

func init() {
	rootCmd.AddCommand(fmtCmd)
}
```

---

## File: cmd/history.go
```go
package cmd

import (
	"context"
	"fmt"
	"os"
	"text/tabwriter"
	"time"

	"github.com/david22573/codepicker/infra/storage"
	"github.com/spf13/cobra"
)

var historyCmd = &cobra.Command{
	Use:   "history",
	Short: "List past agent execution sessions",
	RunE: func(cmd *cobra.Command, args []string) error {
		repo, err := getRepo()
		if err != nil {
			return err
		}

		list, err := repo.ListExecutions(context.Background(), 20)
		if err != nil {
			return fmt.Errorf("failed to fetch history: %w", err)
		}

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
		fmt.Fprintln(w, "EXEC ID\tSTATUS\tTIME\tPLAN ID")
		fmt.Fprintln(w, "-------\t------\t----\t-------")

		for _, item := range list {
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\n",
				item.ID,
				item.Status,
				item.StartTime.Format(time.RFC3339),
				item.PlanID,
			)
		}
		w.Flush()
		return nil
	},
}

var inspectCmd = &cobra.Command{
	Use:   "inspect [exec_id]",
	Short: "Replay the timeline of a specific execution",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		id := args[0]
		repo, err := getRepo()
		if err != nil {
			return err
		}

		exec, err := repo.GetExecution(context.Background(), id)
		if err != nil {
			return fmt.Errorf("failed to load execution: %w", err)
		}

		fmt.Printf("🔍 INSPECTING SESSION: %s\n", exec.ID)
		fmt.Printf("📅 Date: %s\n", exec.StartTime.Format(time.RFC822))
		fmt.Printf("🚦 Final Status: %s\n", exec.Status)
		fmt.Println("===================================================")

		for _, turn := range exec.History {
			fmt.Printf("\n[Turn %d]\n", turn.TurnID)
			fmt.Printf("🧠 Thought: %s\n", turn.Thought)
			if turn.ToolName != "" {
				fmt.Printf("🛠️  Tool: %s\n", turn.ToolName)
				fmt.Printf("📥 Input: %s\n", turn.ToolArgs)
				// Truncate long outputs for readability
				out := turn.ToolOut
				if len(out) > 300 {
					out = out[:300] + "... (truncated)"
				}
				fmt.Printf("📤 Output: %s\n", out)
			} else {
				fmt.Println("🛑 Action: (None/Final Answer)")
			}
			fmt.Println("---")
		}

		return nil
	},
}

// Helper to quickly grab the repo without the full container overhead
func getRepo() (*storage.SQLiteRepository, error) {
	cwd, _ := os.Getwd()
	dbPath := fmt.Sprintf("%s/.codepicker/state.db", cwd)
	return storage.NewSQLiteRepository(dbPath)
}

func init() {
	rootCmd.AddCommand(historyCmd)
	rootCmd.AddCommand(inspectCmd)
}
```

---

## File: cmd/improve.go
```go
package cmd

import (
	"context"
	"fmt"
	"os"

	"github.com/david22573/codepicker/app"
	"github.com/spf13/cobra"
)

var improveCmd = &cobra.Command{
	Use:   "improve",
	Short: "Automatically suggest and apply codebase improvements",
	Run: func(cmd *cobra.Command, args []string) {
		apiKey := os.Getenv("OPENROUTER_API_KEY")
		if apiKey == "" {
			fmt.Println("❌ Error: OPENROUTER_API_KEY is not set.")
			os.Exit(1)
		}

		cwd, _ := os.Getwd()

		// Initialize the container with current configuration
		container, err := app.NewContainer(apiKey, cwd, "", dryRunFlag, ciFlag)
		if err != nil {
			fmt.Printf("❌ Container Init Failed: %v\n", err)
			os.Exit(1)
		}

		ctx := context.Background()

		fmt.Println("📡 [SCOUT] Searching for potential improvements...")

		// The Auditor uses the LLM to scan for safe refactors
		tasks, err := container.Auditor.SuggestImprovements(ctx)
		if err != nil {
			fmt.Printf("❌ Audit Failed: %v\n", err)
			os.Exit(1)
		}

		if len(tasks) == 0 {
			fmt.Println("✅ No immediate improvements suggested. Your code is looking sharp!")
			return
		}

		fmt.Printf("\n✨ Found %d suggested improvements:\n", len(tasks))
		for i, t := range tasks {
			fmt.Printf("%d. %s\n", i+1, t)
		}

		fmt.Println("\n💡 To apply one, run: codepicker run \"<task_description>\"")
	},
}

func init() {
	rootCmd.AddCommand(improveCmd)
}
```

---

## File: cmd/plans.go
```go
package cmd

import (
	"context"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/david22573/codepicker/app"
	"github.com/david22573/codepicker/domain/task"
	"github.com/spf13/cobra"
)

var plansCmd = &cobra.Command{
	Use:   "plans",
	Short: "List and manage coding plans",
	Run: func(cmd *cobra.Command, args []string) {
		apiKey := os.Getenv("OPENROUTER_API_KEY")
		cwd, _ := os.Getwd()

		// Initialize container to get access to the repository
		container, err := app.NewContainer(apiKey, cwd, "", false, false)
		if err != nil {
			fmt.Printf("❌ Failed to initialize: %v\n", err)
			return
		}

		printDashboard(container)
	},
}

func printDashboard(c *app.Container) {
	// Clear screen for a clean dashboard view
	fmt.Print("\033[H\033[2J")

	// Repository now returns agent.PlanSummary which includes StepCount
	summaries, err := c.Repository.ListPlans(context.Background(), 10)
	if err != nil {
		fmt.Printf("Error loading plans: %v\n", err)
		return
	}

	fmt.Println("===============================================================================")
	fmt.Println("                              CODEPICKER PLANS                                 ")
	fmt.Println("===============================================================================")

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "ID\tSTATUS\tSTEPS\tCREATED\tTASK")
	fmt.Fprintln(w, "--\t------\t-----\t-------\t----")

	for _, s := range summaries {
		statusIcon := "⚪"
		switch s.Status {
		case task.StatusCompleted:
			statusIcon = "🟢"
		case task.StatusRunning:
			statusIcon = "🟠"
		case task.StatusFailed:
			statusIcon = "🔴"
		}

		taskDisplay := s.OriginalTask
		if len(taskDisplay) > 50 {
			taskDisplay = taskDisplay[:47] + "..."
		}

		// Using the fields defined in domain/agent/agent.go: PlanSummary struct
		fmt.Fprintf(w, "%s\t%s %s\t%d\t%s\t%s\n",
			s.ID,
			statusIcon,
			s.Status,
			s.StepCount,
			s.CreatedAt.Format("01-02 15:04"),
			taskDisplay,
		)
	}
	w.Flush()
	fmt.Println("===============================================================================")
	fmt.Println("Use 'codepicker run --plan <ID>' to execute or resume a plan.")
}

func init() {
	rootCmd.AddCommand(plansCmd)
}
```

---

## File: cmd/root.go
```go
package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "codepicker",
	Short: "CodePicker: The Autonomous Coding Agent",
	Long:  `CodePicker is a ReAct-based agent that safely refactors code using a shadow filesystem.`,
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
```

---

## File: cmd/run.go
```go
package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/david22573/codepicker/app"
	"github.com/david22573/codepicker/domain/task"
	"github.com/spf13/cobra"
)

var (
	dryRunFlag bool
	ciFlag     bool
	planIDFlag string
)

var runCmd = &cobra.Command{
	Use:   "run [task]",
	Short: "Execute a coding task (via plan)",
	Run: func(cmd *cobra.Command, args []string) {
		if planIDFlag == "" && len(args) < 1 {
			fmt.Println("Error: You must provide a task string OR a --plan <id>")
			_ = cmd.Usage()
			os.Exit(1)
		}

		taskInput := ""
		if len(args) > 0 {
			taskInput = args[0]
		}

		if err := executeRun(taskInput, planIDFlag); err != nil {
			if ciFlag {
				res := task.CIResult{
					Status: "failure",
					Task:   taskInput,
					Error:  err.Error(),
				}
				json.NewEncoder(os.Stdout).Encode(res)
				os.Exit(1)
			}
			fmt.Printf("\n❌ EXECUTION FAILED: %v\n", err)
			os.Exit(1)
		}
	},
}

func executeRun(taskInput, planID string) error {
	apiKey := os.Getenv("OPENROUTER_API_KEY")
	if apiKey == "" {
		return fmt.Errorf("OPENROUTER_API_KEY required")
	}

	cwd, _ := os.Getwd()

	container, err := app.NewContainer(apiKey, cwd, "", dryRunFlag, ciFlag)
	if err != nil {
		return err
	}

	if !ciFlag {
		printSafetyBanner(dryRunFlag)
	}

	ctx := context.Background()
	var plan *task.Plan

	if planID != "" {
		if !ciFlag {
			fmt.Printf("📂 [SYSTEM] Loading Plan %s...\n", planID)
		}
		p, err := container.Repository.GetPlan(ctx, planID)
		if err != nil {
			return fmt.Errorf("failed to load plan: %w", err)
		}
		plan = p
		if plan.Status == task.StatusCompleted && !ciFlag {
			fmt.Println("⚠️  [SYSTEM] Warning: This plan is already marked as completed.")
		}
	} else {
		if !ciFlag {
			fmt.Printf("🚀 [SYSTEM] Initializing task: %s\n", taskInput)
			fmt.Println("🧠 [AGENT] Generating plan...")
		}

		// FIX: Use BuildForTask(taskInput) instead of Build()
		// The new builder returns a string (markdown context), not an object.
		fileContext, err := container.ContextBuilder.BuildForTask(taskInput)
		if err != nil {
			fmt.Printf("⚠️  Warning: Context generation incomplete: %v\n", err)
		}

		// Pass the generated string context to the planner
		p, err := container.Planner.CreatePlan(ctx, taskInput, fileContext)
		if err != nil {
			return err
		}
		plan = p
		if !ciFlag {
			fmt.Printf("✅ [SYSTEM] Plan Generated (ID: %s)\n", plan.ID)
		}
	}

	if !ciFlag {
		fmt.Println("▶️  [SYSTEM] Starting Execution Phase...")
	}

	execErr := container.PlanExecutor.Execute(ctx, plan)

	if ciFlag {
		return handleCIOutput(plan, execErr)
	}

	if execErr != nil {
		return execErr
	}
	fmt.Println("\n✅ [SYSTEM] Task Completed Successfully.")
	return nil
}
func printSafetyBanner(isDryRun bool) {
	fmt.Println("\n===================================================")
	fmt.Println("🛡️  CodePicker Safety Guardrails Active")
	fmt.Println("===================================================")

	if isDryRun {
		fmt.Println("🔒 MODE: DRY-RUN (Read-Only)")
		fmt.Println("   • File system writes are DISABLED")
		fmt.Println("   • Shell commands are DISABLED")
	} else {
		fmt.Println("⚡ MODE: LIVE EXECUTION (Write-Enabled)")
		fmt.Println("   ⚠️  The agent has permission to modify files.")
		fmt.Println("   ⚠️  Shadow filesystem is ACTIVE for safety rollback.")
		fmt.Println("   • Monitor the '🤖 [AGENT]' logs below closely.")
	}
	fmt.Println("===================================================\n")
}

func handleCIOutput(plan *task.Plan, execErr error) error {
	failedCount := 0
	for _, s := range plan.Steps {
		if s.Status == task.StatusFailed {
			failedCount++
		}
	}

	status := "success"
	errMsg := ""
	if execErr != nil || failedCount > 0 {
		status = "failure"
		if execErr != nil {
			errMsg = execErr.Error()
		} else {
			errMsg = "One or more steps failed"
		}
	}

	result := task.CIResult{
		Status:      status,
		Task:        plan.OriginalTask,
		PlanSummary: plan.Reasoning,
		StepsTotal:  len(plan.Steps),
		StepsFailed: failedCount,
		Error:       errMsg,
	}

	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(result)
}

func init() {
	runCmd.Flags().BoolVar(&dryRunFlag, "dry-run", false, "Enable read-only mode")
	runCmd.Flags().BoolVar(&ciFlag, "ci", false, "Enable CI mode (JSON output, no prompts, strict safety)")
	runCmd.Flags().StringVar(&planIDFlag, "plan", "", "Execute a specific pre-generated plan ID")
	rootCmd.AddCommand(runCmd)
}
```

---

## File: domain/agent/agent.go
```go
package agent

import (
	"context"
	"time"

	"github.com/david22573/codepicker/domain/task"
)

// Agent represents the high-level autonomous entity
type Agent interface {
	Name() string
	Run(ctx context.Context, input string) (string, error)
}

// Tool represents a capability the agent can use
type Tool interface {
	Name() string
	Description() string
	Execute(ctx context.Context, args string) (string, error)
}

// Policy defines the security rules
type Policy interface {
	CanExecute(toolName string, args string) (bool, string)
	Mode() string
}

// LLMClient abstracts the AI provider
type LLMClient interface {
	Chat(ctx context.Context, systemPrompt string, userMessage string) (string, error)
}

// ExecutionSummary is a lightweight view for listing executions
type ExecutionSummary struct {
	ID        string
	PlanID    string
	Status    task.Status
	StartTime time.Time
}

// PlanSummary is a lightweight view for listing plans (New for UX)
type PlanSummary struct {
	ID           string
	OriginalTask string
	Status       task.Status
	StepCount    int
	CreatedAt    time.Time
}

// Repository defines how we save/load executions and plans
type Repository interface {
	SaveExecution(ctx context.Context, exec *Execution) error
	GetExecution(ctx context.Context, id string) (*Execution, error)
	ListExecutions(ctx context.Context, limit int) ([]ExecutionSummary, error)

	// Phase 2: Plan Persistence
	SavePlan(ctx context.Context, plan *task.Plan) error
	GetPlan(ctx context.Context, id string) (*task.Plan, error)

	// Phase 5: Plan Management (Dashboard)
	ListPlans(ctx context.Context, limit int) ([]PlanSummary, error)
	DeletePlan(ctx context.Context, id string) error
}
```

---

## File: domain/agent/execution.go
```go
package agent

import (
	"time"

	"github.com/david22573/codepicker/domain/task"
)

// Execution represents a running session of an agent
type Execution struct {
	ID        string
	PlanID    string
	Status    task.Status
	History   []Interaction
	StartTime time.Time
	EndTime   time.Time
}

// Interaction records a single "turn" in the agent loop
type Interaction struct {
	TurnID    int
	Thought   string // The reasoning provided by the LLM
	ToolName  string // The tool the LLM chose
	ToolArgs  string // The arguments passed
	ToolOut   string // The result from the tool execution
	Timestamp time.Time
}

// NewExecution creates a new execution context
func NewExecution(id, planID string) *Execution {
	return &Execution{
		ID:        id,
		PlanID:    planID,
		Status:    task.StatusRunning,
		StartTime: time.Now(),
		History:   make([]Interaction, 0),
	}
}

// RecordTurn adds an interaction to the history
func (e *Execution) RecordTurn(thought, tool, args, output string) {
	e.History = append(e.History, Interaction{
		TurnID:    len(e.History) + 1,
		Thought:   thought,
		ToolName:  tool,
		ToolArgs:  args,
		ToolOut:   output,
		Timestamp: time.Now(),
	})
}

func (e *Execution) Finish() {
	e.Status = task.StatusCompleted
	e.EndTime = time.Now()
}

func (e *Execution) Fail() {
	e.Status = task.StatusFailed
	e.EndTime = time.Now()
}
```

---

## File: domain/agent/planner.go
```go
package agent

import (
	"context"

	contextDomain "github.com/david22573/codepicker/domain/context"
	"github.com/david22573/codepicker/domain/task"
)

// Planner defines the interface for generating and optimizing execution plans.
type Planner interface {
	// CreatePlan generates a new plan based on the structured LLM context.
	CreatePlan(ctx context.Context, taskInput string, llmCtx *contextDomain.LLMContext) (*task.Plan, error)

	// OptimizePlan uses AI to refine an existing plan based on feedback.
	OptimizePlan(ctx context.Context, plan *task.Plan, feedback string) (*task.Plan, error)
}
```

---

## File: domain/audit/provenance.go
```go
package audit

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
)

// Provenance holds the forensic data about an automated change.
type Provenance struct {
	Model        string
	Task         string
	ContextID    string
	ContextHash  string
	AnalysisHash string
	PolicyHash   string
}

// FormatCommitMessage generates the standardized forensic commit message.
func (p *Provenance) FormatCommitMessage() string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("fix: %s (auto-generated)\n\n", p.Task))

	sb.WriteString("🤖 CodePicker Verification: PASSED\n")
	sb.WriteString("This change was analyzed, patched, and verified automatically.\n\n")

	sb.WriteString(fmt.Sprintf("Model: %s\n", p.Model))
	sb.WriteString(fmt.Sprintf("Context-ID: %s\n", p.ContextID))
	sb.WriteString(fmt.Sprintf("Context-Hash: %s\n", p.ContextHash))
	sb.WriteString(fmt.Sprintf("Analysis-Hash: %s\n", p.AnalysisHash))

	if p.PolicyHash != "" {
		sb.WriteString(fmt.Sprintf("Policy-Hash: %s\n", p.PolicyHash))
	}

	return sb.String()
}

// CalculateHash is a helper to generate SHA256 hashes for strings to ensure integrity.
func CalculateHash(content string) string {
	h := sha256.New()
	h.Write([]byte(content))
	return hex.EncodeToString(h.Sum(nil))
}
```

---

## File: domain/audit/report.go
```go
package audit

import (
	"time"
)

// Report represents the output of an audit session
type Report struct {
	ID        string    `json:"id"`
	Timestamp time.Time `json:"timestamp"`
	Model     string    `json:"model"`
	Content   string    `json:"content"`  // The Markdown analysis
	Artifact  string    `json:"artifact"` // Path to saved file
}
```

---

## File: domain/context/llmcontext.go
```go
package context

import (
	"time"
)

type MessageRole string

const (
	RoleSystem    MessageRole = "system"
	RoleUser      MessageRole = "user"
	RoleAssistant MessageRole = "assistant"
	RoleTool      MessageRole = "tool"
)

type LLMMessage struct {
	Role       MessageRole    `json:"role"`
	Content    string         `json:"content"`
	Name       string         `json:"name,omitempty"`
	ToolCallID string         `json:"tool_call_id,omitempty"`
	Metadata   map[string]any `json:"metadata,omitempty"`
	Timestamp  time.Time      `json:"timestamp"`
}

type TokenUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

type LLMContext struct {
	ID         string         `json:"id"`
	Messages   []LLMMessage   `json:"messages"`
	Usage      TokenUsage     `json:"usage"`
	CreatedAt  time.Time      `json:"created_at"`
	UpdatedAt  time.Time      `json:"updated_at"`
	Properties map[string]any `json:"properties"`
}

func NewLLMContext(id string) *LLMContext {
	now := time.Now()
	return &LLMContext{
		ID:         id,
		Messages:   make([]LLMMessage, 0),
		Properties: make(map[string]any),
		CreatedAt:  now,
		UpdatedAt:  now,
	}
}

func (c *LLMContext) AddMessage(role MessageRole, content string) {
	c.Messages = append(c.Messages, LLMMessage{
		Role:      role,
		Content:   content,
		Timestamp: time.Now(),
	})
	c.UpdatedAt = time.Now()
}

func (c *LLMContext) TotalTokens() int {
	return c.Usage.TotalTokens
}
```

---

## File: domain/context/slice.go
```go
package context

import "time"

// SliceType defines the semantic category of a code chunk
type SliceType string

const (
	SliceTypeFunction  SliceType = "function"
	SliceTypeStruct    SliceType = "struct"
	SliceTypeInterface SliceType = "interface"
	SliceTypeImport    SliceType = "import"
	SliceTypeComment   SliceType = "comment"
	SliceTypeBlock     SliceType = "block"
)

// CodeSlice represents a semantic chunk of code extracted from a file
type CodeSlice struct {
	ID           string            `json:"id"`
	FilePath     string            `json:"file_path"`
	StartLine    int               `json:"start_line"`
	EndLine      int               `json:"end_line"`
	Content      string            `json:"content"`
	Language     string            `json:"language"`
	SliceType    SliceType         `json:"slice_type"`
	Metadata     map[string]string `json:"metadata"`
	Symbols      []string          `json:"symbols"`      // function names, type names, etc.
	Dependencies []string          `json:"dependencies"` // imported packages
	Hash         string            `json:"hash"`         // content hash for cache invalidation
	IndexedAt    time.Time         `json:"indexed_at"`
}

// SliceQuery defines search parameters for retrieving relevant code
type SliceQuery struct {
	Keywords   []string
	FilePath   string
	SliceTypes []SliceType
	Symbols    []string
	MaxResults int
}

// SliceStore defines the interface for persisting and querying code slices
type SliceStore interface {
	// Indexing
	IndexFile(filePath string, slices []CodeSlice) error

	// Querying
	Query(query SliceQuery) ([]CodeSlice, error)
	GetByID(id string) (*CodeSlice, error)
	GetByFile(filePath string) ([]CodeSlice, error)
	GetBySymbol(symbol string) ([]CodeSlice, error)

	// Maintenance
	InvalidateFile(filePath string) error
	GetStats() (*IndexStats, error)
}

// IndexStats provides insights into the health of the code index
type IndexStats struct {
	TotalSlices   int
	TotalFiles    int
	LastIndexedAt time.Time
	CacheHitRate  float64
}
```

---

## File: domain/errors/errors.go
```go
package errors

import "fmt"

// ErrorCode defines stable error codes for the system
type ErrorCode string

const (
	CodeValidation ErrorCode = "VALIDATION" // User input error
	CodeSystem     ErrorCode = "SYSTEM"     // Internal crash/failure
	CodePolicy     ErrorCode = "POLICY"     // Security block
	CodeLLM        ErrorCode = "LLM"        // AI Provider failure
	CodeNotFound   ErrorCode = "NOT_FOUND"  // Resource missing
)

// DomainError is the standard error type for the domain layer
type DomainError struct {
	Op      string    // Operation where error occurred (e.g., "agent.Run")
	Code    ErrorCode // Machine-readable code
	Message string    // Human-readable message
	Err     error     // Underlying error (optional)
}

func (e *DomainError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("[%s] %s: %s (cause: %v)", e.Code, e.Op, e.Message, e.Err)
	}
	return fmt.Sprintf("[%s] %s: %s", e.Code, e.Op, e.Message)
}

func (e *DomainError) Unwrap() error { return e.Err }

// Constructors

func NewValidation(op, msg string) *DomainError {
	return &DomainError{Op: op, Code: CodeValidation, Message: msg}
}

func NewSystem(op, msg string, cause error) *DomainError {
	return &DomainError{Op: op, Code: CodeSystem, Message: msg, Err: cause}
}

func NewPolicy(op, msg string) *DomainError {
	return &DomainError{Op: op, Code: CodePolicy, Message: msg}
}

func NewLLM(op string, cause error) *DomainError {
	return &DomainError{Op: op, Code: CodeLLM, Message: "AI provider failure", Err: cause}
}
```

---

## File: domain/interaction/types.go
```go
package interaction

// Analysis holds the insights gathered during the read-only phase
// of the Two-Pass workflow.
type Analysis struct {
	// Markdown is the natural language report/diagnosis from the agent.
	Markdown string `json:"markdown"`

	// Files is the list of files identified as relevant to the task.
	Files []string `json:"files"`
}

// Patch holds the generated code fix.
type Patch struct {
	// Diff contains the raw Git Unified Diff string.
	Diff string `json:"diff"`
}
```

---

## File: domain/task/result.go
```go
package task

// CIResult defines the schema for machine-readable output
type CIResult struct {
	Status      string   `json:"status"` // success, failure
	Task        string   `json:"task"`
	PlanSummary string   `json:"plan_summary"`
	StepsTotal  int      `json:"steps_total"`
	StepsFailed int      `json:"steps_failed"`
	Artifacts   []string `json:"artifacts,omitempty"` // Generated files (shadow paths)
	Error       string   `json:"error,omitempty"`
}
```

---

## File: domain/task/task.go
```go
package task

import (
	"time"

	"github.com/david22573/codepicker/domain/errors"
)

// Status represents the lifecycle state of a task or step
type Status string

const (
	StatusPending   Status = "pending"
	StatusRunning   Status = "running"
	StatusCompleted Status = "completed"
	StatusFailed    Status = "failed"
)

// Step represents a single unit of work in a plan
type Step struct {
	ID          int      `json:"id"`
	Description string   `json:"description"` // Human-readable goal
	Instruction string   `json:"instruction"` // Prompt for the worker agent
	Files       []string `json:"files"`       // Context files needed
	Status      Status   `json:"status"`
	Result      string   `json:"result,omitempty"`
	Error       error    `json:"-"`
}

// Plan represents a sequence of steps to achieve a goal
type Plan struct {
	ID            string    `json:"id"`
	OriginalTask  string    `json:"original_task"`
	Reasoning     string    `json:"reasoning"`
	Steps         []Step    `json:"steps"`
	EstimatedCost float64   `json:"estimated_cost"`
	Status        Status    `json:"status"`
	CreatedAt     time.Time `json:"created_at"`
}

// NewPlan creates a fresh plan
func NewPlan(id, taskStr, reasoning string) *Plan {
	return &Plan{
		ID:           id,
		OriginalTask: taskStr,
		Reasoning:    reasoning,
		Status:       StatusPending,
		CreatedAt:    time.Now(),
		Steps:        make([]Step, 0),
	}
}

// AddStep appends a step to the plan
func (p *Plan) AddStep(description, instruction string, files []string) {
	p.Steps = append(p.Steps, Step{
		ID:          len(p.Steps) + 1,
		Description: description,
		Instruction: instruction,
		Files:       files,
		Status:      StatusPending,
	})
}

// MarkStepComplete updates a step's status
func (p *Plan) MarkStepComplete(stepID int, result string) error {
	if stepID < 1 || stepID > len(p.Steps) {
		return errors.NewValidation("plan.MarkStepComplete", "invalid step ID")
	}
	// Steps are 1-indexed for display, 0-indexed for slice
	p.Steps[stepID-1].Status = StatusCompleted
	p.Steps[stepID-1].Result = result
	return nil
}

// MarkStepFailed updates a step's error state
func (p *Plan) MarkStepFailed(stepID int, err error) error {
	if stepID < 1 || stepID > len(p.Steps) {
		return errors.NewValidation("plan.MarkStepFailed", "invalid step ID")
	}
	p.Steps[stepID-1].Status = StatusFailed
	p.Steps[stepID-1].Error = err
	return nil
}
```

---

## File: domain/validation/validator.go
```go
package validation

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/david22573/codepicker/domain/errors"
)

// Validator provides centralized input validation for security and correctness.
// It prevents injection attacks, path traversal, and LLM context overflow.
type Validator struct {
	maxInputLength    int
	maxTokens         int
	allowedFileExts   map[string]bool
	forbiddenPatterns []*regexp.Regexp
	mu                sync.RWMutex
}

// NewValidator creates a Validator with secure defaults optimized for Kimi K2.5.
func NewValidator() *Validator {
	v := &Validator{
		maxInputLength: 10000,
		maxTokens:      180000, // 200K limit minus safety buffer
		allowedFileExts: map[string]bool{
			".go": true, ".md": true, ".yaml": true, ".yml": true,
			".json": true, ".mod": true, ".sum": true, ".txt": true,
			".gitignore": true, ".dockerignore": true, ".toml": true,
		},
	}
	v.compilePatterns()
	return v
}

// compilePatterns initializes regex patterns for dangerous content detection.
func (v *Validator) compilePatterns() {
	patterns := []string{
		`(?i)(rm\s+-rf|drop\s+table|delete\s+from\s+\w+\s+where\s+1\s*=\s*1)`,
		`(?i)(chmod\s+777|eval\s*\(|os\.exec\s*\(|exec\.command)`,
		`(?i)(curl\s+.*\|\s*sh|wget\s+.*\|\s*sh|\|\s*bash)`,
		`(?i)(<script|javascript:|onerror\s*=|onload\s*=)`,
		`(\.\./|\.\.\\)`, // Path traversal attempts
		`(?i)(sudo|su\s+-|passwd|/etc/shadow|mkfs|dd\s+if=)`,
	}

	for _, p := range patterns {
		if re, err := regexp.Compile(p); err == nil {
			v.forbiddenPatterns = append(v.forbiddenPatterns, re)
		}
	}
}

// ValidateUserInput checks task descriptions and user prompts for safety and size limits.
func (v *Validator) ValidateUserInput(input string) error {
	if strings.TrimSpace(input) == "" {
		return errors.NewValidation("validator.user_input", "input cannot be empty")
	}

	v.mu.RLock()
	maxLen := v.maxInputLength
	v.mu.RUnlock()

	if len(input) > maxLen {
		return errors.NewValidation("validator.user_input",
			fmt.Sprintf("input exceeds maximum length of %d characters", maxLen))
	}

	if !utf8.ValidString(input) {
		return errors.NewValidation("validator.user_input", "input contains invalid UTF-8 characters")
	}

	// Check for forbidden patterns (injection protection)
	for _, pattern := range v.forbiddenPatterns {
		if pattern.MatchString(input) {
			return errors.NewValidation("validator.user_input",
				"input contains potentially dangerous content")
		}
	}

	return nil
}

// ValidateFilePath ensures paths are safe, relative, and have allowed extensions.
// This is the primary defense against path traversal attacks (Phase 1.1).
func (v *Validator) ValidateFilePath(path string) error {
	if path == "" {
		return errors.NewValidation("validator.file_path", "path cannot be empty")
	}

	// Normalize and check for traversal
	clean := filepath.Clean(path)

	// Reject absolute paths
	if filepath.IsAbs(clean) {
		return errors.NewValidation("validator.file_path", "absolute paths not allowed")
	}

	// Check for path traversal after cleaning (defense in depth)
	if strings.Contains(clean, "..") {
		return errors.NewValidation("validator.file_path", "path traversal detected")
	}

	// Validate extension unless it's a special allowed file
	ext := filepath.Ext(clean)
	if ext == "" && !v.isAllowedSpecialFile(clean) {
		return errors.NewValidation("validator.file_path", "file extension required")
	}

	if ext != "" {
		v.mu.RLock()
		allowed := v.allowedFileExts[ext]
		v.mu.RUnlock()

		if !allowed {
			return errors.NewValidation("validator.file_path",
				fmt.Sprintf("file type %s not allowed", ext))
		}
	}

	return nil
}

// SanitizePath validates and returns a cleaned relative path, verifying it stays within project root.
// Use this before any filesystem operations in ShadowManager or similar components.
func (v *Validator) SanitizePath(projectRoot, relPath string) (string, error) {
	if err := v.ValidateFilePath(relPath); err != nil {
		return "", err
	}

	clean := filepath.Clean(relPath)
	fullPath := filepath.Join(projectRoot, clean)

	// Verify the resolved path doesn't escape project root
	// Handle symlinks by evaluating them
	resolved, err := filepath.EvalSymlinks(fullPath)
	if err != nil {
		// File might not exist yet (new file creation), check parent directory
		resolved = fullPath
	}

	rootResolved, err := filepath.EvalSymlinks(projectRoot)
	if err != nil {
		rootResolved = projectRoot
	}

	// Ensure resolved path has the root as prefix
	if !strings.HasPrefix(strings.ToLower(resolved), strings.ToLower(rootResolved)) {
		return "", errors.NewValidation("validator.sanitize_path", "path escapes project root")
	}

	return clean, nil
}

// ValidateJSON ensures input is valid JSON and within size limits.
// Use this for validating tool arguments before unmarshaling.
func (v *Validator) ValidateJSON(input string, maxSize int) error {
	if maxSize > 0 && len(input) > maxSize {
		return errors.NewValidation("validator.json", "JSON payload exceeds maximum size")
	}

	if !json.Valid([]byte(input)) {
		return errors.NewValidation("validator.json", "invalid JSON format")
	}

	// Additional check: ensure it's an object (prevent JSON bombs)
	var js map[string]any
	if err := json.Unmarshal([]byte(input), &js); err != nil {
		return errors.NewValidation("validator.json", "JSON must be an object")
	}

	return nil
}

// ValidateCommand checks shell commands for dangerous patterns (Phase 1.2 enhancement).
func (v *Validator) ValidateCommand(cmd string) error {
	if strings.TrimSpace(cmd) == "" {
		return errors.NewValidation("validator.command", "command cannot be empty")
	}

	if len(cmd) > 1000 {
		return errors.NewValidation("validator.command", "command exceeds maximum length of 1000 chars")
	}

	// Block dangerous shell operators
	dangerous := []string{";", "&&", "||", "|", "`", "$(", "${", ">", "<("}
	for _, char := range dangerous {
		if strings.Contains(cmd, char) {
			return errors.NewValidation("validator.command",
				fmt.Sprintf("command contains forbidden operator: %s", char))
		}
	}

	return nil
}

// ValidateTokenCount checks estimated token usage against LLM limits (Phase 3).
func (v *Validator) ValidateTokenCount(estimatedTokens int) error {
	if estimatedTokens < 0 {
		return errors.NewValidation("validator.tokens", "token count cannot be negative")
	}

	v.mu.RLock()
	maxTokens := v.maxTokens
	v.mu.RUnlock()

	if estimatedTokens > maxTokens {
		return errors.NewValidation("validator.tokens",
			fmt.Sprintf("estimated tokens %d exceeds limit %d", estimatedTokens, maxTokens))
	}

	return nil
}

// isAllowedSpecialFile checks for extension-less files like Makefile, Dockerfile.
func (v *Validator) isAllowedSpecialFile(name string) bool {
	allowed := []string{"Makefile", "Dockerfile", "LICENSE", "README", "CHANGELOG", "NOTICE"}
	base := strings.ToUpper(filepath.Base(name))
	for _, a := range allowed {
		if base == a || strings.HasPrefix(base, a+".") {
			return true
		}
	}
	return false
}

// SetMaxTokens updates the token limit dynamically (thread-safe).
func (v *Validator) SetMaxTokens(max int) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.maxTokens = max
}

// AddAllowedExtension adds a file extension to the whitelist (thread-safe).
func (v *Validator) AddAllowedExtension(ext string) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if !strings.HasPrefix(ext, ".") {
		ext = "." + ext
	}
	v.allowedFileExts[strings.ToLower(ext)] = true
}

// RemoveAllowedExtension removes a file extension from the whitelist.
func (v *Validator) RemoveAllowedExtension(ext string) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if !strings.HasPrefix(ext, ".") {
		ext = "." + ext
	}
	delete(v.allowedFileExts, strings.ToLower(ext))
}
```

---

## File: go.mod
```mod
module github.com/david22573/codepicker

go 1.25.6

require (
	github.com/spf13/cobra v1.10.2
	go.uber.org/zap v1.27.1
	modernc.org/sqlite v1.44.3
)

require (
	github.com/dustin/go-humanize v1.0.1 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/ncruces/go-strftime v1.0.0 // indirect
	github.com/remyoudompheng/bigfft v0.0.0-20230129092748-24d4a6f8daec // indirect
	github.com/spf13/pflag v1.0.9 // indirect
	go.uber.org/multierr v1.10.0 // indirect
	golang.org/x/exp v0.0.0-20251023183803-a4bb9ffd2546 // indirect
	golang.org/x/sys v0.37.0 // indirect
	modernc.org/libc v1.67.6 // indirect
	modernc.org/mathutil v1.7.1 // indirect
	modernc.org/memory v1.11.0 // indirect
)
```

---

## File: go.sum
```sum
github.com/cpuguy83/go-md2man/v2 v2.0.6/go.mod h1:oOW0eioCTA6cOiMLiUPZOpcVxMig6NIQQ7OS05n1F4g=
github.com/davecgh/go-spew v1.1.1 h1:vj9j/u1bqnvCEfJOwUhtlOARqs3+rkHYY13jYWTU97c=
github.com/davecgh/go-spew v1.1.1/go.mod h1:J7Y8YcW2NihsgmVo/mv3lAwl/skON4iLHjSsI+c5H38=
github.com/dustin/go-humanize v1.0.1 h1:GzkhY7T5VNhEkwH0PVJgjz+fX1rhBrR7pRT3mDkpeCY=
github.com/dustin/go-humanize v1.0.1/go.mod h1:Mu1zIs6XwVuF/gI1OepvI0qD18qycQx+mFykh5fBlto=
github.com/google/pprof v0.0.0-20250317173921-a4b03ec1a45e h1:ijClszYn+mADRFY17kjQEVQ1XRhq2/JR1M3sGqeJoxs=
github.com/google/pprof v0.0.0-20250317173921-a4b03ec1a45e/go.mod h1:boTsfXsheKC2y+lKOCMpSfarhxDeIzfZG1jqGcPl3cA=
github.com/google/uuid v1.6.0 h1:NIvaJDMOsjHA8n1jAhLSgzrAzy1Hgr+hNrb57e+94F0=
github.com/google/uuid v1.6.0/go.mod h1:TIyPZe4MgqvfeYDBFedMoGGpEw/LqOeaOT+nhxU+yHo=
github.com/hashicorp/golang-lru/v2 v2.0.7 h1:a+bsQ5rvGLjzHuww6tVxozPZFVghXaHOwFs4luLUK2k=
github.com/hashicorp/golang-lru/v2 v2.0.7/go.mod h1:QeFd9opnmA6QUJc5vARoKUSoFhyfM2/ZepoAG6RGpeM=
github.com/inconshreveable/mousetrap v1.1.0 h1:wN+x4NVGpMsO7ErUn/mUI3vEoE6Jt13X2s0bqwp9tc8=
github.com/inconshreveable/mousetrap v1.1.0/go.mod h1:vpF70FUmC8bwa3OWnCshd2FqLfsEA9PFc4w1p2J65bw=
github.com/mattn/go-isatty v0.0.20 h1:xfD0iDuEKnDkl03q4limB+vH+GxLEtL/jb4xVJSWWEY=
github.com/mattn/go-isatty v0.0.20/go.mod h1:W+V8PltTTMOvKvAeJH7IuucS94S2C6jfK/D7dTCTo3Y=
github.com/ncruces/go-strftime v1.0.0 h1:HMFp8mLCTPp341M/ZnA4qaf7ZlsbTc+miZjCLOFAw7w=
github.com/ncruces/go-strftime v1.0.0/go.mod h1:Fwc5htZGVVkseilnfgOVb9mKy6w1naJmn9CehxcKcls=
github.com/pmezard/go-difflib v1.0.0 h1:4DBwDE0NGyQoBHbLQYPwSUPoCMWR5BEzIk/f1lZbAQM=
github.com/pmezard/go-difflib v1.0.0/go.mod h1:iKH77koFhYxTK1pcRnkKkqfTogsbg7gZNVY4sRDYZ/4=
github.com/remyoudompheng/bigfft v0.0.0-20230129092748-24d4a6f8daec h1:W09IVJc94icq4NjY3clb7Lk8O1qJ8BdBEF8z0ibU0rE=
github.com/remyoudompheng/bigfft v0.0.0-20230129092748-24d4a6f8daec/go.mod h1:qqbHyh8v60DhA7CoWK5oRCqLrMHRGoxYCSS9EjAz6Eo=
github.com/russross/blackfriday/v2 v2.1.0/go.mod h1:+Rmxgy9KzJVeS9/2gXHxylqXiyQDYRxCVz55jmeOWTM=
github.com/spf13/cobra v1.10.2 h1:DMTTonx5m65Ic0GOoRY2c16WCbHxOOw6xxezuLaBpcU=
github.com/spf13/cobra v1.10.2/go.mod h1:7C1pvHqHw5A4vrJfjNwvOdzYu0Gml16OCs2GRiTUUS4=
github.com/spf13/pflag v1.0.9 h1:9exaQaMOCwffKiiiYk6/BndUBv+iRViNW+4lEMi0PvY=
github.com/spf13/pflag v1.0.9/go.mod h1:McXfInJRrz4CZXVZOBLb0bTZqETkiAhM9Iw0y3An2Bg=
github.com/stretchr/testify v1.8.1 h1:w7B6lhMri9wdJUVmEZPGGhZzrYTPvgJArz7wNPgYKsk=
github.com/stretchr/testify v1.8.1/go.mod h1:w2LPCIKwWwSfY2zedu0+kehJoqGctiVI29o6fzry7u4=
go.uber.org/goleak v1.3.0 h1:2K3zAYmnTNqV73imy9J1T3WC+gmCePx2hEGkimedGto=
go.uber.org/goleak v1.3.0/go.mod h1:CoHD4mav9JJNrW/WLlf7HGZPjdw8EucARQHekz1X6bE=
go.uber.org/multierr v1.10.0 h1:S0h4aNzvfcFsC3dRF1jLoaov7oRaKqRGC/pUEJ2yvPQ=
go.uber.org/multierr v1.10.0/go.mod h1:20+QtiLqy0Nd6FdQB9TLXag12DsQkrbs3htMFfDN80Y=
go.uber.org/zap v1.27.1 h1:08RqriUEv8+ArZRYSTXy1LeBScaMpVSTBhCeaZYfMYc=
go.uber.org/zap v1.27.1/go.mod h1:GB2qFLM7cTU87MWRP2mPIjqfIDnGu+VIO4V/SdhGo2E=
go.yaml.in/yaml/v3 v3.0.4/go.mod h1:DhzuOOF2ATzADvBadXxruRBLzYTpT36CKvDb3+aBEFg=
golang.org/x/exp v0.0.0-20251023183803-a4bb9ffd2546 h1:mgKeJMpvi0yx/sU5GsxQ7p6s2wtOnGAHZWCHUM4KGzY=
golang.org/x/exp v0.0.0-20251023183803-a4bb9ffd2546/go.mod h1:j/pmGrbnkbPtQfxEe5D0VQhZC6qKbfKifgD0oM7sR70=
golang.org/x/mod v0.29.0 h1:HV8lRxZC4l2cr3Zq1LvtOsi/ThTgWnUk/y64QSs8GwA=
golang.org/x/mod v0.29.0/go.mod h1:NyhrlYXJ2H4eJiRy/WDBO6HMqZQ6q9nk4JzS3NuCK+w=
golang.org/x/sync v0.17.0 h1:l60nONMj9l5drqw6jlhIELNv9I0A4OFgRsG9k2oT9Ug=
golang.org/x/sync v0.17.0/go.mod h1:9KTHXmSnoGruLpwFjVSX0lNNA75CykiMECbovNTZqGI=
golang.org/x/sys v0.6.0/go.mod h1:oPkhp1MJrh7nUepCBck5+mAzfO9JrbApNNgaTdGDITg=
golang.org/x/sys v0.37.0 h1:fdNQudmxPjkdUTPnLn5mdQv7Zwvbvpaxqs831goi9kQ=
golang.org/x/sys v0.37.0/go.mod h1:OgkHotnGiDImocRcuBABYBEXf8A9a87e/uXjp9XT3ks=
golang.org/x/tools v0.38.0 h1:Hx2Xv8hISq8Lm16jvBZ2VQf+RLmbd7wVUsALibYI/IQ=
golang.org/x/tools v0.38.0/go.mod h1:yEsQ/d/YK8cjh0L6rZlY8tgtlKiBNTL14pGDJPJpYQs=
gopkg.in/check.v1 v0.0.0-20161208181325-20d25e280405/go.mod h1:Co6ibVJAznAaIkqp8huTwlJQCZ016jof/cbN4VW5Yz0=
gopkg.in/yaml.v3 v3.0.1 h1:fxVm/GzAzEWqLHuvctI91KS9hhNmmWOoWu0XTYJS7CA=
gopkg.in/yaml.v3 v3.0.1/go.mod h1:K4uyk7z7BCEPqu6E+C64Yfv1cQ7kz7rIZviUmN+EgEM=
modernc.org/cc/v4 v4.27.1 h1:9W30zRlYrefrDV2JE2O8VDtJ1yPGownxciz5rrbQZis=
modernc.org/cc/v4 v4.27.1/go.mod h1:uVtb5OGqUKpoLWhqwNQo/8LwvoiEBLvZXIQ/SmO6mL0=
modernc.org/ccgo/v4 v4.30.1 h1:4r4U1J6Fhj98NKfSjnPUN7Ze2c6MnAdL0hWw6+LrJpc=
modernc.org/ccgo/v4 v4.30.1/go.mod h1:bIOeI1JL54Utlxn+LwrFyjCx2n2RDiYEaJVSrgdrRfM=
modernc.org/fileutil v1.3.40 h1:ZGMswMNc9JOCrcrakF1HrvmergNLAmxOPjizirpfqBA=
modernc.org/fileutil v1.3.40/go.mod h1:HxmghZSZVAz/LXcMNwZPA/DRrQZEVP9VX0V4LQGQFOc=
modernc.org/gc/v2 v2.6.5 h1:nyqdV8q46KvTpZlsw66kWqwXRHdjIlJOhG6kxiV/9xI=
modernc.org/gc/v2 v2.6.5/go.mod h1:YgIahr1ypgfe7chRuJi2gD7DBQiKSLMPgBQe9oIiito=
modernc.org/gc/v3 v3.1.1 h1:k8T3gkXWY9sEiytKhcgyiZ2L0DTyCQ/nvX+LoCljoRE=
modernc.org/gc/v3 v3.1.1/go.mod h1:HFK/6AGESC7Ex+EZJhJ2Gni6cTaYpSMmU/cT9RmlfYY=
modernc.org/goabi0 v0.2.0 h1:HvEowk7LxcPd0eq6mVOAEMai46V+i7Jrj13t4AzuNks=
modernc.org/goabi0 v0.2.0/go.mod h1:CEFRnnJhKvWT1c1JTI3Avm+tgOWbkOu5oPA8eH8LnMI=
modernc.org/libc v1.67.6 h1:eVOQvpModVLKOdT+LvBPjdQqfrZq+pC39BygcT+E7OI=
modernc.org/libc v1.67.6/go.mod h1:JAhxUVlolfYDErnwiqaLvUqc8nfb2r6S6slAgZOnaiE=
modernc.org/mathutil v1.7.1 h1:GCZVGXdaN8gTqB1Mf/usp1Y/hSqgI2vAGGP4jZMCxOU=
modernc.org/mathutil v1.7.1/go.mod h1:4p5IwJITfppl0G4sUEDtCr4DthTaT47/N3aT6MhfgJg=
modernc.org/memory v1.11.0 h1:o4QC8aMQzmcwCK3t3Ux/ZHmwFPzE6hf2Y5LbkRs+hbI=
modernc.org/memory v1.11.0/go.mod h1:/JP4VbVC+K5sU2wZi9bHoq2MAkCnrt2r98UGeSK7Mjw=
modernc.org/opt v0.1.4 h1:2kNGMRiUjrp4LcaPuLY2PzUfqM/w9N23quVwhKt5Qm8=
modernc.org/opt v0.1.4/go.mod h1:03fq9lsNfvkYSfxrfUhZCWPk1lm4cq4N+Bh//bEtgns=
modernc.org/sortutil v1.2.1 h1:+xyoGf15mM3NMlPDnFqrteY07klSFxLElE2PVuWIJ7w=
modernc.org/sortutil v1.2.1/go.mod h1:7ZI3a3REbai7gzCLcotuw9AC4VZVpYMjDzETGsSMqJE=
modernc.org/sqlite v1.44.3 h1:+39JvV/HWMcYslAwRxHb8067w+2zowvFOUrOWIy9PjY=
modernc.org/sqlite v1.44.3/go.mod h1:CzbrU2lSB1DKUusvwGz7rqEKIq+NUd8GWuBBZDs9/nA=
modernc.org/strutil v1.2.1 h1:UneZBkQA+DX2Rp35KcM69cSsNES9ly8mQWD71HKlOA0=
modernc.org/strutil v1.2.1/go.mod h1:EHkiggD70koQxjVdSBM3JKM7k6L0FbGE5eymy9i3B9A=
modernc.org/token v1.1.0 h1:Xl7Ap9dKaEs5kLoOQeQmPWevfnk/DM5qcLcYlA8ys6Y=
modernc.org/token v1.1.0/go.mod h1:UGzOrNV1mAFSEB63lOFHIpNRUVMvYTc6yu1SMY/XTDM=
```

---

## File: infra/fs/diff.go
```go
package fs

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"path/filepath"

	"github.com/david22573/codepicker/domain/errors"
)

// ChangeType indicates the nature of the file operation
type ChangeType string

const (
	ChangeNew      ChangeType = "NEW"
	ChangeModified ChangeType = "MODIFIED"
	ChangeNoOp     ChangeType = "NO-OP"
)

// FileChangeSummary holds the stats for a pending operation
type FileChangeSummary struct {
	Path       string
	Type       ChangeType
	OldLines   int
	NewLines   int
	DeltaLines int
}

func (s *FileChangeSummary) String() string {
	if s.Type == ChangeNew {
		return fmt.Sprintf("[NEW]      %s (+%d lines)", s.Path, s.NewLines)
	}
	if s.Type == ChangeNoOp {
		return fmt.Sprintf("[NO-OP]    %s (content identical)", s.Path)
	}

	// Format for Modified
	sign := "+"
	if s.DeltaLines < 0 {
		sign = "" // negative number already has sign
	}
	return fmt.Sprintf("[MODIFIED] %s (Lines: %d -> %d | %s%d)", s.Path, s.OldLines, s.NewLines, sign, s.DeltaLines)
}

// Diff analyzes the differences between the shadow file and the real file
func (s *ShadowManager) Diff(relPath string) (*FileChangeSummary, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	cleanPath, err := s.sanitizePath(relPath)
	if err != nil {
		return nil, err
	}

	shadowPath := filepath.Join(s.ProjectRoot, ShadowDir, cleanPath)
	realPath := filepath.Join(s.ProjectRoot, cleanPath)

	shadowContent, err := os.ReadFile(shadowPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("shadow file does not exist: %s", cleanPath)
		}
		return nil, err
	}

	realContent, err := os.ReadFile(realPath)
	isNew := os.IsNotExist(err)

	shadowLines := countLines(shadowContent)

	if isNew {
		return &FileChangeSummary{
			Path:       cleanPath,
			Type:       ChangeNew,
			OldLines:   0,
			NewLines:   shadowLines,
			DeltaLines: shadowLines,
		}, nil
	}

	if bytes.Equal(shadowContent, realContent) {
		return &FileChangeSummary{
			Path:     cleanPath,
			Type:     ChangeNoOp,
			OldLines: shadowLines,
			NewLines: shadowLines,
		}, nil
	}

	realLines := countLines(realContent)
	return &FileChangeSummary{
		Path:       cleanPath,
		Type:       ChangeModified,
		OldLines:   realLines,
		NewLines:   shadowLines,
		DeltaLines: shadowLines - realLines,
	}, nil
}

// Apply moves a file from shadow to real FS (The "Commit" action)
// Updated with Mutex and Sanitization for Phase 1.1
func (s *ShadowManager) Apply(relPath string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	cleanPath, err := s.sanitizePath(relPath)
	if err != nil {
		return err
	}

	shadowPath := filepath.Join(s.ProjectRoot, ShadowDir, cleanPath)
	realPath := filepath.Join(s.ProjectRoot, cleanPath)

	content, err := os.ReadFile(shadowPath)
	if err != nil {
		return errors.NewValidation("fs.Apply", "shadow file not found: "+cleanPath)
	}

	if err := os.MkdirAll(filepath.Dir(realPath), 0755); err != nil {
		return errors.NewSystem("fs.Apply", "failed to create dirs", err)
	}

	if err := os.WriteFile(realPath, content, 0644); err != nil {
		return errors.NewSystem("fs.Apply", "failed to write real file", err)
	}

	_ = os.Remove(shadowPath)
	return nil
}

// Helper for Diff
func countLines(data []byte) int {
	sc := bufio.NewScanner(bytes.NewReader(data))
	count := 0
	for sc.Scan() {
		count++
	}
	return count
}
```

---

## File: infra/fs/manager.go
```go
package fs

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// WorkspaceManager handles the lifecycle of execution workspaces and audit trails
type WorkspaceManager struct {
	ProjectRoot string
}

// RunWorkspace represents the isolated directory for a specific execution
type RunWorkspace struct {
	ID      string
	DirPath string
}

func NewWorkspaceManager(root string) *WorkspaceManager {
	return &WorkspaceManager{ProjectRoot: root}
}

// CreateRunWorkspace initializes the directory structure: .codepicker/runs/<timestamp>
func (m *WorkspaceManager) CreateRunWorkspace() (*RunWorkspace, error) {
	// FIX: Corrected time layout from "2006-01-28" to "2006-01-02" so the day is accurate
	timestamp := time.Now().Format("2006-01-02T15-04-05")
	runDirName := timestamp

	fullPath := filepath.Join(m.ProjectRoot, ".codepicker", "runs", runDirName)

	if err := os.MkdirAll(fullPath, 0755); err != nil {
		return nil, fmt.Errorf("failed to create run workspace: %w", err)
	}

	return &RunWorkspace{
		ID:      runDirName,
		DirPath: fullPath,
	}, nil
}

// SaveArtifact writes a file (like context.txt, policy.json) to the run workspace
func (w *RunWorkspace) SaveArtifact(filename string, content []byte) error {
	path := filepath.Join(w.DirPath, filename)
	return os.WriteFile(path, content, 0644)
}

// Path returns the full path to a file within this workspace
func (w *RunWorkspace) Path(filename string) string {
	return filepath.Join(w.DirPath, filename)
}

// ListExecutions returns a list of past run directories (New helper for dashboarding)
func (m *WorkspaceManager) ListExecutions() ([]string, error) {
	runsDir := filepath.Join(m.ProjectRoot, ".codepicker", "runs")
	entries, err := os.ReadDir(runsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return []string{}, nil
		}
		return nil, err
	}

	var runs []string
	for _, e := range entries {
		if e.IsDir() {
			runs = append(runs, e.Name())
		}
	}
	return runs, nil
}
```

---

## File: infra/fs/sandbox.go
```go
package fs

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// Sandbox represents an isolated copy of the project for verification
type Sandbox struct {
	OriginalRoot string
	SandboxRoot  string
}

// NewSandbox creates a temporary directory and syncs the project files to it
// It explicitly excludes .git, .codepicker, and other heavy artifacts.
func NewSandbox(projectRoot string) (*Sandbox, error) {
	// Create a temp directory for the sandbox
	tmpDir, err := os.MkdirTemp("", "codepicker-sandbox-*")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp dir: %w", err)
	}

	s := &Sandbox{
		OriginalRoot: projectRoot,
		SandboxRoot:  tmpDir,
	}

	// Copy files from original project to sandbox
	if err := s.syncFiles(); err != nil {
		_ = os.RemoveAll(tmpDir) // Clean up on failure
		return nil, fmt.Errorf("failed to sync files to sandbox: %w", err)
	}

	return s, nil
}

// Cleanup removes the temporary sandbox directory
func (s *Sandbox) Cleanup() {
	_ = os.RemoveAll(s.SandboxRoot)
}

// ApplyPatch runs 'git apply' inside the sandbox
func (s *Sandbox) ApplyPatch(patchContent []byte) error {
	patchPath := filepath.Join(s.SandboxRoot, "temp.diff")
	if err := os.WriteFile(patchPath, patchContent, 0644); err != nil {
		return fmt.Errorf("failed to write patch file: %w", err)
	}

	cmd := exec.Command("git", "apply", "temp.diff")
	cmd.Dir = s.SandboxRoot

	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("patch failed: %v\nOutput: %s", err, string(out))
	}

	return nil
}

// RunGoCommand executes a go command (test, build, vet) inside the sandbox
func (s *Sandbox) RunGoCommand(ctx context.Context, args ...string) (string, error) {
	// 2-minute timeout prevents hanging tests from blocking the agent forever
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()

	cmd := exec.CommandContext(ctx, "go", args...)
	cmd.Dir = s.SandboxRoot
	cmd.Env = os.Environ() // Inherit environment (PATH, GOPATH, etc.)

	out, err := cmd.CombinedOutput()
	if err != nil {
		return string(out), err
	}

	return string(out), nil
}

// syncFiles copies the source code to the sandbox
func (s *Sandbox) syncFiles() error {
	return filepath.Walk(s.OriginalRoot, func(path string, info os.FileInfo, err error) error {
		// FIX: Stop swallowing errors. If we can't read a file, we should fail.
		if err != nil {
			return fmt.Errorf("access error at %s: %w", path, err)
		}

		relPath, _ := filepath.Rel(s.OriginalRoot, path)
		if relPath == "." {
			return nil
		}

		// Skip hidden dirs, vendor, and node_modules to keep sandbox light
		if info.IsDir() {
			name := info.Name()
			if (strings.HasPrefix(name, ".") && name != ".") || name == "vendor" || name == "node_modules" {
				return filepath.SkipDir
			}
			// Create directory in sandbox
			return os.MkdirAll(filepath.Join(s.SandboxRoot, relPath), info.Mode())
		}

		// Skip hidden files (like .env or .DS_Store)
		if strings.HasPrefix(info.Name(), ".") {
			return nil
		}

		// Perform the copy
		src, err := os.Open(path)
		if err != nil {
			return err
		}
		defer src.Close()

		dst, err := os.Create(filepath.Join(s.SandboxRoot, relPath))
		if err != nil {
			return err
		}
		defer dst.Close()

		_, err = io.Copy(dst, src)
		return err
	})
}
```

---

## File: infra/fs/shadow.go
```go
package fs

import (
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/david22573/codepicker/domain/errors"
)

const ShadowDir = ".codepicker/shadow"

// ShadowManager handles the safe staging of file changes.
type ShadowManager struct {
	ProjectRoot string
	mu          sync.RWMutex
}

func NewShadowManager(root string) *ShadowManager {
	return &ShadowManager{ProjectRoot: root}
}

// sanitizePath prevents directory traversal attacks.
func (s *ShadowManager) sanitizePath(relPath string) (string, error) {
	clean := filepath.Clean(relPath)
	if filepath.IsAbs(clean) {
		return "", errors.NewValidation("fs.sanitize", "absolute paths not allowed")
	}
	if strings.HasPrefix(clean, "..") {
		return "", errors.NewValidation("fs.sanitize", "path traversal detected")
	}
	return clean, nil
}

// Write saves content to the shadow directory.
func (s *ShadowManager) Write(relPath string, content []byte) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	cleanPath, err := s.sanitizePath(relPath)
	if err != nil {
		return "", err
	}

	shadowPath := filepath.Join(s.ProjectRoot, ShadowDir, cleanPath)
	if err := os.MkdirAll(filepath.Dir(shadowPath), 0755); err != nil {
		return "", errors.NewSystem("fs.Write", "failed to create shadow dirs", err)
	}

	if err := os.WriteFile(shadowPath, content, 0644); err != nil {
		return "", errors.NewSystem("fs.Write", "failed to write shadow file", err)
	}

	return shadowPath, nil
}

// Read from shadow first, then real FS.
func (s *ShadowManager) Read(relPath string) ([]byte, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	cleanPath, err := s.sanitizePath(relPath)
	if err != nil {
		return nil, err
	}

	shadowPath := filepath.Join(s.ProjectRoot, ShadowDir, cleanPath)
	if _, err := os.Stat(shadowPath); err == nil {
		return os.ReadFile(shadowPath)
	}

	return os.ReadFile(filepath.Join(s.ProjectRoot, cleanPath))
}

// Commit changes to the real filesystem.
func (s *ShadowManager) Commit(relPath string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	cleanPath, err := s.sanitizePath(relPath)
	if err != nil {
		return err
	}

	shadowPath := filepath.Join(s.ProjectRoot, ShadowDir, cleanPath)
	realPath := filepath.Join(s.ProjectRoot, cleanPath)

	content, err := os.ReadFile(shadowPath)
	if err != nil {
		return errors.NewValidation("fs.Commit", "shadow file not found")
	}

	if err := os.MkdirAll(filepath.Dir(realPath), 0755); err != nil {
		return errors.NewSystem("fs.Commit", "failed to create dirs", err)
	}

	if err := os.WriteFile(realPath, content, 0644); err != nil {
		return errors.NewSystem("fs.Commit", "failed to write real file", err)
	}

	return os.Remove(shadowPath)
}
```

---

## File: infra/fs/shadow_test.go
```go
package fs

import (
	"os"
	"testing"
)

func TestShadowManager_Security(t *testing.T) {
	sm := NewShadowManager("/tmp/project")

	attacks := []string{"/etc/passwd", "../../secret", "cmd/../../evil"}
	for _, path := range attacks {
		_, err := sm.Write(path, []byte("test"))
		if err == nil {
			t.Errorf("Security Breach: failed to block path %s", path)
		}
	}
}

func TestShadowManager_Cycle(t *testing.T) {
	tmp, _ := os.MkdirTemp("", "shadow-test-*")
	defer os.RemoveAll(tmp)

	sm := NewShadowManager(tmp)
	rel := "test.go"
	content := []byte("data")

	// Fix: handle BOTH return values
	path, err := sm.Write(rel, content)
	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}
	if path == "" {
		t.Fatal("Expected path, got empty string")
	}

	if err := sm.Commit(rel); err != nil {
		t.Fatalf("Commit failed: %v", err)
	}
}
```

---

## File: infra/git/client.go
```go
package git

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/david22573/codepicker/domain/audit"
)

// Client wraps git command line operations.
type Client struct {
	ProjectRoot string
}

func NewClient(root string) *Client {
	return &Client{ProjectRoot: root}
}

// StageAll runs 'git add .' to stage all changes.
func (c *Client) StageAll() error {
	cmd := exec.Command("git", "add", ".")
	cmd.Dir = c.ProjectRoot
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git add failed: %s", string(out))
	}
	return nil
}

// Commit creates a new commit with the provenance message.
func (c *Client) Commit(p *audit.Provenance) (string, error) {
	msg := p.FormatCommitMessage()

	// We use the -m flag. For very large messages, passing via stdin is safer,
	// but for metadata this is sufficient.
	cmd := exec.Command("git", "commit", "-m", msg)
	cmd.Dir = c.ProjectRoot

	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git commit failed: %s", string(out))
	}

	// Return the output (usually contains the short hash and subject)
	return strings.TrimSpace(string(out)), nil
}

// IsDirty checks if there are uncommitted changes (to prevent committing unintended files).
func (c *Client) IsDirty() bool {
	cmd := exec.Command("git", "status", "--porcelain")
	cmd.Dir = c.ProjectRoot
	out, _ := cmd.Output()
	return len(out) > 0
}
```

---

## File: infra/hooks/formatter.go
```go
package hooks

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"time"
)

// Formatter defines a supported code formatter configuration
type Formatter struct {
	Name    string
	Command string
	Args    []string
}

// SupportedFormatters maps extensions to their respective tools
var SupportedFormatters = map[string]Formatter{
	".go":   {Name: "gofmt", Command: "gofmt", Args: []string{"-w"}},
	".js":   {Name: "prettier", Command: "npx", Args: []string{"prettier", "--write"}},
	".ts":   {Name: "prettier", Command: "npx", Args: []string{"prettier", "--write"}},
	".json": {Name: "prettier", Command: "npx", Args: []string{"prettier", "--write"}},
	".py":   {Name: "black", Command: "black", Args: []string{}},
}

// RunFormatter attempts to format a specific file based on its extension
func RunFormatter(ctx context.Context, path string) error {
	ext := filepath.Ext(path)
	formatter, exists := SupportedFormatters[ext]
	if !exists {
		return nil // No formatter defined for this type
	}

	// Verify tool exists in PATH
	if _, err := exec.LookPath(formatter.Command); err != nil {
		fmt.Printf("⚠️  Skipping format for %s: '%s' not found in PATH.\n", filepath.Base(path), formatter.Command)
		return nil
	}

	// Execute with timeout to prevent hangs
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	fullArgs := append(formatter.Args, path)
	cmd := exec.CommandContext(ctx, formatter.Command, fullArgs...)

	output, err := cmd.CombinedOutput()
	if err != nil {
		// Return error but include output for debugging
		return fmt.Errorf("formatter %s failed: %v\nOutput: %s", formatter.Name, err, string(output))
	}

	fmt.Printf("✨ Formatted %s using %s\n", filepath.Base(path), formatter.Name)
	return nil
}
```

---

## File: infra/indexer/code_slicer.go
```go
package indexer

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"os"

	"github.com/david22573/codepicker/domain/context"
)

type CodeSlicer struct {
	fset *token.FileSet
}

func NewCodeSlicer() *CodeSlicer {
	return &CodeSlicer{
		fset: token.NewFileSet(),
	}
}

// SliceFile parses a Go file and breaks it into semantic CodeSlices
func (s *CodeSlicer) SliceFile(filePath string) ([]context.CodeSlice, error) {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	// 1. Parse the file into an AST
	node, err := parser.ParseFile(s.fset, filePath, content, parser.ParseComments)
	if err != nil {
		return nil, fmt.Errorf("failed to parse AST: %w", err)
	}

	var slices []context.CodeSlice
	fileHash := computeHash(content)

	// 2. Walk the AST to find top-level declarations
	for _, decl := range node.Decls {
		switch d := decl.(type) {
		case *ast.FuncDecl:
			// Extract Function/Method
			slices = append(slices, s.createSlice(filePath, d, context.SliceTypeFunction, d.Name.Name, content, fileHash))

		case *ast.GenDecl:
			// Extract Types (Structs/Interfaces)
			for _, spec := range d.Specs {
				if typeSpec, ok := spec.(*ast.TypeSpec); ok {
					slices = append(slices, s.createSlice(filePath, d, s.getSliceType(typeSpec), typeSpec.Name.Name, content, fileHash))
				}
			}
		}
	}

	return slices, nil
}

// createSlice transforms an AST node into our domain CodeSlice model
func (s *CodeSlicer) createSlice(path string, node ast.Node, sType context.SliceType, name string, fullContent []byte, fileHash string) context.CodeSlice {
	start := s.fset.Position(node.Pos()).Line
	end := s.fset.Position(node.End()).Line

	// Extract the actual source code for this node
	var buf bytes.Buffer
	format.Node(&buf, s.fset, node)

	return context.CodeSlice{
		ID:        fmt.Sprintf("%s-%s-%d", path, name, start),
		FilePath:  path,
		StartLine: start,
		EndLine:   end,
		Content:   buf.String(),
		Language:  "go",
		SliceType: sType,
		Symbols:   []string{name},
		Hash:      fileHash,
	}
}

func (s *CodeSlicer) getSliceType(spec *ast.TypeSpec) context.SliceType {
	switch spec.Type.(type) {
	case *ast.StructType:
		return context.SliceTypeStruct
	case *ast.InterfaceType:
		return context.SliceTypeInterface
	default:
		return context.SliceTypeBlock
	}
}

func computeHash(data []byte) string {
	h := sha256.New()
	h.Write(data)
	return hex.EncodeToString(h.Sum(nil))
}
```

---

## File: infra/indexer/code_slicer_test.go
```go
package indexer

import (
	"os"
	"testing"
)

func TestCodeSlicer_SliceFile(t *testing.T) {
	// Create a temporary Go file to test
	testFile := "test_slice.go"
	content := []byte(`package test
	
// MyStruct is a test struct
type MyStruct struct {
	ID int
}

func (m *MyStruct) GetID() int {
	return m.ID
}
`)
	os.WriteFile(testFile, content, 0644)
	defer os.Remove(testFile)

	slicer := NewCodeSlicer()
	slices, err := slicer.SliceFile(testFile)

	if err != nil {
		t.Fatalf("Slicing failed: %v", err)
	}

	if len(slices) != 2 {
		t.Errorf("Expected 2 slices, got %d", len(slices))
	}

	// Check if we found the struct and the method
	foundStruct := false
	foundMethod := false
	for _, s := range slices {
		if s.Symbols[0] == "MyStruct" {
			foundStruct = true
		}
		if s.Symbols[0] == "GetID" {
			foundMethod = true
		}
	}

	if !foundStruct || !foundMethod {
		t.Error("Failed to identify struct or method symbols")
	}
}
```

---

## File: infra/indexer/manager.go
```go
package indexer

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/david22573/codepicker/domain/context"
)

type IndexManager struct {
	slicer *CodeSlicer
	store  context.SliceStore
}

func NewIndexManager(s *CodeSlicer, store context.SliceStore) *IndexManager {
	return &IndexManager{slicer: s, store: store}
}

// IndexDirectory scans the directory and populates the store
func (m *IndexManager) IndexDirectory(rootPath string) error {
	// 1. Resolve to absolute path for reliable walking
	absRoot, err := filepath.Abs(rootPath)
	if err != nil {
		return err
	}

	fmt.Printf("📂 Walking Directory: %s\n", absRoot)

	return filepath.Walk(absRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			fmt.Printf("  ⚠️  Access Error: %s -> %v\n", path, err)
			return nil
		}

		// 2. Skip obvious noise
		if info.IsDir() {
			name := info.Name()
			if (strings.HasPrefix(name, ".") && name != ".") || name == "vendor" || name == "node_modules" {
				return filepath.SkipDir
			}
			return nil
		}

		// 3. Match Go Source Files
		if strings.HasSuffix(info.Name(), ".go") && !strings.HasSuffix(info.Name(), "_test.go") {
			// Get relative path for consistent DB keys
			relPath, _ := filepath.Rel(absRoot, path)

			fmt.Printf("  📄 Slicing: %s\n", relPath)

			slices, err := m.slicer.SliceFile(path)
			if err != nil {
				fmt.Printf("  ❌ Slicer Error in %s: %v\n", relPath, err)
				return nil
			}

			if len(slices) == 0 {
				fmt.Printf("  ℹ️  No semantic units found in %s\n", relPath)
				return nil
			}

			// Store the results using the RELATIVE path as the key
			if err := m.store.IndexFile(relPath, slices); err != nil {
				fmt.Printf("  ❌ Database Error in %s: %v\n", relPath, err)
				return err
			}
			fmt.Printf("  ✅ Indexed %d slices\n", len(slices))
		}
		return nil
	})
}
```

---

## File: infra/llm/openrouter.go
```go
package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/david22573/codepicker/domain/errors"
)

const openRouterURL = "https://openrouter.ai/api/v1/chat/completions"

// OpenRouterAdapter implements domain.agent.LLMClient
type OpenRouterAdapter struct {
	apiKey string
	model  string
	client *http.Client
}

// NewOpenRouterAdapter creates a new client instance
func NewOpenRouterAdapter(apiKey, model string) *OpenRouterAdapter {
	return &OpenRouterAdapter{
		apiKey: apiKey,
		model:  model,
		// Increased timeout to 120s because "thinking" models (like Nemotron/Llama-3)
		// take longer to generate the first token.
		client: &http.Client{Timeout: 120 * time.Second},
	}
}

// Chat fulfills the domain.agent.LLMClient interface with retries
func (c *OpenRouterAdapter) Chat(ctx context.Context, systemPrompt, userMsg string) (string, error) {
	// Simple retry logic (3 attempts)
	maxRetries := 3
	var lastErr error

	for i := 0; i < maxRetries; i++ {
		response, err := c.doChat(ctx, systemPrompt, userMsg)
		if err == nil {
			return response, nil
		}

		// If it's a context cancellation (user pressed Ctrl+C), stop immediately
		if ctx.Err() != nil {
			return "", ctx.Err()
		}

		lastErr = err
		// Exponential backoff: 1s, 2s, 4s
		time.Sleep(time.Duration(1<<i) * time.Second)
	}

	return "", lastErr
}

func (c *OpenRouterAdapter) doChat(ctx context.Context, systemPrompt, userMsg string) (string, error) {
	// Prepare the request payload
	reqBody := map[string]interface{}{
		"model": c.model,
		"messages": []map[string]string{
			{"role": "system", "content": systemPrompt},
			{"role": "user", "content": userMsg},
		},
		"temperature": 0.1, // Slight temp often helps avoid "stuck" loops better than 0.0
		// Optional: Add OpenRouter specific provider preferences if needed
		// "provider": map[string]string{"order": "Liquid,DeepInfra"},
	}

	// Specific tweak for models that support/require "reasoning" parameter
	if strings.Contains(c.model, "nemotron") || strings.Contains(c.model, "thinking") {
		reqBody["include_reasoning"] = true
	}

	jsonBytes, err := json.Marshal(reqBody)
	if err != nil {
		return "", errors.NewSystem("llm.Chat", "failed to marshal request", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", openRouterURL, bytes.NewBuffer(jsonBytes))
	if err != nil {
		return "", errors.NewSystem("llm.Chat", "failed to create request", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("HTTP-Referer", "https://github.com/david22573/codepicker")
	req.Header.Set("X-Title", "CodePicker")

	resp, err := c.client.Do(req)
	if err != nil {
		return "", errors.NewLLM("llm.Chat", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	// IMPROVEMENT: Handle non-200 status codes gracefully
	if resp.StatusCode != http.StatusOK {
		return "", errors.NewLLM("llm.Chat", fmt.Errorf("API error %d: %s", resp.StatusCode, string(body)))
	}

	// IMPROVEMENT: Handle empty bodies which cause "unexpected end of JSON"
	if len(body) == 0 {
		return "", errors.NewLLM("llm.Chat", fmt.Errorf("received empty response body from API"))
	}

	// Parse the response
	var result struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
			FinishReason string `json:"finish_reason"` // Useful for debugging
		} `json:"choices"`
		Error *struct { // Capture API-level errors that return 200 OK (rare but possible)
			Message string `json:"message"`
		} `json:"error,omitempty"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		// Log the raw body for debugging if parsing fails
		return "", errors.NewSystem("llm.Chat", fmt.Sprintf("failed to parse response: %s", string(body)), err)
	}

	if result.Error != nil {
		return "", errors.NewLLM("llm.Chat", fmt.Errorf("provider error: %s", result.Error.Message))
	}

	if len(result.Choices) == 0 {
		return "", errors.NewLLM("llm.Chat", fmt.Errorf("empty response choices from API"))
	}

	return result.Choices[0].Message.Content, nil
}
```

---

## File: infra/logging/logger.go
```go
package logging

import (
	"context"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type Logger struct {
	zap *zap.Logger
}

type contextKey string

const (
	requestIDKey   contextKey = "request_id"
	executionIDKey contextKey = "execution_id"
)

// NewLogger initializes the logging system.
// env: "production" (JSON, Info level) or "development" (Console, Debug level)
func NewLogger(env string) (*Logger, error) {
	var config zap.Config

	if env == "production" {
		config = zap.NewProductionConfig()
		config.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
		config.OutputPaths = []string{"stdout", ".codepicker/app.log"}
	} else {
		config = zap.NewDevelopmentConfig()
		config.EncoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
		config.EncoderConfig.EncodeTime = zapcore.TimeEncoderOfLayout("15:04:05")
		// Shorten caller path to just package/file
		config.EncoderConfig.EncodeCaller = zapcore.ShortCallerEncoder
	}

	// Build with stacktrace only for errors
	logger, err := config.Build(
		zap.AddCaller(),
		zap.AddStacktrace(zapcore.ErrorLevel),
	)
	if err != nil {
		return nil, err
	}

	return &Logger{zap: logger}, nil
}

// WithContext enriches the logger with trace IDs from the context
func (l *Logger) WithContext(ctx context.Context) *Logger {
	fields := []zap.Field{}

	// Extract standard tracing keys if they exist
	if reqID, ok := ctx.Value(requestIDKey).(string); ok {
		fields = append(fields, zap.String("request_id", reqID))
	}

	if execID, ok := ctx.Value(executionIDKey).(string); ok {
		fields = append(fields, zap.String("execution_id", execID))
	}

	if len(fields) > 0 {
		return &Logger{zap: l.zap.With(fields...)}
	}
	return l
}

// Core logging methods
func (l *Logger) Info(msg string, fields ...zap.Field) {
	l.zap.Info(msg, fields...)
}

func (l *Logger) Error(msg string, fields ...zap.Field) {
	l.zap.Error(msg, fields...)
}

func (l *Logger) Warn(msg string, fields ...zap.Field) {
	l.zap.Warn(msg, fields...)
}

func (l *Logger) Debug(msg string, fields ...zap.Field) {
	l.zap.Debug(msg, fields...)
}

func (l *Logger) Fatal(msg string, fields ...zap.Field) {
	l.zap.Fatal(msg, fields...)
}

// Sync flushes any buffered logs (call this on exit)
func (l *Logger) Sync() error {
	return l.zap.Sync()
}

// --- Domain Specific Helpers ---

// LogAgentAction logs high-level agent decisions
func (l *Logger) LogAgentAction(agentName, action string, metadata map[string]interface{}) {
	fields := []zap.Field{
		zap.String("component", "agent"),
		zap.String("agent_name", agentName),
		zap.String("action", action),
	}

	for k, v := range metadata {
		fields = append(fields, zap.Any(k, v))
	}

	l.Info("Agent Action", fields...)
}

// LogToolExecution records the result and duration of a tool call
func (l *Logger) LogToolExecution(toolName, args string, duration time.Duration, err error) {
	fields := []zap.Field{
		zap.String("component", "tool"),
		zap.String("tool", toolName),
		zap.String("args", args),
		zap.Duration("duration", duration),
	}

	if err != nil {
		fields = append(fields, zap.Error(err))
		l.Error("Tool Execution Failed", fields...)
	} else {
		l.Info("Tool Execution Success", fields...)
	}
}

// LogLLMCall tracks token usage and latency for cost monitoring
func (l *Logger) LogLLMCall(model string, duration time.Duration, promptTokens, completionTokens int, err error) {
	fields := []zap.Field{
		zap.String("component", "llm"),
		zap.String("model", model),
		zap.Duration("duration", duration),
		zap.Int("tokens_prompt", promptTokens),
		zap.Int("tokens_completion", completionTokens),
		zap.Int("tokens_total", promptTokens+completionTokens),
	}

	if err != nil {
		fields = append(fields, zap.Error(err))
		l.Error("LLM Request Failed", fields...)
	} else {
		l.Info("LLM Request Completed", fields...)
	}
}
```

---

## File: infra/shell/executor.go
```go
package shell

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"time"

	"github.com/david22573/codepicker/domain/errors"
)

type Executor struct {
	Timeout   time.Duration
	MaxOutput int
}

func NewExecutor(timeout time.Duration, maxOutput int) *Executor {
	return &Executor{
		Timeout:   timeout,
		MaxOutput: maxOutput,
	}
}

func (e *Executor) Run(ctx context.Context, command string, args ...string) (string, error) {
	// Create a context with timeout if one isn't already set
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, e.Timeout)
		defer cancel()
	}

	cmd := exec.CommandContext(ctx, command, args...)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()

	output := stdout.String()
	if stderr.Len() > 0 {
		output += "\nSTDERR:\n" + stderr.String()
	}

	// Truncate if too long
	if len(output) > e.MaxOutput {
		output = output[:e.MaxOutput] + "\n...(truncated)"
	}

	if ctx.Err() == context.DeadlineExceeded {
		return output, errors.NewSystem("shell.Run", "command timed out", ctx.Err())
	}

	if err != nil {
		return output, errors.NewSystem("shell.Run", fmt.Sprintf("command failed: %v", err), err)
	}

	return output, nil
}
```

---

## File: infra/storage/sqlite.go
```go
package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/david22573/codepicker/domain/agent"
	ctxDomain "github.com/david22573/codepicker/domain/context"
	"github.com/david22573/codepicker/domain/task"
	_ "modernc.org/sqlite" // Pure-go driver for maximum Termux compatibility
)

// SQLiteRepository implements agent.Repository and context.SliceStore.
// Enhanced for Phase 2.1 with WAL mode and connection pooling.
type SQLiteRepository struct {
	db *sql.DB
	mu sync.RWMutex
}

// NewSQLiteRepository initializes the DB with production-grade pragmas.
func NewSQLiteRepository(path string) (*SQLiteRepository, error) {
	// DSN optimized for concurrency and reliability on mobile filesystems.
	// journal_mode=WAL: Allows concurrent reads while writing.
	// busy_timeout=5000: Prevents "database is locked" errors during high I/O.
	dsn := fmt.Sprintf("%s?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=synchronous(NORMAL)&_pragma=cache_size(-64000)", path)

	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open sqlite: %w", err)
	}

	// Connection Pool Tuning: SQLite performs best with a single writer.
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(time.Hour)

	if err := migrate(db); err != nil {
		db.Close()
		return nil, err
	}

	return &SQLiteRepository{db: db}, nil
}

func migrate(db *sql.DB) error {
	queries := []string{
		`CREATE TABLE IF NOT EXISTS executions (id TEXT PRIMARY KEY, plan_id TEXT, status TEXT, history_json TEXT, start_time DATETIME, end_time DATETIME);`,
		`CREATE TABLE IF NOT EXISTS plans (id TEXT PRIMARY KEY, original_task TEXT, reasoning TEXT, steps_json TEXT, status TEXT, estimated_cost REAL, created_at DATETIME);`,
		`CREATE TABLE IF NOT EXISTS code_slices (
			id TEXT PRIMARY KEY,
			file_path TEXT NOT NULL,
			start_line INTEGER NOT NULL,
			end_line INTEGER NOT NULL,
			content TEXT NOT NULL,
			language TEXT,
			slice_type TEXT,
			symbols_json TEXT,
			content_hash TEXT,
			indexed_at DATETIME
		);`,
		`CREATE INDEX IF NOT EXISTS idx_file_path ON code_slices(file_path);`,
		`CREATE VIRTUAL TABLE IF NOT EXISTS slices_fts USING fts5(
			id UNINDEXED,
			file_path,
			content,
			symbols,
			content='code_slices',
			content_rowid='rowid'
		);`,
	}
	for _, q := range queries {
		if _, err := db.Exec(q); err != nil {
			return err
		}
	}
	return nil
}

// --- Execution Management ---

func (r *SQLiteRepository) SaveExecution(ctx context.Context, exec *agent.Execution) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	history, _ := json.Marshal(exec.History)
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO executions (id, plan_id, status, history_json, start_time, end_time) 
		VALUES (?, ?, ?, ?, ?, ?) 
		ON CONFLICT(id) DO UPDATE SET 
			status=excluded.status, 
			history_json=excluded.history_json, 
			end_time=excluded.end_time`,
		exec.ID, exec.PlanID, string(exec.Status), string(history), exec.StartTime, exec.EndTime,
	)
	return err
}

func (r *SQLiteRepository) GetExecution(ctx context.Context, id string) (*agent.Execution, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	row := r.db.QueryRowContext(ctx, "SELECT id, plan_id, status, history_json, start_time, end_time FROM executions WHERE id = ?", id)
	var ex agent.Execution
	var hist, status string
	var end sql.NullTime
	if err := row.Scan(&ex.ID, &ex.PlanID, &status, &hist, &ex.StartTime, &end); err != nil {
		return nil, err
	}
	ex.Status = task.Status(status)
	if end.Valid {
		ex.EndTime = end.Time
	}
	json.Unmarshal([]byte(hist), &ex.History)
	return &ex, nil
}

func (r *SQLiteRepository) ListExecutions(ctx context.Context, limit int) ([]agent.ExecutionSummary, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	rows, err := r.db.QueryContext(ctx, "SELECT id, plan_id, status, start_time FROM executions ORDER BY start_time DESC LIMIT ?", limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var res []agent.ExecutionSummary
	for rows.Next() {
		var s agent.ExecutionSummary
		var stat string
		rows.Scan(&s.ID, &s.PlanID, &stat, &s.StartTime)
		s.Status = task.Status(stat)
		res = append(res, s)
	}
	return res, nil
}

// --- Plan Management ---

func (r *SQLiteRepository) SavePlan(ctx context.Context, plan *task.Plan) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	steps, _ := json.Marshal(plan.Steps)
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO plans (id, original_task, reasoning, steps_json, status, estimated_cost, created_at) 
		VALUES (?, ?, ?, ?, ?, ?, ?) 
		ON CONFLICT(id) DO UPDATE SET 
			status=excluded.status, 
			steps_json=excluded.steps_json, 
			reasoning=excluded.reasoning`,
		plan.ID, plan.OriginalTask, plan.Reasoning, string(steps), string(plan.Status), plan.EstimatedCost, plan.CreatedAt,
	)
	return err
}

func (r *SQLiteRepository) GetPlan(ctx context.Context, id string) (*task.Plan, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	row := r.db.QueryRowContext(ctx, "SELECT id, original_task, reasoning, steps_json, status, estimated_cost, created_at FROM plans WHERE id = ?", id)
	var p task.Plan
	var steps, status string
	if err := row.Scan(&p.ID, &p.OriginalTask, &p.Reasoning, &steps, &status, &p.EstimatedCost, &p.CreatedAt); err != nil {
		return nil, err
	}
	p.Status = task.Status(status)
	json.Unmarshal([]byte(steps), &p.Steps)
	return &p, nil
}

func (r *SQLiteRepository) ListPlans(ctx context.Context, limit int) ([]agent.PlanSummary, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	rows, _ := r.db.QueryContext(ctx, "SELECT id, original_task, status, steps_json, created_at FROM plans ORDER BY created_at DESC LIMIT ?", limit)
	defer rows.Close()

	var res []agent.PlanSummary
	for rows.Next() {
		var p agent.PlanSummary
		var steps, stat string
		rows.Scan(&p.ID, &p.OriginalTask, &stat, &steps, &p.CreatedAt)
		p.Status = task.Status(stat)
		var s []task.Step
		json.Unmarshal([]byte(steps), &s)
		p.StepCount = len(s)
		res = append(res, p)
	}
	return res, nil
}

func (r *SQLiteRepository) DeletePlan(ctx context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	_, err := r.db.ExecContext(ctx, "DELETE FROM plans WHERE id = ?", id)
	return err
}

// --- Context & Slice Management ---

func (r *SQLiteRepository) IndexFile(filePath string, slices []ctxDomain.CodeSlice) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec("DELETE FROM code_slices WHERE file_path = ?", filePath); err != nil {
		return err
	}

	for _, s := range slices {
		syms, _ := json.Marshal(s.Symbols)
		_, err = tx.Exec(`
			INSERT INTO code_slices (id, file_path, start_line, end_line, content, language, slice_type, symbols_json, indexed_at) 
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			s.ID, filePath, s.StartLine, s.EndLine, s.Content, s.Language, string(s.SliceType), string(syms), time.Now(),
		)
		if err != nil {
			return err
		}
	}

	_, _ = tx.Exec("INSERT INTO slices_fts(slices_fts) VALUES('rebuild')")
	return tx.Commit()
}

func (r *SQLiteRepository) Query(q ctxDomain.SliceQuery) ([]ctxDomain.CodeSlice, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	searchQuery := strings.Join(q.Keywords, " ")
	if strings.TrimSpace(searchQuery) == "" {
		return nil, nil
	}

	limit := 20
	if q.MaxResults > 0 {
		limit = q.MaxResults
	}

	query := fmt.Sprintf(`
		SELECT id, file_path, start_line, end_line, content, slice_type, symbols_json 
		FROM code_slices 
		WHERE rowid IN (SELECT rowid FROM slices_fts WHERE slices_fts MATCH ?) 
		LIMIT %d`, limit)

	rows, err := r.db.Query(query, searchQuery)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var res []ctxDomain.CodeSlice
	for rows.Next() {
		var s ctxDomain.CodeSlice
		var st, syms string
		rows.Scan(&s.ID, &s.FilePath, &s.StartLine, &s.EndLine, &s.Content, &st, &syms)
		s.SliceType = ctxDomain.SliceType(st)
		json.Unmarshal([]byte(syms), &s.Symbols)
		res = append(res, s)
	}
	return res, nil
}

func (r *SQLiteRepository) GetByFile(path string) ([]ctxDomain.CodeSlice, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	rows, _ := r.db.Query("SELECT id, file_path, start_line, end_line, content, slice_type FROM code_slices WHERE file_path = ?", path)
	defer rows.Close()
	var res []ctxDomain.CodeSlice
	for rows.Next() {
		var s ctxDomain.CodeSlice
		var st string
		rows.Scan(&s.ID, &s.FilePath, &s.StartLine, &s.EndLine, &s.Content, &st)
		s.SliceType = ctxDomain.SliceType(st)
		res = append(res, s)
	}
	return res, nil
}

func (r *SQLiteRepository) InvalidateFile(path string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	_, err := r.db.Exec("DELETE FROM code_slices WHERE file_path = ?", path)
	return err
}

func (r *SQLiteRepository) GetStats() (*ctxDomain.IndexStats, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var s ctxDomain.IndexStats
	err := r.db.QueryRow("SELECT COUNT(*), COUNT(DISTINCT file_path) FROM code_slices").Scan(&s.TotalSlices, &s.TotalFiles)
	s.LastIndexedAt = time.Now()
	return &s, err
}

func (r *SQLiteRepository) GetByID(id string) (*ctxDomain.CodeSlice, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	row := r.db.QueryRow("SELECT id, file_path, start_line, end_line, content, slice_type FROM code_slices WHERE id = ?", id)
	var s ctxDomain.CodeSlice
	var st string
	if err := row.Scan(&s.ID, &s.FilePath, &s.StartLine, &s.EndLine, &s.Content, &st); err != nil {
		return nil, err
	}
	s.SliceType = ctxDomain.SliceType(st)
	return &s, nil
}

func (r *SQLiteRepository) GetBySymbol(symbol string) ([]ctxDomain.CodeSlice, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	rows, err := r.db.Query("SELECT id, file_path, start_line, end_line, content FROM code_slices WHERE symbols_json LIKE ?", "%"+symbol+"%")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var res []ctxDomain.CodeSlice
	for rows.Next() {
		var s ctxDomain.CodeSlice
		rows.Scan(&s.ID, &s.FilePath, &s.StartLine, &s.EndLine, &s.Content)
		res = append(res, s)
	}
	return res, nil
}
```

---

## File: main.go
```go
package main

import "github.com/david22573/codepicker/cmd"

func main() {
	cmd.Execute()
}
```

---

