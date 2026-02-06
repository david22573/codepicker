package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/david22573/codepicker/domain/agent"
	ctxDomain "github.com/david22573/codepicker/domain/context"
	"github.com/david22573/codepicker/domain/task"
	_ "modernc.org/sqlite" // Pure-go driver for maximum Termux compatibility
)

// SQLiteRepository implements agent.Repository and context.SliceStore.
// Enhanced for Phase 2.1 with WAL mode and connection pooling.
type SQLiteRepository struct {
	db *sql.DB
	mu sync.RWMutex
}

// NewSQLiteRepository initializes the DB with production-grade pragmas.
func NewSQLiteRepository(path string) (*SQLiteRepository, error) {
	// DSN optimized for concurrency and reliability on mobile filesystems.
	// journal_mode=WAL: Allows concurrent reads while writing.
	// busy_timeout=5000: Prevents "database is locked" errors during high I/O.
	dsn := fmt.Sprintf("%s?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=synchronous(NORMAL)&_pragma=cache_size(-64000)", path)

	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open sqlite: %w", err)
	}

	// Connection Pool Tuning: SQLite performs best with a single writer.
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(time.Hour)

	if err := migrate(db); err != nil {
		db.Close()
		return nil, err
	}

	return &SQLiteRepository{db: db}, nil
}

// Close terminates the database connection.
func (r *SQLiteRepository) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.db != nil {
		return r.db.Close()
	}
	return nil
}

func migrate(db *sql.DB) error {
	queries := []string{
		`CREATE TABLE IF NOT EXISTS executions (id TEXT PRIMARY KEY, plan_id TEXT, status TEXT, history_json TEXT, start_time DATETIME, end_time DATETIME);`,
		`CREATE TABLE IF NOT EXISTS plans (id TEXT PRIMARY KEY, original_task TEXT, reasoning TEXT, steps_json TEXT, status TEXT, estimated_cost REAL, created_at DATETIME);`,
		`CREATE TABLE IF NOT EXISTS code_slices (
			id TEXT PRIMARY KEY,
			file_path TEXT NOT NULL,
			start_line INTEGER NOT NULL,
			end_line INTEGER NOT NULL,
			content TEXT NOT NULL,
			language TEXT,
			slice_type TEXT,
			symbols_json TEXT,
			content_hash TEXT,
			indexed_at DATETIME
		);`,
		`CREATE INDEX IF NOT EXISTS idx_file_path ON code_slices(file_path);`,
		`CREATE VIRTUAL TABLE IF NOT EXISTS slices_fts USING fts5(
			id UNINDEXED,
			file_path,
			content,
			symbols,
			content='code_slices',
			content_rowid='rowid'
		);`,
	}
	for _, q := range queries {
		if _, err := db.Exec(q); err != nil {
			return err
		}
	}
	return nil
}

// --- Execution Management ---

