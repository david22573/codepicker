package agents

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/david22573/codepicker/internal/agent"
	"github.com/david22573/codepicker/internal/config"
	"github.com/david22573/codepicker/internal/constants"
	"github.com/david22573/codepicker/internal/database"
	"github.com/david22573/codepicker/internal/logger"
	"github.com/david22573/codepicker/internal/shadow"
	"github.com/david22573/codepicker/internal/vfs"
	"github.com/david22573/codepicker/pkg/openrouter"
)

type Orchestrator struct {
	Self       *BaseAgent
	Team       map[AgentType]*BaseAgent
	Refinement *RefinementSystem
	Shadow     *shadow.Manager
}

type PlanStep struct {
	Agent AgentType `json:"agent"`
	Task  string    `json:"task"`
}

type ExecutionPlan struct {
	Steps []PlanStep `json:"steps"`
}

func NewOrchestrator(
	client *openrouter.Client,
	srcRoot string,
	log logger.Logger,
	store *database.Store,
	cfg *config.ConfigFile,
) (*Orchestrator, error) {

	shadowMgr, err := shadow.NewManager(srcRoot)
	if err != nil {
		return nil, err
	}

	model := constants.DefaultModel
	if cfg != nil && cfg.AI.Model != "" {
		model = cfg.AI.Model
	}

	fs := vfs.NewOverlayFS(srcRoot, shadowMgr)

	mem := agent.NewMemory(store, fs)
	limits := config.DefaultLimits()
	sentinel := agent.NewSentinel(limits)

	// Initialize the Refinement System (Proposer/Judge)
	refinement := NewRefinementSystem(client, model, log)

	spawn := func(t AgentType, prompt string) *BaseAgent {
		return NewBaseAgent(
			t, client, model, prompt,
			mem, fs, sentinel, log, limits, cfg,
		)
	}

	orch := &Orchestrator{
		Self:       spawn(AgentOrchestrator, PromptOrchestrator),
		Team:       make(map[AgentType]*BaseAgent),
		Refinement: refinement,
		Shadow:     shadowMgr,
	}

	orch.Team[AgentContext] = spawn(AgentContext, PromptContext)
	orch.Team[AgentModifier] = spawn(AgentModifier, PromptModifier)
	orch.Team[AgentSystem] = spawn(AgentSystem, PromptSystem)
	orch.Team[AgentQuality] = spawn(AgentQuality, PromptQuality)

	return orch, nil
}

