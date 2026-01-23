package database

import (
	"database/sql"
	"fmt"
	"log"
)

type Migration struct {
	Version int
	Up      string
}

var migrations = []Migration{
	{
		Version: 1,
		Up: `
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
		`,
	},
	{
		Version: 2,
		Up: `
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
		CREATE INDEX IF NOT EXISTS idx_jobs_status ON jobs(status);
		CREATE INDEX IF NOT EXISTS idx_jobs_priority ON jobs(priority DESC, created_at ASC);
		`,
	},
	{
		Version: 3,
		Up: `
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
		`,
	},
	{
		Version: 4,
		Up: `
		ALTER TABLE memory_files ADD COLUMN content_hash TEXT DEFAULT '';
		`,
	},
	{
		Version: 5,
		Up: `
		ALTER TABLE memory_files ADD COLUMN access_count INTEGER DEFAULT 1;
		`,
	},
	{
		Version: 6,
		Up: `
		CREATE TABLE IF NOT EXISTS checkpoints (
			id TEXT PRIMARY KEY,
			session_id TEXT NOT NULL,
			plan_id TEXT,
			task TEXT NOT NULL,
			timestamp DATETIME DEFAULT CURRENT_TIMESTAMP,
			
			-- Execution State
			current_step INTEGER DEFAULT 0,
			steps_status TEXT, -- JSON map: step_id -> status
			step_results TEXT, -- JSON map: step_id -> result
			turn_count INTEGER DEFAULT 0,
			error_count INTEGER DEFAULT 0,
			last_error TEXT,
			last_tool_used TEXT,
			progress REAL DEFAULT 0.0,
			status TEXT DEFAULT 'active',
			
			-- Cost Tracking
			total_cost REAL DEFAULT 0.0,
			request_count INTEGER DEFAULT 0,
			
			-- Session Approvals
			approved_write BOOLEAN DEFAULT 0,
			approved_exec BOOLEAN DEFAULT 0,
			
			-- Memory State (JSON)
			memory_snapshot TEXT,
			
			-- Shadow Workspace State
			shadow_files TEXT, -- JSON map: path -> content_hash
			shadow_manifest TEXT,
			
			-- Metadata
			agent_model TEXT,
			worker_model TEXT,
			policy_name TEXT,
			metadata TEXT -- JSON map for additional fields
		);
		
		CREATE INDEX IF NOT EXISTS idx_checkpoints_session ON checkpoints(session_id);
		CREATE INDEX IF NOT EXISTS idx_checkpoints_timestamp ON checkpoints(timestamp DESC);
		CREATE INDEX IF NOT EXISTS idx_checkpoints_status ON checkpoints(status);
		`,
	},
}

func Migrate(db *sql.DB) error {

	var currentVersion int
	row := db.QueryRow("PRAGMA user_version")
	if err := row.Scan(&currentVersion); err != nil {
		return fmt.Errorf("failed to fetch user_version: %w", err)
	}

	for _, m := range migrations {
		if m.Version > currentVersion {
			log.Printf("Applying database migration v%d...", m.Version)

			tx, err := db.Begin()
			if err != nil {
				return err
			}

			if _, err := tx.Exec(m.Up); err != nil {
				tx.Rollback()
				return fmt.Errorf("migration v%d failed: %w", m.Version, err)
			}

			if _, err := tx.Exec(fmt.Sprintf("PRAGMA user_version = %d", m.Version)); err != nil {
				tx.Rollback()
				return fmt.Errorf("failed to update user_version: %w", err)
			}

			if err := tx.Commit(); err != nil {
				return err
			}
		}
	}

	return nil
}
