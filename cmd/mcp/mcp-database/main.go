package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	_ "modernc.org/sqlite"
)

// dangerousPattern matches SQL statements that should be rejected.
var dangerousPattern = regexp.MustCompile(`(?i)\b(DROP|ALTER|ATTACH|DETACH|VACUUM|REINDEX)\b`)

// identifierPattern validates table/column names to prevent SQL injection.
var identifierPattern = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`)

var db *sql.DB

func main() {
	dbPath := os.Getenv("DOMAIN_DB_PATH")
	if dbPath == "" {
		fmt.Fprintf(os.Stderr, "DOMAIN_DB_PATH environment variable is required\n")
		os.Exit(1)
	}

	var err error
	db, err = sql.Open("sqlite", dbPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to open database %s: %v\n", dbPath, err)
		os.Exit(1)
	}
	defer db.Close()

	s := server.NewMCPServer("mcp-database", "1.0.0")

	s.AddTool(mcp.NewTool("query",
		mcp.WithDescription("Execute a read-only SQL query (SELECT) and return results as JSON array."),
		mcp.WithString("sql", mcp.Required(), mcp.Description("The SQL SELECT query to execute.")),
	), handleQuery)

	s.AddTool(mcp.NewTool("execute",
		mcp.WithDescription("Execute a write SQL statement (INSERT, UPDATE, DELETE). Returns rows affected."),
		mcp.WithString("sql", mcp.Required(), mcp.Description("The SQL statement to execute.")),
	), handleExecute)

	s.AddTool(mcp.NewTool("list_tables",
		mcp.WithDescription("List all tables in the database."),
	), handleListTables)

	s.AddTool(mcp.NewTool("describe_table",
		mcp.WithDescription("Describe the columns and types of a table."),
		mcp.WithString("table", mcp.Required(), mcp.Description("Name of the table to describe.")),
	), handleDescribeTable)

	s.AddTool(mcp.NewTool("check_exists",
		mcp.WithDescription("Check if a record exists in a table by column value. Returns true/false. Use for deduplication before inserting."),
		mcp.WithString("table", mcp.Required(), mcp.Description("Table name to check.")),
		mcp.WithString("column", mcp.Required(), mcp.Description("Column name to match.")),
		mcp.WithString("value", mcp.Required(), mcp.Description("Value to search for.")),
	), handleCheckExists)

	s.AddTool(mcp.NewTool("insert",
		mcp.WithDescription("Insert a record into a table. Accepts table name and a JSON object of column:value pairs. Returns the inserted row ID."),
		mcp.WithString("table", mcp.Required(), mcp.Description("Table name to insert into.")),
		mcp.WithObject("data", mcp.Required(), mcp.Description("Key-value pairs of column names and values to insert.")),
	), handleInsert)

	s.AddTool(mcp.NewTool("batch_insert",
		mcp.WithDescription("Insert multiple records into a table in one call within a transaction. Much more efficient than calling insert repeatedly. Returns count of inserted rows and any errors."),
		mcp.WithString("table", mcp.Required(), mcp.Description("Table name to insert into.")),
		mcp.WithArray("rows", mcp.Required(), mcp.Description("Array of JSON objects, each with column:value pairs to insert.")),
	), handleBatchInsert)

	if err := server.ServeStdio(s); err != nil {
		fmt.Fprintf(os.Stderr, "server error: %v\n", err)
		os.Exit(1)
	}
}

func handleQuery(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	sqlStr := req.GetString("sql", "")
	if sqlStr == "" {
		return mcp.NewToolResultError("sql parameter is required"), nil
	}

	upper := strings.TrimSpace(strings.ToUpper(sqlStr))
	if !strings.HasPrefix(upper, "SELECT") && !strings.HasPrefix(upper, "WITH") &&
		!strings.HasPrefix(upper, "PRAGMA") {
		return mcp.NewToolResultError("query tool only accepts SELECT/WITH/PRAGMA statements"), nil
	}

	if dangerousPattern.MatchString(sqlStr) {
		return mcp.NewToolResultError("query contains disallowed SQL keywords"), nil
	}

	// Add LIMIT if not present by wrapping in a subquery to avoid breaking
	// queries with ORDER BY / GROUP BY clauses.
	if !strings.Contains(strings.ToUpper(sqlStr), "LIMIT") {
		sqlStr = "SELECT * FROM (" + sqlStr + ") LIMIT 1000"
	}

	rows, err := db.Query(sqlStr)
	if err != nil {
		return safeError("query", err)
	}
	defer rows.Close()

	result, err := rowsToJSON(rows)
	if err != nil {
		return safeError("reading results", err)
	}

	data, _ := json.Marshal(result)
	return mcp.NewToolResultText(string(data)), nil
}

func handleExecute(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	sqlStr := req.GetString("sql", "")
	if sqlStr == "" {
		return mcp.NewToolResultError("sql parameter is required"), nil
	}

	if dangerousPattern.MatchString(sqlStr) {
		return mcp.NewToolResultError("statement contains disallowed SQL keywords (DROP, ALTER, ATTACH, etc.)"), nil
	}

	upper := strings.TrimSpace(strings.ToUpper(sqlStr))
	if strings.HasPrefix(upper, "SELECT") {
		return mcp.NewToolResultError("use the 'query' tool for SELECT statements"), nil
	}

	res, err := db.Exec(sqlStr)
	if err != nil {
		return safeError("execute", err)
	}

	affected, _ := res.RowsAffected()
	return mcp.NewToolResultText(fmt.Sprintf(`{"rows_affected": %d}`, affected)), nil
}

func handleListTables(_ context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	rows, err := db.Query("SELECT name FROM sqlite_master WHERE type='table' ORDER BY name")
	if err != nil {
		return safeError("list tables", err)
	}
	defer rows.Close()

	var tables []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return safeError("list tables", err)
		}
		tables = append(tables, name)
	}

	data, _ := json.Marshal(tables)
	return mcp.NewToolResultText(string(data)), nil
}

func handleDescribeTable(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	table, err := req.RequireString("table")
	if err != nil {
		return mcp.NewToolResultError("table parameter is required"), nil
	}

	if !identifierPattern.MatchString(table) {
		return mcp.NewToolResultError("invalid table name"), nil
	}

	rows, err := db.Query(fmt.Sprintf("PRAGMA table_info(%s)", table))
	if err != nil {
		return safeError("describe table", err)
	}
	defer rows.Close()

	result, err := rowsToJSON(rows)
	if err != nil {
		return safeError("reading results", err)
	}

	data, _ := json.Marshal(result)
	return mcp.NewToolResultText(string(data)), nil
}

func handleCheckExists(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	table, err := req.RequireString("table")
	if err != nil {
		return mcp.NewToolResultError("table parameter is required"), nil
	}
	column, err := req.RequireString("column")
	if err != nil {
		return mcp.NewToolResultError("column parameter is required"), nil
	}
	value, err := req.RequireString("value")
	if err != nil {
		return mcp.NewToolResultError("value parameter is required"), nil
	}

	if !identifierPattern.MatchString(table) {
		return mcp.NewToolResultError("invalid table name"), nil
	}
	if !identifierPattern.MatchString(column) {
		return mcp.NewToolResultError("invalid column name"), nil
	}

	query := fmt.Sprintf("SELECT 1 FROM %s WHERE %s = ? LIMIT 1", table, column)
	var dummy int
	err = db.QueryRow(query, value).Scan(&dummy)
	if err == sql.ErrNoRows {
		return mcp.NewToolResultText(`{"exists": false}`), nil
	}
	if err != nil {
		return safeError("check_exists", err)
	}
	return mcp.NewToolResultText(`{"exists": true}`), nil
}

func handleInsert(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	table, err := req.RequireString("table")
	if err != nil {
		return mcp.NewToolResultError("table parameter is required"), nil
	}

	args := req.GetArguments()
	dataRaw, ok := args["data"]
	if !ok {
		return mcp.NewToolResultError("data parameter is required"), nil
	}

	dataMap, ok := dataRaw.(map[string]any)
	if !ok {
		return mcp.NewToolResultError("data must be a JSON object"), nil
	}

	if !identifierPattern.MatchString(table) {
		return mcp.NewToolResultError("invalid table name"), nil
	}
	if len(dataMap) == 0 {
		return mcp.NewToolResultError("data must contain at least one column"), nil
	}

	var columns []string
	var placeholders []string
	var values []any

	for col, val := range dataMap {
		if !identifierPattern.MatchString(col) {
			return mcp.NewToolResultError(fmt.Sprintf("invalid column name: %s", col)), nil
		}
		columns = append(columns, col)
		placeholders = append(placeholders, "?")
		values = append(values, val)
	}

	query := fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s)",
		table,
		strings.Join(columns, ", "),
		strings.Join(placeholders, ", "),
	)

	res, err := db.Exec(query, values...)
	if err != nil {
		return safeError("insert", err)
	}

	id, _ := res.LastInsertId()
	return mcp.NewToolResultText(fmt.Sprintf(`{"id": %d}`, id)), nil
}

func handleBatchInsert(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	table, err := req.RequireString("table")
	if err != nil {
		return mcp.NewToolResultError("table parameter is required"), nil
	}
	if !identifierPattern.MatchString(table) {
		return mcp.NewToolResultError("invalid table name"), nil
	}

	args := req.GetArguments()
	rowsRaw, ok := args["rows"].([]any)
	if !ok || len(rowsRaw) == 0 {
		return mcp.NewToolResultError("rows must be a non-empty array"), nil
	}

	tx, err := db.Begin()
	if err != nil {
		return safeError("begin transaction", err)
	}
	defer tx.Rollback() // no-op after successful Commit

	inserted := 0
	var errors []string

	for i, item := range rowsRaw {
		dataMap, ok := item.(map[string]any)
		if !ok {
			errors = append(errors, fmt.Sprintf("#%d: not a JSON object", i+1))
			continue
		}
		if len(dataMap) == 0 {
			errors = append(errors, fmt.Sprintf("#%d: empty object", i+1))
			continue
		}

		var columns []string
		var placeholders []string
		var values []any

		for col, val := range dataMap {
			if !identifierPattern.MatchString(col) {
				errors = append(errors, fmt.Sprintf("#%d: invalid column name: %s", i+1, col))
				continue
			}
			columns = append(columns, col)
			placeholders = append(placeholders, "?")
			values = append(values, val)
		}

		query := fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s)",
			table,
			strings.Join(columns, ", "),
			strings.Join(placeholders, ", "),
		)

		_, err := tx.Exec(query, values...)
		if err != nil {
			fmt.Fprintf(os.Stderr, "mcp-database: batch insert row #%d: %v\n", i+1, err)
			errors = append(errors, fmt.Sprintf("#%d: insert failed", i+1))
			continue
		}
		inserted++
	}

	if err := tx.Commit(); err != nil {
		return safeError("commit", err)
	}

	result := fmt.Sprintf(`{"inserted": %d, "errors": %d, "total": %d}`, inserted, len(errors), len(rowsRaw))
	if len(errors) > 0 {
		result = fmt.Sprintf(`{"inserted": %d, "errors": %d, "total": %d, "error_details": %q}`,
			inserted, len(errors), len(rowsRaw), strings.Join(errors, "; "))
	}
	return mcp.NewToolResultText(result), nil
}

// safeError logs the detailed error to stderr and returns a sanitized
// error result that does not expose internal details to the client.
func safeError(operation string, err error) (*mcp.CallToolResult, error) {
	fmt.Fprintf(os.Stderr, "mcp-database: %s: %v\n", operation, err)
	return mcp.NewToolResultError(operation + " failed"), nil
}

// rowsToJSON converts sql.Rows to a slice of maps.
func rowsToJSON(rows *sql.Rows) ([]map[string]any, error) {
	cols, err := rows.Columns()
	if err != nil {
		return nil, err
	}

	var result []map[string]any
	for rows.Next() {
		values := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range values {
			ptrs[i] = &values[i]
		}

		if err := rows.Scan(ptrs...); err != nil {
			return nil, err
		}

		row := make(map[string]any)
		for i, col := range cols {
			v := values[i]
			if b, ok := v.([]byte); ok {
				v = string(b)
			}
			row[col] = v
		}
		result = append(result, row)
	}
	return result, rows.Err()
}
