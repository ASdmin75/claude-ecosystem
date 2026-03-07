package config

import "time"

const (
	// DefaultTimeout is the default task execution timeout.
	DefaultTimeout = 5 * time.Minute
	// DefaultDebounce is the default file-watch debounce interval.
	DefaultDebounce = 3 * time.Second
)

// Task defines a single unit of work (formerly called Agent).
type Task struct {
	Name               string   `yaml:"name" json:"name"`
	Prompt             string   `yaml:"prompt" json:"prompt"`
	WorkDir            string   `yaml:"work_dir" json:"work_dir"`
	Schedule           string   `yaml:"schedule,omitempty" json:"schedule,omitempty"`
	Watch              *Watch   `yaml:"watch,omitempty" json:"watch,omitempty"`
	Tags               []string `yaml:"tags,omitempty" json:"tags,omitempty"`
	Model              string   `yaml:"model,omitempty" json:"model,omitempty"`
	Timeout            string   `yaml:"timeout,omitempty" json:"timeout,omitempty"`
	Agents             []string `yaml:"agents,omitempty" json:"agents,omitempty"`
	MCPServers         []string `yaml:"mcp_servers,omitempty" json:"mcp_servers,omitempty"`
	AllowedTools       []string `yaml:"allowed_tools,omitempty" json:"allowed_tools,omitempty"`
	DisallowedTools    []string `yaml:"disallowed_tools,omitempty" json:"disallowed_tools,omitempty"`
	JSONSchema         string   `yaml:"json_schema,omitempty" json:"json_schema,omitempty"`
	AppendSystemPrompt string   `yaml:"append_system_prompt,omitempty" json:"append_system_prompt,omitempty"`
	MaxTurns           int      `yaml:"max_turns,omitempty" json:"max_turns,omitempty"`
	MaxBudgetUSD       float64  `yaml:"max_budget_usd,omitempty" json:"max_budget_usd,omitempty"`
	OutputFormat       string   `yaml:"output_format,omitempty" json:"output_format,omitempty"`
}

// ParsedTimeout returns the task timeout as a time.Duration.
// Falls back to DefaultTimeout when the field is empty or unparseable.
func (t Task) ParsedTimeout() time.Duration {
	if t.Timeout == "" {
		return DefaultTimeout
	}
	d, err := time.ParseDuration(t.Timeout)
	if err != nil {
		return DefaultTimeout
	}
	return d
}

// Watch describes filesystem paths to monitor for changes.
type Watch struct {
	Paths      []string `yaml:"paths" json:"paths"`
	Extensions []string `yaml:"extensions" json:"extensions"`
	Debounce   string   `yaml:"debounce" json:"debounce"`
}

// ParsedDebounce returns the debounce duration, defaulting to DefaultDebounce.
func (w Watch) ParsedDebounce() time.Duration {
	if w.Debounce == "" {
		return DefaultDebounce
	}
	d, err := time.ParseDuration(w.Debounce)
	if err != nil {
		return DefaultDebounce
	}
	return d
}
