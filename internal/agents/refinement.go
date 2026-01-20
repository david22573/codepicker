// File: internal/agents/refinement.go

package agents

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/david22573/codepicker/internal/logger"
	"github.com/david22573/codepicker/pkg/openrouter"
)

type JudgeResult struct {
	Pass     bool   `json:"pass"`
	Score    int    `json:"score"`
	Feedback string `json:"feedback"`
}

type RefinementSystem struct {
	Client *openrouter.Client
	Model  string
	Logger logger.Logger
}

func NewRefinementSystem(client *openrouter.Client, model string, log logger.Logger) *RefinementSystem {
	return &RefinementSystem{
		Client: client,
		Model:  model,
		Logger: log,
	}
}

// OptimizePrompt takes a vague user task and turns it into a technical spec
func (r *RefinementSystem) OptimizePrompt(ctx context.Context, originalTask string) (string, error) {
	r.Logger.Info("✨ Proposer is optimizing your request...")

	messages := []openrouter.ChatMessage{
		{Role: "system", Content: PromptProposer},
		{Role: "user", Content: fmt.Sprintf("User Request: %s\n\nGenerate the Optimized Prompt:", originalTask)},
	}

	req := openrouter.ChatCompletionRequest{
		Model:    r.Model,
		Messages: messages,
	}

	resp, err := r.Client.CreateChatCompletion(ctx, req)
	if err != nil {
		return originalTask, err // Fallback to original on error
	}

	if len(resp.Choices) == 0 || resp.Choices[0].Message == nil {
		return originalTask, fmt.Errorf("empty response from proposer")
	}

	optimized := fmt.Sprintf("%v", resp.Choices[0].Message.Content)
	r.Logger.Info(fmt.Sprintf("📝 Optimized Task: %s", optimized))
	return optimized, nil
}

// EvaluateWork reviews the outcome of a task
func (r *RefinementSystem) EvaluateWork(ctx context.Context, task, agentOutput, diffContext string) (*JudgeResult, error) {
	r.Logger.Info("⚖️  Judge is evaluating the work...")

	userContent := fmt.Sprintf(`TASK: %s

AGENT OUTPUT:
%s

CODE CHANGES (Diff):
%s

Verdict (JSON):`, task, agentOutput, diffContext)

	messages := []openrouter.ChatMessage{
		{Role: "system", Content: PromptJudge},
		{Role: "user", Content: userContent},
	}

	req := openrouter.ChatCompletionRequest{
		Model:          r.Model,
		Messages:       messages,
		ResponseFormat: &openrouter.ResponseFormat{Type: "json_object"},
	}

	resp, err := r.Client.CreateChatCompletion(ctx, req)
	if err != nil {
		return nil, err
	}

	content := fmt.Sprintf("%v", resp.Choices[0].Message.Content)

	// Strip markdown code blocks if present
	content = strings.TrimSpace(content)
	if strings.HasPrefix(content, "```") {
		lines := strings.Split(content, "\n")
		if len(lines) >= 2 {
			content = strings.Join(lines[1:len(lines)-1], "\n")
		}
	}

	var result JudgeResult
	if err := json.Unmarshal([]byte(content), &result); err != nil {
		r.Logger.Warn("Judge output invalid JSON: " + content)
		return nil, err
	}

	return &result, nil
}
