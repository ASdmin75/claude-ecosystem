package api

import (
	"net/http"

	"github.com/asdmin/claude-ecosystem/internal/wizard"
)

// handleWizardGenerate accepts a description and runs Claude to generate a plan.
// POST /api/v1/wizard/generate
func (s *Server) handleWizardGenerate(w http.ResponseWriter, r *http.Request) {
	var req wizard.GenerateRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.Description == "" {
		writeError(w, http.StatusBadRequest, "description is required")
		return
	}

	plan, err := s.wizardGen.Generate(r.Context(), req, s.cfg)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	s.wizardStore.Put(plan)
	writeJSON(w, http.StatusOK, plan)
}

// handleWizardGetPlan returns a stored plan by ID.
// GET /api/v1/wizard/plans/{id}
func (s *Server) handleWizardGetPlan(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	plan, ok := s.wizardStore.Get(id)
	if !ok {
		writeError(w, http.StatusNotFound, "plan not found or expired")
		return
	}
	writeJSON(w, http.StatusOK, plan)
}

// handleWizardUpdatePlan allows editing a plan before applying.
// PUT /api/v1/wizard/plans/{id}
func (s *Server) handleWizardUpdatePlan(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	existing, ok := s.wizardStore.Get(id)
	if !ok {
		writeError(w, http.StatusNotFound, "plan not found or expired")
		return
	}
	if existing.Status != "draft" {
		writeError(w, http.StatusConflict, "plan is not in draft status")
		return
	}

	var updated wizard.Plan
	if err := readJSON(r, &updated); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	// Preserve ID and status
	updated.ID = id
	updated.Status = "draft"
	s.wizardStore.Put(&updated)
	writeJSON(w, http.StatusOK, &updated)
}

// handleWizardApply creates all entities from the plan.
// POST /api/v1/wizard/plans/{id}/apply
func (s *Server) handleWizardApply(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	plan, ok := s.wizardStore.Get(id)
	if !ok {
		writeError(w, http.StatusNotFound, "plan not found or expired")
		return
	}
	if plan.Status != "draft" {
		writeError(w, http.StatusConflict, "plan is not in draft status")
		return
	}

	applier := wizard.NewApplier(s.cfg, s.subagentMgr, s.domainMgr)
	result, err := applier.Apply(plan)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	plan.Status = "applied"
	s.wizardStore.Put(plan)
	writeJSON(w, http.StatusOK, result)
}

// handleWizardDiscard discards a plan.
// DELETE /api/v1/wizard/plans/{id}
func (s *Server) handleWizardDiscard(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	plan, ok := s.wizardStore.Get(id)
	if !ok {
		writeError(w, http.StatusNotFound, "plan not found or expired")
		return
	}

	plan.Status = "discarded"
	s.wizardStore.Delete(id)
	writeJSON(w, http.StatusOK, map[string]string{"status": "discarded"})
}
