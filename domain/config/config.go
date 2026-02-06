package config

import (
	"encoding/json"
	"os"
)

type LLMConfig struct {
	Model           string  `json:"model"`
	TimeoutSeconds  int     `json:"timeout_seconds"`
	MaxTokens       int     `json:"max_tokens"`
	Temperature     float64 `json:"temperature"`
	InputCostPer1M  float64 `json:"input_cost_per_1m"`
	OutputCostPer1M float64 `json:"output_cost_per_1m"`
	BudgetCap       float64 `json:"budget_cap"` // Max $ per session
}

type AgentConfig struct {
	MaxTurns       int  `json:"max_turns"`
	UseReflexion   bool `json:"use_reflexion"`
	MaxContextSize int  `json:"max_context_size"` // Tokens
}

type AppConfig struct {
	LLM   LLMConfig   `json:"llm"`
	Agent AgentConfig `json:"agent"`
}

// DefaultConfig provides safe values if config.json is missing
func DefaultConfig() *AppConfig {
	return &AppConfig{
		LLM: LLMConfig{
			Model:           "moonshotai/kimi-k2.5", // Default logic/coding model
			TimeoutSeconds:  120,
			MaxTokens:       16000,
			Temperature:     0.1,
			InputCostPer1M:  0.30, // Approximate Kimi pricing
			OutputCostPer1M: 0.60,
			BudgetCap:       1.00, // Safety stop at $1.00
		},
		Agent: AgentConfig{
			MaxTurns:       15,
			UseReflexion:   true,
			MaxContextSize: 128000,
		},
	}
}

// LoadConfig reads from config.json or returns default
func LoadConfig(path string) (*AppConfig, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return DefaultConfig(), nil
	}
	if err != nil {
		return nil, err
	}

	cfg := DefaultConfig() // Start with defaults
	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}
