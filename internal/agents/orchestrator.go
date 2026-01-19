package agents

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/david22573/codepicker/internal/agent" // Legacy for memory/sentinel
	"github.com/david22573/codepicker/internal/config"
	"github.com/david22573/codepicker/internal/database"
	"github.com/david22573/codepicker/internal/logger"
	"github.com/david22573/codepicker/internal/shadow"
	"github.com/david22573/codepicker/pkg/openrouter"
)

type Orchestrator struct {
	Self *BaseAgent
	Team map[AgentType]*BaseAgent
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
) (*Orchestrator, error) {

	// Shared resources for the whole team
	shadowMgr, err := shadow.NewManager(srcRoot)
	if err != nil {
		return nil, err
	}

	mem := agent.NewMemory(srcRoot, store, shadowMgr)
	limits := config.DefaultLimits()
	sentinel := agent.NewSentinel(limits)

	// Default model for everyone (can be overridden per agent if needed)
	model := "deepseek/deepseek-chat"

	// Helper to spawn agents
	spawn := func(t AgentType, prompt string) *BaseAgent {
		return NewBaseAgent(
			t, client, model, prompt,
			mem, shadowMgr, sentinel, log, limits,
		)
	}

	orch := &Orchestrator{
		Self: spawn(AgentOrchestrator, PromptOrchestrator),
		Team: make(map[AgentType]*BaseAgent),
	}

	// Initialize the squad
	orch.Team[AgentContext] = spawn(AgentContext, PromptContext)
	orch.Team[AgentModifier] = spawn(AgentModifier, PromptModifier)
	orch.Team[AgentSystem] = spawn(AgentSystem, PromptSystem)
	orch.Team[AgentQuality] = spawn(AgentQuality, PromptQuality)

	return orch, nil
}

func (o *Orchestrator) RunTask(ctx context.Context, userTask string) error {
	o.Self.Logger.Info("🎼 Orchestrator planning task: " + userTask)

	// 1. Planning Phase
	plan, err := o.createPlan(ctx, userTask)
	if err != nil {
		return fmt.Errorf("planning failed: %w", err)
	}

	o.Self.Logger.Info(fmt.Sprintf("📋 Plan generated with %d steps", len(plan.Steps)))

	// 2. Execution Phase
	for i, step := range plan.Steps {
		worker := o.Team[step.Agent]
		if worker == nil {
			o.Self.Logger.Warn(fmt.Sprintf("⚠️ Unknown agent type '%s', defaulting to CodeModifier", step.Agent))
			worker = o.Team[AgentModifier]
		}

		o.Self.Logger.Info(fmt.Sprintf("\n▶️  Step %d [%s]: %s", i+1, step.Agent, step.Task))

		result, err := worker.Execute(ctx, step.Task)
		if err != nil {
			o.Self.Logger.Error(fmt.Sprintf("❌ [%s] Failed: %v", step.Agent, err))
			// In the future, we can add auto-recovery or re-planning here
			return err
		}

		// Clean up the output for logging
		cleanResult := result
		if len(cleanResult) > 200 {
			cleanResult = cleanResult[:200] + "..."
		}
		o.Self.Logger.Info(fmt.Sprintf("✅ [%s] Result: %s", step.Agent, cleanResult))

		// Add the result to shared memory so subsequent agents know what happened
		// We format it as a system observation
		observation := fmt.Sprintf("Observation from %s: %s", step.Agent, result)
		o.Self.Memory.AddNote(observation)
	}

	o.Self.Logger.Info("\n✨ All steps completed successfully.")
	return nil
}

func (o *Orchestrator) createPlan(ctx context.Context, task string) (*ExecutionPlan, error) {
	// We ask the Orchestrator (DeepSeek) to output JSON
	prompt := fmt.Sprintf(`TASK: %s

Available Agents:
- Context: Read/Search code.
- CodeModifier: Write code to shadow files.
- System: Run shell commands/tests.
- Quality: Linting/Security checks.

Return a valid JSON object with a list of steps.
Example: {"steps": [{"agent": "Context", "task": "Search for auth logic"}, {"agent": "CodeModifier", "task": "Add JWT middleware"}]}`, task)

	// We force JSON mode by appending instructions or using response_format if supported
	resp, err := o.Self.Execute(ctx, prompt)
	if err != nil {
		return nil, err
	}

	// Attempt to clean markdown blocks if present (e.g. ```json ... ```)
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
