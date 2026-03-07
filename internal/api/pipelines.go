package api

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/asdmin/claude-ecosystem/internal/events"
	"github.com/asdmin/claude-ecosystem/internal/store"
	"github.com/asdmin/claude-ecosystem/internal/task"
	"github.com/google/uuid"
)

// pipelineRunResponse is returned by the synchronous pipeline run endpoint.
type pipelineRunResponse struct {
	ExecutionID string `json:"execution_id"`
	Pipeline    string `json:"pipeline"`
	Status      string `json:"status"`
	Output      string `json:"output,omitempty"`
	Error       string `json:"error,omitempty"`
	Iterations  int    `json:"iterations"`
	DurationMS  int64  `json:"duration_ms"`
}

// handleListPipelines returns all pipelines from the config.
// GET /api/v1/pipelines
func (s *Server) handleListPipelines(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.cfg.Pipelines)
}

// handleGetPipeline returns a single pipeline by name.
// GET /api/v1/pipelines/{name}
func (s *Server) handleGetPipeline(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	p := s.findPipeline(name)
	if p == nil {
		writeError(w, http.StatusNotFound, "pipeline not found: "+name)
		return
	}
	writeJSON(w, http.StatusOK, p)
}

// runPipeline executes a pipeline sequentially, chaining step outputs via {{.PrevOutput}}.
// It stops when the stop_signal is found in the output or max_iterations is reached.
func (s *Server) runPipeline(ctx context.Context, pipelineName string, execID string) pipelineRunResponse {
	start := time.Now()
	p := s.findPipeline(pipelineName)
	if p == nil {
		return pipelineRunResponse{
			ExecutionID: execID,
			Pipeline:    pipelineName,
			Status:      "failed",
			Error:       "pipeline not found: " + pipelineName,
			DurationMS:  time.Since(start).Milliseconds(),
		}
	}

	maxIter := p.MaxIter()
	var lastOutput string
	var iterations int

	for i := 0; i < maxIter; i++ {
		iterations = i + 1
		for _, step := range p.Steps {
			t := s.findTask(step.Task)
			if t == nil {
				return pipelineRunResponse{
					ExecutionID: execID,
					Pipeline:    pipelineName,
					Status:      "failed",
					Error:       fmt.Sprintf("step task not found: %s", step.Task),
					Iterations:  iterations,
					DurationMS:  time.Since(start).Milliseconds(),
				}
			}

			vars := map[string]string{
				"PrevOutput": lastOutput,
			}

			result := s.taskRunner.Run(ctx, *t, task.RunOptions{}, vars)
			if result.Error != "" {
				return pipelineRunResponse{
					ExecutionID: execID,
					Pipeline:    pipelineName,
					Status:      "failed",
					Error:       fmt.Sprintf("step %s failed: %s", step.Task, result.Error),
					Output:      lastOutput,
					Iterations:  iterations,
					DurationMS:  time.Since(start).Milliseconds(),
				}
			}

			lastOutput = result.Output

			// Check for stop signal.
			if p.StopSignal != "" && strings.Contains(lastOutput, p.StopSignal) {
				return pipelineRunResponse{
					ExecutionID: execID,
					Pipeline:    pipelineName,
					Status:      "completed",
					Output:      lastOutput,
					Iterations:  iterations,
					DurationMS:  time.Since(start).Milliseconds(),
				}
			}
		}
	}

	return pipelineRunResponse{
		ExecutionID: execID,
		Pipeline:    pipelineName,
		Status:      "completed",
		Output:      lastOutput,
		Iterations:  iterations,
		DurationMS:  time.Since(start).Milliseconds(),
	}
}

// handleRunPipeline runs a pipeline synchronously.
// POST /api/v1/pipelines/{name}/run
func (s *Server) handleRunPipeline(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	p := s.findPipeline(name)
	if p == nil {
		writeError(w, http.StatusNotFound, "pipeline not found: "+name)
		return
	}

	execID := uuid.New().String()
	now := time.Now().UTC()

	exec := &store.Execution{
		ID:           execID,
		PipelineName: p.Name,
		Status:       "running",
		Trigger:      "manual",
		StartedAt:    now,
	}
	if err := s.store.CreateExecution(r.Context(), exec); err != nil {
		s.logger.Error("failed to create pipeline execution record", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to create execution record")
		return
	}

	resp := s.runPipeline(r.Context(), name, execID)

	completedAt := time.Now().UTC()
	exec.Status = resp.Status
	exec.Output = resp.Output
	exec.Error = resp.Error
	exec.DurationMS = resp.DurationMS
	exec.CompletedAt = &completedAt

	if err := s.store.UpdateExecution(r.Context(), exec); err != nil {
		s.logger.Error("failed to update pipeline execution record", "error", err)
	}

	s.bus.Publish(events.Event{
		Type: "pipeline.completed",
		Payload: map[string]string{
			"execution_id": execID,
			"pipeline":     name,
			"status":       resp.Status,
		},
	})

	writeJSON(w, http.StatusOK, resp)
}

// handleRunPipelineAsync starts a pipeline asynchronously and returns the execution ID.
// POST /api/v1/pipelines/{name}/run-async
func (s *Server) handleRunPipelineAsync(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	p := s.findPipeline(name)
	if p == nil {
		writeError(w, http.StatusNotFound, "pipeline not found: "+name)
		return
	}

	execID := uuid.New().String()
	now := time.Now().UTC()

	exec := &store.Execution{
		ID:           execID,
		PipelineName: p.Name,
		Status:       "running",
		Trigger:      "manual",
		StartedAt:    now,
	}
	if err := s.store.CreateExecution(r.Context(), exec); err != nil {
		s.logger.Error("failed to create pipeline execution record", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to create execution record")
		return
	}

	go func() {
		ctx := context.Background()
		resp := s.runPipeline(ctx, name, execID)

		completedAt := time.Now().UTC()
		exec.Status = resp.Status
		exec.Output = resp.Output
		exec.Error = resp.Error
		exec.DurationMS = resp.DurationMS
		exec.CompletedAt = &completedAt

		if err := s.store.UpdateExecution(ctx, exec); err != nil {
			s.logger.Error("failed to update async pipeline execution record", "error", err)
		}

		s.bus.Publish(events.Event{
			Type: "pipeline.completed",
			Payload: map[string]string{
				"execution_id": execID,
				"pipeline":     name,
				"status":       resp.Status,
			},
		})
	}()

	writeJSON(w, http.StatusAccepted, asyncRunResponse{
		ExecutionID: execID,
		Status:      "running",
	})
}
