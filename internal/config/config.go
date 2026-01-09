package config

// Config holds the runtime configuration for the scanner
type Config struct {
	IgnoredDirs map[string]bool
	AllowedExts map[string]bool
}

// NewConfig creates a configuration with default values
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
			// Programming Languages
			".go": true, ".js": true, ".ts": true, ".tsx": true, ".jsx": true,
			".py": true, ".java": true, ".kt": true, ".rb": true, ".php": true,
			".c": true, ".cpp": true, ".h": true, ".hpp": true, ".rs": true,
			".cs": true, ".swift": true, ".scala": true, ".sh": true, ".bat": true,
			".lua": true, ".pl": true, ".ex": true, ".exs": true,
			// Web & Config
			".html": true, ".css": true, ".scss": true, ".sql": true, ".graphql": true,
			".json": true, ".yaml": true, ".yml": true, ".toml": true, ".xml": true,
			".env": false,
			// Docs
			".md": true, ".txt": true, ".rst": true,
		},
	}
}

// IsSpecialFile handles files that don't have standard extensions
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

// AddAllowedExtensions parses a comma-separated string of extensions (e.g., ".vue,.svelte")
func (c *Config) AddAllowedExtensions(exts []string) {
	for _, ext := range exts {
		if ext != "" {
			if ext[0] != '.' {
				ext = "." + ext
			}
			c.AllowedExts[ext] = true
		}
	}
}

// AddIgnoredDirs parses a comma-separated list of directories to ignore
func (c *Config) AddIgnoredDirs(dirs []string) {
	for _, dir := range dirs {
		if dir != "" {
			c.IgnoredDirs[dir] = true
		}
	}
}

