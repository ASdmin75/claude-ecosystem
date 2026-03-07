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
		Name:        "read_spreadsheet",
		Description: "Read data from an Excel spreadsheet.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path": map[string]any{
					"type":        "string",
					"description": "Path to the Excel file (.xlsx or .xls).",
				},
				"sheet": map[string]any{
					"type":        "string",
					"description": "Name of the sheet to read. Defaults to the first sheet.",
				},
				"range": map[string]any{
					"type":        "string",
					"description": "Cell range to read, e.g. 'A1:D10'. Reads entire sheet if omitted.",
				},
			},
			"required": []string{"path"},
		},
	},
	{
		Name:        "write_spreadsheet",
		Description: "Write data to an existing Excel spreadsheet.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path": map[string]any{
					"type":        "string",
					"description": "Path to the Excel file.",
				},
				"sheet": map[string]any{
					"type":        "string",
					"description": "Name of the sheet to write to.",
				},
				"cell": map[string]any{
					"type":        "string",
					"description": "Starting cell for the write, e.g. 'A1'.",
				},
				"data": map[string]any{
					"type":        "array",
					"description": "2D array of values to write (rows of columns).",
					"items": map[string]any{
						"type":  "array",
						"items": map[string]any{"type": "string"},
					},
				},
			},
			"required": []string{"path", "sheet", "cell", "data"},
		},
	},
	{
		Name:        "create_spreadsheet",
		Description: "Create a new Excel spreadsheet with optional initial data.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path": map[string]any{
					"type":        "string",
					"description": "Path for the new Excel file.",
				},
				"sheet": map[string]any{
					"type":        "string",
					"description": "Name of the initial sheet. Defaults to 'Sheet1'.",
				},
				"headers": map[string]any{
					"type":        "array",
					"description": "Column headers for the first row.",
					"items":       map[string]any{"type": "string"},
				},
			},
			"required": []string{"path"},
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
				"serverInfo":     map[string]any{"name": "mcp-excel", "version": "0.1.0"},
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
