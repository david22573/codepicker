package openrouter

import "encoding/json"

// -----------------------------------------------------------------------------
// Models API (GET /models)
// -----------------------------------------------------------------------------

// ListModelsResponse represents the response from the /models endpoint.
type ListModelsResponse struct {
	Data []Model `json:"data"`
}

// Model represents a single model available on OpenRouter.
type Model struct {
	// The unique identifier for the model (e.g., "anthropic/claude-3-opus").
	ID string `json:"id"`

	// The human-readable name of the model.
	Name string `json:"name"`

	// A description of the model's capabilities.
	Description string `json:"description"`

	// The maximum context length (tokens) supported by the model.
	ContextLength int `json:"context_length"`

	// The architecture of the model.
	Architecture ModelArchitecture `json:"architecture"`

	// Pricing information for the model.
	Pricing ModelPricing `json:"pricing"`

	// The primary provider for this model in OpenRouter's routing.
	TopProvider ProviderInfo `json:"top_provider"`
}

// ModelArchitecture describes the technical details of the model.
type ModelArchitecture struct {
	// The type of tokenizer used (e.g., "cl100k_base").
	Tokenizer string `json:"tokenizer"`

	// The instruction format (e.g., "alpaca", "llama-2").
	InstructType string `json:"instruct_type,omitempty"`

	// The modality of the model (e.g., "text->text", "text+image->text").
	Modality string `json:"modality"`
}

// ModelPricing defines the cost structure for the model.
// Costs are typically representing in USD per token/image.
type ModelPricing struct {
	// Cost per input token (string to preserve precision).
	Prompt string `json:"prompt"`

	// Cost per output token (string to preserve precision).
	Completion string `json:"completion"`

	// Cost per image (if applicable).
	Image string `json:"image"`

	// Cost per request (if applicable).
	Request string `json:"request"`
}

// ProviderInfo contains details about the model provider.
type ProviderInfo struct {
	Name string `json:"name"`
}

// -----------------------------------------------------------------------------
// Chat Completions API (POST /chat/completions)
// -----------------------------------------------------------------------------

// ChatCompletionRequest represents a request to the chat completions endpoint.
// It includes standard OpenAI parameters and OpenRouter-specific extensions.
type ChatCompletionRequest struct {
	// Standard OpenAI Parameters
	Messages          []ChatMessage   `json:"messages"`
	Model             string          `json:"model"` // Primary model ID
	Stream            bool            `json:"stream,omitempty"`
	Temperature       *float32        `json:"temperature,omitempty"`
	TopP              *float32        `json:"top_p,omitempty"`
	TopK              *int            `json:"top_k,omitempty"`
	FrequencyPenalty  *float32        `json:"frequency_penalty,omitempty"`
	PresencePenalty   *float32        `json:"presence_penalty,omitempty"`
	RepetitionPenalty *float32        `json:"repetition_penalty,omitempty"`
	MinP              *float32        `json:"min_p,omitempty"`
	TopA              *float32        `json:"top_a,omitempty"`
	Seed              *int            `json:"seed,omitempty"`
	MaxTokens         int             `json:"max_tokens,omitempty"`
	LogitBias         map[string]int  `json:"logit_bias,omitempty"`
	Stop              []string        `json:"stop,omitempty"`
	Tools             []Tool          `json:"tools,omitempty"`
	ToolChoice        interface{}     `json:"tool_choice,omitempty"` // "none", "auto", or specific tool struct
	ResponseFormat    *ResponseFormat `json:"response_format,omitempty"`

	// OpenRouter Specific Parameters

	// List of model IDs to fallback to if the primary model fails.
	Models []string `json:"models,omitempty"`

	// Routing preference (e.g., "fallback").
	Route string `json:"route,omitempty"`

	// Provider preferences for routing.
	Provider *ProviderPreferences `json:"provider,omitempty"`

	// List of transforms to apply (e.g., ["middle-out"]).
	Transforms []string `json:"transforms,omitempty"`
}

// ChatMessage represents a single message in the conversation history.
type ChatMessage struct {
	Role       string      `json:"role"`
	Content    interface{} `json:"content"` // Can be string or []ContentPart
	Name       string      `json:"name,omitempty"`
	ToolCalls  []ToolCall  `json:"tool_calls,omitempty"`
	ToolCallID string      `json:"tool_call_id,omitempty"` // For role: tool
}

// ContentPart represents a part of a multimodal message (text or image).
type ContentPart struct {
	Type     string    `json:"type"`
	Text     string    `json:"text,omitempty"`
	ImageURL *ImageURL `json:"image_url,omitempty"`
}

type ImageURL struct {
	URL    string `json:"url"`
	Detail string `json:"detail,omitempty"` // "auto", "low", "high"
}

// ProviderPreferences defines how OpenRouter should select providers.
type ProviderPreferences struct {
	// Whether to allow OpenAI to run this request if the primary provider fails.
	AllowFallbacks *bool `json:"allow_fallbacks,omitempty"`

	// Enforce specific providers.
	Order []string `json:"order,omitempty"`

	// Filter providers by data collection policy.
	DataCollection string `json:"data_collection,omitempty"` // "deny" or "allow"

	// Require providers to support specific parameters.
	RequireParameters []string `json:"require_parameters,omitempty"`
}

// ResponseFormat specifies the output format (e.g., JSON mode).
type ResponseFormat struct {
	Type string `json:"type"` // e.g., "json_object"
}

// Tool represents a function or capability available to the model.
type Tool struct {
	Type     string       `json:"type"` // Currently only "function"
	Function ToolFunction `json:"function"`
}

type ToolFunction struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters"` // JSON Schema object
}

// ToolCall represents a model's request to call a tool.
type ToolCall struct {
	ID       string           `json:"id"`
	Type     string           `json:"type"`
	Function ToolCallFunction `json:"function"`
}

type ToolCallFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"` // JSON string of arguments
}

// -----------------------------------------------------------------------------
// Chat Responses
// -----------------------------------------------------------------------------

// ChatCompletionResponse represents the API response.
type ChatCompletionResponse struct {
	ID                string   `json:"id"`
	Object            string   `json:"object"`
	Created           int64    `json:"created"`
	Model             string   `json:"model"` // The actual model used
	Choices           []Choice `json:"choices"`
	Usage             *Usage   `json:"usage,omitempty"`
	SystemFingerprint string   `json:"system_fingerprint,omitempty"`
	Provider          string   `json:"provider,omitempty"` // OpenRouter provider used
}

// Choice represents a single completion choice.
type Choice struct {
	Index        int          `json:"index"`
	Message      *ChatMessage `json:"message,omitempty"` // Present in non-stream
	Delta        *ChatMessage `json:"delta,omitempty"`   // Present in stream
	FinishReason string       `json:"finish_reason"`     // stop, length, tool_calls, content_filter
}

// Usage provides token counts and cost information.
type Usage struct {
	PromptTokens     int     `json:"prompt_tokens"`
	CompletionTokens int     `json:"completion_tokens"`
	TotalTokens      int     `json:"total_tokens"`
	TotalCost        float64 `json:"total_cost,omitempty"` // OpenRouter specific: cost in USD
}

// ErrorResponse represents an API error.
type ErrorResponse struct {
	Error ErrorDetails `json:"error"`
}

type ErrorDetails struct {
	Message  string                 `json:"message"`
	Type     string                 `json:"type"`
	Param    interface{}            `json:"param,omitempty"`
	Code     interface{}            `json:"code,omitempty"`
	Metadata map[string]interface{} `json:"metadata,omitempty"` // OpenRouter specific metadata
}
