package config

import (
	"fmt"
	"os"
	"path/filepath"
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

	// 2. Resolve Config Path (Walk up if necessary)
	resolvedPath := l.findConfigPath(l.configPath)

	// 3. Read YAML file if it exists
	if resolvedPath != "" {
		data, err := os.ReadFile(resolvedPath)
		if err != nil {
			return nil, fmt.Errorf("failed to read config file: %w", err)
		}

		if err := yaml.Unmarshal(data, cfg); err != nil {
			return nil, fmt.Errorf("failed to parse config file: %w", err)
		}

		if cfg.LLM.DeprecatedAPIKey != "" {
			fmt.Println("⚠️  [SECURITY WARNING] 'api_key' found in codepicker.yaml. Please remove it and use the OPENROUTER_API_KEY environment variable instead.")
			cfg.LLM.DeprecatedAPIKey = "" // Wipe from memory
		}

		fmt.Printf("📄 [CONFIG] Loaded configuration from %s\n", resolvedPath)
	} else {
		// Only warn if the user explicitly provided a path that wasn't found
		if l.configPath != "" && l.configPath != "codepicker.yaml" {
			fmt.Printf("⚠️ [CONFIG] Config file not found at %s, using defaults.\n", l.configPath)
		}
	}

	// 4. Apply Environment Variable Overrides
	l.applyEnvOverrides(cfg)

	return cfg, nil
}

// findConfigPath attempts to locate the config file.
// If the path is relative, it searches up the directory tree.
func (l *Loader) findConfigPath(path string) string {
	if path == "" {
		return ""
	}

	// If absolute, check it directly
	if filepath.IsAbs(path) {
		if _, err := os.Stat(path); err == nil {
			return path
		}
		return ""
	}

	// If relative, start at CWD and walk up
	cwd, err := os.Getwd()
	if err != nil {
		return ""
	}

	currentDir := cwd
	for {
		target := filepath.Join(currentDir, path)
		if _, err := os.Stat(target); err == nil {
			return target
		}

		parent := filepath.Dir(currentDir)
		if parent == currentDir {
			break // Reached root
		}
		currentDir = parent
	}

	return ""
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
