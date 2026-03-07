package task

import "time"

// Result holds the outcome of a completed task execution.
type Result struct {
	TaskName  string        `json:"task_name"`
	Output    string        `json:"output"`
	Duration  time.Duration `json:"duration"`
	Error     string        `json:"error,omitempty"`
	Model     string        `json:"model,omitempty"`
	SessionID string        `json:"session_id,omitempty"`
	CostUSD   float64       `json:"cost_usd,omitempty"`
}

// StreamChunk represents a single chunk of streaming output from the claude CLI.
type StreamChunk struct {
	Type     string `json:"type"`      // "assistant", "result", "error"
	Content  string `json:"content"`
	TaskName string `json:"task_name"`
}
