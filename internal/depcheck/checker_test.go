package depcheck

import (
	"testing"

	"github.com/asdmin/claude-ecosystem/internal/config"
)

func TestAnalyzeTaskDelete_NoRefs(t *testing.T) {
	cfg := &config.Config{
		Tasks:     []config.Task{{Name: "standalone"}},
		Pipelines: []config.Pipeline{},
	}
	a := AnalyzeTaskDelete(cfg, "standalone")
	if !a.CanDelete {
		t.Fatal("expected CanDelete=true for unreferenced task")
	}
	if a.Blocked {
		t.Fatal("expected Blocked=false")
	}
}

func TestAnalyzeTaskDelete_UsedByPipeline(t *testing.T) {
	cfg := &config.Config{
		Tasks: []config.Task{{Name: "t1"}},
		Pipelines: []config.Pipeline{
			{Name: "p1", Steps: []config.PipelineStep{{Task: "t1"}}},
		},
	}
	a := AnalyzeTaskDelete(cfg, "t1")
	if a.CanDelete {
		t.Fatal("expected CanDelete=false for task used by pipeline")
	}
	if !a.Blocked {
		t.Fatal("expected Blocked=true")
	}
	if len(a.UsedBy) != 1 || a.UsedBy[0].Name != "p1" {
		t.Fatalf("expected UsedBy=[p1], got %v", a.UsedBy)
	}
}

func TestAnalyzeTaskDelete_UsedByMultiplePipelines(t *testing.T) {
	cfg := &config.Config{
		Tasks: []config.Task{{Name: "shared"}},
		Pipelines: []config.Pipeline{
			{Name: "p1", Steps: []config.PipelineStep{{Task: "shared"}}},
			{Name: "p2", Steps: []config.PipelineStep{{Task: "shared"}}},
		},
	}
	a := AnalyzeTaskDelete(cfg, "shared")
	if a.CanDelete {
		t.Fatal("expected blocked")
	}
	if len(a.UsedBy) != 2 {
		t.Fatalf("expected 2 usedBy, got %d", len(a.UsedBy))
	}
}

func TestAnalyzePipelineDelete_CascadeExclusiveTasks(t *testing.T) {
	cfg := &config.Config{
		Tasks: []config.Task{
			{Name: "exclusive"},
			{Name: "shared"},
		},
		Pipelines: []config.Pipeline{
			{Name: "p1", Steps: []config.PipelineStep{{Task: "exclusive"}, {Task: "shared"}}},
			{Name: "p2", Steps: []config.PipelineStep{{Task: "shared"}}},
		},
	}
	a := AnalyzePipelineDelete(cfg, "p1")
	if !a.CanDelete {
		t.Fatal("pipelines should always be deletable")
	}

	var foundExclusive, foundShared bool
	for _, ci := range a.CascadeItems {
		if ci.Name == "exclusive" {
			foundExclusive = true
		}
		if ci.Name == "shared" {
			foundShared = true
		}
	}
	if !foundExclusive {
		t.Fatal("expected 'exclusive' in cascade items")
	}
	if foundShared {
		t.Fatal("'shared' should NOT be in cascade items (used by p2)")
	}
}

func TestAnalyzePipelineDelete_CascadeAgents(t *testing.T) {
	cfg := &config.Config{
		Tasks: []config.Task{
			{Name: "t1", Agents: []string{"agent-exclusive", "agent-shared"}},
			{Name: "t2", Agents: []string{"agent-shared"}},
		},
		Pipelines: []config.Pipeline{
			{Name: "p1", Steps: []config.PipelineStep{{Task: "t1"}}},
		},
	}
	a := AnalyzePipelineDelete(cfg, "p1")

	cascadeNames := make(map[string]bool)
	for _, ci := range a.CascadeItems {
		cascadeNames[ci.Name] = true
	}
	if !cascadeNames["t1"] {
		t.Fatal("expected t1 in cascade")
	}
	if !cascadeNames["agent-exclusive"] {
		t.Fatal("expected agent-exclusive in cascade")
	}
	if cascadeNames["agent-shared"] {
		t.Fatal("agent-shared should NOT be in cascade (used by t2)")
	}
}

func TestAnalyzeSubAgentDelete_NoRefs(t *testing.T) {
	cfg := &config.Config{
		Tasks: []config.Task{{Name: "t1"}},
	}
	a := AnalyzeSubAgentDelete(cfg, "unused-agent")
	if !a.CanDelete {
		t.Fatal("expected CanDelete=true")
	}
}

func TestAnalyzeSubAgentDelete_UsedByTask(t *testing.T) {
	cfg := &config.Config{
		Tasks: []config.Task{{Name: "t1", Agents: []string{"my-agent"}}},
	}
	a := AnalyzeSubAgentDelete(cfg, "my-agent")
	if a.CanDelete {
		t.Fatal("expected CanDelete=false")
	}
	if !a.Blocked {
		t.Fatal("expected Blocked=true")
	}
	if len(a.UsedBy) != 1 || a.UsedBy[0].Name != "t1" {
		t.Fatalf("expected UsedBy=[t1], got %v", a.UsedBy)
	}
}

