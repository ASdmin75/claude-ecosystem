package task

import (
	"os"

	"github.com/asdmin/claude-ecosystem/internal/config"
	"github.com/asdmin/claude-ecosystem/internal/mcpmanager"
	"github.com/asdmin/claude-ecosystem/internal/subagent"
)

// ResolveRunOptions builds RunOptions from a task's config by resolving
// agents (via subagent.Manager) and MCP servers (via mcpmanager.Manager).
func ResolveRunOptions(t config.Task, subMgr *subagent.Manager, mcpMgr *mcpmanager.Manager) (RunOptions, func(), error) {
	var opts RunOptions
	var cleanup func()

	// Resolve agents → AgentsJSON
	if len(t.Agents) > 0 && subMgr != nil {
		var agents []subagent.SubAgent
		for _, name := range t.Agents {
			a, err := subMgr.Get(name)
			if err != nil {
				return opts, nil, err
			}
			agents = append(agents, *a)
		}
		jsonStr, err := subagent.ToAgentsJSON(agents)
		if err != nil {
			return opts, nil, err
		}
		opts.AgentsJSON = jsonStr
	}

	// Resolve MCP servers → MCPConfigPath
	if len(t.MCPServers) > 0 && mcpMgr != nil {
		path, err := mcpMgr.GenerateConfigFile(t.MCPServers)
		if err != nil {
			return opts, nil, err
		}
		opts.MCPConfigPath = path
		cleanup = func() { os.Remove(path) }
	}

	return opts, cleanup, nil
}
