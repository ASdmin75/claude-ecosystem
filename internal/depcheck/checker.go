package depcheck

import (
	"slices"

	"github.com/asdmin/claude-ecosystem/internal/config"
)

// EntityType identifies the kind of entity being analyzed.
type EntityType string

const (
	EntityTask     EntityType = "task"
	EntityPipeline EntityType = "pipeline"
	EntitySubAgent EntityType = "subagent"
)

// Dependency represents a reference to a named entity.
type Dependency struct {
	Type EntityType `json:"type"`
	Name string     `json:"name"`
}

// DeleteAnalysis describes what happens when an entity is deleted.
type DeleteAnalysis struct {
	Entity       Dependency   `json:"entity"`
	UsedBy       []Dependency `json:"used_by"`
	CanDelete    bool         `json:"can_delete"`
	CascadeItems []Dependency `json:"cascade_items"`
	Blocked      bool         `json:"blocked"`
	BlockReason  string       `json:"block_reason,omitempty"`
}

// AnalyzeTaskDelete checks whether a task can be safely deleted.
// A task is blocked if it is referenced by any pipeline step.
func AnalyzeTaskDelete(cfg *config.Config, taskName string) DeleteAnalysis {
	a := DeleteAnalysis{
		Entity: Dependency{Type: EntityTask, Name: taskName},
	}

	usedBy := findPipelinesUsingTask(cfg, taskName)
	a.UsedBy = usedBy

	if len(usedBy) > 0 {
		a.Blocked = true
		a.CanDelete = false
		if len(usedBy) == 1 {
			a.BlockReason = "Task is used by pipeline \"" + usedBy[0].Name + "\". Delete the pipeline first or remove this task from it."
		} else {
			a.BlockReason = "Task is used by " + itoa(len(usedBy)) + " pipelines. Remove it from all pipelines before deleting."
		}
		return a
	}

	a.CanDelete = true
	return a
}

// AnalyzePipelineDelete checks what gets cascade-deleted when a pipeline is removed.
// Tasks that are exclusive to this pipeline become cascade items.
// Agents that are only used by cascade tasks also become cascade items.
func AnalyzePipelineDelete(cfg *config.Config, pipelineName string) DeleteAnalysis {
	a := DeleteAnalysis{
		Entity:    Dependency{Type: EntityPipeline, Name: pipelineName},
		CanDelete: true,
	}

	p := findPipeline(cfg, pipelineName)
	if p == nil {
		return a
	}

	// Collect task names used by this pipeline.
	pipelineTaskNames := make(map[string]bool)
	for _, step := range p.Steps {
		pipelineTaskNames[step.Task] = true
	}

	// Find tasks exclusive to this pipeline.
	var cascadeTasks []string
	for taskName := range pipelineTaskNames {
		if isTaskExclusiveToPipeline(cfg, taskName, pipelineName) {
			cascadeTasks = append(cascadeTasks, taskName)
			a.CascadeItems = append(a.CascadeItems, Dependency{Type: EntityTask, Name: taskName})
		}
	}

	// Find agents exclusive to cascade tasks.
	cascadeTaskSet := make(map[string]bool)
	for _, t := range cascadeTasks {
		cascadeTaskSet[t] = true
	}
	checkedAgents := make(map[string]bool)
	for _, taskName := range cascadeTasks {
		task := findTask(cfg, taskName)
		if task == nil {
			continue
		}
		for _, agentName := range task.Agents {
			if checkedAgents[agentName] {
				continue
			}
			checkedAgents[agentName] = true
			if isAgentExclusiveToTasks(cfg, agentName, cascadeTaskSet) {
				a.CascadeItems = append(a.CascadeItems, Dependency{Type: EntitySubAgent, Name: agentName})
			}
		}
	}

	return a
}

// AnalyzeSubAgentDelete checks whether a sub-agent can be safely deleted.
// A sub-agent is blocked if any task references it in its agents list.
func AnalyzeSubAgentDelete(cfg *config.Config, agentName string) DeleteAnalysis {
	a := DeleteAnalysis{
		Entity: Dependency{Type: EntitySubAgent, Name: agentName},
	}

	usedBy := findTasksUsingAgent(cfg, agentName)
	a.UsedBy = usedBy

	if len(usedBy) > 0 {
		a.Blocked = true
		a.CanDelete = false
		if len(usedBy) == 1 {
			a.BlockReason = "Agent is used by task \"" + usedBy[0].Name + "\". Remove it from the task before deleting."
		} else {
			a.BlockReason = "Agent is used by " + itoa(len(usedBy)) + " tasks. Remove it from all tasks before deleting."
		}
		return a
	}

	a.CanDelete = true
	return a
}

// findPipelinesUsingTask returns all pipelines that reference the given task.
func findPipelinesUsingTask(cfg *config.Config, taskName string) []Dependency {
	var deps []Dependency
	for _, p := range cfg.Pipelines {
		if slices.ContainsFunc(p.Steps, func(s config.PipelineStep) bool { return s.Task == taskName }) {
			deps = append(deps, Dependency{Type: EntityPipeline, Name: p.Name})
		}
	}
	return deps
}

// findTasksUsingAgent returns all tasks that reference the given agent.
func findTasksUsingAgent(cfg *config.Config, agentName string) []Dependency {
	var deps []Dependency
	for _, t := range cfg.Tasks {
		if slices.Contains(t.Agents, agentName) {
			deps = append(deps, Dependency{Type: EntityTask, Name: t.Name})
		}
	}
	return deps
}

// isTaskExclusiveToPipeline returns true if the task is not used by any pipeline
// other than the specified one.
func isTaskExclusiveToPipeline(cfg *config.Config, taskName, pipelineName string) bool {
	for _, p := range cfg.Pipelines {
		if p.Name == pipelineName {
			continue
		}
		if slices.ContainsFunc(p.Steps, func(s config.PipelineStep) bool { return s.Task == taskName }) {
			return false
		}
	}
	return true
}

// isAgentExclusiveToTasks returns true if the agent is only used by tasks in the given set.
func isAgentExclusiveToTasks(cfg *config.Config, agentName string, taskSet map[string]bool) bool {
	for _, t := range cfg.Tasks {
		if taskSet[t.Name] {
			continue
		}
		if slices.Contains(t.Agents, agentName) {
			return false
		}
	}
	return true
}

func findPipeline(cfg *config.Config, name string) *config.Pipeline {
	for i := range cfg.Pipelines {
		if cfg.Pipelines[i].Name == name {
			return &cfg.Pipelines[i]
		}
	}
	return nil
}

func findTask(cfg *config.Config, name string) *config.Task {
	for i := range cfg.Tasks {
		if cfg.Tasks[i].Name == name {
			return &cfg.Tasks[i]
		}
	}
	return nil
}

func itoa(n int) string {
	if n < 10 {
		return string(rune('0' + n))
	}
	return itoa(n/10) + string(rune('0'+n%10))
}
