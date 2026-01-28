package storage

import (
	"context"
	"database/sql"
	"encoding/json"

	"github.com/david22573/codepicker/domain/agent"
	"github.com/david22573/codepicker/domain/errors"
	"github.com/david22573/codepicker/domain/task"
	_ "modernc.org/sqlite"
)

// SQLiteRepository implements domain.agent.Repository
type SQLiteRepository struct {
	db *sql.DB
}

func NewSQLiteRepository(path string) (*SQLiteRepository, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}

	if err := migrate(db); err != nil {
		db.Close()
		return nil, err
	}

	return &SQLiteRepository{db: db}, nil
}

func migrate(db *sql.DB) error {
	// 1. Executions Table
	queryExec := `
	CREATE TABLE IF NOT EXISTS executions (
		id TEXT PRIMARY KEY,
		plan_id TEXT,
		status TEXT,
		history_json TEXT,
		start_time DATETIME,
		end_time DATETIME
	);
	`
	if _, err := db.Exec(queryExec); err != nil {
		return err
	}

	// 2. Plans Table
	queryPlans := `
	CREATE TABLE IF NOT EXISTS plans (
		id TEXT PRIMARY KEY,
		original_task TEXT,
		reasoning TEXT,
		steps_json TEXT,
		status TEXT,
		estimated_cost REAL,
		created_at DATETIME
	);
	`
	_, err := db.Exec(queryPlans)
	return err
}

// --- Execution Methods ---

func (r *SQLiteRepository) ListExecutions(ctx context.Context, limit int) ([]agent.ExecutionSummary, error) {
	query := `
	SELECT id, plan_id, status, start_time 
	FROM executions 
	ORDER BY start_time DESC 
	LIMIT ?
	`

	rows, err := r.db.QueryContext(ctx, query, limit)
	if err != nil {
		return nil, errors.NewSystem("repo.ListExecutions", "query failed", err)
	}
	defer rows.Close()

	var summaries []agent.ExecutionSummary
	for rows.Next() {
		var s agent.ExecutionSummary
		var statusStr string
		if err := rows.Scan(&s.ID, &s.PlanID, &statusStr, &s.StartTime); err != nil {
			return nil, errors.NewSystem("repo.ListExecutions", "scan failed", err)
		}
		s.Status = task.Status(statusStr)
		summaries = append(summaries, s)
	}

	return summaries, nil
}

func (r *SQLiteRepository) SaveExecution(ctx context.Context, exec *agent.Execution) error {
	historyBytes, err := json.Marshal(exec.History)
	if err != nil {
		return errors.NewSystem("repo.SaveExecution", "json marshal failed", err)
	}

	query := `
	INSERT INTO executions (id, plan_id, status, history_json, start_time, end_time)
	VALUES (?, ?, ?, ?, ?, ?)
	ON CONFLICT(id) DO UPDATE SET
		status=excluded.status,
		history_json=excluded.history_json,
		end_time=excluded.end_time;
	`

	_, err = r.db.ExecContext(ctx, query,
		exec.ID,
		exec.PlanID,
		string(exec.Status),
		string(historyBytes),
		exec.StartTime,
		exec.EndTime,
	)

	if err != nil {
		return errors.NewSystem("repo.SaveExecution", "db insert failed", err)
	}
	return nil
}

func (r *SQLiteRepository) GetExecution(ctx context.Context, id string) (*agent.Execution, error) {
	row := r.db.QueryRowContext(ctx, "SELECT id, plan_id, status, history_json, start_time, end_time FROM executions WHERE id = ?", id)

	var ex agent.Execution
	var historyRaw string
	var statusStr string
	var endTime sql.NullTime

	if err := row.Scan(&ex.ID, &ex.PlanID, &statusStr, &historyRaw, &ex.StartTime, &endTime); err != nil {
		if err == sql.ErrNoRows {
			return nil, errors.NewValidation("repo.GetExecution", "execution not found")
		}
		return nil, errors.NewSystem("repo.GetExecution", "db select failed", err)
	}

	ex.Status = task.Status(statusStr)
	if endTime.Valid {
		ex.EndTime = endTime.Time
	}

	if err := json.Unmarshal([]byte(historyRaw), &ex.History); err != nil {
		return nil, errors.NewSystem("repo.GetExecution", "history corruption", err)
	}

	return &ex, nil
}

