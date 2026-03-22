package backup

import (
	"context"
	"database/sql"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

func setupTestManager(t *testing.T) *Manager {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	m, err := New(db, dir, logger)
	if err != nil {
		t.Fatalf("New manager: %v", err)
	}
	return m
}

func TestCreateBackup(t *testing.T) {
	m := setupTestManager(t)
	ctx := context.Background()

	entry, err := m.CreateBackup(ctx, "task", "my-task", "delete", "", "config: yaml content", nil)
	if err != nil {
		t.Fatalf("CreateBackup: %v", err)
	}
	if entry.ID == "" {
		t.Fatal("expected non-empty ID")
	}
	if entry.EntityType != "task" {
		t.Errorf("entity_type: got %q, want %q", entry.EntityType, "task")
	}
	if entry.EntityName != "my-task" {
		t.Errorf("entity_name: got %q, want %q", entry.EntityName, "my-task")
	}
	if entry.ConfigSnap != "config: yaml content" {
		t.Error("config_snap not preserved")
	}
}

func TestCreateBackupWithFiles(t *testing.T) {
	m := setupTestManager(t)
	ctx := context.Background()

	// Create a source file to back up.
	srcDir := t.TempDir()
	srcFile := filepath.Join(srcDir, "agent.md")
	if err := os.WriteFile(srcFile, []byte("# Agent Instructions"), 0o644); err != nil {
		t.Fatalf("write source: %v", err)
	}

	files := map[string]string{"agents/agent.md": srcFile}
	entry, err := m.CreateBackup(ctx, "subagent", "agent", "delete", "", "", files)
	if err != nil {
		t.Fatalf("CreateBackup: %v", err)
	}

	// Verify file was copied.
	backupPath := filepath.Join(m.backupDir(entry.ID), "agents", "agent.md")
	data, err := os.ReadFile(backupPath)
	if err != nil {
		t.Fatalf("read backup file: %v", err)
	}
	if string(data) != "# Agent Instructions" {
		t.Errorf("backup content: got %q", string(data))
	}
}

func TestGetBackup(t *testing.T) {
	m := setupTestManager(t)
	ctx := context.Background()

	entry, _ := m.CreateBackup(ctx, "pipeline", "p1", "delete", "", "snap", nil)

	got, err := m.GetBackup(ctx, entry.ID)
	if err != nil {
		t.Fatalf("GetBackup: %v", err)
	}
	if got == nil {
		t.Fatal("expected non-nil entry")
	}
	if got.EntityName != "p1" {
		t.Errorf("entity_name: got %q, want %q", got.EntityName, "p1")
	}
}

func TestGetBackupNotFound(t *testing.T) {
	m := setupTestManager(t)
	ctx := context.Background()

	got, err := m.GetBackup(ctx, "nonexistent-id")
	if err != nil {
		t.Fatalf("GetBackup: unexpected error: %v", err)
	}
	if got != nil {
		t.Fatal("expected nil for nonexistent ID")
	}
}

func TestListBackups(t *testing.T) {
	m := setupTestManager(t)
	ctx := context.Background()

	m.CreateBackup(ctx, "task", "t1", "delete", "", "", nil)
	m.CreateBackup(ctx, "pipeline", "p1", "delete", "", "", nil)
	m.CreateBackup(ctx, "task", "t2", "delete", "", "", nil)

	// List all.
	all, err := m.ListBackups(ctx, Filter{})
	if err != nil {
		t.Fatalf("ListBackups: %v", err)
	}
	if len(all) != 3 {
		t.Fatalf("expected 3 backups, got %d", len(all))
	}

	// Filter by entity_type.
	tasks, err := m.ListBackups(ctx, Filter{EntityType: "task"})
	if err != nil {
		t.Fatalf("ListBackups filtered: %v", err)
	}
	if len(tasks) != 2 {
		t.Errorf("expected 2 task backups, got %d", len(tasks))
	}
}

func TestListBackupsOrderDesc(t *testing.T) {
	m := setupTestManager(t)
	ctx := context.Background()

	m.CreateBackup(ctx, "task", "first", "delete", "", "", nil)
	m.CreateBackup(ctx, "task", "second", "delete", "", "", nil)

	list, _ := m.ListBackups(ctx, Filter{})
	if len(list) != 2 {
		t.Fatalf("expected 2, got %d", len(list))
	}
	// Most recent first.
	if list[0].EntityName != "second" {
		t.Errorf("expected 'second' first, got %q", list[0].EntityName)
	}
}

func TestRestoreFiles(t *testing.T) {
	m := setupTestManager(t)
	ctx := context.Background()

	srcDir := t.TempDir()
	srcFile := filepath.Join(srcDir, "test.md")
	os.WriteFile(srcFile, []byte("restore me"), 0o644)

	entry, _ := m.CreateBackup(ctx, "subagent", "sa", "delete", "", "", map[string]string{"agents/test.md": srcFile})

	files, err := m.RestoreFiles(ctx, entry.ID)
	if err != nil {
		t.Fatalf("RestoreFiles: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(files))
	}
	if string(files["agents/test.md"]) != "restore me" {
		t.Errorf("restored content: got %q", string(files["agents/test.md"]))
	}
}

func TestMarkRestored(t *testing.T) {
	m := setupTestManager(t)
	ctx := context.Background()

	entry, _ := m.CreateBackup(ctx, "task", "t1", "delete", "", "", nil)
	if entry.RestoredAt != nil {
		t.Fatal("expected nil RestoredAt initially")
	}

	if err := m.MarkRestored(ctx, entry.ID); err != nil {
		t.Fatalf("MarkRestored: %v", err)
	}

	got, _ := m.GetBackup(ctx, entry.ID)
	if got.RestoredAt == nil {
		t.Fatal("expected non-nil RestoredAt after marking")
	}
}

func TestGetChildren(t *testing.T) {
	m := setupTestManager(t)
	ctx := context.Background()

	parent, _ := m.CreateBackup(ctx, "pipeline", "p1", "delete", "", "snap", nil)
	m.CreateBackup(ctx, "task", "t1", "cascade_delete", parent.ID, "", nil)
	m.CreateBackup(ctx, "subagent", "a1", "cascade_delete", parent.ID, "", nil)

	children, err := m.GetChildren(ctx, parent.ID)
	if err != nil {
		t.Fatalf("GetChildren: %v", err)
	}
	if len(children) != 2 {
		t.Errorf("expected 2 children, got %d", len(children))
	}
}

func TestBackupDirCreated(t *testing.T) {
	m := setupTestManager(t)
	ctx := context.Background()

	entry, _ := m.CreateBackup(ctx, "task", "t1", "delete", "", "", nil)

	dir := m.backupDir(entry.ID)
	info, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("backup dir not created: %v", err)
	}
	if !info.IsDir() {
		t.Error("expected directory")
	}
}

func TestRestoreFilesNotFound(t *testing.T) {
	m := setupTestManager(t)
	ctx := context.Background()

	_, err := m.RestoreFiles(ctx, "nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent backup")
	}
}
