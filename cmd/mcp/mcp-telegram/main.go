package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"time"

	tele "gopkg.in/telebot.v4"
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
		Name:        "send_message",
		Description: "Send a text message to a Telegram chat.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"chat_id": map[string]any{
					"type":        "string",
					"description": "Telegram chat ID. Uses TELEGRAM_CHAT_ID env var if omitted.",
				},
				"text": map[string]any{
					"type":        "string",
					"description": "Message text to send.",
				},
				"parse_mode": map[string]any{
					"type":        "string",
					"description": "Parse mode: Markdown or HTML. Optional.",
				},
			},
			"required": []string{"text"},
		},
	},
	{
		Name:        "send_document",
		Description: "Send a file (document) to a Telegram chat.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"chat_id": map[string]any{
					"type":        "string",
					"description": "Telegram chat ID. Uses TELEGRAM_CHAT_ID env var if omitted.",
				},
				"file_path": map[string]any{
					"type":        "string",
					"description": "Path to the file to send.",
				},
				"caption": map[string]any{
					"type":        "string",
					"description": "Optional caption for the document.",
				},
			},
			"required": []string{"file_path"},
		},
	},
}

func getBot() (*tele.Bot, error) {
	token := os.Getenv("TELEGRAM_BOT_TOKEN")
	if token == "" {
		return nil, fmt.Errorf("TELEGRAM_BOT_TOKEN env var is not set")
	}
	pref := tele.Settings{
		Token:  token,
		Poller: &tele.LongPoller{Timeout: 1 * time.Second},
	}
	return tele.NewBot(pref)
}

func getChatID(params map[string]any) (int64, error) {
	if id, ok := params["chat_id"].(string); ok && id != "" {
		return strconv.ParseInt(id, 10, 64)
	}
	envID := os.Getenv("TELEGRAM_CHAT_ID")
	if envID == "" {
		return 0, fmt.Errorf("chat_id not provided and TELEGRAM_CHAT_ID env var is not set")
	}
	return strconv.ParseInt(envID, 10, 64)
}

func handleToolCall(params map[string]any) (any, error) {
	toolName, _ := params["name"].(string)
	args, _ := params["arguments"].(map[string]any)

	switch toolName {
	case "send_message":
		return handleSendMessage(args)
	case "send_document":
		return handleSendDocument(args)
	default:
		return nil, fmt.Errorf("unknown tool: %s", toolName)
	}
}

func handleSendMessage(args map[string]any) (any, error) {
	bot, err := getBot()
	if err != nil {
		return nil, err
	}

	chatID, err := getChatID(args)
	if err != nil {
		return nil, err
	}

	text, _ := args["text"].(string)
	if text == "" {
		return nil, fmt.Errorf("text is required")
	}

	opts := &tele.SendOptions{}
	if pm, ok := args["parse_mode"].(string); ok {
		switch pm {
		case "Markdown", "MarkdownV2":
			opts.ParseMode = tele.ModeMarkdownV2
		case "HTML":
			opts.ParseMode = tele.ModeHTML
		}
	}

	chat := &tele.Chat{ID: chatID}
	msg, err := bot.Send(chat, text, opts)
	if err != nil {
		return nil, fmt.Errorf("failed to send message: %w", err)
	}

	return map[string]any{
		"content": []map[string]any{
			{"type": "text", "text": fmt.Sprintf("Message sent successfully. Message ID: %d", msg.ID)},
		},
	}, nil
}

func handleSendDocument(args map[string]any) (any, error) {
	bot, err := getBot()
	if err != nil {
		return nil, err
	}

	chatID, err := getChatID(args)
	if err != nil {
		return nil, err
	}

	filePath, _ := args["file_path"].(string)
	if filePath == "" {
		return nil, fmt.Errorf("file_path is required")
	}

	if _, err := os.Stat(filePath); err != nil {
		return nil, fmt.Errorf("file not found: %s", filePath)
	}

	doc := &tele.Document{
		File: tele.FromDisk(filePath),
	}
	if caption, ok := args["caption"].(string); ok {
		doc.Caption = caption
	}

	chat := &tele.Chat{ID: chatID}
	msg, err := bot.Send(chat, doc)
	if err != nil {
		return nil, fmt.Errorf("failed to send document: %w", err)
	}

	return map[string]any{
		"content": []map[string]any{
			{"type": "text", "text": fmt.Sprintf("Document sent successfully. Message ID: %d", msg.ID)},
		},
	}, nil
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
				"serverInfo":     map[string]any{"name": "mcp-telegram", "version": "0.1.0"},
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
