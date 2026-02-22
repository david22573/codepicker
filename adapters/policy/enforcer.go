package policy

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/david22573/codepicker/infra/pathutil"
)

type Enforcer struct {
	config           PolicySchema
	readOnly         bool
	forbiddenRegex   []*regexp.Regexp
	commandWhitelist map[string]bool
}

func NewEnforcer(config PolicySchema, readOnly bool) *Enforcer {
	var regexList []*regexp.Regexp
	for _, keyword := range config.ForbiddenKeywords {
		if len(keyword) == 0 {
			continue
		}

		cleanKeyword := strings.ReplaceAll(regexp.QuoteMeta(keyword), " ", `\s+`)
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

	if strings.Contains(cleanCmd, "..") {
		return false, "BLOCKED: Path traversal (..) detected in command"
	}

	parts := strings.Fields(cleanCmd)
	if len(parts) == 0 {
		return false, "BLOCKED: Malformed command"
	}

	baseCmd := parts[0]
	if !e.commandWhitelist[baseCmd] {
		return false, fmt.Sprintf("BLOCKED: Command '%s' is not in the whitelist", baseCmd)
	}

	dangerous := []string{"|", ">", "&&", "||", ";", "`", "$(", "<", ">>", "&"}
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

	if _, err := pathutil.Clean(input.Path); err != nil {
		return false, fmt.Sprintf("BLOCKED: %s", err.Error())
	}

	return true, ""
}