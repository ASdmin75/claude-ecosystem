package api

import (
	"context"
	"io/fs"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/asdmin/claude-ecosystem/internal/auth"
	"github.com/asdmin/claude-ecosystem/internal/config"
	"github.com/asdmin/claude-ecosystem/internal/domain"
	"github.com/asdmin/claude-ecosystem/internal/events"
	"github.com/asdmin/claude-ecosystem/internal/mcpmanager"
	"github.com/asdmin/claude-ecosystem/internal/runguard"
	"github.com/asdmin/claude-ecosystem/internal/store"
	"github.com/asdmin/claude-ecosystem/internal/subagent"
	"github.com/asdmin/claude-ecosystem/internal/task"
	"github.com/asdmin/claude-ecosystem/internal/ui"
	"github.com/asdmin/claude-ecosystem/internal/wizard"
)

// Server holds all dependencies required by the REST API handlers.
type Server struct {
	cfg         *config.Config
	taskRunner  *task.Runner
	subagentMgr *subagent.Manager
	mcpMgr      *mcpmanager.Manager
	domainMgr   *domain.Manager
	store       store.ExecutionStore
	authStore   store.AuthStore
	authMw      *auth.Middleware
	paseto      *auth.PASETOManager
	bus         *events.Bus
	logger      *slog.Logger
	cancels     sync.Map // map[executionID]context.CancelFunc
	guard       *runguard.Guard
	wizardGen   *wizard.Generator
	wizardStore *wizard.PlanStore
}

// NewServer creates a new Server with all required dependencies.
func NewServer(
	cfg *config.Config,
	taskRunner *task.Runner,
	subagentMgr *subagent.Manager,
	mcpMgr *mcpmanager.Manager,
	domainMgr *domain.Manager,
	execStore store.ExecutionStore,
	authStore store.AuthStore,
	authMw *auth.Middleware,
	paseto *auth.PASETOManager,
	bus *events.Bus,
	guard *runguard.Guard,
	logger *slog.Logger,
) *Server {
	s := &Server{
		cfg:         cfg,
		taskRunner:  taskRunner,
		subagentMgr: subagentMgr,
		mcpMgr:      mcpMgr,
		domainMgr:   domainMgr,
		store:       execStore,
		authStore:   authStore,
		authMw:      authMw,
		paseto:      paseto,
		bus:         bus,
		guard:       guard,
		logger:      logger,
	}
	s.wizardGen = wizard.NewGenerator(taskRunner, logger)
	s.wizardStore = wizard.NewPlanStore()
	s.cleanupStaleExecutions()
	return s
}

// cleanupStaleExecutions marks any "running" executions from a previous server
// instance as failed, since their processes are no longer tracked.
func (s *Server) cleanupStaleExecutions() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	execs, err := s.store.ListExecutions(ctx, store.ExecutionFilter{Status: "running", Limit: 100})
	if err != nil {
		s.logger.Error("failed to list stale executions", "error", err)
		return
	}

	now := time.Now().UTC()
	for i := range execs {
		execs[i].Status = "failed"
		execs[i].Error = "server restarted while task was running"
		execs[i].CompletedAt = &now
		if err := s.store.UpdateExecution(ctx, &execs[i]); err != nil {
			s.logger.Error("failed to clean up stale execution", "id", execs[i].ID, "error", err)
			continue
		}
		s.logger.Info("cleaned up stale execution", "id", execs[i].ID, "task", execs[i].TaskName)
	}
}

