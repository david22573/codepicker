package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// ConfigFile represents the structure of .codepicker.yml
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

// LoadConfigFile attempts to load configuration from a YAML file
func LoadConfigFile(path string) (*ConfigFile, error) {
	if path == "" {
		// Try default locations
		for _, loc := range []string{".codepicker.yml", ".codepicker.yaml", "codepicker.yml"} {
			if _, err := os.Stat(loc); err == nil {
				path = loc
				break
			}
		}
		if path == "" {
			return nil, nil // No config file found, not an error
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
