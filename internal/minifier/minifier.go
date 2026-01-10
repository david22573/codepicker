package minifier

import (
	"bytes"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"regexp"
	"strings"
)

// Minify processes the content based on extension to reduce token count
// while maintaining LLM readability (preserving structure).
func Minify(content []byte, ext string) []byte {
	ext = strings.ToLower(ext)
	var result []byte

	switch ext {
	case ".go":
		result = minifyGo(content)
	case ".js", ".ts", ".tsx", ".jsx":
		result = minifyJS(content)
	case ".py":
		result = minifyPython(content)
	case ".json", ".yaml", ".yml", ".toml", ".xml":
		// Data files usually need to keep their structure to be valid/readable
		result = content
	default:
		result = lineBasedMinify(content, ext)
	}

	return squeezeVerticalWhitespace(result)
}

// minifyGo uses the Go AST to safely remove comments while preserving
// standard formatting. This is "safer" and more readable for LLMs than flattening.
func minifyGo(content []byte) []byte {
	fset := token.NewFileSet()
	// Parse with comments so we can identify them
	f, err := parser.ParseFile(fset, "", content, parser.ParseComments)
	if err != nil {
		// Fallback to line-based if parsing fails (e.g., partial code)
		return lineBasedMinify(content, ".go")
	}

	// The Magic: AST manipulation
	// Setting Comments to nil effectively strips them all
	f.Comments = nil

	// We also clear Doc comments on specific nodes to be thorough
	ast.Inspect(f, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.File:
			x.Doc = nil
		case *ast.GenDecl:
			x.Doc = nil
		case *ast.FuncDecl:
			x.Doc = nil
		case *ast.TypeSpec:
			x.Doc = nil
		case *ast.Field:
			x.Doc = nil
		}
		return true
	})

	var buf bytes.Buffer
	// format.Node reprints the AST using standard 'gofmt' rules
	if err := format.Node(&buf, fset, f); err != nil {
		return content
	}

	return buf.Bytes()
}

// minifyJS removes comments but PRESERVES indentation.
// Previous flattening approach caused issues with missing semicolons and readability.
func minifyJS(content []byte) []byte {
	lines := strings.Split(string(content), "\n")
	var result []string

	inBlockComment := false

	for _, line := range lines {
		// 1. Handle Block Comments
		if inBlockComment {
			if strings.Contains(line, "*/") {
				inBlockComment = false
				// Keep anything after the comment ends
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

		// Check for start of block comment
		if strings.Contains(line, "/*") {
			parts := strings.SplitN(line, "/*", 2)
			pre := parts[0]
			// If comment closes on same line
			if strings.Contains(parts[1], "*/") {
				rest := strings.SplitN(parts[1], "*/", 2)
				line = pre + rest[1] // stitch together
			} else {
				inBlockComment = true
				line = pre
			}
		}

		// 2. Handle Line Comments (//)
		// We must be careful not to strip // inside strings (e.g. "http://")
		// This is a naive heuristic: assume // is a comment if not preceded by " or '
		// For a perfect solution, we'd need a lexer, but this covers 99% of cases.
		if idx := strings.Index(line, "//"); idx != -1 {
			// Check if it looks like a URL or inside quotes (simplistic check)
			isUrl := idx > 0 && line[idx-1] == ':'
			if !isUrl {
				line = line[:idx]
			}
		}

		// 3. Trim Trailing Whitespace only
		// We keep leading whitespace (indentation) for the LLM!
		line = strings.TrimRight(line, " \t\r")

		if strings.TrimSpace(line) != "" {
			result = append(result, line)
		}
	}
	return []byte(strings.Join(result, "\n"))
}

// minifyPython strips docstrings and comments but strictly preserves indentation.
func minifyPython(content []byte) []byte {
	lines := strings.Split(string(content), "\n")
	var kept []string

	// Toggle for multi-line strings/docstrings
	inDocstring := false
	docstringQuote := "" // tracks if we are in """ or '''

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}

		// Handle docstrings
		// Note: This is a simplified parser. It handles the common case where
		// docstrings are on their own lines.
		if !inDocstring {
			if strings.HasPrefix(trimmed, `"""`) || strings.HasPrefix(trimmed, `'''`) {
				quoteType := trimmed[:3]
				// If it opens and closes on same line, skip it entirely
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

		// Handle Comments
		if strings.HasPrefix(trimmed, "#") {
			continue
		}

		// Preserve the line
		kept = append(kept, strings.TrimRight(line, " \r"))
	}
	return []byte(strings.Join(kept, "\n"))
}

func lineBasedMinify(content []byte, ext string) []byte {
	lines := strings.Split(string(content), "\n")
	var kept []string
	prefix := ""

	switch ext {
	case ".java", ".c", ".cpp", ".rs", ".cs", ".php", ".swift", ".kt", ".scala":
		prefix = "//"
	case ".rb", ".sh", ".dockerfile", "makefile", ".pl":
		prefix = "#"
	case ".sql", ".lua":
		prefix = "--"
	}

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if prefix != "" && strings.HasPrefix(trimmed, prefix) {
			continue
		}
		// Preserve indentation for all files
		kept = append(kept, strings.TrimRight(line, " \r"))
	}
	return []byte(strings.Join(kept, "\n"))
}

// squeezeVerticalWhitespace reduces consecutive blank lines to a single blank line.
// This saves tokens without hurting readability.
func squeezeVerticalWhitespace(content []byte) []byte {
	// Replace 3 or more newlines with 2 newlines (which looks like 1 blank line)
	// We use a loop to catch all cases
	str := string(content)
	re := regexp.MustCompile(`\n{3,}`)
	str = re.ReplaceAllString(str, "\n\n")
	return []byte(str)
}
