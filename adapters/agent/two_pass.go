package agent

import (
	"context"
	"fmt"
	"strings"

	"github.com/david22573/codepicker/domain/agent"
	"github.com/david22573/codepicker/domain/interaction"
	"github.com/david22573/codepicker/infra/llm"
	"github.com/david22573/codepicker/infra/logging"
	"github.com/david22573/codepicker/infra/ratelimit"
)

type TwoPassEngine struct {
	model         *llm.OpenRouterAdapter
	repo          agent.Repository
	tools         []agent.Tool
	policy        agent.Policy
	logger        *logging.Logger
	costTracker   *llm.CostTracker
	budgetGuard   *llm.BudgetGuard // Added Guard
	rateLimiter   *ratelimit.ToolRateLimiter
	PackedContext string
}

func NewTwoPassEngine(
	model *llm.OpenRouterAdapter,
	repo agent.Repository,
	tools []agent.Tool,
	policy agent.Policy,
	logger *logging.Logger,
	costTracker *llm.CostTracker,
	rateLimiter *ratelimit.ToolRateLimiter,
	budget float64, // Added Budget Cap
	packedContext string,
) *TwoPassEngine {
	// Create a guard sharing the global cost tracker
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

// RunAnalysis performs Phase 1: The Analyst (Read-Only)
func (e *TwoPassEngine) RunAnalysis(ctx context.Context, task, contextFile, primer string) (*interaction.Analysis, error) {
	// Filter tools for read-only safety
	var readTools []agent.Tool
	for _, t := range e.tools {
		name := t.Name()
		if name != "write_file" && name != "run_cmd" {
			readTools = append(readTools, t)
		}
	}

	systemPrompt := fmt.Sprintf(`%s

You are the CodePicker Analyst.
Your goal is to diagnose the issue described in the TASK.
You have READ-ONLY access.
Locate the specific lines of code that need changing.
Provide a clear, technical explanation of the bug and the required fix as your Final Answer.`, primer)

	// Analyst uses ReActAgent, which handles its own budget checks internally
	// We pass the guard's remaining limit or the global cap.
	analyst := NewReActAgent(e.model, readTools, nil, e.logger, e.policy, e.costTracker, e.rateLimiter, e.budgetGuard.Remaining(), 100)
	analyst.UpdateSystemPrompt(systemPrompt)

	input := fmt.Sprintf("TASK: %s\nInitial focus file: %s", task, contextFile)

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

// GeneratePatch performs Phase 2: The Engineer
func (e *TwoPassEngine) GeneratePatch(ctx context.Context, task string, analysis *interaction.Analysis) (*interaction.Patch, error) {
	basePrompt := `You are the CodePicker Engineer.
Write a Git Unified Diff to fix the issue.
RULES:
1. Output ONLY raw diff content.
2. Context lines must match the original file EXACTLY (including whitespace).
3. If you are unsure about exact context matching, use larger context blocks.`

	systemPrompt := basePrompt
	if e.PackedContext != "" {
		systemPrompt = fmt.Sprintf("%s\nPROJECT_STRUCTURE:\n%s", basePrompt, e.PackedContext)
	}

	messages := []llm.Message{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: fmt.Sprintf("TASK: %s\nANALYSIS:\n%s", task, analysis.Markdown)},
	}

	// --- BUDGET PROTECTION ---
	// Patch generation is token-heavy, estimate $0.005
	estCost := 0.005
	if err := e.budgetGuard.Reserve(estCost); err != nil {
		return nil, fmt.Errorf("patch generation halted by budget: %w", err)
	}
	defer e.budgetGuard.Commit(estCost)
	// -------------------------

	fmt.Println("📝 [PHASE 2] Engineer is generating patch...")
	resp, _, err := e.model.ChatNative(ctx, messages, nil)
	if err != nil {
		return nil, err
	}

	return &interaction.Patch{
		Diff: cleanPatch(resp.Content),
	}, nil
}

// RefinePatch performs Phase 5: Self-Correction
func (e *TwoPassEngine) RefinePatch(ctx context.Context, task string, analysis *interaction.Analysis, originalDiff string, feedback string) (*interaction.Patch, error) {
	systemPrompt := `You are the CodePicker Repair Engineer.
The previous patch failed. Correct the Git Unified Diff based on the error feedback.
Output ONLY the raw diff.`

	userPrompt := fmt.Sprintf("TASK: %s FAILED\nPATCH: %s\nERROR: %s", task, originalDiff, feedback)

	messages := []llm.Message{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: userPrompt},
	}

	// --- BUDGET PROTECTION ---
	estCost := 0.003
	if err := e.budgetGuard.Reserve(estCost); err != nil {
		return nil, fmt.Errorf("patch refinement halted by budget: %w", err)
	}
	defer e.budgetGuard.Commit(estCost)
	// -------------------------

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
	raw = strings.TrimPrefix(raw, "```diff")
	raw = strings.TrimPrefix(raw, "```")
	raw = strings.TrimSuffix(raw, "```")
	return strings.TrimSpace(raw)
}
