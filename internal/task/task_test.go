package task

import (
	"strings"
	"testing"

	"github.com/asdmin/claude-ecosystem/internal/config"
)

func TestBuildArgsMinimal(t *testing.T) {
	task := config.Task{Name: "test"}
	opts := RunOptions{}

	args := BuildArgs(task, opts)

	if args[0] != "-p" {
		t.Errorf("first arg should be -p, got %s", args[0])
	}
	// Should have --output-format json by default
	if !contains(args, "--output-format") || !contains(args, "json") {
		t.Errorf("should contain --output-format json, got %v", args)
	}
}

func TestBuildArgsModel(t *testing.T) {
	task := config.Task{Model: "opus"}
	args := BuildArgs(task, RunOptions{})

	if !contains(args, "--model") || !contains(args, "opus") {
		t.Errorf("should contain --model opus, got %v", args)
	}
}

func TestBuildArgsOutputFormatOverride(t *testing.T) {
	task := config.Task{OutputFormat: "text"}
	opts := RunOptions{OutputFormat: "stream-json"}

	args := BuildArgs(task, opts)

	// opts should override task config
	found := false
	for i, a := range args {
		if a == "--output-format" && i+1 < len(args) {
			if args[i+1] != "stream-json" {
				t.Errorf("output format should be stream-json, got %s", args[i+1])
			}
			found = true
		}
	}
	if !found {
		t.Error("--output-format not found")
	}
}

func TestBuildArgsMCPConfig(t *testing.T) {
	task := config.Task{}
	opts := RunOptions{MCPConfigPath: "/tmp/mcp.json"}

	args := BuildArgs(task, opts)
	if !contains(args, "--mcp-config") || !contains(args, "/tmp/mcp.json") {
		t.Errorf("should contain --mcp-config, got %v", args)
	}
}

func TestBuildArgsAgentsJSON(t *testing.T) {
	task := config.Task{}
	opts := RunOptions{AgentsJSON: `{"agent1":{}}`}

	args := BuildArgs(task, opts)
	if !contains(args, "--agents") {
		t.Errorf("should contain --agents, got %v", args)
	}
}

func TestBuildArgsAllowedTools(t *testing.T) {
	task := config.Task{AllowedTools: []string{"Read", "Write", "Agent"}}
	args := BuildArgs(task, RunOptions{})

	if !contains(args, "--allowedTools") {
		t.Errorf("should contain --allowedTools, got %v", args)
	}
	if !contains(args, "Read") || !contains(args, "Agent") {
		t.Errorf("should contain tool names, got %v", args)
	}
}

func TestBuildArgsMaxTurns(t *testing.T) {
	task := config.Task{MaxTurns: 50}
	args := BuildArgs(task, RunOptions{})

	if !contains(args, "--max-turns") || !contains(args, "50") {
		t.Errorf("should contain --max-turns 50, got %v", args)
	}
}

func TestBuildArgsMaxBudget(t *testing.T) {
	task := config.Task{MaxBudgetUSD: 1.5}
	args := BuildArgs(task, RunOptions{})

	if !contains(args, "--max-budget-usd") || !contains(args, "1.50") {
		t.Errorf("should contain --max-budget-usd 1.50, got %v", args)
	}
}

func TestBuildArgsAppendSystemPrompt(t *testing.T) {
	task := config.Task{AppendSystemPrompt: "task prompt"}
	opts := RunOptions{AppendSystemPrompt: "domain doc"}

	args := BuildArgs(task, opts)

	// Should merge both
	found := false
	for i, a := range args {
		if a == "--append-system-prompt" && i+1 < len(args) {
			if !strings.Contains(args[i+1], "task prompt") || !strings.Contains(args[i+1], "domain doc") {
				t.Errorf("should contain both prompts, got %s", args[i+1])
			}
			found = true
		}
	}
	if !found {
		t.Error("--append-system-prompt not found")
	}
}

func TestBuildArgsResume(t *testing.T) {
	task := config.Task{}
	opts := RunOptions{ResumeSessionID: "session-123"}

	args := BuildArgs(task, opts)
	if !contains(args, "--resume") || !contains(args, "session-123") {
		t.Errorf("should contain --resume session-123, got %v", args)
	}
}

func TestRenderPromptNoVars(t *testing.T) {
	prompt, err := renderPrompt("Hello world", nil)
	if err != nil {
		t.Fatal(err)
	}
	if prompt != "Hello world" {
		t.Errorf("got %q", prompt)
	}
}

func TestRenderPromptWithVars(t *testing.T) {
	prompt, err := renderPrompt("Hello {{.Name}}, date: {{.Date}}", map[string]string{
		"Name": "Alice",
		"Date": "2026-03-17",
	})
	if err != nil {
		t.Fatal(err)
	}
	if prompt != "Hello Alice, date: 2026-03-17" {
		t.Errorf("got %q", prompt)
	}
}

func TestRenderPromptNoTemplate(t *testing.T) {
	// When prompt has no {{ }}, should skip template parsing even with vars
	prompt, err := renderPrompt("No templates here", map[string]string{"key": "val"})
	if err != nil {
		t.Fatal(err)
	}
	if prompt != "No templates here" {
		t.Errorf("got %q", prompt)
	}
}

func TestParseJSONOutput(t *testing.T) {
	output := `{"result":"task output","session_id":"sess-1","model":"claude-4","cost_usd":0.05}`
	result := parseJSONOutput([]byte(output))

	if result.Output != "task output" {
		t.Errorf("output: got %q", result.Output)
	}
	if result.SessionID != "sess-1" {
		t.Errorf("session_id: got %q", result.SessionID)
	}
	if result.Model != "claude-4" {
		t.Errorf("model: got %q", result.Model)
	}
	if result.CostUSD != 0.05 {
		t.Errorf("cost_usd: got %f", result.CostUSD)
	}
}

func TestParseJSONOutputStructured(t *testing.T) {
	output := `{"structured_output":{"key":"value"},"session_id":"s1","model":"m1"}`
	result := parseJSONOutput([]byte(output))

	if !strings.Contains(result.Output, `"key":"value"`) {
		t.Errorf("structured output not preserved: %q", result.Output)
	}
}

func TestParseJSONOutputInvalid(t *testing.T) {
	result := parseJSONOutput([]byte("not json"))
	if result.Output != "not json" {
		t.Errorf("should return raw text for invalid JSON: got %q", result.Output)
	}
}

func contains(s []string, val string) bool {
	for _, v := range s {
		if v == val {
			return true
		}
	}
	return false
}
