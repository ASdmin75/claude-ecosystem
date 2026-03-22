package subagent

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test-agent.md")

	content := `---
description: "A test agent"
tools:
  - Read
  - Grep
model: sonnet
maxTurns: 10
---

You are a helpful test agent.
Do things.`

	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	agent, err := ParseFile(path)
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}

	if agent.Name != "test-agent" {
		t.Errorf("name: got %q, want %q", agent.Name, "test-agent")
	}
	if agent.Description != "A test agent" {
		t.Errorf("description: got %q", agent.Description)
	}
	if len(agent.Tools) != 2 || agent.Tools[0] != "Read" {
		t.Errorf("tools: got %v", agent.Tools)
	}
	if agent.Model != "sonnet" {
		t.Errorf("model: got %q", agent.Model)
	}
	if agent.MaxTurns != 10 {
		t.Errorf("maxTurns: got %d", agent.MaxTurns)
	}
	if !strings.Contains(agent.Instructions, "helpful test agent") {
		t.Errorf("instructions: got %q", agent.Instructions)
	}
}

func TestParseFileMissingFrontmatter(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.md")
	os.WriteFile(path, []byte("no frontmatter"), 0o644)

	_, err := ParseFile(path)
	if err == nil {
		t.Fatal("expected error for missing frontmatter")
	}
}

func TestParseFileMissingClosingDelimiter(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.md")
	os.WriteFile(path, []byte("---\ndescription: test\nno closing"), 0o644)

	_, err := ParseFile(path)
	if err == nil {
		t.Fatal("expected error for missing closing delimiter")
	}
}

func TestSerializeToMarkdown(t *testing.T) {
	agent := &SubAgent{
		Name:        "my-agent",
		Description: "A serialized agent",
		Tools:       []string{"Read", "Write"},
		Model:       "opus",
		Instructions: "Do something useful.",
	}

	data, err := SerializeToMarkdown(agent)
	if err != nil {
		t.Fatalf("SerializeToMarkdown: %v", err)
	}

	s := string(data)
	if !strings.HasPrefix(s, "---\n") {
		t.Error("should start with ---")
	}
	if !strings.Contains(s, "description: A serialized agent") {
		t.Error("should contain description")
	}
	if !strings.Contains(s, "Do something useful.") {
		t.Error("should contain instructions")
	}
}

func TestRoundTrip(t *testing.T) {
	dir := t.TempDir()
	original := &SubAgent{
		Name:           "roundtrip",
		Description:    "Roundtrip test",
		Tools:          []string{"Read"},
		Model:          "haiku",
		PermissionMode: "dontAsk",
		MaxTurns:       5,
		Instructions:   "Test instructions.",
	}

	data, err := SerializeToMarkdown(original)
	if err != nil {
		t.Fatal(err)
	}

	path := filepath.Join(dir, "roundtrip.md")
	os.WriteFile(path, data, 0o644)

	parsed, err := ParseFile(path)
	if err != nil {
		t.Fatal(err)
	}

	if parsed.Description != original.Description {
		t.Errorf("description mismatch: %q vs %q", parsed.Description, original.Description)
	}
	if parsed.Model != original.Model {
		t.Errorf("model mismatch: %q vs %q", parsed.Model, original.Model)
	}
	if parsed.MaxTurns != original.MaxTurns {
		t.Errorf("maxTurns mismatch: %d vs %d", parsed.MaxTurns, original.MaxTurns)
	}
	if parsed.Instructions != original.Instructions {
		t.Errorf("instructions mismatch")
	}
}

func TestToAgentsJSON(t *testing.T) {
	agents := []SubAgent{
		{
			Name:         "agent-a",
			Description:  "First agent",
			Tools:        []string{"Read"},
			Model:        "sonnet",
			Instructions: "Do A.",
		},
		{
			Name:         "agent-b",
			Description:  "Second agent",
			Instructions: "Do B.",
		},
	}

	jsonStr, err := ToAgentsJSON(agents)
	if err != nil {
		t.Fatalf("ToAgentsJSON: %v", err)
	}

	var result map[string]map[string]interface{}
	if err := json.Unmarshal([]byte(jsonStr), &result); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	if result["agent-a"]["description"] != "First agent" {
		t.Error("agent-a description mismatch")
	}
	if result["agent-a"]["prompt"] != "Do A." {
		t.Error("agent-a prompt mismatch")
	}
	if result["agent-a"]["model"] != "sonnet" {
		t.Error("agent-a model mismatch")
	}

	// agent-b should not have "model" or "tools" keys
	if _, ok := result["agent-b"]["model"]; ok {
		t.Error("agent-b should not have model")
	}
	if _, ok := result["agent-b"]["tools"]; ok {
		t.Error("agent-b should not have tools")
	}
}

