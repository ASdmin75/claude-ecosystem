package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/asdmin/claude-ecosystem/internal/api"
	"github.com/asdmin/claude-ecosystem/internal/auth"
	"github.com/asdmin/claude-ecosystem/internal/config"
	"github.com/asdmin/claude-ecosystem/internal/events"
	"github.com/asdmin/claude-ecosystem/internal/mcpmanager"
	"github.com/asdmin/claude-ecosystem/internal/pipeline"
	"github.com/asdmin/claude-ecosystem/internal/scheduler"
	"github.com/asdmin/claude-ecosystem/internal/store"
	"github.com/asdmin/claude-ecosystem/internal/store/sqlite"
	"github.com/asdmin/claude-ecosystem/internal/subagent"
	"github.com/asdmin/claude-ecosystem/internal/task"
	"github.com/asdmin/claude-ecosystem/internal/watcher"
)

func main() {
	cfgPath := flag.String("config", "", "path to config file (default: tasks.yaml or agents.yaml)")
	runOnce := flag.String("run", "", "run a specific task once by name and exit")
	runPipeline := flag.String("pipeline", "", "run a pipeline by name and exit")
	flag.Parse()

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))

	cfg, err := config.Load(*cfgPath)
	if err != nil {
		logger.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	taskRunner := task.NewRunner(cfg.ClaudeBin)
	bus := events.NewBus()

	var outputMu sync.Mutex

	bus.Subscribe("task.completed", func(e events.Event) {
		if e.Payload["error"] != "" {
			logger.Error("task result", "task", e.Payload["task"], "error", e.Payload["error"])
			return
		}
		outputMu.Lock()
		fmt.Fprintf(os.Stdout, "\n=== %s ===\n%s\n", e.Payload["task"], e.Payload["output"])
		outputMu.Unlock()
	})

	// Initialize sub-agent manager and MCP manager (needed for task resolution in all modes)
	subagentMgr := subagent.NewManager(".claude/agents")
	mcpMgr := mcpmanager.New(cfg.MCPServers, logger)
	defer mcpMgr.StopAll()

	// Run single task mode
	if *runOnce != "" {
		for _, t := range cfg.Tasks {
			if t.Name == *runOnce {
				opts, cleanup, resolveErr := task.ResolveRunOptions(t, subagentMgr, mcpMgr)
				if resolveErr != nil {
					logger.Error("failed to resolve run options", "error", resolveErr)
					os.Exit(1)
				}
				if cleanup != nil {
					defer cleanup()
				}
				timeout := t.ParsedTimeout()
				ctx, cancel := context.WithTimeout(context.Background(), timeout)
				defer cancel()
				result := taskRunner.Run(ctx, t, opts, nil)
				if result.Error != "" {
					logger.Error("task failed", "error", result.Error)
					os.Exit(1)
				}
				fmt.Println(result.Output)
				return
			}
		}
		logger.Error("task not found", "name", *runOnce)
		os.Exit(1)
	}

	// Run pipeline mode
	if *runPipeline != "" {
		pr := pipeline.NewRunner(taskRunner, cfg.Tasks, subagentMgr, mcpMgr, logger)
		for _, p := range cfg.Pipelines {
			if p.Name == *runPipeline {
				output, err := pr.Run(context.Background(), p)
				if err != nil {
					logger.Error("pipeline failed", "error", err)
					os.Exit(1)
				}
				fmt.Println(output)
				return
			}
		}
		logger.Error("pipeline not found", "name", *runPipeline)
		os.Exit(1)
	}

	// Daemon mode: start HTTP server + scheduler + watcher
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Initialize storage
	if err := os.MkdirAll(cfg.Server.DataDir, 0o755); err != nil {
		logger.Error("failed to create data directory", "error", err)
		os.Exit(1)
	}
	dbPath := filepath.Join(cfg.Server.DataDir, "claude-ecosystem.db")
	db, err := sqlite.New(dbPath)
	if err != nil {
		logger.Error("failed to open database", "error", err)
		os.Exit(1)
	}
	defer db.Close()

	// Clean up stale "running" executions from previous runs
	if n, err := db.MarkStaleRunning(ctx); err != nil {
		logger.Error("failed to mark stale executions", "error", err)
	} else if n > 0 {
		logger.Info("marked stale running executions as failed", "count", n)
	}

	// Seed users from config into database
	for _, u := range cfg.Auth.Users {
		existing, _ := db.GetUserByUsername(ctx, u.Username)
		if existing == nil {
			if err := db.CreateUser(ctx, &store.User{
				ID:           u.Username,
				Username:     u.Username,
				PasswordHash: u.Password,
			}); err != nil {
				logger.Error("failed to seed user", "username", u.Username, "error", err)
			} else {
				logger.Info("seeded user from config", "username", u.Username)
			}
		}
	}

	// Initialize auth
	var pasetoMgr *auth.PASETOManager
	if cfg.Auth.PASETOKey != "" {
		pasetoMgr, err = auth.NewPASETOManager(cfg.Auth.PASETOKey)
		if err != nil {
			logger.Error("failed to create PASETO manager", "error", err)
			os.Exit(1)
		}
	} else {
		// Generate a key for this session (tokens won't persist across restarts)
		key := auth.GenerateKey()
		pasetoMgr, _ = auth.NewPASETOManager(key)
		logger.Warn("no PASETO key configured, generated ephemeral key")
	}
	bearerAuth := auth.NewBearerAuth(cfg.Auth.BearerTokens)
	authMw := auth.NewMiddleware(pasetoMgr, bearerAuth)

	// Initialize scheduler and watcher
	sched := scheduler.New(taskRunner, subagentMgr, mcpMgr, bus, logger)
	w, err := watcher.New(taskRunner, subagentMgr, mcpMgr, bus, logger)
	if err != nil {
		logger.Error("failed to create watcher", "error", err)
		os.Exit(1)
	}
	defer w.Close()

	for _, t := range cfg.Tasks {
		if t.Schedule != "" {
			if err := sched.Register(t); err != nil {
				logger.Error("failed to register scheduled task", "task", t.Name, "error", err)
			}
		}
		if t.Watch != nil {
			if err := w.Register(t); err != nil {
				logger.Error("failed to register watcher task", "task", t.Name, "error", err)
			}
		}
	}

	sched.Start(ctx)
	defer sched.Stop()
	go w.Start(ctx)

	// Initialize REST API
	apiServer := api.NewServer(
		cfg, taskRunner, subagentMgr, mcpMgr,
		db, db, authMw, pasetoMgr,
		bus, logger,
	)

	httpServer := &http.Server{
		Addr:         cfg.Server.Addr,
		Handler:      apiServer.Handler(),
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 0, // no write timeout for SSE
		IdleTimeout:  120 * time.Second,
	}

	go func() {
		logger.Info("server starting", "addr", cfg.Server.Addr, "tasks", len(cfg.Tasks))
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("server error", "error", err)
			os.Exit(1)
		}
	}()

	// Wait for shutdown signal
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig

	logger.Info("shutting down")
	cancel()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer shutdownCancel()
	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		logger.Error("server shutdown error", "error", err)
	}

	bus.Wait()
}
