package indexer

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/david22573/codepicker/domain/context"
)

type IndexManager struct {
	slicer *CodeSlicer
	store  context.SliceStore
}

func NewIndexManager(s *CodeSlicer, store context.SliceStore) *IndexManager {
	return &IndexManager{slicer: s, store: store}
}

// IndexDirectory scans the directory and populates the store
func (m *IndexManager) IndexDirectory(rootPath string) error {
	// 1. Resolve to absolute path for reliable walking
	absRoot, err := filepath.Abs(rootPath)
	if err != nil {
		return err
	}

	fmt.Printf("📂 Walking Directory: %s\n", absRoot)

	return filepath.Walk(absRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			fmt.Printf("  ⚠️  Access Error: %s -> %v\n", path, err)
			return nil
		}

		// 2. Skip obvious noise
		if info.IsDir() {
			name := info.Name()
			if (strings.HasPrefix(name, ".") && name != ".") || name == "vendor" || name == "node_modules" {
				return filepath.SkipDir
			}
			return nil
		}

		// 3. Match Go Source Files
		if strings.HasSuffix(info.Name(), ".go") && !strings.HasSuffix(info.Name(), "_test.go") {
			// Get relative path for consistent DB keys
			relPath, _ := filepath.Rel(absRoot, path)

			fmt.Printf("  📄 Slicing: %s\n", relPath)

			slices, err := m.slicer.SliceFile(path)
			if err != nil {
				fmt.Printf("  ❌ Slicer Error in %s: %v\n", relPath, err)
				return nil
			}

			if len(slices) == 0 {
				fmt.Printf("  ℹ️  No semantic units found in %s\n", relPath)
				return nil
			}

			// Store the results using the RELATIVE path as the key
			if err := m.store.IndexFile(relPath, slices); err != nil {
				fmt.Printf("  ❌ Database Error in %s: %v\n", relPath, err)
				return err
			}
			fmt.Printf("  ✅ Indexed %d slices\n", len(slices))
		}
		return nil
	})
}
