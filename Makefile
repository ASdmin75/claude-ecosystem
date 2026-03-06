.PHONY: build install run run-agent clean

build:
	go build -o bin/orchestrator ./cmd/orchestrator
	go build -o bin/hook ./cmd/hook

install: build
	@echo "Installing hook binary..."
	cp bin/hook ~/.local/bin/claude-hook
	@echo "Done. Add to ~/.claude/settings.json:"
	@echo '  "hooks": {'
	@echo '    "PreToolUse": [{"matcher": "Bash", "command": "claude-hook"}]'
	@echo '  }'

run:
	go run ./cmd/orchestrator -config agents.yaml

run-agent:
	@test -n "$(AGENT)" || (echo "Usage: make run-agent AGENT=code-review" && exit 1)
	go run ./cmd/orchestrator -config agents.yaml -run $(AGENT)

clean:
	rm -rf bin/
