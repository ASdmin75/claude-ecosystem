package wizard

import (
	"context"
	"fmt"
	"testing"

	"github.com/asdmin/claude-ecosystem/internal/config"
)

func TestDiagnoseError_GenerateEmpty(t *testing.T) {
	err := fmt.Errorf("claude returned empty output")
	d := DiagnoseError("generate", err, "", nil, nil)
	if d.Category != ErrCatEmptyOutput {
		t.Errorf("expected category %s, got %s", ErrCatEmptyOutput, d.Category)
	}
	if len(d.Suggestions) == 0 {
		t.Error("expected at least one suggestion")
	}
}

func TestDiagnoseError_GenerateJSONParse(t *testing.T) {
	err := fmt.Errorf("parsing plan JSON: unexpected token")
	raw := `{"broken json`
	d := DiagnoseError("generate", err, raw, nil, nil)
	if d.Category != ErrCatJSONParse {
		t.Errorf("expected category %s, got %s", ErrCatJSONParse, d.Category)
	}
	if d.Details != raw {
		t.Error("expected raw output in details")
	}
}

func TestDiagnoseError_GenerateTimeout(t *testing.T) {
	err := fmt.Errorf("context: %w", context.DeadlineExceeded)
	d := DiagnoseError("generate", err, "", nil, nil)
	if d.Category != ErrCatTimeout {
		t.Errorf("expected category %s, got %s", ErrCatTimeout, d.Category)
	}
}

func TestDiagnoseError_ValidateDuplicate(t *testing.T) {
	err := fmt.Errorf(`task "my-task" already exists`)
	plan := &Plan{
		Tasks: []TaskPlan{{Name: "my-task", Prompt: "test"}},
	}
	cfg := &config.Config{
		Tasks: []config.Task{{Name: "my-task"}},
	}
	d := DiagnoseError("validate", err, "", plan, cfg)
	if d.Category != ErrCatDuplicateName {
		t.Errorf("expected category %s, got %s", ErrCatDuplicateName, d.Category)
	}
	// Should have auto_fix suggestion with patched plan
	found := false
	for _, s := range d.Suggestions {
		if s.ID == "auto_fix" && s.PatchedPlan != nil {
			found = true
			if s.PatchedPlan.Tasks[0].Name == "my-task" {
				t.Error("patched plan should have renamed the task")
			}
		}
	}
	if !found {
		t.Error("expected auto_fix suggestion with patched plan")
	}
}

func TestDiagnoseError_ValidateMissingRef(t *testing.T) {
	err := fmt.Errorf(`task "x" references unknown agent "y"`)
	d := DiagnoseError("validate", err, "", nil, nil)
	if d.Category != ErrCatMissingReference {
		t.Errorf("expected category %s, got %s", ErrCatMissingReference, d.Category)
	}
}

func TestDiagnoseError_ValidatePermission(t *testing.T) {
	err := fmt.Errorf(`task "x" permission_mode is "plan"`)
	d := DiagnoseError("validate", err, "", nil, nil)
	if d.Category != ErrCatPermissionMode {
		t.Errorf("expected category %s, got %s", ErrCatPermissionMode, d.Category)
	}
}

func TestDiagnoseError_Apply(t *testing.T) {
	err := fmt.Errorf("saving config: disk full")
	d := DiagnoseError("apply", err, "", nil, nil)
	if d.Category != ErrCatApplyFailed {
		t.Errorf("expected category %s, got %s", ErrCatApplyFailed, d.Category)
	}
}

func TestDiagnoseError_TestSoftFailure(t *testing.T) {
	err := fmt.Errorf("tool not available")
	d := DiagnoseError("test", err, "some output", nil, nil)
	if d.Category != ErrCatTestSoftFailure {
		t.Errorf("expected category %s, got %s", ErrCatTestSoftFailure, d.Category)
	}
}

