package wizard

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/asdmin/claude-ecosystem/internal/config"
	"github.com/asdmin/claude-ecosystem/internal/domain"
	"github.com/asdmin/claude-ecosystem/internal/subagent"
)

// mcpBaseTools defines tools that are always needed when a task uses a given
// MCP server. Models (especially smaller ones like Haiku) often call these
// "discovery" tools first to understand the available schema/structure.
// Without them in allowed_tools the call is denied and the model loops
// until max_turns.
var mcpBaseTools = map[string][]string{
	"database":   {"mcp__database__list_tables", "mcp__database__describe_table"},
	"filesystem": {"mcp__filesystem__list_directory"},
	"excel":      {"mcp__excel__read_spreadsheet"},
}

// isOpenAPIServer checks whether a named MCP server is openapi-based by
// inspecting existing config and planned servers for "mcp-openapi" in the command.
func isOpenAPIServer(name string, cfg *config.Config, plan *Plan) bool {
	for _, m := range cfg.MCPServers {
		if m.Name == name && strings.Contains(m.Command, "mcp-openapi") {
			return true
		}
	}
	for _, m := range plan.MCPServers {
		if m.Name == name && strings.Contains(m.Command, "mcp-openapi") {
			return true
		}
	}
	return false
}

// Applier creates all entities from a plan in dependency order.
type Applier struct {
	cfg         *config.Config
	subagentMgr *subagent.Manager
	domainMgr   *domain.Manager
}

// NewApplier creates a new Applier.
func NewApplier(cfg *config.Config, subagentMgr *subagent.Manager, domainMgr *domain.Manager) *Applier {
	return &Applier{cfg: cfg, subagentMgr: subagentMgr, domainMgr: domainMgr}
}

