package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/david22573/codepicker/internal/code"
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

func (p *Planner) CreatePlan(ctx context.Context, task string, projectTree string) (*Plan, error) {
	p.Logger.Info("🧠 Thinking about plan...")

	userMsg := fmt.Sprintf("TASK: %s\n\nPROJECT STRUCTURE:\n%s", task, projectTree)

	req := openrouter.ChatCompletionRequest{
		Model: p.Model,
		Messages: []openrouter.ChatMessage{
			{Role: "system", Content: prompts.Planner},
			{Role: "user", Content: userMsg},
		},
		ResponseFormat: &openrouter.ResponseFormat{Type: "json_object"},
		// Prompt engineering: Force JSON mode with a prefill
		Prefill: "{\n  \"reasoning\": \"",
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

func (p *Planner) GenerateContextSummary(root string, files []string) string {
	var sb strings.Builder
	sb.WriteString("### SELECTED FILE SKELETONS:\n")

	for _, path := range files {
		fullPath := filepath.Join(root, path)
		content, err := os.ReadFile(fullPath)
		if err != nil {
			continue
		}

		if strings.HasSuffix(path, ".go") {
			// Fix: Added 'true' (keepDocs) so the planner can see function comments
			// This matches the updated signature: func Skeletonize(filename string, src []byte, keepDocs bool)
			skel, err := code.Skeletonize(path, content, true)
			if err == nil {
				sb.WriteString(fmt.Sprintf("\n--- FILE: %s ---\n%s\n", path, string(skel)))
				continue
			}
		}

		lines := strings.Split(string(content), "\n")
		if len(lines) > 10 {
			lines = lines[:10]
		}
		sb.WriteString(fmt.Sprintf("\n--- FILE: %s (Head) ---\n%s\n", path, strings.Join(lines, "\n")))
	}

	return sb.String()
}
