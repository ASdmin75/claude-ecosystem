.PHONY: build build-ui install run run-task run-pipeline clean test docker-build docker-up docker-down

build:
	go build -o bin/server ./cmd/server
	go build -o bin/hook ./cmd/hook
	@for dir in cmd/mcp/*/; do \
		name=$$(basename $$dir); \
		go build -o bin/$$name ./$$dir; \
	done

build-ui:
	cd web && npm install && npm run build
	rm -rf internal/ui/dist
	cp -r web/dist internal/ui/dist

install: build
	@echo "Installing hook binary..."
	cp bin/hook ~/.local/bin/claude-hook
	@echo "Done. Add to ~/.claude/settings.json:"
	@echo '  "hooks": {'
	@echo '    "PreToolUse": [{"matcher": "Bash", "command": "claude-hook"}]'
	@echo '  }'

run:
	go run ./cmd/server -config tasks.yaml

run-task:
	@test -n "$(TASK)" || (echo "Usage: make run-task TASK=code-review" && exit 1)
	go run ./cmd/server -config tasks.yaml -run $(TASK)

run-pipeline:
	@test -n "$(PIPELINE)" || (echo "Usage: make run-pipeline PIPELINE=review-fix" && exit 1)
	go run ./cmd/server -config tasks.yaml -pipeline $(PIPELINE)

test:
	go test ./internal/...

clean:
	rm -rf bin/

docker-build:
	docker compose build

docker-up:
	docker compose up -d

docker-down:
	docker compose down
