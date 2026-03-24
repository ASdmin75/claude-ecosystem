package config

import (
	"fmt"
	"log/slog"
	"time"

	"github.com/robfig/cron/v3"
)

// Validate checks the configuration for common errors:
//   - Unique task names
//   - Required fields (name, prompt)
//   - Parseable timeout and debounce durations
//   - Pipeline steps reference existing tasks
//   - Sequential pipelines require a stop_signal
func Validate(cfg *Config) error {
	seen := make(map[string]bool, len(cfg.Tasks))

	for _, t := range cfg.Tasks {
		if t.Name == "" {
			return fmt.Errorf("task name is required")
		}
		if seen[t.Name] {
			return fmt.Errorf("duplicate task name: %s", t.Name)
		}
		seen[t.Name] = true

		if t.Prompt == "" {
			return fmt.Errorf("task %s: prompt is required", t.Name)
		}

		if t.Timeout != "" {
			if _, err := time.ParseDuration(t.Timeout); err != nil {
				return fmt.Errorf("task %s: invalid timeout %q: %w", t.Name, t.Timeout, err)
			}
		}

		if t.Schedule != "" {
			if _, err := cron.ParseStandard(t.Schedule); err != nil {
				return fmt.Errorf("task %s: invalid cron schedule %q: %w", t.Name, t.Schedule, err)
			}
		}

		if t.Schedule != "" && t.Watch != nil {
			slog.Warn("task has both schedule and watch configured; both triggers will fire independently", "task", t.Name)
		}

		if t.Watch != nil && t.Watch.Debounce != "" {
			if _, err := time.ParseDuration(t.Watch.Debounce); err != nil {
				return fmt.Errorf("task %s: invalid debounce %q: %w", t.Name, t.Watch.Debounce, err)
			}
		}

		if t.MaxTurns < 0 {
			return fmt.Errorf("task %s: max_turns must be >= 0", t.Name)
		}

		if t.Domain != "" {
			if _, ok := cfg.Domains[t.Domain]; !ok {
				return fmt.Errorf("task %s: references unknown domain %q", t.Name, t.Domain)
			}
		}

		if t.Notify != nil && t.Notify.Trigger != "" {
			switch t.Notify.Trigger {
			case "on_success", "on_failure", "always":
			default:
				return fmt.Errorf("task %s: invalid notify trigger %q (must be \"on_success\", \"on_failure\", or \"always\")", t.Name, t.Notify.Trigger)
			}
		}
	}

	for _, p := range cfg.Pipelines {
		if p.Name == "" {
			return fmt.Errorf("pipeline name is required")
		}
		if len(p.Steps) == 0 {
			return fmt.Errorf("pipeline %s: at least one step is required", p.Name)
		}

		if p.Schedule != "" {
			if _, err := cron.ParseStandard(p.Schedule); err != nil {
				return fmt.Errorf("pipeline %s: invalid cron schedule %q: %w", p.Name, p.Schedule, err)
			}
		}

		mode := p.EffectiveMode()
		if mode != "sequential" && mode != "parallel" {
			return fmt.Errorf("pipeline %s: invalid mode %q (must be \"sequential\" or \"parallel\")", p.Name, mode)
		}

		// Sequential pipelines that loop (max_iterations > 1) need a stop signal
		// to know when to exit early. Single-pass pipelines don't need one.
		if mode == "sequential" && p.StopSignal == "" && p.MaxIter() > 1 {
			return fmt.Errorf("pipeline %s: stop_signal is required for sequential pipelines with max_iterations > 1", p.Name)
		}

		for _, step := range p.Steps {
			if !seen[step.Task] {
				return fmt.Errorf("pipeline %s: step references unknown task %q", p.Name, step.Task)
			}
		}

		// session_chain requires sequential mode and consistent work_dir across steps.
		if p.SessionChain {
			if mode != "sequential" {
				return fmt.Errorf("pipeline %s: session_chain is only supported in sequential mode", p.Name)
			}
			var firstDir string
			for i, step := range p.Steps {
				t := taskByName(cfg.Tasks, step.Task)
				if t == nil {
					continue // already validated above
				}
				if i == 0 {
					firstDir = t.WorkDir
				} else if t.WorkDir != firstDir {
					return fmt.Errorf("pipeline %s: session_chain requires all steps to share the same work_dir (step %q has %q, expected %q)", p.Name, step.Task, t.WorkDir, firstDir)
				}
			}
		}
	}

	// Validate domain references
	for name, d := range cfg.Domains {
		for _, taskName := range d.Tasks {
			if !seen[taskName] {
				return fmt.Errorf("domain %s: references unknown task %q", name, taskName)
			}
		}
	}

	return nil
}

// taskByName returns a pointer to the task with the given name, or nil.
func taskByName(tasks []Task, name string) *Task {
	for i := range tasks {
		if tasks[i].Name == name {
			return &tasks[i]
		}
	}
	return nil
}
