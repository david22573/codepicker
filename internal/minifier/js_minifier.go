package minifier

import (
	"strings"
)

type JSMinifier struct{}

func (m *JSMinifier) Minify(content []byte) []byte {
	lines := strings.Split(string(content), "\n")
	var result []string

	inBlockComment := false

	for _, line := range lines {
		// Handle Block Comments
		if inBlockComment {
			if strings.Contains(line, "*/") {
				inBlockComment = false
				parts := strings.SplitN(line, "*/", 2)
				if len(parts) > 1 {
					line = parts[1]
				} else {
					continue
				}
			} else {
				continue
			}
		}

		if strings.Contains(line, "/*") {
			parts := strings.SplitN(line, "/*", 2)
			pre := parts[0]

			if strings.Contains(parts[1], "*/") {
				// Inline block comment /* ... */
				rest := strings.SplitN(parts[1], "*/", 2)
				line = pre + rest[1]
			} else {
				// Start of multi-line block comment
				inBlockComment = true
				line = pre
			}
		}

		// Handle Line Comments (//)
		// Note: This simple check misses URLs like http://...
		// but is sufficient for a basic minifier without a full JS parser
		if idx := strings.Index(line, "//"); idx != -1 {
			isUrl := idx > 0 && line[idx-1] == ':'
			if !isUrl {
				line = line[:idx]
			}
		}

		line = strings.TrimRight(line, " \t\r")

		if strings.TrimSpace(line) != "" {
			result = append(result, line)
		}
	}
	return []byte(strings.Join(result, "\n"))
}
