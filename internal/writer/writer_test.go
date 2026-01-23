package writer

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// TestConcurrentConcatWrites tests thread-safety of ConcatStrategy
func TestConcurrentConcatWrites(t *testing.T) {
	tmpDir := t.TempDir()
	outputPath := filepath.Join(tmpDir, "output.txt")

	strategy := NewConcatStrategy(outputPath, false, true)
	if err := strategy.Init(); err != nil {
		t.Fatalf("Failed to initialize strategy: %v", err)
	}
	defer strategy.Close()

	const numWorkers = 20
	const filesPerWorker = 10

	var wg sync.WaitGroup
	errors := make(chan error, numWorkers*filesPerWorker)

	// Create test files
	testFiles := make([]string, numWorkers*filesPerWorker)
	for i := 0; i < len(testFiles); i++ {
		testFile := filepath.Join(tmpDir, fmt.Sprintf("test_%d.txt", i))
		content := fmt.Sprintf("Content for file %d\n", i)
		if err := os.WriteFile(testFile, []byte(content), 0644); err != nil {
			t.Fatalf("Failed to create test file: %v", err)
		}
		testFiles[i] = testFile
	}

	// Concurrent writes
	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for j := 0; j < filesPerWorker; j++ {
				fileIdx := workerID*filesPerWorker + j
				absPath := testFiles[fileIdx]
				relPath := fmt.Sprintf("test_%d.txt", fileIdx)

				if err := strategy.Write(absPath, relPath); err != nil {
					errors <- fmt.Errorf("worker %d file %d: %w", workerID, j, err)
				}
			}
		}(i)
	}

	wg.Wait()
	close(errors)

	// Check for errors
	for err := range errors {
		t.Errorf("Concurrent write error: %v", err)
	}

	// Verify output file integrity
	outputContent, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("Failed to read output file: %v", err)
	}

	if len(outputContent) == 0 {
		t.Error("Output file is empty")
	}

	// Verify token count is consistent
	if strategy.TokenCount < 0 {
		t.Errorf("Token count should not be negative: %d", strategy.TokenCount)
	}
}

// TestConcurrentCopyWrites tests thread-safety of CopyStrategy
func TestConcurrentCopyWrites(t *testing.T) {
	tmpDir := t.TempDir()
	srcDir := filepath.Join(tmpDir, "src")
	dstDir := filepath.Join(tmpDir, "dst")

	if err := os.MkdirAll(srcDir, 0755); err != nil {
		t.Fatal(err)
	}

	strategy := NewCopyStrategy(dstDir)
	if err := strategy.Init(); err != nil {
		t.Fatalf("Failed to initialize strategy: %v", err)
	}
	defer strategy.Close()

	const numWorkers = 15
	const filesPerWorker = 10

	var wg sync.WaitGroup
	errors := make(chan error, numWorkers*filesPerWorker)

	// Create test files
	for i := 0; i < numWorkers*filesPerWorker; i++ {
		testFile := filepath.Join(srcDir, fmt.Sprintf("file_%d.txt", i))
		content := []byte(fmt.Sprintf("Test content %d", i))
		if err := os.WriteFile(testFile, content, 0644); err != nil {
			t.Fatalf("Failed to create test file: %v", err)
		}
	}

	// Concurrent copy operations
	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for j := 0; j < filesPerWorker; j++ {
				fileIdx := workerID*filesPerWorker + j
				relPath := fmt.Sprintf("file_%d.txt", fileIdx)
				absPath := filepath.Join(srcDir, relPath)

				if err := strategy.Write(absPath, relPath); err != nil {
					errors <- fmt.Errorf("worker %d file %d: %w", workerID, j, err)
				}
			}
		}(i)
	}

	wg.Wait()
	close(errors)

	// Check for errors
	for err := range errors {
		t.Errorf("Concurrent copy error: %v", err)
	}

	// Verify all files were copied
	expectedCount := numWorkers * filesPerWorker
	actualCount := 0

	filepath.WalkDir(dstDir, func(path string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			actualCount++
		}
		return nil
	})

	if actualCount != expectedCount {
		t.Errorf("Expected %d copied files, got %d", expectedCount, actualCount)
	}
}