// Apply creates all entities from the plan in strict order:
// domains → agents → tasks → pipelines → save config.
// On failure, it executes rollbacks in reverse order.
func (a *Applier) Apply(plan *Plan) (*ApplyResult, error) {
	result := &ApplyResult{}
	var rollbacks []func()

	// Validate before mutating
	warnings, err := a.validate(plan)
	if err != nil {
		return nil, fmt.Errorf("validation failed: %w", err)
	}
	result.Warnings = warnings

	// 1. MCP Servers (must come first so tasks can reference them)
	for _, mp := range plan.MCPServers {
		mcp := config.MCPServerConfig{
			Name:    mp.Name,
			Command: mp.Command,
			Args:    mp.Args,
			Env:     mp.Env,
		}
		a.cfg.MCPServers = append(a.cfg.MCPServers, mcp)
		result.MCPServersCreated = append(result.MCPServersCreated, mp.Name)
		mcpName := mp.Name
		rollbacks = append(rollbacks, func() {
			for i := len(a.cfg.MCPServers) - 1; i >= 0; i-- {
				if a.cfg.MCPServers[i].Name == mcpName {
					a.cfg.MCPServers = append(a.cfg.MCPServers[:i], a.cfg.MCPServers[i+1:]...)
					break
				}
			}
		})
	}

	// 2. Domains
	for _, dp := range plan.Domains {
		d := config.Domain{
			Name:        dp.Name,
			Description: dp.Description,
			DataDir:     dp.DataDir,
			DB:          dp.DB,
			Schema:      dp.Schema,
			DomainDoc:   dp.DomainDoc,
			Tasks:       dp.Tasks,
			Pipelines:   dp.Pipelines,
			Agents:      dp.Agents,
			MCPServers:  dp.MCPServers,
		}
		if err := a.domainMgr.AddDomain(dp.Name, d); err != nil {
			a.rollback(rollbacks)
			return nil, fmt.Errorf("creating domain %s: %w", dp.Name, err)
		}
		if a.cfg.Domains == nil {
			a.cfg.Domains = make(map[string]config.Domain)
		}
		a.cfg.Domains[dp.Name] = d
		result.DomainsCreated = append(result.DomainsCreated, dp.Name)
		name := dp.Name
		rollbacks = append(rollbacks, func() {
			delete(a.cfg.Domains, name)
		})
	}

	// 3. Agents
	for _, ap := range plan.Agents {
		agent := &subagent.SubAgent{
			Name:           ap.Name,
			Description:    ap.Description,
			Tools:          ap.Tools,
			Model:          ap.Model,
			PermissionMode: ap.PermissionMode,
			Instructions:   ap.Instructions,
			Scope:          ap.Scope,
		}
		if agent.Scope == "" {
			agent.Scope = "project"
		}
		if err := a.subagentMgr.Create(agent); err != nil {
			a.rollback(rollbacks)
			return nil, fmt.Errorf("creating agent %s: %w", ap.Name, err)
		}
		result.AgentsCreated = append(result.AgentsCreated, ap.Name)
		agentName := ap.Name
		rollbacks = append(rollbacks, func() {
			_ = a.subagentMgr.Delete(agentName)
		})
	}

	// 4. Tasks
	for _, tp := range plan.Tasks {
		allowedTools := ensureBaseTools(tp.MCPServers, tp.AllowedTools)
		t := config.Task{
			Name:           tp.Name,
			Prompt:         tp.Prompt,
			WorkDir:        tp.WorkDir,
			Schedule:       tp.Schedule,
			Tags:           tp.Tags,
			Model:          tp.Model,
			Timeout:        tp.Timeout,
			Agents:         tp.Agents,
			MCPServers:     tp.MCPServers,
			AllowedTools:   allowedTools,
			MaxTurns:       tp.MaxTurns,
			MaxBudgetUSD:   tp.MaxBudgetUSD,
			PermissionMode: tp.PermissionMode,
			Domain:         tp.Domain,
		}
		a.cfg.Tasks = append(a.cfg.Tasks, t)
		result.TasksCreated = append(result.TasksCreated, tp.Name)
		rollbacks = append(rollbacks, func() {
			// Remove last added task
			for i := len(a.cfg.Tasks) - 1; i >= 0; i-- {
				if a.cfg.Tasks[i].Name == t.Name {
					a.cfg.Tasks = append(a.cfg.Tasks[:i], a.cfg.Tasks[i+1:]...)
					break
				}
			}
		})
	}

	// 5. Pipelines
	for _, pp := range plan.Pipelines {
		steps := make([]config.PipelineStep, len(pp.Steps))
		for i, s := range pp.Steps {
			steps[i] = config.PipelineStep{Task: s}
		}
		p := config.Pipeline{
			Name:          pp.Name,
			Mode:          pp.Mode,
			Steps:         steps,
			MaxIterations: pp.MaxIterations,
			StopSignal:    pp.StopSignal,
			SessionChain:  pp.SessionChain,
		}
		if p.Mode == "" {
			p.Mode = "sequential"
		}
		a.cfg.Pipelines = append(a.cfg.Pipelines, p)
		result.PipelinesCreated = append(result.PipelinesCreated, pp.Name)
		rollbacks = append(rollbacks, func() {
			for i := len(a.cfg.Pipelines) - 1; i >= 0; i-- {
				if a.cfg.Pipelines[i].Name == p.Name {
					a.cfg.Pipelines = append(a.cfg.Pipelines[:i], a.cfg.Pipelines[i+1:]...)
					break
				}
			}
		})
	}

	// 6. Save config
	if err := a.cfg.Save(); err != nil {
		a.rollback(rollbacks)
		return nil, fmt.Errorf("saving config: %w", err)
	}

	// 7. Generate SETUP.md (non-fatal)
	a.generateSetupDoc(plan, result)

	return result, nil
}

// ValidateOnly checks the plan without applying it. Returns warnings and validation errors.
func (a *Applier) ValidateOnly(plan *Plan) ([]string, error) {
	return a.validate(plan)
}

