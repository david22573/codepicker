package shadow

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestConcurrentWriteFile(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "shadow-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	m, err := NewManager(tmpDir)
	if err != nil {
		t.Fatal(err)
	}

	// Test concurrent writes to different files
	var wg sync.WaitGroup
	numGoroutines := 10
	errChan := make(chan error, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			path := fmt.Sprintf("file%d.txt", id)
			content := []byte(fmt.Sprintf("content from goroutine %d", id))
			_, err := m.WriteFile(path, content)
			if err != nil {
				errChan <- err
			}
		}(i)
	}

	wg.Wait()
	close(errChan)

	for err := range errChan {
		t.Errorf("concurrent write failed: %v", err)
	}

	// Verify all files were written
	files, err := m.ListShadowFiles()
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != numGoroutines {
		t.Errorf("expected %d files, got %d", numGoroutines, len(files))
	}
}

func TestConcurrentWriteToSameFile(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "shadow-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	m, err := NewManager(tmpDir)
	if err != nil {
		t.Fatal(err)
	}

	// Test concurrent writes to the same file
	var wg sync.WaitGroup
	numGoroutines := 20
	filename := "concurrent.txt"

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			content := []byte(fmt.Sprintf("write attempt %d at %v", id, time.Now()))
			m.WriteFile(filename, content)
		}(i)
	}

	wg.Wait()

	// File should exist and contain content from one of the writes
	shadowPath := filepath.Join(m.ShadowRoot, filename)
	content, err := os.ReadFile(shadowPath)
	if err != nil {
		t.Fatalf("file should exist: %v", err)
	}
	if len(content) == 0 {
		t.Error("file should have content")
	}
}

func TestConcurrentRecordAttribution(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "shadow-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	m, err := NewManager(tmpDir)
	if err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	numGoroutines := 50

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			path := fmt.Sprintf("file%d.go", id)
			agent := fmt.Sprintf("agent-%d", id%5)
			task := fmt.Sprintf("task-%d", id)
			err := m.RecordAttribution(path, agent, task)
			if err != nil {
				t.Errorf("RecordAttribution failed: %v", err)
			}
		}(i)
	}

	wg.Wait()

	// Verify all attributions were recorded
	changes := m.GetManifestChanges()
	if len(changes) != numGoroutines {
		t.Errorf("expected %d attributions, got %d", numGoroutines, len(changes))
	}
}

func TestGetManifestChanges(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "shadow-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	m, err := NewManager(tmpDir)
	if err != nil {
		t.Fatal(err)
	}

	// Record some attributions
	m.RecordAttribution("file1.go", "agent1", "task1")
	m.RecordAttribution("file2.go", "agent2", "task2")

	// Get changes (should return a copy)
	changes1 := m.GetManifestChanges()
	if len(changes1) != 2 {
		t.Errorf("expected 2 changes, got %d", len(changes1))
	}

	// Modify the returned map (shouldn't affect internal state)
	changes1["file3.go"] = ChangeMeta{File: "file3.go", Agent: "agent3", Task: "task3"}

	// Get changes again
	changes2 := m.GetManifestChanges()
	if len(changes2) != 2 {
		t.Errorf("expected 2 changes after external modification, got %d", len(changes2))
	}

	// Verify file3 is not in the manager
	if _, exists := changes2["file3.go"]; exists {
		t.Error("external modification should not affect internal state")
	}
}

func TestConcurrentGetManifestChanges(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "shadow-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	m, err := NewManager(tmpDir)
	if err != nil {
		t.Fatal(err)
	}

	// Add some initial data
	for i := 0; i < 10; i++ {
		m.RecordAttribution(fmt.Sprintf("file%d.go", i), "agent", "task")
	}

	var wg sync.WaitGroup
	numReaders := 20
	numWriters := 5

	// Concurrent readers
	for i := 0; i < numReaders; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < 10; j++ {
				changes := m.GetManifestChanges()
				if len(changes) < 10 {
					// Should have at least the initial 10 entries
					t.Errorf("reader %d iteration %d: expected at least 10 changes, got %d", id, j, len(changes))
				}
				time.Sleep(1 * time.Millisecond)
			}
		}(i)
	}

	// Concurrent writers
	for i := 0; i < numWriters; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < 5; j++ {
				path := fmt.Sprintf("new-file-%d-%d.go", id, j)
				m.RecordAttribution(path, fmt.Sprintf("agent-%d", id), fmt.Sprintf("task-%d", j))
				time.Sleep(2 * time.Millisecond)
			}
		}(i)
	}

	wg.Wait()

	// Verify final state
	finalChanges := m.GetManifestChanges()
	expectedMin := 10 + (numWriters * 5)
	if len(finalChanges) < expectedMin {
		t.Errorf("expected at least %d changes, got %d", expectedMin, len(finalChanges))
	}
}

