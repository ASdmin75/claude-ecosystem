package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/asdmin/claude-ecosystem/internal/safepath"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	tele "gopkg.in/telebot.v4"
)

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

func getChatID(args map[string]any) (int64, error) {
	if id, ok := args["chat_id"].(string); ok && id != "" {
		return strconv.ParseInt(id, 10, 64)
	}
	envID := os.Getenv("TELEGRAM_CHAT_ID")
	if envID == "" {
		return 0, fmt.Errorf("chat_id not provided and TELEGRAM_CHAT_ID env var is not set")
	}
	return strconv.ParseInt(envID, 10, 64)
}

func handleSendMessage(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	bot, err := getBot()
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	chatID, err := getChatID(req.GetArguments())
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	text, err := req.RequireString("text")
	if err != nil {
		return mcp.NewToolResultError("text is required"), nil
	}

	opts := &tele.SendOptions{}
	if pm := req.GetString("parse_mode", ""); pm != "" {
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
		return mcp.NewToolResultError(fmt.Sprintf("failed to send message: %v", err)), nil
	}

	return mcp.NewToolResultText(fmt.Sprintf("Message sent successfully. Message ID: %d", msg.ID)), nil
}

func handleSendDocument(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	bot, err := getBot()
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	chatID, err := getChatID(req.GetArguments())
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	filePath, err := req.RequireString("file_path")
	if err != nil {
		return mcp.NewToolResultError("file_path is required"), nil
	}

	// Validate path to prevent directory traversal
	if pathValidator != nil {
		filePath, err = pathValidator.Validate(filePath)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("path rejected: %v", err)), nil
		}
	}

	if _, err := os.Stat(filePath); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("file not found: %s", filePath)), nil
	}

	doc := &tele.Document{
		File:     tele.FromDisk(filePath),
		FileName: filepath.Base(filePath),
	}
	if caption := req.GetString("caption", ""); caption != "" {
		doc.Caption = caption
	}

	chat := &tele.Chat{ID: chatID}
	msg, err := bot.Send(chat, doc)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to send document: %v", err)), nil
	}

	return mcp.NewToolResultText(fmt.Sprintf("Document sent successfully. Message ID: %d", msg.ID)), nil
}

var pathValidator *safepath.Validator

func main() {
	var err error
	pathValidator, err = safepath.NewFromEnv("TELEGRAM_ALLOWED_DIRS")
	if err != nil {
		fmt.Fprintf(os.Stderr, "mcp-telegram: invalid TELEGRAM_ALLOWED_DIRS: %v\n", err)
		os.Exit(1)
	}

	s := server.NewMCPServer("mcp-telegram", "0.1.0")

	s.AddTool(mcp.NewTool("send_message",
		mcp.WithDescription("Send a text message to a Telegram chat."),
		mcp.WithString("chat_id", mcp.Description("Telegram chat ID. Uses TELEGRAM_CHAT_ID env var if omitted.")),
		mcp.WithString("text", mcp.Required(), mcp.Description("Message text to send.")),
		mcp.WithString("parse_mode", mcp.Description("Parse mode: Markdown or HTML. Optional.")),
	), handleSendMessage)

	s.AddTool(mcp.NewTool("send_document",
		mcp.WithDescription("Send a file (document) to a Telegram chat."),
		mcp.WithString("chat_id", mcp.Description("Telegram chat ID. Uses TELEGRAM_CHAT_ID env var if omitted.")),
		mcp.WithString("file_path", mcp.Required(), mcp.Description("Path to the file to send.")),
		mcp.WithString("caption", mcp.Description("Optional caption for the document.")),
	), handleSendDocument)

	if err := server.ServeStdio(s); err != nil {
		fmt.Fprintf(os.Stderr, "server error: %v\n", err)
		os.Exit(1)
	}
}
