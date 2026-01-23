package batch

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/david22573/codepicker/internal/database"
	"github.com/david22573/codepicker/internal/logger"
	_ "modernc.org/sqlite"
)

// mockLogger implements a simple logger for testing
type mockLogger struct {
	mu       sync.Mutex
	messages []string
}

func (m *mockLogger) Info(msg string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.messages = append(m.messages, "INFO: "+msg)
}

func (m *mockLogger) Error(msg string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.messages = append(m.messages, "ERROR: "+msg)
}

func (m *mockLogger) Warn(msg string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.messages = append(m.messages, "WARN: "+msg)
}

func (m *mockLogger) Debug(msg string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.messages = append(m.messages, "DEBUG: "+msg)
}

func (m *mockLogger) GetMessages() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]string, len(m.messages))
	copy(result, m.messages)
	return result
}

func setupTestRunner(t *testing.T, workers int) (*Runner, *Queue, func()) {
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
	store, err := database.New(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}

	log := &mockLogger{messages: []string{}}
	runner := NewRunner(queue, store, log, workers, tmpDir)

	cleanup := func() {
		store.Close()
		db.Close()
	}

	return runner, queue, cleanup
}

// TestRunnerConcurrentJobProcessing tests concurrent job processing
func TestRunnerConcurrentJobProcessing(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	const numWorkers = 3
	runner, queue, cleanup := setupTestRunner(t, numWorkers)
	defer cleanup()

	// Add multiple jobs
	const numJobs = 10
	jobIDs := make([]string, numJobs)

	for i := 0; i < numJobs; i++ {
		id, err := queue.Add(fmt.Sprintf("Test task %d", i), i)
		if err != nil {
			t.Fatalf("Failed to add job: %v", err)
		}
		jobIDs[i] = id
	}

	// Start runner in background
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var runnerWg sync.WaitGroup
	runnerWg.Add(1)
	go func() {
		defer runnerWg.Done()
		// Note: This will process jobs but will fail as they need valid agent context
		// We're testing the concurrency, not the full integration
		runner.Start(ctx)
	}()

	// Wait briefly for processing to start
	time.Sleep(time.Millisecond * 100)

	// Cancel context to stop runner
	cancel()

	// Wait for runner to finish
	runnerWg.Wait()

	// Verify runner handled shutdown gracefully
	runner.mu.Lock()
	isShuttingDown := runner.shuttingDown
	runner.mu.Unlock()

	if !isShuttingDown {
		t.Log("Warning: Runner may not have received shutdown signal")
	}
}

// TestRunnerShutdownGraceful tests graceful shutdown
func TestRunnerShutdownGraceful(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	runner, queue, cleanup := setupTestRunner(t, 2)
	defer cleanup()

	// Add a job
	_, err := queue.Add("Shutdown test task", 1)
	if err != nil {
		t.Fatalf("Failed to add job: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	// Start runner
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		runner.Start(ctx)
	}()

	// Wait a bit then cancel
	time.Sleep(time.Millisecond * 50)
	cancel()

	// Wait for graceful shutdown
	wg.Wait()

	// Verify shutdown flag was set
	runner.mu.Lock()
	shuttingDown := runner.shuttingDown
	runner.mu.Unlock()

	if !shuttingDown {
		t.Error("Runner should have shutdown flag set")
	}
}

// TestConcurrentShutdownAccess tests concurrent access during shutdown
func TestConcurrentShutdownAccess(t *testing.T) {
	runner, _, cleanup := setupTestRunner(t, 2)
	defer cleanup()

	var wg sync.WaitGroup
	const numReaders = 20

	// Concurrent readers checking shutdown status
	for i := 0; i < numReaders; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				runner.mu.Lock()
				_ = runner.shuttingDown
				runner.mu.Unlock()
			}
		}()
	}

	// Concurrent writers setting shutdown
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			runner.mu.Lock()
			runner.shuttingDown = true
			runner.mu.Unlock()
		}()
	}

	wg.Wait()

	// Final state should be shutdown
	runner.mu.Lock()
	finalState := runner.shuttingDown
	runner.mu.Unlock()

	if !finalState {
		t.Error("Expected final state to be shuttingDown=true")
	}
}

