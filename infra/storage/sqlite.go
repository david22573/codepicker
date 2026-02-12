package storage

import (
	"context"
	"database/sql"
	"encoding/json"

	"github.com/david22573/codepicker/domain/agent"
	domainContext "github.com/david22573/codepicker/domain/context"
	"github.com/david22573/codepicker/domain/task"
	_ "github.com/mattn/go-sqlite3"
)

type SQLiteRepository struct {
	db *sql.DB
}

func NewSQLiteRepository(dbPath string) (*SQLiteRepository, error) {
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, err
	}

	// Schema: Supports Plans, Executions, and Expanded Code Slices
	schema := `
	CREATE TABLE IF NOT EXISTS code_slices (
		id TEXT PRIMARY KEY,
		file_path TEXT,
		content TEXT,
		start_line INTEGER,
		end_line INTEGER,
		language TEXT,
		slice_type TEXT,
		symbols TEXT,
		hash TEXT
	);
	CREATE TABLE IF NOT EXISTS plans (
		id TEXT PRIMARY KEY,
		task TEXT,
		reasoning TEXT,
		status TEXT,
		data TEXT,
		created_at DATETIME
	);
	CREATE TABLE IF NOT EXISTS executions (
		id TEXT PRIMARY KEY,
		plan_id TEXT,
		status TEXT,
		history TEXT,
		start_time DATETIME
	);`

	if _, err := db.Exec(schema); err != nil {
		return nil, err
	}

	return &SQLiteRepository{db: db}, nil
}

// --- Plan Management (Satisfies agent.Repository) ---

func (r *SQLiteRepository) SavePlan(ctx context.Context, plan *task.Plan) error {
	data, err := json.Marshal(plan)
	if err != nil {
		return err
	}
	query := `INSERT OR REPLACE INTO plans (id, task, reasoning, status, data, created_at) VALUES (?, ?, ?, ?, ?, ?)`
	_, err = r.db.ExecContext(ctx, query, plan.ID, plan.OriginalTask, plan.Reasoning, plan.Status, string(data), plan.CreatedAt)
	return err
}

func (r *SQLiteRepository) GetPlan(ctx context.Context, id string) (*task.Plan, error) {
	var data string
	err := r.db.QueryRowContext(ctx, "SELECT data FROM plans WHERE id = ?", id).Scan(&data)
	if err != nil {
		return nil, err
	}
	var plan task.Plan
	err = json.Unmarshal([]byte(data), &plan)
	return &plan, err
}

