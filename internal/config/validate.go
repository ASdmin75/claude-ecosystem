package config

import (
	"fmt"
	"time"
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

		if t.Watch != nil && t.Watch.Debounce != "" {
			if _, err := time.ParseDuration(t.Watch.Debounce); err != nil {
				return fmt.Errorf("task %s: invalid debounce %q: %w", t.Name, t.Watch.Debounce, err)
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

		mode := p.EffectiveMode()
		if mode != "sequential" && mode != "parallel" {
			return fmt.Errorf("pipeline %s: invalid mode %q (must be \"sequential\" or \"parallel\")", p.Name, mode)
		}

		// Sequential pipelines need a stop signal to know when to exit the loop.
		if mode == "sequential" && p.StopSignal == "" {
			return fmt.Errorf("pipeline %s: stop_signal is required for sequential pipelines", p.Name)
		}

		for _, step := range p.Steps {
			if !seen[step.Task] {
				return fmt.Errorf("pipeline %s: step references unknown task %q", p.Name, step.Task)
			}
		}
	}

	return nil
}
