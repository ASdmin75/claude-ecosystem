package api

import (
	"net/http"

	"github.com/asdmin/claude-ecosystem/internal/backup"
	"github.com/asdmin/claude-ecosystem/internal/config"
	"gopkg.in/yaml.v3"
)

// handleListBackups returns all backup entries.
// GET /api/v1/backups
func (s *Server) handleListBackups(w http.ResponseWriter, r *http.Request) {
	f := backup.Filter{Limit: 50}
	if et := r.URL.Query().Get("entity_type"); et != "" {
		f.EntityType = et
	}
	entries, err := s.backupMgr.ListBackups(r.Context(), f)
	if err != nil {
		s.logger.Error("failed to list backups", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to list backups")
		return
	}
	if entries == nil {
		entries = []backup.Entry{}
	}
	writeJSON(w, http.StatusOK, entries)
}

// handleGetBackup returns a single backup entry.
// GET /api/v1/backups/{id}
func (s *Server) handleGetBackup(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	entry, err := s.backupMgr.GetBackup(r.Context(), id)
	if err != nil {
		s.logger.Error("failed to get backup", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to get backup")
		return
	}
	if entry == nil {
		writeError(w, http.StatusNotFound, "backup not found: "+id)
		return
	}
	writeJSON(w, http.StatusOK, entry)
}

// handleRestoreBackup restores an entity from a backup.
// POST /api/v1/backups/{id}/restore
func (s *Server) handleRestoreBackup(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	if !s.guard.TryAcquire("config:write") {
		writeError(w, http.StatusConflict, "another config modification is in progress")
		return
	}
	defer s.guard.Release("config:write")

	entry, err := s.backupMgr.GetBackup(r.Context(), id)
	if err != nil || entry == nil {
		writeError(w, http.StatusNotFound, "backup not found: "+id)
		return
	}

	if entry.RestoredAt != nil {
		writeError(w, http.StatusConflict, "backup already restored")
		return
	}

	// Parse the backed-up config snapshot (required for task and pipeline restores).
	var snapCfg *config.Config
	if entry.ConfigSnap != "" {
		var cfg config.Config
		if err := yaml.Unmarshal([]byte(entry.ConfigSnap), &cfg); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to parse backup config: "+err.Error())
			return
		}
		snapCfg = &cfg
	}

	switch entry.EntityType {
	case "task":
		if snapCfg == nil {
			writeError(w, http.StatusBadRequest, "backup has no config snapshot")
			return
		}
		if err := s.restoreTask(snapCfg, entry.EntityName); err != nil {
			writeError(w, http.StatusConflict, err.Error())
			return
		}
	case "pipeline":
		if snapCfg == nil {
			writeError(w, http.StatusBadRequest, "backup has no config snapshot")
			return
		}
		if err := s.restorePipeline(r, snapCfg, entry); err != nil {
			writeError(w, http.StatusConflict, err.Error())
			return
		}
	case "subagent":
		if err := s.restoreSubAgent(r, entry); err != nil {
			writeError(w, http.StatusConflict, err.Error())
			return
		}
	default:
		writeError(w, http.StatusBadRequest, "unsupported entity type: "+entry.EntityType)
		return
	}

	if err := s.cfg.Save(); err != nil {
		s.logger.Error("failed to save config after restore", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to save config: "+err.Error())
		return
	}

	if err := s.backupMgr.MarkRestored(r.Context(), id); err != nil {
		s.logger.Error("failed to mark backup as restored", "error", err)
	}

	// Also mark children as restored.
	children, _ := s.backupMgr.GetChildren(r.Context(), id)
	for _, child := range children {
		_ = s.backupMgr.MarkRestored(r.Context(), child.ID)
	}

	s.logger.Info("backup restored", "id", id, "entity_type", entry.EntityType, "entity_name", entry.EntityName)
	writeJSON(w, http.StatusOK, map[string]string{"status": "restored", "backup_id": id})
}

func (s *Server) restoreTask(snapCfg *config.Config, taskName string) error {
	// Check conflict.
	if s.findTask(taskName) != nil {
		return errConflict("task \"" + taskName + "\" already exists, delete or rename it first")
	}
	// Find task in snapshot.
	var found bool
	for _, t := range snapCfg.Tasks {
		if t.Name == taskName {
			s.cfg.Tasks = append(s.cfg.Tasks, t)
			found = true
			break
		}
	}
	if !found {
		return errConflict("task \"" + taskName + "\" not found in backup snapshot")
	}

	// Restore domains that referenced this task.
	for k, d := range snapCfg.Domains {
		if _, exists := s.cfg.Domains[k]; exists {
			continue
		}
		for _, t := range d.Tasks {
			if t == taskName {
				if s.cfg.Domains == nil {
					s.cfg.Domains = make(map[string]config.Domain)
				}
				s.cfg.Domains[k] = d
				break
			}
		}
	}

	return nil
}

func (s *Server) restorePipeline(r *http.Request, snapCfg *config.Config, entry *backup.Entry) error {
	// Check pipeline conflict.
	if s.findPipeline(entry.EntityName) != nil {
		return errConflict("pipeline \"" + entry.EntityName + "\" already exists")
	}
	// Restore the pipeline from snapshot.
	var found bool
	for _, p := range snapCfg.Pipelines {
		if p.Name == entry.EntityName {
			s.cfg.Pipelines = append(s.cfg.Pipelines, p)
			found = true
			break
		}
	}
	if !found {
		return errConflict("pipeline not found in backup snapshot")
	}

	// Restore cascade children (tasks and sub-agents).
	children, _ := s.backupMgr.GetChildren(r.Context(), entry.ID)
	for _, child := range children {
		switch child.EntityType {
		case "task":
			if s.findTask(child.EntityName) == nil {
				for _, t := range snapCfg.Tasks {
					if t.Name == child.EntityName {
						s.cfg.Tasks = append(s.cfg.Tasks, t)
						break
					}
				}
			}
		case "subagent":
			files, err := s.backupMgr.RestoreFiles(r.Context(), child.ID)
			if err == nil {
				for _, data := range files {
					_ = s.subagentMgr.CreateFromBytes(child.EntityName, data)
				}
			}
		}
	}

	// Restore domains that referenced this pipeline or its cascade entities.
	restoredEntities := map[string]struct{}{entry.EntityName: {}}
	for _, child := range children {
		restoredEntities[child.EntityName] = struct{}{}
	}
	for k, d := range snapCfg.Domains {
		if _, exists := s.cfg.Domains[k]; exists {
			continue
		}
		// Check if the domain references any restored entity.
		refs := false
		for _, p := range d.Pipelines {
			if _, ok := restoredEntities[p]; ok {
				refs = true
				break
			}
		}
		if !refs {
			for _, t := range d.Tasks {
				if _, ok := restoredEntities[t]; ok {
					refs = true
					break
				}
			}
		}
		if !refs {
			for _, a := range d.Agents {
				if _, ok := restoredEntities[a]; ok {
					refs = true
					break
				}
			}
		}
		if refs {
			if s.cfg.Domains == nil {
				s.cfg.Domains = make(map[string]config.Domain)
			}
			s.cfg.Domains[k] = d
		}
	}

	return nil
}

func (s *Server) restoreSubAgent(r *http.Request, entry *backup.Entry) error {
	// Check conflict.
	if _, err := s.subagentMgr.Get(entry.EntityName); err == nil {
		return errConflict("sub-agent \"" + entry.EntityName + "\" already exists")
	}

	files, err := s.backupMgr.RestoreFiles(r.Context(), entry.ID)
	if err != nil {
		return errConflict("failed to read backup files: " + err.Error())
	}
	for _, data := range files {
		if err := s.subagentMgr.CreateFromBytes(entry.EntityName, data); err != nil {
			return errConflict("failed to restore sub-agent file: " + err.Error())
		}
	}
	return nil
}

type conflictError struct {
	msg string
}

func (e *conflictError) Error() string { return e.msg }

func errConflict(msg string) error { return &conflictError{msg: msg} }

