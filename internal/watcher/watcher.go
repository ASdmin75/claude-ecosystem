package watcher

import (
	"context"
	"log/slog"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/asdmin/claude-ecosystem/internal/config"
	"github.com/asdmin/claude-ecosystem/internal/domain"
	"github.com/asdmin/claude-ecosystem/internal/events"
	"github.com/asdmin/claude-ecosystem/internal/mcpmanager"
	"github.com/asdmin/claude-ecosystem/internal/runguard"
	"github.com/asdmin/claude-ecosystem/internal/subagent"
	"github.com/asdmin/claude-ecosystem/internal/task"
	"github.com/fsnotify/fsnotify"
)

const maxConcurrentTasks = 4

type Watcher struct {
	fsw       *fsnotify.Watcher
	runner    *task.Runner
	subMgr    *subagent.Manager
	mcpMgr    *mcpmanager.Manager
	domainMgr *domain.Manager
	bus       *events.Bus
	guard     *runguard.Guard
	logger    *slog.Logger
	mu        sync.RWMutex
	tasks     []config.Task
	sem       chan struct{}   // concurrency limiter
	wg        sync.WaitGroup // tracks in-flight task goroutines
}

func New(runner *task.Runner, subMgr *subagent.Manager, mcpMgr *mcpmanager.Manager, domainMgr *domain.Manager, bus *events.Bus, guard *runguard.Guard, logger *slog.Logger) (*Watcher, error) {
	fsw, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}
	return &Watcher{
		fsw:       fsw,
		runner:    runner,
		subMgr:    subMgr,
		mcpMgr:    mcpMgr,
		domainMgr: domainMgr,
		bus:       bus,
		guard:     guard,
		logger:    logger,
		sem:       make(chan struct{}, maxConcurrentTasks),
	}, nil
}

func (w *Watcher) Register(t config.Task) error {
	if t.Watch == nil {
		return nil
	}

	w.mu.Lock()
	w.tasks = append(w.tasks, t)
	w.mu.Unlock()

	for _, p := range t.Watch.Paths {
		absPath, err := filepath.Abs(filepath.Join(t.WorkDir, p))
		if err != nil {
			return err
		}
		if err := w.fsw.Add(absPath); err != nil {
			return err
		}
		w.logger.Info("watching path", "task", t.Name, "path", absPath)
	}
	return nil
}

func (w *Watcher) Start(ctx context.Context) {
	debounce := make(map[string]time.Time)
	cleanTicker := time.NewTicker(5 * time.Minute)
	defer cleanTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			return

		case <-cleanTicker.C:
			// Purge stale debounce entries
			now := time.Now()
			for k, t := range debounce {
				if now.Sub(t) > time.Minute {
					delete(debounce, k)
				}
			}

		case event, ok := <-w.fsw.Events:
			if !ok {
				return
			}
			if event.Op&(fsnotify.Write|fsnotify.Create) == 0 {
				continue
			}

			w.mu.RLock()
			tasks := make([]config.Task, len(w.tasks))
			copy(tasks, w.tasks)
			w.mu.RUnlock()

			for _, t := range tasks {
				if !matchesExtensions(event.Name, t.Watch.Extensions) {
					continue
				}

				debounceKey := t.Name + ":" + event.Name
				debounceDur := t.Watch.ParsedDebounce()
				if last, exists := debounce[debounceKey]; exists && time.Since(last) < debounceDur {
					continue
				}
				debounce[debounceKey] = time.Now()

				w.logger.Info("file change detected, running task", "task", t.Name, "file", event.Name)

				if !t.ConcurrentAllowed() {
					if !w.guard.TryAcquire("task:" + t.Name) {
						w.logger.Info("watched task is already running, skipping", "task", t.Name)
						continue
					}
				}

				// Acquire semaphore to limit concurrency
				select {
				case w.sem <- struct{}{}:
				case <-ctx.Done():
					if !t.ConcurrentAllowed() {
						w.guard.Release("task:" + t.Name)
					}
					return
				}

				w.wg.Add(1)
				go func(t config.Task, file string) {
					defer w.wg.Done()
					defer func() { <-w.sem }()
					if !t.ConcurrentAllowed() {
						defer w.guard.Release("task:" + t.Name)
					}

					opts, cleanup, err := task.ResolveRunOptions(t, w.subMgr, w.mcpMgr, w.domainMgr)
					if err != nil {
						w.logger.Error("failed to resolve run options", "task", t.Name, "error", err)
						return
					}
					if cleanup != nil {
						defer cleanup()
					}

					timeout := t.ParsedTimeout()
					runCtx, cancel := context.WithTimeout(ctx, timeout)
					defer cancel()

					vars := map[string]string{"File": file}
					result := w.runner.Run(runCtx, t, opts, vars)

					if result.Error != "" {
						w.logger.Error("watcher task failed", "task", t.Name, "error", result.Error)
					} else {
						w.logger.Info("watcher task completed", "task", t.Name, "duration", result.Duration)
					}

					w.bus.Publish(events.Event{
						Type: "task.completed",
						Payload: map[string]string{
							"task":   t.Name,
							"file":   file,
							"output": result.Output,
							"error":  result.Error,
						},
					})
				}(t, event.Name)
			}

		case err, ok := <-w.fsw.Errors:
			if !ok {
				return
			}
			w.logger.Error("watcher error", "error", err)
		}
	}
}

// Close stops the filesystem watcher and waits up to 30 seconds for in-flight
// task runs to finish.
func (w *Watcher) Close() error {
	err := w.fsw.Close()
	done := make(chan struct{})
	go func() {
		w.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(30 * time.Second):
		w.logger.Warn("timed out waiting for in-flight tasks to finish")
	}
	return err
}

// Reset removes all watched paths and clears registered tasks.
// Callers must re-register tasks after calling Reset.
func (w *Watcher) Reset() {
	w.mu.Lock()
	w.tasks = nil
	w.mu.Unlock()

	for _, path := range w.fsw.WatchList() {
		_ = w.fsw.Remove(path)
	}
}

func matchesExtensions(file string, exts []string) bool {
	if len(exts) == 0 {
		return true
	}
	ext := filepath.Ext(file)
	for _, e := range exts {
		if strings.EqualFold(ext, e) {
			return true
		}
	}
	return false
}
