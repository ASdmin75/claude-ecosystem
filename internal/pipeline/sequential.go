package pipeline

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/asdmin/claude-ecosystem/internal/config"
	"github.com/asdmin/claude-ecosystem/internal/mcpmanager"
	"github.com/asdmin/claude-ecosystem/internal/subagent"
	"github.com/asdmin/claude-ecosystem/internal/task"
)

// Runner orchestrates pipeline execution by delegating to the task runner.
type Runner struct {
	taskRunner *task.Runner
	tasks      map[string]config.Task
	subMgr     *subagent.Manager
	mcpMgr     *mcpmanager.Manager
	logger     *slog.Logger
}

// NewRunner creates a pipeline Runner. The tasks slice is indexed by name for
// quick lookup when resolving pipeline steps.
func NewRunner(taskRunner *task.Runner, tasks []config.Task, subMgr *subagent.Manager, mcpMgr *mcpmanager.Manager, logger *slog.Logger) *Runner {
	m := make(map[string]config.Task, len(tasks))
	for _, t := range tasks {
		m[t.Name] = t
	}
	return &Runner{
		taskRunner: taskRunner,
		tasks:      m,
		subMgr:     subMgr,
		mcpMgr:     mcpMgr,
		logger:     logger,
	}
}

// RunSequential runs a pipeline in sequential mode: it loops through the steps
// up to MaxIterations times, passing each step's output to the next as
// {{.PrevOutput}}. It stops early when a step's output contains the pipeline's
// StopSignal. Returns the final output and any error.
func (r *Runner) RunSequential(ctx context.Context, p config.Pipeline) (string, error) {
	maxIter := p.MaxIter()
	prevOutput := ""

	for i := 1; i <= maxIter; i++ {
		r.logger.Info("pipeline iteration", "pipeline", p.Name, "iteration", i, "max", maxIter)

		for stepIdx, step := range p.Steps {
			t, ok := r.tasks[step.Task]
			if !ok {
				return prevOutput, fmt.Errorf("pipeline %s: unknown task %q at step %d", p.Name, step.Task, stepIdx)
			}

			timeout := t.ParsedTimeout()
			stepCtx, cancel := context.WithTimeout(ctx, timeout)

			vars := map[string]string{
				"PrevOutput": prevOutput,
				"Iteration":  fmt.Sprintf("%d", i),
			}

			opts, cleanup, err := task.ResolveRunOptions(t, r.subMgr, r.mcpMgr)
			if err != nil {
				cancel()
				return prevOutput, fmt.Errorf("pipeline %s, step %s: resolve options: %w", p.Name, step.Task, err)
			}
			if cleanup != nil {
				defer cleanup()
			}

			r.logger.Info("running step", "pipeline", p.Name, "step", step.Task, "iteration", i)
			result := r.taskRunner.Run(stepCtx, t, opts, vars)
			cancel()

			if result.Error != "" {
				return prevOutput, fmt.Errorf("pipeline %s, step %s (iteration %d): %s", p.Name, step.Task, i, result.Error)
			}

			fmt.Printf("\n=== [%s] iteration %d, step %d: %s ===\n%s\n",
				p.Name, i, stepIdx+1, step.Task, result.Output)

			prevOutput = result.Output

			// Check stop signal after each step (typically after the review step).
			if p.StopSignal != "" && strings.Contains(result.Output, p.StopSignal) {
				r.logger.Info("stop signal detected",
					"pipeline", p.Name,
					"step", step.Task,
					"iteration", i,
					"signal", p.StopSignal,
				)
				return prevOutput, nil
			}
		}
	}

	r.logger.Warn("pipeline reached max iterations", "pipeline", p.Name, "max", maxIter)
	return prevOutput, nil
}
