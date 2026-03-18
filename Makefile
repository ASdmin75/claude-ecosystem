.PHONY: build build-ui install run run-task run-pipeline clean test \
       docker-build docker-up docker-down \
       daemon-install daemon-uninstall daemon-start daemon-stop daemon-restart daemon-status daemon-logs \
       rebuild setup-whisper

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
	@touch internal/ui/embed.go

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
	go test ./internal/... ./cmd/...

clean:
	rm -rf bin/

docker-build:
	docker compose build

docker-up:
	docker compose up -d

docker-down:
	docker compose down

# --- Systemd user daemon ---

SYSTEMD_DIR = $(HOME)/.config/systemd/user
SERVICE_NAME = claude-ecosystem

daemon-install: build
	@mkdir -p $(SYSTEMD_DIR)
	@sed 's|%h|$(HOME)|g' deploy/claude-ecosystem.service > $(SYSTEMD_DIR)/$(SERVICE_NAME).service
	@systemctl --user daemon-reload
	@systemctl --user enable $(SERVICE_NAME)
	@echo "Daemon installed and enabled. Run: make daemon-start"

daemon-uninstall:
	@systemctl --user stop $(SERVICE_NAME) 2>/dev/null || true
	@systemctl --user disable $(SERVICE_NAME) 2>/dev/null || true
	@rm -f $(SYSTEMD_DIR)/$(SERVICE_NAME).service
	@systemctl --user daemon-reload
	@echo "Daemon uninstalled."

daemon-start:
	@systemctl --user start $(SERVICE_NAME)
	@echo "Started. View logs: make daemon-logs"

daemon-stop:
	@systemctl --user stop $(SERVICE_NAME)

daemon-restart:
	@systemctl --user restart $(SERVICE_NAME)

daemon-status:
	@systemctl --user status $(SERVICE_NAME)

daemon-logs:
	@journalctl --user-unit $(SERVICE_NAME) -f

rebuild:
	@docker compose down 2>/dev/null || true
	@pgrep -x server | xargs -r kill 2>/dev/null || true
	@sleep 1
	$(MAKE) build-ui build
	@systemctl --user restart $(SERVICE_NAME) 2>/dev/null || bin/server -config tasks.yaml &
	@echo "Rebuilt and restarted."

# --- Whisper.cpp setup ---

WHISPER_DIR := data/whisper
WHISPER_SRC := $(WHISPER_DIR)/whisper.cpp
WHISPER_BIN_FILE := $(WHISPER_DIR)/bin/whisper-cli
WHISPER_MODEL := $(WHISPER_DIR)/models/ggml-small.bin

setup-whisper: $(WHISPER_BIN_FILE) $(WHISPER_MODEL)

$(WHISPER_BIN_FILE): $(WHISPER_SRC)
	cd $(WHISPER_SRC) && cmake -B build -DCMAKE_BUILD_TYPE=Release
	cd $(WHISPER_SRC) && cmake --build build --config Release -j$$(nproc)
	mkdir -p $(WHISPER_DIR)/bin
	cp $(WHISPER_SRC)/build/bin/whisper-cli $(WHISPER_BIN_FILE)

$(WHISPER_SRC):
	mkdir -p $(WHISPER_DIR)
	git clone --depth 1 https://github.com/ggml-org/whisper.cpp $(WHISPER_SRC)

$(WHISPER_MODEL):
	mkdir -p $(WHISPER_DIR)/models
	cd $(WHISPER_SRC)/models && bash download-ggml-model.sh small $(CURDIR)/$(WHISPER_DIR)/models/
