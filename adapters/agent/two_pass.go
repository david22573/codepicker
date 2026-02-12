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
	rateLimiter   *ratelimit.ToolRateLimiter
	PackedContext string // FIX: Added missing field for global codebase context
}

func NewTwoPassEngine(
	model *llm.OpenRouterAdapter,
	repo agent.Repository,
	tools []agent.Tool,
	policy agent.Policy,
	logger *logging.Logger,
	costTracker *llm.CostTracker,
	rateLimiter *ratelimit.ToolRateLimiter,
	packedContext string, // FIX: Inject packed context during initialization
) *TwoPassEngine {
	return &TwoPassEngine{
		model:         model,
		repo:          repo,
		tools:         tools,
		policy:        policy,
		logger:        logger,
		costTracker:   costTracker,
		rateLimiter:   rateLimiter,
		PackedContext: packedContext,
	}
}

// RunAnalysis performs Phase 1: The Analyst (Read-Only)
func (e *TwoPassEngine) RunAnalysis(ctx context.Context, task, contextFile, primer string) (*interaction.Analysis, error) {
	var readTools []agent.Tool
	for _, t := range e.tools {
		name := t.Name()
		if name != "write_file" && name != "run_cmd" {
			readTools = append(readTools, t)
		}
	}

	systemPrompt := fmt.Sprintf(`%s

You are the CodePicker Analyst. Your goal is to diagnose the issue described in the TASK.
You have READ-ONLY access. Locate the specific lines of code that need changing.
Provide a clear, technical explanation of the bug and the required fix as your Final Answer.`, primer)

	// Inject the refactored ReActAgent
	analyst := NewReActAgent(
		e.model,
		readTools,
		nil,
		e.logger,
		e.costTracker,
		e.rateLimiter,
		1.0,
	)

	// FIX: Use the declared systemPrompt to satisfy the compiler and set agent persona
	analyst.UpdateSystemPrompt(systemPrompt)

	input := fmt.Sprintf("TASK: %s\n\nInitial focus file: %s", task, contextFile)

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
	basePrompt := `You are the CodePicker Engineer. Write a Git Unified Diff.
RULES:
1. Output ONLY raw diff content.
2. Context lines must be exact.`

	// FIX: Correctly reference the struct field PackedContext
	systemPrompt := basePrompt
	if e.PackedContext != "" {
		systemPrompt = fmt.Sprintf("%s\n\nPROJECT_STRUCTURE:\n%s", basePrompt, e.PackedContext)
	}

	messages := []llm.Message{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: fmt.Sprintf("TASK: %s\n\nANALYSIS:\n%s", task, analysis.Markdown)},
	}

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

	userPrompt := fmt.Sprintf("TASK: %s\n\nFAILED PATCH:\n%s\n\nERROR:\n%s", task, originalDiff, feedback)

	messages := []llm.Message{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: userPrompt},
	}

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
