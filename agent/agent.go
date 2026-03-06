package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"text/template"
	"time"
)

type Result struct {
	AgentName string        `json:"agent_name"`
	Output    string        `json:"output"`
	Duration  time.Duration `json:"duration"`
	Error     string        `json:"error,omitempty"`
}

type Runner struct {
	ClaudeBin string
}

func NewRunner(claudeBin string) *Runner {
	if claudeBin == "" {
		claudeBin = "claude"
	}
	return &Runner{ClaudeBin: claudeBin}
}

// Run executes a claude agent with the given prompt and working directory.
// templateVars can be used to expand {{.Var}} in the prompt template.
func (r *Runner) Run(ctx context.Context, name, promptTpl, workDir string, templateVars map[string]string) Result {
	start := time.Now()

	prompt, err := renderPrompt(promptTpl, templateVars)
	if err != nil {
		return Result{AgentName: name, Error: fmt.Sprintf("template error: %v", err), Duration: time.Since(start)}
	}

	cmd := exec.CommandContext(ctx, r.ClaudeBin, "-p", "--output-format", "json")
	cmd.Dir = workDir
	cmd.Stdin = strings.NewReader(prompt)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return Result{
			AgentName: name,
			Output:    stdout.String(),
			Error:     fmt.Sprintf("%v: %s", err, stderr.String()),
			Duration:  time.Since(start),
		}
	}

	output := extractResult(stdout.Bytes())
	return Result{
		AgentName: name,
		Output:    output,
		Duration:  time.Since(start),
	}
}

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

// extractResult tries to parse JSON output and extract the text result.
// Falls back to raw output if parsing fails.
func extractResult(data []byte) string {
	var resp struct {
		Result string `json:"result"`
	}
	if err := json.Unmarshal(data, &resp); err == nil && resp.Result != "" {
		return resp.Result
	}
	return string(data)
}
