package context

import (
	"context"
)

// SliceType defines the semantic category of a code chunk.
type SliceType string

const (
	SliceTypeFunction  SliceType = "function"
	SliceTypeStruct    SliceType = "struct"
	SliceTypeInterface SliceType = "interface"
	SliceTypeBlock     SliceType = "block"
)

// IndexStats holds summary data about the semantic index.
type IndexStats struct {
	TotalSlices int
	TotalFiles  int
}

// CodeSlice represents a semantically meaningful chunk of code.
type CodeSlice struct {
	ID        string    `json:"id"`
	FilePath  string    `json:"file_path"`
	Content   string    `json:"content"`
	StartLine int       `json:"start_line"`
	EndLine   int       `json:"end_line"`
	Language  string    `json:"language"`
	SliceType SliceType `json:"slice_type"`
	Symbols   []string  `json:"symbols"`
	Hash      string    `json:"hash"`
}

// SliceStore is the interface for persisting and retrieving code chunks.
type SliceStore interface {
	// SearchSlices retrieves relevant code chunks based on a query.
	SearchSlices(ctx context.Context, query string, limit int) ([]CodeSlice, error)

	// SaveSlices persists extracted chunks for a specific file.
	SaveSlices(ctx context.Context, filePath string, slices []CodeSlice) error

	// GetSlicesByFile retrieves all chunks belonging to a single file.
	GetSlicesByFile(ctx context.Context, filePath string) ([]CodeSlice, error)

	// IndexFile is a wrapper used by the IndexManager to save slices.
	IndexFile(filePath string, slices []CodeSlice) error

	// GetStats returns the total count of slices and files.
	GetStats() (IndexStats, error)
}
