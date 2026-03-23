package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"time"

	"github.com/david22573/codepicker/domain/agent"
	domainContext "github.com/david22573/codepicker/domain/context"
	"github.com/david22573/codepicker/domain/task"
	_ "modernc.org/sqlite"
)

type SQLiteRepository struct {
	db *sql.DB
}

func NewSQLiteRepository(dbPath string) (*SQLiteRepository, error) {
	dsn := fmt.Sprintf("%s?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)", dbPath)

	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}

	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(0)

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
		hash TEXT,
		embedding TEXT
	);
	CREATE INDEX IF NOT EXISTS idx_code_slices_file_path ON code_slices(file_path);

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
		start_time DATETIME,
		cost REAL DEFAULT 0.0,
		tokens INTEGER DEFAULT 0,
		tool_cost REAL DEFAULT 0.0,
		llm_cost REAL DEFAULT 0.0
	);
	
	CREATE TABLE IF NOT EXISTS repo_map_cache (
		file_path TEXT PRIMARY KEY,
		data TEXT,
		mod_time DATETIME
	);
	
	CREATE TABLE IF NOT EXISTS sessions (
		id TEXT PRIMARY KEY,
		task TEXT,
		created_at DATETIME,
		messages TEXT,
		edits_made TEXT,
		outcome TEXT
	);

	CREATE TABLE IF NOT EXISTS learnings (
		id TEXT PRIMARY KEY,
		task TEXT,
		note TEXT,
		embedding TEXT,
		created_at DATETIME
	);`

	if _, err := db.Exec(schema); err != nil {
		return nil, err
	}

	migrations := []string{
		"ALTER TABLE executions ADD COLUMN cost REAL DEFAULT 0.0",
		"ALTER TABLE executions ADD COLUMN tokens INTEGER DEFAULT 0",
		"ALTER TABLE executions ADD COLUMN tool_cost REAL DEFAULT 0.0",
		"ALTER TABLE executions ADD COLUMN llm_cost REAL DEFAULT 0.0",
		"ALTER TABLE code_slices ADD COLUMN symbols TEXT",
		"ALTER TABLE code_slices ADD COLUMN hash TEXT",
		"ALTER TABLE code_slices ADD COLUMN embedding TEXT",
		"ALTER TABLE plans ADD COLUMN task TEXT",
		"ALTER TABLE plans ADD COLUMN reasoning TEXT",
		"ALTER TABLE plans ADD COLUMN data TEXT",
	}

	for _, stmt := range migrations {
		_, _ = db.Exec(stmt)
	}

	return &SQLiteRepository{db: db}, nil
}

// --- Phase 4: Session Memory & Learning Methods ---

func (r *SQLiteRepository) SaveSession(ctx context.Context, s *agent.Session) error {
	messages, _ := json.Marshal(s.Messages)
	edits, _ := json.Marshal(s.EditsMade)
	query := `INSERT OR REPLACE INTO sessions (id, task, created_at, messages, edits_made, outcome) VALUES (?, ?, ?, ?, ?, ?)`
	_, err := r.db.ExecContext(ctx, query, s.ID, s.Task, s.CreatedAt, string(messages), string(edits), s.Outcome)
	return err
}

func (r *SQLiteRepository) GetSession(ctx context.Context, id string) (*agent.Session, error) {
	var s agent.Session
	var messagesStr, editsStr string
	query := `SELECT id, task, created_at, messages, edits_made, outcome FROM sessions WHERE id = ?`

	err := r.db.QueryRowContext(ctx, query, id).Scan(&s.ID, &s.Task, &s.CreatedAt, &messagesStr, &editsStr, &s.Outcome)
	if err != nil {
		return nil, err
	}

	json.Unmarshal([]byte(messagesStr), &s.Messages)
	json.Unmarshal([]byte(editsStr), &s.EditsMade)
	return &s, nil
}

func (r *SQLiteRepository) ListSessions(ctx context.Context, limit int) ([]agent.Session, error) {
	rows, err := r.db.QueryContext(ctx, "SELECT id, task, created_at, outcome FROM sessions ORDER BY created_at DESC LIMIT ?", limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sessions []agent.Session
	for rows.Next() {
		var s agent.Session
		if err := rows.Scan(&s.ID, &s.Task, &s.CreatedAt, &s.Outcome); err == nil {
			sessions = append(sessions, s)
		}
	}
	return sessions, nil
}

func (r *SQLiteRepository) SaveLearning(ctx context.Context, l *agent.Learning) error {
	vec, _ := json.Marshal(l.Embedding)
	query := `INSERT INTO learnings (id, task, note, embedding, created_at) VALUES (?, ?, ?, ?, ?)`
	_, err := r.db.ExecContext(ctx, query, l.ID, l.Task, l.Note, string(vec), l.CreatedAt)
	return err
}

func (r *SQLiteRepository) SearchLearnings(ctx context.Context, queryVector []float32, limit int) ([]agent.Learning, error) {
	rows, err := r.db.QueryContext(ctx, "SELECT id, task, note, embedding, created_at FROM learnings WHERE embedding IS NOT NULL")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	type candidate struct {
		l     agent.Learning
		score float64
	}
	var candidates []candidate

	for rows.Next() {
		var l agent.Learning
		var vecStr string
		if err := rows.Scan(&l.ID, &l.Task, &l.Note, &vecStr, &l.CreatedAt); err != nil {
			continue
		}

		var vec []float32
		if err := json.Unmarshal([]byte(vecStr), &vec); err != nil {
			continue
		}
		l.Embedding = vec

		score := cosineSimilarity(queryVector, vec)
		if score > 0.4 {
			candidates = append(candidates, candidate{l: l, score: score})
		}
	}

	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].score > candidates[j].score
	})

	if len(candidates) > limit {
		candidates = candidates[:limit]
	}

	var results []agent.Learning
	for _, c := range candidates {
		results = append(results, c.l)
	}
	return results, nil
}

// --- Phase 1.4: Repo Map Caching Methods ---

func (r *SQLiteRepository) SaveRepoMapCache(ctx context.Context, filePath string, data string, modTime time.Time) error {
	query := `INSERT OR REPLACE INTO repo_map_cache (file_path, data, mod_time) VALUES (?, ?, ?)`
	_, err := r.db.ExecContext(ctx, query, filePath, data, modTime)
	return err
}

func (r *SQLiteRepository) GetRepoMapCache(ctx context.Context) (map[string]string, map[string]time.Time, error) {
	rows, err := r.db.QueryContext(ctx, "SELECT file_path, data, mod_time FROM repo_map_cache")
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()

	dataMap := make(map[string]string)
	timeMap := make(map[string]time.Time)

	for rows.Next() {
		var path, data string
		var modTime time.Time
		if err := rows.Scan(&path, &data, &modTime); err == nil {
			dataMap[path] = data
			timeMap[path] = modTime
		}
	}
	return dataMap, timeMap, nil
}

func (r *SQLiteRepository) DeleteRepoMapCache(ctx context.Context, filePath string) error {
	_, err := r.db.ExecContext(ctx, "DELETE FROM repo_map_cache WHERE file_path = ?", filePath)
	return err
}

// --- General Methods ---

func (r *SQLiteRepository) UpdateSliceEmbedding(ctx context.Context, sliceID string, embedding []float32) error {
	data, err := json.Marshal(embedding)
	if err != nil {
		return err
	}
	_, err = r.db.ExecContext(ctx, "UPDATE code_slices SET embedding = ? WHERE id = ?", string(data), sliceID)
	return err
}

func (r *SQLiteRepository) VectorSearch(ctx context.Context, vector []float32, limit int) ([]agent.SearchResult, error) {
	slices, err := r.SearchByVector(ctx, vector, limit)
	if err != nil {
		return nil, err
	}

	var results []agent.SearchResult
	for _, s := range slices {
		results = append(results, agent.SearchResult{
			FilePath: s.FilePath,
			Content:  s.Content,
			Score:    0.0,
		})
	}
	return results, nil
}

func (r *SQLiteRepository) SearchByVector(ctx context.Context, queryVector []float32, limit int) ([]domainContext.CodeSlice, error) {
	rows, err := r.db.QueryContext(ctx, "SELECT id, embedding FROM code_slices WHERE embedding IS NOT NULL")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	type candidate struct {
		id    string
		score float64
	}
	var candidates []candidate

	for rows.Next() {
		var id, vecStr string
		if err := rows.Scan(&id, &vecStr); err != nil {
			continue
		}

		var vec []float32
		if err := json.Unmarshal([]byte(vecStr), &vec); err != nil {
			continue
		}

		score := cosineSimilarity(queryVector, vec)
		if score > 0.3 {
			candidates = append(candidates, candidate{id: id, score: score})
		}
	}

	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].score > candidates[j].score
	})

	if len(candidates) > limit {
		candidates = candidates[:limit]
	}

	var results []domainContext.CodeSlice
	for _, c := range candidates {
		s, err := r.GetSliceByID(ctx, c.id)
		if err == nil {
			results = append(results, *s)
		}
	}

	return results, nil
}

func cosineSimilarity(a, b []float32) float64 {
	if len(a) != len(b) {
		return 0.0
	}
	var dot, magA, magB float64
	for i := 0; i < len(a); i++ {
		dot += float64(a[i] * b[i])
		magA += float64(a[i] * a[i])
		magB += float64(b[i] * b[i])
	}
	if magA == 0 || magB == 0 {
		return 0.0
	}
	return dot / (math.Sqrt(magA) * math.Sqrt(magB))
}

func (r *SQLiteRepository) GetSliceByID(ctx context.Context, id string) (*domainContext.CodeSlice, error) {
	var s domainContext.CodeSlice
	var symbolsStr, typeStr, hash string
	var embedding sql.NullString

	err := r.db.QueryRowContext(ctx, `
		SELECT id, file_path, content, start_line, end_line, language, slice_type, symbols, hash, embedding 
		FROM code_slices WHERE id = ?`, id).
		Scan(&s.ID, &s.FilePath, &s.Content, &s.StartLine, &s.EndLine, &s.Language, &typeStr, &symbolsStr, &hash, &embedding)

	if err != nil {
		return nil, err
	}

	s.SliceType = domainContext.SliceType(typeStr)
	json.Unmarshal([]byte(symbolsStr), &s.Symbols)
	s.Hash = hash
	return &s, nil
}

func (r *SQLiteRepository) SavePlan(ctx context.Context, plan *task.Plan) error {
	data, err := json.Marshal(plan)
	if err != nil {
		return err
	}
	query := `INSERT OR REPLACE INTO plans (id, task, reasoning, status, data, created_at) VALUES (?, ?, ?, ?, ?, ?)`
	_, err = r.db.ExecContext(ctx, query, plan.ID, plan.OriginalTask, plan.Reasoning, string(plan.Status), string(data), plan.CreatedAt)
	return err
}

func (r *SQLiteRepository) GetPlan(ctx context.Context, id string) (*task.Plan, error) {
	var data string
	err := r.db.QueryRowContext(ctx, "SELECT data FROM plans WHERE id = ?").Scan(&data)
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
		var id, taskStr, statusStr, createdStr string

		if err := rows.Scan(&id, &taskStr, &statusStr, &createdStr); err != nil {
			continue
		}

		s := agent.PlanSummary{
			ID:           id,
			OriginalTask: taskStr,
			Status:       task.Status(statusStr),
		}

		if t, err := time.Parse(time.RFC3339, createdStr); err == nil {
			s.CreatedAt = t
		}

		summaries = append(summaries, s)
	}
	return summaries, nil
}

func (r *SQLiteRepository) DeletePlan(ctx context.Context, id string) error {
	_, err := r.db.ExecContext(ctx, "DELETE FROM plans WHERE id = ?")
	return err
}

func (r *SQLiteRepository) SaveExecution(ctx context.Context, exec *agent.Execution) error {
	history, _ := json.Marshal(exec.History)
	query := `INSERT OR REPLACE INTO executions (id, plan_id, status, history, start_time, cost, tokens, tool_cost, llm_cost) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`
	_, err := r.db.ExecContext(ctx, query, exec.ID, exec.PlanID, string(exec.Status), string(history), exec.StartTime, exec.Cost, exec.Tokens, exec.ToolCost, exec.LLMCost)
	return err
}

func (r *SQLiteRepository) GetExecution(ctx context.Context, id string) (*agent.Execution, error) {
	var exec agent.Execution
	var historyStr, statusStr string
	query := `SELECT id, plan_id, status, history, start_time, cost, tokens, tool_cost, llm_cost FROM executions WHERE id = ?`

	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&exec.ID,
		&exec.PlanID,
		&statusStr,
		&historyStr,
		&exec.StartTime,
		&exec.Cost,
		&exec.Tokens,
		&exec.ToolCost,
		&exec.LLMCost,
	)
	if err != nil {
		return nil, err
	}

	exec.Status = task.Status(statusStr)
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
		var statusStr string
		if err := rows.Scan(&s.ID, &s.PlanID, &statusStr, &s.StartTime); err != nil {
			return nil, err
		}
		s.Status = task.Status(statusStr)
		list = append(list, s)
	}
	return list, nil
}

func (r *SQLiteRepository) GetTotalCost(ctx context.Context) (float64, int, error) {
	var totalCost float64
	var totalTokens int
	err := r.db.QueryRowContext(ctx, "SELECT COALESCE(SUM(cost), 0), COALESCE(SUM(tokens), 0) FROM executions").Scan(&totalCost, &totalTokens)
	return totalCost, totalTokens, err
}

func (r *SQLiteRepository) IndexFile(filePath string, slices []domainContext.CodeSlice) error {
	return r.SaveSlices(context.Background(), filePath, slices)
}

func (r *SQLiteRepository) SaveSlices(ctx context.Context, filePath string, slices []domainContext.CodeSlice) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	_, _ = tx.ExecContext(ctx, "DELETE FROM code_slices WHERE file_path = ?", filePath)

	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO code_slices (id, file_path, content, start_line, end_line, language, slice_type, symbols, hash, embedding) 
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, s := range slices {
		symbolsJSON, _ := json.Marshal(s.Symbols)
		_, err := stmt.ExecContext(ctx, s.ID, s.FilePath, s.Content, s.StartLine, s.EndLine, s.Language, string(s.SliceType), string(symbolsJSON), s.Hash, nil)
		if err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (r *SQLiteRepository) SearchSlices(ctx context.Context, query string, limit int) ([]domainContext.CodeSlice, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, file_path, content, start_line, end_line, language, slice_type, symbols, hash, embedding 
		FROM code_slices 
		WHERE content LIKE ? LIMIT ?`, "%"+query+"%", limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return r.scanSlices(rows)
}

