package notify

import (
	"fmt"
	"html"
	"strings"
)

// renderEmailHTML produces an HTML email body for a task completion notification.
func renderEmailHTML(taskName, status, output, errMsg, execID string) string {
	statusColor := "#22c55e" // green
	if status == "failed" {
		statusColor = "#ef4444" // red
	} else if status == "cancelled" {
		statusColor = "#f59e0b" // amber
	}

	var content string
	if errMsg != "" {
		content = fmt.Sprintf(`<h3 style="color:#ef4444">Error</h3><pre style="background:#fef2f2;padding:12px;border-radius:6px;overflow-x:auto;font-size:13px">%s</pre>`, html.EscapeString(errMsg))
	}
	if output != "" {
		// Truncate very long output for email readability.
		truncated := output
		if len(truncated) > 10000 {
			truncated = truncated[:10000] + "\n\n... (truncated)"
		}
		content += fmt.Sprintf(`<h3>Output</h3><pre style="background:#f8fafc;padding:12px;border-radius:6px;overflow-x:auto;font-size:13px;border:1px solid #e2e8f0">%s</pre>`, html.EscapeString(truncated))
	}

	var execInfo string
	if execID != "" {
		execInfo = fmt.Sprintf(`<p style="color:#94a3b8;font-size:12px">Execution ID: %s</p>`, html.EscapeString(execID))
	}

	return fmt.Sprintf(`<!DOCTYPE html>
<html>
<head><meta charset="utf-8"></head>
<body style="font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',Roboto,sans-serif;margin:0;padding:0;background:#f1f5f9">
<div style="max-width:640px;margin:24px auto;background:#fff;border-radius:8px;overflow:hidden;box-shadow:0 1px 3px rgba(0,0,0,0.1)">
  <div style="background:%s;padding:20px 24px">
    <h1 style="margin:0;color:#fff;font-size:18px">Task: %s</h1>
    <p style="margin:4px 0 0;color:rgba(255,255,255,0.9);font-size:14px">Status: %s</p>
  </div>
  <div style="padding:24px">
    %s
    %s
  </div>
  <div style="padding:16px 24px;background:#f8fafc;border-top:1px solid #e2e8f0;font-size:12px;color:#94a3b8">
    Sent by Claude Ecosystem
  </div>
</div>
</body>
</html>`,
		statusColor,
		html.EscapeString(taskName),
		html.EscapeString(status),
		content,
		execInfo,
	)
}

// renderEmailPlain produces a plain-text fallback for email clients that don't support HTML.
func renderEmailPlain(taskName, status, output, errMsg, execID string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Task: %s\nStatus: %s\n", taskName, status)
	if execID != "" {
		fmt.Fprintf(&b, "Execution ID: %s\n", execID)
	}
	b.WriteString("\n")

	if errMsg != "" {
		fmt.Fprintf(&b, "--- Error ---\n%s\n\n", errMsg)
	}
	if output != "" {
		truncated := output
		if len(truncated) > 10000 {
			truncated = truncated[:10000] + "\n\n... (truncated)"
		}
		fmt.Fprintf(&b, "--- Output ---\n%s\n", truncated)
	}

	b.WriteString("\n-- Sent by Claude Ecosystem\n")
	return b.String()
}
