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

type Orchestrator struct {
	Self       *Engine
	Team       map[AgentType]*Engine
	Refinement *RefinementSystem
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
		// [Fixed] Pass empty DebugConfig
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

func (o *Orchestrator) RunTask(ctx context.Context, userTask string) error {
	optimizedTask, err := o.Refinement.OptimizePrompt(ctx, userTask)
	if err != nil {
		o.Self.Logger.Warn("Proposer failed (using original): " + err.Error())
		optimizedTask = userTask
	}
	o.Self.Logger.Info("🎼 Orchestrator planning task: " + optimizedTask)

	plan, err := o.createPlan(ctx, optimizedTask)
	if err != nil {
		return fmt.Errorf("planning failed: %w", err)
	}

	o.Self.Logger.Info(fmt.Sprintf("📋 Plan generated with %d steps", len(plan.Steps)))

	for i, step := range plan.Steps {
		worker := o.Team[step.Agent]
		if worker == nil {
			worker = o.Team[AgentModifier]
		}
		o.Self.Logger.Info(fmt.Sprintf("\n▶️  Step %d [%s]: %s", i+1, step.Agent, step.Task))
		result, err := worker.Run(ctx, step.Task, nil)
		if err != nil {
			return fmt.Errorf("step execution failed: %w", err)
		}
		o.Self.Logger.Info(fmt.Sprintf("✅ Result: %s", truncate(result, 100)))
		o.Self.Memory.AddNote(fmt.Sprintf("Observation from %s: %s", step.Agent, result))
	}
	return nil
}

func (o *Orchestrator) createPlan(ctx context.Context, task string) (*ExecutionPlan, error) {
	prompt := fmt.Sprintf(`TASK: %s
Available Agents: Context, CodeModifier, System, Quality.
Return JSON: {"steps": [{"agent": "Context", "task": "Search..."}]}`, task)

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