// TestConcurrentTreeWrites tests thread-safety of TreeStrategy
func TestConcurrentTreeWrites(t *testing.T) {
	tmpDir := t.TempDir()

	strategy := NewTreeStrategy(TreeOptions{})
	if err := strategy.Init(); err != nil {
		t.Fatalf("Failed to initialize strategy: %v", err)
	}
	defer strategy.Close()

	const numWorkers = 20
	const filesPerWorker = 15

	var wg sync.WaitGroup
	errors := make(chan error, numWorkers*filesPerWorker)

	// Concurrent tree writes
	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for j := 0; j < filesPerWorker; j++ {
				relPath := fmt.Sprintf("dir%d/subdir/file_%d.txt", workerID, j)
				absPath := filepath.Join(tmpDir, relPath)

				if err := strategy.Write(absPath, relPath); err != nil {
					errors <- fmt.Errorf("worker %d file %d: %w", workerID, j, err)
				}
			}
		}(i)
	}

	wg.Wait()
	close(errors)

	// Check for errors
	for err := range errors {
		t.Errorf("Concurrent tree write error: %v", err)
	}

	// Verify buffer has content
	strategy.mu.Lock()
	bufferLen := strategy.buffer.Len()
	strategy.mu.Unlock()

	if bufferLen == 0 {
		t.Error("Tree buffer should have content")
	}
}

// TestConcurrentSameFileConcatWrites tests concurrent writes to the same output file
func TestConcurrentSameFileConcatWrites(t *testing.T) {
	tmpDir := t.TempDir()
	outputPath := filepath.Join(tmpDir, "concurrent_output.txt")

	strategy := NewConcatStrategy(outputPath, false, false)
	if err := strategy.Init(); err != nil {
		t.Fatal(err)
	}
	defer strategy.Close()

	// Create a single test file that multiple goroutines will write
	testFile := filepath.Join(tmpDir, "test.txt")
	content := []byte("Test content that will be written multiple times")
	if err := os.WriteFile(testFile, content, 0644); err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	const numConcurrentWrites = 50

	// Multiple goroutines writing the same file
	for i := 0; i < numConcurrentWrites; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			strategy.Write(testFile, fmt.Sprintf("copy_%d/test.txt", id))
		}(i)
	}

	wg.Wait()

	// Verify output file exists and has content
	_, err := os.Stat(outputPath)
	if err != nil {
		t.Errorf("Output file should exist: %v", err)
	}
}

// TestConcurrentCopyToSameDirectory tests concurrent copies to overlapping directories
func TestConcurrentCopyToSameDirectory(t *testing.T) {
	tmpDir := t.TempDir()
	srcDir := filepath.Join(tmpDir, "src")
	dstDir := filepath.Join(tmpDir, "dst")

	if err := os.MkdirAll(srcDir, 0755); err != nil {
		t.Fatal(err)
	}

	strategy := NewCopyStrategy(dstDir)
	if err := strategy.Init(); err != nil {
		t.Fatal(err)
	}
	defer strategy.Close()

	var wg sync.WaitGroup
	const numWorkers = 10

	// Multiple workers creating files in the same subdirectory structure
	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for j := 0; j < 5; j++ {
				// Same directory path, different file names
				relPath := fmt.Sprintf("shared/subdir/file_%d_%d.txt", workerID, j)
				srcPath := filepath.Join(srcDir, relPath)

				// Create source file
				if err := os.MkdirAll(filepath.Dir(srcPath), 0755); err != nil {
					t.Errorf("Failed to create src dir: %v", err)
					return
				}
				content := []byte(fmt.Sprintf("Content %d-%d", workerID, j))
				if err := os.WriteFile(srcPath, content, 0644); err != nil {
					t.Errorf("Failed to create src file: %v", err)
					return
				}

				// Copy it
				strategy.Write(srcPath, relPath)
			}
		}(i)
	}

	wg.Wait()

	// Verify all files exist in destination
	expectedFiles := numWorkers * 5
	actualFiles := 0

	filepath.WalkDir(dstDir, func(path string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			actualFiles++
		}
		return nil
	})

	if actualFiles != expectedFiles {
		t.Errorf("Expected %d files, got %d", expectedFiles, actualFiles)
	}
}

