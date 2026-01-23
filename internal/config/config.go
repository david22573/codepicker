package config

import (
	"fmt"
	"os"
	"strings"

	"github.com/joho/godotenv"
)

// LLMConfig holds settings for the AI provider
type LLMConfig struct {
	Provider string // "openai" or "openrouter"
	Model    string
	APIKey   string
}

// Config is the unified configuration struct
// It satisfies requirements for both the Scanner (file filtering) and the Engine (LLM/Agent)
type Config struct {
	// --- Scanner/Context Fields ---
	IgnoredDirs map[string]bool
	AllowedExts map[string]bool

	// --- Agent/Engine Fields ---
	ProjectRoot string
	LLM         LLMConfig
}

// NewConfig creates a Config with default scanner settings
func NewConfig() *Config {
	return &Config{
		IgnoredDirs: map[string]bool{
			".git":           true,
			".vscode":        true,
			".idea":          true,
			"node_modules":   true,
			"vendor":         true,
			"bin":            true,
			"dist":           true,
			"build":          true,
			"__pycache__":    true,
			"codepicker_out": true,
			".codepicker":    true,
		},
		AllowedExts: map[string]bool{
			".go": true, ".js": true, ".ts": true, ".tsx": true, ".jsx": true,
			".py": true, ".java": true, ".kt": true, ".rb": true, ".php": true,
			".c": true, ".cpp": true, ".h": true, ".hpp": true, ".rs": true,
			".md": true, ".txt": true, ".json": true, ".yaml": true, ".toml": true,
			".sql": true, ".html": true, ".css": true,
		},
	}
}

// Load populates the Config with environment variables and defaults for the Agent
func Load() (*Config, error) {
	// Start with defaults
	cfg := NewConfig()

	_ = godotenv.Load()

	cwd, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("failed to get current working directory: %w", err)
	}
	cfg.ProjectRoot = cwd

	// Load LLM Settings
	cfg.LLM = LLMConfig{
		Provider: getEnvOrDefault("LLM_PROVIDER", "openrouter"),
		Model:    getEnvOrDefault("LLM_MODEL", "openai/gpt-4o-mini"),
		APIKey:   os.Getenv("LLM_API_KEY"),
	}

	// Fallback keys if the main one is missing
	if cfg.LLM.APIKey == "" {
		if cfg.LLM.Provider == "openai" {
			cfg.LLM.APIKey = os.Getenv("OPENAI_API_KEY")
		} else {
			cfg.LLM.APIKey = os.Getenv("OPENROUTER_API_KEY")
		}
	}

	return cfg, nil
}

// IsSpecialFile checks for files that should always be included regardless of extension
// This fixes the "undefined: config.IsSpecialFile" error
func IsSpecialFile(name string) bool {
	special := map[string]bool{
		"makefile":     true,
		"dockerfile":   true,
		"gemfile":      true,
		"jenkinsfile":  true,
		"procfile":     true,
		"readme":       true,
		"license":      true,
		"go.mod":       true,
		"go.sum":       true,
		"package.json": true,
	}
	return special[strings.ToLower(name)]
}

// Helper method for Scanner to add dynamic extensions
func (c *Config) AddAllowedExtensions(exts []string) {
	if c.AllowedExts == nil {
		c.AllowedExts = make(map[string]bool)
	}
	for _, ext := range exts {
		ext = strings.TrimSpace(ext)
		if ext != "" {
			if !strings.HasPrefix(ext, ".") {
				ext = "." + ext
			}
			c.AllowedExts[ext] = true
		}
	}
}

// Helper method for Scanner to add dynamic ignore dirs
func (c *Config) AddIgnoredDirs(dirs []string) {
	if c.IgnoredDirs == nil {
		c.IgnoredDirs = make(map[string]bool)
	}
	for _, dir := range dirs {
		dir = strings.TrimSpace(dir)
		if dir != "" {
			c.IgnoredDirs[dir] = true
		}
	}
}

func getEnvOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
