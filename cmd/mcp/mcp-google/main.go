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
		Name:        "read_doc",
		Description: "Read the content of a Google Doc by document ID.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"document_id": map[string]any{
					"type":        "string",
					"description": "The Google Docs document ID.",
				},
			},
			"required": []string{"document_id"},
		},
	},
	{
		Name:        "write_doc",
		Description: "Append or insert content into a Google Doc.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"document_id": map[string]any{
					"type":        "string",
					"description": "The Google Docs document ID.",
				},
				"content": map[string]any{
					"type":        "string",
					"description": "Text content to insert.",
				},
				"index": map[string]any{
					"type":        "integer",
					"description": "Character index at which to insert. Appends to end if omitted.",
				},
			},
			"required": []string{"document_id", "content"},
		},
	},
	{
		Name:        "read_sheet",
		Description: "Read data from a Google Sheets spreadsheet.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"spreadsheet_id": map[string]any{
					"type":        "string",
					"description": "The Google Sheets spreadsheet ID.",
				},
				"range": map[string]any{
					"type":        "string",
					"description": "A1 notation range to read, e.g. 'Sheet1!A1:D10'.",
				},
			},
			"required": []string{"spreadsheet_id", "range"},
		},
	},
	{
		Name:        "write_sheet",
		Description: "Write data to a Google Sheets spreadsheet.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"spreadsheet_id": map[string]any{
					"type":        "string",
					"description": "The Google Sheets spreadsheet ID.",
				},
				"range": map[string]any{
					"type":        "string",
					"description": "A1 notation range to write to, e.g. 'Sheet1!A1'.",
				},
				"values": map[string]any{
					"type":        "array",
					"description": "2D array of values to write (rows of columns).",
					"items": map[string]any{
						"type":  "array",
						"items": map[string]any{"type": "string"},
					},
				},
			},
			"required": []string{"spreadsheet_id", "range", "values"},
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
				"serverInfo":     map[string]any{"name": "mcp-google", "version": "0.1.0"},
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
