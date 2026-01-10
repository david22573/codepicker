package minifier

import (
	"regexp"
	"strings"
)

// PassthroughMinifier does nothing (for data files)
type PassthroughMinifier struct{}

func (p *PassthroughMinifier) Minify(content []byte) []byte {
	return content
}

// GenericMinifier handles line-based comments for C-style and Script-style languages
type GenericMinifier struct{}

func (g *GenericMinifier) Minify(content []byte) []byte {
	lines := strings.Split(string(content), "\n")
	var kept []string

	// We can't easily know the extension here in the generic fallback without context,
	// so we strip common comment prefixes or just remove empty lines.
	// For safer generic minification, we primarily strip empty lines and trailing whitespace.

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		// Basic comment stripping for C-style // and Shell/Ruby #
		// Be conservative here to avoid breaking code that uses these chars in strings
		if strings.HasPrefix(trimmed, "//") || strings.HasPrefix(trimmed, "#") {
			// Heuristic: If it looks purely like a comment line, skip it.
			continue
		}

		kept = append(kept, strings.TrimRight(line, " \r"))
	}
	return []byte(strings.Join(kept, "\n"))
}

// SqueezeVerticalWhitespace removes 3+ consecutive newlines
func SqueezeVerticalWhitespace(content []byte) []byte {
	str := string(content)
	// Replace 3 or more newlines with 2 (one empty line between code)
	re := regexp.MustCompile(`\n{3,}`)
	str = re.ReplaceAllString(str, "\n\n")
	return []byte(str)
}
