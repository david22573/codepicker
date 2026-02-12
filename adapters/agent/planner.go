package agent

import (
	"context"
	"fmt"
	"time"

	"github.com/david22573/codepicker/domain/agent"
	"github.com/david22573/codepicker/domain/task"
	"github.com/david22573/codepicker/infra/llm"
)

type Planner struct {
	model llm.StructuredLLM // UPDATED: Depends on the structured interface
	repo  agent.Repository
}

// NewPlanner now wraps the raw client in the StructuredAdapter internally.
func NewPlanner(model agent.LLMClient, repo agent.Repository) *Planner {
	return &Planner{
		model: llm.NewStructuredAdapter(model), // Inject wrapper
		repo:  repo,
	}
}

// planSchema defines the expected structure for unmarshaling.
type planSchema struct {
	Reasoning string      `json:"reasoning"`
	Steps     []task.Step `json:"steps"`
}

// CreatePlan generates a new plan based on the task and file context.
func (p *Planner) CreatePlan(ctx context.Context, taskInput, fileContext, primer string) (*task.Plan, error) {
	systemPrompt := `You are the Lead Architect.
Break down the task into sequential steps.
STRICTLY only reference files that exist in the PROJECT CONTEXT.

OUTPUT FORMAT:
Return valid JSON matching this structure:
{
  "reasoning": "string",
  "steps": [
    {
      "id": 1,
      "description": "string",
      "instruction": "string",
      "files": ["string"]
    }
  ]
}`

	userMessage := fmt.Sprintf("PROJECT STARTER INFO:\n%s\n\nRELEVANT CODE SNIPPETS:\n%s\n\nTASK: %s", primer, fileContext, taskInput)

	// UPDATED: Use ChatJSON for type safety and auto-repair
	var planData planSchema
	if err := p.model.ChatJSON(ctx, systemPrompt, userMessage, &planData); err != nil {
		return nil, err
	}

	// Domain mapping
	planID := fmt.Sprintf("plan-%d", time.Now().UnixNano())
	domainPlan := task.NewPlan(planID, taskInput, planData.Reasoning)
	domainPlan.Steps = planData.Steps

	for i := range domainPlan.Steps {
		domainPlan.Steps[i].Status = task.StatusPending
	}

	if err := p.repo.SavePlan(ctx, domainPlan); err != nil {
		return nil, fmt.Errorf("failed to persist plan: %w", err)
	}

	return domainPlan, nil
}

// OptimizePlan uses AI to refine an existing plan based on feedback.
func (p *Planner) OptimizePlan(ctx context.Context, plan *task.Plan, feedback string) (*task.Plan, error) {
	systemPrompt := `You are the Lead Architect.
Refine the plan based on feedback.
OUTPUT FORMAT: Return valid JSON matching the original plan structure.`

	// We only need the steps and reasoning for context
	currentContext := struct {
		Reasoning string      `json:"reasoning"`
		Steps     []task.Step `json:"steps"`
	}{
		Reasoning: plan.Reasoning,
		Steps:     plan.Steps,
	}

	userMessage := fmt.Sprintf("CURRENT PLAN:\n%+v\n\nUSER FEEDBACK: %s\n\nRefine the plan.", currentContext, feedback)

	var planData planSchema
	if err := p.model.ChatJSON(ctx, systemPrompt, userMessage, &planData); err != nil {
		return nil, err
	}

	plan.Reasoning = planData.Reasoning
	plan.Steps = planData.Steps
	plan.Status = task.StatusPending
	for i := range plan.Steps {
		plan.Steps[i].Status = task.StatusPending
	}

	if err := p.repo.SavePlan(ctx, plan); err != nil {
		return nil, err
	}

	return plan, nil
}
