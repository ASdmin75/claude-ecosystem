package api

import (
	"net/http"
	"os"
	"strings"

	"github.com/asdmin/claude-ecosystem/internal/depcheck"
)

// deleteResponse is returned by delete endpoints with backup info.
type deleteResponse struct {
	BackupID string   `json:"backup_id"`
	Deleted  []string `json:"deleted"`
}

// handleTaskDeleteInfo returns dependency analysis for deleting a task.
// GET /api/v1/tasks/{name}/delete-info
func (s *Server) handleTaskDeleteInfo(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if s.findTask(name) == nil {
		writeError(w, http.StatusNotFound, "task not found: "+name)
		return
	}
	analysis := depcheck.AnalyzeTaskDelete(s.cfg, name)
	writeJSON(w, http.StatusOK, analysis)
}

// handlePipelineDeleteInfo returns dependency analysis for deleting a pipeline.
// GET /api/v1/pipelines/{name}/delete-info
func (s *Server) handlePipelineDeleteInfo(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if s.findPipeline(name) == nil {
		writeError(w, http.StatusNotFound, "pipeline not found: "+name)
		return
	}
	analysis := depcheck.AnalyzePipelineDelete(s.cfg, name)
	writeJSON(w, http.StatusOK, analysis)
}

// handleSubAgentDeleteInfo returns dependency analysis for deleting a sub-agent.
// GET /api/v1/subagents/{name}/delete-info
func (s *Server) handleSubAgentDeleteInfo(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if _, err := s.subagentMgr.Get(name); err != nil {
		if strings.Contains(err.Error(), "not found") {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to get sub-agent")
		return
	}
	analysis := depcheck.AnalyzeSubAgentDelete(s.cfg, name)
	writeJSON(w, http.StatusOK, analysis)
}

// readConfigSnap reads the current tasks.yaml content for backup.
func (s *Server) readConfigSnap() (string, error) {
	data, err := os.ReadFile(s.cfg.FilePath)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// cleanDomainRefs removes a task name from all domain Tasks lists
// and removes a pipeline name from all domain Pipelines lists.
func (s *Server) cleanDomainRefs(taskNames []string, pipelineNames []string) {
	for k, d := range s.cfg.Domains {
		changed := false
		for _, tn := range taskNames {
			for i, t := range d.Tasks {
				if t == tn {
					d.Tasks = append(d.Tasks[:i], d.Tasks[i+1:]...)
					changed = true
					break
				}
			}
		}
		for _, pn := range pipelineNames {
			for i, p := range d.Pipelines {
				if p == pn {
					d.Pipelines = append(d.Pipelines[:i], d.Pipelines[i+1:]...)
					changed = true
					break
				}
			}
		}
		if changed {
			s.cfg.Domains[k] = d
		}
	}
}
