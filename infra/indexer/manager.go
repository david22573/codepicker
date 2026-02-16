package indexer

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	domainCtx "github.com/david22573/codepicker/domain/context"
	"github.com/david22573/codepicker/infra/llm"
)

type IndexManager struct {
	slicer   *CodeSlicer
	store    ContextRepository
	embedder *llm.EmbeddingClient
}

// ContextRepository interface exposing necessary methods for RAG
type ContextRepository interface {
	domainCtx.SliceStore
	UpdateSliceEmbedding(ctx context.Context, sliceID string, embedding []float32) error
	GetSliceByID(ctx context.Context, id string) (*domainCtx.CodeSlice, error)
	SearchByVector(ctx context.Context, queryVector []float32, limit int) ([]domainCtx.CodeSlice, error)
}

func NewIndexManager(s *CodeSlicer, store ContextRepository, embedder *llm.EmbeddingClient) *IndexManager {
	return &IndexManager{slicer: s, store: store, embedder: embedder}
}

// IndexDirectory scans, slices, AND embeds the codebase using a concurrent Worker Pool.
func (m *IndexManager) IndexDirectory(rootPath string) error {
	absRoot, err := filepath.Abs(rootPath)
	if err != nil {
		return err
	}

	fmt.Printf("📂 Walking Directory: %s\n", absRoot)

	// 1. Setup Worker Pool
	// We use NumCPU * 2 workers to keep the CPU busy while waiting for IO/Network
	workerCount := runtime.NumCPU() * 2
	jobs := make(chan string, 100)
	var wg sync.WaitGroup

	// Start Workers
	for w := range workerCount {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			m.worker(id, absRoot, jobs)
		}(w)
	}

	// 2. Walk and Feed Jobs
	err = filepath.Walk(absRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}

		if info.IsDir() {
			name := info.Name()
			// Skip hidden dirs, vendor, node_modules
			if (strings.HasPrefix(name, ".") && name != ".") || name == "vendor" || name == "node_modules" {
				return filepath.SkipDir
			}
			return nil
		}

		// Filter for Go files (or other supported languages)
		if strings.HasSuffix(info.Name(), ".go") && !strings.HasSuffix(info.Name(), "_test.go") {
			jobs <- path // Send file path to workers
		}
		return nil
	})

	close(jobs) // Signal no more work
	wg.Wait()   // Wait for all workers to finish

	return err
}

// worker handles the processing pipeline for a single file
func (m *IndexManager) worker(id int, rootPath string, jobs <-chan string) {
	ctx := context.Background()

	for path := range jobs {
		relPath, _ := filepath.Rel(rootPath, path)

		// 1. Slice
		slices, err := m.slicer.SliceFile(path)
		if err != nil {
			fmt.Printf("[Worker %d] ❌ Slice failed for %s: %v\n", id, relPath, err)
			continue
		}
		if len(slices) == 0 {
			continue
		}

		fmt.Printf("[Worker %d] 📄 Processing %s (%d slices)\n", id, relPath, len(slices))

		// 2. Save Slices (clears old embeddings for this file)
		if err := m.store.IndexFile(relPath, slices); err != nil {
			fmt.Printf("[Worker %d] ❌ Save failed for %s: %v\n", id, relPath, err)
			continue
		}

		// 3. Generate Embeddings (RAG)
		var contents []string
		var ids []string

		for _, s := range slices {
			// We embed: FilePath + Symbol + Content for rich context
			contents = append(contents, fmt.Sprintf("File: %s\nSymbol: %v\nCode:\n%s", s.FilePath, s.Symbols, s.Content))
			ids = append(ids, s.ID)
		}

		// Batch call to OpenAI/OpenRouter
		// Note: The embedding client handles its own timeouts, but we are running in parallel now.
		vectors, _, err := m.embedder.CreateEmbeddings(ctx, contents)
		if err != nil {
			fmt.Printf("[Worker %d] ⚠️  Embedding failed for %s: %v\n", id, relPath, err)
			continue
		}

		// 4. Save Vectors
		for i, vec := range vectors {
			if err := m.store.UpdateSliceEmbedding(ctx, ids[i], vec); err != nil {
				fmt.Printf("[Worker %d] ⚠️  Failed to save vector: %v\n", id, err)
			}
		}
	}
}
