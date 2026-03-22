package indexer

import (
	"regexp"
	"strings"
)

var (
	tsClassRegex  = regexp.MustCompile(`^(?:export\s+)?(?:default\s+)?(?:abstract\s+)?class\s+([a-zA-Z0-9_]+)`)
	tsFuncRegex   = regexp.MustCompile(`^(?:export\s+)?(?:default\s+)?(?:async\s+)?function\s+([a-zA-Z0-9_]+)`)
	tsConstRegex  = regexp.MustCompile(`^(?:export\s+)?const\s+([a-zA-Z0-9_]+)\s*=\s*(?:async\s*)?(?:\([^)]*\)|[^=]*)\s*=>`)
	tsTypeRegex   = regexp.MustCompile(`^(?:export\s+)?(?:type|interface)\s+([a-zA-Z0-9_]+)`)
	tsImportRegex = regexp.MustCompile(`^import\s+.*from\s+['"]([^'"]+)['"]`)
)

func parseTypeScript(content []byte, fm *FileMap) {
	lines := strings.Split(string(content), "\n")
	inComment := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		if !inComment {
			if strings.HasPrefix(trimmed, "/*") {
				if !strings.Contains(trimmed, "*/") {
					inComment = true
				}
				continue
			}
		} else {
			if strings.Contains(trimmed, "*/") {
				inComment = false
			}
			continue
		}

		if strings.HasPrefix(trimmed, "//") {
			continue
		}

		if tsImportRegex.MatchString(trimmed) {
			fm.Imports = append(fm.Imports, trimmed)
			continue
		}

		if matches := tsClassRegex.FindStringSubmatch(trimmed); matches != nil {
			fm.Symbols = append(fm.Symbols, Symbol{
				Name:      matches[1],
				Kind:      "class",
				Signature: extractTSSignature(trimmed),
			})
			continue
		}

		if matches := tsFuncRegex.FindStringSubmatch(trimmed); matches != nil {
			fm.Symbols = append(fm.Symbols, Symbol{
				Name:      matches[1],
				Kind:      "func",
				Signature: extractTSSignature(trimmed),
			})
			continue
		}

		if matches := tsTypeRegex.FindStringSubmatch(trimmed); matches != nil {
			fm.Symbols = append(fm.Symbols, Symbol{
				Name:      matches[1],
				Kind:      "type",
				Signature: extractTSSignature(trimmed),
			})
			continue
		}

		if matches := tsConstRegex.FindStringSubmatch(trimmed); matches != nil {
			fm.Symbols = append(fm.Symbols, Symbol{
				Name:      matches[1],
				Kind:      "func",
				Signature: extractTSSignature(trimmed),
			})
		}
	}
}

func extractTSSignature(line string) string {
	idx := strings.Index(line, "{")
	if idx > 0 {
		return strings.TrimSpace(line[:idx])
	}
	return line
}
