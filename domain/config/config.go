package config

import (
	"os"

	"gopkg.in/yaml.v3"
)

type LLMConfig struct {
	Model            string  `json:"model" yaml:"model"`
	DeprecatedAPIKey string  `json:"api_key,omitempty" yaml:"api_key,omitempty"` // Deprecated for security
	TimeoutSeconds   int     `json:"timeout_seconds" yaml:"timeout_seconds"`
	MaxTokens        int     `json:"max_tokens" yaml:"max_tokens"`
	Temperature      float64 `json:"temperature" yaml:"temperature"`
	InputCostPer1M   float64 `json:"input_cost_per_1m" yaml:"input_cost_per_1m"`
	OutputCostPer1M  float64 `json:"output_cost_per_1m" yaml:"output_cost_per_1m"`
	BudgetCap        float64 `json:"budget_cap" yaml:"budget_cap"`
}

type EmbeddingConfig struct {
	Model     string  `json:"model" yaml:"model"`
	BatchSize int     `json:"batch_size" yaml:"batch_size"`
	CostPer1M float64 `json:"cost_per_1m" yaml:"cost_per_1m"`
}

type AgentConfig struct {
	MaxTurns       int  `json:"max_turns" yaml:"max_turns"`
	UseReflexion   bool `json:"use_reflexion" yaml:"use_reflexion"`
	MaxContextSize int  `json:"max_context_size" yaml:"max_context_size"`
}

type ServerConfig struct {
	Port        int  `json:"port" yaml:"port"`
	MetricsPort int  `json:"metrics_port" yaml:"metrics_port"`
	EnablePprof bool `json:"enable_pprof" yaml:"enable_pprof"`
}

type VerifyConfig struct {
	Commands   []string `json:"commands" yaml:"commands"`
	FailClosed bool     `json:"fail_closed" yaml:"fail_closed"`
}

type AppConfig struct {
	Environment string          `json:"environment" yaml:"environment"`
	LLM         LLMConfig       `json:"llm" yaml:"llm"`
	Embedding   EmbeddingConfig `json:"embedding" yaml:"embedding"`
	Agent       AgentConfig     `json:"agent" yaml:"agent"`
	Server      ServerConfig    `json:"server" yaml:"server"`
	Verify      VerifyConfig    `json:"verify" yaml:"verify"`
}

// DefaultConfig provides safe values if config files are missing.
func DefaultConfig() *AppConfig {
	return &AppConfig{
		Environment: "development",
		LLM: LLMConfig{
			Model:           "deepseek/deepseek-v3.2",
			TimeoutSeconds:  3000,
			MaxTokens:       8000,
			Temperature:     0.0,
			InputCostPer1M:  0.14,
			OutputCostPer1M: 0.28,
			BudgetCap:       10.00,
		},
		Embedding: EmbeddingConfig{
			Model:     "text-embedding-3-small",
			BatchSize: 10,
			CostPer1M: 0.02,
		},
		Agent: AgentConfig{
			MaxTurns:       2000,
			UseReflexion:   true,
			MaxContextSize: 200000,
		},
		Server: ServerConfig{
			Port:        8080,
			MetricsPort: 9090,
			EnablePprof: false,
		},
		Verify: VerifyConfig{
			Commands:   []string{},
			FailClosed: true,
		},
	}
}

// LoadConfig loads the configuration from a YAML file.
func LoadConfig(path string) (*AppConfig, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return DefaultConfig(), nil
	}
	if err != nil {
		return nil, err
	}

	cfg := DefaultConfig()
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}
