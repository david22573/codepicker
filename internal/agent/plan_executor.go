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

	// Update DB status
	pe.Engine.Memory.Store.UpdatePlanStatus(pe.Plan.ID, "running")

	for i := range pe.Plan.Steps {
		step := &pe.Plan.Steps[i]

		// Skip if already done (allows resuming later)
		if step.Status == "completed" {
			pe.Engine.Logger.Info(fmt.Sprintf("⏭️  Skipping completed step %d", step.ID))
			continue
		}

		pe.Engine.Logger.Info(fmt.Sprintf("\n⚡ Executing Step %d/%d: %s", step.ID, len(pe.Plan.Steps), step.Description))
		step.Status = "running"
		pe.savePlanState() // Checkpoint

		// Load relevant files into context if specified
		if len(step.Files) > 0 {
			for _, f := range step.Files {
				if err := pe.Engine.Memory.Add(f); err != nil {
					pe.Engine.Logger.Warn(fmt.Sprintf("Could not load context file %s: %v", f, err))
				}
			}
		}

		// Inject step context into the Engine's history/system prompt for this run
		// We execute the step using the main Engine.Run
		printUpdate := func(msg openrouter.ChatMessage) {
			if msg.Role == "assistant" && msg.Content != nil {
				content := fmt.Sprintf("%v", msg.Content)
				if content != "" && !strings.Contains(content, "tool_calls") {
					fmt.Printf("🤖 Step %d Thought: %s\n", step.ID, content)
				}
			}
		}

		result, err := pe.Engine.Run(ctx, step.Instruction, printUpdate)

		if err != nil {
			step.Status = "failed"
			step.Result = err.Error()
			pe.savePlanState()
			pe.Engine.Logger.Error(fmt.Sprintf("❌ Step %d Failed: %v", step.ID, err))

			if step.Critical {
				pe.Engine.Memory.Store.UpdatePlanStatus(pe.Plan.ID, "failed")
				return fmt.Errorf("critical step %d failed, aborting plan", step.ID)
			}
			continue
		}

		step.Status = "completed"
		step.Result = result
		pe.savePlanState()
		pe.Engine.Logger.Info(fmt.Sprintf("✅ Step %d Complete", step.ID))

		// Small cool-down to prevent rate limits
		time.Sleep(1 * time.Second)
	}

	pe.Engine.Memory.Store.UpdatePlanStatus(pe.Plan.ID, "completed")
	pe.Engine.Logger.Info("\n✨ Plan Execution Finished Successfully.")
	return nil
}

func (pe *PlanExecutor) savePlanState() {
	// Sync current in-memory plan state to DB
	// Note: In a real batch system we would update specific rows, but here we update the blob
	// to keep it simple for Phase 1.
	pe.Engine.Memory.Store.SavePlan(pe.Plan.ID, pe.Plan.OriginalTask, pe.Plan.Steps, pe.Plan.EstimatedCost)
}
