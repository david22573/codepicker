package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/david22573/codepicker/infra/fs"
)

type WriteFileTool struct {
	shadow *fs.ShadowManager
}

func NewWriteFileTool(shadow *fs.ShadowManager) *WriteFileTool {
	return &WriteFileTool{shadow: shadow}
}

func (t *WriteFileTool) Name() string {
	return "write_file"
}

func (t *WriteFileTool) Description() string {
	return `Write a file to the filesystem.
Input: JSON with "path" and "content".
Example: {"path": "main.go", "content": "package main..."}`
}

func (t *WriteFileTool) Execute(ctx context.Context, args string) (string, error) {
	var input struct {
		Path    string `json:"path"`
		Content string `json:"content"`
	}

	// Sanitize input to remove Markdown blocks
	cleanArgs := t.cleanJSON(args)

	if err := json.Unmarshal([]byte(cleanArgs), &input); err != nil {
		// FALLBACK: LLMs often fail to escape newlines and quotes in large JSON strings.
		// If standard unmarshaling fails, use our robust manual extractor.
		path, content, fallbackErr := t.robustParse(cleanArgs)
		if fallbackErr != nil {
			return "", fmt.Errorf("JSON parsing failed and fallback recovery unsuccessful: %w (Original error: %v)", fallbackErr, err)
		}
		input.Path = path
		input.Content = content
	}

	path, err := t.shadow.Write(input.Path, []byte(input.Content))
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("Success: File written to shadow storage at %s", path), nil
}

func (t *WriteFileTool) cleanJSON(input string) string {
	input = strings.TrimSpace(input)
	if strings.HasPrefix(input, "```json") {
		input = input[7:]
	} else if strings.HasPrefix(input, "```") {
		input = input[3:]
	}
	if strings.HasSuffix(input, "```") {
		input = input[:len(input)-3]
	}
	return strings.TrimSpace(input)
}

// robustParse manually extracts the path and content from a malformed JSON string.
func (t *WriteFileTool) robustParse(args string) (string, string, error) {
	// Find the path
	pathRe := regexp.MustCompile(`"path"\s*:\s*"([^"]+)"`)
	pathMatches := pathRe.FindStringSubmatch(args)
	if len(pathMatches) < 2 {
		return "", "", fmt.Errorf("could not locate 'path' key in malformed JSON")
	}
	path := pathMatches[1]

	// Find the content
	contentPrefix := `"content":`
	idx := strings.Index(args, contentPrefix)
	if idx == -1 {
		return "", "", fmt.Errorf("could not locate 'content' key in malformed JSON")
	}

	contentStr := strings.TrimSpace(args[idx+len(contentPrefix):])

	// Strip surrounding quotes and braces
	if strings.HasPrefix(contentStr, `"`) {
		contentStr = contentStr[1:]
	}
	if strings.HasSuffix(contentStr, `}`) {
		contentStr = strings.TrimSpace(contentStr[:len(contentStr)-1])
	}
	if strings.HasSuffix(contentStr, `"`) {
		contentStr = contentStr[:len(contentStr)-1]
	}

	// Unescape standard JSON sequences if the LLM attempted to escape them,
	// but leave raw newlines untouched.
	contentStr = strings.ReplaceAll(contentStr, `\n`, "\n")
	contentStr = strings.ReplaceAll(contentStr, `\"`, "\"")
	contentStr = strings.ReplaceAll(contentStr, `\t`, "\t")

	return path, contentStr, nil
}