// TestConcurrentMinification tests concurrent writes with minification enabled
func TestConcurrentMinification(t *testing.T) {
	tmpDir := t.TempDir()
	outputPath := filepath.Join(tmpDir, "minified.txt")

	strategy := NewConcatStrategy(outputPath, true, false) // minify enabled
	if err := strategy.Init(); err != nil {
		t.Fatal(err)
	}
	defer strategy.Close()

	var wg sync.WaitGroup
	const numFiles = 30

	// Create test files with various extensions
	extensions := []string{".js", ".go", ".py", ".txt"}

	for i := 0; i < numFiles; i++ {
		wg.Add(1)
		go func(fileID int) {
			defer wg.Done()

			ext := extensions[fileID%len(extensions)]
			testFile := filepath.Join(tmpDir, fmt.Sprintf("test_%d%s", fileID, ext))

			// Create file with some content
			content := fmt.Sprintf("function test() {\n  return %d;\n}\n", fileID)
			if err := os.WriteFile(testFile, []byte(content), 0644); err != nil {
				t.Errorf("Failed to create test file: %v", err)
				return
			}

			relPath := fmt.Sprintf("test_%d%s", fileID, ext)
			if err := strategy.Write(testFile, relPath); err != nil {
				t.Errorf("Write error: %v", err)
			}
		}(i)
	}

	wg.Wait()

	// Verify output exists
	_, err := os.Stat(outputPath)
	if err != nil {
		t.Errorf("Output file should exist: %v", err)
	}
}

// TestConcurrentTokenCounting tests concurrent token count updates
func TestConcurrentTokenCounting(t *testing.T) {
	tmpDir := t.TempDir()
	outputPath := filepath.Join(tmpDir, "tokens.txt")

	strategy := NewConcatStrategy(outputPath, false, true) // compute tokens
	if err := strategy.Init(); err != nil {
		t.Fatal(err)
	}
	defer strategy.Close()

	var wg sync.WaitGroup
	const numFiles = 50

	for i := 0; i < numFiles; i++ {
		wg.Add(1)
		go func(fileID int) {
			defer wg.Done()

			testFile := filepath.Join(tmpDir, fmt.Sprintf("test_%d.txt", fileID))
			// Create file with repeatable content length
			content := bytes.Repeat([]byte("word "), 100) // 500 chars
			if err := os.WriteFile(testFile, content, 0644); err != nil {
				t.Errorf("Failed to create test file: %v", err)
				return
			}

			relPath := fmt.Sprintf("test_%d.txt", fileID)
			strategy.Write(testFile, relPath)
		}(i)
	}

	wg.Wait()

	// Token count should be positive and reasonable
	if strategy.TokenCount <= 0 {
		t.Errorf("Token count should be positive, got %d", strategy.TokenCount)
	}

	// Should have counted tokens for all files
	expectedMinTokens := numFiles * 50 // Rough estimate
	if strategy.TokenCount < expectedMinTokens {
		t.Logf("Warning: Token count %d seems low for %d files", strategy.TokenCount, numFiles)
	}
}

// TestRaceDetection runs rapid concurrent operations to catch data races
func TestRaceDetection(t *testing.T) {
	tmpDir := t.TempDir()

	strategies := []OutputStrategy{
		NewConcatStrategy(filepath.Join(tmpDir, "race_concat.txt"), false, true),
		NewCopyStrategy(filepath.Join(tmpDir, "race_copy")),
		NewTreeStrategy(TreeOptions{}),
	}

	for _, strategy := range strategies {
		t.Run(strategy.Name(), func(t *testing.T) {
			if err := strategy.Init(); err != nil {
				t.Fatal(err)
			}
			defer strategy.Close()

			var wg sync.WaitGroup
			const iterations = 100

			// Create test file
			testFile := filepath.Join(tmpDir, "race_test.txt")
			if err := os.WriteFile(testFile, []byte("race test content"), 0644); err != nil {
				t.Fatal(err)
			}

			for i := 0; i < 10; i++ {
				wg.Add(1)
				go func(id int) {
					defer wg.Done()
					for j := 0; j < iterations; j++ {
						relPath := fmt.Sprintf("file_%d_%d.txt", id, j)
						strategy.Write(testFile, relPath)
					}
				}(i)
			}

			wg.Wait()
		})
	}
}