// --- Plan Methods ---

func (r *SQLiteRepository) SavePlan(ctx context.Context, plan *task.Plan) error {
	stepsBytes, err := json.Marshal(plan.Steps)
	if err != nil {
		return errors.NewSystem("repo.SavePlan", "json marshal failed", err)
	}

	query := `
	INSERT INTO plans (id, original_task, reasoning, steps_json, status, estimated_cost, created_at)
	VALUES (?, ?, ?, ?, ?, ?, ?)
	ON CONFLICT(id) DO UPDATE SET
		status=excluded.status,
		steps_json=excluded.steps_json,
		reasoning=excluded.reasoning;
	`

	_, err = r.db.ExecContext(ctx, query,
		plan.ID,
		plan.OriginalTask,
		plan.Reasoning,
		string(stepsBytes),
		string(plan.Status),
		plan.EstimatedCost,
		plan.CreatedAt,
	)

	if err != nil {
		return errors.NewSystem("repo.SavePlan", "db insert failed", err)
	}
	return nil
}

func (r *SQLiteRepository) GetPlan(ctx context.Context, id string) (*task.Plan, error) {
	query := `SELECT id, original_task, reasoning, steps_json, status, estimated_cost, created_at FROM plans WHERE id = ?`
	row := r.db.QueryRowContext(ctx, query, id)

	var p task.Plan
	var stepsRaw string
	var statusStr string

	if err := row.Scan(&p.ID, &p.OriginalTask, &p.Reasoning, &stepsRaw, &statusStr, &p.EstimatedCost, &p.CreatedAt); err != nil {
		if err == sql.ErrNoRows {
			return nil, errors.NewValidation("repo.GetPlan", "plan not found")
		}
		return nil, errors.NewSystem("repo.GetPlan", "db select failed", err)
	}

	p.Status = task.Status(statusStr)
	if err := json.Unmarshal([]byte(stepsRaw), &p.Steps); err != nil {
		return nil, errors.NewSystem("repo.GetPlan", "steps corruption", err)
	}

	return &p, nil
}

func (r *SQLiteRepository) ListPlans(ctx context.Context, limit int) ([]agent.PlanSummary, error) {
	query := `
	SELECT id, original_task, status, steps_json, created_at 
	FROM plans 
	ORDER BY created_at DESC 
	LIMIT ?`

	rows, err := r.db.QueryContext(ctx, query, limit)
	if err != nil {
		return nil, errors.NewSystem("repo.ListPlans", "query failed", err)
	}
	defer rows.Close()

	var summaries []agent.PlanSummary
	for rows.Next() {
		var p agent.PlanSummary
		var stepsRaw string
		var statusStr string

		if err := rows.Scan(&p.ID, &p.OriginalTask, &statusStr, &stepsRaw, &p.CreatedAt); err != nil {
			return nil, err
		}

		p.Status = task.Status(statusStr)

		// Parse steps just to get the count
		var steps []task.Step
		if err := json.Unmarshal([]byte(stepsRaw), &steps); err == nil {
			p.StepCount = len(steps)
		}

		summaries = append(summaries, p)
	}
	return summaries, nil
}

func (r *SQLiteRepository) DeletePlan(ctx context.Context, id string) error {
	_, err := r.db.ExecContext(ctx, "DELETE FROM plans WHERE id = ?", id)
	if err != nil {
		return errors.NewSystem("repo.DeletePlan", "delete failed", err)
	}
	return nil
}
