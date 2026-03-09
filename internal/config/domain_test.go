package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDomainDBPath(t *testing.T) {
	d := Domain{DataDir: "data/leads", DB: "leads.db"}
	want := filepath.Join("data/leads", "leads.db")
	if got := d.DBPath(); got != want {
		t.Errorf("DBPath() = %q, want %q", got, want)
	}
}

func TestDomainDBPathEmpty(t *testing.T) {
	d := Domain{DataDir: "data/leads"}
	if got := d.DBPath(); got != "" {
		t.Errorf("DBPath() = %q, want empty string", got)
	}
}

func TestDomainDocPath(t *testing.T) {
	d := Domain{DataDir: "data/leads", DomainDoc: "DOMAIN.md"}
	want := filepath.Join("data/leads", "DOMAIN.md")
	if got := d.DomainDocPath(); got != want {
		t.Errorf("DomainDocPath() = %q, want %q", got, want)
	}
}

func TestDomainDocPathEmpty(t *testing.T) {
	d := Domain{DataDir: "data/leads"}
	if got := d.DomainDocPath(); got != "" {
		t.Errorf("DomainDocPath() = %q, want empty string", got)
	}
}

func TestLoadConfigWithDomains(t *testing.T) {
	content := `
tasks:
  - name: test-task
    prompt: "do something"
    domain: mydom
domains:
  mydom:
    description: "test domain"
    data_dir: /tmp/test-domain-data
    db: test.db
    schema: |
      CREATE TABLE IF NOT EXISTS items (id INTEGER PRIMARY KEY);
    domain_doc: DOMAIN.md
    tasks: [test-task]
`
	f, err := os.CreateTemp("", "config-domain-*.yaml")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(f.Name())
	f.WriteString(content)
	f.Close()

	cfg, err := Load(f.Name())
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if len(cfg.Domains) != 1 {
		t.Fatalf("expected 1 domain, got %d", len(cfg.Domains))
	}

	d, ok := cfg.Domains["mydom"]
	if !ok {
		t.Fatal("domain 'mydom' not found")
	}
	if d.Name != "mydom" {
		t.Errorf("domain.Name = %q, want %q", d.Name, "mydom")
	}
	if d.Description != "test domain" {
		t.Errorf("domain.Description = %q, want %q", d.Description, "test domain")
	}
	if d.DB != "test.db" {
		t.Errorf("domain.DB = %q, want %q", d.DB, "test.db")
	}
	if cfg.Tasks[0].Domain != "mydom" {
		t.Errorf("task.Domain = %q, want %q", cfg.Tasks[0].Domain, "mydom")
	}
}

func TestValidateDomainUnknownReference(t *testing.T) {
	cfg := &Config{
		Tasks: []Task{
			{Name: "t1", Prompt: "p", Domain: "nonexistent"},
		},
	}
	if err := Validate(cfg); err == nil {
		t.Error("expected validation error for unknown domain reference")
	}
}

func TestValidateDomainUnknownTask(t *testing.T) {
	cfg := &Config{
		Tasks: []Task{
			{Name: "t1", Prompt: "p"},
		},
		Domains: map[string]Domain{
			"d1": {Name: "d1", DataDir: "/tmp", Tasks: []string{"nonexistent"}},
		},
	}
	if err := Validate(cfg); err == nil {
		t.Error("expected validation error for domain referencing unknown task")
	}
}

func TestExpandDomainEnvVars(t *testing.T) {
	os.Setenv("TEST_DOMAIN_DIR", "/custom/path")
	defer os.Unsetenv("TEST_DOMAIN_DIR")

	cfg := &Config{
		Domains: map[string]Domain{
			"d1": {DataDir: "${TEST_DOMAIN_DIR}/data", DB: "test.db"},
		},
	}
	expandConfigEnvVars(cfg)

	d := cfg.Domains["d1"]
	if d.DataDir != "/custom/path/data" {
		t.Errorf("domain.DataDir = %q, want %q", d.DataDir, "/custom/path/data")
	}
}