// Handler builds and returns the top-level http.Handler with all routes registered.
// Public routes (auth) are mounted without authentication.
// All other /api/v1/* routes are wrapped with the auth middleware.
// A catch-all GET / serves static files for a React SPA.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	// --- Public auth routes (no middleware) ---
	mux.HandleFunc("POST /api/v1/auth/login", s.handleLogin)
	mux.HandleFunc("POST /api/v1/auth/refresh", s.withAuth(s.handleRefresh))

	// --- Protected API routes ---
	// Tasks
	mux.HandleFunc("GET /api/v1/tasks", s.withAuth(s.handleListTasks))
	mux.HandleFunc("POST /api/v1/tasks", s.withAuth(s.handleCreateTask))
	mux.HandleFunc("GET /api/v1/tasks/{name}", s.withAuth(s.handleGetTask))
	mux.HandleFunc("PUT /api/v1/tasks/{name}", s.withAuth(s.handleUpdateTask))
	mux.HandleFunc("POST /api/v1/tasks/{name}/run", s.withAuth(s.handleRunTask))
	mux.HandleFunc("POST /api/v1/tasks/{name}/run-async", s.withAuth(s.handleRunTaskAsync))
	mux.HandleFunc("GET /api/v1/tasks/{name}/stream", s.withAuth(s.handleTaskStream))

	// Sub-agents
	mux.HandleFunc("GET /api/v1/subagents", s.withAuth(s.handleListSubAgents))
	mux.HandleFunc("GET /api/v1/subagents/{name}", s.withAuth(s.handleGetSubAgent))
	mux.HandleFunc("POST /api/v1/subagents", s.withAuth(s.handleCreateSubAgent))
	mux.HandleFunc("PUT /api/v1/subagents/{name}", s.withAuth(s.handleUpdateSubAgent))
	mux.HandleFunc("DELETE /api/v1/subagents/{name}", s.withAuth(s.handleDeleteSubAgent))

	// Pipelines
	mux.HandleFunc("GET /api/v1/pipelines", s.withAuth(s.handleListPipelines))
	mux.HandleFunc("POST /api/v1/pipelines", s.withAuth(s.handleCreatePipeline))
	mux.HandleFunc("GET /api/v1/pipelines/{name}", s.withAuth(s.handleGetPipeline))
	mux.HandleFunc("PUT /api/v1/pipelines/{name}", s.withAuth(s.handleUpdatePipeline))
	mux.HandleFunc("DELETE /api/v1/pipelines/{name}", s.withAuth(s.handleDeletePipeline))
	mux.HandleFunc("POST /api/v1/pipelines/{name}/run", s.withAuth(s.handleRunPipeline))
	mux.HandleFunc("POST /api/v1/pipelines/{name}/run-async", s.withAuth(s.handleRunPipelineAsync))

	// Executions
	mux.HandleFunc("GET /api/v1/executions", s.withAuth(s.handleListExecutions))
	mux.HandleFunc("GET /api/v1/executions/{id}", s.withAuth(s.handleGetExecution))
	mux.HandleFunc("DELETE /api/v1/executions/{id}", s.withAuth(s.handleDeleteExecution))
	mux.HandleFunc("GET /api/v1/executions/{id}/stream", s.withAuth(s.handleExecutionStream))
	mux.HandleFunc("POST /api/v1/executions/{id}/cancel", s.withAuth(s.handleCancelExecution))

	// MCP Servers
	mux.HandleFunc("GET /api/v1/mcp-servers", s.withAuth(s.handleListMCPServers))
	mux.HandleFunc("POST /api/v1/mcp-servers/{name}/start", s.withAuth(s.handleStartMCPServer))
	mux.HandleFunc("POST /api/v1/mcp-servers/{name}/stop", s.withAuth(s.handleStopMCPServer))

	// Global SSE event stream
	mux.HandleFunc("GET /api/v1/events", s.withAuth(s.handleEvents))

	// Wizard
	mux.HandleFunc("POST /api/v1/wizard/generate", s.withAuth(s.handleWizardGenerate))
	mux.HandleFunc("GET /api/v1/wizard/plans/{id}", s.withAuth(s.handleWizardGetPlan))
	mux.HandleFunc("PUT /api/v1/wizard/plans/{id}", s.withAuth(s.handleWizardUpdatePlan))
	mux.HandleFunc("POST /api/v1/wizard/plans/{id}/apply", s.withAuth(s.handleWizardApply))
	mux.HandleFunc("DELETE /api/v1/wizard/plans/{id}", s.withAuth(s.handleWizardDiscard))

	// Dashboard
	mux.HandleFunc("GET /api/v1/dashboard", s.withAuth(s.handleDashboard))

	// --- Static file serving for React SPA ---
	mux.HandleFunc("GET /", s.handleSPA)

	return s.requestLogger(mux)
}

// statusWriter wraps http.ResponseWriter to capture the status code.
type statusWriter struct {
	http.ResponseWriter
	status int
}

func (sw *statusWriter) WriteHeader(code int) {
	sw.status = code
	sw.ResponseWriter.WriteHeader(code)
}

// requestLogger returns middleware that logs each HTTP request.
func (s *Server) requestLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(sw, r)
		s.logger.Info("http request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", sw.status,
			"duration", time.Since(start),
		)
	})
}

// withAuth wraps a handler function with the auth middleware.
func (s *Server) withAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		s.authMw.Handler(next).ServeHTTP(w, r)
	}
}

// handleSPA serves static files for the React SPA from the embedded filesystem.
// For any path that does not match a known file, it falls back to index.html
// so that client-side routing works.
func (s *Server) handleSPA(w http.ResponseWriter, r *http.Request) {
	if strings.HasPrefix(r.URL.Path, "/api/") {
		http.NotFound(w, r)
		return
	}

	distFS, err := ui.DistFS()
	if err != nil {
		http.Error(w, "UI not built", http.StatusInternalServerError)
		return
	}

	// Try to serve the requested file
	path := r.URL.Path
	if path == "/" {
		path = "index.html"
	} else {
		path = strings.TrimPrefix(path, "/")
	}

	if _, err := fs.Stat(distFS, path); err != nil {
		// Fall back to index.html for client-side routing
		path = "index.html"
	}

	http.ServeFileFS(w, r, distFS, path)
}

// findTask looks up a task by name in the config.
func (s *Server) findTask(name string) *config.Task {
	for i := range s.cfg.Tasks {
		if s.cfg.Tasks[i].Name == name {
			return &s.cfg.Tasks[i]
		}
	}
	return nil
}

// findPipeline looks up a pipeline by name in the config.
func (s *Server) findPipeline(name string) *config.Pipeline {
	for i := range s.cfg.Pipelines {
		if s.cfg.Pipelines[i].Name == name {
			return &s.cfg.Pipelines[i]
		}
	}
	return nil
}
