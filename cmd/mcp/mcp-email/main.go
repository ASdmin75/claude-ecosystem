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
		Name:        "send_email",
		Description: "Send an email message.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"to": map[string]any{
					"type":        "array",
					"description": "List of recipient email addresses.",
					"items":       map[string]any{"type": "string"},
				},
				"subject": map[string]any{
					"type":        "string",
					"description": "Email subject line.",
				},
				"body": map[string]any{
					"type":        "string",
					"description": "Email body content.",
				},
				"cc": map[string]any{
					"type":        "array",
					"description": "List of CC recipient email addresses.",
					"items":       map[string]any{"type": "string"},
				},
			},
			"required": []string{"to", "subject", "body"},
		},
	},
	{
		Name:        "read_inbox",
		Description: "Read recent messages from the inbox.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"limit": map[string]any{
					"type":        "integer",
					"description": "Maximum number of messages to return. Defaults to 10.",
				},
				"unread_only": map[string]any{
					"type":        "boolean",
					"description": "Whether to return only unread messages. Defaults to false.",
				},
			},
			"required": []string{},
		},
	},
	{
		Name:        "search_emails",
		Description: "Search emails by query string.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"query": map[string]any{
					"type":        "string",
					"description": "Search query to match against subject, body, and sender.",
				},
				"limit": map[string]any{
					"type":        "integer",
					"description": "Maximum number of results to return. Defaults to 20.",
				},
				"from": map[string]any{
					"type":        "string",
					"description": "Filter by sender email address.",
				},
			},
			"required": []string{"query"},
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
				"serverInfo":     map[string]any{"name": "mcp-email", "version": "0.1.0"},
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
