package main

import (
	"context"
	"fmt"
	"os"
	"strconv"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"gopkg.in/gomail.v2"
)

func main() {
	s := server.NewMCPServer("mcp-email", "1.1.0")

	s.AddTool(mcp.NewTool("send_email",
		mcp.WithDescription("Send an email message with optional attachments, HTML body, CC, and BCC recipients."),
		mcp.WithArray("to", mcp.Required(), mcp.Description("List of recipient email addresses."), mcp.WithStringItems()),
		mcp.WithString("subject", mcp.Required(), mcp.Description("Email subject line.")),
		mcp.WithString("body", mcp.Required(), mcp.Description("Plain text email body.")),
		mcp.WithString("html_body", mcp.Description("HTML email body (alternative to plain text body).")),
		mcp.WithArray("cc", mcp.Description("List of CC recipient email addresses."), mcp.WithStringItems()),
		mcp.WithArray("bcc", mcp.Description("List of BCC recipient email addresses (hidden from other recipients). Use for mass distribution of reports."), mcp.WithStringItems()),
		mcp.WithString("reply_to", mcp.Description("Reply-To email address (if different from sender).")),
		mcp.WithArray("attachments", mcp.Description("List of file paths to attach."), mcp.WithStringItems()),
	), handleSendEmail)

	s.AddTool(mcp.NewTool("send_report",
		mcp.WithDescription("Send a report email to multiple recipients with attachments. Convenience wrapper: accepts a recipients list (all go to BCC for privacy), a single visible 'from_name', and file attachments. Ideal for periodic report distribution to management."),
		mcp.WithArray("recipients", mcp.Required(), mcp.Description("List of email addresses to receive the report (sent as BCC for privacy)."), mcp.WithStringItems()),
		mcp.WithString("subject", mcp.Required(), mcp.Description("Email subject line.")),
		mcp.WithString("body", mcp.Required(), mcp.Description("Plain text email body with report summary.")),
		mcp.WithString("html_body", mcp.Description("HTML email body with formatted report.")),
		mcp.WithArray("attachments", mcp.Description("List of file paths to attach (e.g. Excel reports)."), mcp.WithStringItems()),
	), handleSendReport)

	s.AddTool(mcp.NewTool("read_inbox",
		mcp.WithDescription("Read recent messages from the inbox (not implemented)."),
		mcp.WithNumber("limit", mcp.Description("Maximum number of messages to return. Defaults to 10.")),
		mcp.WithBoolean("unread_only", mcp.Description("Whether to return only unread messages. Defaults to false.")),
	), handleReadInbox)

	s.AddTool(mcp.NewTool("search_emails",
		mcp.WithDescription("Search emails by query string (not implemented)."),
		mcp.WithString("query", mcp.Required(), mcp.Description("Search query to match against subject, body, and sender.")),
		mcp.WithNumber("limit", mcp.Description("Maximum number of results to return. Defaults to 20.")),
		mcp.WithString("from", mcp.Description("Filter by sender email address.")),
	), handleSearchEmails)

	if err := server.ServeStdio(s); err != nil {
		fmt.Fprintf(os.Stderr, "server error: %v\n", err)
		os.Exit(1)
	}
}

func getDialer() (*gomail.Dialer, string, error) {
	host := os.Getenv("SMTP_HOST")
	portStr := os.Getenv("SMTP_PORT")
	user := os.Getenv("SMTP_USER")
	password := os.Getenv("SMTP_PASSWORD")
	from := os.Getenv("SMTP_FROM")

	if host == "" || portStr == "" {
		return nil, "", fmt.Errorf("SMTP_HOST and SMTP_PORT env vars are required")
	}
	if from == "" {
		from = user
	}

	port, err := strconv.Atoi(portStr)
	if err != nil {
		return nil, "", fmt.Errorf("invalid SMTP_PORT: %w", err)
	}

	return gomail.NewDialer(host, port, user, password), from, nil
}

func parseStringList(raw any) []string {
	arr, ok := raw.([]any)
	if !ok {
		return nil
	}
	var result []string
	for _, r := range arr {
		if s, ok := r.(string); ok && s != "" {
			result = append(result, s)
		}
	}
	return result
}

func attachFiles(m *gomail.Message, args map[string]any, key string) error {
	paths := parseStringList(args[key])
	for _, path := range paths {
		if _, err := os.Stat(path); err != nil {
			return fmt.Errorf("attachment not found: %s", path)
		}
		m.Attach(path)
	}
	return nil
}

func handleSendEmail(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := req.GetArguments()

	d, from, err := getDialer()
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	to := parseStringList(args["to"])
	if len(to) == 0 {
		return mcp.NewToolResultError("at least one recipient is required"), nil
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
	if cc := parseStringList(args["cc"]); len(cc) > 0 {
		m.SetHeader("Cc", cc...)
	}

	// BCC
	if bcc := parseStringList(args["bcc"]); len(bcc) > 0 {
		m.SetHeader("Bcc", bcc...)
	}

	// Reply-To
	if replyTo, ok := args["reply_to"].(string); ok && replyTo != "" {
		m.SetHeader("Reply-To", replyTo)
	}

	// Attachments
	if err := attachFiles(m, args, "attachments"); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	if err := d.DialAndSend(m); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to send email: %v", err)), nil
	}

	allRecipients := append(to, parseStringList(args["cc"])...)
	allRecipients = append(allRecipients, parseStringList(args["bcc"])...)
	return mcp.NewToolResultText(fmt.Sprintf("Email sent successfully to %d recipients (subject: %s)", len(allRecipients), subject)), nil
}

func handleSendReport(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := req.GetArguments()

	d, from, err := getDialer()
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	recipients := parseStringList(args["recipients"])
	if len(recipients) == 0 {
		return mcp.NewToolResultError("at least one recipient is required"), nil
	}

	subject, _ := args["subject"].(string)
	body, _ := args["body"].(string)
	htmlBody, _ := args["html_body"].(string)

	m := gomail.NewMessage()
	m.SetHeader("From", from)
	// Send to self, all recipients in BCC for privacy
	m.SetHeader("To", from)
	m.SetHeader("Bcc", recipients...)
	m.SetHeader("Subject", subject)

	if htmlBody != "" {
		m.SetBody("text/html", htmlBody)
		if body != "" {
			m.AddAlternative("text/plain", body)
		}
	} else {
		m.SetBody("text/plain", body)
	}

	// Attachments
	if err := attachFiles(m, args, "attachments"); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	if err := d.DialAndSend(m); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to send report: %v", err)), nil
	}

	return mcp.NewToolResultText(fmt.Sprintf("Report sent successfully to %d recipients (subject: %s)", len(recipients), subject)), nil
}

func handleReadInbox(_ context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return mcp.NewToolResultText("This tool is not implemented yet."), nil
}

func handleSearchEmails(_ context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return mcp.NewToolResultText("This tool is not implemented yet."), nil
}
