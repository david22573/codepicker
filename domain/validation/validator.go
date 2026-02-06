package validation

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/david22573/codepicker/domain/errors"
)

// Validator provides centralized input validation for security and correctness.
type Validator struct {
	maxInputLength    int
	maxTokens         int
	allowedFileExts   map[string]bool
	forbiddenPatterns []*regexp.Regexp
	mu                sync.RWMutex
}

// NewValidator creates a Validator with secure defaults optimized for Kimi K2.5.
func NewValidator() *Validator {
	v := &Validator{
		maxInputLength: 10000,
		maxTokens:      180000,
		allowedFileExts: map[string]bool{
			".go": true, ".md": true, ".yaml": true, ".yml": true,
			".json": true, ".mod": true, ".sum": true, ".txt": true,
			".gitignore": true, ".dockerignore": true, ".toml": true,
		},
	}
	v.compilePatterns()
	return v
}

func (v *Validator) compilePatterns() {
	patterns := []string{
		`(?i)(rm\s+-rf|drop\s+table|delete\s+from\s+\w+\s+where\s+1\s*=\s*1)`,
		`(?i)(chmod\s+777|eval\s*\(|os\.exec\s*\(|exec\.command)`,
		`(?i)(curl\s+.*\|\s*sh|wget\s+.*\|\s*sh|\|\s*bash)`,
		`(?i)(sudo|su\s+-|passwd|/etc/shadow|mkfs|dd\s+if=)`,
	}

	for _, p := range patterns {
		if re, err := regexp.Compile(p); err == nil {
			v.forbiddenPatterns = append(v.forbiddenPatterns, re)
		}
	}
}

// ValidateTask checks task descriptions for safety and size limits.
// FIXED: Method name matches the call in adapters/agent/react.go
func (v *Validator) ValidateTask(input string) error {
	if strings.TrimSpace(input) == "" {
		return errors.NewValidation("validator.task", "input cannot be empty")
	}

	v.mu.RLock()
	maxLen := v.maxInputLength
	v.mu.RUnlock()

	if len(input) > maxLen {
		return errors.NewValidation("validator.task",
			fmt.Sprintf("input exceeds maximum length of %d characters", maxLen))
	}

	if !utf8.ValidString(input) {
		return errors.NewValidation("validator.task", "input contains invalid UTF-8 characters")
	}

	for _, pattern := range v.forbiddenPatterns {
		if pattern.MatchString(input) {
			return errors.NewValidation("validator.task", "input contains potentially dangerous content")
		}
	}

	return nil
}

// ValidateFilePath ensures paths are safe and relative.
func (v *Validator) ValidateFilePath(path string) error {
	if path == "" {
		return errors.NewValidation("validator.file_path", "path cannot be empty")
	}

	clean := filepath.Clean(path)

	if filepath.IsAbs(clean) {
		return errors.NewValidation("validator.file_path", "absolute paths not allowed")
	}

	if strings.Contains(clean, "..") {
		return errors.NewValidation("validator.file_path", "path traversal detected")
	}

	ext := filepath.Ext(clean)
	if ext != "" {
		v.mu.RLock()
		allowed := v.allowedFileExts[ext]
		v.mu.RUnlock()

		if !allowed {
			return errors.NewValidation("validator.file_path",
				fmt.Sprintf("file type %s not allowed", ext))
		}
	}

	return nil
}

func (v *Validator) ValidateJSON(input string, maxSize int) error {
	if maxSize > 0 && len(input) > maxSize {
		return errors.NewValidation("validator.json", "JSON payload exceeds maximum size")
	}

	if !json.Valid([]byte(input)) {
		return errors.NewValidation("validator.json", "invalid JSON format")
	}
	return nil
}
