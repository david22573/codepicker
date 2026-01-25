package batch_test

import (
	"database/sql"
	"path/filepath"
	"sync"
	"testing"

	"github.com/david22573/codepicker/internal/batch"
	"github.com/david22573/codepicker/internal/logger"
	_ "modernc.org/sqlite" // Pure Go SQLite driver
)

// setupTestDB creates a temporary in-memory database for testing
func setupTestDB(t *testing.T) *sql.DB {
	t.Helper()
	// Using a file-based DB for concurrency tests to avoid "database is locked" in strict memory mode
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test_batch.db")

	db, err := sql.Open("sqlite", dbPath+"?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)")
	if err != nil {
		t.Fatalf("Failed to open test DB: %v", err)
	}

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
		t.Fatalf("Failed to create schema: %v", err)
	}

	return db
}

func TestRunner_ProcessQueue(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	q := batch.NewQueue(db)
	_ = &logger.NoOpLogger{} // Logger reference for context

	// Add jobs
	job1ID, _ := q.Add("Job 1", 10)
	job2ID, _ := q.Add("Job 2", 5)

	// Verify they are pending and priority is respected
	job1, _ := q.Next()
	if job1.ID != job1ID {
		t.Errorf("Expected highest priority job %s, got %s", job1ID, job1.ID)
	}

	// Simulate Runner picking it up
	if err := q.UpdateStatus(job1ID, batch.StatusRunning, "", ""); err != nil {
		t.Errorf("Failed to mark running: %v", err)
	}

	// Verify next job is Job 2
	job2, _ := q.Next()
	if job2.ID != job2ID {
		t.Errorf("Expected next job %s, got %s", job2ID, job2.ID)
	}

	// Mark Job 1 complete
	if err := q.UpdateStatus(job1ID, batch.StatusCompleted, "Done", ""); err != nil {
		t.Errorf("Failed to complete job: %v", err)
	}

	// Verify Job 1 status persistence
	var status string
	var result string
	err := db.QueryRow("SELECT status, result FROM jobs WHERE id = ?", job1ID).Scan(&status, &result)
	if err != nil {
		t.Fatalf("Failed to query job: %v", err)
	}
	if status != string(batch.StatusCompleted) {
		t.Errorf("Expected completed status, got %s", status)
	}
	if result != "Done" {
		t.Errorf("Expected result 'Done', got %s", result)
	}
}

// TestQueue_Concurrency verifies that Next() is thread-safe and doesn't hand out the same job
func TestQueue_Concurrency(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	q := batch.NewQueue(db)

	// Add 50 jobs
	for i := 0; i < 50; i++ {
		q.Add("Task", 1)
	}

	// Spawn 10 concurrent workers trying to claim jobs
	var wg sync.WaitGroup
	claimed := make(chan string, 50)

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				// Simulate the runner loop logic
				job, _ := q.Next()
				if job == nil {
					return
				}

				// Attempt to claim it
				err := q.UpdateStatus(job.ID, batch.StatusRunning, "", "")
				if err == nil {
					claimed <- job.ID
				}
			}
		}()
	}

	wg.Wait()
	close(claimed)

	// Count unique claims
	uniqueClaims := make(map[string]bool)
	count := 0
	for id := range claimed {
		if uniqueClaims[id] {
			t.Errorf("Job %s was claimed multiple times!", id)
		}
		uniqueClaims[id] = true
		count++
	}

	if count != 50 {
		t.Errorf("Expected 50 claimed jobs, got %d", count)
	}
}
