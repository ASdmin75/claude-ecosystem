package wizard

import (
	"strings"
	"testing"

	"github.com/asdmin/claude-ecosystem/internal/config"
)

func TestGenerateSetupDoc_Basic(t *testing.T) {
	plan := &Plan{
		Description: "Email notification system",
		Tasks: []TaskPlan{
			{Name: "send-report", Prompt: "Send report"},
			{Name: "check-inbox", Prompt: "Check inbox"},
		},
		Pipelines: []PipelinePlan{
			{Name: "email-flow", Mode: "sequential", Steps: []string{"check-inbox", "send-report"}},
		},
		Agents: []AgentPlan{
			{Name: "email-agent", Description: "Handles email"},
		},
		Domains: []DomainPlan{
			{Name: "notifications", DataDir: "data/notifications"},
		},
	}

	doc := GenerateSetupDoc(plan, &config.Config{})

	// Check all sections present
	for _, section := range []string{
		"# Setup Guide: Email notification system",
		"## What Was Created",
		"send-report",
		"check-inbox",
		"email-flow",
		"email-agent",
		"notifications",
		"## How to Run",
		"make run-task TASK=send-report",
		"make run-pipeline PIPELINE=email-flow",
	} {
		if !strings.Contains(doc, section) {
			t.Errorf("expected doc to contain %q", section)
		}
	}
}

func TestGenerateSetupDoc_MCPEnvVars(t *testing.T) {
	// Set one env var to test status detection
	t.Setenv("TELEGRAM_BOT_TOKEN", "test-token")

	plan := &Plan{
		Description: "Telegram bot",
		Tasks:       []TaskPlan{{Name: "notify", Prompt: "Send notification"}},
		MCPServers: []MCPServerPlan{
			{Name: "telegram", Command: "./bin/mcp-telegram"},
		},
	}

	doc := GenerateSetupDoc(plan, &config.Config{})

	if !strings.Contains(doc, "TELEGRAM_BOT_TOKEN") {
		t.Error("expected doc to contain TELEGRAM_BOT_TOKEN")
	}
	if !strings.Contains(doc, "TELEGRAM_CHAT_ID") {
		t.Error("expected doc to contain TELEGRAM_CHAT_ID")
	}
	// BOT_TOKEN should show as set
	if !strings.Contains(doc, "TELEGRAM_BOT_TOKEN | Bot API token from @BotFather | ✅ set") {
		t.Error("expected TELEGRAM_BOT_TOKEN to show as set")
	}
	// CHAT_ID should show as missing
	if !strings.Contains(doc, "TELEGRAM_CHAT_ID | Target chat/group ID | ❌ MISSING") {
		t.Error("expected TELEGRAM_CHAT_ID to show as MISSING")
	}
}

func TestGenerateSetupDoc_DynamicEnvRefs(t *testing.T) {
	plan := &Plan{
		Description: "CRM integration",
		Tasks:       []TaskPlan{{Name: "sync-crm", Prompt: "Sync CRM"}},
		MCPServers: []MCPServerPlan{
			{
				Name:    "crm-api",
				Command: "./bin/mcp-openapi",
				Env: map[string]string{
					"OPENAPI_SPEC_PATH":  "specs/crm.yaml",
					"OPENAPI_BASE_URL":   "${CRM_BASE_URL}",
					"OPENAPI_AUTH_TOKEN": "${CRM_TOKEN}",
				},
			},
		},
	}

	doc := GenerateSetupDoc(plan, &config.Config{})

	// Dynamic refs should be extracted
	if !strings.Contains(doc, "CRM_BASE_URL") {
		t.Error("expected doc to contain CRM_BASE_URL from ${} reference")
	}
	if !strings.Contains(doc, "CRM_TOKEN") {
		t.Error("expected doc to contain CRM_TOKEN from ${} reference")
	}
}

