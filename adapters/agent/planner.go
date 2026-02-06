package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/david22573/codepicker/domain/agent"
	"github.com/david22573/codepicker/domain/errors"
	"github.com/david22573/codepicker/domain/task"
)

type Planner struct {
	model agent.LLMClient
	repo  agent.Repository
}

func NewPlanner(model agent.LLMClient, repo agent.Repository) *Planner {
	return &Planner{
		model: model,
		repo:  repo,
	}
}

// CreatePlan generates a new plan based on the task and file context
// FIX: Updated signature to accept 'fileContext string' instead of '*domain.LLMContext'
func (p *Planner) CreatePlan(ctx context.Context, taskInput, fileContext string) (*task.Plan, error) {
	systemPrompt := `You are the Lead Architect.
Break down the task into sequential steps.
STRICTLY only reference files that exist in the PROJECT CONTEXT.

OUTPUT FORMAT:
Return ONLY raw JSON. Do not include markdown formatting.

EXAMPLE:
{
  "reasoning": "I need to read X to understand Y...",
  "steps": [
    {
      "id": 1,
      "description": "Analyze main.go",
      "instruction": "Read main.go and check imports",
      "files": ["main.go"]
    }
  ],
  "estimated_cost": 0.1
}`

	// Combine the markdown context and the user task into a single string prompt
	userMessage := fmt.Sprintf("PROJECT CONTEXT:\n%s\n\nTASK: %s", fileContext, taskInput)

	resp, err := p.model.Chat(ctx, systemPrompt, userMessage)
	if err != nil {
		return nil, errors.NewLLM("planner.CreatePlan", err)
	}

	cleanResp := strings.TrimSpace(resp)
	cleanResp = strings.TrimPrefix(cleanResp, "```json")
	cleanResp = strings.TrimPrefix(cleanResp, "```")
	cleanResp = strings.TrimSuffix(cleanResp, "```")

	var planData struct {
		Reasoning string      `json:"reasoning"`
		Steps     []task.Step `json:"steps"`
	}

	if err := json.Unmarshal([]byte(cleanResp), &planData); err != nil {
		return nil, errors.NewSystem("planner.CreatePlan", "failed to parse plan JSON: "+cleanResp, err)
	}

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

// OptimizePlan uses AI to refine an existing plan based on feedback
func (p *Planner) OptimizePlan(ctx context.Context, plan *task.Plan, feedback string) (*task.Plan, error) {
	systemPrompt := `You are the Lead Architect. Refine the plan based on feedback.
OUTPUT FORMAT: Return ONLY raw JSON matching the original structure.`

	planBytes, _ := json.Marshal(plan)

	userMessage := fmt.Sprintf("CURRENT PLAN:\n%s\n\nUSER FEEDBACK: %s\n\nRefine the plan.", string(planBytes), feedback)

	resp, err := p.model.Chat(ctx, systemPrompt, userMessage)
	if err != nil {
		return nil, errors.NewLLM("planner.OptimizePlan", err)
	}

	cleanResp := strings.TrimSpace(resp)
	cleanResp = strings.TrimPrefix(cleanResp, "```json")
	cleanResp = strings.TrimPrefix(cleanResp, "```")
	cleanResp = strings.TrimSuffix(cleanResp, "```")

	var planData struct {
		Reasoning string      `json:"reasoning"`
		Steps     []task.Step `json:"steps"`
	}

	if err := json.Unmarshal([]byte(cleanResp), &planData); err != nil {
		return nil, errors.NewSystem("planner.OptimizePlan", "failed to parse optimized plan", err)
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
