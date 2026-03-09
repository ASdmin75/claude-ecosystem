package main

import (
	"bufio"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"

	_ "modernc.org/sqlite"
)

type jsonRPCRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      any             `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type jsonRPCResponse struct {
	JSONRPC string `json:"jsonrpc"`
	ID      any    `json:"id"`
	Result  any    `json:"result,omitempty"`
	Error   any    `json:"error,omitempty"`
}

type tool struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
}

type toolCallParams struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

type contentItem struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type toolResult struct {
	Content []contentItem `json:"content"`
	IsError bool          `json:"isError,omitempty"`
}

var tools = []tool{
	{
		Name:        "query",
		Description: "Execute a read-only SQL query (SELECT) and return results as JSON array.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"sql": map[string]any{
					"type":        "string",
					"description": "The SQL SELECT query to execute.",
				},
			},
			"required": []string{"sql"},
		},
	},
	{
		Name:        "execute",
		Description: "Execute a write SQL statement (INSERT, UPDATE, DELETE). Returns rows affected.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"sql": map[string]any{
					"type":        "string",
					"description": "The SQL statement to execute.",
				},
			},
			"required": []string{"sql"},
		},
	},
	{
		Name:        "list_tables",
		Description: "List all tables in the database.",
		InputSchema: map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		},
	},
	{
		Name:        "describe_table",
		Description: "Describe the columns and types of a table.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"table": map[string]any{
					"type":        "string",
					"description": "Name of the table to describe.",
				},
			},
			"required": []string{"table"},
		},
	},
	{
		Name:        "check_exists",
		Description: "Check if a record exists in a table by column value. Returns true/false. Use for deduplication before inserting.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"table": map[string]any{
					"type":        "string",
					"description": "Table name to check.",
				},
				"column": map[string]any{
					"type":        "string",
					"description": "Column name to match.",
				},
				"value": map[string]any{
					"type":        "string",
					"description": "Value to search for.",
				},
			},
			"required": []string{"table", "column", "value"},
		},
	},
	{
		Name:        "insert",
		Description: "Insert a record into a table. Accepts table name and a JSON object of column:value pairs. Returns the inserted row ID.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"table": map[string]any{
					"type":        "string",
					"description": "Table name to insert into.",
				},
				"data": map[string]any{
					"type":        "object",
					"description": "Key-value pairs of column names and values to insert.",
				},
			},
			"required": []string{"table", "data"},
		},
	},
}

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

	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)
	enc := json.NewEncoder(os.Stdout)

	for scanner.Scan() {
		var req jsonRPCRequest
		if err := json.Unmarshal(scanner.Bytes(), &req); err != nil {
			continue
		}

		var resp jsonRPCResponse
		resp.JSONRPC = "2.0"
		resp.ID = req.ID

		switch req.Method {
		case "initialize":
			resp.Result = map[string]any{
				"protocolVersion": "2024-11-05",
				"capabilities":   map[string]any{"tools": map[string]any{}},
				"serverInfo":     map[string]any{"name": "mcp-database", "version": "1.0.0"},
			}
		case "tools/list":
			resp.Result = map[string]any{"tools": tools}
		case "tools/call":
			resp.Result = handleToolCall(req.Params)
		default:
			resp.Error = map[string]any{"code": -32601, "message": "method not found"}
		}

		enc.Encode(resp)
	}
}

func handleToolCall(params json.RawMessage) toolResult {
	var p toolCallParams
	if err := json.Unmarshal(params, &p); err != nil {
		return errorResult("invalid params: " + err.Error())
	}

	switch p.Name {
	case "query":
		return handleQuery(p.Arguments)
	case "execute":
		return handleExecute(p.Arguments)
	case "list_tables":
		return handleListTables()
	case "describe_table":
		return handleDescribeTable(p.Arguments)
	case "check_exists":
		return handleCheckExists(p.Arguments)
	case "insert":
		return handleInsert(p.Arguments)
	default:
		return errorResult("unknown tool: " + p.Name)
	}
}

func handleQuery(args json.RawMessage) toolResult {
	var a struct {
		SQL string `json:"sql"`
	}
	if err := json.Unmarshal(args, &a); err != nil {
		return errorResult("invalid arguments: " + err.Error())
	}

	upper := strings.TrimSpace(strings.ToUpper(a.SQL))
	if !strings.HasPrefix(upper, "SELECT") && !strings.HasPrefix(upper, "WITH") &&
		!strings.HasPrefix(upper, "PRAGMA") {
		return errorResult("query tool only accepts SELECT/WITH/PRAGMA statements")
	}

	if dangerousPattern.MatchString(a.SQL) {
		return errorResult("query contains disallowed SQL keywords")
	}

	// Add LIMIT if not present
	sqlStr := a.SQL
	if !strings.Contains(strings.ToUpper(sqlStr), "LIMIT") {
		sqlStr = sqlStr + " LIMIT 1000"
	}

	rows, err := db.Query(sqlStr)
	if err != nil {
		return errorResult("query error: " + err.Error())
	}
	defer rows.Close()

	result, err := rowsToJSON(rows)
	if err != nil {
		return errorResult("reading results: " + err.Error())
	}

	data, _ := json.Marshal(result)
	return textResult(string(data))
}

func handleExecute(args json.RawMessage) toolResult {
	var a struct {
		SQL string `json:"sql"`
	}
	if err := json.Unmarshal(args, &a); err != nil {
		return errorResult("invalid arguments: " + err.Error())
	}

	if dangerousPattern.MatchString(a.SQL) {
		return errorResult("statement contains disallowed SQL keywords (DROP, ALTER, ATTACH, etc.)")
	}

	upper := strings.TrimSpace(strings.ToUpper(a.SQL))
	if strings.HasPrefix(upper, "SELECT") {
		return errorResult("use the 'query' tool for SELECT statements")
	}

	res, err := db.Exec(a.SQL)
	if err != nil {
		return errorResult("execute error: " + err.Error())
	}

	affected, _ := res.RowsAffected()
	return textResult(fmt.Sprintf(`{"rows_affected": %d}`, affected))
}

func handleListTables() toolResult {
	rows, err := db.Query("SELECT name FROM sqlite_master WHERE type='table' ORDER BY name")
	if err != nil {
		return errorResult("error listing tables: " + err.Error())
	}
	defer rows.Close()

	var tables []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return errorResult("error scanning table name: " + err.Error())
		}
		tables = append(tables, name)
	}

	data, _ := json.Marshal(tables)
	return textResult(string(data))
}

func handleDescribeTable(args json.RawMessage) toolResult {
	var a struct {
		Table string `json:"table"`
	}
	if err := json.Unmarshal(args, &a); err != nil {
		return errorResult("invalid arguments: " + err.Error())
	}

	if !identifierPattern.MatchString(a.Table) {
		return errorResult("invalid table name")
	}

	rows, err := db.Query(fmt.Sprintf("PRAGMA table_info(%s)", a.Table))
	if err != nil {
		return errorResult("error describing table: " + err.Error())
	}
	defer rows.Close()

	result, err := rowsToJSON(rows)
	if err != nil {
		return errorResult("reading results: " + err.Error())
	}

	data, _ := json.Marshal(result)
	return textResult(string(data))
}

func handleCheckExists(args json.RawMessage) toolResult {
	var a struct {
		Table  string `json:"table"`
		Column string `json:"column"`
		Value  string `json:"value"`
	}
	if err := json.Unmarshal(args, &a); err != nil {
		return errorResult("invalid arguments: " + err.Error())
	}

	if !identifierPattern.MatchString(a.Table) {
		return errorResult("invalid table name")
	}
	if !identifierPattern.MatchString(a.Column) {
		return errorResult("invalid column name")
	}

	query := fmt.Sprintf("SELECT 1 FROM %s WHERE %s = ? LIMIT 1", a.Table, a.Column)
	var dummy int
	err := db.QueryRow(query, a.Value).Scan(&dummy)
	if err == sql.ErrNoRows {
		return textResult(`{"exists": false}`)
	}
	if err != nil {
		return errorResult("check_exists error: " + err.Error())
	}
	return textResult(`{"exists": true}`)
}

func handleInsert(args json.RawMessage) toolResult {
	var a struct {
		Table string         `json:"table"`
		Data  map[string]any `json:"data"`
	}
	if err := json.Unmarshal(args, &a); err != nil {
		return errorResult("invalid arguments: " + err.Error())
	}

	if !identifierPattern.MatchString(a.Table) {
		return errorResult("invalid table name")
	}
	if len(a.Data) == 0 {
		return errorResult("data must contain at least one column")
	}

	var columns []string
	var placeholders []string
	var values []any

	for col, val := range a.Data {
		if !identifierPattern.MatchString(col) {
			return errorResult(fmt.Sprintf("invalid column name: %s", col))
		}
		columns = append(columns, col)
		placeholders = append(placeholders, "?")
		values = append(values, val)
	}

	query := fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s)",
		a.Table,
		strings.Join(columns, ", "),
		strings.Join(placeholders, ", "),
	)

	res, err := db.Exec(query, values...)
	if err != nil {
		return errorResult("insert error: " + err.Error())
	}

	id, _ := res.LastInsertId()
	return textResult(fmt.Sprintf(`{"id": %d}`, id))
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

func textResult(text string) toolResult {
	return toolResult{
		Content: []contentItem{{Type: "text", Text: text}},
	}
}

func errorResult(msg string) toolResult {
	return toolResult{
		Content: []contentItem{{Type: "text", Text: msg}},
		IsError: true,
	}
}
