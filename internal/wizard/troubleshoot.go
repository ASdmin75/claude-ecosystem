package wizard

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/asdmin/claude-ecosystem/internal/config"
)

// DiagnoseError categorizes a wizard error and returns structured diagnosis with recovery suggestions.
func DiagnoseError(phase string, err error, rawOutput string, plan *Plan, cfg *config.Config) *WizardDiagnosis {
	errMsg := err.Error()

	switch phase {
	case "generate":
		return diagnoseGenerate(err, errMsg, rawOutput)
	case "validate":
		return diagnoseValidate(errMsg, plan, cfg)
	case "apply":
		return diagnoseApply(errMsg)
	case "test":
		return diagnoseTest(errMsg, rawOutput)
	default:
		return &WizardDiagnosis{
			Category: ErrCatUnknown,
			Message:  errMsg,
			Suggestions: []RecoveryAction{
				{ID: "retry", Label: "Retry", Description: "Try generating the plan again"},
			},
		}
	}
}

func diagnoseGenerate(err error, errMsg, rawOutput string) *WizardDiagnosis {
	// Timeout
	if errors.Is(err, context.DeadlineExceeded) || strings.Contains(errMsg, "deadline exceeded") {
		return &WizardDiagnosis{
			Category: ErrCatTimeout,
			Message:  "Generation timed out — Claude took too long to respond",
			Suggestions: []RecoveryAction{
				{ID: "retry", Label: "Retry", Description: "Try again — timeouts are often transient"},
				{ID: "retry_simplified", Label: "Simplify & Retry", Description: "Shorten description and try again"},
			},
		}
	}

	// Empty output
	if strings.Contains(errMsg, "empty output") {
		return &WizardDiagnosis{
			Category: ErrCatEmptyOutput,
			Message:  "Claude returned empty output — no plan was generated",
			Suggestions: []RecoveryAction{
				{ID: "retry", Label: "Retry", Description: "Try again — this is often a transient issue"},
				{ID: "retry_with_hint", Label: "Add Context & Retry", Description: "Provide more detail about what you need"},
			},
		}
	}

	// JSON parse error
	if strings.Contains(errMsg, "parsing plan JSON") {
		d := &WizardDiagnosis{
			Category: ErrCatJSONParse,
			Message:  "Claude returned invalid JSON — could not parse the plan",
			Details:  rawOutput,
			Suggestions: []RecoveryAction{
				{ID: "retry", Label: "Retry", Description: "Try generating again"},
				{ID: "retry_with_hint", Label: "Add Context & Retry", Description: "Provide additional guidance"},
			},
		}
		return d
	}

	// Claude CLI error
	if strings.Contains(errMsg, "generation failed") {
		return &WizardDiagnosis{
			Category: ErrCatUnknown,
			Message:  errMsg,
			Details:  rawOutput,
			Suggestions: []RecoveryAction{
				{ID: "retry", Label: "Retry", Description: "Try generating again"},
			},
		}
	}

	return &WizardDiagnosis{
		Category: ErrCatUnknown,
		Message:  errMsg,
		Details:  rawOutput,
		Suggestions: []RecoveryAction{
			{ID: "retry", Label: "Retry", Description: "Try generating again"},
		},
	}
}

