package config

import (
	"fmt"
	"os"

	"github.com/david22573/codepicker/internal/constants"
	"gopkg.in/yaml.v3"
)

type ConfigFile struct {
	Src     string   `yaml:"src"`
	Output  string   `yaml:"output"`
	Include []string `yaml:"include"`
	Exclude []string `yaml:"exclude"`
	Minify  bool     `yaml:"minify"`
	Tokens  bool     `yaml:"tokens"`
	Verbose bool     `yaml:"verbose"`
	AI      AIConfig `yaml:"ai"`
}

type AIConfig struct {
	Model       string  `yaml:"model"`
	Temperature float32 `yaml:"temperature"`
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

// GetModel returns the configured model or the default if not set
func (c *ConfigFile) GetModel() string {
	if c.AI.Model != "" {
		return c.AI.Model
	}
	return constants.DefaultModel
}
