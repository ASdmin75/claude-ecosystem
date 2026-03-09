package mcpmanager

import (
	"encoding/json"
	"log/slog"
	"os"
	"testing"

	"github.com/asdmin/claude-ecosystem/internal/config"
)

func TestGenerateConfigFileWithEnv(t *testing.T) {
	cfgs := []config.MCPServerConfig{
		{
			Name:    "database",
			Command: "./bin/mcp-database",
			Env:     map[string]string{"EXISTING_KEY": "existing_value"},
		},
		{
			Name:    "excel",
			Command: "./bin/mcp-excel",
		},
	}

	mgr := New(cfgs, slog.Default())

	extraEnv := map[string]string{
		"DOMAIN_DB_PATH":  "/data/leads/leads.db",
		"DOMAIN_DATA_DIR": "/data/leads",
	}

	path, err := mgr.GenerateConfigFileWithEnv([]string{"database", "excel"}, extraEnv)
	if err != nil {
		t.Fatalf("GenerateConfigFileWithEnv: %v", err)
	}
	defer os.Remove(path)

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading config file: %v", err)
	}

	var cfg mcpConfigFile
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("parsing config file: %v", err)
	}

	// Check database server has both original and extra env
	dbEntry, ok := cfg.MCPServers["database"]
	if !ok {
		t.Fatal("database server not found in config")
	}
	if dbEntry.Env["EXISTING_KEY"] != "existing_value" {
		t.Errorf("EXISTING_KEY = %q, want %q", dbEntry.Env["EXISTING_KEY"], "existing_value")
	}
	if dbEntry.Env["DOMAIN_DB_PATH"] != "/data/leads/leads.db" {
		t.Errorf("DOMAIN_DB_PATH = %q, want %q", dbEntry.Env["DOMAIN_DB_PATH"], "/data/leads/leads.db")
	}
	if dbEntry.Env["DOMAIN_DATA_DIR"] != "/data/leads" {
		t.Errorf("DOMAIN_DATA_DIR = %q, want %q", dbEntry.Env["DOMAIN_DATA_DIR"], "/data/leads")
	}

	// Check excel server also gets extra env
	excelEntry, ok := cfg.MCPServers["excel"]
	if !ok {
		t.Fatal("excel server not found in config")
	}
	if excelEntry.Env["DOMAIN_DB_PATH"] != "/data/leads/leads.db" {
		t.Errorf("excel DOMAIN_DB_PATH = %q, want %q", excelEntry.Env["DOMAIN_DB_PATH"], "/data/leads/leads.db")
	}
}

func TestGenerateConfigFileWithNilEnv(t *testing.T) {
	cfgs := []config.MCPServerConfig{
		{
			Name:    "test",
			Command: "./bin/test",
			Env:     map[string]string{"KEY": "val"},
		},
	}

	mgr := New(cfgs, slog.Default())

	path, err := mgr.GenerateConfigFileWithEnv([]string{"test"}, nil)
	if err != nil {
		t.Fatalf("GenerateConfigFileWithEnv: %v", err)
	}
	defer os.Remove(path)

	data, _ := os.ReadFile(path)
	var cfg mcpConfigFile
	json.Unmarshal(data, &cfg)

	entry := cfg.MCPServers["test"]
	if entry.Env["KEY"] != "val" {
		t.Errorf("KEY = %q, want %q", entry.Env["KEY"], "val")
	}
}

func TestGenerateConfigFileDelegatesToWithEnv(t *testing.T) {
	cfgs := []config.MCPServerConfig{
		{Name: "test", Command: "./bin/test"},
	}

	mgr := New(cfgs, slog.Default())

	path, err := mgr.GenerateConfigFile([]string{"test"})
	if err != nil {
		t.Fatalf("GenerateConfigFile: %v", err)
	}
	defer os.Remove(path)

	if _, err := os.Stat(path); err != nil {
		t.Errorf("config file not created: %v", err)
	}
}
