package minifier

import (
	"strings"
)

type PythonMinifier struct{}

// Minify performs a safe, whitespace-only optimization.
// PREVIOUSLY: Attempted to parse docstrings/comments, which is unsafe without a lexer.
// NOW: Removes empty lines and trailing whitespace.
// CRITICAL: Preserves leading whitespace (indentation) as it is semantically significant in Python.
func (m *PythonMinifier) Minify(content []byte) []byte {
	lines := strings.Split(string(content), "\n")
	var kept []string

	for _, line := range lines {
		// Remove trailing whitespace only
		trimmedRight := strings.TrimRight(line, " \t\r")

		// If the line is completely empty, skip it to reduce vertical token usage
		if len(strings.TrimSpace(trimmedRight)) == 0 {
			continue
		}

		// Append the line with original indentation
		kept = append(kept, trimmedRight)
	}

	return []byte(strings.Join(kept, "\n"))
}
