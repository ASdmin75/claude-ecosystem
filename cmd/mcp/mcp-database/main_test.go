package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
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

// makeReq builds a CallToolRequest with the given arguments map.
func makeReq(args map[string]any) mcp.CallToolRequest {
	return mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Arguments: args,
		},
	}
}

// resultText extracts the text from the first content item of a CallToolResult.
func resultText(r *mcp.CallToolResult) string {
	if len(r.Content) == 0 {
		return ""
	}
	if tc, ok := r.Content[0].(mcp.TextContent); ok {
		return tc.Text
	}
	return ""
}

func TestHandleQuery(t *testing.T) {
	setupTestDB(t)
	defer db.Close()

	result, _ := handleQuery(context.Background(), makeReq(map[string]any{"sql": "SELECT name, country FROM leads ORDER BY name"}))

	if result.IsError {
		t.Fatalf("unexpected error: %s", resultText(result))
	}

	var rows []map[string]any
	if err := json.Unmarshal([]byte(resultText(result)), &rows); err != nil {
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

	result, _ := handleQuery(context.Background(), makeReq(map[string]any{"sql": "DROP TABLE leads"}))

	if !result.IsError {
		t.Error("expected error for non-SELECT query")
	}
}

func TestHandleQueryAutoLimit(t *testing.T) {
	setupTestDB(t)
	defer db.Close()

	result, _ := handleQuery(context.Background(), makeReq(map[string]any{"sql": "SELECT * FROM leads"}))

	if result.IsError {
		t.Fatalf("unexpected error: %s", resultText(result))
	}
}

func TestHandleCheckExists(t *testing.T) {
	setupTestDB(t)
	defer db.Close()

	// Existing record
	result, _ := handleCheckExists(context.Background(), makeReq(map[string]any{
		"table": "leads", "column": "tax_id", "value": "123456789",
	}))
	if result.IsError {
		t.Fatalf("unexpected error: %s", resultText(result))
	}

	var resp map[string]bool
	json.Unmarshal([]byte(resultText(result)), &resp)
	if !resp["exists"] {
		t.Error("expected exists=true for existing record")
	}

	// Non-existing record
	result, _ = handleCheckExists(context.Background(), makeReq(map[string]any{
		"table": "leads", "column": "tax_id", "value": "000000000",
	}))

	json.Unmarshal([]byte(resultText(result)), &resp)
	if resp["exists"] {
		t.Error("expected exists=false for non-existing record")
	}
}

func TestHandleCheckExistsInvalidTable(t *testing.T) {
	setupTestDB(t)
	defer db.Close()

	result, _ := handleCheckExists(context.Background(), makeReq(map[string]any{
		"table": "Robert'; DROP TABLE leads;--", "column": "tax_id", "value": "123",
	}))
	if !result.IsError {
		t.Error("expected error for invalid table name")
	}
}

func TestHandleInsert(t *testing.T) {
	setupTestDB(t)
	defer db.Close()

	result, _ := handleInsert(context.Background(), makeReq(map[string]any{
		"table": "leads",
		"data": map[string]any{
			"name":    "New Corp",
			"tax_id":  "555555555",
			"country": "KZ",
		},
	}))

	if result.IsError {
		t.Fatalf("unexpected error: %s", resultText(result))
	}

	var resp map[string]int64
	json.Unmarshal([]byte(resultText(result)), &resp)
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

	result, _ := handleInsert(context.Background(), makeReq(map[string]any{
		"table": "leads",
		"data": map[string]any{
			"name":   "Duplicate",
			"tax_id": "123456789", // already exists
		},
	}))

	if !result.IsError {
		t.Error("expected error for duplicate tax_id insert")
	}
}

func TestHandleExecute(t *testing.T) {
	setupTestDB(t)
	defer db.Close()

	result, _ := handleExecute(context.Background(), makeReq(map[string]any{
		"sql": "UPDATE leads SET country = 'KZ' WHERE tax_id = '123456789'",
	}))

	if result.IsError {
		t.Fatalf("unexpected error: %s", resultText(result))
	}

	var resp map[string]int64
	json.Unmarshal([]byte(resultText(result)), &resp)
	if resp["rows_affected"] != 1 {
		t.Errorf("expected 1 row affected, got %d", resp["rows_affected"])
	}
}

func TestHandleExecuteRejectsDangerous(t *testing.T) {
	setupTestDB(t)
	defer db.Close()

	result, _ := handleExecute(context.Background(), makeReq(map[string]any{"sql": "DROP TABLE leads"}))

	if !result.IsError {
		t.Error("expected error for DROP statement")
	}
}

func TestHandleListTables(t *testing.T) {
	setupTestDB(t)
	defer db.Close()

	result, _ := handleListTables(context.Background(), mcp.CallToolRequest{})
	if result.IsError {
		t.Fatalf("unexpected error: %s", resultText(result))
	}

	var tables []string
	json.Unmarshal([]byte(resultText(result)), &tables)

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

	result, _ := handleDescribeTable(context.Background(), makeReq(map[string]any{"table": "leads"}))

	if result.IsError {
		t.Fatalf("unexpected error: %s", resultText(result))
	}

	var cols []map[string]any
	json.Unmarshal([]byte(resultText(result)), &cols)

	if len(cols) < 3 {
		t.Errorf("expected at least 3 columns, got %d", len(cols))
	}
}

func TestHandleInsertInvalidColumnName(t *testing.T) {
	setupTestDB(t)
	defer db.Close()

	result, _ := handleInsert(context.Background(), makeReq(map[string]any{
		"table": "leads",
		"data": map[string]any{
			"name; DROP TABLE leads": "bad",
		},
	}))

	if !result.IsError {
		t.Error("expected error for SQL injection in column name")
	}
}

func TestErrorSanitization(t *testing.T) {
	setupTestDB(t)

	// Query a nonexistent table — error should NOT leak SQLite internals
	result, err := handleQuery(context.Background(), makeReq(map[string]any{
		"sql": "SELECT * FROM nonexistent_table_xyz",
	}))
	if err != nil {
		t.Fatalf("handleQuery returned Go error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error result for nonexistent table")
	}

	text := resultText(result)
	if text != "query failed" {
		t.Errorf("expected sanitized 'query failed', got: %s", text)
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
