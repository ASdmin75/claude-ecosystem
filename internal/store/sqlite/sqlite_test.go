package sqlite

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/asdmin/claude-ecosystem/internal/store"
)

func setupTestStore(t *testing.T) *Store {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	s, err := New(dbPath)
	if err != nil {
		t.Fatalf("New store: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestCreateAndGetExecution(t *testing.T) {
	s := setupTestStore(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)

	exec := &store.Execution{
		ID:        "exec-1",
		TaskName:  "test-task",
		Status:    "running",
		Trigger:   "manual",
		Prompt:    "do something",
		StartedAt: now,
	}

	if err := s.CreateExecution(ctx, exec); err != nil {
		t.Fatalf("CreateExecution: %v", err)
	}

	got, err := s.GetExecution(ctx, "exec-1")
	if err != nil {
		t.Fatalf("GetExecution: %v", err)
	}

	if got.TaskName != "test-task" {
		t.Errorf("task_name: got %q", got.TaskName)
	}
	if got.Status != "running" {
		t.Errorf("status: got %q", got.Status)
	}
	if got.Trigger != "manual" {
		t.Errorf("trigger: got %q", got.Trigger)
	}
}

func TestUpdateExecution(t *testing.T) {
	s := setupTestStore(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)

	exec := &store.Execution{
		ID:        "exec-2",
		TaskName:  "update-task",
		Status:    "running",
		Trigger:   "schedule",
		StartedAt: now,
	}
	s.CreateExecution(ctx, exec)

	completed := now.Add(5 * time.Second)
	exec.Status = "completed"
	exec.Output = "result output"
	exec.Model = "claude-4"
	exec.CostUSD = 0.05
	exec.DurationMS = 5000
	exec.CompletedAt = &completed

	if err := s.UpdateExecution(ctx, exec); err != nil {
		t.Fatalf("UpdateExecution: %v", err)
	}

	got, _ := s.GetExecution(ctx, "exec-2")
	if got.Status != "completed" {
		t.Errorf("status: got %q", got.Status)
	}
	if got.Output != "result output" {
		t.Errorf("output: got %q", got.Output)
	}
	if got.CostUSD != 0.05 {
		t.Errorf("cost: got %f", got.CostUSD)
	}
}

func TestUpdateNonExistentExecution(t *testing.T) {
	s := setupTestStore(t)
	ctx := context.Background()

	exec := &store.Execution{ID: "nonexistent", Status: "failed"}
	err := s.UpdateExecution(ctx, exec)
	if err == nil {
		t.Fatal("expected error for nonexistent execution")
	}
}

func TestListExecutions(t *testing.T) {
	s := setupTestStore(t)
	ctx := context.Background()
	now := time.Now().UTC()

	for i := range 5 {
		status := "completed"
		if i%2 == 0 {
			status = "failed"
		}
		s.CreateExecution(ctx, &store.Execution{
			ID:        "list-" + string(rune('a'+i)),
			TaskName:  "task-x",
			Status:    status,
			Trigger:   "manual",
			StartedAt: now.Add(time.Duration(i) * time.Second),
		})
	}

	// List all
	all, err := s.ListExecutions(ctx, store.ExecutionFilter{})
	if err != nil {
		t.Fatalf("ListExecutions: %v", err)
	}
	if len(all) != 5 {
		t.Fatalf("expected 5, got %d", len(all))
	}

	// Filter by status
	failed, _ := s.ListExecutions(ctx, store.ExecutionFilter{Status: "failed"})
	if len(failed) != 3 {
		t.Errorf("expected 3 failed, got %d", len(failed))
	}

	// Limit
	limited, _ := s.ListExecutions(ctx, store.ExecutionFilter{Limit: 2})
	if len(limited) != 2 {
		t.Errorf("expected 2 with limit, got %d", len(limited))
	}
}

func TestDeleteExecution(t *testing.T) {
	s := setupTestStore(t)
	ctx := context.Background()

	s.CreateExecution(ctx, &store.Execution{
		ID:        "del-1",
		TaskName:  "del-task",
		Status:    "completed",
		Trigger:   "manual",
		StartedAt: time.Now().UTC(),
	})

	if err := s.DeleteExecution(ctx, "del-1"); err != nil {
		t.Fatalf("DeleteExecution: %v", err)
	}

	_, err := s.GetExecution(ctx, "del-1")
	if err == nil {
		t.Fatal("expected error after delete")
	}
}

func TestDeleteNonExistentExecution(t *testing.T) {
	s := setupTestStore(t)
	err := s.DeleteExecution(context.Background(), "nope")
	if err == nil {
		t.Fatal("expected error for nonexistent")
	}
}

func TestCreateAndGetUser(t *testing.T) {
	s := setupTestStore(t)
	ctx := context.Background()

	user := &store.User{
		ID:           "user-1",
		Username:     "admin",
		PasswordHash: "$2a$10$hash",
	}

	if err := s.CreateUser(ctx, user); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	got, err := s.GetUserByUsername(ctx, "admin")
	if err != nil {
		t.Fatalf("GetUserByUsername: %v", err)
	}

	if got.ID != "user-1" {
		t.Errorf("id: got %q", got.ID)
	}
	if got.Username != "admin" {
		t.Errorf("username: got %q", got.Username)
	}
}

func TestGetNonExistentUser(t *testing.T) {
	s := setupTestStore(t)
	_, err := s.GetUserByUsername(context.Background(), "nobody")
	if err == nil {
		t.Fatal("expected error for nonexistent user")
	}
}

func TestPipelineExecution(t *testing.T) {
	s := setupTestStore(t)
	ctx := context.Background()

	exec := &store.Execution{
		ID:           "pipe-1",
		PipelineName: "deploy-pipeline",
		Status:       "running",
		Trigger:      "schedule",
		StartedAt:    time.Now().UTC(),
	}
	s.CreateExecution(ctx, exec)

	// Filter by pipeline name
	results, _ := s.ListExecutions(ctx, store.ExecutionFilter{PipelineName: "deploy-pipeline"})
	if len(results) != 1 {
		t.Fatalf("expected 1 pipeline execution, got %d", len(results))
	}
	if results[0].PipelineName != "deploy-pipeline" {
		t.Errorf("pipeline_name: got %q", results[0].PipelineName)
	}
}
