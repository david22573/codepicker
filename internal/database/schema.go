package database

import (
	"database/sql"
	"fmt"
	"log"
)

// Migration represents a single database change
type Migration struct {
	Version int
	Up      string
}

// migrations defines the schema evolution of the application.
// DO NOT reorder or modify existing entries; only append new versions.
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
}

// Migrate brings the database schema up to date.
func Migrate(db *sql.DB) error {
	// 1. Check current version
	var currentVersion int
	row := db.QueryRow("PRAGMA user_version")
	if err := row.Scan(&currentVersion); err != nil {
		return fmt.Errorf("failed to fetch user_version: %w", err)
	}

	// 2. Apply pending migrations
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

			// Update version
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