func TestManagerCRUD(t *testing.T) {
	dir := t.TempDir()
	m := &Manager{
		userDir:    filepath.Join(dir, "user"),
		projectDir: filepath.Join(dir, "project"),
	}

	agent := &SubAgent{
		Name:         "test-crud",
		Description:  "CRUD test",
		Instructions: "Test.",
		Scope:        "project",
	}

	// Create
	if err := m.Create(agent); err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Get
	got, err := m.Get("test-crud")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Description != "CRUD test" {
		t.Errorf("got description %q", got.Description)
	}

	// Update
	agent.Description = "Updated"
	if err := m.Update(agent); err != nil {
		t.Fatalf("Update: %v", err)
	}
	got, _ = m.Get("test-crud")
	if got.Description != "Updated" {
		t.Errorf("after update: got description %q", got.Description)
	}

	// List
	list, err := m.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1 agent, got %d", len(list))
	}

	// Delete
	if err := m.Delete("test-crud"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	_, err = m.Get("test-crud")
	if err == nil {
		t.Fatal("expected error after delete")
	}
}

func TestCreateFromBytes(t *testing.T) {
	dir := t.TempDir()
	m := &Manager{
		userDir:    filepath.Join(dir, "user"),
		projectDir: filepath.Join(dir, "project"),
	}

	content := []byte("---\ndescription: restored agent\n---\n\nInstructions here.\n")
	if err := m.CreateFromBytes("restored-agent", content); err != nil {
		t.Fatalf("CreateFromBytes: %v", err)
	}

	// Verify file exists.
	path := filepath.Join(dir, "project", "restored-agent.md")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read file: %v", err)
	}
	if string(data) != string(content) {
		t.Errorf("content mismatch: got %q", string(data))
	}

	// Verify we can Get it.
	got, err := m.Get("restored-agent")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Description != "restored agent" {
		t.Errorf("description: got %q", got.Description)
	}
}

func TestCreateFromBytes_AlreadyExists(t *testing.T) {
	dir := t.TempDir()
	m := &Manager{
		userDir:    filepath.Join(dir, "user"),
		projectDir: filepath.Join(dir, "project"),
	}

	content := []byte("---\ndescription: test\n---\n\nTest.\n")
	if err := m.CreateFromBytes("dup", content); err != nil {
		t.Fatalf("first CreateFromBytes: %v", err)
	}
	if err := m.CreateFromBytes("dup", content); err == nil {
		t.Fatal("expected error on duplicate CreateFromBytes")
	}
}

func TestGetFilePath(t *testing.T) {
	dir := t.TempDir()
	m := &Manager{
		userDir:    filepath.Join(dir, "user"),
		projectDir: filepath.Join(dir, "project"),
	}

	agent := &SubAgent{
		Name:         "filepath-test",
		Description:  "Test",
		Instructions: "Test.",
		Scope:        "project",
	}
	if err := m.Create(agent); err != nil {
		t.Fatalf("Create: %v", err)
	}

	fp, err := m.GetFilePath("filepath-test")
	if err != nil {
		t.Fatalf("GetFilePath: %v", err)
	}
	if !strings.HasSuffix(fp, "filepath-test.md") {
		t.Errorf("expected path ending in filepath-test.md, got %q", fp)
	}
}

func TestGetFilePath_NotFound(t *testing.T) {
	dir := t.TempDir()
	m := &Manager{
		userDir:    filepath.Join(dir, "user"),
		projectDir: filepath.Join(dir, "project"),
	}

	_, err := m.GetFilePath("nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent agent")
	}
}
