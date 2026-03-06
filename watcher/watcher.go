package watcher

import (
	"context"
	"log/slog"
	"path/filepath"
	"strings"
	"time"

	"github.com/ASdmin75/claude-ecosystem/agent"
	"github.com/ASdmin75/claude-ecosystem/config"
	"github.com/ASdmin75/claude-ecosystem/internal/events"
	"github.com/fsnotify/fsnotify"
)

type Watcher struct {
	fsw    *fsnotify.Watcher
	runner *agent.Runner
	bus    *events.Bus
	logger *slog.Logger
	agents []config.Agent
}

func New(runner *agent.Runner, bus *events.Bus, logger *slog.Logger) (*Watcher, error) {
	fsw, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}
	return &Watcher{
		fsw:    fsw,
		runner: runner,
		bus:    bus,
		logger: logger,
	}, nil
}

func (w *Watcher) Register(ag config.Agent) error {
	if ag.Watch == nil {
		return nil
	}

	w.agents = append(w.agents, ag)

	for _, p := range ag.Watch.Paths {
		absPath, err := filepath.Abs(filepath.Join(ag.WorkDir, p))
		if err != nil {
			return err
		}
		if err := w.fsw.Add(absPath); err != nil {
			return err
		}
		w.logger.Info("watching path", "agent", ag.Name, "path", absPath)
	}
	return nil
}

func (w *Watcher) Start(ctx context.Context) {
	debounce := make(map[string]time.Time)

	for {
		select {
		case <-ctx.Done():
			return
		case event, ok := <-w.fsw.Events:
			if !ok {
				return
			}
			if event.Op&(fsnotify.Write|fsnotify.Create) == 0 {
				continue
			}

			if last, exists := debounce[event.Name]; exists && time.Since(last) < 3*time.Second {
				continue
			}
			debounce[event.Name] = time.Now()

			for _, ag := range w.agents {
				if !matchesExtensions(event.Name, ag.Watch.Extensions) {
					continue
				}

				w.logger.Info("file change detected, running agent", "agent", ag.Name, "file", event.Name)

				go func(ag config.Agent, file string) {
					vars := map[string]string{"File": file}
					result := w.runner.Run(ctx, ag.Name, ag.Prompt, ag.WorkDir, vars)

					if result.Error != "" {
						w.logger.Error("watcher agent failed", "agent", ag.Name, "error", result.Error)
					} else {
						w.logger.Info("watcher agent completed", "agent", ag.Name, "duration", result.Duration)
					}

					w.bus.Publish(events.Event{
						Type: "agent.completed",
						Payload: map[string]string{
							"agent":  ag.Name,
							"file":   file,
							"output": result.Output,
							"error":  result.Error,
						},
					})
				}(ag, event.Name)
			}

		case err, ok := <-w.fsw.Errors:
			if !ok {
				return
			}
			w.logger.Error("watcher error", "error", err)
		}
	}
}

func (w *Watcher) Close() error {
	return w.fsw.Close()
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