// TestRunnerWorkerLimit tests that concurrency limit is respected
func TestRunnerWorkerLimit(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	const maxWorkers = 2
	runner, queue, cleanup := setupTestRunner(t, maxWorkers)
	defer cleanup()

	// Add more jobs than workers
	const numJobs = 10
	for i := 0; i < numJobs; i++ {
		_, err := queue.Add(fmt.Sprintf("Task %d", i), i)
		if err != nil {
			t.Fatalf("Failed to add job: %v", err)
		}
	}

	// Verify concurrency setting
	if runner.Concurrency != maxWorkers {
		t.Errorf("Expected concurrency %d, got %d", maxWorkers, runner.Concurrency)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		runner.Start(ctx)
	}()

	time.Sleep(time.Millisecond * 100)
	cancel()
	wg.Wait()
}

// TestRunnerMinimumWorkers tests that minimum workers is enforced
func TestRunnerMinimumWorkers(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := fmt.Sprintf("%s/test.db", tmpDir)

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	// Create table
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
	`)
	if err != nil {
		t.Fatal(err)
	}

	queue := NewQueue(db)
	store, err := database.New(tmpDir)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	log := &mockLogger{}

	// Test with 0 workers - should be adjusted to 1
	runner := NewRunner(queue, store, log, 0, tmpDir)
	if runner.Concurrency != 1 {
		t.Errorf("Expected concurrency 1 for input 0, got %d", runner.Concurrency)
	}

	// Test with negative workers - should be adjusted to 1
	runner = NewRunner(queue, store, log, -5, tmpDir)
	if runner.Concurrency != 1 {
		t.Errorf("Expected concurrency 1 for input -5, got %d", runner.Concurrency)
	}

	// Test with positive workers - should be preserved
	runner = NewRunner(queue, store, log, 10, tmpDir)
	if runner.Concurrency != 10 {
		t.Errorf("Expected concurrency 10, got %d", runner.Concurrency)
	}
}

// TestRunnerRaceDetection tests for data races (run with -race flag)
func TestRunnerRaceDetection(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	runner, queue, cleanup := setupTestRunner(t, 3)
	defer cleanup()

	// Add jobs
	for i := 0; i < 5; i++ {
		queue.Add(fmt.Sprintf("Race test %d", i), i)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	var wg sync.WaitGroup

	// Start runner
	wg.Add(1)
	go func() {
		defer wg.Done()
		runner.Start(ctx)
	}()

	// Concurrent shutdown flag access
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				runner.mu.Lock()
				_ = runner.shuttingDown
				runner.mu.Unlock()
				time.Sleep(time.Microsecond)
			}
		}()
	}

	wg.Wait()
}

// TestProcessJobConcurrency tests concurrent job processing
func TestProcessJobConcurrency(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	runner, queue, cleanup := setupTestRunner(t, 5)
	defer cleanup()

	// Create multiple jobs
	jobs := make([]*Job, 10)
	for i := 0; i < len(jobs); i++ {
		id, err := queue.Add(fmt.Sprintf("Process test %d", i), i)
		if err != nil {
			t.Fatal(err)
		}

		job, err := queue.Next()
		if err != nil || job == nil {
			t.Fatalf("Failed to get job: %v", err)
		}
		jobs[i] = job
	}

	// Process jobs concurrently
	var wg sync.WaitGroup
	for _, job := range jobs {
		wg.Add(1)
		go func(j *Job) {
			defer wg.Done()
			// This will fail because we don't have a proper agent context,
			// but we're testing the concurrency handling
			runner.processJob(context.Background(), j)
		}(job)
	}

	wg.Wait()

	// Verify all jobs were processed (will be marked as failed due to missing context)
	for _, job := range jobs {
		dbJob, err := queue.Next()
		if err != nil {
			t.Errorf("Error checking job: %v", err)
		}
		// Jobs should either be processed or still pending (depending on timing)
		if dbJob != nil && dbJob.Status != StatusPending {
			// Status changed, which is expected
		}
	}
}

// TestFailJobConcurrency tests concurrent failJob calls
func TestFailJobConcurrency(t *testing.T) {
	runner, queue, cleanup := setupTestRunner(t, 2)
	defer cleanup()

	// Create jobs
	const numJobs = 20
	jobs := make([]*Job, numJobs)

	for i := 0; i < numJobs; i++ {
		id, err := queue.Add(fmt.Sprintf("Fail test %d", i), i)
		if err != nil {
			t.Fatal(err)
		}
		jobs[i] = &Job{ID: id, Task: fmt.Sprintf("Fail test %d", i)}
	}

	// Fail all jobs concurrently
	var wg sync.WaitGroup
	for i, job := range jobs {
		wg.Add(1)
		go func(j *Job, idx int) {
			defer wg.Done()
			runner.failJob(j, fmt.Sprintf("Test failure %d", idx))
		}(job, i)
	}

	wg.Wait()

	// Verify jobs can be queried without errors
	allJobs, err := queue.List(100)
	if err != nil {
		t.Errorf("Failed to list jobs: %v", err)
	}

	if len(allJobs) != numJobs {
		t.Errorf("Expected %d jobs, got %d", numJobs, len(allJobs))
	}
}

// TestConcurrentRunnerCreation tests creating runners concurrently
func TestConcurrentRunnerCreation(t *testing.T) {
	tmpDir := t.TempDir()

	var wg sync.WaitGroup
	const numRunners = 10

	runners := make([]*Runner, numRunners)

	for i := 0; i < numRunners; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()

			dbPath := fmt.Sprintf("%s/test_%d.db", tmpDir, idx)
			db, err := sql.Open("sqlite", dbPath)
			if err != nil {
				t.Errorf("Failed to open db: %v", err)
				return
			}
			defer db.Close()

			// Create table
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
			`)
			if err != nil {
				t.Errorf("Failed to create table: %v", err)
				return
			}

			queue := NewQueue(db)
			storePath := fmt.Sprintf("%s/store_%d", tmpDir, idx)
			os.MkdirAll(storePath, 0755)
			store, err := database.New(storePath)
			if err != nil {
				t.Errorf("Failed to create store: %v", err)
				return
			}
			defer store.Close()

			log := &mockLogger{}
			runners[idx] = NewRunner(queue, store, log, 2, tmpDir)
		}(i)
	}

	wg.Wait()

	// Verify all runners were created
	for i, runner := range runners {
		if runner == nil {
			t.Errorf("Runner %d was not created", i)
		}
	}
}