func diagnoseValidate(errMsg string, plan *Plan, cfg *config.Config) *WizardDiagnosis {
	// Duplicate names
	if strings.Contains(errMsg, "already exists") {
		d := &WizardDiagnosis{
			Category: ErrCatDuplicateName,
			Message:  errMsg,
			Suggestions: []RecoveryAction{
				{ID: "edit_plan", Label: "Edit Plan", Description: "Manually rename conflicting entities"},
			},
		}
		// Try to offer auto-fix
		if plan != nil && cfg != nil {
			if patched := AutoFixDuplicateNames(plan, cfg); patched != nil {
				d.Suggestions = append([]RecoveryAction{{
					ID:          "auto_fix",
					Label:       "Auto-fix Names",
					Description: "Automatically rename conflicting entities with a -v2 suffix",
					PatchedPlan: patched,
				}}, d.Suggestions...)
			}
		}
		return d
	}

	// Missing references
	if strings.Contains(errMsg, "references unknown") {
		return &WizardDiagnosis{
			Category: ErrCatMissingReference,
			Message:  errMsg,
			Suggestions: []RecoveryAction{
				{ID: "edit_plan", Label: "Edit Plan", Description: "Fix the missing references in the plan"},
				{ID: "retry", Label: "Regenerate", Description: "Generate a new plan from scratch"},
			},
		}
	}

	// Permission mode
	if strings.Contains(errMsg, "permission_mode") {
		return &WizardDiagnosis{
			Category: ErrCatPermissionMode,
			Message:  errMsg,
			Suggestions: []RecoveryAction{
				{ID: "edit_plan", Label: "Edit Plan", Description: "Fix permission_mode in the plan"},
				{ID: "retry", Label: "Regenerate", Description: "Generate a new plan from scratch"},
			},
		}
	}

	return &WizardDiagnosis{
		Category: ErrCatApplyFailed,
		Message:  fmt.Sprintf("Validation failed: %s", errMsg),
		Suggestions: []RecoveryAction{
			{ID: "edit_plan", Label: "Edit Plan", Description: "Fix the issue in the plan"},
			{ID: "retry", Label: "Regenerate", Description: "Generate a new plan from scratch"},
		},
	}
}

func diagnoseApply(errMsg string) *WizardDiagnosis {
	return &WizardDiagnosis{
		Category: ErrCatApplyFailed,
		Message:  errMsg,
		Suggestions: []RecoveryAction{
			{ID: "edit_plan", Label: "Edit Plan", Description: "Fix the issue and try applying again"},
			{ID: "retry", Label: "Regenerate", Description: "Generate a new plan from scratch"},
		},
	}
}

func diagnoseTest(errMsg, output string) *WizardDiagnosis {
	// If errMsg came from outputcheck (soft failure)
	if errMsg != "" && output != "" {
		return &WizardDiagnosis{
			Category: ErrCatTestSoftFailure,
			Message:  fmt.Sprintf("Task completed but output indicates a problem: %s", errMsg),
			Details:  truncate(output, 2000),
			Suggestions: []RecoveryAction{
				{ID: "edit_task", Label: "Edit Task", Description: "Modify the task prompt or configuration"},
				{ID: "retry_test", Label: "Run Again", Description: "Run the test again"},
			},
		}
	}

	return &WizardDiagnosis{
		Category: ErrCatTestHardFailure,
		Message:  errMsg,
		Details:  truncate(output, 2000),
		Suggestions: []RecoveryAction{
			{ID: "edit_task", Label: "Edit Task", Description: "Modify the task prompt or configuration"},
			{ID: "retry_test", Label: "Run Again", Description: "Run the test again"},
		},
	}
}

// AutoFixDuplicateNames creates a copy of the plan with conflicting names suffixed.
// Returns nil if no conflicts found or rename is not possible.
func AutoFixDuplicateNames(plan *Plan, cfg *config.Config) *Plan {
	if plan == nil || cfg == nil {
		return nil
	}

	// Collect existing names
	existingTasks := make(map[string]bool)
	for _, t := range cfg.Tasks {
		existingTasks[t.Name] = true
	}
	existingPipelines := make(map[string]bool)
	for _, p := range cfg.Pipelines {
		existingPipelines[p.Name] = true
	}
	existingDomains := make(map[string]bool)
	for name := range cfg.Domains {
		existingDomains[name] = true
	}
	existingMCP := make(map[string]bool)
	for _, m := range cfg.MCPServers {
		existingMCP[m.Name] = true
	}

	// Build rename map: old name -> new name
	renames := make(map[string]string)
	anyRenamed := false

	for _, t := range plan.Tasks {
		if existingTasks[t.Name] {
			newName := findUniqueName(t.Name, existingTasks)
			renames[t.Name] = newName
			existingTasks[newName] = true
			anyRenamed = true
		}
	}
	for _, p := range plan.Pipelines {
		if existingPipelines[p.Name] {
			newName := findUniqueName(p.Name, existingPipelines)
			renames[p.Name] = newName
			existingPipelines[newName] = true
			anyRenamed = true
		}
	}
	for _, d := range plan.Domains {
		if existingDomains[d.Name] {
			newName := findUniqueName(d.Name, existingDomains)
			renames[d.Name] = newName
			existingDomains[newName] = true
			anyRenamed = true
		}
	}
	for _, m := range plan.MCPServers {
		if existingMCP[m.Name] {
			newName := findUniqueName(m.Name, existingMCP)
			renames[m.Name] = newName
			existingMCP[newName] = true
			anyRenamed = true
		}
	}

	if !anyRenamed {
		return nil
	}

	// Deep-copy and apply renames
	patched := deepCopyPlan(plan)
	applyRenames(patched, renames)
	return patched
}

