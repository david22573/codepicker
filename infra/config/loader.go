package config

import (
	"fmt"
	"os"
	"strconv"

	"github.com/david22573/codepicker/domain/config"
	"gopkg.in/yaml.v3"
)

// Loader handles configuration resolution from files and environment variables.
type Loader struct {
	configPath string
}

func NewLoader(path string) *Loader {
	return &Loader{
		configPath: path,
	}
}

// Load reads the YAML file and applies environment overrides.
func (l *Loader) Load() (*config.AppConfig, error) {
	// 1. Start with defaults
	cfg := config.DefaultConfig()

	// 2. Read YAML file if it exists
	if l.configPath != "" {
		data, err := os.ReadFile(l.configPath)
		if err != nil && !os.IsNotExist(err) {
			return nil, fmt.Errorf("failed to read config file: %w", err)
		}

		if err == nil {
			if err := yaml.Unmarshal(data, cfg); err != nil {
				return nil, fmt.Errorf("failed to parse config file: %w", err)
			}
			fmt.Printf("📄 [CONFIG] Loaded configuration from %s\n", l.configPath)
		} else {
			fmt.Println("⚠️ [CONFIG] No config file found, using defaults.")
		}
	}

	// 3. Apply Environment Variable Overrides
	l.applyEnvOverrides(cfg)

	return cfg, nil
}

// applyEnvOverrides allows 12-factor app configuration via env vars.
// Prefix: CODEPICKER_
func (l *Loader) applyEnvOverrides(cfg *config.AppConfig) {
	// Core
	if v := os.Getenv("CODEPICKER_ENV"); v != "" {
		cfg.Environment = v
	}

	// LLM
	if v := os.Getenv("CODEPICKER_LLM_MODEL"); v != "" {
		cfg.LLM.Model = v
	}
	if v := os.Getenv("CODEPICKER_LLM_KEY"); v != "" {
		// API Key is usually handled separately, but could be config if needed.
		// Kept separate in main.go for security best practices.
	}
	if v := os.Getenv("CODEPICKER_LLM_TIMEOUT"); v != "" {
		if i, err := strconv.Atoi(v); err == nil {
			cfg.LLM.TimeoutSeconds = i
		}
	}
	if v := os.Getenv("CODEPICKER_LLM_BUDGET"); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			cfg.LLM.BudgetCap = f
		}
	}

	// Agent
	if v := os.Getenv("CODEPICKER_AGENT_MAX_TURNS"); v != "" {
		if i, err := strconv.Atoi(v); err == nil {
			cfg.Agent.MaxTurns = i
		}
	}

	// Server
	if v := os.Getenv("CODEPICKER_METRICS_PORT"); v != "" {
		if i, err := strconv.Atoi(v); err == nil {
			cfg.Server.MetricsPort = i
		}
	}
}