// validate checks for duplicate names, valid references, and tool/permission consistency.
// Returns warnings (non-fatal issues) and an error (fatal issues that block apply).
func (a *Applier) validate(plan *Plan) ([]string, error) {
	var warnings []string
	// Check duplicate task names
	taskNames := make(map[string]bool)
	for _, t := range a.cfg.Tasks {
		taskNames[t.Name] = true
	}
	for _, t := range plan.Tasks {
		if taskNames[t.Name] {
			return nil, fmt.Errorf("task %q already exists", t.Name)
		}
		taskNames[t.Name] = true
	}

	// Check duplicate pipeline names
	pipelineNames := make(map[string]bool)
	for _, p := range a.cfg.Pipelines {
		pipelineNames[p.Name] = true
	}
	for _, p := range plan.Pipelines {
		if pipelineNames[p.Name] {
			return nil, fmt.Errorf("pipeline %q already exists", p.Name)
		}
		pipelineNames[p.Name] = true
	}

	// Check duplicate domain names
	for _, d := range plan.Domains {
		if _, exists := a.cfg.Domains[d.Name]; exists {
			return nil, fmt.Errorf("domain %q already exists", d.Name)
		}
	}

	// Collect planned agent names
	agentNames := make(map[string]bool)
	for _, ag := range plan.Agents {
		agentNames[ag.Name] = true
	}

	// Validate agent references in tasks
	for _, t := range plan.Tasks {
		for _, agentName := range t.Agents {
			if !agentNames[agentName] {
				// Check if agent exists already
				if _, err := a.subagentMgr.Get(agentName); err != nil {
					return nil, fmt.Errorf("task %q references unknown agent %q", t.Name, agentName)
				}
			}
		}
	}

	// Check duplicate MCP server names
	mcpNames := make(map[string]bool)
	for _, mcp := range a.cfg.MCPServers {
		mcpNames[mcp.Name] = true
	}
	for _, mcp := range plan.MCPServers {
		if mcpNames[mcp.Name] {
			return nil, fmt.Errorf("MCP server %q already exists", mcp.Name)
		}
		if mcp.Command == "" {
			return nil, fmt.Errorf("MCP server %q has empty command", mcp.Name)
		}
		mcpNames[mcp.Name] = true
	}

	// Validate MCP server references in tasks (including servers from this plan)
	for _, t := range plan.Tasks {
		for _, m := range t.MCPServers {
			if !mcpNames[m] {
				return nil, fmt.Errorf("task %q references unknown MCP server %q", t.Name, m)
			}
		}
	}

	// Validate pipeline step references
	for _, p := range plan.Pipelines {
		for _, step := range p.Steps {
			if !taskNames[step] {
				return nil, fmt.Errorf("pipeline %q references unknown task %q", p.Name, step)
			}
		}
	}

	// Validate stop_signal not present in non-final step prompts
	planTaskPrompts := make(map[string]string)
	for _, t := range plan.Tasks {
		planTaskPrompts[t.Name] = t.Prompt
	}
	for _, p := range plan.Pipelines {
		if p.StopSignal == "" || len(p.Steps) < 2 {
			continue
		}
		for i, step := range p.Steps {
			if i == len(p.Steps)-1 {
				break // last step is allowed to contain the stop signal
			}
			prompt := planTaskPrompts[step]
			if prompt != "" && strings.Contains(prompt, p.StopSignal) {
				return nil, fmt.Errorf(
					"pipeline %q: non-final step %q (step %d of %d) prompt contains stop_signal %q — "+
						"this will cause the pipeline to stop before reaching later steps; "+
						"remove the signal from this step's prompt or move it to the last step only",
					p.Name, step, i+1, len(p.Steps), p.StopSignal,
				)
			}
		}
	}

	// Validate and auto-fix tool/permission consistency for tasks
	for i := range plan.Tasks {
		t := &plan.Tasks[i]
		needsDontAsk := len(t.MCPServers) > 0 || len(t.Agents) > 0

		if needsDontAsk && t.PermissionMode != "" && t.PermissionMode != "dontAsk" {
			return nil, fmt.Errorf(
				"task %q uses MCP servers or agents but permission_mode is %q (must be \"dontAsk\" for automated tool use)",
				t.Name, t.PermissionMode,
			)
		}

		// Auto-set permission_mode to "dontAsk" when task uses MCP tools or agents
		if needsDontAsk && t.PermissionMode == "" {
			t.PermissionMode = "dontAsk"
			warnings = append(warnings, fmt.Sprintf(
				"task %q: auto-set permission_mode to \"dontAsk\" (required for MCP tools/agents)",
				t.Name,
			))
		}

		if len(t.Agents) > 0 && len(t.AllowedTools) > 0 {
			hasAgent := false
			for _, tool := range t.AllowedTools {
				if tool == "Agent" {
					hasAgent = true
					break
				}
			}
			if !hasAgent {
				return nil, fmt.Errorf(
					"task %q has agents %v but \"Agent\" is not in allowed_tools — Claude won't be able to delegate to agents",
					t.Name, t.Agents,
				)
			}
		}

		if len(t.MCPServers) > 0 {
			for _, server := range t.MCPServers {
				prefix := "mcp__" + server + "__"

				if len(t.AllowedTools) > 0 {
					found := false
					for _, tool := range t.AllowedTools {
						if strings.HasPrefix(tool, prefix) {
							found = true
							break
						}
					}
					if !found {
						warnings = append(warnings, fmt.Sprintf(
							"task %q references MCP server %q but allowed_tools has no tools with prefix %q",
							t.Name, server, prefix,
						))
					}
				}

				// Require explicit allowed_tools for openapi servers (external API safety)
				if isOpenAPIServer(server, a.cfg, plan) && len(t.AllowedTools) == 0 {
					warnings = append(warnings, fmt.Sprintf(
						"task %q uses openapi server %q without allowed_tools — "+
							"Claude will have unrestricted access to ALL API endpoints including destructive ones; "+
							"specify explicit allowed_tools to whitelist only needed endpoints",
						t.Name, server,
					))
				}
			}
		}
	}

	// Validate and auto-fix permission consistency for sub-agents
	for i := range plan.Agents {
		ag := &plan.Agents[i]
		hasMCPTools := false
		for _, tool := range ag.Tools {
			if strings.HasPrefix(tool, "mcp__") {
				hasMCPTools = true
				break
			}
		}

		if hasMCPTools && ag.PermissionMode != "" && ag.PermissionMode != "dontAsk" {
			return nil, fmt.Errorf(
				"agent %q uses MCP tools but permission_mode is %q (must be \"dontAsk\" for automated tool use)",
				ag.Name, ag.PermissionMode,
			)
		}

		// Auto-set permission_mode to "dontAsk" when agent uses MCP tools
		if hasMCPTools && ag.PermissionMode == "" {
			ag.PermissionMode = "dontAsk"
			warnings = append(warnings, fmt.Sprintf(
				"agent %q: auto-set permission_mode to \"dontAsk\" (required for MCP tools)",
				ag.Name,
			))
		}
	}

	return warnings, nil
}