func (o *Orchestrator) RunTask(ctx context.Context, userTask string) error {
	// --- PHASE 1: PROPOSER (Prompt Optimization) ---
	// We use the Proposer to refine the user's intent before planning.
	optimizedTask, err := o.Refinement.OptimizePrompt(ctx, userTask)
	if err != nil {
		o.Self.Logger.Warn("Proposer failed (using original task): " + err.Error())
		optimizedTask = userTask
	}

	o.Self.Logger.Info("🎼 Orchestrator planning task: " + optimizedTask)

	plan, err := o.createPlan(ctx, optimizedTask)
	if err != nil {
		return fmt.Errorf("planning failed: %w", err)
	}

	o.Self.Logger.Info(fmt.Sprintf("📋 Plan generated with %d steps", len(plan.Steps)))

	// --- PHASE 2: EXECUTION LOOP ---
	for i, step := range plan.Steps {
		maxRetries := 3
		var stepResult string
		var stepErr error

		// Determine the worker for this step
		worker := o.Team[step.Agent]
		if worker == nil {
			o.Self.Logger.Warn(fmt.Sprintf("⚠️ Unknown agent type '%s', defaulting to CodeModifier", step.Agent))
			worker = o.Team[AgentModifier]
		}

		// We loop here to allow the Judge to reject work and force a retry
		for attempt := 1; attempt <= maxRetries; attempt++ {
			o.Self.Logger.Info(fmt.Sprintf("\n▶️  Step %d [%s] (Attempt %d/%d): %s", i+1, step.Agent, attempt, maxRetries, step.Task))

			// Execute the Agent Task
			stepResult, stepErr = worker.Execute(ctx, step.Task)
			if stepErr != nil {
				o.Self.Logger.Error(fmt.Sprintf("❌ [%s] Execution Failed: %v", step.Agent, stepErr))
				return stepErr
			}

			// --- PHASE 3: JUDGE (Evaluation) ---
			// Only trigger the Judge if we are Modifying Code to save tokens.
			if step.Agent == AgentModifier {
				// Gather context: The actual code changes currently in the shadow dir
				files, _ := o.Shadow.ListShadowFiles()
				diffContext := ""
				for _, f := range files {
					diff, _ := o.Shadow.PreviewDiff(f)
					diffContext += diff + "\n"
				}

				// If no changes were made, the judge might be confused, but we run it anyway
				// to ensure the "Agent Output" (reasoning) is valid.
				if diffContext == "" {
					diffContext = "(No file changes detected in shadow workspace)"
				}

				judgeResult, err := o.Refinement.EvaluateWork(ctx, step.Task, stepResult, diffContext)
				if err != nil {
					o.Self.Logger.Warn("Judge failed to evaluate (skipping check): " + err.Error())
					break // If judge breaks, assume success to prevent infinite loops
				}

				if judgeResult.Pass {
					o.Self.Logger.Info(fmt.Sprintf("✅ Judge PASSED (Score: %d/10)", judgeResult.Score))
					break // Exit retry loop, proceed to next step
				} else {
					o.Self.Logger.Warn(fmt.Sprintf("❌ Judge REJECTED: %s", judgeResult.Feedback))

					// CRITICAL: Update the task with the feedback so the agent knows what to fix
					step.Task = fmt.Sprintf("Previous attempt failed.\nOriginal Task: %s\n\nJudge Feedback (YOU MUST FIX THIS): %s", step.Task, judgeResult.Feedback)

					if attempt == maxRetries {
						return fmt.Errorf("step failed after %d attempts. Final Judge feedback: %s", maxRetries, judgeResult.Feedback)
					}
					// Loop continues to next attempt...
				}
			} else {
				// For non-modifier agents (Context, System), we trust the output immediately.
				break
			}
		}

		// Clean up result for logging
		cleanResult := stepResult
		if len(cleanResult) > 200 {
			cleanResult = cleanResult[:200] + "..."
		}
		o.Self.Logger.Info(fmt.Sprintf("✅ [%s] Result: %s", step.Agent, cleanResult))

		// Store observation in Orchestrator memory for context in subsequent steps
		observation := fmt.Sprintf("Observation from %s: %s", step.Agent, stepResult)
		o.Self.Memory.AddNote(observation)
	}

	o.Self.Logger.Info("\n✨ All steps completed successfully.")
	return nil
}

func (o *Orchestrator) createPlan(ctx context.Context, task string) (*ExecutionPlan, error) {

	prompt := fmt.Sprintf(`TASK: %s

Available Agents:
- Context: Read/Search code.
- CodeModifier: Write code to shadow files.
- System: Run shell commands/tests.
- Quality: Linting/Security checks.

Return a valid JSON object with a list of steps.
Example: {"steps": [{"agent": "Context", "task": "Search for auth logic"}, {"agent": "CodeModifier", "task": "Add JWT middleware"}]}`, task)

	resp, err := o.Self.Execute(ctx, prompt)
	if err != nil {
		return nil, err
	}

	cleaned := stripMarkdown(resp)

	var plan ExecutionPlan
	if err := json.Unmarshal([]byte(cleaned), &plan); err != nil {
		o.Self.Logger.Error("Failed to parse plan JSON: " + cleaned)
		return nil, fmt.Errorf("invalid plan format: %w", err)
	}

	return &plan, nil
}

func stripMarkdown(content string) string {
	content = strings.TrimSpace(content)
	if strings.HasPrefix(content, "```") {
		lines := strings.Split(content, "\n")
		if len(lines) >= 2 {
			if strings.HasPrefix(lines[0], "```") {
				lines = lines[1:]
			}
			if len(lines) > 0 && strings.HasPrefix(lines[len(lines)-1], "```") {
				lines = lines[:len(lines)-1]
			}
			return strings.Join(lines, "\n")
		}
	}
	return content
}
