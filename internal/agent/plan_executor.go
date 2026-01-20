package agent

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/david22573/codepicker/pkg/openrouter"
)

type PlanExecutor struct {
	Engine *Engine
	Plan   *Plan
}

func NewPlanExecutor(eng *Engine, plan *Plan) *PlanExecutor {
	return &PlanExecutor{
		Engine: eng,
		Plan:   plan,
	}
}

func (pe *PlanExecutor) Execute(ctx context.Context) error {
	pe.Engine.Logger.Info(fmt.Sprintf("🚀 Starting Plan Execution: %s (%d steps)", pe.Plan.ID, len(pe.Plan.Steps)))

	pe.Engine.Memory.Store.UpdatePlanStatus(pe.Plan.ID, "running")

	// Load retry limit from config, default to 2 if 0
	maxRetries := pe.Engine.Limits.MaxRecoveryAttempts
	if maxRetries <= 0 {
		maxRetries = 2
	}

	for i := range pe.Plan.Steps {
		step := &pe.Plan.Steps[i]

		if step.Status == "completed" {
			pe.Engine.Logger.Info(fmt.Sprintf("⏭️  Skipping completed step %d", step.ID))
			continue
		}

		pe.Engine.Logger.Info(fmt.Sprintf("\n⚡ Executing Step %d/%d: %s", step.ID, len(pe.Plan.Steps), step.Description))
		step.Status = "running"
		pe.savePlanState()

		if len(step.Files) > 0 {
			for _, f := range step.Files {
				if err := pe.Engine.Memory.Add(f); err != nil {
					pe.Engine.Logger.Warn(fmt.Sprintf("Could not load context file %s: %v", f, err))
				}
			}
		}

		printUpdate := func(msg openrouter.ChatMessage) {
			if msg.Role == "assistant" && msg.Content != nil {
				content := fmt.Sprintf("%v", msg.Content)
				if content != "" && !strings.Contains(content, "tool_calls") {
					fmt.Printf("🤖 Step %d Thought: %s\n", step.ID, content)
				}
			}
		}

		var result string
		var err error

		// --- RETRY LOOP ---
		for attempt := 0; attempt <= maxRetries; attempt++ {
			if attempt > 0 {
				pe.Engine.Logger.Warn(fmt.Sprintf("🔄 Retry attempt %d/%d for Step %d...", attempt, maxRetries, step.ID))
				// Simple backoff
				time.Sleep(time.Duration(attempt) * 2 * time.Second)
			}

			// Add "Retry Context" to instruction if this is a retry
			instruction := step.Instruction
			if attempt > 0 && err != nil {
				instruction = fmt.Sprintf("%s\n\n(NOTE: Previous attempt failed with error: %v. Please fix and retry.)", instruction, err)
			}

			result, err = pe.Engine.Run(ctx, instruction, printUpdate)
			if err == nil {
				break // Success!
			}

			pe.Engine.Logger.Warn(fmt.Sprintf("Step %d execution error: %v", step.ID, err))
		}
		// ------------------

		if err != nil {
			step.Status = "failed"
			step.Result = err.Error()
			pe.savePlanState()
			pe.Engine.Logger.Error(fmt.Sprintf("❌ Step %d Failed after %d attempts: %v", step.ID, maxRetries+1, err))

			if step.Critical {
				pe.Engine.Memory.Store.UpdatePlanStatus(pe.Plan.ID, "failed")
				return fmt.Errorf("critical step %d failed, aborting plan", step.ID)
			}
			// Allow non-critical steps to fail without stopping the whole plan
			pe.Engine.Logger.Warn("⚠️  Non-critical step failed. Continuing plan...")
			continue
		}

		step.Status = "completed"
		step.Result = result
		pe.savePlanState()
		pe.Engine.Logger.Info(fmt.Sprintf("✅ Step %d Complete", step.ID))

		time.Sleep(1 * time.Second)
	}

	pe.Engine.Memory.Store.UpdatePlanStatus(pe.Plan.ID, "completed")
	pe.Engine.Logger.Info("\n✨ Plan Execution Finished Successfully.")
	return nil
}

func (pe *PlanExecutor) savePlanState() {
	pe.Engine.Memory.Store.SavePlan(pe.Plan.ID, pe.Plan.OriginalTask, pe.Plan.Steps, pe.Plan.EstimatedCost)
}
