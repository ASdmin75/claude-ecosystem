package subagent

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// ParseFile reads a .md file and returns a SubAgent.
// File format:
//
//	---
//	description: "..."
//	tools: [Read, Grep]
//	model: sonnet
//	...
//	---
//
//	Markdown instructions here...
func ParseFile(path string) (*SubAgent, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read sub-agent file %s: %w", path, err)
	}

	content := string(data)

	// The file must start with "---"
	if !strings.HasPrefix(content, "---") {
		return nil, fmt.Errorf("sub-agent file %s: missing YAML frontmatter delimiter", path)
	}

	// Find the closing "---" delimiter (skip the opening one)
	rest := content[3:]
	idx := strings.Index(rest, "\n---")
	if idx < 0 {
		return nil, fmt.Errorf("sub-agent file %s: missing closing frontmatter delimiter", path)
	}

	frontmatter := rest[:idx]
	// Body starts after the closing "---" and its newline
	body := rest[idx+4:] // len("\n---") == 4

	agent := &SubAgent{}
	if err := yaml.Unmarshal([]byte(frontmatter), agent); err != nil {
		return nil, fmt.Errorf("sub-agent file %s: parse frontmatter: %w", path, err)
	}

	agent.Instructions = strings.TrimSpace(body)

	// Derive name from filename (without .md extension)
	base := filepath.Base(path)
	agent.Name = strings.TrimSuffix(base, ".md")

	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("sub-agent file %s: resolve path: %w", path, err)
	}
	agent.FilePath = absPath

	return agent, nil
}

// frontmatterFields is used for serializing only the YAML frontmatter fields,
// excluding Name, Instructions, and FilePath which are derived/stored separately.
type frontmatterFields struct {
	Description     string   `yaml:"description,omitempty"`
	Tools           []string `yaml:"tools,omitempty"`
	DisallowedTools []string `yaml:"disallowedTools,omitempty"`
	Model           string   `yaml:"model,omitempty"`
	PermissionMode  string   `yaml:"permissionMode,omitempty"`
	MaxTurns        int      `yaml:"maxTurns,omitempty"`
	MCPServers      []string `yaml:"mcpServers,omitempty"`
}

// SerializeToMarkdown converts a SubAgent back to the .md file format with YAML frontmatter.
func SerializeToMarkdown(agent *SubAgent) ([]byte, error) {
	fm := frontmatterFields{
		Description:     agent.Description,
		Tools:           agent.Tools,
		DisallowedTools: agent.DisallowedTools,
		Model:           agent.Model,
		PermissionMode:  agent.PermissionMode,
		MaxTurns:        agent.MaxTurns,
		MCPServers:      agent.MCPServers,
	}

	yamlData, err := yaml.Marshal(&fm)
	if err != nil {
		return nil, fmt.Errorf("marshal sub-agent frontmatter: %w", err)
	}

	var buf strings.Builder
	buf.WriteString("---\n")
	buf.Write(yamlData)
	buf.WriteString("---\n\n")
	buf.WriteString(agent.Instructions)
	buf.WriteString("\n")

	return []byte(buf.String()), nil
}
