package config

import (
	"encoding/json"
	"os"
)

type LLMConfig struct {
	Model           string  `json:"model" yaml:"model"`
	TimeoutSeconds  int     `json:"timeout_seconds" yaml:"timeout_seconds"`
	MaxTokens       int     `json:"max_tokens" yaml:"max_tokens"`
	Temperature     float64 `json:"temperature" yaml:"temperature"`
	InputCostPer1M  float64 `json:"input_cost_per_1m" yaml:"input_cost_per_1m"`
	OutputCostPer1M float64 `json:"output_cost_per_1m" yaml:"output_cost_per_1m"`
	BudgetCap       float64 `json:"budget_cap" yaml:"budget_cap"` // Max $ per session
}

type AgentConfig struct {
	MaxTurns       int  `json:"max_turns" yaml:"max_turns"`
	UseReflexion   bool `json:"use_reflexion" yaml:"use_reflexion"`
	MaxContextSize int  `json:"max_context_size" yaml:"max_context_size"` // Tokens
}

type ServerConfig struct {
	Port        int  `json:"port" yaml:"port"`
	MetricsPort int  `json:"metrics_port" yaml:"metrics_port"`
	EnablePprof bool `json:"enable_pprof" yaml:"enable_pprof"`
}

type AppConfig struct {
	Environment string       `json:"environment" yaml:"environment"`
	LLM         LLMConfig    `json:"llm" yaml:"llm"`
	Agent       AgentConfig  `json:"agent" yaml:"agent"`
	Server      ServerConfig `json:"server" yaml:"server"`
}

// DefaultConfig provides safe values if config files are missing.
func DefaultConfig() *AppConfig {
	return &AppConfig{
		Environment: "development",
		LLM: LLMConfig{
			Model:           "moonshotai/kimi-k2.5",
			TimeoutSeconds:  120,
			MaxTokens:       16000,
			Temperature:     0.1,
			InputCostPer1M:  0.30,
			OutputCostPer1M: 0.60,
			BudgetCap:       2.00,
		},
		Agent: AgentConfig{
			MaxTurns:       20,
			UseReflexion:   true,
			MaxContextSize: 128000,
		},
		Server: ServerConfig{
			Port:        8080,
			MetricsPort: 9090,
			EnablePprof: false,
		},
	}
}

// LoadConfig reads from a JSON file (Legacy support).
// The new YAML loader in infra/config should be preferred.
func LoadConfig(path string) (*AppConfig, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return DefaultConfig(), nil
	}
	if err != nil {
		return nil, err
	}

	cfg := DefaultConfig()
	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}
