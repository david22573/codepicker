package batch

import (
	"database/sql"
	"fmt"
	"sync"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

func setupTestQueue(t *testing.T) (*Queue, func()) {
	tmpDir := t.TempDir()
	dbPath := fmt.Sprintf("%s/test.db", tmpDir)

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("Failed to open test database: %v", err)
	}

	// Create jobs table
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS jobs (
			id TEXT PRIMARY KEY,
			task TEXT NOT NULL,
			priority INTEGER DEFAULT 0,
			status TEXT NOT NULL,
			plan_json TEXT,
			result TEXT,
			error TEXT,
			cost_usd REAL DEFAULT 0.0,
			tokens_used INTEGER DEFAULT 0,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			started_at DATETIME,
			completed_at DATETIME
		);
		CREATE INDEX IF NOT EXISTS idx_jobs_status ON jobs(status);
		CREATE INDEX IF NOT EXISTS idx_jobs_priority ON jobs(priority DESC, created_at ASC);
	`)
	if err != nil {
		t.Fatalf("Failed to create jobs table: %v", err)
	}

	queue := NewQueue(db)

	cleanup := func() {
		db.Close()
	}

	return queue, cleanup
}

// TestConcurrentAdd tests thread-safety of adding jobs
func TestConcurrentAdd(t *testing.T) {
	queue, cleanup := setupTestQueue(t)
	defer cleanup()

	const numWorkers = 10
	const jobsPerWorker = 20

	var wg sync.WaitGroup
	errors := make(chan error, numWorkers*jobsPerWorker)
	jobIDs := make(chan string, numWorkers*jobsPerWorker)

	// Launch concurrent job creators
	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for j := 0; j < jobsPerWorker; j++ {
				task := fmt.Sprintf("Task from worker %d, job %d", workerID, j)
				priority := (workerID * jobsPerWorker) + j

				id, err := queue.Add(task, priority)
				if err != nil {
					errors <- fmt.Errorf("worker %d: %w", workerID, err)
				} else {
					jobIDs <- id
				}
			}
		}(i)
	}

	wg.Wait()
	close(errors)
	close(jobIDs)

	// Check for errors
	for err := range errors {
		t.Errorf("Concurrent add error: %v", err)
	}

	// Verify all jobs were added with unique IDs
	uniqueIDs := make(map[string]bool)
	for id := range jobIDs {
		if uniqueIDs[id] {
			t.Errorf("Duplicate job ID: %s", id)
		}
		uniqueIDs[id] = true
	}

	expectedCount := numWorkers * jobsPerWorker
	if len(uniqueIDs) != expectedCount {
		t.Errorf("Expected %d unique job IDs, got %d", expectedCount, len(uniqueIDs))
	}

	// Verify jobs in database
	jobs, err := queue.List(1000)
	if err != nil {
		t.Fatalf("Failed to list jobs: %v", err)
	}

	if len(jobs) != expectedCount {
		t.Errorf("Expected %d jobs in database, got %d", expectedCount, len(jobs))
	}
}

// TestConcurrentNextAndUpdate simulates multiple workers picking up jobs
func TestConcurrentNextAndUpdate(t *testing.T) {
	queue, cleanup := setupTestQueue(t)
	defer cleanup()

	const numJobs = 50
	const numWorkers = 5

	// Create jobs
	for i := 0; i < numJobs; i++ {
		task := fmt.Sprintf("Job %d", i)
		if _, err := queue.Add(task, i); err != nil {
			t.Fatalf("Failed to create job: %v", err)
		}
	}

	var wg sync.WaitGroup
	errors := make(chan error, numWorkers*10)
	processedJobs := make(chan string, numJobs)
	var processedMu sync.Mutex
	processed := make(map[string]bool)

	// Launch workers that pick up and process jobs
	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()

			for {
				// Get next job
				job, err := queue.Next()
				if err != nil {
					errors <- fmt.Errorf("worker %d next: %w", workerID, err)
					return
				}

				if job == nil {
					// No more pending jobs
					return
				}

				// Check for duplicate processing
				processedMu.Lock()
				if processed[job.ID] {
					processedMu.Unlock()
					errors <- fmt.Errorf("worker %d: duplicate job %s", workerID, job.ID)
					return
				}
				processed[job.ID] = true
				processedMu.Unlock()

				// Mark as running
				if err := queue.UpdateStatus(job.ID, StatusRunning, "", ""); err != nil {
					errors <- fmt.Errorf("worker %d update running: %w", workerID, err)
					return
				}

				// Simulate work
				time.Sleep(time.Millisecond * 1)

				// Mark as completed
				result := fmt.Sprintf("Processed by worker %d", workerID)
				if err := queue.UpdateStatus(job.ID, StatusCompleted, result, ""); err != nil {
					errors <- fmt.Errorf("worker %d update completed: %w", workerID, err)
					return
				}

				processedJobs <- job.ID
			}
		}(i)
	}

	wg.Wait()
	close(errors)
	close(processedJobs)

	// Check for errors
	for err := range errors {
		t.Errorf("Concurrent next/update error: %v", err)
	}

	// Verify all jobs were processed
	processedCount := 0
	for range processedJobs {
		processedCount++
	}

	if processedCount != numJobs {
		t.Errorf("Expected %d jobs processed, got %d", numJobs, processedCount)
	}

	// Verify all jobs are completed
	jobs, err := queue.List(1000)
	if err != nil {
		t.Fatalf("Failed to list jobs: %v", err)
	}

	completedCount := 0
	for _, job := range jobs {
		if job.Status == StatusCompleted {
			completedCount++
		}
	}

	if completedCount != numJobs {
		t.Errorf("Expected %d completed jobs, got %d", numJobs, completedCount)
	}
}

// TestConcurrentListOperations tests thread-safety of list operations
func TestConcurrentListOperations(t *testing.T) {
	queue, cleanup := setupTestQueue(t)
	defer cleanup()

	// Create some initial jobs
	for i := 0; i < 20; i++ {
		if _, err := queue.Add(fmt.Sprintf("Job %d", i), i); err != nil {
			t.Fatalf("Failed to create job: %v", err)
		}
	}

	var wg sync.WaitGroup
	errors := make(chan error, 50)

	// Launch concurrent readers and writers
	for i := 0; i < 10; i++ {
		wg.Add(2)

		// Reader
		go func() {
			defer wg.Done()
			for j := 0; j < 10; j++ {
				if _, err := queue.List(50); err != nil {
					errors <- fmt.Errorf("list error: %w", err)
				}
				time.Sleep(time.Millisecond)
			}
		}()

		// Writer
		go func(id int) {
			defer wg.Done()
			for j := 0; j < 5; j++ {
				task := fmt.Sprintf("Concurrent job %d-%d", id, j)
				if _, err := queue.Add(task, id*10+j); err != nil {
					errors <- fmt.Errorf("add error: %w", err)
				}
				time.Sleep(time.Millisecond * 2)
			}
		}(i)
	}

	wg.Wait()
	close(errors)

	// Check for errors
	for err := range errors {
		t.Errorf("Concurrent list operation error: %v", err)
	}
}

// TestConcurrentClear tests thread-safety of clearing old jobs
func TestConcurrentClear(t *testing.T) {
	queue, cleanup := setupTestQueue(t)
	defer cleanup()

	// Create jobs with different ages
	for i := 0; i < 30; i++ {
		id, err := queue.Add(fmt.Sprintf("Job %d", i), i)
		if err != nil {
			t.Fatalf("Failed to create job: %v", err)
		}

		// Mark half as completed
		if i < 15 {
			queue.UpdateStatus(id, StatusCompleted, "done", "")
		}
	}

	var wg sync.WaitGroup
	errors := make(chan error, 10)

	// Launch concurrent clear operations
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			// Try to clear jobs older than 1 hour (should be all completed jobs)
			if _, err := queue.Clear(time.Hour); err != nil {
				errors <- fmt.Errorf("clear error: %w", err)
			}
		}()
	}

	wg.Wait()
	close(errors)

	// Check for errors
	for err := range errors {
		t.Errorf("Concurrent clear error: %v", err)
	}

	// Verify pending jobs remain
	jobs, err := queue.List(1000)
	if err != nil {
		t.Fatalf("Failed to list jobs: %v", err)
	}

	pendingCount := 0
	for _, job := range jobs {
		if job.Status == StatusPending {
			pendingCount++
		}
	}

	if pendingCount != 15 {
		t.Errorf("Expected 15 pending jobs remaining, got %d", pendingCount)
	}
}

// TestMixedQueueOperations tests all queue operations running concurrently
func TestMixedQueueOperations(t *testing.T) {
	queue, cleanup := setupTestQueue(t)
	defer cleanup()

	const duration = 100 * time.Millisecond
	var wg sync.WaitGroup
	errors := make(chan error, 100)
	done := make(chan bool)

	// Job adder
	wg.Add(1)
	go func() {
		defer wg.Done()
		i := 0
		for {
			select {
			case <-done:
				return
			default:
				if _, err := queue.Add(fmt.Sprintf("Job %d", i), i); err != nil {
					errors <- err
				}
				i++
				time.Sleep(time.Millisecond * 2)
			}
		}
	}()

	// Job processor
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-done:
				return
			default:
				job, err := queue.Next()
				if err != nil {
					errors <- err
					continue
				}
				if job != nil {
					queue.UpdateStatus(job.ID, StatusRunning, "", "")
					time.Sleep(time.Millisecond)
					queue.UpdateStatus(job.ID, StatusCompleted, "done", "")
				}
				time.Sleep(time.Millisecond * 5)
			}
		}
	}()

	// Job lister
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-done:
				return
			default:
				if _, err := queue.List(20); err != nil {
					errors <- err
				}
				time.Sleep(time.Millisecond * 3)
			}
		}
	}()

	// Let operations run
	time.Sleep(duration)
	close(done)
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

// TestRaceDetection ensures no data races in queue operations (run with -race flag)
func TestRaceDetection(t *testing.T) {
	queue, cleanup := setupTestQueue(t)
	defer cleanup()

	var wg sync.WaitGroup
	const iterations = 50

	// Rapid concurrent operations
	for i := 0; i < 5; i++ {
		wg.Add(3)

		// Adder
		go func(id int) {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				queue.Add(fmt.Sprintf("task_%d_%d", id, j), j)
			}
		}(i)

		// Getter
		go func() {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				queue.Next()
			}
		}()

		// Lister
		go func() {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				queue.List(10)
			}
		}()
	}

	wg.Wait()
}

// TestJobPriorityOrdering verifies concurrent adds maintain priority ordering
func TestJobPriorityOrdering(t *testing.T) {
	queue, cleanup := setupTestQueue(t)
	defer cleanup()

	var wg sync.WaitGroup
	const numJobs = 100

	// Add jobs with random priorities concurrently
	for i := 0; i < numJobs; i++ {
		wg.Add(1)
		go func(priority int) {
			defer wg.Done()
			task := fmt.Sprintf("Priority task %d", priority)
			queue.Add(task, priority)
		}(i)
	}

	wg.Wait()

	// Verify jobs are retrieved in priority order
	prevPriority := numJobs
	for {
		job, err := queue.Next()
		if err != nil {
			t.Fatalf("Failed to get next job: %v", err)
		}
		if job == nil {
			break
		}

		if job.Priority > prevPriority {
			t.Errorf("Priority ordering violated: got %d after %d", job.Priority, prevPriority)
		}
		prevPriority = job.Priority

		// Mark as completed to continue
		queue.UpdateStatus(job.ID, StatusCompleted, "", "")
	}
}