func TestGenerateSetupDoc_Dependencies(t *testing.T) {
	plan := &Plan{
		Description: "Audio pipeline",
		Tasks:       []TaskPlan{{Name: "transcribe", Prompt: "Transcribe audio"}},
		MCPServers: []MCPServerPlan{
			{Name: "whisper", Command: "./bin/mcp-whisper"},
		},
	}

	doc := GenerateSetupDoc(plan, &config.Config{})

	if !strings.Contains(doc, "## Dependencies") {
		t.Error("expected Dependencies section")
	}
	if !strings.Contains(doc, "whisper.cpp") {
		t.Error("expected whisper.cpp dependency")
	}
	if !strings.Contains(doc, "ffmpeg") {
		t.Error("expected ffmpeg dependency")
	}
}

func TestGenerateSetupDoc_SetupNotes(t *testing.T) {
	plan := &Plan{
		Description: "Test plan",
		Tasks:       []TaskPlan{{Name: "test-task", Prompt: "Test"}},
		SetupNotes:  "Create a Gmail app password at https://myaccount.google.com/apppasswords",
	}

	doc := GenerateSetupDoc(plan, &config.Config{})

	if !strings.Contains(doc, "## Notes") {
		t.Error("expected Notes section")
	}
	if !strings.Contains(doc, "Gmail app password") {
		t.Error("expected setup notes content")
	}
}

func TestGenerateSetupDoc_NoNotesSection(t *testing.T) {
	plan := &Plan{
		Description: "Simple plan",
		Tasks:       []TaskPlan{{Name: "simple", Prompt: "Do thing"}},
	}

	doc := GenerateSetupDoc(plan, &config.Config{})

	if strings.Contains(doc, "## Notes") {
		t.Error("expected no Notes section when SetupNotes is empty")
	}
}

func TestExtractEnvRefs(t *testing.T) {
	env := map[string]string{
		"OPENAPI_BASE_URL":   "${CRM_BASE_URL}",
		"OPENAPI_AUTH_TOKEN": "${CRM_TOKEN}",
		"OPENAPI_SPEC_PATH":  "specs/crm.yaml", // no reference
	}

	refs := extractEnvRefs(env)

	refSet := make(map[string]bool)
	for _, r := range refs {
		refSet[r] = true
	}

	if !refSet["CRM_BASE_URL"] {
		t.Error("expected CRM_BASE_URL in refs")
	}
	if !refSet["CRM_TOKEN"] {
		t.Error("expected CRM_TOKEN in refs")
	}
	if len(refs) != 2 {
		t.Errorf("expected 2 refs, got %d", len(refs))
	}
}

func TestBinaryName(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"./bin/mcp-openapi", "mcp-openapi"},
		{"/usr/local/bin/mcp-email", "mcp-email"},
		{"mcp-telegram", "mcp-telegram"},
	}
	for _, tt := range tests {
		got := binaryName(tt.input)
		if got != tt.want {
			t.Errorf("binaryName(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestResolveSetupPath(t *testing.T) {
	// With domain
	plan := &Plan{
		Domains: []DomainPlan{{Name: "test", DataDir: "data/test"}},
		Tasks:   []TaskPlan{{Name: "t", WorkDir: "/work"}},
	}
	if got := resolveSetupPath(plan); got != "data/test/SETUP.md" {
		t.Errorf("expected data/test/SETUP.md, got %s", got)
	}

	// Without domain, with work_dir
	plan2 := &Plan{
		Tasks: []TaskPlan{{Name: "t", WorkDir: "/work"}},
	}
	if got := resolveSetupPath(plan2); got != "/work/SETUP.md" {
		t.Errorf("expected /work/SETUP.md, got %s", got)
	}

	// No domain, no work_dir
	plan3 := &Plan{
		Tasks: []TaskPlan{{Name: "t"}},
	}
	if got := resolveSetupPath(plan3); got != "SETUP.md" {
		t.Errorf("expected SETUP.md, got %s", got)
	}
}

func TestResolveAllSetupPaths_MultipleDomains(t *testing.T) {
	plan := &Plan{
		Domains: []DomainPlan{
			{Name: "a", DataDir: "data/a"},
			{Name: "b", DataDir: "data/b"},
		},
	}
	paths := resolveAllSetupPaths(plan)
	if len(paths) != 2 {
		t.Fatalf("expected 2 paths, got %d", len(paths))
	}
	if paths[0] != "data/a/SETUP.md" || paths[1] != "data/b/SETUP.md" {
		t.Errorf("unexpected paths: %v", paths)
	}
}
