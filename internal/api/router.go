package api

import (
	"io/fs"
	"log/slog"
	"net/http"
	"strings"
	"sync"

	"github.com/asdmin/claude-ecosystem/internal/auth"
	"github.com/asdmin/claude-ecosystem/internal/config"
	"github.com/asdmin/claude-ecosystem/internal/events"
	"github.com/asdmin/claude-ecosystem/internal/mcpmanager"
	"github.com/asdmin/claude-ecosystem/internal/store"
	"github.com/asdmin/claude-ecosystem/internal/subagent"
	"github.com/asdmin/claude-ecosystem/internal/task"
	"github.com/asdmin/claude-ecosystem/internal/ui"
)

// Server holds all dependencies required by the REST API handlers.
type Server struct {
	cfg         *config.Config
	taskRunner  *task.Runner
	subagentMgr *subagent.Manager
	mcpMgr      *mcpmanager.Manager
	store       store.ExecutionStore
	authStore   store.AuthStore
	authMw      *auth.Middleware
	paseto      *auth.PASETOManager
	bus         *events.Bus
	logger      *slog.Logger
	cancels     sync.Map // map[executionID]context.CancelFunc
}

// NewServer creates a new Server with all required dependencies.
func NewServer(
	cfg *config.Config,
	taskRunner *task.Runner,
	subagentMgr *subagent.Manager,
	mcpMgr *mcpmanager.Manager,
	execStore store.ExecutionStore,
	authStore store.AuthStore,
	authMw *auth.Middleware,
	paseto *auth.PASETOManager,
	bus *events.Bus,
	logger *slog.Logger,
) *Server {
	return &Server{
		cfg:         cfg,
		taskRunner:  taskRunner,
		subagentMgr: subagentMgr,
		mcpMgr:      mcpMgr,
		store:       execStore,
		authStore:   authStore,
		authMw:      authMw,
		paseto:      paseto,
		bus:         bus,
		logger:      logger,
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
	mux.HandleFunc("GET /api/v1/executions/{id}/stream", s.withAuth(s.handleExecutionStream))
	mux.HandleFunc("POST /api/v1/executions/{id}/cancel", s.withAuth(s.handleCancelExecution))

	// MCP Servers
	mux.HandleFunc("GET /api/v1/mcp-servers", s.withAuth(s.handleListMCPServers))
	mux.HandleFunc("POST /api/v1/mcp-servers/{name}/start", s.withAuth(s.handleStartMCPServer))
	mux.HandleFunc("POST /api/v1/mcp-servers/{name}/stop", s.withAuth(s.handleStopMCPServer))

	// Dashboard
	mux.HandleFunc("GET /api/v1/dashboard", s.withAuth(s.handleDashboard))

	// --- Static file serving for React SPA ---
	mux.HandleFunc("GET /", s.handleSPA)

	return mux
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
