package wizard

// GenerateRequest is the input for wizard plan generation.
type GenerateRequest struct {
	Description string `json:"description"`
	WorkDir     string `json:"work_dir,omitempty"`
}

// Plan holds the full wizard-generated configuration plan.
type Plan struct {
	ID          string         `json:"id"`
	Description string         `json:"description"`
	Summary     string         `json:"summary"`
	MCPServers  []MCPServerPlan `json:"mcp_servers,omitempty"`
	Domains     []DomainPlan    `json:"domains,omitempty"`
	Agents      []AgentPlan     `json:"agents,omitempty"`
	Tasks       []TaskPlan      `json:"tasks,omitempty"`
	Pipelines   []PipelinePlan  `json:"pipelines,omitempty"`
	Status      string         `json:"status"` // "draft", "applied", "discarded"
}

// DomainPlan describes a domain to be created.
type DomainPlan struct {
	Name        string   `json:"name"`
	Description string   `json:"description,omitempty"`
	DataDir     string   `json:"data_dir"`
	DB          string   `json:"db,omitempty"`
	Schema      string   `json:"schema,omitempty"`
	DomainDoc   string   `json:"domain_doc,omitempty"`
	Tasks       []string `json:"tasks,omitempty"`
	Pipelines   []string `json:"pipelines,omitempty"`
	Agents      []string `json:"agents,omitempty"`
	MCPServers  []string `json:"mcp_servers,omitempty"`
}

// AgentPlan describes a sub-agent to be created.
type AgentPlan struct {
	Name           string   `json:"name"`
	Description    string   `json:"description"`
	Tools          []string `json:"tools,omitempty"`
	Model          string   `json:"model,omitempty"`
	PermissionMode string   `json:"permission_mode,omitempty"`
	Instructions   string   `json:"instructions"`
	Scope          string   `json:"scope,omitempty"` // "user" or "project", default "project"
}

// TaskPlan describes a task to be created.
type TaskPlan struct {
	Name           string   `json:"name"`
	Prompt         string   `json:"prompt"`
	WorkDir        string   `json:"work_dir,omitempty"`
	Schedule       string   `json:"schedule,omitempty"`
	Tags           []string `json:"tags,omitempty"`
	Model          string   `json:"model,omitempty"`
	Timeout        string   `json:"timeout,omitempty"`
	Agents         []string `json:"agents,omitempty"`
	MCPServers     []string `json:"mcp_servers,omitempty"`
	AllowedTools   []string `json:"allowed_tools,omitempty"`
	MaxTurns       int      `json:"max_turns,omitempty"`
	MaxBudgetUSD   float64  `json:"max_budget_usd,omitempty"`
	PermissionMode string   `json:"permission_mode,omitempty"`
	Domain         string   `json:"domain,omitempty"`
}

// MCPServerPlan describes an MCP server to be created (currently only mcp-openapi).
type MCPServerPlan struct {
	Name    string            `json:"name"`
	Command string            `json:"command"`
	Args    []string          `json:"args,omitempty"`
	Env     map[string]string `json:"env,omitempty"`
}

// PipelinePlan describes a pipeline to be created.
type PipelinePlan struct {
	Name          string   `json:"name"`
	Mode          string   `json:"mode,omitempty"` // "sequential" or "parallel"
	Steps         []string `json:"steps"`          // task names
	MaxIterations int      `json:"max_iterations,omitempty"`
	StopSignal    string   `json:"stop_signal,omitempty"`
}

// ApplyResult holds the outcome of applying a plan.
type ApplyResult struct {
	MCPServersCreated []string `json:"mcp_servers_created,omitempty"`
	DomainsCreated    []string `json:"domains_created,omitempty"`
	AgentsCreated    []string `json:"agents_created,omitempty"`
	TasksCreated     []string `json:"tasks_created,omitempty"`
	PipelinesCreated []string `json:"pipelines_created,omitempty"`
	Errors           []string `json:"errors,omitempty"`
}
