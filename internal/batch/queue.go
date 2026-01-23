package batch

import (
	"database/sql"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
)

type JobStatus string

const (
	StatusPending   JobStatus = "pending"
	StatusRunning   JobStatus = "running"
	StatusCompleted JobStatus = "completed"
	StatusFailed    JobStatus = "failed"
	StatusCancelled JobStatus = "cancelled"
)

type Job struct {
	ID          string
	Task        string
	Priority    int
	Status      JobStatus
	Result      string
	Error       string
	CreatedAt   time.Time
	StartedAt   *time.Time
	CompletedAt *time.Time
}

type Queue struct {
	db *sql.DB
	mu sync.RWMutex // Protects all queue operations
}

func NewQueue(db *sql.DB) *Queue {
	return &Queue{db: db}
}

// Add enqueues a new job with thread-safe write locking
func (q *Queue) Add(task string, priority int) (string, error) {
	q.mu.Lock()
	defer q.mu.Unlock()

	id := uuid.New().String()
	_, err := q.db.Exec(`
		INSERT INTO jobs (id, task, priority, status, created_at)
		VALUES (?, ?, ?, ?, ?)
	`, id, task, priority, StatusPending, time.Now())
	if err != nil {
		return "", fmt.Errorf("failed to enqueue job: %w", err)
	}
	return id, nil
}

// Next retrieves the next pending job with thread-safe write locking
// Note: This uses write lock because it's typically followed by UpdateStatus
func (q *Queue) Next() (*Job, error) {
	q.mu.RLock()
	defer q.mu.RUnlock()

	var j Job
	var started sql.NullTime
	var completed sql.NullTime

	// Find the highest priority pending job
	row := q.db.QueryRow(`
		SELECT id, task, priority, status, created_at, started_at, completed_at 
		FROM jobs 
		WHERE status = ? 
		ORDER BY priority DESC, created_at ASC 
		LIMIT 1
	`, StatusPending)

	if err := row.Scan(&j.ID, &j.Task, &j.Priority, &j.Status, &j.CreatedAt, &started, &completed); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil // Queue is empty
		}
		return nil, err
	}

	if started.Valid {
		j.StartedAt = &started.Time
	}
	if completed.Valid {
		j.CompletedAt = &completed.Time
	}

	return &j, nil
}

// UpdateStatus updates a job's status with thread-safe write locking
func (q *Queue) UpdateStatus(id string, status JobStatus, result, errMsg string) error {
	q.mu.Lock()
	defer q.mu.Unlock()

	now := time.Now()
	var err error

	if status == StatusRunning {
		_, err = q.db.Exec(`UPDATE jobs SET status = ?, started_at = ? WHERE id = ?`, status, now, id)
	} else if status == StatusCompleted || status == StatusFailed {
		_, err = q.db.Exec(`
			UPDATE jobs 
			SET status = ?, result = ?, error = ?, completed_at = ? 
			WHERE id = ?`,
			status, result, errMsg, now, id)
	} else {
		_, err = q.db.Exec(`UPDATE jobs SET status = ? WHERE id = ?`, status, id)
	}

	return err
}

// List retrieves recent jobs with thread-safe read locking
func (q *Queue) List(limit int) ([]Job, error) {
	q.mu.RLock()
	defer q.mu.RUnlock()

	rows, err := q.db.Query(`
		SELECT id, task, priority, status, created_at, started_at, completed_at 
		FROM jobs 
		ORDER BY created_at DESC LIMIT ?
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var jobs []Job
	for rows.Next() {
		var j Job
		var started, completed sql.NullTime
		if err := rows.Scan(&j.ID, &j.Task, &j.Priority, &j.Status, &j.CreatedAt, &started, &completed); err != nil {
			continue
		}
		if started.Valid {
			j.StartedAt = &started.Time
		}
		if completed.Valid {
			j.CompletedAt = &completed.Time
		}
		jobs = append(jobs, j)
	}
	return jobs, nil
}

// Clear removes old completed/failed jobs with thread-safe write locking
func (q *Queue) Clear(olderThan time.Duration) (int64, error) {
	q.mu.Lock()
	defer q.mu.Unlock()

	cutoff := time.Now().Add(-olderThan)
	res, err := q.db.Exec(`DELETE FROM jobs WHERE (status = ? OR status = ?) AND created_at < ?`, StatusCompleted, StatusFailed, cutoff)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}
