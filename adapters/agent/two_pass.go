package agent

import (
	"context"
	"fmt"
	"strings"

	domainAgent "github.com/david22573/codepicker/domain/agent"
	"github.com/david22573/codepicker/domain/event"
	"github.com/david22573/codepicker/domain/interaction"
	"github.com/david22573/codepicker/infra/llm"
	"github.com/david22573/codepicker/infra/logging"
	"github.com/david22573/codepicker/infra/ratelimit"
)

type TwoPassEngine struct {
	model         *llm.OpenRouterAdapter
	repo          domainAgent.Repository
	tools         []domainAgent.Tool
	policy        domainAgent.Policy
	logger        *logging.Logger
	costTracker   *llm.CostTracker
	budgetGuard   *llm.BudgetGuard
	rateLimiter   *ratelimit.ToolRateLimiter
	PackedContext string
}

func NewTwoPassEngine(
	model *llm.OpenRouterAdapter,
	repo domainAgent.Repository,
	tools []domainAgent.Tool,
	policy domainAgent.Policy,
	logger *logging.Logger,
	costTracker *llm.CostTracker,
	rateLimiter *ratelimit.ToolRateLimiter,
	budget float64,
	packedContext string,
) *TwoPassEngine {
	bg := llm.NewBudgetGuard(costTracker, budget)

	return &TwoPassEngine{
		model:         model,
		repo:          repo,
		tools:         tools,
		policy:        policy,
		logger:        logger,
		costTracker:   costTracker,
		budgetGuard:   bg,
		rateLimiter:   rateLimiter,
		PackedContext: packedContext,
	}
}

func (e *TwoPassEngine) RunAnalysis(ctx context.Context, task, contextFile, primer string) (*interaction.Analysis, error) {
	var readTools []domainAgent.Tool
	for _, t := range e.tools {
		name := t.Name()
		if name != "write_file" && name != "run_cmd" {
			readTools = append(readTools, t)
		}
	}

	systemPrompt := fmt.Sprintf(`<project_context>
%s
</project_context>

<role>
You are the CodePicker Analyst. Your goal is to diagnose the issue described in the TASK.
</role>

<constraints>
- You have READ-ONLY access.
- Locate the specific lines of code that need changing.
- Provide a clear, technical explanation of the bug and the required fix as your Final Answer.
</constraints>`, primer)

	bus := event.NewDataBus()
	defer bus.Close()

	analyst := NewReActAgent(e.model, readTools, bus, e.logger, e.policy, e.costTracker, e.rateLimiter, e.budgetGuard.Remaining(), 100)
	analyst.UpdateSystemPrompt(systemPrompt)

	input := fmt.Sprintf("<task>\n%s\n</task>\n\n<initial_focus_file>\n%s\n</initial_focus_file>", task, contextFile)

	fmt.Println("🔍 [PHASE 1] Analyst is diagnosing issue...")
	summary, err := analyst.Run(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("analyst phase failed: %w", err)
	}

	return &interaction.Analysis{
		Markdown: summary,
		Files:    []string{contextFile},
	}, nil
}

func (e *TwoPassEngine) GeneratePatch(ctx context.Context, task string, analysis *interaction.Analysis) (*interaction.Patch, error) {
	basePrompt := `<role>
You are the CodePicker Engineer. Write SEARCH/REPLACE blocks to fix the issue.
</role>

<rules>
1. Output ONLY SEARCH/REPLACE blocks. Do not explain your changes.
2. The SEARCH block MUST match the existing file exactly, including whitespace and indentation.
3. You may use multiple blocks for multiple changes.
</rules>

<format>
### relative/path/to/file.go
<<<<
exact original code to be replaced
====
new replacement code
>>>>
</format>`

	systemPrompt := basePrompt
	if e.PackedContext != "" {
		systemPrompt = fmt.Sprintf("%s\n\n<project_structure>\n%s\n</project_structure>", basePrompt, e.PackedContext)
	}

	messages := []llm.Message{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: fmt.Sprintf("<task>\n%s\n</task>\n\n<analysis>\n%s\n</analysis>", task, analysis.Markdown)},
	}

	estCost := 0.005
	if err := e.budgetGuard.Reserve(estCost); err != nil {
		return nil, fmt.Errorf("patch generation halted by budget: %w", err)
	}
	defer e.budgetGuard.Commit(estCost)

	fmt.Println("📝 [PHASE 2] Engineer is generating patch...")
	resp, _, err := e.model.ChatNative(ctx, messages, nil)
	if err != nil {
		return nil, err
	}

	return &interaction.Patch{
		Diff: cleanPatch(resp.Content),
	}, nil
}

func (e *TwoPassEngine) RefinePatch(ctx context.Context, task string, analysis *interaction.Analysis, originalDiff string, feedback string) (*interaction.Patch, error) {
	systemPrompt := `<role>
You are the CodePicker Repair Engineer.
</role>

<objective>
The previous SEARCH/REPLACE block failed to apply. Correct it based on the error feedback.
</objective>

<rules>
1. Ensure your SEARCH block matches the file exactly.
2. Output ONLY the raw SEARCH/REPLACE block. No conversational filler.
</rules>`

	userPrompt := fmt.Sprintf("<task>\n%s FAILED\n</task>\n\n<failed_block>\n%s\n</failed_block>\n\n<error>\n%s\n</error>", task, originalDiff, feedback)

	messages := []llm.Message{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: userPrompt},
	}

	estCost := 0.003
	if err := e.budgetGuard.Reserve(estCost); err != nil {
		return nil, fmt.Errorf("patch refinement halted by budget: %w", err)
	}
	defer e.budgetGuard.Commit(estCost)

	fmt.Println("🩹 [PHASE 5] Refining patch based on feedback...")
	resp, _, err := e.model.ChatNative(ctx, messages, nil)
	if err != nil {
		return nil, err
	}

	return &interaction.Patch{
		Diff: cleanPatch(resp.Content),
	}, nil
}

func cleanPatch(raw string) string {
	raw = strings.TrimSpace(raw)
	// Strip markdown formatting if the LLM wraps the blocks
	if strings.HasPrefix(raw, "```") {
		lines := strings.Split(raw, "\n")
		if len(lines) > 2 {
			raw = strings.Join(lines[1:len(lines)-1], "\n")
		}
	}
	return strings.TrimSpace(raw)
}