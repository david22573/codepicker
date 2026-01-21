package config

import (
	"fmt"
	"os"
	"slices"
	"strings"
	"sync"

	"github.com/david22573/codepicker/internal/constants"
	"gopkg.in/yaml.v3"
)

var defaultIgnoredDirs = []string{".git", "node_modules", "vendor", ".codepicker"}

type ConfigFile struct {
	Src         string            `yaml:"src"`
	Output      string            `yaml:"output"`
	Include     []string          `yaml:"include"`
	Exclude     []string          `yaml:"exclude"`
	Minify      bool              `yaml:"minify"`
	Tokens      bool              `yaml:"tokens"`
	Verbose     bool              `yaml:"verbose"`
	AI          AIConfig          `yaml:"ai"`
	Server      ServerConfig      `yaml:"server"`
	CustomTools []CustomTool      `yaml:"tools"`
	MCPServers  []MCPServerConfig `yaml:"mcp_servers"` // NEW: Phase 4
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
	Command     string `yaml:"command"`
	Arguments   string `yaml:"args_schema"`
}

// NEW: Configuration for an MCP Server (e.g. "github", "postgres")
type MCPServerConfig struct {
	Name    string   `yaml:"name"`
	Command string   `yaml:"command"`
	Args    []string `yaml:"args"`
	Env     []string `yaml:"env"`
}

var (
	globalConfig *ConfigFile
	configOnce   sync.Once
	configErr    error
)

func GetOrLoadConfig(path string) (*ConfigFile, error) {
	configOnce.Do(func() {
		if path == "" {
			for _, loc := range []string{".codepicker.yml", ".codepicker.yaml", "codepicker.yml"} {
				if _, err := os.Stat(loc); err == nil {
					path = loc
					break
				}
			}
		}

		if path == "" {
			return
		}

		data, err := os.ReadFile(path)
		if err != nil {
			configErr = fmt.Errorf("failed to read config file: %w", err)
			return
		}

		var cfg ConfigFile
		if err := yaml.Unmarshal(data, &cfg); err != nil {
			configErr = fmt.Errorf("invalid YAML format in %s: %w", path, err)
			return
		}
		globalConfig = &cfg
	})

	return globalConfig, configErr
}

func (c *ConfigFile) GetModel() string {
	if c != nil && c.AI.Model != "" {
		return c.AI.Model
	}
	return constants.DefaultModel
}

func (c *ConfigFile) IsExtensionAllowed(ext string) bool {
	if c == nil || len(c.Include) == 0 {
		safeDefaults := map[string]bool{
			".go": true, ".js": true, ".ts": true, ".py": true,
			".md": true, ".txt": true, ".json": true,
		}
		return safeDefaults[ext]
	}
	for _, allowed := range c.Include {
		if strings.TrimPrefix(allowed, ".") == strings.TrimPrefix(ext, ".") {
			return true
		}
	}
	return false
}

func (c *ConfigFile) IsDirIgnored(dir string) bool {
	if c == nil {
		return false
	}
	return slices.Contains(c.Exclude, dir) || slices.Contains(defaultIgnoredDirs, dir)
}
