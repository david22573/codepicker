package config

import (
	"fmt"
	"os"
	"sync"

	"github.com/david22573/codepicker/internal/constants"
	"gopkg.in/yaml.v3"
)

type ConfigFile struct {
	Src         string       `yaml:"src"`
	Output      string       `yaml:"output"`
	Include     []string     `yaml:"include"`
	Exclude     []string     `yaml:"exclude"`
	Minify      bool         `yaml:"minify"`
	Tokens      bool         `yaml:"tokens"`
	Verbose     bool         `yaml:"verbose"`
	AI          AIConfig     `yaml:"ai"`
	Server      ServerConfig `yaml:"server"`
	CustomTools []CustomTool `yaml:"tools"` // Added Plugin Support
}

type AIConfig struct {
	Model       string  `yaml:"model"`
	WorkerModel string  `yaml:"worker_model"`
	Temperature float32 `yaml:"temperature"`
}

type ServerConfig struct {
	AllowedOrigins []string `yaml:"allowed_origins"`
	Host           string   `yaml:"host"`
}

type CustomTool struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
	Command     string `yaml:"command"`     // The actual shell command to run
	Arguments   string `yaml:"args_schema"` // JSON schema for arguments
}

// Global singleton instance
var (
	globalConfig *ConfigFile
	configOnce   sync.Once
	configErr    error
)

// GetOrLoadConfig ensures the config is loaded exactly once.
// If path is empty, it searches default locations.
func GetOrLoadConfig(path string) (*ConfigFile, error) {
	configOnce.Do(func() {
		// 1. Determine Path
		if path == "" {
			// Look in standard locations
			for _, loc := range []string{".codepicker.yml", ".codepicker.yaml", "codepicker.yml"} {
				if _, err := os.Stat(loc); err == nil {
					path = loc
					break
				}
			}
		}

		// 2. If still empty, return default (nil) without error
		if path == "" {
			return
		}

		// 3. Read File
		data, err := os.ReadFile(path)
		if err != nil {
			configErr = fmt.Errorf("failed to read config file: %w", err)
			return
		}

		// 4. Parse
		var cfg ConfigFile
		if err := yaml.Unmarshal(data, &cfg); err != nil {
			configErr = fmt.Errorf("invalid YAML format in %s: %w", path, err)
			return
		}
		globalConfig = &cfg
	})

	return globalConfig, configErr
}

// Helper method to get model with fallback
func (c *ConfigFile) GetModel() string {
	if c != nil && c.AI.Model != "" {
		return c.AI.Model
	}
	return constants.DefaultModel
}
