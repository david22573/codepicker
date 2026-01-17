package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/david22573/codepicker/internal/logger"
	"github.com/david22573/codepicker/pkg/openrouter"
	"github.com/google/uuid"
)

const SystemPlannerPrompt = `You are a Senior Technical Project Manager and Architect.
Your goal is to break down a complex coding task into smaller, sequential, executable steps for a junior developer agent.

RULES:
1. Each step must be concrete and actionable.
2. Steps should be sequential (Step 1 must be done before Step 2).
3. If the user asks for a simple task, provide a 1-step plan.
4. "Instruction" is what will be fed to the coding agent. It must be explicit.
5. Identify specific files involved in each step if possible.

Output JSON ONLY using this schema:
{
  "reasoning": "Brief explanation of the approach",
  "estimated_cost": 0.05,
  "steps": [
    {
      "id": 1,
      "description": "Create the interface",
      "instruction": "Create file internal/interfaces.go with the User interface...",
      "critical": true,
      "files": ["internal/interfaces.go"]
    }
  ]
}`

type Planner struct {
	Client *openrouter.Client
	Model  string
	Logger logger.Logger
}

func NewPlanner(client *openrouter.Client, model string, log logger.Logger) *Planner {
	return &Planner{
		Client: client,
		Model:  model,
		Logger: log,
	}
}

func (p *Planner) CreatePlan(ctx context.Context, task string, projectContext string) (*Plan, error) {
	p.Logger.Info("🧠 Thinking about plan...")

	userMsg := fmt.Sprintf("TASK: %s\n\nPROJECT STRUCTURE:\n%s", task, projectContext)

	req := openrouter.ChatCompletionRequest{
		Model: p.Model,
		Messages: []openrouter.ChatMessage{
			{Role: "system", Content: SystemPlannerPrompt},
			{Role: "user", Content: userMsg},
		},
		ResponseFormat: &openrouter.ResponseFormat{Type: "json_object"},
	}

	resp, err := p.Client.CreateChatCompletion(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("planning request failed: %w", err)
	}

	if len(resp.Choices) == 0 || resp.Choices[0].Message == nil {
		return nil, fmt.Errorf("received empty response from AI")
	}

	content := fmt.Sprintf("%v", resp.Choices[0].Message.Content)
	content = stripMarkdown(content)

	var aiResp AIPlanResponse
	if err := json.Unmarshal([]byte(content), &aiResp); err != nil {
		p.Logger.Error(fmt.Sprintf("Raw AI Plan response: %s", content))
		return nil, fmt.Errorf("failed to parse plan JSON: %w", err)
	}

	// Sanitize and default status
	for i := range aiResp.Steps {
		aiResp.Steps[i].Status = "pending"
		if aiResp.Steps[i].ID == 0 {
			aiResp.Steps[i].ID = i + 1
		}
	}

	plan := &Plan{
		ID:            uuid.New().String(),
		OriginalTask:  task,
		Steps:         aiResp.Steps,
		EstimatedCost: aiResp.CostEst,
		Reasoning:     aiResp.Reasoning,
	}

	return plan, nil
}

func stripMarkdown(content string) string {
	content = strings.TrimSpace(content)
	if strings.HasPrefix(content, "```") {
		lines := strings.Split(content, "\n")
		if len(lines) >= 2 {
			// Remove first line (```json) and last line (```)
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