func (r *SQLiteRepository) SaveExecution(ctx context.Context, exec *agent.Execution) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	history, _ := json.Marshal(exec.History)
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO executions (id, plan_id, status, history_json, start_time, end_time) 
		VALUES (?, ?, ?, ?, ?, ?) 
		ON CONFLICT(id) DO UPDATE SET 
			status=excluded.status, 
			history_json=excluded.history_json, 
			end_time=excluded.end_time`,
		exec.ID, exec.PlanID, string(exec.Status), string(history), exec.StartTime, exec.EndTime,
	)
	return err
}

func (r *SQLiteRepository) GetExecution(ctx context.Context, id string) (*agent.Execution, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	row := r.db.QueryRowContext(ctx, "SELECT id, plan_id, status, history_json, start_time, end_time FROM executions WHERE id = ?", id)
	var ex agent.Execution
	var hist, status string
	var end sql.NullTime
	if err := row.Scan(&ex.ID, &ex.PlanID, &status, &hist, &ex.StartTime, &end); err != nil {
		return nil, err
	}
	ex.Status = task.Status(status)
	if end.Valid {
		ex.EndTime = end.Time
	}
	json.Unmarshal([]byte(hist), &ex.History)
	return &ex, nil
}

func (r *SQLiteRepository) ListExecutions(ctx context.Context, limit int) ([]agent.ExecutionSummary, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	rows, err := r.db.QueryContext(ctx, "SELECT id, plan_id, status, start_time FROM executions ORDER BY start_time DESC LIMIT ?", limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var res []agent.ExecutionSummary
	for rows.Next() {
		var s agent.ExecutionSummary
		var stat string
		rows.Scan(&s.ID, &s.PlanID, &stat, &s.StartTime)
		s.Status = task.Status(stat)
		res = append(res, s)
	}
	return res, nil
}

// --- Plan Management ---

func (r *SQLiteRepository) SavePlan(ctx context.Context, plan *task.Plan) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	steps, _ := json.Marshal(plan.Steps)
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO plans (id, original_task, reasoning, steps_json, status, estimated_cost, created_at) 
		VALUES (?, ?, ?, ?, ?, ?, ?) 
		ON CONFLICT(id) DO UPDATE SET 
			status=excluded.status, 
			steps_json=excluded.steps_json, 
			reasoning=excluded.reasoning`,
		plan.ID, plan.OriginalTask, plan.Reasoning, string(steps), string(plan.Status), plan.EstimatedCost, plan.CreatedAt,
	)
	return err
}

func (r *SQLiteRepository) GetPlan(ctx context.Context, id string) (*task.Plan, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	row := r.db.QueryRowContext(ctx, "SELECT id, original_task, reasoning, steps_json, status, estimated_cost, created_at FROM plans WHERE id = ?", id)
	var p task.Plan
	var steps, status string
	if err := row.Scan(&p.ID, &p.OriginalTask, &p.Reasoning, &steps, &status, &p.EstimatedCost, &p.CreatedAt); err != nil {
		return nil, err
	}
	p.Status = task.Status(status)
	json.Unmarshal([]byte(steps), &p.Steps)
	return &p, nil
}

func (r *SQLiteRepository) ListPlans(ctx context.Context, limit int) ([]agent.PlanSummary, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	rows, _ := r.db.QueryContext(ctx, "SELECT id, original_task, status, steps_json, created_at FROM plans ORDER BY created_at DESC LIMIT ?", limit)
	defer rows.Close()

	var res []agent.PlanSummary
	for rows.Next() {
		var p agent.PlanSummary
		var steps, stat string
		rows.Scan(&p.ID, &p.OriginalTask, &stat, &steps, &p.CreatedAt)
		p.Status = task.Status(stat)
		var s []task.Step
		json.Unmarshal([]byte(steps), &s)
		p.StepCount = len(s)
		res = append(res, p)
	}
	return res, nil
}

func (r *SQLiteRepository) DeletePlan(ctx context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	_, err := r.db.ExecContext(ctx, "DELETE FROM plans WHERE id = ?", id)
	return err
}

// --- Context & Slice Management ---

