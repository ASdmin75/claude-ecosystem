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
			AllowedTools:   tp.AllowedTools,
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

	// Validate tool/permission consistency for tasks
	for _, t := range plan.Tasks {
		needsDontAsk := len(t.MCPServers) > 0 || len(t.Agents) > 0
		if needsDontAsk && t.PermissionMode != "" && t.PermissionMode != "dontAsk" {
			return nil, fmt.Errorf(
				"task %q uses MCP servers or agents but permission_mode is %q (must be \"dontAsk\" for automated tool use)",
				t.Name, t.PermissionMode,
			)
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

		if len(t.MCPServers) > 0 && len(t.AllowedTools) > 0 {
			for _, server := range t.MCPServers {
				prefix := "mcp__" + server + "__"
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

// rollback executes rollback functions in reverse order.
func (a *Applier) rollback(fns []func()) {
	for i := len(fns) - 1; i >= 0; i-- {
		fns[i]()
	}
}
