package mcpmanager

import (
	"os/exec"

	"github.com/asdmin/claude-ecosystem/internal/config"
)

// ServerStatus describes the current state of a managed MCP server.
type ServerStatus struct {
	Name    string `json:"name"`
	Running bool   `json:"running"`
	PID     int    `json:"pid,omitempty"`
}

// managedServer wraps a config entry with runtime state.
type managedServer struct {
	config  config.MCPServerConfig
	cmd     *exec.Cmd
	running bool
}
