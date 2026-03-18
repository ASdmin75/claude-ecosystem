package config

// Pipeline defines a multi-step agent-to-agent workflow.
type Pipeline struct {
	Name          string         `yaml:"name" json:"name"`
	Mode          string         `yaml:"mode" json:"mode"` // "sequential" (default) | "parallel"
	Steps         []PipelineStep `yaml:"steps" json:"steps"`
	MaxIterations int            `yaml:"max_iterations" json:"max_iterations"`
	StopSignal    string         `yaml:"stop_signal,omitempty" json:"stop_signal,omitempty"`
	Collector     string         `yaml:"collector,omitempty" json:"collector,omitempty"`
	SessionChain  bool           `yaml:"session_chain,omitempty" json:"session_chain,omitempty"`
	Schedule        string         `yaml:"schedule,omitempty" json:"schedule,omitempty"`
	AllowConcurrent *bool          `yaml:"allow_concurrent,omitempty" json:"allow_concurrent,omitempty"`
}

// PipelineStep references a task by name.
type PipelineStep struct {
	Task string `yaml:"task" json:"task"`
}

// ConcurrentAllowed reports whether concurrent runs of this pipeline are permitted.
// Defaults to true when AllowConcurrent is not explicitly set.
func (p Pipeline) ConcurrentAllowed() bool {
	if p.AllowConcurrent == nil {
		return true
	}
	return *p.AllowConcurrent
}

// MaxIter returns MaxIterations with a default of 10.
func (p Pipeline) MaxIter() int {
	if p.MaxIterations <= 0 {
		return 10
	}
	return p.MaxIterations
}

// EffectiveMode returns the pipeline mode, defaulting to "sequential".
func (p Pipeline) EffectiveMode() string {
	if p.Mode == "" {
		return "sequential"
	}
	return p.Mode
}
