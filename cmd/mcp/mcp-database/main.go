package main

import (
	"bufio"
	"encoding/json"
	"os"
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

var tools = []tool{
	{
		Name:        "query",
		Description: "Execute a read-only SQL query and return results.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"sql": map[string]any{
					"type":        "string",
					"description": "The SQL SELECT query to execute.",
				},
				"params": map[string]any{
					"type":        "array",
					"description": "Positional parameters for the query.",
					"items":       map[string]any{"type": "string"},
				},
			},
			"required": []string{"sql"},
		},
	},
	{
		Name:        "execute",
		Description: "Execute a write SQL statement (INSERT, UPDATE, DELETE).",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"sql": map[string]any{
					"type":        "string",
					"description": "The SQL statement to execute.",
				},
				"params": map[string]any{
					"type":        "array",
					"description": "Positional parameters for the statement.",
					"items":       map[string]any{"type": "string"},
				},
			},
			"required": []string{"sql"},
		},
	},
	{
		Name:        "list_tables",
		Description: "List all tables in the database.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"schema": map[string]any{
					"type":        "string",
					"description": "Database schema to list tables from. Defaults to 'public'.",
				},
			},
			"required": []string{},
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
				"schema": map[string]any{
					"type":        "string",
					"description": "Database schema. Defaults to 'public'.",
				},
			},
			"required": []string{"table"},
		},
	},
}

func main() {
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
				"serverInfo":     map[string]any{"name": "mcp-database", "version": "0.1.0"},
			}
		case "tools/list":
			resp.Result = map[string]any{"tools": tools}
		case "tools/call":
			resp.Error = map[string]any{"code": -32601, "message": "not implemented yet"}
		default:
			resp.Error = map[string]any{"code": -32601, "message": "method not found"}
		}

		enc.Encode(resp)
	}
}