func findUniqueName(name string, existing map[string]bool) string {
	for i := 2; ; i++ {
		candidate := fmt.Sprintf("%s-v%d", name, i)
		if !existing[candidate] {
			return candidate
		}
	}
}

func rename(name string, renames map[string]string) string {
	if newName, ok := renames[name]; ok {
		return newName
	}
	return name
}

func renameSlice(names []string, renames map[string]string) []string {
	if len(names) == 0 {
		return names
	}
	result := make([]string, len(names))
	for i, n := range names {
		result[i] = rename(n, renames)
	}
	return result
}

func applyRenames(plan *Plan, renames map[string]string) {
	for i := range plan.Tasks {
		plan.Tasks[i].Name = rename(plan.Tasks[i].Name, renames)
		plan.Tasks[i].MCPServers = renameSlice(plan.Tasks[i].MCPServers, renames)
		plan.Tasks[i].Domain = rename(plan.Tasks[i].Domain, renames)
	}
	for i := range plan.Pipelines {
		plan.Pipelines[i].Name = rename(plan.Pipelines[i].Name, renames)
		plan.Pipelines[i].Steps = renameSlice(plan.Pipelines[i].Steps, renames)
	}
	for i := range plan.Domains {
		plan.Domains[i].Name = rename(plan.Domains[i].Name, renames)
		plan.Domains[i].Tasks = renameSlice(plan.Domains[i].Tasks, renames)
		plan.Domains[i].Pipelines = renameSlice(plan.Domains[i].Pipelines, renames)
		plan.Domains[i].Agents = renameSlice(plan.Domains[i].Agents, renames)
		plan.Domains[i].MCPServers = renameSlice(plan.Domains[i].MCPServers, renames)
	}
	for i := range plan.MCPServers {
		plan.MCPServers[i].Name = rename(plan.MCPServers[i].Name, renames)
	}
}

func deepCopyPlan(p *Plan) *Plan {
	cp := *p
	cp.Tasks = make([]TaskPlan, len(p.Tasks))
	for i, t := range p.Tasks {
		cp.Tasks[i] = t
		cp.Tasks[i].Tags = copyStrings(t.Tags)
		cp.Tasks[i].Agents = copyStrings(t.Agents)
		cp.Tasks[i].MCPServers = copyStrings(t.MCPServers)
		cp.Tasks[i].AllowedTools = copyStrings(t.AllowedTools)
	}
	cp.Pipelines = make([]PipelinePlan, len(p.Pipelines))
	for i, pp := range p.Pipelines {
		cp.Pipelines[i] = pp
		cp.Pipelines[i].Steps = copyStrings(pp.Steps)
	}
	cp.Domains = make([]DomainPlan, len(p.Domains))
	for i, d := range p.Domains {
		cp.Domains[i] = d
		cp.Domains[i].Tasks = copyStrings(d.Tasks)
		cp.Domains[i].Pipelines = copyStrings(d.Pipelines)
		cp.Domains[i].Agents = copyStrings(d.Agents)
		cp.Domains[i].MCPServers = copyStrings(d.MCPServers)
	}
	cp.MCPServers = make([]MCPServerPlan, len(p.MCPServers))
	for i, m := range p.MCPServers {
		cp.MCPServers[i] = m
		if m.Env != nil {
			cp.MCPServers[i].Env = make(map[string]string, len(m.Env))
			for k, v := range m.Env {
				cp.MCPServers[i].Env[k] = v
			}
		}
	}
	cp.Agents = make([]AgentPlan, len(p.Agents))
	for i, a := range p.Agents {
		cp.Agents[i] = a
		cp.Agents[i].Tools = copyStrings(a.Tools)
	}
	return &cp
}

func copyStrings(s []string) []string {
	if s == nil {
		return nil
	}
	cp := make([]string, len(s))
	copy(cp, s)
	return cp
}
