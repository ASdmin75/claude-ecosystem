package runguard

import "sync"

// Guard tracks which tasks and pipelines are currently running
// and prevents concurrent execution when configured.
type Guard struct {
	mu      sync.Mutex
	running map[string]bool
}

// New creates a new Guard.
func New() *Guard {
	return &Guard{running: make(map[string]bool)}
}

// TryAcquire attempts to mark a name as running.
// Returns true if acquired (not already running), false if already running.
func (g *Guard) TryAcquire(name string) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.running[name] {
		return false
	}
	g.running[name] = true
	return true
}

// Release marks a name as no longer running.
func (g *Guard) Release(name string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	delete(g.running, name)
}
