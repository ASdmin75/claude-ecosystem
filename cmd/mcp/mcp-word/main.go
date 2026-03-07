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
		Name:        "read_document",
		Description: "Read the text content of a Word document.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path": map[string]any{
					"type":        "string",
					"description": "Path to the Word document (.docx).",
				},
				"include_formatting": map[string]any{
					"type":        "boolean",
					"description": "Whether to include formatting metadata. Defaults to false.",
				},
			},
			"required": []string{"path"},
		},
	},
	{
		Name:        "write_document",
		Description: "Append or replace content in an existing Word document.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path": map[string]any{
					"type":        "string",
					"description": "Path to the Word document.",
				},
				"content": map[string]any{
					"type":        "string",
					"description": "Text content to write.",
				},
				"mode": map[string]any{
					"type":        "string",
					"description": "Write mode: 'append' or 'replace'. Defaults to 'append'.",
					"enum":        []string{"append", "replace"},
				},
			},
			"required": []string{"path", "content"},
		},
	},
	{
		Name:        "create_document",
		Description: "Create a new Word document with the given content.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path": map[string]any{
					"type":        "string",
					"description": "Path for the new Word document.",
				},
				"title": map[string]any{
					"type":        "string",
					"description": "Document title.",
				},
				"content": map[string]any{
					"type":        "string",
					"description": "Initial text content for the document.",
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
				"serverInfo":     map[string]any{"name": "mcp-word", "version": "0.1.0"},
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
