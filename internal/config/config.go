package config

import "strings"

type Config struct {
	IgnoredDirs map[string]bool
	AllowedExts map[string]bool
}

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
			"tmp":            true,
			"coverage":       true,
			".next":          true,
			"target":         true,
		},
		AllowedExts: map[string]bool{
			".go": true, ".js": true, ".ts": true, ".tsx": true, ".jsx": true,
			".py": true, ".java": true, ".kt": true, ".rb": true, ".php": true,
			".c": true, ".cpp": true, ".h": true, ".hpp": true, ".rs": true,
			".cs": true, ".swift": true, ".scala": true, ".sh": true, ".bat": true,
			".lua": true, ".pl": true, ".ex": true, ".exs": true,
			".html": true, ".css": true, ".scss": true, ".sql": true, ".graphql": true,
			".json": true, ".yaml": true, ".yml": true, ".toml": true, ".xml": true,
			".env": false, // Explicitly false by default for security
			".md":  true, ".txt": true, ".rst": true,
		},
	}
}

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
	return special[name]
}

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
