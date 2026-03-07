package subagent

import (
	"encoding/json"
	"fmt"
)

// SubAgent represents a Claude Code sub-agent defined as a .md file
// with YAML frontmatter in the .claude/agents/ directory.
type SubAgent struct {
	Name            string   `yaml:"name" json:"name"`
	Description     string   `yaml:"description" json:"description"`
	Tools           []string `yaml:"tools,omitempty" json:"tools,omitempty"`
	DisallowedTools []string `yaml:"disallowedTools,omitempty" json:"disallowed_tools,omitempty"`
	Model           string   `yaml:"model,omitempty" json:"model,omitempty"`
	PermissionMode  string   `yaml:"permissionMode,omitempty" json:"permission_mode,omitempty"`
	MaxTurns        int      `yaml:"maxTurns,omitempty" json:"max_turns,omitempty"`
	MCPServers      []string `yaml:"mcpServers,omitempty" json:"mcp_servers,omitempty"`
	Instructions    string   `yaml:"-" json:"instructions"` // markdown body after frontmatter
	FilePath        string   `yaml:"-" json:"file_path"`    // absolute path to .md file
}

// ToAgentsJSON converts a list of SubAgents to the JSON format expected by `claude --agents`.
// Format: {"agent-name": {"description":"...","prompt":"...","tools":[...],"model":"..."}}
func ToAgentsJSON(agents []SubAgent) (string, error) {
	result := make(map[string]interface{}, len(agents))

	for _, a := range agents {
		entry := map[string]interface{}{
			"description": a.Description,
			"prompt":      a.Instructions,
		}
		if len(a.Tools) > 0 {
			entry["tools"] = a.Tools
		}
		if a.Model != "" {
			entry["model"] = a.Model
		}
		result[a.Name] = entry
	}

	data, err := json.Marshal(result)
	if err != nil {
		return "", fmt.Errorf("marshal agents JSON: %w", err)
	}
	return string(data), nil
}
