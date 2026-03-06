package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/ASdmin75/claude-ecosystem/agent"
	"github.com/ASdmin75/claude-ecosystem/config"
	"github.com/ASdmin75/claude-ecosystem/internal/events"
	"github.com/ASdmin75/claude-ecosystem/scheduler"
	"github.com/ASdmin75/claude-ecosystem/watcher"
)

func main() {
	cfgPath := flag.String("config", "agents.yaml", "path to agents config file")
	runOnce := flag.String("run", "", "run a specific agent once by name and exit")
	flag.Parse()

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))

	cfg, err := config.Load(*cfgPath)
	if err != nil {
		logger.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	runner := agent.NewRunner(cfg.ClaudeBin)
	bus := events.NewBus()

	// Log all agent completions
	bus.Subscribe("agent.completed", func(e events.Event) {
		if e.Payload["error"] != "" {
			logger.Error("agent result", "agent", e.Payload["agent"], "error", e.Payload["error"])
			return
		}
		fmt.Fprintf(os.Stdout, "\n=== %s ===\n%s\n", e.Payload["agent"], e.Payload["output"])
	})

	// Run single agent mode
	if *runOnce != "" {
		for _, ag := range cfg.Agents {
			if ag.Name == *runOnce {
				result := runner.Run(context.Background(), ag.Name, ag.Prompt, ag.WorkDir, nil)
				if result.Error != "" {
					logger.Error("agent failed", "error", result.Error)
					os.Exit(1)
				}
				fmt.Println(result.Output)
				return
			}
		}
		logger.Error("agent not found", "name", *runOnce)
		os.Exit(1)
	}

	// Daemon mode: start scheduler + watcher
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sched := scheduler.New(runner, bus, logger)
	w, err := watcher.New(runner, bus, logger)
	if err != nil {
		logger.Error("failed to create watcher", "error", err)
		os.Exit(1)
	}
	defer w.Close()

	for _, ag := range cfg.Agents {
		if ag.Schedule != "" {
			if err := sched.Register(ag); err != nil {
				logger.Error("failed to register scheduled agent", "agent", ag.Name, "error", err)
			}
		}
		if ag.Watch != nil {
			if err := w.Register(ag); err != nil {
				logger.Error("failed to register watcher agent", "agent", ag.Name, "error", err)
			}
		}
	}

	sched.Start()
	defer sched.Stop()

	go w.Start(ctx)

	logger.Info("orchestrator started", "agents", len(cfg.Agents))

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig

	logger.Info("shutting down")
	cancel()
}
