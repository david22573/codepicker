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

// Safety cap to prevent blowing up the context window (100k tokens)
// If you use smaller models, you might want to lower this.
const MaxContextTokens = 100000

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

	// CRITICAL FIX: Enable WAL mode for concurrency and set a busy timeout.
	// This allows readers (Agent) and writers (Workers) to coexist without locking.
	dsn := fmt.Sprintf("%s?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)", dbPath)

	db, err := sql.Open("sqlite", dsn)
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

func (s *Store) UpdateWorkingMemory(path string, content string) error {
	newHash := calculateHash(content)

	var currentHash string
	err := s.db.QueryRow("SELECT content_hash FROM memory_files WHERE path = ?", path).Scan(&currentHash)

	if err == nil && currentHash == newHash {
		// Just update the timestamp to keep it "fresh" in the context
		_, err := s.db.Exec("UPDATE memory_files SET last_accessed = ? WHERE path = ?", time.Now(), path)
		return err
	}

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
	// CRITICAL FIX: Order by last_accessed DESC.
	// We want the files we touched MOST RECENTLY to be guaranteed in the context.
	rows, err := s.db.Query("SELECT path, content, token_count FROM memory_files ORDER BY last_accessed DESC")
	if err != nil {
		return "", 0, err
	}
	defer rows.Close()

	// We'll collect files, but we need to reverse them at the end so the prompt flows naturally
	// (usually prompt engineering prefers file A, file B, file C...)
	// But for filtering, we process mostly-recently-used first.
	type fileEntry struct {
		path    string
		content string
	}

	var keptFiles []fileEntry
	totalTokens := 0
	droppedCount := 0

	for rows.Next() {
		var path, content string
		var tokens int
		if err := rows.Scan(&path, &content, &tokens); err != nil {
			continue
		}

		// Circuit Breaker: If adding this file exceeds our safety cap, skip it (effectively dropping old files)
		if totalTokens+tokens > MaxContextTokens {
			droppedCount++
			continue
		}

		keptFiles = append(keptFiles, fileEntry{path, content})
		totalTokens += tokens
	}

	if len(keptFiles) == 0 {
		return "", 0, nil
	}

	var sb strings.Builder
	sb.WriteString("\n### ACTIVE WORKING MEMORY (Files you have read):\n")

	// Write them out. (Order: newest accessed first in the list, which is fine,
	// or you can reverse iterate if you prefer specific ordering).
	for _, f := range keptFiles {
		sb.WriteString(fmt.Sprintf("--- BEGIN FILE: %s ---\n%s\n--- END FILE: %s ---\n\n", f.path, f.content, f.path))
	}

	if droppedCount > 0 {
		sb.WriteString(fmt.Sprintf("\n[System Note: %d older files were dropped from context to save space]\n", droppedCount))
	}

	return sb.String(), totalTokens, nil
}

func (s *Store) ClearWorkingMemory() error {
	_, err := s.db.Exec("DELETE FROM memory_files")
	return err
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

func (s *Store) RestoreSnapshot(snap *MemorySnapshot) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}

	if _, err := tx.Exec("DELETE FROM memory_files"); err != nil {
		tx.Rollback()
		return err
	}

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
