package database

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/david22573/codepicker/pkg/openrouter"
)

func setupTestStore(t *testing.T) (*Store, func()) {
	tmpDir := t.TempDir()
	store, err := New(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create test store: %v", err)
	}

	cleanup := func() {
		store.Close()
	}

	return store, cleanup
}

// TestConcurrentAddMessage tests thread-safety of AddMessage
func TestConcurrentAddMessage(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	const numWorkers = 10
	const messagesPerWorker = 20

	var wg sync.WaitGroup
	errors := make(chan error, numWorkers*messagesPerWorker)

	// Launch concurrent writers
	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for j := 0; j < messagesPerWorker; j++ {
				content := fmt.Sprintf("Message from worker %d, iteration %d", workerID, j)
				role := "user"
				if j%2 == 0 {
					role = "assistant"
				}
				if err := store.AddMessage(role, content); err != nil {
					errors <- fmt.Errorf("worker %d: %w", workerID, err)
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

	// Verify total count
	messages, err := store.GetContextAwareHistory(1000000)
	if err != nil {
		t.Fatalf("Failed to get history: %v", err)
	}

	expectedCount := numWorkers * messagesPerWorker
	if len(messages) != expectedCount {
		t.Errorf("Expected %d messages, got %d", expectedCount, len(messages))
	}
}

// TestConcurrentMemoryOperations tests thread-safety of working memory operations
func TestConcurrentMemoryOperations(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	const numWorkers = 10
	const operationsPerWorker = 20

	var wg sync.WaitGroup
	errors := make(chan error, numWorkers*operationsPerWorker)

	// Launch concurrent workers doing mixed operations
	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for j := 0; j < operationsPerWorker; j++ {
				path := fmt.Sprintf("file_%d.txt", workerID)
				content := fmt.Sprintf("Content from worker %d, iteration %d", workerID, j)

				// Update memory
				if err := store.UpdateWorkingMemory(path, content); err != nil {
					errors <- fmt.Errorf("worker %d update: %w", workerID, err)
					continue
				}

				// Read memory
				if _, _, err := store.GetWorkingMemory(); err != nil {
					errors <- fmt.Errorf("worker %d read: %w", workerID, err)
					continue
				}

				// List files
				if _, err := store.ListMemoryFiles(); err != nil {
					errors <- fmt.Errorf("worker %d list: %w", workerID, err)
					continue
				}
			}
		}(i)
	}

	wg.Wait()
	close(errors)

	// Check for errors
	for err := range errors {
		t.Errorf("Concurrent memory operation error: %v", err)
	}

	// Verify final state
	files, err := store.ListMemoryFiles()
	if err != nil {
		t.Fatalf("Failed to list files: %v", err)
	}

	if len(files) != numWorkers {
		t.Errorf("Expected %d files, got %d", numWorkers, len(files))
	}
}

// TestConcurrentSnapshotOperations tests thread-safety of snapshot/restore
func TestConcurrentSnapshotOperations(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	// Prepare initial data
	for i := 0; i < 10; i++ {
		path := fmt.Sprintf("initial_%d.txt", i)
		content := fmt.Sprintf("Initial content %d", i)
		if err := store.UpdateWorkingMemory(path, content); err != nil {
			t.Fatalf("Failed to setup initial data: %v", err)
		}
	}

	const numWorkers = 5
	var wg sync.WaitGroup
	errors := make(chan error, numWorkers*2)

	// Launch concurrent snapshot creators and restorers
	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()

			// Create snapshot
			snap, err := store.CreateSnapshot()
			if err != nil {
				errors <- fmt.Errorf("worker %d create snapshot: %w", workerID, err)
				return
			}

			// Small delay to create interleaving
			time.Sleep(time.Millisecond * 10)

			// Restore snapshot (note: this modifies the database)
			if err := store.RestoreSnapshot(snap); err != nil {
				errors <- fmt.Errorf("worker %d restore snapshot: %w", workerID, err)
			}
		}(i)
	}

	wg.Wait()
	close(errors)

	// Check for errors
	for err := range errors {
		t.Errorf("Concurrent snapshot error: %v", err)
	}

	// Verify data integrity - should have the initial files
	files, err := store.ListMemoryFiles()
	if err != nil {
		t.Fatalf("Failed to list files after snapshots: %v", err)
	}

	if len(files) != 10 {
		t.Errorf("Expected 10 files after concurrent snapshots, got %d", len(files))
	}
}

// TestConcurrentPlanOperations tests thread-safety of plan operations
func TestConcurrentPlanOperations(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	const numPlans = 20
	var wg sync.WaitGroup
	errors := make(chan error, numPlans*3)

	// Create plans concurrently
	for i := 0; i < numPlans; i++ {
		wg.Add(1)
		go func(planID int) {
			defer wg.Done()

			id := fmt.Sprintf("plan_%d", planID)
			task := fmt.Sprintf("Task %d", planID)
			steps := []string{"step1", "step2", "step3"}

			// Save plan
			if err := store.SavePlan(id, task, steps, 0.5); err != nil {
				errors <- fmt.Errorf("plan %d save: %w", planID, err)
				return
			}

			// Read plan
			if _, err := store.GetPlan(id); err != nil {
				errors <- fmt.Errorf("plan %d get: %w", planID, err)
				return
			}

			// Update plan status
			if err := store.UpdatePlanStatus(id, "completed"); err != nil {
				errors <- fmt.Errorf("plan %d update: %w", planID, err)
			}
		}(i)
	}

	wg.Wait()
	close(errors)

	// Check for errors
	for err := range errors {
		t.Errorf("Concurrent plan operation error: %v", err)
	}

	// Verify all plans exist
	for i := 0; i < numPlans; i++ {
		id := fmt.Sprintf("plan_%d", i)
		plan, err := store.GetPlan(id)
		if err != nil {
			t.Errorf("Failed to retrieve plan %s: %v", id, err)
			continue
		}
		if plan.Status != "completed" {
			t.Errorf("Plan %s has status %s, expected 'completed'", id, plan.Status)
		}
	}
}

