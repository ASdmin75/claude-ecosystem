package api

import (
	"context"
	"net/http"
	"time"

	"github.com/asdmin/claude-ecosystem/internal/config"
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

// handleCreateTask creates a new task and persists to disk.
// POST /api/v1/tasks
func (s *Server) handleCreateTask(w http.ResponseWriter, r *http.Request) {
	var task config.Task
	if err := readJSON(r, &task); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	if task.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	if task.Prompt == "" {
		writeError(w, http.StatusBadRequest, "prompt is required")
		return
	}

	if s.findTask(task.Name) != nil {
		writeError(w, http.StatusConflict, "task already exists: "+task.Name)
		return
	}

	s.cfg.Tasks = append(s.cfg.Tasks, task)

	if err := s.cfg.Save(); err != nil {
		// Rollback
		s.cfg.Tasks = s.cfg.Tasks[:len(s.cfg.Tasks)-1]
		s.logger.Error("failed to save config", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to save config: "+err.Error())
		return
	}

	s.logger.Info("task created", "name", task.Name)
	writeJSON(w, http.StatusCreated, s.findTask(task.Name))
}

// handleUpdateTask updates a task's configuration in-memory and persists to disk.
// PUT /api/v1/tasks/{name}
func (s *Server) handleUpdateTask(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	var updated config.Task
	if err := readJSON(r, &updated); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	// Find existing task
	var found bool
	for i := range s.cfg.Tasks {
		if s.cfg.Tasks[i].Name == name {
			// Preserve the original name (rename not supported)
			updated.Name = name
			s.cfg.Tasks[i] = updated
			found = true
			break
		}
	}
	if !found {
		writeError(w, http.StatusNotFound, "task not found: "+name)
		return
	}

	// Persist to disk
	if err := s.cfg.Save(); err != nil {
		s.logger.Error("failed to save config", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to save config: "+err.Error())
		return
	}

	s.logger.Info("task updated", "name", name)
	writeJSON(w, http.StatusOK, s.findTask(name))
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

	if !t.ConcurrentAllowed() {
		if !s.guard.TryAcquire("task:" + t.Name) {
			writeError(w, http.StatusConflict, "task is already running: "+t.Name)
			return
		}
		defer s.guard.Release("task:" + t.Name)
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

	opts, cleanup, resolveErr := task.ResolveRunOptions(*t, s.subagentMgr, s.mcpMgr, s.domainMgr)
	if resolveErr != nil {
		s.logger.Error("failed to resolve run options", "task", t.Name, "error", resolveErr)
		writeError(w, http.StatusInternalServerError, "failed to resolve run options: "+resolveErr.Error())
		return
	}
	if cleanup != nil {
		defer cleanup()
	}

	s.bus.Publish(events.Event{
		Type: "task.started",
		Payload: map[string]string{
			"execution_id": execID,
			"task":         t.Name,
		},
	})

	timeout := t.ParsedTimeout()
	ctx, cancel := context.WithTimeout(r.Context(), timeout)
	defer cancel()

	result := s.taskRunner.Run(ctx, *t, opts, req.TemplateVars)

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

	dbCtx, dbCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer dbCancel()
	if err := s.store.UpdateExecution(dbCtx, exec); err != nil {
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

	if !t.ConcurrentAllowed() {
		if !s.guard.TryAcquire("task:" + t.Name) {
			writeError(w, http.StatusConflict, "task is already running: "+t.Name)
			return
		}
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
		if !t.ConcurrentAllowed() {
			s.guard.Release("task:" + t.Name)
		}
		writeError(w, http.StatusInternalServerError, "failed to create execution record")
		return
	}

	// Resolve options before launching goroutine to report errors synchronously.
	asyncOpts, asyncCleanup, asyncResolveErr := task.ResolveRunOptions(*t, s.subagentMgr, s.mcpMgr, s.domainMgr)
	if asyncResolveErr != nil {
		s.logger.Error("failed to resolve run options", "task", t.Name, "error", asyncResolveErr)
		if !t.ConcurrentAllowed() {
			s.guard.Release("task:" + t.Name)
		}
		writeError(w, http.StatusInternalServerError, "failed to resolve run options: "+asyncResolveErr.Error())
		return
	}

	s.bus.Publish(events.Event{
		Type: "task.started",
		Payload: map[string]string{
			"execution_id": execID,
			"task":         t.Name,
		},
	})

	// Run in background goroutine.
	taskCopy := *t
	templateVars := req.TemplateVars
	go func() {
		if !taskCopy.ConcurrentAllowed() {
			defer s.guard.Release("task:" + taskCopy.Name)
		}
		if asyncCleanup != nil {
			defer asyncCleanup()
		}

		ctx, cancel := context.WithTimeout(context.Background(), taskCopy.ParsedTimeout())
		defer cancel()

		s.cancels.Store(execID, cancel)
		defer s.cancels.Delete(execID)

		result := s.taskRunner.Run(ctx, taskCopy, asyncOpts, templateVars)

		completedAt := time.Now().UTC()
		status := "completed"
		if ctx.Err() == context.Canceled {
			status = "cancelled"
		} else if result.Error != "" {
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

		// Use a fresh context — the task ctx may be cancelled due to timeout.
		dbCtx, dbCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer dbCancel()
		if err := s.store.UpdateExecution(dbCtx, exec); err != nil {
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

// handleCancelExecution cancels a running execution.
// POST /api/v1/executions/{id}/cancel
func (s *Server) handleCancelExecution(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	// Try in-memory cancel first (active process).
	if cancelFn, ok := s.cancels.Load(id); ok {
		cancelFn.(context.CancelFunc)()
		s.logger.Info("execution cancelled", "id", id)
		writeJSON(w, http.StatusOK, map[string]string{"status": "cancelled", "execution_id": id})
		return
	}

	// Fallback: mark stale "running" execution as cancelled in DB
	// (e.g. process lost after server restart).
	exec, err := s.store.GetExecution(r.Context(), id)
	if err != nil || exec == nil {
		writeError(w, http.StatusNotFound, "execution not found: "+id)
		return
	}
	if exec.Status != "running" {
		writeError(w, http.StatusConflict, "execution is not running: "+exec.Status)
		return
	}

	now := time.Now().UTC()
	exec.Status = "cancelled"
	exec.Error = "cancelled by user (process no longer tracked)"
	exec.CompletedAt = &now
	if err := s.store.UpdateExecution(r.Context(), exec); err != nil {
		s.logger.Error("failed to update cancelled execution", "id", id, "error", err)
		writeError(w, http.StatusInternalServerError, "failed to update execution")
		return
	}

	s.logger.Info("stale execution marked as cancelled", "id", id)
	writeJSON(w, http.StatusOK, map[string]string{"status": "cancelled", "execution_id": id})
}
