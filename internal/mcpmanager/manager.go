package mcpmanager

import (
	"fmt"
	"log/slog"
	"os/exec"
	"sync"
	"syscall"
	"time"

	"github.com/asdmin/claude-ecosystem/internal/config"
)

// Manager manages the lifecycle of MCP servers defined in config.
type Manager struct {
	mu      sync.RWMutex
	servers map[string]*managedServer
	logger  *slog.Logger
}

// New creates a Manager from the given MCP server configurations.
func New(configs []config.MCPServerConfig, logger *slog.Logger) *Manager {
	servers := make(map[string]*managedServer, len(configs))
	for _, c := range configs {
		servers[c.Name] = &managedServer{
			config: c,
		}
	}
	return &Manager{
		servers: servers,
		logger:  logger,
	}
}

// Start starts an MCP server by name (lazy start). No-op if already running.
func (m *Manager) Start(name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	srv, ok := m.servers[name]
	if !ok {
		return fmt.Errorf("unknown MCP server: %s", name)
	}
	if srv.running {
		return nil
	}

	cmd := exec.Command(srv.config.Command, srv.config.Args...)
	for k, v := range srv.config.Env {
		cmd.Env = append(cmd.Env, k+"="+v)
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("starting MCP server %s: %w", name, err)
	}

	srv.cmd = cmd
	srv.running = true
	m.logger.Info("started MCP server", "name", name, "pid", cmd.Process.Pid)

	return nil
}

// Stop stops an MCP server by name. Sends SIGTERM, waits 5s, then SIGKILL.
func (m *Manager) Stop(name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	return m.stopLocked(name)
}

// stopLocked stops a server while the lock is already held.
func (m *Manager) stopLocked(name string) error {
	srv, ok := m.servers[name]
	if !ok {
		return fmt.Errorf("unknown MCP server: %s", name)
	}
	if !srv.running {
		return nil
	}

	m.logger.Info("stopping MCP server", "name", name, "pid", srv.cmd.Process.Pid)

	// Send SIGTERM.
	if err := srv.cmd.Process.Signal(syscall.SIGTERM); err != nil {
		m.logger.Warn("failed to send SIGTERM", "name", name, "error", err)
	}

	// Wait for exit with a 5-second timeout; SIGKILL if it doesn't exit.
	done := make(chan error, 1)
	go func() {
		done <- srv.cmd.Wait()
	}()

	select {
	case <-done:
		// Process exited gracefully.
	case <-time.After(5 * time.Second):
		m.logger.Warn("MCP server did not exit after SIGTERM, sending SIGKILL", "name", name)
		if err := srv.cmd.Process.Signal(syscall.SIGKILL); err != nil {
			m.logger.Warn("failed to send SIGKILL", "name", name, "error", err)
		}
		<-done
	}

	srv.running = false
	srv.cmd = nil
	m.logger.Info("stopped MCP server", "name", name)

	return nil
}

// StopAll stops all running MCP servers. Called on app shutdown.
func (m *Manager) StopAll() {
	m.mu.Lock()
	defer m.mu.Unlock()

	for name, srv := range m.servers {
		if srv.running {
			if err := m.stopLocked(name); err != nil {
				m.logger.Warn("error stopping MCP server", "name", name, "error", err)
			}
		}
	}
}

// Status returns the status of all configured MCP servers.
func (m *Manager) Status() []ServerStatus {
	m.mu.RLock()
	defer m.mu.RUnlock()

	statuses := make([]ServerStatus, 0, len(m.servers))
	for name, srv := range m.servers {
		s := ServerStatus{
			Name:    name,
			Running: srv.running,
		}
		if srv.running && srv.cmd != nil && srv.cmd.Process != nil {
			s.PID = srv.cmd.Process.Pid
		}
		statuses = append(statuses, s)
	}
	return statuses
}

// EnsureRunning starts all servers in the given list if not already running.
func (m *Manager) EnsureRunning(names []string) error {
	for _, name := range names {
		if err := m.Start(name); err != nil {
			return err
		}
	}
	return nil
}
