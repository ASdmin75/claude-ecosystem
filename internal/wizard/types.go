package wizard

// GenerateRequest is the input for wizard plan generation.
type GenerateRequest struct {
	Description  string        `json:"description"`
	WorkDir      string        `json:"work_dir,omitempty"`
	RetryContext *RetryContext  `json:"retry_context,omitempty"`
}

// RetryContext carries context from a previous failed generation attempt.
type RetryContext struct {
	PreviousError     string `json:"previous_error"`
	PreviousRawOutput string `json:"previous_raw_output,omitempty"`
	UserHint          string `json:"user_hint,omitempty"`
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
	SetupNotes  string         `json:"setup_notes,omitempty"`
	Status      string         `json:"status"`              // "draft", "applied", "discarded"
	RawOutput   string         `json:"raw_output,omitempty"` // stored for troubleshooting on parse errors
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
	SessionChain  bool     `json:"session_chain,omitempty"`
}

// ErrorCategory classifies a wizard failure for the troubleshooter UI.
type ErrorCategory string

const (
	ErrCatEmptyOutput      ErrorCategory = "empty_output"
	ErrCatJSONParse        ErrorCategory = "json_parse"
	ErrCatTimeout          ErrorCategory = "timeout"
	ErrCatDuplicateName    ErrorCategory = "duplicate_name"
	ErrCatMissingReference ErrorCategory = "missing_reference"
	ErrCatPermissionMode   ErrorCategory = "permission_mode"
	ErrCatApplyFailed      ErrorCategory = "apply_failed"
	ErrCatTestSoftFailure  ErrorCategory = "test_soft_failure"
	ErrCatTestHardFailure  ErrorCategory = "test_hard_failure"
	ErrCatUnknown          ErrorCategory = "unknown"
)

// WizardDiagnosis is a structured error returned to the frontend.
type WizardDiagnosis struct {
	Category    ErrorCategory    `json:"category"`
	Message     string           `json:"message"`
	Details     string           `json:"details,omitempty"`
	Suggestions []RecoveryAction `json:"suggestions"`
}

// RecoveryAction describes one way the user can fix the problem.
type RecoveryAction struct {
	ID          string `json:"id"`
	Label       string `json:"label"`
	Description string `json:"description"`
	PatchedPlan *Plan  `json:"patched_plan,omitempty"`
}

// GenerateError wraps a generation error with the raw Claude output for diagnosis.
type GenerateError struct {
	Err       error
	RawOutput string
}

func (e *GenerateError) Error() string { return e.Err.Error() }
func (e *GenerateError) Unwrap() error { return e.Err }

// TestRunRequest specifies which task to test after plan apply.
type TestRunRequest struct {
	TaskName string `json:"task_name"`
}

// TestRunResult holds the outcome of a wizard test run.
type TestRunResult struct {
	TaskName    string           `json:"task_name"`
	Output      string           `json:"output"`
	Error       string           `json:"error,omitempty"`
	SoftFailure string           `json:"soft_failure,omitempty"`
	DurationMS  int64            `json:"duration_ms"`
	CostUSD     float64          `json:"cost_usd,omitempty"`
	Diagnosis   *WizardDiagnosis `json:"diagnosis,omitempty"`
}

// ApplyResult holds the outcome of applying a plan.
type ApplyResult struct {
	MCPServersCreated []string `json:"mcp_servers_created,omitempty"`
	DomainsCreated    []string `json:"domains_created,omitempty"`
	AgentsCreated     []string `json:"agents_created,omitempty"`
	TasksCreated      []string `json:"tasks_created,omitempty"`
	PipelinesCreated  []string `json:"pipelines_created,omitempty"`
	SetupDocPath      string   `json:"setup_doc_path,omitempty"`
	Warnings          []string `json:"warnings,omitempty"`
	Errors            []string `json:"errors,omitempty"`
}
