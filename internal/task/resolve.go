package task

import (
	"os"

	"github.com/asdmin/claude-ecosystem/internal/config"
	"github.com/asdmin/claude-ecosystem/internal/domain"
	"github.com/asdmin/claude-ecosystem/internal/mcpmanager"
	"github.com/asdmin/claude-ecosystem/internal/subagent"
)

// ResolveRunOptions builds RunOptions from a task's config by resolving
// agents (via subagent.Manager), MCP servers (via mcpmanager.Manager),
// and domain context (via domain.Manager).
func ResolveRunOptions(t config.Task, subMgr *subagent.Manager, mcpMgr *mcpmanager.Manager, domainMgr *domain.Manager) (RunOptions, func(), error) {
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

	// Resolve domain env vars for MCP servers
	var domainEnv map[string]string
	if t.Domain != "" && domainMgr != nil {
		domainEnv = domainMgr.DomainEnvVars(t.Domain)

		// Inject domain doc content into append_system_prompt
		docContent, err := domainMgr.DomainDocContent(t.Domain)
		if err != nil {
			return opts, nil, err
		}
		if docContent != "" {
			opts.AppendSystemPrompt = docContent
		}
	}

	// Resolve MCP servers → MCPConfigPath
	if len(t.MCPServers) > 0 && mcpMgr != nil {
		path, err := mcpMgr.GenerateConfigFileWithEnv(t.MCPServers, domainEnv)
		if err != nil {
			return opts, nil, err
		}
		opts.MCPConfigPath = path
		cleanup = func() { os.Remove(path) }
	}

	return opts, cleanup, nil
}
