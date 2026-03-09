package domain

import (
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/asdmin/claude-ecosystem/internal/config"
)

func TestInitCreatesDirectoryAndSchema(t *testing.T) {
	dir := t.TempDir()
	dataDir := filepath.Join(dir, "leads")

	domains := map[string]config.Domain{
		"leads": {
			Name:      "leads",
			DataDir:   dataDir,
			DB:        "test.db",
			Schema:    "CREATE TABLE IF NOT EXISTS items (id INTEGER PRIMARY KEY, name TEXT NOT NULL);",
			DomainDoc: "DOMAIN.md",
		},
	}

	mgr := New(domains, slog.Default())
	if err := mgr.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}

	// Check directory was created
	if _, err := os.Stat(dataDir); err != nil {
		t.Errorf("data dir not created: %v", err)
	}

	// Check database was created with schema
	dbPath := filepath.Join(dataDir, "test.db")
	if _, err := os.Stat(dbPath); err != nil {
		t.Errorf("database not created: %v", err)
	}

	// Check DOMAIN.md was created
	docPath := filepath.Join(dataDir, "DOMAIN.md")
	if _, err := os.Stat(docPath); err != nil {
		t.Errorf("DOMAIN.md not created: %v", err)
	}

	// Read and verify DOMAIN.md contains expected content
	content, err := os.ReadFile(docPath)
	if err != nil {
		t.Fatalf("reading DOMAIN.md: %v", err)
	}
	if len(content) == 0 {
		t.Error("DOMAIN.md is empty")
	}
}

func TestInitDoesNotOverwriteExistingDoc(t *testing.T) {
	dir := t.TempDir()
	dataDir := filepath.Join(dir, "leads")
	os.MkdirAll(dataDir, 0o755)

	// Create existing DOMAIN.md
	docPath := filepath.Join(dataDir, "DOMAIN.md")
	os.WriteFile(docPath, []byte("custom content"), 0o644)

	domains := map[string]config.Domain{
		"leads": {
			Name:      "leads",
			DataDir:   dataDir,
			DomainDoc: "DOMAIN.md",
		},
	}

	mgr := New(domains, slog.Default())
	if err := mgr.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}

	// Verify file was NOT overwritten
	content, _ := os.ReadFile(docPath)
	if string(content) != "custom content" {
		t.Errorf("DOMAIN.md was overwritten, got: %s", content)
	}
}

func TestDomainEnvVars(t *testing.T) {
	domains := map[string]config.Domain{
		"leads": {
			Name:      "leads",
			DataDir:   "/data/leads",
			DB:        "leads.db",
			DomainDoc: "DOMAIN.md",
		},
	}

	mgr := New(domains, slog.Default())
	vars := mgr.DomainEnvVars("leads")

	if vars["DOMAIN_NAME"] != "leads" {
		t.Errorf("DOMAIN_NAME = %q, want %q", vars["DOMAIN_NAME"], "leads")
	}
	if vars["DOMAIN_DATA_DIR"] != "/data/leads" {
		t.Errorf("DOMAIN_DATA_DIR = %q, want %q", vars["DOMAIN_DATA_DIR"], "/data/leads")
	}
	if vars["DOMAIN_DB_PATH"] == "" {
		t.Error("DOMAIN_DB_PATH is empty")
	}
	if vars["DOMAIN_DOC_PATH"] != "/data/leads/DOMAIN.md" {
		t.Errorf("DOMAIN_DOC_PATH = %q, want %q", vars["DOMAIN_DOC_PATH"], "/data/leads/DOMAIN.md")
	}
}

func TestDomainEnvVarsUnknown(t *testing.T) {
	mgr := New(nil, slog.Default())
	vars := mgr.DomainEnvVars("nonexistent")
	if vars != nil {
		t.Errorf("expected nil for unknown domain, got %v", vars)
	}
}

func TestDomainDocContent(t *testing.T) {
	dir := t.TempDir()
	docPath := filepath.Join(dir, "DOMAIN.md")
	os.WriteFile(docPath, []byte("# Test Domain\nsome docs"), 0o644)

	domains := map[string]config.Domain{
		"test": {
			Name:      "test",
			DataDir:   dir,
			DomainDoc: "DOMAIN.md",
		},
	}

	mgr := New(domains, slog.Default())
	content, err := mgr.DomainDocContent("test")
	if err != nil {
		t.Fatalf("DomainDocContent: %v", err)
	}
	if content != "# Test Domain\nsome docs" {
		t.Errorf("unexpected content: %q", content)
	}
}

func TestDomainDocContentMissing(t *testing.T) {
	domains := map[string]config.Domain{
		"test": {
			Name:      "test",
			DataDir:   "/nonexistent",
			DomainDoc: "DOMAIN.md",
		},
	}

	mgr := New(domains, slog.Default())
	content, err := mgr.DomainDocContent("test")
	if err != nil {
		t.Fatalf("DomainDocContent: %v", err)
	}
	if content != "" {
		t.Errorf("expected empty string for missing file, got %q", content)
	}
}

func TestGetDomain(t *testing.T) {
	domains := map[string]config.Domain{
		"test": {Name: "test", DataDir: "/tmp"},
	}
	mgr := New(domains, slog.Default())

	d, ok := mgr.GetDomain("test")
	if !ok {
		t.Fatal("expected to find domain 'test'")
	}
	if d.Name != "test" {
		t.Errorf("Name = %q, want %q", d.Name, "test")
	}

	_, ok = mgr.GetDomain("nonexistent")
	if ok {
		t.Error("expected not to find domain 'nonexistent'")
	}
}
