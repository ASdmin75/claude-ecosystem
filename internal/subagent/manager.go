package subagent

import (
	"fmt"
	"os"
	"path/filepath"
)

// Manager provides CRUD operations for sub-agent .md files
// stored in user (~/.claude/agents/) and project (.claude/agents/) directories.
type Manager struct {
	userDir    string
	projectDir string
}

// NewManager creates a Manager that operates on both user and project directories.
// projectDir is typically ".claude/agents" relative to the working directory.
func NewManager(projectDir string) *Manager {
	if projectDir == "" {
		projectDir = filepath.Join(".claude", "agents")
	}
	home, _ := os.UserHomeDir()
	userDir := filepath.Join(home, ".claude", "agents")
	return &Manager{userDir: userDir, projectDir: projectDir}
}

// dirForScope returns the base directory for the given scope.
func (m *Manager) dirForScope(scope string) string {
	if scope == "project" {
		return m.projectDir
	}
	return m.userDir
}

// List returns all sub-agents found in both user and project directories.
func (m *Manager) List() ([]SubAgent, error) {
	var agents []SubAgent
	for _, dir := range []struct {
		path  string
		scope string
	}{
		{m.userDir, "user"},
		{m.projectDir, "project"},
	} {
		pattern := filepath.Join(dir.path, "*.md")
		matches, err := filepath.Glob(pattern)
		if err != nil {
			return nil, fmt.Errorf("glob sub-agent files: %w", err)
		}
		for _, path := range matches {
			agent, err := ParseFile(path)
			if err != nil {
				return nil, err
			}
			agent.Scope = dir.scope
			agents = append(agents, *agent)
		}
	}
	return agents, nil
}

// Get returns a specific sub-agent by name, checking project first then user.
func (m *Manager) Get(name string) (*SubAgent, error) {
	// Check project scope first, then user scope.
	for _, dir := range []struct {
		path  string
		scope string
	}{
		{m.projectDir, "project"},
		{m.userDir, "user"},
	} {
		p := filepath.Join(dir.path, name+".md")
		if _, err := os.Stat(p); err == nil {
			agent, err := ParseFile(p)
			if err != nil {
				return nil, err
			}
			agent.Scope = dir.scope
			return agent, nil
		}
	}
	return nil, fmt.Errorf("sub-agent %q not found", name)
}

// GetScoped returns a specific sub-agent by name from the given scope.
func (m *Manager) GetScoped(name, scope string) (*SubAgent, error) {
	dir := m.dirForScope(scope)
	p := filepath.Join(dir, name+".md")
	if _, err := os.Stat(p); os.IsNotExist(err) {
		return nil, fmt.Errorf("sub-agent %q not found in %s scope", name, scope)
	}
	agent, err := ParseFile(p)
	if err != nil {
		return nil, err
	}
	agent.Scope = scope
	return agent, nil
}

// Create creates a new sub-agent .md file in the specified scope.
// If agent.Scope is empty, defaults to "user".
func (m *Manager) Create(agent *SubAgent) error {
	if agent.Scope == "" {
		agent.Scope = "user"
	}
	dir := m.dirForScope(agent.Scope)
	path := filepath.Join(dir, agent.Name+".md")

	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("sub-agent %q already exists in %s scope", agent.Name, agent.Scope)
	}

	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create agents directory: %w", err)
	}

	data, err := SerializeToMarkdown(agent)
	if err != nil {
		return err
	}

	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write sub-agent file: %w", err)
	}

	absPath, err := filepath.Abs(path)
	if err == nil {
		agent.FilePath = absPath
	}
	return nil
}

// Update updates an existing sub-agent .md file.
// Uses agent.Scope to determine which directory to write to.
func (m *Manager) Update(agent *SubAgent) error {
	if agent.Scope == "" {
		agent.Scope = "user"
	}
	dir := m.dirForScope(agent.Scope)
	path := filepath.Join(dir, agent.Name+".md")

	if _, err := os.Stat(path); os.IsNotExist(err) {
		return fmt.Errorf("sub-agent %q not found in %s scope", agent.Name, agent.Scope)
	}

	data, err := SerializeToMarkdown(agent)
	if err != nil {
		return err
	}

	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write sub-agent file: %w", err)
	}

	absPath, err := filepath.Abs(path)
	if err == nil {
		agent.FilePath = absPath
	}
	return nil
}

// Delete removes a sub-agent .md file. Checks project scope first, then user.
func (m *Manager) Delete(name string) error {
	for _, dir := range []string{m.projectDir, m.userDir} {
		path := filepath.Join(dir, name+".md")
		if _, err := os.Stat(path); err == nil {
			if err := os.Remove(path); err != nil {
				return fmt.Errorf("delete sub-agent file: %w", err)
			}
			return nil
		}
	}
	return fmt.Errorf("sub-agent %q not found", name)
}

// DeleteScoped removes a sub-agent .md file from a specific scope.
func (m *Manager) DeleteScoped(name, scope string) error {
	dir := m.dirForScope(scope)
	path := filepath.Join(dir, name+".md")

	if _, err := os.Stat(path); os.IsNotExist(err) {
		return fmt.Errorf("sub-agent %q not found in %s scope", name, scope)
	}

	if err := os.Remove(path); err != nil {
		return fmt.Errorf("delete sub-agent file: %w", err)
	}
	return nil
}
