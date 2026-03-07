package api

import (
	"context"
	"net/http"
	"time"

	"github.com/asdmin/claude-ecosystem/internal/events"
	"github.com/asdmin/claude-ecosystem/internal/store"
	"github.com/asdmin/claude-ecosystem/internal/task"
	"github.com/google/uuid"
)

// runTaskRequest is the optional JSON body for task run endpoints.
type runTaskRequest struct {
	TemplateVars map[string]string `json:"template_vars,omitempty"`
}

// runTaskResponse is the JSON response for a synchronous task run.
type runTaskResponse struct {
	ExecutionID string  `json:"execution_id"`
	TaskName    string  `json:"task_name"`
	Status      string  `json:"status"`
	Output      string  `json:"output,omitempty"`
	Error       string  `json:"error,omitempty"`
	Model       string  `json:"model,omitempty"`
	CostUSD     float64 `json:"cost_usd,omitempty"`
	DurationMS  int64   `json:"duration_ms"`
}

// asyncRunResponse is the JSON response for an async task run.
type asyncRunResponse struct {
	ExecutionID string `json:"execution_id"`
	Status      string `json:"status"`
}

// handleListTasks returns all tasks from the config.
// GET /api/v1/tasks
func (s *Server) handleListTasks(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.cfg.Tasks)
}

// handleGetTask returns a single task by name.
// GET /api/v1/tasks/{name}
func (s *Server) handleGetTask(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	t := s.findTask(name)
	if t == nil {
		writeError(w, http.StatusNotFound, "task not found: "+name)
		return
	}
	writeJSON(w, http.StatusOK, t)
}

// handleRunTask runs a task synchronously and returns the result.
// POST /api/v1/tasks/{name}/run
func (s *Server) handleRunTask(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	t := s.findTask(name)
	if t == nil {
		writeError(w, http.StatusNotFound, "task not found: "+name)
		return
	}

	var req runTaskRequest
	// Body is optional; ignore decode errors for empty bodies.
	_ = readJSON(r, &req)

	execID := uuid.New().String()
	now := time.Now().UTC()

	exec := &store.Execution{
		ID:        execID,
		TaskName:  t.Name,
		Status:    "running",
		Trigger:   "manual",
		Prompt:    t.Prompt,
		StartedAt: now,
	}
	if err := s.store.CreateExecution(r.Context(), exec); err != nil {
		s.logger.Error("failed to create execution record", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to create execution record")
		return
	}

	timeout := t.ParsedTimeout()
	ctx, cancel := context.WithTimeout(r.Context(), timeout)
	defer cancel()

	result := s.taskRunner.Run(ctx, *t, task.RunOptions{}, req.TemplateVars)

	completedAt := time.Now().UTC()
	status := "completed"
	if result.Error != "" {
		status = "failed"
	}

	exec.Status = status
	exec.Output = result.Output
	exec.Error = result.Error
	exec.Model = result.Model
	exec.CostUSD = result.CostUSD
	exec.DurationMS = result.Duration.Milliseconds()
	exec.SessionID = result.SessionID
	exec.CompletedAt = &completedAt

	if err := s.store.UpdateExecution(r.Context(), exec); err != nil {
		s.logger.Error("failed to update execution record", "error", err)
	}

	s.bus.Publish(events.Event{
		Type: "task.completed",
		Payload: map[string]string{
			"execution_id": execID,
			"task":         t.Name,
			"status":       status,
			"output":       result.Output,
			"error":        result.Error,
		},
	})

	writeJSON(w, http.StatusOK, runTaskResponse{
		ExecutionID: execID,
		TaskName:    t.Name,
		Status:      status,
		Output:      result.Output,
		Error:       result.Error,
		Model:       result.Model,
		CostUSD:     result.CostUSD,
		DurationMS:  result.Duration.Milliseconds(),
	})
}

// handleRunTaskAsync starts a task asynchronously and returns the execution ID.
// POST /api/v1/tasks/{name}/run-async
func (s *Server) handleRunTaskAsync(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	t := s.findTask(name)
	if t == nil {
		writeError(w, http.StatusNotFound, "task not found: "+name)
		return
	}

	var req runTaskRequest
	_ = readJSON(r, &req)

	execID := uuid.New().String()
	now := time.Now().UTC()

	exec := &store.Execution{
		ID:        execID,
		TaskName:  t.Name,
		Status:    "running",
		Trigger:   "manual",
		Prompt:    t.Prompt,
		StartedAt: now,
	}
	if err := s.store.CreateExecution(r.Context(), exec); err != nil {
		s.logger.Error("failed to create execution record", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to create execution record")
		return
	}

	// Run in background goroutine.
	taskCopy := *t
	templateVars := req.TemplateVars
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), taskCopy.ParsedTimeout())
		defer cancel()

		result := s.taskRunner.Run(ctx, taskCopy, task.RunOptions{}, templateVars)

		completedAt := time.Now().UTC()
		status := "completed"
		if result.Error != "" {
			status = "failed"
		}

		exec.Status = status
		exec.Output = result.Output
		exec.Error = result.Error
		exec.Model = result.Model
		exec.CostUSD = result.CostUSD
		exec.DurationMS = result.Duration.Milliseconds()
		exec.SessionID = result.SessionID
		exec.CompletedAt = &completedAt

		if err := s.store.UpdateExecution(ctx, exec); err != nil {
			s.logger.Error("failed to update async execution record", "error", err)
		}

		s.bus.Publish(events.Event{
			Type: "task.completed",
			Payload: map[string]string{
				"execution_id": execID,
				"task":         taskCopy.Name,
				"status":       status,
				"output":       result.Output,
				"error":        result.Error,
			},
		})
	}()

	writeJSON(w, http.StatusAccepted, asyncRunResponse{
		ExecutionID: execID,
		Status:      "running",
	})
}
