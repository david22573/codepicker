package policy

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
)

// Enforcer implements agent.Policy with robust regex-based rules.
type Enforcer struct {
	config           PolicySchema
	readOnly         bool
	forbiddenRegex   []*regexp.Regexp
	commandWhitelist map[string]bool
}

// NewEnforcer creates a production-hardened policy engine.
func NewEnforcer(config PolicySchema, readOnly bool) *Enforcer {
	// Compile forbidden patterns into regex for robust matching
	var regexList []*regexp.Regexp
	for _, keyword := range config.ForbiddenKeywords {
		// Make pattern whitespace-insensitive (e.g., "rm -rf" matches "rm    -rf")
		pattern := strings.ReplaceAll(regexp.QuoteMeta(keyword), " ", `\s+`)
		regex, err := regexp.Compile(`(?i)` + pattern)
		if err == nil {
			regexList = append(regexList, regex)
		}
	}

	// Initialize the command whitelist from roadmap recommendations
	whitelist := map[string]bool{
		"go fmt":     true,
		"go test":    true,
		"go build":   true,
		"go mod":     true,
		"git status": true,
		"git diff":   true,
		"ls":         true,
	}

	return &Enforcer{
		config:           config,
		readOnly:         readOnly,
		forbiddenRegex:   regexList,
		commandWhitelist: whitelist,
	}
}

func (e *Enforcer) Mode() string {
	if e.readOnly {
		return "guarded-readonly"
	}
	return "guarded-active"
}

// CanExecute enforces the JSON policy rules on tool usage.
func (e *Enforcer) CanExecute(toolName string, args string) (bool, string) {
	// 1. Check Global Read-Only Mode
	if e.readOnly {
		if toolName == "write_file" || toolName == "run_cmd" {
			return false, "BLOCKED: Running in READ-ONLY mode."
		}
	}

	// 2. Regex-based Forbidden Pattern Matching
	for _, regex := range e.forbiddenRegex {
		if regex.MatchString(args) {
			return false, fmt.Sprintf("BLOCKED: Forbidden pattern detected: %s", regex.String())
		}
	}

	// 3. Tool-Specific Validation
	switch toolName {
	case "run_cmd":
		return e.validateCommand(args)
	case "write_file", "read_file":
		return e.validateFileSystemAccess(toolName, args)
	}

	return true, ""
}

func (e *Enforcer) validateCommand(args string) (bool, string) {
	var input struct {
		Command string `json:"command"`
	}
	if err := json.Unmarshal([]byte(args), &input); err != nil {
		return false, "BLOCKED: Invalid JSON for run_cmd"
	}

	cleanCmd := strings.TrimSpace(input.Command)
	if cleanCmd == "" {
		return false, "BLOCKED: Command cannot be empty"
	}

	// Extract base command (e.g., "go test" from "go test ./...")
	parts := strings.Fields(cleanCmd)
	if len(parts) == 0 {
		return false, "BLOCKED: Malformed command"
	}

	baseCmd := parts[0]
	if len(parts) > 1 && (baseCmd == "go" || baseCmd == "git") {
		baseCmd = baseCmd + " " + parts[1]
	}

	if !e.commandWhitelist[baseCmd] {
		return false, fmt.Sprintf("BLOCKED: Command '%s' is not in the whitelist", baseCmd)
	}

	return true, ""
}

func (e *Enforcer) validateFileSystemAccess(toolName, args string) (bool, string) {
	var input struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal([]byte(args), &input); err != nil {
		return false, fmt.Sprintf("BLOCKED: Invalid JSON for %s", toolName)
	}

	if input.Path == "" {
		return false, "BLOCKED: Path argument is missing"
	}

	// Existing Path Traversal Check
	if strings.Contains(input.Path, "..") {
		return false, "BLOCKED: Path traversal (..) detected"
	}

	if !e.isPathAllowed(input.Path) {
		return false, fmt.Sprintf("BLOCKED: Path '%s' is not in allowed_globs.", input.Path)
	}

	return true, ""
}

func (e *Enforcer) isPathAllowed(path string) bool {
	cleanPath := filepath.ToSlash(filepath.Clean(path))
	for _, pattern := range e.config.AllowedGlobs {
		if strings.HasSuffix(pattern, "/**") {
			prefix := strings.TrimSuffix(pattern, "**")
			if strings.HasPrefix(cleanPath, prefix) {
				return true
			}
			continue
		}
		matched, _ := filepath.Match(pattern, cleanPath)
		if matched {
			return true
		}
	}
	return false
}
