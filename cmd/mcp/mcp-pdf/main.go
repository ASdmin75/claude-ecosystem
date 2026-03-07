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
		Name:        "read_pdf",
		Description: "Read metadata and text content from a PDF file.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path": map[string]any{
					"type":        "string",
					"description": "Path to the PDF file.",
				},
				"pages": map[string]any{
					"type":        "string",
					"description": "Page range to read, e.g. '1-5' or '1,3,7'. Reads all pages if omitted.",
				},
			},
			"required": []string{"path"},
		},
	},
	{
		Name:        "extract_text",
		Description: "Extract plain text from a PDF file.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path": map[string]any{
					"type":        "string",
					"description": "Path to the PDF file.",
				},
				"pages": map[string]any{
					"type":        "string",
					"description": "Page range to extract, e.g. '1-5'. Extracts all pages if omitted.",
				},
				"layout": map[string]any{
					"type":        "boolean",
					"description": "Whether to preserve spatial layout. Defaults to false.",
				},
			},
			"required": []string{"path"},
		},
	},
	{
		Name:        "extract_tables",
		Description: "Extract tabular data from a PDF file.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path": map[string]any{
					"type":        "string",
					"description": "Path to the PDF file.",
				},
				"pages": map[string]any{
					"type":        "string",
					"description": "Page range to scan for tables, e.g. '1-5'. Scans all pages if omitted.",
				},
				"format": map[string]any{
					"type":        "string",
					"description": "Output format for tables: 'json' or 'csv'. Defaults to 'json'.",
					"enum":        []string{"json", "csv"},
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
				"serverInfo":     map[string]any{"name": "mcp-pdf", "version": "0.1.0"},
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
