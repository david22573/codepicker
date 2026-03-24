package indexer

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

// DB interface allows us to talk to the repo cache without a circular dependency
type DB interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
}

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

// LoadCache pulls the indexed graph from SQLite so we skip full re-indexing
func (rm *RepoMapper) LoadCache(ctx context.Context, db DB) error {
	rows, err := db.QueryContext(ctx, "SELECT path, data FROM repo_map_cache")
	if err != nil {
		return err
	}
	defer rows.Close()

	rm.mu.Lock()
	defer rm.mu.Unlock()
	for rows.Next() {
		var path, data string
		if err := rows.Scan(&path, &data); err == nil {
			var fm FileMap
			if err := json.Unmarshal([]byte(data), &fm); err == nil {
				rm.files[path] = &fm
			}
		}
	}
	return rows.Err()
}

// SaveCache writes the current graph to SQLite in the background
func (rm *RepoMapper) SaveCache(ctx context.Context, db DB) error {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	for path, fm := range rm.files {
		data, err := json.Marshal(fm)
		if err != nil {
			continue
		}
		// INSERT OR REPLACE handles upsert safely assuming path is the primary key
		_, _ = db.ExecContext(ctx, "INSERT OR REPLACE INTO repo_map_cache (path, data) VALUES (?, ?)", path, string(data))
	}
	return nil
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

	estimatedTokens := 10 // baseline

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

		headerTokens := len(fileHeader) / 4
		if estimatedTokens+headerTokens > budgetTokens {
			sb.WriteString("  ... (truncated to fit context budget)\n")
			return sb.String()
		}
		sb.WriteString(fileHeader)
		estimatedTokens += headerTokens

		for _, sym := range fm.Symbols {
			line := fmt.Sprintf("  - %s\n", sym.Signature)
			lineTokens := len(line) / 4

			if estimatedTokens+lineTokens > budgetTokens {
				sb.WriteString("  ... (truncated to fit context budget)\n")
				return sb.String()
			}
			sb.WriteString(line)
			estimatedTokens += lineTokens
		}
	}

	return sb.String()
}
