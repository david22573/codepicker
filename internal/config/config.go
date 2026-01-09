package config

// IgnoredDirs are folders we never want to look inside
var IgnoredDirs = map[string]bool{
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
}

// AllowedExts are the file types we want to harvest
var AllowedExts = map[string]bool{
	// Programming Languages
	".go": true, ".js": true, ".ts": true, ".tsx": true, ".jsx": true,
	".py": true, ".java": true, ".kt": true, ".rb": true, ".php": true,
	".c": true, ".cpp": true, ".h": true, ".hpp": true, ".rs": true,
	".cs": true, ".swift": true, ".scala": true, ".sh": true, ".bat": true,
	".lua": true, ".pl": true, ".ex": true, ".exs": true,

	// Web & Config
	".html": true, ".css": true, ".scss": true, ".sql": true, ".graphql": true,
	".json": true, ".yaml": true, ".yml": true, ".toml": true, ".xml": true,
	".env": false, // explicitly false for safety

	// Docs
	".md": true, ".txt": true, ".rst": true,
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
