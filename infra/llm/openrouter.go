package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	domainContext "github.com/david22573/codepicker/domain/context"
	"github.com/david22573/codepicker/domain/errors"
)

const openRouterURL = "https://openrouter.ai/api/v1/chat/completions"

// --- Request/Response Structures ---
type chatRequest struct {
	Model    string           `json:"model"`
	Messages []Message        `json:"messages"`
	Tools    []ToolDefinition `json:"tools,omitempty"`
}

type chatResponse struct {
	Choices []struct {
		Message Message `json:"message"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

type OpenRouterAdapter struct {
	apiKey         string
	model          string
	client         *http.Client
	circuitBreaker *CircuitBreaker
}

func NewOpenRouterAdapter(apiKey, model string, timeout time.Duration) *OpenRouterAdapter {
	return &OpenRouterAdapter{
		apiKey:         apiKey,
		model:          model,
		client:         &http.Client{Timeout: timeout},
		circuitBreaker: NewCircuitBreaker(5, 60*time.Second),
	}
}

// ChatNative implements structured tool calling.
func (o *OpenRouterAdapter) ChatNative(ctx context.Context, messages []Message, tools []ToolDefinition) (Message, domainContext.TokenUsage, error) {
	reqBody := chatRequest{
		Model:    o.model,
		Messages: messages,
		Tools:    tools,
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return Message{}, domainContext.TokenUsage{}, errors.NewSystem("llm.ChatNative", "failed to marshal request", err)
	}

	var responseMessage Message
	var usage domainContext.TokenUsage

	opErr := o.circuitBreaker.Execute(func() error {
		req, err := http.NewRequestWithContext(ctx, "POST", openRouterURL, bytes.NewBuffer(jsonBody))
		if err != nil {
			return err
		}

		req.Header.Set("Authorization", "Bearer "+o.apiKey)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("HTTP-Referer", "https://github.com/david22573/codepicker")

		resp, err := o.client.Do(req)
		if err != nil {
			return errors.NewLLM("llm.ChatNative", err)
		}
		defer resp.Body.Close()

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return err
		}

		if resp.StatusCode != http.StatusOK {
			return errors.NewLLM("llm.ChatNative", fmt.Errorf("API error (%d): %s", resp.StatusCode, string(body)))
		}

		var chatResp chatResponse
		if err := json.Unmarshal(body, &chatResp); err != nil {
			return err
		}

		if chatResp.Error != nil {
			return errors.NewLLM("llm.ChatNative", fmt.Errorf("api error: %s", chatResp.Error.Message))
		}

		if len(chatResp.Choices) > 0 {
			responseMessage = chatResp.Choices[0].Message
			usage = domainContext.TokenUsage{
				PromptTokens:     chatResp.Usage.PromptTokens,
				CompletionTokens: chatResp.Usage.CompletionTokens,
				TotalTokens:      chatResp.Usage.TotalTokens,
			}
		}

		return nil
	})

	return responseMessage, usage, opErr
}

// Chat maintains backward compatibility for simple system/user prompts.
func (o *OpenRouterAdapter) Chat(ctx context.Context, systemPrompt, userPrompt string) (string, error) {
	messages := []Message{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: userPrompt},
	}
	msg, _, err := o.ChatNative(ctx, messages, nil)
	return msg.Content, err
}