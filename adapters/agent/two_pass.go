package agent

import (
	"context"
	"fmt"
	"strings"

	"github.com/david22573/codepicker/domain/agent"
	"github.com/david22573/codepicker/domain/interaction"
	"github.com/david22573/codepicker/infra/llm"
	"github.com/david22573/codepicker/infra/logging"
)

type TwoPassEngine struct {
	model       agent.LLMClient
	repo        agent.Repository
	tools       []agent.Tool
	policy      agent.Policy
	logger      *logging.Logger
	costTracker *llm.CostTracker
}

func NewTwoPassEngine(
	model agent.LLMClient,
	repo agent.Repository,
	tools []agent.Tool,
	policy agent.Policy,
	logger *logging.Logger,
	costTracker *llm.CostTracker,
) *TwoPassEngine {
	return &TwoPassEngine{
		model:       model,
		repo:        repo,
		tools:       tools,
		policy:      policy,
		logger:      logger,
		costTracker: costTracker,
	}
}

// RunAnalysis performs Phase 2.1: The Analyst (Read-Only)
// UPDATED: Now accepts 'primer string'
func (e *TwoPassEngine) RunAnalysis(ctx context.Context, task, contextFile, primer string) (*interaction.Analysis, error) {
	readMap := make(map[string]agent.Tool)
	var toolDescs strings.Builder

	for _, t := range e.tools {
		name := t.Name()
		if name != "write_file" && name != "run_cmd" {
			readMap[name] = t
			toolDescs.WriteString(fmt.Sprintf("- %s: %s\n", name, t.Description()))
		}
	}

	// Inject Primer into System Prompt
	systemPrompt := fmt.Sprintf(`%s

You are the CodePicker Analyst.
Your goal is to diagnose the issue described in the TASK.
You have READ-ONLY access.
Locate the specific lines of code that need changing.

AVAILABLE TOOLS:
%s

FORMAT:
Thought: ...
Action: ...
Input: ...

When you have found the issue, output your findings as the Final Answer.`, primer, toolDescs.String())

	analyst := &ReActAgent{
		model:  e.model,
		tools:  readMap,
		sysMsg: systemPrompt,
		logger: e.logger, // FIX: Injected Logger
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
