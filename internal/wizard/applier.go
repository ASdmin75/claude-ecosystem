package wizard

import (
	"fmt"

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
	if err := a.validate(plan); err != nil {
		return nil, fmt.Errorf("validation failed: %w", err)
	}

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

	return result, nil
}

// validate checks for duplicate names and valid references.
func (a *Applier) validate(plan *Plan) error {
	// Check duplicate task names
	taskNames := make(map[string]bool)
	for _, t := range a.cfg.Tasks {
		taskNames[t.Name] = true
	}
	for _, t := range plan.Tasks {
		if taskNames[t.Name] {
			return fmt.Errorf("task %q already exists", t.Name)
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
			return fmt.Errorf("pipeline %q already exists", p.Name)
		}
		pipelineNames[p.Name] = true
	}

	// Check duplicate domain names
	for _, d := range plan.Domains {
		if _, exists := a.cfg.Domains[d.Name]; exists {
			return fmt.Errorf("domain %q already exists", d.Name)
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
					return fmt.Errorf("task %q references unknown agent %q", t.Name, agentName)
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
			return fmt.Errorf("MCP server %q already exists", mcp.Name)
		}
		if mcp.Command == "" {
			return fmt.Errorf("MCP server %q has empty command", mcp.Name)
		}
		mcpNames[mcp.Name] = true
	}

	// Validate MCP server references in tasks (including servers from this plan)
	for _, t := range plan.Tasks {
		for _, m := range t.MCPServers {
			if !mcpNames[m] {
				return fmt.Errorf("task %q references unknown MCP server %q", t.Name, m)
			}
		}
	}

	// Validate pipeline step references
	for _, p := range plan.Pipelines {
		for _, step := range p.Steps {
			if !taskNames[step] {
				return fmt.Errorf("pipeline %q references unknown task %q", p.Name, step)
			}
		}
	}

	return nil
}

// rollback executes rollback functions in reverse order.
func (a *Applier) rollback(fns []func()) {
	for i := len(fns) - 1; i >= 0; i-- {
		fns[i]()
	}
}
