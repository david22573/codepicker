package openrouter

import "encoding/json"

type ListModelsResponse struct {
	Data []Model `json:"data"`
}

type Model struct {
	ID            string            `json:"id"`
	Name          string            `json:"name"`
	Description   string            `json:"description"`
	ContextLength int               `json:"context_length"`
	Architecture  ModelArchitecture `json:"architecture"`
	Pricing       ModelPricing      `json:"pricing"`
	TopProvider   ProviderInfo      `json:"top_provider"`
}

type ModelArchitecture struct {
	Tokenizer    string `json:"tokenizer"`
	InstructType string `json:"instruct_type,omitempty"`
	Modality     string `json:"modality"`
}

type ModelPricing struct {
	Prompt     string `json:"prompt"`
	Completion string `json:"completion"`
	Image      string `json:"image"`
	Request    string `json:"request"`
}

type ProviderInfo struct {
	Name string `json:"name"`
}

type ChatCompletionRequest struct {
	Messages          []ChatMessage        `json:"messages"`
	Model             string               `json:"model"` // Primary model ID
	Stream            bool                 `json:"stream,omitempty"`
	Temperature       *float32             `json:"temperature,omitempty"`
	TopP              *float32             `json:"top_p,omitempty"`
	TopK              *int                 `json:"top_k,omitempty"`
	FrequencyPenalty  *float32             `json:"frequency_penalty,omitempty"`
	PresencePenalty   *float32             `json:"presence_penalty,omitempty"`
	RepetitionPenalty *float32             `json:"repetition_penalty,omitempty"`
	MinP              *float32             `json:"min_p,omitempty"`
	TopA              *float32             `json:"top_a,omitempty"`
	Seed              *int                 `json:"seed,omitempty"`
	MaxTokens         int                  `json:"max_tokens,omitempty"`
	LogitBias         map[string]int       `json:"logit_bias,omitempty"`
	Stop              []string             `json:"stop,omitempty"`
	Tools             []Tool               `json:"tools,omitempty"`
	ToolChoice        interface{}          `json:"tool_choice,omitempty"`
	ResponseFormat    *ResponseFormat      `json:"response_format,omitempty"`
	Models            []string             `json:"models,omitempty"`
	Route             string               `json:"route,omitempty"`
	Provider          *ProviderPreferences `json:"provider,omitempty"`
	Transforms        []string             `json:"transforms,omitempty"`
}

type ChatMessage struct {
	Role       string      `json:"role"`
	Content    interface{} `json:"content"`
	Name       string      `json:"name,omitempty"`
	ToolCalls  []ToolCall  `json:"tool_calls,omitempty"`
	ToolCallID string      `json:"tool_call_id,omitempty"`
}

type ContentPart struct {
	Type     string    `json:"type"`
	Text     string    `json:"text,omitempty"`
	ImageURL *ImageURL `json:"image_url,omitempty"`
}

type ImageURL struct {
	URL    string `json:"url"`
	Detail string `json:"detail,omitempty"`
}

type ProviderPreferences struct {
	AllowFallbacks    *bool    `json:"allow_fallbacks,omitempty"`
	Order             []string `json:"order,omitempty"`
	DataCollection    string   `json:"data_collection,omitempty"`
	RequireParameters []string `json:"require_parameters,omitempty"`
}

type ResponseFormat struct {
	Type string `json:"type"`
}

type Tool struct {
	Type     string       `json:"type"`
	Function ToolFunction `json:"function"`
}

type ToolFunction struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters"`
}

type ToolCall struct {
	Index    int              `json:"index"` // Added this field
	ID       string           `json:"id"`
	Type     string           `json:"type"`
	Function ToolCallFunction `json:"function"`
}

type ToolCallFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type ChatCompletionResponse struct {
	ID                string   `json:"id"`
	Object            string   `json:"object"`
	Created           int64    `json:"created"`
	Model             string   `json:"model"`
	Choices           []Choice `json:"choices"`
	Usage             *Usage   `json:"usage,omitempty"`
	SystemFingerprint string   `json:"system_fingerprint,omitempty"`
	Provider          string   `json:"provider,omitempty"`
}

type Choice struct {
	Index        int          `json:"index"`
	Message      *ChatMessage `json:"message,omitempty"`
	Delta        *ChatMessage `json:"delta,omitempty"`
	FinishReason string       `json:"finish_reason"`
}

type Usage struct {
	PromptTokens     int     `json:"prompt_tokens"`
	CompletionTokens int     `json:"completion_tokens"`
	TotalTokens      int     `json:"total_tokens"`
	TotalCost        float64 `json:"total_cost,omitempty"`
}

type ErrorResponse struct {
	Error ErrorDetails `json:"error"`
}

type ErrorDetails struct {
	Message  string                 `json:"message"`
	Type     string                 `json:"type"`
	Param    interface{}            `json:"param,omitempty"`
	Code     interface{}            `json:"code,omitempty"`
	Metadata map[string]interface{} `json:"metadata,omitempty"`
}
