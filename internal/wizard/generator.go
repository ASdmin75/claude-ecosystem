package wizard

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"github.com/asdmin/claude-ecosystem/internal/config"
	"github.com/asdmin/claude-ecosystem/internal/subagent"
	"github.com/asdmin/claude-ecosystem/internal/task"
	"github.com/google/uuid"
)

// Generator uses the task runner to invoke Claude for plan generation.
type Generator struct {
	runner      *task.Runner
	subagentMgr *subagent.Manager
	logger      *slog.Logger
}

// NewGenerator creates a new Generator.
func NewGenerator(runner *task.Runner, subagentMgr *subagent.Manager, logger *slog.Logger) *Generator {
	return &Generator{runner: runner, subagentMgr: subagentMgr, logger: logger}
}

// Generate runs Claude with the wizard prompt and parses the output into a Plan.
func (g *Generator) Generate(ctx context.Context, req GenerateRequest, cfg *config.Config) (*Plan, error) {
	var agents []subagent.SubAgent
	if g.subagentMgr != nil {
		var err error
		agents, err = g.subagentMgr.List()
		if err != nil {
			g.logger.Warn("wizard: failed to list agents, proceeding without", "error", err)
		}
	}
	prompt := buildWizardPrompt(req, cfg, agents)

	syntheticTask := config.Task{
		Name:           "_wizard-generate",
		Prompt:         prompt,
		Model:          "sonnet",
		PermissionMode: "plan",
		Timeout:        "5m",
		MaxTurns:       2,
		OutputFormat:    "json",
		DisallowedTools: []string{
			"Read", "Edit", "Write", "Bash", "Glob", "Grep",
			"Agent", "NotebookEdit", "WebFetch", "WebSearch",
		},
	}

	if req.WorkDir != "" {
		syntheticTask.WorkDir = req.WorkDir
	}

	result := g.runner.Run(ctx, syntheticTask, task.RunOptions{}, nil)

	g.logger.Info("wizard: claude raw output",
		"output_len", len(result.Output),
		"output_preview", truncate(result.Output, 500),
		"error", result.Error,
	)

	if result.Error != "" {
		return nil, &GenerateError{
			Err:       fmt.Errorf("claude generation failed: %s", result.Error),
			RawOutput: result.Output,
		}
	}

	if result.Output == "" {
		return nil, &GenerateError{
			Err:       fmt.Errorf("claude returned empty output"),
			RawOutput: "",
		}
	}

	// Claude may return JSON wrapped in markdown code fences or inside a CLI envelope.
	// Try multiple extraction strategies.
	planJSON := stripCodeFences(result.Output)

	var plan Plan
	if err := json.Unmarshal([]byte(planJSON), &plan); err != nil || plan.Summary == "" {
		// Try extracting from CLI envelope (structured_output or result field)
		extracted := extractPlanJSON(result.Output)
		extracted = stripCodeFences(extracted)
		g.logger.Info("wizard: fallback extraction",
			"extracted_len", len(extracted),
			"extracted_preview", truncate(extracted, 500),
		)
		if err2 := json.Unmarshal([]byte(extracted), &plan); err2 != nil {
			return nil, &GenerateError{
				Err:       fmt.Errorf("parsing plan JSON: %w", err2),
				RawOutput: result.Output,
			}
		}
	}

	g.logger.Info("wizard: parsed plan",
		"summary_len", len(plan.Summary),
		"domains", len(plan.Domains),
		"agents", len(plan.Agents),
		"tasks", len(plan.Tasks),
		"pipelines", len(plan.Pipelines),
	)

	plan.ID = uuid.New().String()
	plan.Description = req.Description
	plan.Status = "draft"

	return &plan, nil
}

// extractPlanJSON unwraps the Claude CLI JSON envelope if present.
// With --json-schema, the CLI puts the output in "structured_output" (not "result").
// Without --json-schema, it's in "result" as a string (possibly with markdown fences).
func extractPlanJSON(output string) string {
	var envelope map[string]json.RawMessage
	if err := json.Unmarshal([]byte(output), &envelope); err != nil {
		return output
	}

	// Case 1: structured_output field (used with --json-schema)
	if raw, ok := envelope["structured_output"]; ok {
		return string(raw)
	}

	// Case 2: result field as string
	if raw, ok := envelope["result"]; ok {
		var str string
		if err := json.Unmarshal(raw, &str); err == nil && str != "" {
			// Strip markdown code fences if present
			str = stripCodeFences(str)
			return str
		}
		// result as object
		return string(raw)
	}

	return output
}

// stripCodeFences removes ```json ... ``` wrapping if present.
func stripCodeFences(s string) string {
	trimmed := strings.TrimSpace(s)
	if strings.HasPrefix(trimmed, "```") {
		// Find end of first line (```json or ```)
		if idx := strings.Index(trimmed, "\n"); idx >= 0 {
			trimmed = trimmed[idx+1:]
		}
		// Remove trailing ```
		if idx := strings.LastIndex(trimmed, "```"); idx >= 0 {
			trimmed = trimmed[:idx]
		}
		return strings.TrimSpace(trimmed)
	}
	return s
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