func TestAutoFixDuplicateNames(t *testing.T) {
	plan := &Plan{
		Tasks: []TaskPlan{
			{Name: "task-a", Prompt: "test"},
			{Name: "task-b", Prompt: "test"},
		},
		Pipelines: []PipelinePlan{
			{Name: "pipe-a", Steps: []string{"task-a", "task-b"}},
		},
		Domains: []DomainPlan{
			{Name: "domain-a", DataDir: "data/a", Tasks: []string{"task-a"}},
		},
	}
	cfg := &config.Config{
		Tasks: []config.Task{
			{Name: "task-a"}, // conflicts
		},
	}

	patched := AutoFixDuplicateNames(plan, cfg)
	if patched == nil {
		t.Fatal("expected non-nil patched plan")
	}

	// task-a should be renamed, task-b should stay
	if patched.Tasks[0].Name == "task-a" {
		t.Error("task-a should have been renamed")
	}
	if patched.Tasks[0].Name != "task-a-v2" {
		t.Errorf("expected task-a-v2, got %s", patched.Tasks[0].Name)
	}
	if patched.Tasks[1].Name != "task-b" {
		t.Error("task-b should not have been renamed")
	}

	// Pipeline steps should reflect rename
	if patched.Pipelines[0].Steps[0] != "task-a-v2" {
		t.Errorf("pipeline step should reference renamed task, got %s", patched.Pipelines[0].Steps[0])
	}
	if patched.Pipelines[0].Steps[1] != "task-b" {
		t.Error("pipeline step for task-b should be unchanged")
	}

	// Domain task references should reflect rename
	if patched.Domains[0].Tasks[0] != "task-a-v2" {
		t.Errorf("domain task ref should reference renamed task, got %s", patched.Domains[0].Tasks[0])
	}
}

func TestAutoFixDuplicateNames_NoConflicts(t *testing.T) {
	plan := &Plan{
		Tasks: []TaskPlan{{Name: "unique-task", Prompt: "test"}},
	}
	cfg := &config.Config{
		Tasks: []config.Task{{Name: "other-task"}},
	}

	patched := AutoFixDuplicateNames(plan, cfg)
	if patched != nil {
		t.Error("expected nil when no conflicts")
	}
}

func TestAutoFixDuplicateNames_NilInputs(t *testing.T) {
	if AutoFixDuplicateNames(nil, nil) != nil {
		t.Error("expected nil for nil inputs")
	}
}

func TestAutoFixDuplicateNames_MCPServers(t *testing.T) {
	plan := &Plan{
		MCPServers: []MCPServerPlan{{Name: "database", Command: "./bin/mcp-database"}},
		Tasks:      []TaskPlan{{Name: "new-task", Prompt: "test", MCPServers: []string{"database"}}},
	}
	cfg := &config.Config{
		MCPServers: []config.MCPServerConfig{{Name: "database"}},
	}

	patched := AutoFixDuplicateNames(plan, cfg)
	if patched == nil {
		t.Fatal("expected non-nil patched plan")
	}
	if patched.MCPServers[0].Name == "database" {
		t.Error("database MCP server should have been renamed")
	}
	// Task's MCP server reference should also be updated
	if patched.Tasks[0].MCPServers[0] == "database" {
		t.Error("task MCP server reference should have been updated")
	}
}

func TestDeepCopyPlan(t *testing.T) {
	original := &Plan{
		Summary: "test",
		Tasks:   []TaskPlan{{Name: "t1", Prompt: "p1", Tags: []string{"a"}}},
		MCPServers: []MCPServerPlan{{
			Name: "s1", Command: "cmd",
			Env: map[string]string{"K": "V"},
		}},
	}

	cp := deepCopyPlan(original)

	// Modify copy — should not affect original
	cp.Tasks[0].Name = "modified"
	cp.Tasks[0].Tags[0] = "modified"
	cp.MCPServers[0].Env["K"] = "modified"

	if original.Tasks[0].Name != "t1" {
		t.Error("original task name was modified")
	}
	if original.Tasks[0].Tags[0] != "a" {
		t.Error("original task tags were modified")
	}
	if original.MCPServers[0].Env["K"] != "V" {
		t.Error("original MCP server env was modified")
	}
}
