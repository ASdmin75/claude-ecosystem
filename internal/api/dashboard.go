package api

import (
	"net/http"
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
	s.cfg.RLock()
	totalTasks := len(s.cfg.Tasks)
	totalPipelines := len(s.cfg.Pipelines)
	s.cfg.RUnlock()

	counts, err := s.store.CountExecutions(r.Context())
	if err != nil {
		s.logger.Error("failed to count executions for dashboard", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to load dashboard data")
		return
	}

	resp := dashboardResponse{
		TotalTasks:      totalTasks,
		TotalPipelines:  totalPipelines,
		TotalExecutions: counts.Total,
		Running:         counts.Running,
		Completed:       counts.Completed,
		Failed:          counts.Failed,
	}

	writeJSON(w, http.StatusOK, resp)
}
