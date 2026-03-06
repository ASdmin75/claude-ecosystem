package main

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"strings"
)

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

type HookOutput struct {
	Decision string `json:"decision"` // "approve", "block", "modify"
	Reason   string `json:"reason,omitempty"`
}

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))

	var input HookInput
	if err := json.NewDecoder(os.Stdin).Decode(&input); err != nil {
		logger.Error("failed to decode hook input", "error", err)
		os.Exit(0)
	}

	logger.Info("hook invoked",
		"tool", input.ToolName,
		"session", input.SessionID,
	)

	switch input.ToolName {
	case "Bash":
		if containsDangerous(input.ToolInput.Command) {
			output := HookOutput{
				Decision: "block",
				Reason:   "Command contains potentially dangerous operations",
			}
			json.NewEncoder(os.Stdout).Encode(output)
			return
		}

	case "Write", "Edit":
		logger.Info("file modification",
			"tool", input.ToolName,
			"file", input.ToolInput.FilePath,
		)
	}

	fmt.Fprint(os.Stdout, `{"decision":"approve"}`)
}

var dangerousPatterns = []string{
	"rm -rf /",
	"DROP TABLE",
	"DROP DATABASE",
	":(){ :|:",
	"> /dev/sda",
	"mkfs.",
	"dd if=",
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
