package api

import (
	"net/http"

	"github.com/asdmin/claude-ecosystem/internal/store"
)

// dashboardResponse holds aggregated stats for the dashboard.
type dashboardResponse struct {
	TotalTasks      int `json:"total_tasks"`
	TotalPipelines  int `json:"total_pipelines"`
	TotalExecutions int `json:"total_executions"`
	Running         int `json:"running"`
	Completed       int `json:"completed"`
	Failed          int `json:"failed"`
}

// handleDashboard returns aggregated statistics.
// GET /api/v1/dashboard
func (s *Server) handleDashboard(w http.ResponseWriter, r *http.Request) {
	resp := dashboardResponse{
		TotalTasks:     len(s.cfg.Tasks),
		TotalPipelines: len(s.cfg.Pipelines),
	}

	// Count executions by status. We fetch all with a large limit.
	// A production system would use a dedicated COUNT query.
	allExecs, err := s.store.ListExecutions(r.Context(), store.ExecutionFilter{Limit: 100000})
	if err != nil {
		s.logger.Error("failed to list executions for dashboard", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to load dashboard data")
		return
	}

	resp.TotalExecutions = len(allExecs)
	for _, e := range allExecs {
		switch e.Status {
		case "running":
			resp.Running++
		case "completed":
			resp.Completed++
		case "failed":
			resp.Failed++
		}
	}

	writeJSON(w, http.StatusOK, resp)
}
