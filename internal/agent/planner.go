package agent

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/david22573/codepicker/internal/logger"
	"github.com/david22573/codepicker/internal/prompts"
	"github.com/david22573/codepicker/pkg/openrouter"
	"github.com/google/uuid"
)

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
			{Role: "system", Content: prompts.Planner},
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
