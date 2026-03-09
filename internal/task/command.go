package task

import (
	"fmt"

	"github.com/asdmin/claude-ecosystem/internal/config"
)

// RunOptions provides runtime overrides and additional parameters for task execution.
type RunOptions struct {
	MCPConfigPath      string // path to generated mcp-config JSON file
	AgentsJSON         string // JSON for --agents flag
	ResumeSessionID    string // for --resume
	OutputFormat       string // override output format ("json" or "stream-json")
	AppendSystemPrompt string // additional system prompt (e.g. from domain doc)
}

// BuildArgs constructs the full claude CLI argument list from a task config.
// It handles: -p, --output-format, --model, --agents, --mcp-config,
// --allowedTools, --disallowedTools, --json-schema, --append-system-prompt,
// --max-turns, --max-budget-usd, --resume
func BuildArgs(t config.Task, opts RunOptions) []string {
	args := []string{"-p"}

	// Output format: opts override > task config > default "json"
	outputFormat := "json"
	if t.OutputFormat != "" {
		outputFormat = t.OutputFormat
	}
	if opts.OutputFormat != "" {
		outputFormat = opts.OutputFormat
	}
	args = append(args, "--output-format", outputFormat)

	if t.Model != "" {
		args = append(args, "--model", t.Model)
	}

	if opts.AgentsJSON != "" {
		args = append(args, "--agents", opts.AgentsJSON)
	}

	if opts.MCPConfigPath != "" {
		args = append(args, "--mcp-config", opts.MCPConfigPath)
	}

	if t.PermissionMode != "" {
		args = append(args, "--permission-mode", t.PermissionMode)
	}

	if len(t.AllowedTools) > 0 {
		args = append(args, "--allowedTools")
		args = append(args, t.AllowedTools...)
	}

	if len(t.DisallowedTools) > 0 {
		args = append(args, "--disallowedTools")
		args = append(args, t.DisallowedTools...)
	}

	if t.JSONSchema != "" {
		args = append(args, "--json-schema", t.JSONSchema)
	}

	// Merge append_system_prompt: task config + domain doc from opts
	appendPrompt := t.AppendSystemPrompt
	if opts.AppendSystemPrompt != "" {
		if appendPrompt != "" {
			appendPrompt = appendPrompt + "\n\n" + opts.AppendSystemPrompt
		} else {
			appendPrompt = opts.AppendSystemPrompt
		}
	}
	if appendPrompt != "" {
		args = append(args, "--append-system-prompt", appendPrompt)
	}

	if t.MaxTurns > 0 {
		args = append(args, "--max-turns", fmt.Sprintf("%d", t.MaxTurns))
	}

	if t.MaxBudgetUSD > 0 {
		args = append(args, "--max-budget-usd", fmt.Sprintf("%.2f", t.MaxBudgetUSD))
	}

	if opts.ResumeSessionID != "" {
		args = append(args, "--resume", opts.ResumeSessionID)
	}

	return args
}
