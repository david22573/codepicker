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
	"sync"
	"time"

	"github.com/david22573/codepicker/internal/tokenizer"
	"github.com/david22573/codepicker/pkg/openrouter"
	_ "modernc.org/sqlite" // Pure Go SQLite driver
)

const MaxContextTokens = 100000

type Store struct {
	db      *sql.DB
	mu      sync.RWMutex // Protects all database operations
	scorer  *RelevanceScorer
	evictOn bool // Flag to enable/disable intelligent eviction
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

	dsn := fmt.Sprintf("%s?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)", dbPath)

	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	if err := Migrate(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("database migration failed: %w", err)
	}

	scorer := NewRelevanceScorer(db)

	return &Store{
		db:      db,
		scorer:  scorer,
		evictOn: true, // Enable intelligent eviction by default
	}, nil
}

func (s *Store) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.db.Close()
}

func (s *Store) DB() *sql.DB {
	// Note: Direct access to DB is inherently unsafe in concurrent scenarios.
	// Callers using this method must handle their own synchronization.
	return s.db
}

// EnableIntelligentEviction enables or disables intelligent context eviction
func (s *Store) EnableIntelligentEviction(enabled bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.evictOn = enabled
}

func calculateHash(content string) string {
	hasher := sha256.New()
	hasher.Write([]byte(content))
	return hex.EncodeToString(hasher.Sum(nil))
}

// AddMessage adds a message to the history with thread-safe write locking
func (s *Store) AddMessage(role string, content string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	tokens := tokenizer.CountTokens(content)
	_, err := s.db.Exec(
		"INSERT INTO history (role, content, token_count) VALUES (?, ?, ?)",
		role, content, tokens,
	)
	return err
}

// GetContextAwareHistory retrieves history messages with thread-safe read locking
func (s *Store) GetContextAwareHistory(tokenBudget int) ([]openrouter.ChatMessage, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

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

// ClearHistory clears all history with thread-safe write locking
func (s *Store) ClearHistory() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, err := s.db.Exec("DELETE FROM history")
	return err
}

// UpdateWorkingMemory updates a file in working memory with thread-safe write locking
// Now with intelligent eviction and access tracking
func (s *Store) UpdateWorkingMemory(path string, content string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	newHash := calculateHash(content)

	var currentHash string
	var accessCount int
	err := s.db.QueryRow("SELECT content_hash, access_count FROM memory_files WHERE path = ?", path).Scan(&currentHash, &accessCount)

	if err == nil {
		// File exists - update access tracking
		if currentHash == newHash {
			// Content unchanged, just update access time and count
			_, err := s.db.Exec(
				"UPDATE memory_files SET last_accessed = ?, access_count = access_count + 1 WHERE path = ?",
				time.Now(), path)
			return err
		}
		// Content changed - increment access count
		accessCount++
	} else {
		// New file
		accessCount = 1
	}

	tokens := tokenizer.CountTokens(content)

	// Insert or update the file
	_, err = s.db.Exec(`
		INSERT INTO memory_files (path, content, token_count, content_hash, last_accessed, access_count) 
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(path) DO UPDATE SET 
			content=excluded.content, 
			token_count=excluded.token_count,
			content_hash=excluded.content_hash,
			last_accessed=excluded.last_accessed,
			access_count=excluded.access_count
	`, path, content, tokens, newHash, time.Now(), accessCount)

	if err != nil {
		return err
	}

	// Perform intelligent eviction if needed
	if s.evictOn {
		return s.performIntelligentEviction()
	}

	return nil
}

// performIntelligentEviction evicts low-relevance files when context is too large
func (s *Store) performIntelligentEviction() error {
	// Note: This is called with s.mu already locked from UpdateWorkingMemory

	// Check total tokens
	var totalTokens int
	err := s.db.QueryRow("SELECT COALESCE(SUM(token_count), 0) FROM memory_files").Scan(&totalTokens)
	if err != nil {
		return err
	}

	if totalTokens <= MaxContextTokens {
		return nil // No eviction needed
	}

	// Use relevance scorer to select files for eviction
	toEvict, err := s.scorer.SelectFilesForEviction(MaxContextTokens)
	if err != nil {
		return fmt.Errorf("failed to calculate eviction candidates: %w", err)
	}

	if len(toEvict) == 0 {
		return nil
	}

	// Evict the selected files
	for _, path := range toEvict {
		_, err := s.db.Exec("DELETE FROM memory_files WHERE path = ?", path)
		if err != nil {
			return fmt.Errorf("failed to evict %s: %w", path, err)
		}
	}

	return nil
}

