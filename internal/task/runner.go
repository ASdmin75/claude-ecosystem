package task

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"text/template"
	"time"

	"github.com/asdmin/claude-ecosystem/internal/config"
)

// Runner executes tasks by invoking the claude CLI as a subprocess.
type Runner struct {
	ClaudeBin string
}

// NewRunner creates a new Runner with the given claude binary path.
// If claudeBin is empty, it defaults to "claude".
func NewRunner(claudeBin string) *Runner {
	if claudeBin == "" {
		claudeBin = "claude"
	}
	return &Runner{ClaudeBin: claudeBin}
}

// Run executes a task synchronously and returns a Result.
// templateVars expands {{.Var}} placeholders in the task prompt using text/template.
func (r *Runner) Run(ctx context.Context, t config.Task, opts RunOptions, templateVars map[string]string) Result {
	start := time.Now()

	prompt, err := renderPrompt(t.Prompt, templateVars)
	if err != nil {
		return Result{
			TaskName: t.Name,
			Error:    fmt.Sprintf("template error: %v", err),
			Duration: time.Since(start),
		}
	}

	args := BuildArgs(t, opts)
	cmd := exec.CommandContext(ctx, r.ClaudeBin, args...)
	if t.WorkDir != "" {
		cmd.Dir = t.WorkDir
	}
	cmd.Stdin = strings.NewReader(prompt)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return Result{
			TaskName: t.Name,
			Output:   stdout.String(),
			Error:    fmt.Sprintf("%v: %s", err, stderr.String()),
			Duration: time.Since(start),
		}
	}

	result := parseJSONOutput(stdout.Bytes())
	result.TaskName = t.Name
	result.Duration = time.Since(start)
	return result
}

// RunStream executes a task with streaming output, sending chunks to the provided channel.
// It forces --output-format stream-json. The chunks channel is closed when execution completes.
func (r *Runner) RunStream(ctx context.Context, t config.Task, opts RunOptions, templateVars map[string]string, chunks chan<- StreamChunk) Result {
	defer close(chunks)

	start := time.Now()

	prompt, err := renderPrompt(t.Prompt, templateVars)
	if err != nil {
		return Result{
			TaskName: t.Name,
			Error:    fmt.Sprintf("template error: %v", err),
			Duration: time.Since(start),
		}
	}

	opts.OutputFormat = "stream-json"
	args := BuildArgs(t, opts)
	cmd := exec.CommandContext(ctx, r.ClaudeBin, args...)
	if t.WorkDir != "" {
		cmd.Dir = t.WorkDir
	}
	cmd.Stdin = strings.NewReader(prompt)

	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return Result{
			TaskName: t.Name,
			Error:    fmt.Sprintf("stdout pipe error: %v", err),
			Duration: time.Since(start),
		}
	}

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Start(); err != nil {
		return Result{
			TaskName: t.Name,
			Error:    fmt.Sprintf("start error: %v: %s", err, stderr.String()),
			Duration: time.Since(start),
		}
	}

	var finalOutput string
	var sessionID, model string
	var costUSD float64

	scanner := bufio.NewScanner(stdoutPipe)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var raw map[string]interface{}
		if err := json.Unmarshal(line, &raw); err != nil {
			continue
		}

		chunk := StreamChunk{
			TaskName: t.Name,
		}

		if typ, ok := raw["type"].(string); ok {
			chunk.Type = typ
		}

		// Extract content based on type.
		switch chunk.Type {
		case "assistant":
			if content, ok := raw["content"].(string); ok {
				chunk.Content = content
			}
		case "result":
			if result, ok := raw["result"].(string); ok {
				chunk.Content = result
				finalOutput = result
			}
			if sid, ok := raw["session_id"].(string); ok {
				sessionID = sid
			}
			if m, ok := raw["model"].(string); ok {
				model = m
			}
			if c, ok := raw["cost_usd"].(float64); ok {
				costUSD = c
			}
		case "error":
			if content, ok := raw["error"].(string); ok {
				chunk.Content = content
			}
		}

		chunks <- chunk
	}

	cmdErr := cmd.Wait()

	result := Result{
		TaskName:  t.Name,
		Output:    finalOutput,
		Duration:  time.Since(start),
		SessionID: sessionID,
		Model:     model,
		CostUSD:   costUSD,
	}

	if cmdErr != nil {
		result.Error = fmt.Sprintf("%v: %s", cmdErr, stderr.String())
	}

	return result
}

// renderPrompt applies template variables to the prompt string using text/template.
func renderPrompt(tpl string, vars map[string]string) (string, error) {
	if vars == nil || !strings.Contains(tpl, "{{") {
		return tpl, nil
	}
	t, err := template.New("prompt").Parse(tpl)
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	if err := t.Execute(&buf, vars); err != nil {
		return "", err
	}
	return buf.String(), nil
}

// parseJSONOutput extracts result fields from the claude CLI JSON output.
func parseJSONOutput(data []byte) Result {
	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		return Result{Output: string(data)}
	}

	var result Result

	if r, ok := raw["result"].(string); ok {
		result.Output = r
	} else {
		result.Output = string(data)
	}

	if sid, ok := raw["session_id"].(string); ok {
		result.SessionID = sid
	}

	if m, ok := raw["model"].(string); ok {
		result.Model = m
	}

	if c, ok := raw["cost_usd"].(float64); ok {
		result.CostUSD = c
	}

	return result
}
