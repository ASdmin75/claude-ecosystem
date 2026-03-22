package api

import (
	"testing"

	"github.com/asdmin/claude-ecosystem/internal/config"
)

func TestCleanDomainRefs_Tasks(t *testing.T) {
	s := &Server{cfg: &config.Config{
		Domains: map[string]config.Domain{
			"d1": {Tasks: []string{"a", "b", "c"}, Pipelines: []string{"p1"}, Agents: []string{"ag1"}},
		},
	}}
	s.cleanDomainRefs([]string{"b"}, nil, nil)
	d := s.cfg.Domains["d1"]
	if len(d.Tasks) != 2 || d.Tasks[0] != "a" || d.Tasks[1] != "c" {
		t.Fatalf("expected [a c], got %v", d.Tasks)
	}
}

func TestCleanDomainRefs_Pipelines(t *testing.T) {
	s := &Server{cfg: &config.Config{
		Domains: map[string]config.Domain{
			"d1": {Tasks: []string{"a"}, Pipelines: []string{"p1", "p2"}, Agents: []string{"ag1"}},
		},
	}}
	s.cleanDomainRefs(nil, []string{"p1"}, nil)
	d := s.cfg.Domains["d1"]
	if len(d.Pipelines) != 1 || d.Pipelines[0] != "p2" {
		t.Fatalf("expected [p2], got %v", d.Pipelines)
	}
}

func TestCleanDomainRefs_Agents(t *testing.T) {
	s := &Server{cfg: &config.Config{
		Domains: map[string]config.Domain{
			"d1": {Tasks: []string{"a"}, Pipelines: []string{"p1"}, Agents: []string{"ag1", "ag2"}},
		},
	}}
	s.cleanDomainRefs(nil, nil, []string{"ag1"})
	d := s.cfg.Domains["d1"]
	if len(d.Agents) != 1 || d.Agents[0] != "ag2" {
		t.Fatalf("expected [ag2], got %v", d.Agents)
	}
}

func TestCleanDomainRefs_OrphanRemoval(t *testing.T) {
	s := &Server{cfg: &config.Config{
		Domains: map[string]config.Domain{
			"keep":   {Tasks: []string{"a"}, Agents: []string{"ag1"}},
			"remove": {Tasks: []string{"b"}, Agents: []string{"ag2"}},
		},
	}}
	s.cleanDomainRefs([]string{"b"}, nil, []string{"ag2"})

	if _, ok := s.cfg.Domains["remove"]; ok {
		t.Fatal("expected orphaned domain 'remove' to be deleted")
	}
	if _, ok := s.cfg.Domains["keep"]; !ok {
		t.Fatal("expected domain 'keep' to be preserved")
	}
}

func TestCleanDomainRefs_NoChangeNoOrphan(t *testing.T) {
	s := &Server{cfg: &config.Config{
		Domains: map[string]config.Domain{
			"d1": {Tasks: []string{"a"}, Agents: []string{"ag1"}},
		},
	}}
	// Delete names that don't exist in the domain — should not trigger orphan removal.
	s.cleanDomainRefs([]string{"nonexistent"}, nil, nil)

	if _, ok := s.cfg.Domains["d1"]; !ok {
		t.Fatal("expected domain 'd1' to remain unchanged")
	}
}

func TestCleanDomainRefs_MultiDomain(t *testing.T) {
	s := &Server{cfg: &config.Config{
		Domains: map[string]config.Domain{
			"d1": {Tasks: []string{"shared"}, Agents: []string{"ag1"}},
			"d2": {Tasks: []string{"shared", "other"}, Agents: []string{"ag1"}},
		},
	}}
	s.cleanDomainRefs([]string{"shared"}, nil, []string{"ag1"})

	// d1 had only "shared" task and "ag1" agent → should be removed.
	if _, ok := s.cfg.Domains["d1"]; ok {
		t.Fatal("expected orphaned domain 'd1' to be deleted")
	}
	// d2 still has "other" task → should remain.
	d2 := s.cfg.Domains["d2"]
	if len(d2.Tasks) != 1 || d2.Tasks[0] != "other" {
		t.Fatalf("expected d2.Tasks=[other], got %v", d2.Tasks)
	}
	if len(d2.Agents) != 0 {
		t.Fatalf("expected d2.Agents=[], got %v", d2.Agents)
	}
}

func TestCleanDomainRefs_MCPServers(t *testing.T) {
	s := &Server{cfg: &config.Config{
		Domains: map[string]config.Domain{
			"d1": {Tasks: []string{"t1"}, MCPServers: []string{"db", "excel"}},
		},
	}}
	s.cleanDomainRefs([]string{"t1"}, nil, nil, []string{"db", "excel"})

	// All refs removed → domain should be deleted.
	if _, ok := s.cfg.Domains["d1"]; ok {
		t.Fatal("expected orphaned domain 'd1' to be deleted")
	}
}

func TestCleanDomainRefs_MCPServersPartial(t *testing.T) {
	s := &Server{cfg: &config.Config{
		Domains: map[string]config.Domain{
			"d1": {Tasks: []string{"t1"}, MCPServers: []string{"db", "excel", "telegram"}},
		},
	}}
	// Only some MCP servers cleaned — domain should remain because telegram is still referenced.
	s.cleanDomainRefs([]string{"t1"}, nil, nil, []string{"db", "excel"})

	d, ok := s.cfg.Domains["d1"]
	if !ok {
		t.Fatal("expected domain 'd1' to remain (still has mcp_servers)")
	}
	if len(d.MCPServers) != 1 || d.MCPServers[0] != "telegram" {
		t.Fatalf("expected MCPServers=[telegram], got %v", d.MCPServers)
	}
}

func TestCleanDomainRefs_MCPServersOnlyNoOrphan(t *testing.T) {
	s := &Server{cfg: &config.Config{
		Domains: map[string]config.Domain{
			"d1": {Tasks: []string{"t1"}, MCPServers: []string{"db"}},
		},
	}}
	// Remove task but not MCP server — domain should remain.
	s.cleanDomainRefs([]string{"t1"}, nil, nil)

	d, ok := s.cfg.Domains["d1"]
	if !ok {
		t.Fatal("expected domain 'd1' to remain (still has mcp_servers)")
	}
	if len(d.MCPServers) != 1 {
		t.Fatalf("expected MCPServers=[db], got %v", d.MCPServers)
	}
}
