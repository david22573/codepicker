package database

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/david22573/codepicker/internal/tokenizer"
	"github.com/david22573/codepicker/pkg/openrouter"
	_ "modernc.org/sqlite" // Pure Go SQLite driver
)

const schema = `
CREATE TABLE IF NOT EXISTS history (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	role TEXT NOT NULL,
	content TEXT NOT NULL,
	token_count INTEGER NOT NULL,
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS memory_files (
	path TEXT PRIMARY KEY,
	content TEXT NOT NULL,
	token_count INTEGER NOT NULL,
	last_accessed DATETIME DEFAULT CURRENT_TIMESTAMP
);

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

CREATE TABLE IF NOT EXISTS plans (
    id TEXT PRIMARY KEY,
    task TEXT NOT NULL,
    steps_json TEXT NOT NULL,
    estimated_cost REAL,
    estimated_turns INTEGER,
    actual_cost REAL,
    actual_turns INTEGER,
    status TEXT DEFAULT 'created',
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_jobs_status ON jobs(status);
CREATE INDEX IF NOT EXISTS idx_jobs_priority ON jobs(priority DESC, created_at ASC);
`

type Store struct {
	db *sql.DB
}

func New(storageDir string) (*Store, error) {
	if err := os.MkdirAll(storageDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create storage dir: %w", err)
	}

	dbPath := filepath.Join(storageDir, "codepicker.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to init schema: %w", err)
	}

	return &Store{db: db}, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

// History & Memory Methods (Existing)
func (s *Store) AddMessage(role string, content string) error {
	tokens := tokenizer.CountTokens(content)
	_, err := s.db.Exec(
		"INSERT INTO history (role, content, token_count) VALUES (?, ?, ?)",
		role, content, tokens,
	)
	return err
}

func (s *Store) GetContextAwareHistory(tokenBudget int) ([]openrouter.ChatMessage, error) {
	rows, err := s.db.Query("SELECT role, content, token_count FROM history ORDER BY id DESC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var reversedMessages []openrouter.ChatMessage
	currentTokens := 0

	for rows.Next() {
		var role, content string
		var tokens int
		if err := rows.Scan(&role, &content, &tokens); err != nil {
			continue
		}

		if currentTokens+tokens > tokenBudget {
			break
		}

		reversedMessages = append(reversedMessages, openrouter.ChatMessage{
			Role:    role,
			Content: content,
		})
		currentTokens += tokens
	}

	messages := make([]openrouter.ChatMessage, len(reversedMessages))
	for i, msg := range reversedMessages {
		messages[len(reversedMessages)-1-i] = msg
	}

	return messages, nil
}

func (s *Store) ClearHistory() error {
	_, err := s.db.Exec("DELETE FROM history")
	return err
}

func (s *Store) UpdateWorkingMemory(path string, content string) error {
	tokens := tokenizer.CountTokens(content)
	_, err := s.db.Exec(`
		INSERT INTO memory_files (path, content, token_count, last_accessed) 
		VALUES (?, ?, ?, ?)
		ON CONFLICT(path) DO UPDATE SET 
			content=excluded.content, 
			token_count=excluded.token_count,
			last_accessed=excluded.last_accessed
	`, path, content, tokens, time.Now())
	return err
}

func (s *Store) GetWorkingMemory() (string, int, error) {
	rows, err := s.db.Query("SELECT path, content, token_count FROM memory_files ORDER BY path ASC")
	if err != nil {
		return "", 0, err
	}
	defer rows.Close()

	var sb strings.Builder
	totalTokens := 0
	count := 0

	sb.WriteString("\n### ACTIVE WORKING MEMORY (Files you have read):\n")

	for rows.Next() {
		var path, content string
		var tokens int
		if err := rows.Scan(&path, &content, &tokens); err != nil {
			continue
		}

		sb.WriteString(fmt.Sprintf("--- BEGIN FILE: %s ---\n%s\n--- END FILE: %s ---\n\n", path, content, path))
		totalTokens += tokens
		count++
	}

	if count == 0 {
		return "", 0, nil
	}
	return sb.String(), totalTokens, nil
}

func (s *Store) RemoveFromMemory(path string) error {
	_, err := s.db.Exec("DELETE FROM memory_files WHERE path = ?", path)
	return err
}

func (s *Store) ListMemoryFiles() ([]string, error) {
	rows, err := s.db.Query("SELECT path FROM memory_files ORDER BY path ASC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var files []string
	for rows.Next() {
		var path string
		if err := rows.Scan(&path); err == nil {
			files = append(files, path)
		}
	}
	return files, nil
}

// Plan Methods (New for Phase 1)

type PlanRecord struct {
	ID            string
	Task          string
	StepsJSON     string
	EstimatedCost float64
	Status        string
}

func (s *Store) SavePlan(id, task string, steps interface{}, estCost float64) error {
	stepsJSON, err := json.Marshal(steps)
	if err != nil {
		return err
	}

	_, err = s.db.Exec(`
		INSERT INTO plans (id, task, steps_json, estimated_cost, status)
		VALUES (?, ?, ?, ?, 'created')
		ON CONFLICT(id) DO UPDATE SET
			steps_json=excluded.steps_json,
			estimated_cost=excluded.estimated_cost,
			status='updated'
	`, id, task, string(stepsJSON), estCost)
	return err
}

func (s *Store) GetPlan(id string) (*PlanRecord, error) {
	var p PlanRecord
	err := s.db.QueryRow("SELECT id, task, steps_json, estimated_cost, status FROM plans WHERE id = ?", id).
		Scan(&p.ID, &p.Task, &p.StepsJSON, &p.EstimatedCost, &p.Status)
	if err != nil {
		return nil, err
	}
	return &p, nil
}

func (s *Store) UpdatePlanStatus(id, status string) error {
	_, err := s.db.Exec("UPDATE plans SET status = ? WHERE id = ?", status, id)
	return err
}

func (s *Store) DB() *sql.DB {
	return s.db
}
