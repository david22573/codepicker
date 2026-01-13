package minifier

import (
	"strings"
)

type JSMinifier struct{}

// Minify performs a safe, whitespace-only optimization.
// PREVIOUSLY: Attempted to strip comments via string parsing, which caused corruption.
// NOW: Removes empty lines and trailing whitespace. Preserves indentation to aid LLM comprehension.
func (m *JSMinifier) Minify(content []byte) []byte {
	lines := strings.Split(string(content), "\n")
	var kept []string

	for _, line := range lines {
		// Remove trailing whitespace (spaces, tabs, carriage returns)
		trimmedRight := strings.TrimRight(line, " \t\r")

		// If the line is empty (or was just whitespace), skip it
		if len(strings.TrimSpace(trimmedRight)) == 0 {
			continue
		}

		// We keep the line exactly as is (preserving indentation)
		kept = append(kept, trimmedRight)
	}

	return []byte(strings.Join(kept, "\n"))
}