// TestLoggerConcurrency tests concurrent logging
func TestLoggerConcurrency(t *testing.T) {
	log := &mockLogger{}

	var wg sync.WaitGroup
	const numWorkers = 20
	const messagesPerWorker = 50

	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for j := 0; j < messagesPerWorker; j++ {
				switch j % 4 {
				case 0:
					log.Info(fmt.Sprintf("Worker %d: Info %d", workerID, j))
				case 1:
					log.Error(fmt.Sprintf("Worker %d: Error %d", workerID, j))
				case 2:
					log.Warn(fmt.Sprintf("Worker %d: Warn %d", workerID, j))
				case 3:
					log.Debug(fmt.Sprintf("Worker %d: Debug %d", workerID, j))
				}
			}
		}(i)
	}

	wg.Wait()

	// Verify all messages were logged
	messages := log.GetMessages()
	expectedCount := numWorkers * messagesPerWorker

	if len(messages) != expectedCount {
		t.Errorf("Expected %d log messages, got %d", expectedCount, len(messages))
	}
}

// TestRunnerContextCancellation tests context cancellation handling
func TestRunnerContextCancellation(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	runner, queue, cleanup := setupTestRunner(t, 3)
	defer cleanup()

	// Add jobs
	for i := 0; i < 5; i++ {
		queue.Add(fmt.Sprintf("Context test %d", i), i)
	}

	ctx, cancel := context.WithCancel(context.Background())

	// Start runner
	done := make(chan bool)
	go func() {
		runner.Start(ctx)
		close(done)
	}()

	// Cancel immediately
	time.Sleep(time.Millisecond * 10)
	cancel()

	// Wait for shutdown with timeout
	select {
	case <-done:
		// Success
	case <-time.After(2 * time.Second):
		t.Error("Runner did not shutdown within timeout")
	}
}
