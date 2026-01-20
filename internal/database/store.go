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

type Store struct {
	db *sql.DB
}

// PlanRecord represents a saved plan in the database
type PlanRecord struct {
	ID            string
	Task          string
	StepsJSON     string
	EstimatedCost float64
	Status        string
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

	// CHANGED: Use the migration system instead of raw SQL execution
	if err := Migrate(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("database migration failed: %w", err)
	}

	return &Store{db: db}, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) DB() *sql.DB {
	return s.db
}

// --- History Methods ---

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

	// Reverse back to chronological order
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

// --- Working Memory Methods ---

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

// --- Planning Methods ---

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
