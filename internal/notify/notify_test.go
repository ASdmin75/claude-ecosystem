package notify

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/asdmin/claude-ecosystem/internal/config"
	"github.com/asdmin/claude-ecosystem/internal/events"
)

func TestShouldNotify(t *testing.T) {
	tests := []struct {
		trigger string
		status  string
		want    bool
	}{
		{"on_success", "completed", true},
		{"on_success", "failed", false},
		{"on_failure", "failed", true},
		{"on_failure", "completed", false},
		{"always", "completed", true},
		{"always", "failed", true},
		{"", "completed", true},  // default is "always"
		{"", "failed", true},     // default is "always"
	}

	for _, tt := range tests {
		n := &config.NotifyConfig{Trigger: tt.trigger}
		got := n.ShouldNotify(tt.status)
		if got != tt.want {
			t.Errorf("trigger=%q status=%q: got %v, want %v", tt.trigger, tt.status, got, tt.want)
		}
	}

	// nil config should never notify
	var n *config.NotifyConfig
	if n.ShouldNotify("completed") {
		t.Error("nil NotifyConfig should not notify")
	}
}

func TestWebhookNotification(t *testing.T) {
	var received webhookPayload
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		json.Unmarshal(body, &received)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	tasks := []config.Task{
		{
			Name: "test-task",
			Notify: &config.NotifyConfig{
				Webhook: server.URL,
				Trigger: "always",
			},
		},
	}

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	h := NewHandler(tasks, logger)

	bus := events.NewBus()
	h.Subscribe(bus)

	bus.Publish(events.Event{
		Type: "task.completed",
		Payload: map[string]string{
			"task":         "test-task",
			"status":       "completed",
			"output":       "hello world",
			"execution_id": "exec-123",
		},
	})
	bus.Wait()

	if received.Task != "test-task" {
		t.Errorf("webhook task = %q, want %q", received.Task, "test-task")
	}
	if received.Status != "completed" {
		t.Errorf("webhook status = %q, want %q", received.Status, "completed")
	}
	if received.Output != "hello world" {
		t.Errorf("webhook output = %q, want %q", received.Output, "hello world")
	}
	if received.ExecutionID != "exec-123" {
		t.Errorf("webhook execution_id = %q, want %q", received.ExecutionID, "exec-123")
	}
}

func TestWebhookNotRespected(t *testing.T) {
	called := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	tasks := []config.Task{
		{
			Name: "success-only",
			Notify: &config.NotifyConfig{
				Webhook: server.URL,
				Trigger: "on_success",
			},
		},
	}

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	h := NewHandler(tasks, logger)
	bus := events.NewBus()
	h.Subscribe(bus)

	bus.Publish(events.Event{
		Type: "task.completed",
		Payload: map[string]string{
			"task":   "success-only",
			"status": "failed",
			"error":  "something broke",
		},
	})
	bus.Wait()

	if called {
		t.Error("webhook should not have been called for failed status with on_success trigger")
	}
}

func TestNoNotifyConfigSkipped(t *testing.T) {
	called := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	tasks := []config.Task{
		{Name: "no-notify-task"},
	}

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	h := NewHandler(tasks, logger)
	bus := events.NewBus()
	h.Subscribe(bus)

	bus.Publish(events.Event{
		Type: "task.completed",
		Payload: map[string]string{
			"task":   "no-notify-task",
			"status": "completed",
		},
	})
	bus.Wait()

	if called {
		t.Error("webhook should not have been called for task without notify config")
	}
}

func TestEmailTemplate(t *testing.T) {
	html := renderEmailHTML("my-task", "completed", "result output", "", "exec-456")
	if html == "" {
		t.Fatal("HTML template returned empty string")
	}
	if !containsAll(html, "my-task", "completed", "result output", "exec-456") {
		t.Error("HTML template missing expected content")
	}

	plain := renderEmailPlain("my-task", "failed", "", "boom", "exec-789")
	if !containsAll(plain, "my-task", "failed", "boom", "exec-789") {
		t.Error("plain template missing expected content")
	}
}

func containsAll(s string, subs ...string) bool {
	for _, sub := range subs {
		if !contains(s, sub) {
			return false
		}
	}
	return true
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchString(s, substr)
}

func searchString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
