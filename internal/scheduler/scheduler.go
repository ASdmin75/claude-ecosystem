package scheduler

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/asdmin/claude-ecosystem/internal/config"
	"github.com/asdmin/claude-ecosystem/internal/domain"
	"github.com/asdmin/claude-ecosystem/internal/events"
	"github.com/asdmin/claude-ecosystem/internal/mcpmanager"
	"github.com/asdmin/claude-ecosystem/internal/runguard"
	"github.com/asdmin/claude-ecosystem/internal/subagent"
	"github.com/asdmin/claude-ecosystem/internal/task"
	"github.com/robfig/cron/v3"
)

type Scheduler struct {
	cron      *cron.Cron
	runner    *task.Runner
	subMgr    *subagent.Manager
	mcpMgr    *mcpmanager.Manager
	domainMgr *domain.Manager
	bus       *events.Bus
	guard     *runguard.Guard
	logger    *slog.Logger
	ctxMu     sync.RWMutex
	runCtx    context.Context
	pauseMu   sync.RWMutex
	paused    map[string]bool
}

func New(runner *task.Runner, subMgr *subagent.Manager, mcpMgr *mcpmanager.Manager, domainMgr *domain.Manager, bus *events.Bus, guard *runguard.Guard, logger *slog.Logger) *Scheduler {
	return &Scheduler{
		cron:      cron.New(),
		runner:    runner,
		subMgr:    subMgr,
		mcpMgr:    mcpMgr,
		domainMgr: domainMgr,
		bus:       bus,
		guard:     guard,
		logger:    logger,
		runCtx:    context.Background(),
		paused:    make(map[string]bool),
	}
}

func (s *Scheduler) Register(t config.Task) error {
	if t.Schedule == "" {
		return nil
	}

	_, err := s.cron.AddFunc(t.Schedule, func() {
		if s.IsPaused(t.Name) {
			s.logger.Info("scheduled task is paused, skipping", "task", t.Name)
			return
		}

		if !t.ConcurrentAllowed() {
			if !s.guard.TryAcquire("task:" + t.Name) {
				s.logger.Info("scheduled task is already running, skipping", "task", t.Name)
				return
			}
			defer s.guard.Release("task:" + t.Name)
		}

		s.logger.Info("scheduled task starting", "task", t.Name)

		s.ctxMu.RLock()
		parentCtx := s.runCtx
		s.ctxMu.RUnlock()

		opts, cleanup, err := task.ResolveRunOptions(t, s.subMgr, s.mcpMgr, s.domainMgr)
		if err != nil {
			s.logger.Error("failed to resolve run options", "task", t.Name, "error", err)
			return
		}
		if cleanup != nil {
			defer cleanup()
		}

		timeout := t.ParsedTimeout()
		ctx, cancel := context.WithTimeout(parentCtx, timeout)
		defer cancel()
		now := time.Now()
		vars := map[string]string{
			"Date":     now.Format("2006-01-02"),
			"DateTime": now.Format("2006-01-02_15-04"),
		}
		result := s.runner.Run(ctx, t, opts, vars)

		if result.Error != "" {
			s.logger.Error("task failed", "task", t.Name, "error", result.Error)
		} else {
			s.logger.Info("task completed", "task", t.Name, "duration", result.Duration)
		}

		s.bus.Publish(events.Event{
			Type: "task.completed",
			Payload: map[string]string{
				"task":   t.Name,
				"output": result.Output,
				"error":  result.Error,
			},
		})
	})
	if err != nil {
		return err
	}

	s.logger.Info("registered scheduled task", "task", t.Name, "schedule", t.Schedule)
	return nil
}

// RegisterPipeline registers a pipeline to run on a cron schedule.
// The runFn callback is invoked with a context for each scheduled execution.
func (s *Scheduler) RegisterPipeline(p config.Pipeline, runFn func(ctx context.Context)) error {
	if p.Schedule == "" {
		return nil
	}

	_, err := s.cron.AddFunc(p.Schedule, func() {
		if s.IsPaused(p.Name) {
			s.logger.Info("scheduled pipeline is paused, skipping", "pipeline", p.Name)
			return
		}

		if !p.ConcurrentAllowed() {
			if !s.guard.TryAcquire("pipeline:" + p.Name) {
				s.logger.Info("scheduled pipeline is already running, skipping", "pipeline", p.Name)
				return
			}
			defer s.guard.Release("pipeline:" + p.Name)
		}

		s.logger.Info("scheduled pipeline starting", "pipeline", p.Name)

		s.ctxMu.RLock()
		parentCtx := s.runCtx
		s.ctxMu.RUnlock()

		runFn(parentCtx)
	})
	if err != nil {
		return err
	}

	s.logger.Info("registered scheduled pipeline", "pipeline", p.Name, "schedule", p.Schedule)
	return nil
}

// Pause marks a task as paused so its cron callback will be skipped.
func (s *Scheduler) Pause(taskName string) error {
	s.pauseMu.Lock()
	defer s.pauseMu.Unlock()
	if s.paused[taskName] {
		return fmt.Errorf("task %q is already paused", taskName)
	}
	s.paused[taskName] = true
	s.logger.Info("paused scheduled task", "task", taskName)
	return nil
}

// Resume removes a task from the paused set so it will run on schedule again.
func (s *Scheduler) Resume(taskName string) error {
	s.pauseMu.Lock()
	defer s.pauseMu.Unlock()
	if !s.paused[taskName] {
		return fmt.Errorf("task %q is not paused", taskName)
	}
	delete(s.paused, taskName)
	s.logger.Info("resumed scheduled task", "task", taskName)
	return nil
}

// IsPaused reports whether the named task is currently paused.
func (s *Scheduler) IsPaused(taskName string) bool {
	s.pauseMu.RLock()
	defer s.pauseMu.RUnlock()
	return s.paused[taskName]
}

// Start begins the cron scheduler. The provided context is used as the parent
// for all task runs, so cancelling it will cancel in-flight tasks.
func (s *Scheduler) Start(ctx context.Context) {
	s.ctxMu.Lock()
	s.runCtx = ctx
	s.ctxMu.Unlock()
	s.cron.Start()
}

// Stop halts the cron scheduler and waits for in-flight jobs to complete.
func (s *Scheduler) Stop() {
	ctx := s.cron.Stop()
	<-ctx.Done()
}
