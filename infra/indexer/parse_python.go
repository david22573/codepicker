package indexer

import (
	"regexp"
	"strings"
)

var (
	pyDefRegex    = regexp.MustCompile(`^(\s*)def\s+([a-zA-Z0-9_]+)\s*\(`)
	pyClassRegex  = regexp.MustCompile(`^(\s*)class\s+([a-zA-Z0-9_]+)\s*[\(:]`)
	pyImportRegex = regexp.MustCompile(`^(?:from\s+([a-zA-Z0-9_\.]+)\s+)?import\s+(.*)`)
)

func parsePython(content []byte, fm *FileMap) {
	lines := strings.Split(string(content), "\n")
	inMultiLineString := false
	var quoteChar string

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		if !inMultiLineString {
			if strings.HasPrefix(trimmed, `"""`) || strings.HasPrefix(trimmed, `'''`) {
				if len(trimmed) > 3 && strings.HasSuffix(trimmed[3:], trimmed[:3]) {
					continue
				}
				inMultiLineString = true
				quoteChar = trimmed[:3]
				continue
			}
		} else {
			if strings.Contains(trimmed, quoteChar) {
				inMultiLineString = false
			}
			continue
		}

		if strings.HasPrefix(trimmed, "#") {
			continue
		}

		if pyImportRegex.MatchString(trimmed) {
			fm.Imports = append(fm.Imports, trimmed)
			continue
		}

		if matches := pyClassRegex.FindStringSubmatch(line); matches != nil {
			sig := strings.TrimSpace(strings.Split(line, ":")[0])
			fm.Symbols = append(fm.Symbols, Symbol{
				Name:      matches[2],
				Kind:      "class",
				Signature: sig,
			})
			continue
		}

		if matches := pyDefRegex.FindStringSubmatch(line); matches != nil {
			sig := strings.TrimSpace(strings.Split(line, ":")[0])
			// Track public methods and init
			if !strings.HasPrefix(matches[2], "_") || matches[2] == "__init__" {
				fm.Symbols = append(fm.Symbols, Symbol{
					Name:      matches[2],
					Kind:      "func",
					Signature: sig,
				})
			}
		}
	}
}