func (r *SQLiteRepository) IndexFile(filePath string, slices []ctxDomain.CodeSlice) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec("DELETE FROM code_slices WHERE file_path = ?", filePath); err != nil {
		return err
	}

	for _, s := range slices {
		syms, _ := json.Marshal(s.Symbols)
		_, err = tx.Exec(`
			INSERT INTO code_slices (id, file_path, start_line, end_line, content, language, slice_type, symbols_json, indexed_at) 
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			s.ID, filePath, s.StartLine, s.EndLine, s.Content, s.Language, string(s.SliceType), string(syms), time.Now(),
		)
		if err != nil {
			return err
		}
	}

	_, _ = tx.Exec("INSERT INTO slices_fts(slices_fts) VALUES('rebuild')")
	return tx.Commit()
}

func (r *SQLiteRepository) Query(q ctxDomain.SliceQuery) ([]ctxDomain.CodeSlice, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	searchQuery := strings.Join(q.Keywords, " ")
	if strings.TrimSpace(searchQuery) == "" {
		return nil, nil
	}

	limit := 20
	if q.MaxResults > 0 {
		limit = q.MaxResults
	}

	query := fmt.Sprintf(`
		SELECT id, file_path, start_line, end_line, content, slice_type, symbols_json 
		FROM code_slices 
		WHERE rowid IN (SELECT rowid FROM slices_fts WHERE slices_fts MATCH ?) 
		LIMIT %d`, limit)

	rows, err := r.db.Query(query, searchQuery)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var res []ctxDomain.CodeSlice
	for rows.Next() {
		var s ctxDomain.CodeSlice
		var st, syms string
		rows.Scan(&s.ID, &s.FilePath, &s.StartLine, &s.EndLine, &s.Content, &st, &syms)
		s.SliceType = ctxDomain.SliceType(st)
		json.Unmarshal([]byte(syms), &s.Symbols)
		res = append(res, s)
	}
	return res, nil
}

// GetAllSlices retrieves every code slice (used for full context export)
func (r *SQLiteRepository) GetAllSlices() ([]ctxDomain.CodeSlice, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	rows, err := r.db.Query("SELECT id, file_path, start_line, end_line, content, slice_type, symbols_json FROM code_slices")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var res []ctxDomain.CodeSlice
	for rows.Next() {
		var s ctxDomain.CodeSlice
		var st, syms string
		if err := rows.Scan(&s.ID, &s.FilePath, &s.StartLine, &s.EndLine, &s.Content, &st, &syms); err != nil {
			return nil, err
		}
		s.SliceType = ctxDomain.SliceType(st)
		json.Unmarshal([]byte(syms), &s.Symbols)
		res = append(res, s)
	}
	return res, nil
}

func (r *SQLiteRepository) GetByFile(path string) ([]ctxDomain.CodeSlice, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	rows, _ := r.db.Query("SELECT id, file_path, start_line, end_line, content, slice_type FROM code_slices WHERE file_path = ?", path)
	defer rows.Close()
	var res []ctxDomain.CodeSlice
	for rows.Next() {
		var s ctxDomain.CodeSlice
		var st string
		rows.Scan(&s.ID, &s.FilePath, &s.StartLine, &s.EndLine, &s.Content, &st)
		s.SliceType = ctxDomain.SliceType(st)
		res = append(res, s)
	}
	return res, nil
}

func (r *SQLiteRepository) InvalidateFile(path string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	_, err := r.db.Exec("DELETE FROM code_slices WHERE file_path = ?", path)
	return err
}

func (r *SQLiteRepository) GetStats() (*ctxDomain.IndexStats, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var s ctxDomain.IndexStats
	err := r.db.QueryRow("SELECT COUNT(*), COUNT(DISTINCT file_path) FROM code_slices").Scan(&s.TotalSlices, &s.TotalFiles)
	s.LastIndexedAt = time.Now()
	return &s, err
}

func (r *SQLiteRepository) GetByID(id string) (*ctxDomain.CodeSlice, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	row := r.db.QueryRow("SELECT id, file_path, start_line, end_line, content, slice_type FROM code_slices WHERE id = ?", id)
	var s ctxDomain.CodeSlice
	var st string
	if err := row.Scan(&s.ID, &s.FilePath, &s.StartLine, &s.EndLine, &s.Content, &st); err != nil {
		return nil, err
	}
	s.SliceType = ctxDomain.SliceType(st)
	return &s, nil
}

func (r *SQLiteRepository) GetBySymbol(symbol string) ([]ctxDomain.CodeSlice, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	rows, err := r.db.Query("SELECT id, file_path, start_line, end_line, content FROM code_slices WHERE symbols_json LIKE ?", "%"+symbol+"%")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var res []ctxDomain.CodeSlice
	for rows.Next() {
		var s ctxDomain.CodeSlice
		rows.Scan(&s.ID, &s.FilePath, &s.StartLine, &s.EndLine, &s.Content)
		res = append(res, s)
	}
	return res, nil
}
