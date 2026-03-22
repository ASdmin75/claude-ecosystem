package api

import (
	"net/http"
	"strings"

	"github.com/asdmin/claude-ecosystem/internal/depcheck"
	"github.com/asdmin/claude-ecosystem/internal/subagent"
)

// handleListSubAgents returns all sub-agents.
// GET /api/v1/subagents
func (s *Server) handleListSubAgents(w http.ResponseWriter, r *http.Request) {
	agents, err := s.subagentMgr.List()
	if err != nil {
		s.logger.Error("failed to list sub-agents", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to list sub-agents")
		return
	}
	writeJSON(w, http.StatusOK, agents)
}

// handleGetSubAgent returns a single sub-agent by name.
// GET /api/v1/subagents/{name}
func (s *Server) handleGetSubAgent(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	agent, err := s.subagentMgr.Get(name)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		s.logger.Error("failed to get sub-agent", "name", name, "error", err)
		writeError(w, http.StatusInternalServerError, "failed to get sub-agent")
		return
	}
	writeJSON(w, http.StatusOK, agent)
}

// handleCreateSubAgent creates a new sub-agent.
// POST /api/v1/subagents
func (s *Server) handleCreateSubAgent(w http.ResponseWriter, r *http.Request) {
	var agent subagent.SubAgent
	if err := readJSON(r, &agent); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	if agent.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}

	if err := s.subagentMgr.Create(&agent); err != nil {
		if strings.Contains(err.Error(), "already exists") {
			writeError(w, http.StatusConflict, err.Error())
			return
		}
		s.logger.Error("failed to create sub-agent", "name", agent.Name, "error", err)
		writeError(w, http.StatusInternalServerError, "failed to create sub-agent")
		return
	}

	writeJSON(w, http.StatusCreated, agent)
}

// handleUpdateSubAgent updates an existing sub-agent.
// PUT /api/v1/subagents/{name}
func (s *Server) handleUpdateSubAgent(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	var agent subagent.SubAgent
	if err := readJSON(r, &agent); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	// Ensure the name in the URL matches the body (or set it from URL).
	agent.Name = name

	if err := s.subagentMgr.Update(&agent); err != nil {
		if strings.Contains(err.Error(), "not found") {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		s.logger.Error("failed to update sub-agent", "name", name, "error", err)
		writeError(w, http.StatusInternalServerError, "failed to update sub-agent")
		return
	}

	writeJSON(w, http.StatusOK, agent)
}

// handleDeleteSubAgent deletes a sub-agent with dependency checking and backup.
// DELETE /api/v1/subagents/{name}
func (s *Server) handleDeleteSubAgent(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	if _, err := s.subagentMgr.Get(name); err != nil {
		if strings.Contains(err.Error(), "not found") {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		s.logger.Error("failed to get sub-agent", "name", name, "error", err)
		writeError(w, http.StatusInternalServerError, "failed to get sub-agent")
		return
	}

	// Dependency check.
	analysis := depcheck.AnalyzeSubAgentDelete(s.cfg, name)
	if analysis.Blocked {
		writeJSON(w, http.StatusConflict, analysis)
		return
	}

	// Backup the .md file.
	agentPath, pathErr := s.subagentMgr.GetFilePath(name)
	var files map[string]string
	if pathErr == nil {
		files = map[string]string{"agents/" + name + ".md": agentPath}
	}

	entry, err := s.backupMgr.CreateBackup(r.Context(), "subagent", name, "delete", "", "", files)
	if err != nil {
		s.logger.Error("failed to create backup", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to create backup: "+err.Error())
		return
	}

	if err := s.subagentMgr.Delete(name); err != nil {
		s.logger.Error("failed to delete sub-agent", "name", name, "error", err)
		writeError(w, http.StatusInternalServerError, "failed to delete sub-agent")
		return
	}

	// Clean domain references.
	s.cleanDomainRefs(nil, nil, []string{name})
	if err := s.cfg.Save(); err != nil {
		s.logger.Error("failed to save config after sub-agent delete", "error", err)
	}

	s.logger.Info("sub-agent deleted", "name", name, "backup_id", entry.ID)
	writeJSON(w, http.StatusOK, deleteResponse{
		BackupID: entry.ID,
		Deleted:  []string{"subagent:" + name},
	})
}
