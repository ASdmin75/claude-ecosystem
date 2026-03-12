package api

import (
	"net/http"
	"strconv"

	"github.com/asdmin/claude-ecosystem/internal/store"
)

// handleListExecutions returns execution history with optional filters.
// GET /api/v1/executions?task=...&status=...&trigger=...&limit=...&offset=...
func (s *Server) handleListExecutions(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()

	filter := store.ExecutionFilter{
		TaskName: q.Get("task"),
		Status:   q.Get("status"),
		Trigger:  q.Get("trigger"),
	}

	if v := q.Get("pipeline"); v != "" {
		filter.PipelineName = v
	}

	if v := q.Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			filter.Limit = n
		}
	}
	if filter.Limit == 0 {
		filter.Limit = 50 // default
	}

	if v := q.Get("offset"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			filter.Offset = n
		}
	}

	executions, err := s.store.ListExecutions(r.Context(), filter)
	if err != nil {
		s.logger.Error("failed to list executions", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to list executions")
		return
	}

	writeJSON(w, http.StatusOK, executions)
}

// handleGetExecution returns a single execution by ID.
// GET /api/v1/executions/{id}
func (s *Server) handleGetExecution(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	exec, err := s.store.GetExecution(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, "execution not found: "+id)
		return
	}

	writeJSON(w, http.StatusOK, exec)
}

// handleDeleteExecution removes a single execution record.
// DELETE /api/v1/executions/{id}
func (s *Server) handleDeleteExecution(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	if err := s.store.DeleteExecution(r.Context(), id); err != nil {
		writeError(w, http.StatusNotFound, "execution not found: "+id)
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}
