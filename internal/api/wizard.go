package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/asdmin/claude-ecosystem/internal/outputcheck"
	"github.com/asdmin/claude-ecosystem/internal/task"
	"github.com/asdmin/claude-ecosystem/internal/wizard"
)

// handleWizardGenerate accepts a description and runs Claude to generate a plan.
// POST /api/v1/wizard/generate
// Returns 200 with Plan on success, 422 with WizardDiagnosis on diagnosable errors.
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
		var genErr *wizard.GenerateError
		if errors.As(err, &genErr) {
			diagnosis := wizard.DiagnoseError("generate", genErr, genErr.RawOutput, nil, s.cfg)
			writeJSON(w, http.StatusUnprocessableEntity, diagnosis)
			return
		}
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
// Returns 200 with ApplyResult on success, 422 with WizardDiagnosis on diagnosable errors.
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
		phase := "apply"
		if strings.HasPrefix(err.Error(), "validation failed:") {
			phase = "validate"
		}
		diagnosis := wizard.DiagnoseError(phase, err, "", plan, s.cfg)
		writeJSON(w, http.StatusUnprocessableEntity, diagnosis)
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
	_, ok := s.wizardStore.Get(id)
	if !ok {
		writeError(w, http.StatusNotFound, "plan not found or expired")
		return
	}

	s.wizardStore.Delete(id)
	writeJSON(w, http.StatusOK, map[string]string{"status": "discarded"})
}

// handleWizardValidate validates a plan without applying it.
// POST /api/v1/wizard/plans/{id}/validate
func (s *Server) handleWizardValidate(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	plan, ok := s.wizardStore.Get(id)
	if !ok {
		writeError(w, http.StatusNotFound, "plan not found or expired")
		return
	}

	applier := wizard.NewApplier(s.cfg, s.subagentMgr, s.domainMgr)
	warnings, err := applier.ValidateOnly(plan)
	if err != nil {
		diagnosis := wizard.DiagnoseError("validate", err, "", plan, s.cfg)
		writeJSON(w, http.StatusUnprocessableEntity, diagnosis)
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"valid":    true,
		"warnings": warnings,
	})
}

// handleWizardTestRun runs a single task from an applied plan to verify it works.
// POST /api/v1/wizard/plans/{id}/test
func (s *Server) handleWizardTestRun(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	plan, ok := s.wizardStore.Get(id)
	if !ok {
		writeError(w, http.StatusNotFound, "plan not found or expired")
		return
	}
	if plan.Status != "applied" {
		writeError(w, http.StatusConflict, "plan must be applied before testing")
		return
	}

	var req wizard.TestRunRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.TaskName == "" {
		writeError(w, http.StatusBadRequest, "task_name is required")
		return
	}

	// Find the task in config
	t := s.findTask(req.TaskName)
	if t == nil {
		writeError(w, http.StatusNotFound, fmt.Sprintf("task %q not found", req.TaskName))
		return
	}

	// Resolve run options
	opts, cleanup, resolveErr := task.ResolveRunOptions(*t, s.subagentMgr, s.mcpMgr, s.domainMgr)
	if cleanup != nil {
		defer cleanup()
	}
	if resolveErr != nil {
		writeError(w, http.StatusInternalServerError, "failed to resolve run options: "+resolveErr.Error())
		return
	}

	// Cap test run timeout at 2 minutes
	timeout := t.ParsedTimeout()
	const maxTestTimeout = 2 * time.Minute
	if timeout == 0 || timeout > maxTestTimeout {
		timeout = maxTestTimeout
	}
	ctx, cancel := context.WithTimeout(r.Context(), timeout)
	defer cancel()

	result := s.taskRunner.Run(ctx, *t, opts, nil)

	testResult := wizard.TestRunResult{
		TaskName:   req.TaskName,
		Output:     result.Output,
		Error:      result.Error,
		DurationMS: result.Duration.Milliseconds(),
		CostUSD:    result.CostUSD,
	}

	// Check for soft failures
	if softFail := outputcheck.CheckStepOutput(result.Output); softFail != "" {
		testResult.SoftFailure = softFail
		testResult.Diagnosis = wizard.DiagnoseError("test", fmt.Errorf("%s", softFail), result.Output, plan, s.cfg)
	} else if result.Error != "" {
		testResult.Diagnosis = &wizard.WizardDiagnosis{
			Category: wizard.ErrCatTestHardFailure,
			Message:  result.Error,
			Details:  truncateString(result.Output, 2000),
			Suggestions: []wizard.RecoveryAction{
				{ID: "edit_task", Label: "Edit Task", Description: "Modify the task prompt or configuration"},
				{ID: "retry_test", Label: "Run Again", Description: "Run the test again"},
			},
		}
	}

	writeJSON(w, http.StatusOK, testResult)
}

func truncateString(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
