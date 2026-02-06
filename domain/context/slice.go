package context

import "time"

// SliceType defines the semantic category of a code chunk
type SliceType string

const (
	SliceTypeFunction  SliceType = "function"
	SliceTypeStruct    SliceType = "struct"
	SliceTypeInterface SliceType = "interface"
	SliceTypeImport    SliceType = "import"
	SliceTypeComment   SliceType = "comment"
	SliceTypeBlock     SliceType = "block"
)

// CodeSlice represents a semantic chunk of code extracted from a file
type CodeSlice struct {
	ID           string            `json:"id"`
	FilePath     string            `json:"file_path"`
	StartLine    int               `json:"start_line"`
	EndLine      int               `json:"end_line"`
	Content      string            `json:"content"`
	Language     string            `json:"language"`
	SliceType    SliceType         `json:"slice_type"`
	Metadata     map[string]string `json:"metadata"`
	Symbols      []string          `json:"symbols"`      // function names, type names, etc.
	Dependencies []string          `json:"dependencies"` // imported packages
	Hash         string            `json:"hash"`         // content hash for cache invalidation
	IndexedAt    time.Time         `json:"indexed_at"`
}

// SliceQuery defines search parameters for retrieving relevant code
type SliceQuery struct {
	Keywords   []string
	FilePath   string
	SliceTypes []SliceType
	Symbols    []string
	MaxResults int
}

// SliceStore defines the interface for persisting and querying code slices
type SliceStore interface {
	// Indexing
	IndexFile(filePath string, slices []CodeSlice) error

	// Querying
	Query(query SliceQuery) ([]CodeSlice, error)
	GetByID(id string) (*CodeSlice, error)
	GetByFile(filePath string) ([]CodeSlice, error)
	GetBySymbol(symbol string) ([]CodeSlice, error)

	// Maintenance
	InvalidateFile(filePath string) error
	GetStats() (*IndexStats, error)
}

// IndexStats provides insights into the health of the code index
type IndexStats struct {
	TotalSlices   int
	TotalFiles    int
	LastIndexedAt time.Time
	CacheHitRate  float64
}