// TestMixedConcurrentOperations tests all database operations running concurrently
func TestMixedConcurrentOperations(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	const numWorkers = 5
	const operationsPerWorker = 10

	var wg sync.WaitGroup
	errors := make(chan error, numWorkers*operationsPerWorker*4)

	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()

			for j := 0; j < operationsPerWorker; j++ {
				// Add message
				if err := store.AddMessage("user", fmt.Sprintf("msg_%d_%d", workerID, j)); err != nil {
					errors <- err
				}

				// Update memory
				path := fmt.Sprintf("file_%d_%d.txt", workerID, j)
				if err := store.UpdateWorkingMemory(path, "content"); err != nil {
					errors <- err
				}

				// Get history
				if _, err := store.GetContextAwareHistory(10000); err != nil {
					errors <- err
				}

				// List memory files
				if _, err := store.ListMemoryFiles(); err != nil {
					errors <- err
				}

				// Save plan
				planID := fmt.Sprintf("plan_%d_%d", workerID, j)
				if err := store.SavePlan(planID, "task", []string{"step"}, 0.1); err != nil {
					errors <- err
				}
			}
		}(i)
	}

	wg.Wait()
	close(errors)

	// Check for errors
	errorCount := 0
	for err := range errors {
		errorCount++
		t.Errorf("Mixed operation error: %v", err)
	}

	if errorCount > 0 {
		t.Errorf("Total errors: %d", errorCount)
	}
}

// TestRaceDetection ensures no data races occur (run with -race flag)
func TestRaceDetection(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	var wg sync.WaitGroup
	const iterations = 100

	// Rapid concurrent reads and writes
	for i := 0; i < 10; i++ {
		wg.Add(2)

		// Writer
		go func(id int) {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				store.UpdateWorkingMemory(fmt.Sprintf("file_%d", id), fmt.Sprintf("content_%d", j))
			}
		}(i)

		// Reader
		go func() {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				store.GetWorkingMemory()
				store.ListMemoryFiles()
			}
		}()
	}

	wg.Wait()
}

// TestHistoryConsistency verifies that concurrent writes don't corrupt message order
func TestHistoryConsistency(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	const numMessages = 100
	var wg sync.WaitGroup

	// Write messages concurrently
	for i := 0; i < numMessages; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			content := fmt.Sprintf("message_%04d", id)
			store.AddMessage("user", content)
		}(i)
	}

	wg.Wait()

	// Verify all messages are present
	messages, err := store.GetContextAwareHistory(1000000)
	if err != nil {
		t.Fatalf("Failed to get history: %v", err)
	}

	if len(messages) != numMessages {
		t.Errorf("Expected %d messages, got %d", numMessages, len(messages))
	}

	// Check for duplicates or missing messages
	seen := make(map[string]bool)
	for _, msg := range messages {
		if seen[msg.Content] {
			t.Errorf("Duplicate message found: %s", msg.Content)
		}
		seen[msg.Content] = true
	}
}

// TestMemoryHashOptimization verifies concurrent updates with same content are handled correctly
func TestMemoryHashOptimization(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	const path = "shared_file.txt"
	const content = "shared content that doesn't change"

	var wg sync.WaitGroup
	const numUpdates = 50

	// Multiple goroutines updating the same file with same content
	for i := 0; i < numUpdates; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			store.UpdateWorkingMemory(path, content)
		}()
	}

	wg.Wait()

	// Verify the file exists and has correct content
	files, err := store.ListMemoryFiles()
	if err != nil {
		t.Fatalf("Failed to list files: %v", err)
	}

	if len(files) != 1 || files[0] != path {
		t.Errorf("Expected single file %s, got %v", path, files)
	}

	memory, _, err := store.GetWorkingMemory()
	if err != nil {
		t.Fatalf("Failed to get memory: %v", err)
	}

	if !contains(memory, content) {
		t.Errorf("Memory doesn't contain expected content")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// TestStoreClose verifies safe closure under concurrent operations
func TestStoreClose(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := New(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}

	var wg sync.WaitGroup

	// Start some background operations
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < 10; j++ {
				// These operations might fail after Close() is called, which is expected
				store.UpdateWorkingMemory(fmt.Sprintf("file_%d", id), "content")
				time.Sleep(time.Millisecond)
			}
		}(i)
	}

	// Wait a bit then close
	time.Sleep(time.Millisecond * 20)
	if err := store.Close(); err != nil {
		t.Errorf("Close failed: %v", err)
	}

	wg.Wait()
}
