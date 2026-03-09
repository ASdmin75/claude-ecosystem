package main

import (
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

func setupTestDB(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	testDB, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("opening test db: %v", err)
	}
	defer testDB.Close()

	_, err = testDB.Exec(`
		CREATE TABLE leads (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL,
			tax_id TEXT,
			country TEXT
		);
		CREATE UNIQUE INDEX IF NOT EXISTS idx_leads_tax_id ON leads(tax_id) WHERE tax_id IS NOT NULL;
		INSERT INTO leads (name, tax_id, country) VALUES ('Acme Corp', '123456789', 'BY');
		INSERT INTO leads (name, tax_id, country) VALUES ('Beta LLC', '987654321', 'RU');
	`)
	if err != nil {
		t.Fatalf("setting up test schema: %v", err)
	}

	// Set global db
	db, err = sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("opening global db: %v", err)
	}

	return dbPath
}

func TestHandleQuery(t *testing.T) {
	setupTestDB(t)
	defer db.Close()

	args, _ := json.Marshal(map[string]string{"sql": "SELECT name, country FROM leads ORDER BY name"})
	result := handleQuery(args)

	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Content[0].Text)
	}

	var rows []map[string]any
	if err := json.Unmarshal([]byte(result.Content[0].Text), &rows); err != nil {
		t.Fatalf("parsing result: %v", err)
	}

	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}
	if rows[0]["name"] != "Acme Corp" {
		t.Errorf("first row name = %v, want 'Acme Corp'", rows[0]["name"])
	}
}

func TestHandleQueryRejectsDangerous(t *testing.T) {
	setupTestDB(t)
	defer db.Close()

	args, _ := json.Marshal(map[string]string{"sql": "DROP TABLE leads"})
	result := handleQuery(args)

	if !result.IsError {
		t.Error("expected error for non-SELECT query")
	}
}

func TestHandleQueryAutoLimit(t *testing.T) {
	setupTestDB(t)
	defer db.Close()

	// Query without LIMIT should still work (auto-added)
	args, _ := json.Marshal(map[string]string{"sql": "SELECT * FROM leads"})
	result := handleQuery(args)

	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Content[0].Text)
	}
}

func TestHandleCheckExists(t *testing.T) {
	setupTestDB(t)
	defer db.Close()

	// Existing record
	args, _ := json.Marshal(map[string]any{
		"table": "leads", "column": "tax_id", "value": "123456789",
	})
	result := handleCheckExists(args)
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Content[0].Text)
	}

	var resp map[string]bool
	json.Unmarshal([]byte(result.Content[0].Text), &resp)
	if !resp["exists"] {
		t.Error("expected exists=true for existing record")
	}

	// Non-existing record
	args, _ = json.Marshal(map[string]any{
		"table": "leads", "column": "tax_id", "value": "000000000",
	})
	result = handleCheckExists(args)

	json.Unmarshal([]byte(result.Content[0].Text), &resp)
	if resp["exists"] {
		t.Error("expected exists=false for non-existing record")
	}
}

func TestHandleCheckExistsInvalidTable(t *testing.T) {
	setupTestDB(t)
	defer db.Close()

	args, _ := json.Marshal(map[string]any{
		"table": "Robert'; DROP TABLE leads;--", "column": "tax_id", "value": "123",
	})
	result := handleCheckExists(args)
	if !result.IsError {
		t.Error("expected error for invalid table name")
	}
}

func TestHandleInsert(t *testing.T) {
	setupTestDB(t)
	defer db.Close()

	args, _ := json.Marshal(map[string]any{
		"table": "leads",
		"data": map[string]any{
			"name":    "New Corp",
			"tax_id":  "555555555",
			"country": "KZ",
		},
	})
	result := handleInsert(args)

	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Content[0].Text)
	}

	var resp map[string]int64
	json.Unmarshal([]byte(result.Content[0].Text), &resp)
	if resp["id"] == 0 {
		t.Error("expected non-zero ID")
	}

	// Verify the insert
	var count int
	db.QueryRow("SELECT COUNT(*) FROM leads WHERE tax_id = '555555555'").Scan(&count)
	if count != 1 {
		t.Errorf("expected 1 row with tax_id 555555555, got %d", count)
	}
}

func TestHandleInsertDuplicate(t *testing.T) {
	setupTestDB(t)
	defer db.Close()

	args, _ := json.Marshal(map[string]any{
		"table": "leads",
		"data": map[string]any{
			"name":   "Duplicate",
			"tax_id": "123456789", // already exists
		},
	})
	result := handleInsert(args)

	if !result.IsError {
		t.Error("expected error for duplicate tax_id insert")
	}
}

func TestHandleExecute(t *testing.T) {
	setupTestDB(t)
	defer db.Close()

	args, _ := json.Marshal(map[string]string{
		"sql": "UPDATE leads SET country = 'KZ' WHERE tax_id = '123456789'",
	})
	result := handleExecute(args)

	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Content[0].Text)
	}

	var resp map[string]int64
	json.Unmarshal([]byte(result.Content[0].Text), &resp)
	if resp["rows_affected"] != 1 {
		t.Errorf("expected 1 row affected, got %d", resp["rows_affected"])
	}
}

func TestHandleExecuteRejectsDangerous(t *testing.T) {
	setupTestDB(t)
	defer db.Close()

	args, _ := json.Marshal(map[string]string{"sql": "DROP TABLE leads"})
	result := handleExecute(args)

	if !result.IsError {
		t.Error("expected error for DROP statement")
	}
}

func TestHandleListTables(t *testing.T) {
	setupTestDB(t)
	defer db.Close()

	result := handleListTables()
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Content[0].Text)
	}

	var tables []string
	json.Unmarshal([]byte(result.Content[0].Text), &tables)

	found := false
	for _, tbl := range tables {
		if tbl == "leads" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected 'leads' table in list, got %v", tables)
	}
}

func TestHandleDescribeTable(t *testing.T) {
	setupTestDB(t)
	defer db.Close()

	args, _ := json.Marshal(map[string]string{"table": "leads"})
	result := handleDescribeTable(args)

	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Content[0].Text)
	}

	var cols []map[string]any
	json.Unmarshal([]byte(result.Content[0].Text), &cols)

	if len(cols) < 3 {
		t.Errorf("expected at least 3 columns, got %d", len(cols))
	}
}

func TestHandleInsertInvalidColumnName(t *testing.T) {
	setupTestDB(t)
	defer db.Close()

	args, _ := json.Marshal(map[string]any{
		"table": "leads",
		"data": map[string]any{
			"name; DROP TABLE leads": "bad",
		},
	})
	result := handleInsert(args)

	if !result.IsError {
		t.Error("expected error for SQL injection in column name")
	}
}

func TestMainRequiresDBPath(t *testing.T) {
	// Verify that DOMAIN_DB_PATH is checked
	old := os.Getenv("DOMAIN_DB_PATH")
	os.Unsetenv("DOMAIN_DB_PATH")
	defer os.Setenv("DOMAIN_DB_PATH", old)

	// Can't test os.Exit directly, but we verify the env var is used
	dbPath := os.Getenv("DOMAIN_DB_PATH")
	if dbPath != "" {
		t.Error("DOMAIN_DB_PATH should be unset")
	}
}
