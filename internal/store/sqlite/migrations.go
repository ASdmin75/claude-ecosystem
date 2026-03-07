package sqlite

import "fmt"

// migrate creates the database schema if it does not already exist.
func (s *Store) migrate() error {
	statements := []string{
		`CREATE TABLE IF NOT EXISTS executions (
			id           TEXT PRIMARY KEY,
			task_name    TEXT NOT NULL,
			pipeline_name TEXT NOT NULL DEFAULT '',
			status       TEXT NOT NULL,
			trigger      TEXT NOT NULL,
			prompt       TEXT NOT NULL DEFAULT '',
			output       TEXT NOT NULL DEFAULT '',
			error        TEXT NOT NULL DEFAULT '',
			model        TEXT NOT NULL DEFAULT '',
			cost_usd     REAL NOT NULL DEFAULT 0,
			duration_ms  INTEGER NOT NULL DEFAULT 0,
			session_id   TEXT NOT NULL DEFAULT '',
			started_at   DATETIME NOT NULL,
			completed_at DATETIME,
			metadata     TEXT NOT NULL DEFAULT ''
		)`,
		`CREATE TABLE IF NOT EXISTS users (
			id            TEXT PRIMARY KEY,
			username      TEXT UNIQUE NOT NULL,
			password_hash TEXT NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_executions_task_name ON executions(task_name)`,
		`CREATE INDEX IF NOT EXISTS idx_executions_status ON executions(status)`,
		`CREATE INDEX IF NOT EXISTS idx_executions_started_at ON executions(started_at)`,
	}

	for _, stmt := range statements {
		if _, err := s.db.Exec(stmt); err != nil {
			return fmt.Errorf("migration failed: %w", err)
		}
	}

	return nil
}
