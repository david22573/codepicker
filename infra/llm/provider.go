package llm

import (
	"context"

	domainContext "github.com/david22573/codepicker/domain/context"
)

// Provider defines the interface for an LLM backend, decoupled from OpenRouter.
type Provider interface {
	ChatNative(ctx context.Context, messages []Message, tools []ToolDefinition) (Message, domainContext.TokenUsage, error)
	Chat(ctx context.Context, systemPrompt, userPrompt string) (string, error)
}

// CacheControl defines caching behavior for messages.
type CacheControl struct {
	Type string `json:"type"`
}

// --- Native Tool Structures ---
type ToolDefinition struct {
	Type     string             `json:"type"`
	Function FunctionDefinition `json:"function"`
}

type FunctionDefinition struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Parameters  any    `json:"parameters"`
}

type ToolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

type Message struct {
	Role         string        `json:"role"`
	Content      string        `json:"content,omitempty"`
	ToolCalls    []ToolCall    `json:"tool_calls,omitempty"`
	ToolCallID   string        `json:"tool_call_id,omitempty"`
	Name         string        `json:"name,omitempty"`
	CacheControl *CacheControl `json:"cache_control,omitempty"`
}
