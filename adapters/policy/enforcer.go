package policy

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

// Enforcer implements agent.Policy with robust regex-based rules and whitelisting.
type Enforcer struct {
	config           PolicySchema
	readOnly         bool
	forbiddenRegex   []*regexp.Regexp
	commandWhitelist map[string]bool
}

// NewEnforcer creates a production-hardened policy engine.
func NewEnforcer(config PolicySchema, readOnly bool) *Enforcer {
	var regexList []*regexp.Regexp
	for _, keyword := range config.ForbiddenKeywords {
		if len(keyword) == 0 {
			continue
		}

		// Escape the keyword so dots/slashes are treated as literals
		cleanKeyword := strings.ReplaceAll(regexp.QuoteMeta(keyword), " ", `\s+`)

		// SMART BOUNDARY LOGIC:
		// Only add \b (word boundary) if the keyword starts/ends with a word char (a-z, 0-9).
		// This fixes the bug where "/etc/passwd" wasn't matching because '/' is not a word char.
		pattern := `(?i)`
		if isWordChar(keyword[0]) {
			pattern += `\b`
		}
		pattern += cleanKeyword
		if isWordChar(keyword[len(keyword)-1]) {
			pattern += `\b`
		}

		if regex, err := regexp.Compile(pattern); err == nil {
			regexList = append(regexList, regex)
		}
	}

	whitelist := map[string]bool{
		"go":   true,
		"git":  true,
		"ls":   true,
		"cat":  true,
		"grep": true,
		"make": true,
	}

	return &Enforcer{
		config:           config,
		readOnly:         readOnly,
		forbiddenRegex:   regexList,
		commandWhitelist: whitelist,
	}
}

// isWordChar checks if a byte is a letter, digit, or underscore.
func isWordChar(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_'
}

func (e *Enforcer) Mode() string {
	if e.readOnly {
		return "guarded-readonly"
	}
	return "guarded-active"
}

func (e *Enforcer) CanExecute(toolName string, args string) (bool, string) {
	if e.readOnly {
		if toolName == "write_file" || toolName == "run_cmd" {
			return false, "BLOCKED: Side-effects are disabled in READ-ONLY mode."
		}
	}

	// Regex check against the raw arguments string
	for _, regex := range e.forbiddenRegex {
		if regex.MatchString(args) {
			return false, fmt.Sprintf("BLOCKED: Forbidden pattern detected: %s", regex.String())
		}
	}

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

	parts := strings.Fields(cleanCmd)
	if len(parts) == 0 {
		return false, "BLOCKED: Malformed command"
	}

	baseCmd := parts[0]
	if !e.commandWhitelist[baseCmd] {
		return false, fmt.Sprintf("BLOCKED: Command '%s' is not in the whitelist", baseCmd)
	}

	dangerous := []string{"|", ">", "&&", "||", ";", "`", "$("}
	for _, d := range dangerous {
		if strings.Contains(cleanCmd, d) {
			return false, fmt.Sprintf("BLOCKED: Dangerous shell operator detected: %s", d)
		}
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

	if strings.Contains(input.Path, "..") {
		return false, "BLOCKED: Path traversal (..) detected"
	}

	return true, ""
}
