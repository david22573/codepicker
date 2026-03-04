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

type ContextRepository interface {
	domainCtx.SliceStore
	UpdateSliceEmbedding(ctx context.Context, sliceID string, embedding []float32) error
	GetSliceByID(ctx context.Context, id string) (*domainCtx.CodeSlice, error)
	SearchByVector(ctx context.Context, queryVector []float32, limit int) ([]domainCtx.CodeSlice, error)
}

func NewIndexManager(s *CodeSlicer, store ContextRepository, embedder *llm.EmbeddingClient) *IndexManager {
	return &IndexManager{slicer: s, store: store, embedder: embedder}
}

func (m *IndexManager) IndexDirectory(rootPath string) error {
	absRoot, err := filepath.Abs(rootPath)
	if err != nil {
		return err
	}

	fmt.Printf("📂 Walking Directory: %s\n", absRoot)

	workerCount := runtime.NumCPU() * 2
	jobs := make(chan string, 100)
	var wg sync.WaitGroup

	for w := range workerCount {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			m.worker(id, absRoot, jobs)
		}(w)
	}

	err = filepath.Walk(absRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}

		if info.IsDir() {
			name := info.Name()
			if (strings.HasPrefix(name, ".") && name != ".") || name == "vendor" || name == "node_modules" {
				return filepath.SkipDir
			}
			return nil
		}

		if strings.HasSuffix(info.Name(), ".go") && !strings.HasSuffix(info.Name(), "_test.go") {
			jobs <- path
		}
		return nil
	})

	close(jobs)
	wg.Wait()

	return err
}

func (m *IndexManager) worker(id int, rootPath string, jobs <-chan string) {
	ctx := context.Background()

	for path := range jobs {
		relPath, _ := filepath.Rel(rootPath, path)

		slices, err := m.slicer.SliceFile(path)
		if err != nil {
			fmt.Printf("[Worker %d] ❌ Slice failed for %s: %v\n", id, relPath, err)
			continue
		}
		if len(slices) == 0 {
			continue
		}

		fmt.Printf("[Worker %d] 📄 Processing %s (%d slices)\n", id, relPath, len(slices))

		// Check existing slices from DB to avoid wasteful re-embedding
		existingSlices, _ := m.store.GetSlicesByFile(ctx, relPath)
		existingHashMap := make(map[string]string)
		for _, existing := range existingSlices {
			existingHashMap[existing.ID] = existing.Hash
		}

		if err := m.store.IndexFile(relPath, slices); err != nil {
			fmt.Printf("[Worker %d] ❌ Save failed for %s: %v\n", id, relPath, err)
			continue
		}

		var contents []string
		var ids []string

		for _, s := range slices {
			// If hash hasn't changed, skip embedding call
			if existingHash, ok := existingHashMap[s.ID]; ok && existingHash == s.Hash {
				continue
			}

			contents = append(contents, fmt.Sprintf("File: %s\nSymbol: %v\nCode:\n%s", s.FilePath, s.Symbols, s.Content))
			ids = append(ids, s.ID)
		}

		// Batch call to OpenAI/OpenRouter ONLY if there are new/changed slices
		if len(contents) > 0 {
			vectors, _, err := m.embedder.CreateEmbeddings(ctx, contents)
			if err != nil {
				fmt.Printf("[Worker %d] ⚠️  Embedding failed for %s: %v\n", id, relPath, err)
				continue
			}

			for i, vec := range vectors {
				if err := m.store.UpdateSliceEmbedding(ctx, ids[i], vec); err != nil {
					fmt.Printf("[Worker %d] ⚠️  Failed to save vector: %v\n", id, err)
				}
			}
		}
	}
}