func TestConcurrentManifestOperations(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "shadow-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	m, err := NewManager(tmpDir)
	if err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	numOperations := 30

	// Mix of writes and reads
	for i := 0; i < numOperations; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			if id%2 == 0 {
				// Write operation
				m.RecordAttribution(fmt.Sprintf("file%d.go", id), "test-agent", "test-task")
			} else {
				// Read operation
				m.LoadManifest()
			}
		}(i)
	}

	wg.Wait()

	// Should have at least some entries without panicking
	changes := m.GetManifestChanges()
	if len(changes) == 0 {
		t.Error("expected some manifest changes")
	}
}

func TestConcurrentApplyAndRestore(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "shadow-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	m, err := NewManager(tmpDir)
	if err != nil {
		t.Fatal(err)
	}

	// Create a real file first
	testFile := "test.txt"
	realPath := filepath.Join(tmpDir, testFile)
	err = os.WriteFile(realPath, []byte("original content"), 0644)
	if err != nil {
		t.Fatal(err)
	}

	// Write to shadow
	shadowContent := []byte("shadow content")
	_, err = m.WriteFile(testFile, shadowContent)
	if err != nil {
		t.Fatal(err)
	}

	// Test concurrent applies (should be serialized by mutex)
	var wg sync.WaitGroup
	numAttempts := 5
	results := make(chan string, numAttempts)

	for i := 0; i < numAttempts; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			backupPath, err := m.ApplyAtomic(testFile)
			if err != nil {
				t.Errorf("ApplyAtomic failed: %v", err)
				return
			}
			results <- backupPath
		}()
	}

	wg.Wait()
	close(results)

	// Verify real file has shadow content
	content, err := os.ReadFile(realPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != string(shadowContent) {
		t.Errorf("expected %q, got %q", shadowContent, content)
	}

	// All backup paths should be non-empty
	count := 0
	for backupPath := range results {
		if backupPath != "" {
			count++
		}
	}
	if count == 0 {
		t.Error("expected at least one backup path")
	}
}

func TestConcurrentListAndWrite(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "shadow-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	m, err := NewManager(tmpDir)
	if err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	done := make(chan bool)

	// Writer goroutines
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < 5; j++ {
				select {
				case <-done:
					return
				default:
					path := fmt.Sprintf("file%d-%d.txt", id, j)
					m.WriteFile(path, []byte(fmt.Sprintf("content %d-%d", id, j)))
					time.Sleep(10 * time.Millisecond)
				}
			}
		}(i)
	}

	// Reader goroutines
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 10; j++ {
				select {
				case <-done:
					return
				default:
					_, err := m.ListShadowFiles()
					if err != nil {
						t.Errorf("ListShadowFiles failed: %v", err)
					}
					time.Sleep(15 * time.Millisecond)
				}
			}
		}()
	}

	wg.Wait()
	close(done)

	// Final check - should have all files
	files, err := m.ListShadowFiles()
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 50 {
		t.Logf("expected 50 files, got %d", len(files))
	}
}

func TestInvalidPathProtection(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "shadow-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	m, err := NewManager(tmpDir)
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		path      string
		shouldErr bool
	}{
		{"../etc/passwd", true},
		{"/etc/passwd", true},
		{"valid/path.txt", false},
		{"../../escape", true},
	}

	for _, tt := range tests {
		_, err := m.WriteFile(tt.path, []byte("test"))
		if tt.shouldErr && err == nil {
			t.Errorf("expected error for path %q but got none", tt.path)
		}
		if !tt.shouldErr && err != nil {
			t.Errorf("unexpected error for path %q: %v", tt.path, err)
		}
	}
}

func TestMaxSizeLimit(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "shadow-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	m, err := NewManager(tmpDir)
	if err != nil {
		t.Fatal(err)
	}

	// Create content larger than MaxShadowSize
	largeContent := make([]byte, MaxShadowSize+1)
	for i := range largeContent {
		largeContent[i] = 'A'
	}

	_, err = m.WriteFile("large.txt", largeContent)
	if err == nil {
		t.Error("expected error for content exceeding max size")
	}

	// Content at exactly the limit should work
	exactContent := make([]byte, MaxShadowSize)
	_, err = m.WriteFile("exact.txt", exactContent)
	if err != nil {
		t.Errorf("unexpected error for content at max size: %v", err)
	}
}
