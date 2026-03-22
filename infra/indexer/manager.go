package indexer

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

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

	// Phase 1.4: Repo Map Caching
	SaveRepoMapCache(ctx context.Context, filePath string, data string, modTime time.Time) error
	GetRepoMapCache(ctx context.Context) (map[string]string, map[string]time.Time, error)
	DeleteRepoMapCache(ctx context.Context, filePath string) error
}

func NewIndexManager(s *CodeSlicer, store ContextRepository, embedder *llm.EmbeddingClient) *IndexManager {
	return &IndexManager{slicer: s, store: store, embedder: embedder}
}

// SyncRepoMap incrementally builds or loads the sparse project map.
func (m *IndexManager) SyncRepoMap(ctx context.Context, rootPath string, mapper *RepoMapper) error {
	absRoot, err := filepath.Abs(rootPath)
	if err != nil {
		return err
	}

	dataMap, timeMap, err := m.store.GetRepoMapCache(ctx)
	if err != nil {
		return fmt.Errorf("failed to load repo map cache: %w", err)
	}

	seenFiles := make(map[string]bool)
	allowedExts := map[string]bool{
		".go": true, ".py": true, ".ts": true, ".tsx": true,
		".js": true, ".jsx": true, ".svelte": true,
	}

	err = filepath.Walk(absRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}

		if info.IsDir() {
			name := info.Name()
			if (strings.HasPrefix(name, ".") && name != ".") || name == "vendor" || name == "node_modules" || name == "dist" {
				return filepath.SkipDir
			}
			return nil
		}

		ext := strings.ToLower(filepath.Ext(path))
		if !allowedExts[ext] {
			return nil
		}

		relPath, _ := filepath.Rel(absRoot, path)
		seenFiles[relPath] = true
		diskModTime := info.ModTime()

		cachedTime, timeExists := timeMap[relPath]
		cachedData, dataExists := dataMap[relPath]

		// Cache Hit
		if timeExists && dataExists && diskModTime.Equal(cachedTime) {
			var fm FileMap
			if err := json.Unmarshal([]byte(cachedData), &fm); err == nil {
				mapper.mu.Lock()
				mapper.files[path] = &fm
				mapper.mu.Unlock()
				return nil
			}
		}

		// Cache Miss or File Modified: Re-parse
		if err := mapper.ParseFile(ctx, path); err != nil {
			return nil // Skip on parse failure
		}

		mapper.mu.RLock()
		fm, ok := mapper.files[path]
		mapper.mu.RUnlock()

		if ok {
			fm.Path = relPath // Normalize to relative paths for cache stability
			if jsonData, err := json.Marshal(fm); err == nil {
				_ = m.store.SaveRepoMapCache(ctx, relPath, string(jsonData), diskModTime)
			}
			fm.Path = path // Restore absolute path for the active runtime map
		}

		return nil
	})

	if err != nil {
		return err
	}

	// Cleanup deleted files from cache
	for cachedPath := range timeMap {
		if !seenFiles[cachedPath] {
			_ = m.store.DeleteRepoMapCache(ctx, cachedPath)

			absDeletedPath := filepath.Join(absRoot, cachedPath)
			mapper.mu.Lock()
			delete(mapper.files, absDeletedPath)
			mapper.mu.Unlock()
		}
	}

	return nil
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

	for w := 0; w < workerCount; w++ {
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
			if existingHash, ok := existingHashMap[s.ID]; ok && existingHash == s.Hash {
				continue
			}

			contents = append(contents, fmt.Sprintf("File: %s\nSymbol: %v\nCode:\n%s", s.FilePath, s.Symbols, s.Content))
			ids = append(ids, s.ID)
		}

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