// GetWorkingMemory retrieves working memory with thread-safe read locking
// Now uses intelligent relevance scoring to prioritize important files
func (s *Store) GetWorkingMemory() (string, int, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var files []RelevanceScore
	var err error

	if s.evictOn {
		// Use relevance-based ordering
		files, err = s.scorer.CalculateRelevanceScores()
		if err != nil {
			return "", 0, err
		}
		// Sort by relevance (highest first)
		sortByScoreDesc(files)
	} else {
		// Fall back to simple recency-based ordering
		rows, err := s.db.Query("SELECT path, content, token_count, last_accessed, access_count FROM memory_files ORDER BY last_accessed DESC")
		if err != nil {
			return "", 0, err
		}
		defer rows.Close()

		for rows.Next() {
			var rs RelevanceScore
			if err := rows.Scan(&rs.Path, &rs.Content, &rs.TokenCount, &rs.LastAccessed, &rs.AccessCount); err != nil {
				continue
			}
			files = append(files, rs)
		}
	}

	if len(files) == 0 {
		return "", 0, nil
	}

	type fileEntry struct {
		path    string
		content string
	}

	var keptFiles []fileEntry
	totalTokens := 0
	droppedCount := 0

	for _, f := range files {
		if totalTokens+f.TokenCount > MaxContextTokens {
			droppedCount++
			continue
		}

		keptFiles = append(keptFiles, fileEntry{f.Path, f.Content})
		totalTokens += f.TokenCount
	}

	if len(keptFiles) == 0 {
		return "", 0, nil
	}

	var sb strings.Builder
	sb.WriteString("\n### ACTIVE WORKING MEMORY (Files you have read):\n")

	for _, f := range keptFiles {
		sb.WriteString(fmt.Sprintf("--- BEGIN FILE: %s ---\n%s\n--- END FILE: %s ---\n\n", f.path, f.content, f.path))
	}

	if droppedCount > 0 {
		if s.evictOn {
			sb.WriteString(fmt.Sprintf("\n[System Note: %d lower-relevance files were excluded from context to conserve tokens]\n", droppedCount))
		} else {
			sb.WriteString(fmt.Sprintf("\n[System Note: %d older files were dropped from context to save space]\n", droppedCount))
		}
	}

	return sb.String(), totalTokens, nil
}

// ClearWorkingMemory clears all working memory with thread-safe write locking
func (s *Store) ClearWorkingMemory() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, err := s.db.Exec("DELETE FROM memory_files")
	return err
}

// RemoveFromMemory removes a file from working memory with thread-safe write locking
func (s *Store) RemoveFromMemory(path string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, err := s.db.Exec("DELETE FROM memory_files WHERE path = ?", path)
	return err
}

// ListMemoryFiles lists all files in working memory with thread-safe read locking
func (s *Store) ListMemoryFiles() ([]string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

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

// CreateSnapshot creates a snapshot of working memory with thread-safe read locking
func (s *Store) CreateSnapshot() (*MemorySnapshot, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

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

// RestoreSnapshot restores a snapshot to working memory with thread-safe write locking
func (s *Store) RestoreSnapshot(snap *MemorySnapshot) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	tx, err := s.db.Begin()
	if err != nil {
		return err
	}

	if _, err := tx.Exec("DELETE FROM memory_files"); err != nil {
		tx.Rollback()
		return err
	}

	stmt, err := tx.Prepare(`
		INSERT INTO memory_files (path, content, token_count, content_hash, last_accessed, access_count)
		VALUES (?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		tx.Rollback()
		return err
	}
	defer stmt.Close()

	for _, f := range snap.Files {
		hash := calculateHash(f.Content)
		if _, err := stmt.Exec(f.Path, f.Content, f.TokenCount, hash, time.Now(), 1); err != nil {
			tx.Rollback()
			return err
		}
	}

	return tx.Commit()
}

// SavePlan saves a plan to the database with thread-safe write locking
func (s *Store) SavePlan(id, task string, steps interface{}, estCost float64) error {
	s.mu.Lock()
	defer s.mu.Unlock()

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

// GetPlan retrieves a plan from the database with thread-safe read locking
func (s *Store) GetPlan(id string) (*PlanRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var p PlanRecord
	err := s.db.QueryRow("SELECT id, task, steps_json, estimated_cost, status FROM plans WHERE id = ?", id).
		Scan(&p.ID, &p.Task, &p.StepsJSON, &p.EstimatedCost, &p.Status)
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// UpdatePlanStatus updates a plan's status with thread-safe write locking
func (s *Store) UpdatePlanStatus(id, status string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, err := s.db.Exec("UPDATE plans SET status = ? WHERE id = ?", status, id)
	return err
}

// GetRelevanceScorer returns the relevance scorer for debugging/inspection
func (s *Store) GetRelevanceScorer() *RelevanceScorer {
	return s.scorer
}