// generateSetupDoc creates SETUP.md files from the plan. Non-fatal on failure.
func (a *Applier) generateSetupDoc(plan *Plan, result *ApplyResult) {
	content := GenerateSetupDoc(plan, a.cfg)
	paths := resolveAllSetupPaths(plan)

	for _, p := range paths {
		dir := filepath.Dir(p)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			result.Warnings = append(result.Warnings, fmt.Sprintf("failed to create directory for SETUP.md: %v", err))
			continue
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			result.Warnings = append(result.Warnings, fmt.Sprintf("failed to write %s: %v", p, err))
			continue
		}
		slog.Info("wizard: wrote SETUP.md", "path", p)
		if result.SetupDocPath == "" {
			result.SetupDocPath = p
		}
	}
}

// ensureBaseTools injects mandatory "discovery" tools for each MCP server
// that the task uses. If allowed_tools is empty (allow-all), nothing is added.
func ensureBaseTools(mcpServers, allowedTools []string) []string {
	if len(mcpServers) == 0 || len(allowedTools) == 0 {
		return allowedTools
	}
	existing := make(map[string]bool, len(allowedTools))
	for _, t := range allowedTools {
		existing[t] = true
	}
	result := append([]string(nil), allowedTools...)
	for _, server := range mcpServers {
		base, ok := mcpBaseTools[server]
		if !ok {
			continue
		}
		// Only inject if the task already has at least one tool from this server
		prefix := "mcp__" + server + "__"
		hasServerTool := false
		for _, t := range allowedTools {
			if strings.HasPrefix(t, prefix) {
				hasServerTool = true
				break
			}
		}
		if !hasServerTool {
			continue
		}
		for _, bt := range base {
			if !existing[bt] {
				result = append(result, bt)
				existing[bt] = true
			}
		}
	}
	return result
}

// rollback executes rollback functions in reverse order.
func (a *Applier) rollback(fns []func()) {
	for i := len(fns) - 1; i >= 0; i-- {
		fns[i]()
	}
}
