package agent

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/david22573/codepicker/internal/shadow"
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
	// Phase 3: Plan Validation
	if len(pe.Plan.Steps) == 0 {
		return fmt.Errorf("invalid plan: no steps to execute")
	}

	pe.Engine.Logger.Info(fmt.Sprintf("🚀 Starting Plan Execution: %s (%d steps)", pe.Plan.ID, len(pe.Plan.Steps)))

	pe.Engine.Memory.Store.UpdatePlanStatus(pe.Plan.ID, "running")

	var shadowMgr *shadow.Manager
	if overlay, ok := pe.Engine.Memory.FS.(interface{ GetShadowManager() *shadow.Manager }); ok {
		shadowMgr = overlay.GetShadowManager()
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

		// Load context files for this step
		if len(step.Files) > 0 {
			for _, f := range step.Files {
				if err := pe.Engine.Memory.Add(f); err != nil {
					pe.Engine.Logger.Warn(fmt.Sprintf("Could not load context file %s: %v", f, err))
				}
			}
		}

		// Phase 3: Smart Retry Logic
		// We retry up to 2 times (Total 3 attempts).
		// If it fails, we feed the error back into the prompt.
		maxRetries := 2
		var result string
		var err error
		startTime := time.Now()

		for attempt := 0; attempt <= maxRetries; attempt++ {
			instruction := step.Instruction

			if attempt > 0 {
				pe.Engine.Logger.Warn(fmt.Sprintf("🔄 Retry attempt %d/%d...", attempt, maxRetries))

				// Check if any shadow files were written despite the failure
				var partialProgress []string
				if shadowMgr != nil {
					partialProgress = pe.checkProgress(shadowMgr, startTime)
				}

				if len(partialProgress) > 0 {
					pe.Engine.Logger.Info(fmt.Sprintf("🧠 Detected partial progress on %d files. Adjusting prompt.", len(partialProgress)))
					instruction += fmt.Sprintf(
						"\n\n[SYSTEM NOTICE]: The previous attempt timed out or failed, BUT you successfully wrote these files to shadow: %s.\n"+
							"DO NOT rewrite them unless necessary. Pick up exactly where you left off and complete the remaining work.",
						strings.Join(partialProgress, ", "),
					)
				} else {
					instruction += fmt.Sprintf("\n\n[SYSTEM NOTICE]: Previous attempt failed with error: %v. Please try again. Review your logic and syntax.", err)
				}

				// Backoff
				time.Sleep(2 * time.Second)
			}

			result, err = pe.Engine.Run(ctx, instruction, pe.makePrintCallback(step.ID))
			if err == nil {
				break
			}

			if strings.Contains(err.Error(), "deadline exceeded") || strings.Contains(err.Error(), "timeout") {
				pe.Engine.Logger.Warn("⏳ Step timed out.")
			} else {
				pe.Engine.Logger.Warn(fmt.Sprintf("Step %d execution error: %v", step.ID, err))
			}
		}

		if err != nil {
			pe.handleStepFailure(step, err)
			return fmt.Errorf("step %d failed after retries: %w", step.ID, err)
		}

		step.Status = "completed"
		step.Result = result
		pe.savePlanState()
		pe.Engine.Logger.Info(fmt.Sprintf("✅ Step %d Complete", step.ID))

		startTime = time.Now() // Reset timer for next step
		time.Sleep(1 * time.Second)
	}

	pe.Engine.Memory.Store.UpdatePlanStatus(pe.Plan.ID, "completed")
	pe.Engine.Logger.Info("\n✨ Plan Execution Finished Successfully.")
	return nil
}

func (pe *PlanExecutor) checkProgress(sm *shadow.Manager, since time.Time) []string {
	var recentFiles []string
	sm.LoadManifest() // Reload from disk to see latest changes

	for file, meta := range sm.Manifest.Changes {
		if meta.Timestamp.After(since) {
			recentFiles = append(recentFiles, file)
		}
	}
	return recentFiles
}

func (pe *PlanExecutor) makePrintCallback(stepID int) func(openrouter.ChatMessage) {
	return func(msg openrouter.ChatMessage) {
		if msg.Role == "assistant" && msg.Content != nil {
			content := fmt.Sprintf("%v", msg.Content)
			if content != "" && !strings.Contains(content, "tool_calls") {
				// Only print if we are debugging or it's a significant thought
				pe.Engine.Logger.Debug(fmt.Sprintf("%d: %s", stepID, content))
				// For CLI feedback, we might just print a dot or spinner update
			}
		}
	}
}

func (pe *PlanExecutor) handleStepFailure(step *Step, err error) {
	step.Status = "failed"
	step.Result = err.Error()
	pe.savePlanState()
	pe.Engine.Logger.Error(fmt.Sprintf("❌ Step %d Failed: %v", step.ID, err))
	pe.Engine.Memory.Store.UpdatePlanStatus(pe.Plan.ID, "failed")
}

func (pe *PlanExecutor) savePlanState() {
	pe.Engine.Memory.Store.SavePlan(pe.Plan.ID, pe.Plan.OriginalTask, pe.Plan.Steps, pe.Plan.EstimatedCost)
}
