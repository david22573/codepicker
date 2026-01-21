package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/david22573/codepicker/internal/config"
	"github.com/david22573/codepicker/internal/database"
	"github.com/david22573/codepicker/internal/logger"
	"github.com/david22573/codepicker/internal/policy"
	"github.com/david22573/codepicker/internal/prompts"
	"github.com/david22573/codepicker/pkg/openrouter"
)

type AgentType string

const (
	AgentOrchestrator AgentType = "Orchestrator"
	AgentContext      AgentType = "Context"
	AgentModifier     AgentType = "CodeModifier"
	AgentSystem       AgentType = "System"
	AgentQuality      AgentType = "Quality"
)

// ObserverFunc is a callback for UI updates
type ObserverFunc func(eventType, content string)

type Orchestrator struct {
	Self       *Engine
	Team       map[AgentType]*Engine
	Refinement *RefinementSystem

	PlanReviewHandler func(*ExecutionPlan) bool
	StepErrorHandler  func(step PlanStep, err error, analysis string) string

	// NEW: Hook for the UI
	Observer ObserverFunc
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

	spawn := func(role AgentType, rolePolicy policy.ExecutionPolicy, prompt string) (*Engine, error) {
		model := cfg.GetModel()
		eng, err := NewEngine(client, model, srcRoot, log, config.DefaultLimits(), store, cfg, DebugConfig{})
		if err != nil {
			return nil, err
		}
		eng.SetPolicy(rolePolicy)
		eng.SystemPrompt = prompt
		return eng, nil
	}

	boss, err := spawn(AgentOrchestrator, policy.Batch, prompts.Orchestrator)
	if err != nil {
		return nil, err
	}

	team := make(map[AgentType]*Engine)

	if ctxAgent, err := spawn(AgentContext, policy.Architect, prompts.ContextSpecialist); err == nil {
		team[AgentContext] = ctxAgent
	}
	if modAgent, err := spawn(AgentModifier, policy.Batch, prompts.CodeModifier); err == nil {
		team[AgentModifier] = modAgent
	}
	if sysAgent, err := spawn(AgentSystem, policy.Interactive, prompts.SystemAgent); err == nil {
		team[AgentSystem] = sysAgent
	}
	if qualAgent, err := spawn(AgentQuality, policy.Interactive, prompts.QualityAgent); err == nil {
		team[AgentQuality] = qualAgent
	}

	return &Orchestrator{
		Self:       boss,
		Team:       team,
		Refinement: NewRefinementSystem(client, cfg.GetModel(), log),
	}, nil
}

func (o *Orchestrator) notify(eventType, content string) {
	if o.Observer != nil {
		o.Observer(eventType, content)
	}
}

func (o *Orchestrator) RunTask(ctx context.Context, userTask string) error {
	optimizedTask, err := o.Refinement.OptimizePrompt(ctx, userTask)
	if err != nil {
		optimizedTask = userTask
	}

	o.notify("thought", "Planning: "+optimizedTask)

	plan, err := o.createPlan(ctx, optimizedTask)
	if err != nil {
		return fmt.Errorf("planning failed: %w", err)
	}

	if o.PlanReviewHandler != nil {
		if approved := o.PlanReviewHandler(plan); !approved {
			return fmt.Errorf("plan rejected by user")
		}
	}

	for i, step := range plan.Steps {
		worker := o.Team[step.Agent]
		if worker == nil {
			worker = o.Team[AgentModifier]
		}

		o.notify("step", fmt.Sprintf("[%d/%d] %s: %s", i+1, len(plan.Steps), step.Agent, step.Task))

		for {
			var recentThoughts []string

			// Capture history logic (modified to use Observer)
			captureHistory := func(msg openrouter.ChatMessage) {
				if msg.Role == "assistant" && msg.Content != nil {
					content := fmt.Sprintf("%v", msg.Content)
					if content != "" && !strings.Contains(content, "tool_calls") {
						recentThoughts = append(recentThoughts, content)
						o.notify("thought", content)
					}
				}
				if len(msg.ToolCalls) > 0 {
					for _, tool := range msg.ToolCalls {
						o.notify("tool_start", tool.Function.Name)
					}
				}
				if msg.Role == "tool" {
					o.notify("tool_end", "Completed")
				}
			}

			result, err := worker.Run(ctx, step.Task, captureHistory)

			if err == nil {
				o.Self.Memory.AddNote(fmt.Sprintf("Observation from %s: %s", step.Agent, result))
				break
			}

			// Error handling logic...
			if o.StepErrorHandler != nil {
				analysis := "No analysis available."
				action := o.StepErrorHandler(step, err, analysis)

				if action == "retry" {
					o.notify("thought", "Retrying step...")
					continue
				} else if action == "skip" {
					o.notify("thought", "Skipping step.")
					break
				} else {
					return err
				}
			} else {
				return err
			}
		}
	}
	return nil
}

func (o *Orchestrator) createPlan(ctx context.Context, task string) (*ExecutionPlan, error) {
	prompt := fmt.Sprintf(`TASK: %s
Available Agents: Context, CodeModifier, System, Quality.
Return JSON ONLY: {"steps": [{"agent": "Context", "task": "Search..."}]}`, task)

	resp, err := o.Self.Run(ctx, prompt, nil)
	if err != nil {
		return nil, err
	}

	cleaned := stripMarkdown(resp)
	var plan ExecutionPlan
	if err := json.Unmarshal([]byte(cleaned), &plan); err != nil {
		return nil, fmt.Errorf("invalid plan format: %w", err)
	}
	return &plan, nil
}

func truncate(s string, n int) string {
	if len(s) > n {
		return s[:n] + "..."
	}
	return s
}

func stripMarkdown(content string) string {
	content = strings.TrimSpace(content)
	if strings.HasPrefix(content, "```") {
		lines := strings.Split(content, "\n")
		if len(lines) >= 2 {
			lines = lines[1:]
			if len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "```" {
				lines = lines[:len(lines)-1]
			}
			return strings.Join(lines, "\n")
		}
	}
	return content
}
