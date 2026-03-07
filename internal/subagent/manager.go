package subagent

import (
	"fmt"
	"os"
	"path/filepath"
)

// Manager provides CRUD operations for sub-agent .md files
// stored in a base directory (typically .claude/agents/).
type Manager struct {
	baseDir string
}

// NewManager creates a Manager that operates on the given base directory.
// If baseDir is empty, it defaults to ".claude/agents" relative to the
// current working directory.
func NewManager(baseDir string) *Manager {
	if baseDir == "" {
		baseDir = filepath.Join(".claude", "agents")
	}
	return &Manager{baseDir: baseDir}
}

// List returns all sub-agents found in the agents directory.
func (m *Manager) List() ([]SubAgent, error) {
	pattern := filepath.Join(m.baseDir, "*.md")
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return nil, fmt.Errorf("glob sub-agent files: %w", err)
	}

	agents := make([]SubAgent, 0, len(matches))
	for _, path := range matches {
		agent, err := ParseFile(path)
		if err != nil {
			return nil, err
		}
		agents = append(agents, *agent)
	}
	return agents, nil
}

// Get returns a specific sub-agent by name.
func (m *Manager) Get(name string) (*SubAgent, error) {
	path := filepath.Join(m.baseDir, name+".md")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil, fmt.Errorf("sub-agent %q not found", name)
	}
	return ParseFile(path)
}

// Create creates a new sub-agent .md file. Returns error if already exists.
func (m *Manager) Create(agent *SubAgent) error {
	path := filepath.Join(m.baseDir, agent.Name+".md")

	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("sub-agent %q already exists", agent.Name)
	}

	// Ensure the directory exists.
	if err := os.MkdirAll(m.baseDir, 0o755); err != nil {
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

// Update updates an existing sub-agent .md file. Returns error if not found.
func (m *Manager) Update(agent *SubAgent) error {
	path := filepath.Join(m.baseDir, agent.Name+".md")

	if _, err := os.Stat(path); os.IsNotExist(err) {
		return fmt.Errorf("sub-agent %q not found", agent.Name)
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

// Delete removes a sub-agent .md file. Returns error if not found.
func (m *Manager) Delete(name string) error {
	path := filepath.Join(m.baseDir, name+".md")

	if _, err := os.Stat(path); os.IsNotExist(err) {
		return fmt.Errorf("sub-agent %q not found", name)
	}

	if err := os.Remove(path); err != nil {
		return fmt.Errorf("delete sub-agent file: %w", err)
	}
	return nil
}
