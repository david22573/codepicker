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
}

func NewPlanner(model agent.LLMClient) *Planner {
	return &Planner{model: model}
}

func (p *Planner) CreatePlan(ctx context.Context, taskInput string) (*task.Plan, error) {
	systemPrompt := `You are the Lead Architect for a software project.
Your goal is to break down a complex coding task into a series of strictly sequential, atomic steps.

GUIDELINES:
1. Each step must be self-contained.
2. Explicitly list the files involved in each step.
3. The "Instruction" must be a direct command to a coding agent (e.g., "Read file X and update function Y...").
4. Do not perform the work. Plan the work.

OUTPUT FORMAT:
Return ONLY valid JSON matching this structure:
{
  "reasoning": "High-level thought process...",
  "steps": [
    {
      "id": 1,
      "description": "Short summary",
      "instruction": "Detailed prompt for the worker",
      "files": ["main.go", "utils.go"]
    }
  ],
  "estimated_cost": 0.0
}`

	resp, err := p.model.Chat(ctx, systemPrompt, fmt.Sprintf("TASK: %s", taskInput))
	if err != nil {
		return nil, errors.NewLLM("planner.CreatePlan", err)
	}

	// Clean up potential markdown formatting from LLM
	cleanResp := strings.Trim(resp, "`")
	if strings.HasPrefix(cleanResp, "json") {
		cleanResp = strings.TrimPrefix(cleanResp, "json")
	}

	var planData struct {
		Reasoning string      `json:"reasoning"`
		Steps     []task.Step `json:"steps"`
	}

	if err := json.Unmarshal([]byte(cleanResp), &planData); err != nil {
		return nil, errors.NewSystem("planner.CreatePlan", "failed to parse plan JSON", err)
	}

	// Hydrate the Domain Entity
	planID := fmt.Sprintf("plan-%d", time.Now().Unix())
	domainPlan := task.NewPlan(planID, taskInput, planData.Reasoning)
	domainPlan.Steps = planData.Steps

	// Ensure statuses are initialized
	for i := range domainPlan.Steps {
		domainPlan.Steps[i].Status = task.StatusPending
	}

	return domainPlan, nil
}
