package mcpmanager

import (
	"encoding/json"
	"fmt"
	"os"
)

// mcpConfigFile is the JSON format for the --mcp-config flag.
// Format: {"mcpServers": {"name": {"command": "...", "args": [...]}}}
type mcpConfigFile struct {
	MCPServers map[string]mcpConfigEntry `json:"mcpServers"`
}

// mcpConfigEntry represents a single server in the config file.
type mcpConfigEntry struct {
	Command string            `json:"command"`
	Args    []string          `json:"args,omitempty"`
	Env     map[string]string `json:"env,omitempty"`
}

// GenerateConfigFile generates a --mcp-config JSON file for the given server names.
// The caller is responsible for cleaning up the file.
func (m *Manager) GenerateConfigFile(serverNames []string) (string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	cfg := mcpConfigFile{
		MCPServers: make(map[string]mcpConfigEntry),
	}

	for _, name := range serverNames {
		srv, ok := m.servers[name]
		if !ok {
			return "", fmt.Errorf("unknown MCP server: %s", name)
		}
		entry := mcpConfigEntry{
			Command: srv.config.Command,
			Args:    srv.config.Args,
		}
		if len(srv.config.Env) > 0 {
			entry.Env = srv.config.Env
		}
		cfg.MCPServers[name] = entry
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshaling MCP config: %w", err)
	}

	f, err := os.CreateTemp("", "mcp-*.json")
	if err != nil {
		return "", fmt.Errorf("creating temp MCP config file: %w", err)
	}
	defer f.Close()

	if _, err := f.Write(data); err != nil {
		os.Remove(f.Name())
		return "", fmt.Errorf("writing MCP config file: %w", err)
	}

	return f.Name(), nil
}