func TestAnalyzeSubAgentDelete_UsedByMultipleTasks(t *testing.T) {
	cfg := &config.Config{
		Tasks: []config.Task{
			{Name: "t1", Agents: []string{"shared-agent"}},
			{Name: "t2", Agents: []string{"shared-agent", "other"}},
		},
	}
	a := AnalyzeSubAgentDelete(cfg, "shared-agent")
	if a.CanDelete {
		t.Fatal("expected blocked")
	}
	if len(a.UsedBy) != 2 {
		t.Fatalf("expected 2 usedBy, got %d", len(a.UsedBy))
	}
}

func TestAnalyzePipelineDelete_NoCascadeWhenAllShared(t *testing.T) {
	cfg := &config.Config{
		Tasks: []config.Task{
			{Name: "t1"},
			{Name: "t2"},
		},
		Pipelines: []config.Pipeline{
			{Name: "p1", Steps: []config.PipelineStep{{Task: "t1"}, {Task: "t2"}}},
			{Name: "p2", Steps: []config.PipelineStep{{Task: "t1"}, {Task: "t2"}}},
		},
	}
	a := AnalyzePipelineDelete(cfg, "p1")
	if !a.CanDelete {
		t.Fatal("expected CanDelete=true")
	}
	if len(a.CascadeItems) != 0 {
		t.Fatalf("expected no cascade items when all tasks are shared, got %d", len(a.CascadeItems))
	}
}

func TestAnalyzePipelineDelete_NotFound(t *testing.T) {
	cfg := &config.Config{}
	a := AnalyzePipelineDelete(cfg, "nonexistent")
	if !a.CanDelete {
		t.Fatal("expected CanDelete=true for nonexistent pipeline")
	}
}

func TestAnalyzeTaskDelete_EntityFields(t *testing.T) {
	cfg := &config.Config{
		Tasks: []config.Task{{Name: "t1"}},
	}
	a := AnalyzeTaskDelete(cfg, "t1")
	if a.Entity.Type != EntityTask {
		t.Errorf("entity type: got %q, want %q", a.Entity.Type, EntityTask)
	}
	if a.Entity.Name != "t1" {
		t.Errorf("entity name: got %q, want %q", a.Entity.Name, "t1")
	}
}

func TestAnalyzePipelineDelete_CascadeAgentsFromMultipleExclusiveTasks(t *testing.T) {
	cfg := &config.Config{
		Tasks: []config.Task{
			{Name: "t1", Agents: []string{"a1"}},
			{Name: "t2", Agents: []string{"a2"}},
			{Name: "t3", Agents: []string{"a1"}}, // a1 used by t3 too (not exclusive)
		},
		Pipelines: []config.Pipeline{
			{Name: "p1", Steps: []config.PipelineStep{{Task: "t1"}, {Task: "t2"}}},
		},
	}
	a := AnalyzePipelineDelete(cfg, "p1")

	cascadeNames := make(map[string]bool)
	for _, ci := range a.CascadeItems {
		cascadeNames[ci.Name] = true
	}
	// t1, t2 are exclusive to p1
	if !cascadeNames["t1"] {
		t.Fatal("expected t1 in cascade")
	}
	if !cascadeNames["t2"] {
		t.Fatal("expected t2 in cascade")
	}
	// a1 is used by t3 (not being cascaded), so NOT exclusive
	if cascadeNames["a1"] {
		t.Fatal("a1 should NOT be in cascade (used by t3 which is not being deleted)")
	}
	// a2 is only used by t2 (which is being cascaded), so it IS exclusive
	if !cascadeNames["a2"] {
		t.Fatal("expected a2 in cascade")
	}
}

func TestAnalyzeTaskDelete_BlockReasonSingle(t *testing.T) {
	cfg := &config.Config{
		Tasks: []config.Task{{Name: "t1"}},
		Pipelines: []config.Pipeline{
			{Name: "p1", Steps: []config.PipelineStep{{Task: "t1"}}},
		},
	}
	a := AnalyzeTaskDelete(cfg, "t1")
	if a.BlockReason == "" {
		t.Fatal("expected non-empty BlockReason")
	}
}

func TestAnalyzeSubAgentDelete_BlockReasonSingle(t *testing.T) {
	cfg := &config.Config{
		Tasks: []config.Task{{Name: "t1", Agents: []string{"a1"}}},
	}
	a := AnalyzeSubAgentDelete(cfg, "a1")
	if a.BlockReason == "" {
		t.Fatal("expected non-empty BlockReason")
	}
}
