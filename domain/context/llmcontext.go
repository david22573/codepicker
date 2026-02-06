package context

import (
	"time"
)

type MessageRole string

const (
	RoleSystem    MessageRole = "system"
	RoleUser      MessageRole = "user"
	RoleAssistant MessageRole = "assistant"
	RoleTool      MessageRole = "tool"
)

type LLMMessage struct {
	Role       MessageRole    `json:"role"`
	Content    string         `json:"content"`
	Name       string         `json:"name,omitempty"`
	ToolCallID string         `json:"tool_call_id,omitempty"`
	Metadata   map[string]any `json:"metadata,omitempty"`
	Timestamp  time.Time      `json:"timestamp"`
}

type TokenUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

type LLMContext struct {
	ID         string         `json:"id"`
	Messages   []LLMMessage   `json:"messages"`
	Usage      TokenUsage     `json:"usage"`
	CreatedAt  time.Time      `json:"created_at"`
	UpdatedAt  time.Time      `json:"updated_at"`
	Properties map[string]any `json:"properties"`
}

func NewLLMContext(id string) *LLMContext {
	now := time.Now()
	return &LLMContext{
		ID:         id,
		Messages:   make([]LLMMessage, 0),
		Properties: make(map[string]any),
		CreatedAt:  now,
		UpdatedAt:  now,
	}
}

func (c *LLMContext) AddMessage(role MessageRole, content string) {
	c.Messages = append(c.Messages, LLMMessage{
		Role:      role,
		Content:   content,
		Timestamp: time.Now(),
	})
	c.UpdatedAt = time.Now()
}

func (c *LLMContext) TotalTokens() int {
	return c.Usage.TotalTokens
}
