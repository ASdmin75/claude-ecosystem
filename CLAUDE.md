# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Claude Ecosystem is a Go orchestrator that runs Claude Code CLI (`claude -p`) as automated tasks. Tasks are defined in `tasks.yaml` and can be triggered on a cron schedule, in response to file changes, via REST API, or through the web UI. The system also manages Claude Code sub-agents (`.claude/agents/*.md`) via CRUD, supports MCP server lifecycle management, and provides agent-to-agent (A2A) pipelines.

## Build & Run

```bash
make build              # builds bin/server, bin/hook, and all MCP server binaries
make build-ui           # builds React frontend and copies to internal/ui/dist
make run                # runs server (HTTP + scheduler + watcher) with tasks.yaml
make run-task TASK=code-review       # runs a single task by name and exits
make run-pipeline PIPELINE=review-fix  # runs a pipeline and exits
make install            # installs hook binary to ~/.local/bin/claude-hook
make test               # runs all unit tests
make clean              # removes bin/
```

Requires Go 1.26+. The `claude` CLI must be on PATH (or set `claude_bin` in tasks.yaml).

## Architecture

### Binaries

- **`cmd/server`** — Main binary. HTTP server (REST API + embedded React SPA), scheduler, watcher. Hot-reloads `tasks.yaml` on file changes (tasks, pipelines, schedules). Supports `-run <task>` and `-pipeline <name>` for CLI mode.
- **`cmd/hook`** — Claude Code hook binary. Reads JSON from stdin, blocks dangerous commands, logs file edits.
- **`cmd/mcp/*`** — MCP servers (stdio, built on [mcp-go](https://github.com/mark3labs/mcp-go) v0.45.0). Implemented: filesystem (CRUD + copy), excel (excelize), email (gomail SMTP), telegram (telebot), database (SQLite via domain system), openapi (dynamic tools from OpenAPI specs via libopenapi, configurable OAuth2 with custom body fields and query/header token injection, built-in `download_file` tool, TLS insecure mode), word (docx via stdlib), pdf (ledongthuc/pdf), whisper (audio transcription via whisper.cpp). Stubs: google.

### Core Packages (all under `internal/`)

- **`config/`** — Parses `tasks.yaml` into `Config`/`Task`/`Pipeline`/`MCPServerConfig`/`Domain` structs. Backward-compatible with old `agents.yaml` format.
- **`domain/`** — Domain manager: initializes data directories, applies SQLite schemas, generates DOMAIN.md templates, provides env vars and doc content for task resolution.
- **`task/`** — `Runner` executes `claude -p` with dynamically built CLI args. `ResolveRunOptions()` wires `config.Task.Agents` → `--agents` JSON, `config.Task.MCPServers` → `--mcp-config` file (with domain env vars), and `config.Task.Domain` → DOMAIN.md injection into `--append-system-prompt`. Supports sync `Run()` and streaming `RunStream()`.
- **`pipeline/`** — Runs sequential (loop with `{{.PrevOutput}}`) or parallel (errgroup) pipelines. Factory `Run()` dispatches by mode.
- **`subagent/`** — CRUD manager for `.claude/agents/*.md` files. Parses YAML frontmatter + markdown. Generates `--agents` JSON for task runner.
- **`mcpmanager/`** — Process lifecycle for MCP servers (lazy start, SIGTERM/SIGKILL shutdown, health). Generates `--mcp-config` temp files.
- **`runguard/`** — Concurrency guard: prevents overlapping runs of tasks/pipelines when `allow_concurrent: false`. Shared across scheduler, watcher, and API.
- **`scheduler/`** — Cron scheduler with pause/resume per task and pipeline. Supports `Reset()` for hot reload.
- **`watcher/`** — fsnotify file watcher with extension filtering and debounce. Supports `Reset()` for hot reload.
- **`events/`** — Pub/sub event bus for decoupling task completion from logging/SSE.
- **`auth/`** — PASETO v4.local tokens + bearer token fallback + HTTP middleware.
- **`store/sqlite/`** — SQLite storage for execution history and users (pure Go, no CGo).
- **`api/`** — REST API handlers using Go 1.22+ ServeMux method routing. SSE streaming.
- **`ui/`** — `go:embed` for the React build (internal/ui/dist/).

## Configuration (tasks.yaml)

Each task has: `name`, `prompt` (Go template), `work_dir`, and either `schedule` (cron) or `watch` (paths + extensions). Optional: `tags`, `model`, `timeout`, `agents` (sub-agent names), `mcp_servers`, `allowed_tools`, `json_schema`, `max_turns`, `max_budget_usd`, `output_format`, `domain`, `allow_concurrent`.

Pipelines chain tasks in sequential loops or parallel execution. Pipelines support `schedule` (cron) for automatic periodic execution and `allow_concurrent` (bool, default true) to prevent overlapping runs. Template variables available in pipeline steps: `{{.PrevOutput}}` (output from previous step), `{{.Date}}` (current date YYYY-MM-DD). When using `allowed_tools` with sub-agents, include `Agent` in the list so Claude can delegate to them. Each pipeline step enforces the task's `timeout`. Config also includes `server`, `auth`, `mcp_servers`, and `domains` sections.

### Domains

Optional `domains` section defines business data domains linked to tasks. Each domain has: `data_dir`, `db` (SQLite file), `schema` (SQL to apply on init), `domain_doc` (DOMAIN.md auto-injected into agent system prompt), and reference lists (`tasks`, `pipelines`, `agents`, `mcp_servers`). Tasks reference a domain via `domain: <name>`. Domain env vars (`DOMAIN_DB_PATH`, `DOMAIN_DATA_DIR`, etc.) are automatically injected into MCP server configs. The `data/{domain}/DOMAIN.md` file provides agents with table schemas, deduplication rules, and tool usage examples — all without hardcoding in task prompts.

## REST API

All under `/api/v1/`. Auth required (PASETO or bearer token) except `/auth/login`.

Key endpoints: task CRUD + run, sub-agent CRUD, pipeline run, execution history, MCP server management, SSE streaming (`/events` for global event stream, `/executions/{id}/stream` for per-execution), dashboard stats. Auth supports query param `?token=` for SSE (EventSource limitation).

## Web UI

React 19 + Vite + TypeScript + Tailwind CSS 4 + TanStack Query. Source in `web/`, built output embedded in Go binary via `internal/ui/dist/`. Dark mode support via class-based toggle (`localStorage` persisted, button in sidebar). Real-time updates via SSE (`useSSE` hook) with auto-reconnect — no polling. Toast notifications on task/pipeline start/completion.

## Hook System

`cmd/hook` implements Claude Code's hook protocol. See `claude-hooks.example.json` for the hooks config format. The hook blocks commands matching dangerous patterns (rm -rf /, DROP TABLE, fork bombs, etc.) and logs file edits.
