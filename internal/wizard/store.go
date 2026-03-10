package wizard

import (
	"sync"
	"time"
)

const planTTL = 30 * time.Minute

type planEntry struct {
	plan      *Plan
	createdAt time.Time
}

// PlanStore is a thread-safe in-memory store for wizard plans with TTL expiration.
type PlanStore struct {
	mu    sync.RWMutex
	plans map[string]planEntry
}

// NewPlanStore creates a new PlanStore.
func NewPlanStore() *PlanStore {
	return &PlanStore{plans: make(map[string]planEntry)}
}

// Put stores a plan.
func (s *PlanStore) Put(plan *Plan) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.plans[plan.ID] = planEntry{plan: plan, createdAt: time.Now()}
	s.evictExpired()
}

// Get retrieves a plan by ID. Returns nil, false if not found or expired.
func (s *PlanStore) Get(id string) (*Plan, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	entry, ok := s.plans[id]
	if !ok || time.Since(entry.createdAt) > planTTL {
		return nil, false
	}
	return entry.plan, true
}

// Delete removes a plan by ID.
func (s *PlanStore) Delete(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.plans, id)
}

// evictExpired removes plans older than TTL. Must be called under write lock.
func (s *PlanStore) evictExpired() {
	for id, entry := range s.plans {
		if time.Since(entry.createdAt) > planTTL {
			delete(s.plans, id)
		}
	}
}
