package api

import (
	"net/http"
)

// handleListMCPServers returns all configured MCP servers with their status.
// GET /api/v1/mcp-servers
func (s *Server) handleListMCPServers(w http.ResponseWriter, r *http.Request) {
	statuses := s.mcpMgr.Status()
	writeJSON(w, http.StatusOK, statuses)
}

// handleStartMCPServer starts a named MCP server.
// POST /api/v1/mcp-servers/{name}/start
func (s *Server) handleStartMCPServer(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	if err := s.mcpMgr.Start(name); err != nil {
		s.logger.Error("failed to start MCP server", "name", name, "error", err)
		writeError(w, http.StatusInternalServerError, "failed to start MCP server: "+err.Error())
		return
	}

	s.logger.Info("MCP server started", "name", name)
	writeJSON(w, http.StatusOK, map[string]string{
		"status":  "started",
		"server":  name,
	})
}

// handleStopMCPServer stops a named MCP server.
// POST /api/v1/mcp-servers/{name}/stop
func (s *Server) handleStopMCPServer(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	if err := s.mcpMgr.Stop(name); err != nil {
		s.logger.Error("failed to stop MCP server", "name", name, "error", err)
		writeError(w, http.StatusInternalServerError, "failed to stop MCP server: "+err.Error())
		return
	}

	s.logger.Info("MCP server stopped", "name", name)
	writeJSON(w, http.StatusOK, map[string]string{
		"status":  "stopped",
		"server":  name,
	})
}
