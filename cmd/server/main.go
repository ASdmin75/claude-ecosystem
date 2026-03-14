package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/asdmin/claude-ecosystem/internal/api"
	"github.com/asdmin/claude-ecosystem/internal/auth"
	"github.com/asdmin/claude-ecosystem/internal/config"
	"github.com/asdmin/claude-ecosystem/internal/domain"
	"github.com/asdmin/claude-ecosystem/internal/events"
	"github.com/asdmin/claude-ecosystem/internal/mcpmanager"
	"github.com/asdmin/claude-ecosystem/internal/notify"
	"github.com/asdmin/claude-ecosystem/internal/pipeline"
	"github.com/asdmin/claude-ecosystem/internal/runguard"
	"github.com/asdmin/claude-ecosystem/internal/scheduler"
	"github.com/asdmin/claude-ecosystem/internal/store"
	"github.com/asdmin/claude-ecosystem/internal/store/sqlite"
	"github.com/asdmin/claude-ecosystem/internal/subagent"
	"github.com/asdmin/claude-ecosystem/internal/task"
	"github.com/asdmin/claude-ecosystem/internal/watcher"
	"github.com/fsnotify/fsnotify"
)

func main() {
	cfgPath := flag.String("config", "", "path to config file (default: tasks.yaml or agents.yaml)")
	runOnce := flag.String("run", "", "run a specific task once by name and exit")
	runPipeline := flag.String("pipeline", "", "run a pipeline by name and exit")
	flag.Parse()

	// Bootstrap logger at Info level for config loading; reconfigure after config is loaded.
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))

	cfg, err := config.Load(*cfgPath)
	if err != nil {
		logger.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	// Reconfigure logger based on config (log_level, log_file).
	newLogger, logCleanup, err := setupLogger(cfg.Server)
	if err != nil {
		logger.Error("failed to setup logger", "error", err)
		os.Exit(1)
	}
	logger = newLogger
	if logCleanup != nil {
		defer logCleanup()
	}

	taskRunner := task.NewRunner(cfg.ClaudeBin)
	bus := events.NewBus()

	bus.Subscribe("task.completed", func(e events.Event) {
		if e.Payload["error"] != "" {
			logger.Error("task completed with error", "task", e.Payload["task"], "error", e.Payload["error"])
			return
		}
		logger.Info("task completed", "task", e.Payload["task"], "output_length", len(e.Payload["output"]))
	})

	// Initialize email/webhook notification handler.
	notifier := notify.NewHandler(cfg.Tasks, logger)
	notifier.Subscribe(bus)

	// Initialize sub-agent manager, MCP manager, and domain manager (needed for task resolution in all modes)
	subagentMgr := subagent.NewManager(".claude/agents")
	mcpMgr := mcpmanager.New(cfg.MCPServers, logger)
	defer mcpMgr.StopAll()

	domainMgr := domain.New(cfg.Domains, logger)
	if err := domainMgr.Init(); err != nil {
		logger.Error("failed to initialize domains", "error", err)
		os.Exit(1)
	}

	// Run single task mode
	if *runOnce != "" {
		for _, t := range cfg.Tasks {
			if t.Name == *runOnce {
				opts, cleanup, resolveErr := task.ResolveRunOptions(t, subagentMgr, mcpMgr, domainMgr)
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
		pr := pipeline.NewRunner(taskRunner, cfg.Tasks, subagentMgr, mcpMgr, domainMgr, logger)
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

	// Initialize concurrency guard (shared across scheduler, watcher, and API)
	guard := runguard.New()

	// Initialize scheduler and watcher
	sched := scheduler.New(taskRunner, subagentMgr, mcpMgr, domainMgr, bus, guard, logger)
	w, err := watcher.New(taskRunner, subagentMgr, mcpMgr, domainMgr, bus, guard, logger)
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

	// Initialize REST API
	apiServer := api.NewServer(
		cfg, taskRunner, subagentMgr, mcpMgr, domainMgr,
		db, db, authMw, pasetoMgr,
		bus, guard, logger,
	)

	// Register pipeline schedules (needs apiServer for execution logic)
	for _, p := range cfg.Pipelines {
		if p.Schedule != "" {
			pName := p.Name
			if err := sched.RegisterPipeline(p, func(ctx context.Context) {
				apiServer.RunPipelineByName(ctx, pName, "schedule")
			}); err != nil {
				logger.Error("failed to register scheduled pipeline", "pipeline", p.Name, "error", err)
			}
		}
	}

	sched.Start(ctx)
	defer sched.Stop()
	go w.Start(ctx)

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

	// Watch config file for hot reload
	go watchConfigFile(cfg, sched, w, apiServer, bus, logger)

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

// watchConfigFile monitors the config file for changes and triggers a hot reload.
// It watches the parent directory to survive file renames (editor save strategies).
func watchConfigFile(cfg *config.Config, sched *scheduler.Scheduler, w *watcher.Watcher, apiServer *api.Server, bus *events.Bus, logger *slog.Logger) {
	absPath, err := filepath.Abs(cfg.FilePath)
	if err != nil {
		logger.Error("failed to resolve config file path", "error", err)
		return
	}

	fsw, err := fsnotify.NewWatcher()
	if err != nil {
		logger.Error("failed to create config file watcher", "error", err)
		return
	}
	defer fsw.Close()

	dir := filepath.Dir(absPath)
	base := filepath.Base(absPath)
	if err := fsw.Add(dir); err != nil {
		logger.Error("failed to watch config directory", "path", dir, "error", err)
		return
	}

	logger.Info("watching config file for hot reload", "path", absPath)

	var debounceTimer *time.Timer
	for {
		select {
		case event, ok := <-fsw.Events:
			if !ok {
				return
			}
			if filepath.Base(event.Name) != base {
				continue
			}
			if event.Op&(fsnotify.Write|fsnotify.Create) == 0 {
				continue
			}

			// Debounce: wait for writes to settle (editors may do multiple writes)
			if debounceTimer != nil {
				debounceTimer.Stop()
			}
			debounceTimer = time.AfterFunc(time.Second, func() {
				reloadConfig(cfg, sched, w, apiServer, bus, logger)
			})

		case err, ok := <-fsw.Errors:
			if !ok {
				return
			}
			logger.Error("config watcher error", "error", err)
		}
	}
}

// reloadConfig re-reads the config file and re-registers tasks/pipelines
// in the scheduler and watcher.
func reloadConfig(cfg *config.Config, sched *scheduler.Scheduler, w *watcher.Watcher, apiServer *api.Server, bus *events.Bus, logger *slog.Logger) {
	newCfg, err := config.Load(cfg.FilePath)
	if err != nil {
		logger.Error("config reload failed", "error", err)
		return
	}

	// Update shared config (tasks and pipelines only).
	cfg.Tasks = newCfg.Tasks
	cfg.Pipelines = newCfg.Pipelines

	// Reset and re-register scheduler.
	sched.Reset()
	for _, t := range cfg.Tasks {
		if t.Schedule != "" {
			if err := sched.Register(t); err != nil {
				logger.Error("reload: failed to register scheduled task", "task", t.Name, "error", err)
			}
		}
	}
	for _, p := range cfg.Pipelines {
		if p.Schedule != "" {
			pName := p.Name
			if err := sched.RegisterPipeline(p, func(ctx context.Context) {
				apiServer.RunPipelineByName(ctx, pName, "schedule")
			}); err != nil {
				logger.Error("reload: failed to register scheduled pipeline", "pipeline", p.Name, "error", err)
			}
		}
	}

	// Reset and re-register watcher.
	w.Reset()
	for _, t := range cfg.Tasks {
		if t.Watch != nil {
			if err := w.Register(t); err != nil {
				logger.Error("reload: failed to register watcher task", "task", t.Name, "error", err)
			}
		}
	}

	bus.Publish(events.Event{
		Type: "config.reloaded",
		Payload: map[string]string{
			"tasks":     fmt.Sprintf("%d", len(cfg.Tasks)),
			"pipelines": fmt.Sprintf("%d", len(cfg.Pipelines)),
		},
	})

	logger.Info("config reloaded", "tasks", len(cfg.Tasks), "pipelines", len(cfg.Pipelines))
}

// parseLogLevel converts a string log level name to slog.Level.
func parseLogLevel(s string) slog.Level {
	switch strings.ToLower(s) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// setupLogger creates a slog.Logger based on server configuration.
// It returns the logger and an optional cleanup function to close the log file.
func setupLogger(sc config.ServerConfig) (*slog.Logger, func(), error) {
	level := parseLogLevel(sc.LogLevel)
	opts := &slog.HandlerOptions{Level: level}

	var w io.Writer = os.Stderr
	var cleanup func()

	if sc.LogFile != "" {
		if dir := filepath.Dir(sc.LogFile); dir != "" && dir != "." {
			if err := os.MkdirAll(dir, 0o755); err != nil {
				return nil, nil, fmt.Errorf("creating log directory %s: %w", dir, err)
			}
		}
		f, err := os.OpenFile(sc.LogFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
		if err != nil {
			return nil, nil, fmt.Errorf("opening log file %s: %w", sc.LogFile, err)
		}
		w = io.MultiWriter(os.Stderr, f)
		cleanup = func() { f.Close() }
	}

	return slog.New(slog.NewTextHandler(w, opts)), cleanup, nil
}
