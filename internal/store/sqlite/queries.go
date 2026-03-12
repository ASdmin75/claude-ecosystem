package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/asdmin/claude-ecosystem/internal/store"
)

// CreateExecution inserts a new execution record.
func (s *Store) CreateExecution(ctx context.Context, exec *store.Execution) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO executions
			(id, task_name, pipeline_name, status, trigger, prompt, output, error, model,
			 cost_usd, duration_ms, session_id, started_at, completed_at, metadata)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		exec.ID, exec.TaskName, exec.PipelineName, exec.Status, exec.Trigger,
		exec.Prompt, exec.Output, exec.Error, exec.Model,
		exec.CostUSD, exec.DurationMS, exec.SessionID,
		exec.StartedAt, exec.CompletedAt, exec.Metadata,
	)
	if err != nil {
		return fmt.Errorf("create execution: %w", err)
	}
	return nil
}

// UpdateExecution updates an existing execution record by ID.
func (s *Store) UpdateExecution(ctx context.Context, exec *store.Execution) error {
	result, err := s.db.ExecContext(ctx,
		`UPDATE executions SET
			task_name = ?, pipeline_name = ?, status = ?, trigger = ?,
			prompt = ?, output = ?, error = ?, model = ?,
			cost_usd = ?, duration_ms = ?, session_id = ?,
			started_at = ?, completed_at = ?, metadata = ?
		 WHERE id = ?`,
		exec.TaskName, exec.PipelineName, exec.Status, exec.Trigger,
		exec.Prompt, exec.Output, exec.Error, exec.Model,
		exec.CostUSD, exec.DurationMS, exec.SessionID,
		exec.StartedAt, exec.CompletedAt, exec.Metadata,
		exec.ID,
	)
	if err != nil {
		return fmt.Errorf("update execution: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("update execution rows affected: %w", err)
	}
	if rows == 0 {
		return fmt.Errorf("execution not found: %s", exec.ID)
	}
	return nil
}

// GetExecution retrieves a single execution by ID.
func (s *Store) GetExecution(ctx context.Context, id string) (*store.Execution, error) {
	var exec store.Execution
	err := s.db.QueryRowContext(ctx,
		`SELECT id, task_name, pipeline_name, status, trigger, prompt, output, error,
		        model, cost_usd, duration_ms, session_id, started_at, completed_at, metadata
		 FROM executions WHERE id = ?`, id,
	).Scan(
		&exec.ID, &exec.TaskName, &exec.PipelineName, &exec.Status, &exec.Trigger,
		&exec.Prompt, &exec.Output, &exec.Error,
		&exec.Model, &exec.CostUSD, &exec.DurationMS, &exec.SessionID,
		&exec.StartedAt, &exec.CompletedAt, &exec.Metadata,
	)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("execution not found: %s", id)
	}
	if err != nil {
		return nil, fmt.Errorf("get execution: %w", err)
	}
	return &exec, nil
}

// ListExecutions returns executions matching the given filter.
// Default limit is 50 if not specified.
func (s *Store) ListExecutions(ctx context.Context, filter store.ExecutionFilter) ([]store.Execution, error) {
	var conditions []string
	var args []any

	if filter.TaskName != "" {
		conditions = append(conditions, "task_name = ?")
		args = append(args, filter.TaskName)
	}
	if filter.PipelineName != "" {
		conditions = append(conditions, "pipeline_name = ?")
		args = append(args, filter.PipelineName)
	}
	if filter.Status != "" {
		conditions = append(conditions, "status = ?")
		args = append(args, filter.Status)
	}
	if filter.Trigger != "" {
		conditions = append(conditions, "trigger = ?")
		args = append(args, filter.Trigger)
	}

	query := "SELECT id, task_name, pipeline_name, status, trigger, prompt, output, error, model, cost_usd, duration_ms, session_id, started_at, completed_at, metadata FROM executions"
	if len(conditions) > 0 {
		query += " WHERE " + strings.Join(conditions, " AND ")
	}
	query += " ORDER BY started_at DESC"

	limit := filter.Limit
	if limit <= 0 {
		limit = 50
	}
	query += " LIMIT ?"
	args = append(args, limit)

	if filter.Offset > 0 {
		query += " OFFSET ?"
		args = append(args, filter.Offset)
	}

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list executions: %w", err)
	}
	defer rows.Close()

	var executions []store.Execution
	for rows.Next() {
		var exec store.Execution
		if err := rows.Scan(
			&exec.ID, &exec.TaskName, &exec.PipelineName, &exec.Status, &exec.Trigger,
			&exec.Prompt, &exec.Output, &exec.Error,
			&exec.Model, &exec.CostUSD, &exec.DurationMS, &exec.SessionID,
			&exec.StartedAt, &exec.CompletedAt, &exec.Metadata,
		); err != nil {
			return nil, fmt.Errorf("scan execution: %w", err)
		}
		executions = append(executions, exec)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate executions: %w", err)
	}

	return executions, nil
}

// DeleteExecution removes a single execution record by ID.
func (s *Store) DeleteExecution(ctx context.Context, id string) error {
	result, err := s.db.ExecContext(ctx, `DELETE FROM executions WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete execution: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("delete execution rows affected: %w", err)
	}
	if rows == 0 {
		return fmt.Errorf("execution not found: %s", id)
	}
	return nil
}

// MarkStaleRunning updates all executions stuck in "running" status to "failed".
// This handles cases where the server restarted or a task timed out without updating the DB.
func (s *Store) MarkStaleRunning(ctx context.Context) (int64, error) {
	result, err := s.db.ExecContext(ctx,
		`UPDATE executions SET status = 'failed', error = 'server restarted or timeout (stale)'
		 WHERE status = 'running'`,
	)
	if err != nil {
		return 0, fmt.Errorf("mark stale running: %w", err)
	}
	return result.RowsAffected()
}

// GetUserByUsername retrieves a user by username.
func (s *Store) GetUserByUsername(ctx context.Context, username string) (*store.User, error) {
	var user store.User
	err := s.db.QueryRowContext(ctx,
		`SELECT id, username, password_hash FROM users WHERE username = ?`, username,
	).Scan(&user.ID, &user.Username, &user.PasswordHash)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("user not found: %s", username)
	}
	if err != nil {
		return nil, fmt.Errorf("get user by username: %w", err)
	}
	return &user, nil
}

// CreateUser inserts a new user record.
func (s *Store) CreateUser(ctx context.Context, user *store.User) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO users (id, username, password_hash) VALUES (?, ?, ?)`,
		user.ID, user.Username, user.PasswordHash,
	)
	if err != nil {
		return fmt.Errorf("create user: %w", err)
	}
	return nil
}
