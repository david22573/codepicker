package database

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
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

type PlanRecord struct {
	ID            string
	Task          string
	StepsJSON     string
	EstimatedCost float64
	Status        string
}

// MemorySnapshot holds the state of working memory at a point in time
type MemorySnapshot struct {
	Files []MemoryFile
}

type MemoryFile struct {
	Path       string
	Content    string
	TokenCount int
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

// [2.3] Deduplication Helper
func calculateHash(content string) string {
	hasher := sha256.New()
	hasher.Write([]byte(content))
	return hex.EncodeToString(hasher.Sum(nil))
}

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

// [2.3] UpdateWorkingMemory with Deduplication
func (s *Store) UpdateWorkingMemory(path string, content string) error {
	newHash := calculateHash(content)

	// Check if file exists and hash matches
	var currentHash string
	err := s.db.QueryRow("SELECT content_hash FROM memory_files WHERE path = ?", path).Scan(&currentHash)

	if err == nil && currentHash == newHash {
		// Content hasn't changed, just update timestamp
		_, err := s.db.Exec("UPDATE memory_files SET last_accessed = ? WHERE path = ?", time.Now(), path)
		return err
	}

	// Content changed or new file
	tokens := tokenizer.CountTokens(content)
	_, err = s.db.Exec(`
		INSERT INTO memory_files (path, content, token_count, content_hash, last_accessed) 
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(path) DO UPDATE SET 
			content=excluded.content, 
			token_count=excluded.token_count,
			content_hash=excluded.content_hash,
			last_accessed=excluded.last_accessed
	`, path, content, tokens, newHash, time.Now())
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

// [2.1] Snapshot Implementation
func (s *Store) CreateSnapshot() (*MemorySnapshot, error) {
	rows, err := s.db.Query("SELECT path, content, token_count FROM memory_files")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	snapshot := &MemorySnapshot{}
	for rows.Next() {
		var f MemoryFile
		if err := rows.Scan(&f.Path, &f.Content, &f.TokenCount); err != nil {
			return nil, err
		}
		snapshot.Files = append(snapshot.Files, f)
	}
	return snapshot, nil
}

// [2.1] Restore Implementation
func (s *Store) RestoreSnapshot(snap *MemorySnapshot) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}

	// Wipe current memory
	if _, err := tx.Exec("DELETE FROM memory_files"); err != nil {
		tx.Rollback()
		return err
	}

	// Restore from snapshot
	stmt, err := tx.Prepare(`
		INSERT INTO memory_files (path, content, token_count, content_hash, last_accessed)
		VALUES (?, ?, ?, ?, ?)
	`)
	if err != nil {
		tx.Rollback()
		return err
	}
	defer stmt.Close()

	for _, f := range snap.Files {
		hash := calculateHash(f.Content)
		if _, err := stmt.Exec(f.Path, f.Content, f.TokenCount, hash, time.Now()); err != nil {
			tx.Rollback()
			return err
		}
	}

	return tx.Commit()
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
