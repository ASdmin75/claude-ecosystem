package store

import (
	"context"
	"time"
)

// Execution represents a single agent or pipeline execution record.
type Execution struct {
	ID           string     `json:"id"`
	TaskName     string     `json:"task_name"`
	PipelineName string     `json:"pipeline_name,omitempty"`
	Status       string     `json:"status"` // "running", "completed", "failed"
	Trigger      string     `json:"trigger"` // "manual", "schedule", "watcher", "pipeline"
	Prompt       string     `json:"prompt"`
	Output       string     `json:"output,omitempty"`
	Error        string     `json:"error,omitempty"`
	Model        string     `json:"model,omitempty"`
	CostUSD      float64    `json:"cost_usd,omitempty"`
	DurationMS   int64      `json:"duration_ms,omitempty"`
	SessionID    string     `json:"session_id,omitempty"`
	StartedAt    time.Time  `json:"started_at"`
	CompletedAt  *time.Time `json:"completed_at,omitempty"`
	Metadata     string     `json:"metadata,omitempty"` // JSON blob
}

// ExecutionFilter controls which executions are returned by ListExecutions.
type ExecutionFilter struct {
	TaskName     string
	PipelineName string
	Status       string
	Trigger      string
	Limit        int
	Offset       int
}

// ExecutionStore defines persistence operations for execution records.
type ExecutionStore interface {
	CreateExecution(ctx context.Context, exec *Execution) error
	UpdateExecution(ctx context.Context, exec *Execution) error
	GetExecution(ctx context.Context, id string) (*Execution, error)
	ListExecutions(ctx context.Context, filter ExecutionFilter) ([]Execution, error)
}

// User represents an authenticated user.
type User struct {
	ID           string `json:"id"`
	Username     string `json:"username"`
	PasswordHash string `json:"-"`
}

// AuthStore defines persistence operations for user records.
type AuthStore interface {
	GetUserByUsername(ctx context.Context, username string) (*User, error)
	CreateUser(ctx context.Context, user *User) error
}
