package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strconv"

	"gopkg.in/gomail.v2"
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
		Description: "Send an email message with optional attachments and HTML body.",
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
					"description": "Plain text email body.",
				},
				"html_body": map[string]any{
					"type":        "string",
					"description": "HTML email body (alternative to plain text body).",
				},
				"cc": map[string]any{
					"type":        "array",
					"description": "List of CC recipient email addresses.",
					"items":       map[string]any{"type": "string"},
				},
				"attachments": map[string]any{
					"type":        "array",
					"description": "List of file paths to attach.",
					"items":       map[string]any{"type": "string"},
				},
			},
			"required": []string{"to", "subject", "body"},
		},
	},
	{
		Name:        "read_inbox",
		Description: "Read recent messages from the inbox (not implemented).",
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
		Description: "Search emails by query string (not implemented).",
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

func textResult(text string) any {
	return map[string]any{
		"content": []map[string]any{
			{"type": "text", "text": text},
		},
	}
}

func handleToolCall(params map[string]any) (any, error) {
	toolName, _ := params["name"].(string)
	args, _ := params["arguments"].(map[string]any)

	switch toolName {
	case "send_email":
		return handleSendEmail(args)
	case "read_inbox", "search_emails":
		return textResult("This tool is not implemented yet."), nil
	default:
		return nil, fmt.Errorf("unknown tool: %s", toolName)
	}
}

func handleSendEmail(args map[string]any) (any, error) {
	host := os.Getenv("SMTP_HOST")
	portStr := os.Getenv("SMTP_PORT")
	user := os.Getenv("SMTP_USER")
	password := os.Getenv("SMTP_PASSWORD")
	from := os.Getenv("SMTP_FROM")

	if host == "" || portStr == "" {
		return nil, fmt.Errorf("SMTP_HOST and SMTP_PORT env vars are required")
	}
	if from == "" {
		from = user
	}

	port, err := strconv.Atoi(portStr)
	if err != nil {
		return nil, fmt.Errorf("invalid SMTP_PORT: %w", err)
	}

	// Parse recipients
	toRaw, _ := args["to"].([]any)
	if len(toRaw) == 0 {
		return nil, fmt.Errorf("at least one recipient is required")
	}
	var to []string
	for _, r := range toRaw {
		if s, ok := r.(string); ok {
			to = append(to, s)
		}
	}

	subject, _ := args["subject"].(string)
	body, _ := args["body"].(string)
	htmlBody, _ := args["html_body"].(string)

	m := gomail.NewMessage()
	m.SetHeader("From", from)
	m.SetHeader("To", to...)
	m.SetHeader("Subject", subject)

	if htmlBody != "" {
		m.SetBody("text/html", htmlBody)
		if body != "" {
			m.AddAlternative("text/plain", body)
		}
	} else {
		m.SetBody("text/plain", body)
	}

	// CC
	if ccRaw, ok := args["cc"].([]any); ok {
		var cc []string
		for _, r := range ccRaw {
			if s, ok := r.(string); ok {
				cc = append(cc, s)
			}
		}
		if len(cc) > 0 {
			m.SetHeader("Cc", cc...)
		}
	}

	// Attachments
	if attachRaw, ok := args["attachments"].([]any); ok {
		for _, a := range attachRaw {
			if path, ok := a.(string); ok {
				if _, err := os.Stat(path); err != nil {
					return nil, fmt.Errorf("attachment not found: %s", path)
				}
				m.Attach(path)
			}
		}
	}

	d := gomail.NewDialer(host, port, user, password)

	if err := d.DialAndSend(m); err != nil {
		return nil, fmt.Errorf("failed to send email: %w", err)
	}

	return textResult(fmt.Sprintf("Email sent successfully to %v (subject: %s)", to, subject)), nil
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
			var params map[string]any
			if err := json.Unmarshal(req.Params, &params); err != nil {
				resp.Error = map[string]any{"code": -32602, "message": "invalid params: " + err.Error()}
			} else {
				result, err := handleToolCall(params)
				if err != nil {
					resp.Result = map[string]any{
						"content": []map[string]any{
							{"type": "text", "text": "Error: " + err.Error()},
						},
						"isError": true,
					}
				} else {
					resp.Result = result
				}
			}
		default:
			resp.Error = map[string]any{"code": -32601, "message": "method not found"}
		}

		enc.Encode(resp)
	}
}
