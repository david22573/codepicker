package minifier

import (
	"strings"
)

type PythonMinifier struct{}

func (m *PythonMinifier) Minify(content []byte) []byte {
	lines := strings.Split(string(content), "\n")
	var kept []string

	inDocstring := false
	docstringQuote := ""

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}

		// Handle Docstrings (""" or ''')
		if !inDocstring {
			if strings.HasPrefix(trimmed, `"""`) || strings.HasPrefix(trimmed, `'''`) {
				quoteType := trimmed[:3]
				// Check for one-liner docstring """..."""
				if len(trimmed) > 3 && strings.HasSuffix(trimmed, quoteType) {
					continue
				}
				inDocstring = true
				docstringQuote = quoteType
				continue
			}
		} else {
			if strings.HasSuffix(trimmed, docstringQuote) {
				inDocstring = false
				continue
			}
			continue
		}

		// Handle Comments (#)
		if strings.HasPrefix(trimmed, "#") {
			continue
		}

		kept = append(kept, strings.TrimRight(line, " \r"))
	}
	return []byte(strings.Join(kept, "\n"))
}