func (r *SQLiteRepository) GetSlicesByFile(ctx context.Context, filePath string) ([]domainContext.CodeSlice, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, file_path, content, start_line, end_line, language, slice_type, symbols, hash, embedding 
		FROM code_slices 
		WHERE file_path = ?`, filePath)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return r.scanSlices(rows)
}

func (r *SQLiteRepository) GetAllSlices() ([]domainContext.CodeSlice, error) {
	rows, err := r.db.Query(`SELECT id, file_path, content, start_line, end_line, language, slice_type, symbols, hash, embedding FROM code_slices`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return r.scanSlices(rows)
}

func (r *SQLiteRepository) GetStats() (domainContext.IndexStats, error) {
	var stats domainContext.IndexStats
	err := r.db.QueryRow("SELECT COUNT(*), COUNT(DISTINCT file_path) FROM code_slices").Scan(&stats.TotalSlices, &stats.TotalFiles)
	return stats, err
}

func (r *SQLiteRepository) scanSlices(rows *sql.Rows) ([]domainContext.CodeSlice, error) {
	var slices []domainContext.CodeSlice
	for rows.Next() {
		var s domainContext.CodeSlice
		var symbolsStr, typeStr string
		var embedding sql.NullString

		if err := rows.Scan(&s.ID, &s.FilePath, &s.Content, &s.StartLine, &s.EndLine, &s.Language, &typeStr, &symbolsStr, &s.Hash, &embedding); err != nil {
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
