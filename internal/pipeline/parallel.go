package pipeline

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/asdmin/claude-ecosystem/internal/config"
	"github.com/asdmin/claude-ecosystem/internal/task"
)

// RunParallel runs all pipeline steps concurrently, collects their results,
// and optionally runs a collector task with the aggregated outputs.
// If p.Collector is set, that task is invoked with a {{.Results}} template
// variable containing a JSON-encoded map[string]string of task name to output.
func (r *Runner) RunParallel(ctx context.Context, p config.Pipeline) (string, error) {
	var (
		mu      sync.Mutex
		wg      sync.WaitGroup
		results = make(map[string]string, len(p.Steps))
		errs    []error
	)

	for _, step := range p.Steps {
		t, ok := r.tasks[step.Task]
		if !ok {
			return "", fmt.Errorf("pipeline %s: unknown task %q", p.Name, step.Task)
		}

		wg.Add(1)
		go func(t config.Task) {
			defer wg.Done()

			timeout := t.ParsedTimeout()
			stepCtx, cancel := context.WithTimeout(ctx, timeout)
			defer cancel()

			r.logger.Info("running parallel step", "pipeline", p.Name, "task", t.Name)
			result := r.taskRunner.Run(stepCtx, t, task.RunOptions{}, nil)

			mu.Lock()
			defer mu.Unlock()

			if result.Error != "" {
				errs = append(errs, fmt.Errorf("task %s: %s", t.Name, result.Error))
			} else {
				results[t.Name] = result.Output
			}
		}(t)
	}

	wg.Wait()

	if len(errs) > 0 {
		return "", fmt.Errorf("pipeline %s: %d step(s) failed: %v", p.Name, len(errs), errs)
	}

	// If no collector is configured, return the JSON-encoded results map.
	resultsJSON, err := json.Marshal(results)
	if err != nil {
		return "", fmt.Errorf("pipeline %s: failed to marshal results: %w", p.Name, err)
	}

	if p.Collector == "" {
		return string(resultsJSON), nil
	}

	// Run the collector task with the aggregated results.
	collectorTask, ok := r.tasks[p.Collector]
	if !ok {
		return string(resultsJSON), fmt.Errorf("pipeline %s: unknown collector task %q", p.Name, p.Collector)
	}

	r.logger.Info("running collector", "pipeline", p.Name, "collector", p.Collector)

	timeout := collectorTask.ParsedTimeout()
	collectorCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	vars := map[string]string{
		"Results": string(resultsJSON),
	}
	result := r.taskRunner.Run(collectorCtx, collectorTask, task.RunOptions{}, vars)

	if result.Error != "" {
		return string(resultsJSON), fmt.Errorf("pipeline %s, collector %s: %s", p.Name, p.Collector, result.Error)
	}

	return result.Output, nil
}
