package config

import (
	"fmt"
	"os"

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
	WorkerModel string  `yaml:"worker_model"` // NEW: For Supervisor-Worker pattern
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

func LoadConfigFile(path string) (*ConfigFile, error) {
	if path == "" {
		for _, loc := range []string{".codepicker.yml", ".codepicker.yaml", "codepicker.yml"} {
			if _, err := os.Stat(loc); err == nil {
				path = loc
				break
			}
		}
		if path == "" {
			return nil, nil
		}
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var cfg ConfigFile
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("invalid YAML format: %w", err)
	}

	return &cfg, nil
}

func (c *ConfigFile) GetModel() string {
	if c.AI.Model != "" {
		return c.AI.Model
	}
	return constants.DefaultModel
}