func (r *SQLiteRepository) ListPlans(ctx context.Context, limit int) ([]agent.PlanSummary, error) {
	rows, err := r.db.QueryContext(ctx, "SELECT id, task, status, created_at FROM plans ORDER BY created_at DESC LIMIT ?", limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var summaries []agent.PlanSummary
	for rows.Next() {
		var s agent.PlanSummary
		if err := rows.Scan(&s.ID, &s.OriginalTask, &s.Status, &s.CreatedAt); err != nil {
			return nil, err
		}
		summaries = append(summaries, s)
	}
	return summaries, nil
}

func (r *SQLiteRepository) DeletePlan(ctx context.Context, id string) error {
	_, err := r.db.ExecContext(ctx, "DELETE FROM plans WHERE id = ?", id)
	return err
}

// --- Execution Management (Satisfies agent.Repository) ---

func (r *SQLiteRepository) SaveExecution(ctx context.Context, exec *agent.Execution) error {
	history, _ := json.Marshal(exec.History)
	query := `INSERT OR REPLACE INTO executions (id, plan_id, status, history, start_time) VALUES (?, ?, ?, ?, ?)`
	_, err := r.db.ExecContext(ctx, query, exec.ID, exec.PlanID, exec.Status, string(history), exec.StartTime)
	return err
}

func (r *SQLiteRepository) GetExecution(ctx context.Context, id string) (*agent.Execution, error) {
	var exec agent.Execution
	var historyStr string
	query := `SELECT id, plan_id, status, history, start_time FROM executions WHERE id = ?`
	err := r.db.QueryRowContext(ctx, query, id).Scan(&exec.ID, &exec.PlanID, &exec.Status, &historyStr, &exec.StartTime)
	if err != nil {
		return nil, err
	}
	json.Unmarshal([]byte(historyStr), &exec.History)
	return &exec, nil
}

func (r *SQLiteRepository) ListExecutions(ctx context.Context, limit int) ([]agent.ExecutionSummary, error) {
	rows, err := r.db.QueryContext(ctx, "SELECT id, plan_id, status, start_time FROM executions ORDER BY start_time DESC LIMIT ?", limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []agent.ExecutionSummary
	for rows.Next() {
		var s agent.ExecutionSummary
		if err := rows.Scan(&s.ID, &s.PlanID, &s.Status, &s.StartTime); err != nil {
			return nil, err
		}
		list = append(list, s)
	}
	return list, nil
}

// --- Semantic Code Slices (Satisfies context.SliceStore) ---

func (r *SQLiteRepository) IndexFile(filePath string, slices []domainContext.CodeSlice) error {
	return r.SaveSlices(context.Background(), filePath, slices)
}

func (r *SQLiteRepository) SaveSlices(ctx context.Context, filePath string, slices []domainContext.CodeSlice) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Clear old slices for this file before re-indexing
	_, _ = tx.ExecContext(ctx, "DELETE FROM code_slices WHERE file_path = ?", filePath)

	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO code_slices (id, file_path, content, start_line, end_line, language, slice_type, symbols, hash) 
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, s := range slices {
		symbolsJSON, _ := json.Marshal(s.Symbols)
		_, err := stmt.ExecContext(ctx,
			s.ID,
			s.FilePath,
			s.Content,
			s.StartLine,
			s.EndLine,
			s.Language,
			string(s.SliceType),
			string(symbolsJSON),
			s.Hash,
		)
		if err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (r *SQLiteRepository) SearchSlices(ctx context.Context, query string, limit int) ([]domainContext.CodeSlice, error) {
	// Simple LIKE search implementation
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, file_path, content, start_line, end_line, language, slice_type, symbols, hash 
		FROM code_slices 
		WHERE content LIKE ? 
		LIMIT ?`, "%"+query+"%", limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return r.scanSlices(rows)
}

func (r *SQLiteRepository) GetSlicesByFile(ctx context.Context, filePath string) ([]domainContext.CodeSlice, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, file_path, content, start_line, end_line, language, slice_type, symbols, hash 
		FROM code_slices 
		WHERE file_path = ?`, filePath)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return r.scanSlices(rows)
}

// GetAllSlices is required by cmd/context.go for export
func (r *SQLiteRepository) GetAllSlices() ([]domainContext.CodeSlice, error) {
	rows, err := r.db.Query(`SELECT id, file_path, content, start_line, end_line, language, slice_type, symbols, hash FROM code_slices`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return r.scanSlices(rows)
}

// GetStats satisfies SliceStore.GetStats
func (r *SQLiteRepository) GetStats() (domainContext.IndexStats, error) {
	var stats domainContext.IndexStats
	// Count total slices and distinct files
	err := r.db.QueryRow("SELECT COUNT(*), COUNT(DISTINCT file_path) FROM code_slices").Scan(&stats.TotalSlices, &stats.TotalFiles)
	return stats, err
}

// Helper to avoid duplicating scan logic
func (r *SQLiteRepository) scanSlices(rows *sql.Rows) ([]domainContext.CodeSlice, error) {
	var slices []domainContext.CodeSlice
	for rows.Next() {
		var s domainContext.CodeSlice
		var symbolsStr, typeStr string
		err := rows.Scan(&s.ID, &s.FilePath, &s.Content, &s.StartLine, &s.EndLine, &s.Language, &typeStr, &symbolsStr, &s.Hash)
		if err != nil {
			return nil, err
		}
		s.SliceType = domainContext.SliceType(typeStr)
		json.Unmarshal([]byte(symbolsStr), &s.Symbols)
		slices = append(slices, s)
	}
	return slices, nil
}

func (r *SQLiteRepository) Close() error {
	return r.db.Close()
}