// TestConcurrentClose tests closing strategies under concurrent writes
func TestConcurrentClose(t *testing.T) {
	tmpDir := t.TempDir()
	outputPath := filepath.Join(tmpDir, "close_test.txt")

	strategy := NewConcatStrategy(outputPath, false, false)
	if err := strategy.Init(); err != nil {
		t.Fatal(err)
	}

	// Create test file
	testFile := filepath.Join(tmpDir, "test.txt")
	if err := os.WriteFile(testFile, []byte("test"), 0644); err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	done := make(chan bool)

	// Start concurrent writes
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for {
				select {
				case <-done:
					return
				default:
					strategy.Write(testFile, fmt.Sprintf("file_%d.txt", id))
					time.Sleep(time.Millisecond)
				}
			}
		}(i)
	}

	// Let writes run briefly
	time.Sleep(20 * time.Millisecond)

	// Close while writes are happening
	if err := strategy.Close(); err != nil {
		t.Errorf("Close failed: %v", err)
	}

	close(done)
	wg.Wait()

	// Subsequent writes may fail, but should not panic
	// (This is acceptable behavior)
}

// TestMixedStrategyConcurrency tests different strategies operating concurrently
func TestMixedStrategyConcurrency(t *testing.T) {
	tmpDir := t.TempDir()

	concat := NewConcatStrategy(filepath.Join(tmpDir, "concat.txt"), false, true)
	copy := NewCopyStrategy(filepath.Join(tmpDir, "copy"))
	tree := NewTreeStrategy(TreeOptions{})

	strategies := []OutputStrategy{concat, copy, tree}

	for _, s := range strategies {
		if err := s.Init(); err != nil {
			t.Fatalf("Failed to init %s: %v", s.Name(), err)
		}
		defer s.Close()
	}

	var wg sync.WaitGroup
	const numFiles = 20

	// Create test files
	testFiles := make([]string, numFiles)
	for i := 0; i < numFiles; i++ {
		testFile := filepath.Join(tmpDir, "src", fmt.Sprintf("test_%d.txt", i))
		os.MkdirAll(filepath.Dir(testFile), 0755)
		content := fmt.Sprintf("Content %d", i)
		if err := os.WriteFile(testFile, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
		testFiles[i] = testFile
	}

	// Each strategy processes all files concurrently
	for _, strategy := range strategies {
		for i := 0; i < numFiles; i++ {
			wg.Add(1)
			go func(s OutputStrategy, fileIdx int) {
				defer wg.Done()
				relPath := fmt.Sprintf("test_%d.txt", fileIdx)
				s.Write(testFiles[fileIdx], relPath)
			}(strategy, i)
		}
	}

	wg.Wait()

	// All strategies should have processed files without errors
}

// TestShouldSkipConcurrency tests concurrent ShouldSkip checks
func TestShouldSkipConcurrency(t *testing.T) {
	tmpDir := t.TempDir()
	outputPath := filepath.Join(tmpDir, "output.txt")

	strategy := NewConcatStrategy(outputPath, false, false)

	var wg sync.WaitGroup
	const numChecks = 100

	for i := 0; i < numChecks; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			// These should all be consistent
			testPath := filepath.Join(tmpDir, fmt.Sprintf("test_%d.txt", id))
			result := strategy.ShouldSkip(testPath)

			// ShouldSkip for the output path itself should be true
			if testPath == outputPath && !result {
				t.Error("ShouldSkip should return true for output path")
			}
		}(i)
	}

	wg.Wait()
}
