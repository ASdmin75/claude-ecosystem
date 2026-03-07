package main

import (
	"encoding/json"
	"log/slog"
	"os"
	"strings"
)

// HookInput is the JSON structure Claude Code sends to hooks via stdin.
type HookInput struct {
	SessionID string `json:"session_id"`
	ToolName  string `json:"tool_name"`
	ToolInput struct {
		Command     string `json:"command"`
		FilePath    string `json:"file_path"`
		Content     string `json:"content"`
		Description string `json:"description"`
	} `json:"tool_input"`
}

// HookOutput is the JSON response a hook can return to Claude Code.
type HookOutput struct {
	Decision string `json:"decision"` // "approve", "block", "modify"
	Reason   string `json:"reason,omitempty"`
}

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))

	var input HookInput
	if err := json.NewDecoder(os.Stdin).Decode(&input); err != nil {
		logger.Error("failed to decode hook input", "error", err)
		os.Exit(0) // exit 0 to not block Claude
	}

	logger.Info("hook invoked",
		"tool", input.ToolName,
		"session", input.SessionID,
	)

	switch input.ToolName {
	case "Bash":
		if containsDangerous(input.ToolInput.Command) {
			respond(HookOutput{
				Decision: "block",
				Reason:   "Command contains potentially dangerous operations",
			})
			return
		}

	case "Write", "Edit":
		// Log file modifications for audit trail
		logger.Info("file modification",
			"tool", input.ToolName,
			"file", input.ToolInput.FilePath,
		)
	}

	// Default: approve
	respond(HookOutput{Decision: "approve"})
}

func respond(out HookOutput) {
	if err := json.NewEncoder(os.Stdout).Encode(out); err != nil {
		slog.Error("failed to encode hook output", "error", err)
	}
}

var dangerousPatterns = []string{
	"rm -rf /",
	"rm -rf /*",
	"rm -r -f /",
	"DROP TABLE",
	"DROP DATABASE",
	":(){ :|:",  // fork bomb
	"> /dev/sda",
	"mkfs.",
	"dd if=",
	"chmod -R 777 /",
}

func containsDangerous(cmd string) bool {
	lower := strings.ToLower(cmd)
	for _, p := range dangerousPatterns {
		if strings.Contains(lower, strings.ToLower(p)) {
			return true
		}
	}
	return false
}
