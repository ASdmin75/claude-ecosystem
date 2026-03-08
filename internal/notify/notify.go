package notify

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/asdmin/claude-ecosystem/internal/config"
	"github.com/asdmin/claude-ecosystem/internal/events"
	"gopkg.in/gomail.v2"
)

// SMTPConfig holds SMTP connection parameters for email notifications.
type SMTPConfig struct {
	Host     string
	Port     int
	User     string
	Password string
	From     string
}

// Handler sends email and webhook notifications when tasks complete.
type Handler struct {
	tasks  []config.Task
	smtp   *SMTPConfig
	logger *slog.Logger
}

// NewHandler creates a notification handler. If SMTP env vars are set,
// email notifications are enabled. Tasks are looked up by name from the
// provided slice when events arrive.
func NewHandler(tasks []config.Task, logger *slog.Logger) *Handler {
	h := &Handler{
		tasks:  tasks,
		logger: logger,
	}

	// Load SMTP config from the same env vars used by mcp-email.
	host := lookupEnv("SMTP_HOST")
	portStr := lookupEnv("SMTP_PORT")
	if host != "" && portStr != "" {
		port, err := strconv.Atoi(portStr)
		if err == nil {
			from := lookupEnv("SMTP_FROM")
			if from == "" {
				from = lookupEnv("SMTP_USER")
			}
			h.smtp = &SMTPConfig{
				Host:     host,
				Port:     port,
				User:     lookupEnv("SMTP_USER"),
				Password: lookupEnv("SMTP_PASSWORD"),
				From:     from,
			}
			logger.Info("email notifications enabled", "host", host, "port", port)
		}
	}

	return h
}

// Subscribe registers the handler on the event bus for task and pipeline
// completion events.
func (h *Handler) Subscribe(bus *events.Bus) {
	bus.Subscribe("task.completed", h.handleEvent)
	bus.Subscribe("pipeline.completed", h.handleEvent)
}

func (h *Handler) handleEvent(e events.Event) {
	taskName := e.Payload["task"]
	if taskName == "" {
		taskName = e.Payload["pipeline"]
	}

	t := h.findTask(taskName)
	if t == nil || t.Notify == nil {
		return
	}

	status := e.Payload["status"]
	if !t.Notify.ShouldNotify(status) {
		return
	}

	h.logger.Info("sending notification", "task", taskName, "status", status)

	// Send email notification.
	if len(t.Notify.Email) > 0 {
		if err := h.sendEmail(taskName, e); err != nil {
			h.logger.Error("email notification failed", "task", taskName, "error", err)
		}
	}

	// Send webhook notification.
	if t.Notify.Webhook != "" {
		if err := h.sendWebhook(t.Notify.Webhook, taskName, e); err != nil {
			h.logger.Error("webhook notification failed", "task", taskName, "error", err)
		}
	}
}

func (h *Handler) findTask(name string) *config.Task {
	for i := range h.tasks {
		if h.tasks[i].Name == name {
			return &h.tasks[i]
		}
	}
	return nil
}

func (h *Handler) sendEmail(taskName string, e events.Event) error {
	if h.smtp == nil {
		return fmt.Errorf("SMTP not configured (set SMTP_HOST and SMTP_PORT env vars)")
	}

	t := h.findTask(taskName)
	if t == nil {
		return fmt.Errorf("task not found: %s", taskName)
	}

	status := e.Payload["status"]
	output := e.Payload["output"]
	errMsg := e.Payload["error"]
	execID := e.Payload["execution_id"]

	subject := fmt.Sprintf("[%s] Task %q — %s", statusEmoji(status), taskName, status)
	htmlBody := renderEmailHTML(taskName, status, output, errMsg, execID)
	plainBody := renderEmailPlain(taskName, status, output, errMsg, execID)

	m := gomail.NewMessage()
	m.SetHeader("From", h.smtp.From)
	m.SetHeader("To", t.Notify.Email...)
	m.SetHeader("Subject", subject)
	m.SetBody("text/html", htmlBody)
	m.AddAlternative("text/plain", plainBody)

	d := gomail.NewDialer(h.smtp.Host, h.smtp.Port, h.smtp.User, h.smtp.Password)
	if err := d.DialAndSend(m); err != nil {
		return fmt.Errorf("SMTP send: %w", err)
	}

	h.logger.Info("email sent", "task", taskName, "to", t.Notify.Email)
	return nil
}

// webhookPayload is the JSON body sent to webhook URLs.
type webhookPayload struct {
	Event       string `json:"event"`
	Task        string `json:"task"`
	Status      string `json:"status"`
	ExecutionID string `json:"execution_id,omitempty"`
	Output      string `json:"output,omitempty"`
	Error       string `json:"error,omitempty"`
	Timestamp   string `json:"timestamp"`
}

func (h *Handler) sendWebhook(url, taskName string, e events.Event) error {
	payload := webhookPayload{
		Event:       e.Type,
		Task:        taskName,
		Status:      e.Payload["status"],
		ExecutionID: e.Payload["execution_id"],
		Output:      e.Payload["output"],
		Error:       e.Payload["error"],
		Timestamp:   time.Now().UTC().Format(time.RFC3339),
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal webhook payload: %w", err)
	}

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("POST %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("webhook returned %d", resp.StatusCode)
	}

	h.logger.Info("webhook sent", "task", taskName, "url", url, "status_code", resp.StatusCode)
	return nil
}

func statusEmoji(status string) string {
	switch status {
	case "completed":
		return "OK"
	case "failed":
		return "FAIL"
	case "cancelled":
		return "CANCELLED"
	default:
		return status
	}
}
