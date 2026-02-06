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
// It prevents injection attacks, path traversal, and LLM context overflow.
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
		maxTokens:      180000, // 200K limit minus safety buffer
		allowedFileExts: map[string]bool{
			".go": true, ".md": true, ".yaml": true, ".yml": true,
			".json": true, ".mod": true, ".sum": true, ".txt": true,
			".gitignore": true, ".dockerignore": true, ".toml": true,
		},
	}
	v.compilePatterns()
	return v
}

// compilePatterns initializes regex patterns for dangerous content detection.
func (v *Validator) compilePatterns() {
	patterns := []string{
		`(?i)(rm\s+-rf|drop\s+table|delete\s+from\s+\w+\s+where\s+1\s*=\s*1)`,
		`(?i)(chmod\s+777|eval\s*\(|os\.exec\s*\(|exec\.command)`,
		`(?i)(curl\s+.*\|\s*sh|wget\s+.*\|\s*sh|\|\s*bash)`,
		`(?i)(<script|javascript:|onerror\s*=|onload\s*=)`,
		`(\.\./|\.\.\\)`, // Path traversal attempts
		`(?i)(sudo|su\s+-|passwd|/etc/shadow|mkfs|dd\s+if=)`,
	}

	for _, p := range patterns {
		if re, err := regexp.Compile(p); err == nil {
			v.forbiddenPatterns = append(v.forbiddenPatterns, re)
		}
	}
}

// ValidateUserInput checks task descriptions and user prompts for safety and size limits.
func (v *Validator) ValidateUserInput(input string) error {
	if strings.TrimSpace(input) == "" {
		return errors.NewValidation("validator.user_input", "input cannot be empty")
	}

	v.mu.RLock()
	maxLen := v.maxInputLength
	v.mu.RUnlock()

	if len(input) > maxLen {
		return errors.NewValidation("validator.user_input",
			fmt.Sprintf("input exceeds maximum length of %d characters", maxLen))
	}

	if !utf8.ValidString(input) {
		return errors.NewValidation("validator.user_input", "input contains invalid UTF-8 characters")
	}

	// Check for forbidden patterns (injection protection)
	for _, pattern := range v.forbiddenPatterns {
		if pattern.MatchString(input) {
			return errors.NewValidation("validator.user_input",
				"input contains potentially dangerous content")
		}
	}

	return nil
}

// ValidateFilePath ensures paths are safe, relative, and have allowed extensions.
// This is the primary defense against path traversal attacks (Phase 1.1).
func (v *Validator) ValidateFilePath(path string) error {
	if path == "" {
		return errors.NewValidation("validator.file_path", "path cannot be empty")
	}

	// Normalize and check for traversal
	clean := filepath.Clean(path)

	// Reject absolute paths
	if filepath.IsAbs(clean) {
		return errors.NewValidation("validator.file_path", "absolute paths not allowed")
	}

	// Check for path traversal after cleaning (defense in depth)
	if strings.Contains(clean, "..") {
		return errors.NewValidation("validator.file_path", "path traversal detected")
	}

	// Validate extension unless it's a special allowed file
	ext := filepath.Ext(clean)
	if ext == "" && !v.isAllowedSpecialFile(clean) {
		return errors.NewValidation("validator.file_path", "file extension required")
	}

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

// SanitizePath validates and returns a cleaned relative path, verifying it stays within project root.
// Use this before any filesystem operations in ShadowManager or similar components.
func (v *Validator) SanitizePath(projectRoot, relPath string) (string, error) {
	if err := v.ValidateFilePath(relPath); err != nil {
		return "", err
	}

	clean := filepath.Clean(relPath)
	fullPath := filepath.Join(projectRoot, clean)

	// Verify the resolved path doesn't escape project root
	// Handle symlinks by evaluating them
	resolved, err := filepath.EvalSymlinks(fullPath)
	if err != nil {
		// File might not exist yet (new file creation), check parent directory
		resolved = fullPath
	}

	rootResolved, err := filepath.EvalSymlinks(projectRoot)
	if err != nil {
		rootResolved = projectRoot
	}

	// Ensure resolved path has the root as prefix
	if !strings.HasPrefix(strings.ToLower(resolved), strings.ToLower(rootResolved)) {
		return "", errors.NewValidation("validator.sanitize_path", "path escapes project root")
	}

	return clean, nil
}

// ValidateJSON ensures input is valid JSON and within size limits.
// Use this for validating tool arguments before unmarshaling.
func (v *Validator) ValidateJSON(input string, maxSize int) error {
	if maxSize > 0 && len(input) > maxSize {
		return errors.NewValidation("validator.json", "JSON payload exceeds maximum size")
	}

	if !json.Valid([]byte(input)) {
		return errors.NewValidation("validator.json", "invalid JSON format")
	}

	// Additional check: ensure it's an object (prevent JSON bombs)
	var js map[string]any
	if err := json.Unmarshal([]byte(input), &js); err != nil {
		return errors.NewValidation("validator.json", "JSON must be an object")
	}

	return nil
}

// ValidateCommand checks shell commands for dangerous patterns (Phase 1.2 enhancement).
func (v *Validator) ValidateCommand(cmd string) error {
	if strings.TrimSpace(cmd) == "" {
		return errors.NewValidation("validator.command", "command cannot be empty")
	}

	if len(cmd) > 1000 {
		return errors.NewValidation("validator.command", "command exceeds maximum length of 1000 chars")
	}

	// Block dangerous shell operators
	dangerous := []string{";", "&&", "||", "|", "`", "$(", "${", ">", "<("}
	for _, char := range dangerous {
		if strings.Contains(cmd, char) {
			return errors.NewValidation("validator.command",
				fmt.Sprintf("command contains forbidden operator: %s", char))
		}
	}

	return nil
}

// ValidateTokenCount checks estimated token usage against LLM limits (Phase 3).
func (v *Validator) ValidateTokenCount(estimatedTokens int) error {
	if estimatedTokens < 0 {
		return errors.NewValidation("validator.tokens", "token count cannot be negative")
	}

	v.mu.RLock()
	maxTokens := v.maxTokens
	v.mu.RUnlock()

	if estimatedTokens > maxTokens {
		return errors.NewValidation("validator.tokens",
			fmt.Sprintf("estimated tokens %d exceeds limit %d", estimatedTokens, maxTokens))
	}

	return nil
}

// isAllowedSpecialFile checks for extension-less files like Makefile, Dockerfile.
func (v *Validator) isAllowedSpecialFile(name string) bool {
	allowed := []string{"Makefile", "Dockerfile", "LICENSE", "README", "CHANGELOG", "NOTICE"}
	base := strings.ToUpper(filepath.Base(name))
	for _, a := range allowed {
		if base == a || strings.HasPrefix(base, a+".") {
			return true
		}
	}
	return false
}

// SetMaxTokens updates the token limit dynamically (thread-safe).
func (v *Validator) SetMaxTokens(max int) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.maxTokens = max
}

// AddAllowedExtension adds a file extension to the whitelist (thread-safe).
func (v *Validator) AddAllowedExtension(ext string) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if !strings.HasPrefix(ext, ".") {
		ext = "." + ext
	}
	v.allowedFileExts[strings.ToLower(ext)] = true
}

// RemoveAllowedExtension removes a file extension from the whitelist.
func (v *Validator) RemoveAllowedExtension(ext string) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if !strings.HasPrefix(ext, ".") {
		ext = "." + ext
	}
	delete(v.allowedFileExts, strings.ToLower(ext))
}
