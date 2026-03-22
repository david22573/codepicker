package indexer

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

// Symbol represents a recognized code construct.
type Symbol struct {
	Name      string
	Kind      string
	Signature string
	Lines     int
}

// FileMap represents the exported structure of a single file.
type FileMap struct {
	Path    string
	Package string
	Imports []string
	Symbols []Symbol
}

// RepoMapper builds and maintains a symbol graph of the repository.
type RepoMapper struct {
	mu    sync.RWMutex
	files map[string]*FileMap
}

func NewRepoMapper() *RepoMapper {
	return &RepoMapper{
		files: make(map[string]*FileMap),
	}
}

// ParseFile reads a file and routes it to the native language parser.
func (rm *RepoMapper) ParseFile(ctx context.Context, path string) error {
	content, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	ext := strings.ToLower(filepath.Ext(path))
	fileMap := &FileMap{
		Path:    path,
		Symbols: make([]Symbol, 0),
		Imports: make([]string, 0),
	}

	switch ext {
	case ".go":
		parseGo(content, fileMap)
	case ".py":
		parsePython(content, fileMap)
	case ".ts", ".tsx", ".js", ".jsx", ".svelte":
		parseTypeScript(content, fileMap)
	default:
		// Unsupported languages return an empty map, safely skipped later.
		return nil
	}

	rm.mu.Lock()
	rm.files[path] = fileMap
	rm.mu.Unlock()

	return nil
}

// RenderMap creates a sparse string representation of the repo fitting within the budget.
func (rm *RepoMapper) RenderMap(budgetTokens int) string {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	var paths []string
	for p := range rm.files {
		paths = append(paths, p)
	}
	sort.Strings(paths)

	var sb strings.Builder
	sb.WriteString("### REPOSITORY MAP\n")

	estimatedTokens := 10

	for _, p := range paths {
		fm := rm.files[p]
		if len(fm.Symbols) == 0 {
			continue
		}

		var fileHeader string
		if fm.Package != "" {
			fileHeader = fmt.Sprintf("\n%s (package %s):\n", p, fm.Package)
		} else {
			fileHeader = fmt.Sprintf("\n%s:\n", p)
		}

		sb.WriteString(fileHeader)
		estimatedTokens += 10

		for _, sym := range fm.Symbols {
			line := fmt.Sprintf("  - %s\n", sym.Signature)
			if estimatedTokens+7 > budgetTokens {
				sb.WriteString("  ... (truncated to fit context budget)\n")
				return sb.String()
			}
			sb.WriteString(line)
			estimatedTokens += 7
		}
	}

	return sb.String()
}
