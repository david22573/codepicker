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
	query := `
	CREATE TABLE IF NOT EXISTS executions (
		id TEXT PRIMARY KEY,
		plan_id TEXT,
		status TEXT,
		history_json TEXT,
		start_time DATETIME,
		end_time DATETIME
	);
	`
	_, err := db.Exec(query)
	return err
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
