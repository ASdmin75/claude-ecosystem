# Workflow — Claude Ecosystem

Журнал доработок и план развития проекта.

---

## Выполненные доработки

### 2026-03-07 — v2: Полный рефакторинг архитектуры

**Фаза 1: Реструктуризация ядра**
- Все пакеты перенесены под `internal/` для инкапсуляции
- Переименование: `agent` → `task`, `Agent` → `Task`, `agents.yaml` → `tasks.yaml`
- Новая схема конфигурации: `Config`, `Task`, `Pipeline`, `MCPServerConfig`, `AuthConfig`, `ServerConfig`
- Обратная совместимость с `agents.yaml` (автомиграция при загрузке)
- Go 1.26.1

**Фаза 2: Расширенный task runner**
- Динамическое построение CLI-аргументов `claude -p` (`--agents`, `--mcp-config`, `--json-schema`, `--max-turns`, `--max-budget-usd`, `--allowed-tools`, `--resume`)
- Синхронный `Run()` и потоковый `RunStream()` (stream-json)
- Парсинг session_id, model, cost_usd из ответа

**Фаза 3: Управление суб-агентами**
- CRUD-менеджер для файлов `.claude/agents/*.md`
- Парсер YAML frontmatter + markdown
- Генерация JSON для флага `--agents`

**Фаза 4: MCP-серверы**
- Менеджер жизненного цикла процессов (lazy start, SIGTERM/SIGKILL, health check)
- Генерация `--mcp-config` JSON во временные файлы
- 7 MCP-серверов (stubs): filesystem, excel, word, pdf, email, google, database

**Фаза 5: Хранилище + авторизация**
- SQLite (pure Go, modernc.org/sqlite): таблицы executions, users
- PASETO v4.local токены (XChaCha20-Poly1305) + bearer-токены
- HTTP middleware для аутентификации

**Фаза 6: REST API**
- 10 файлов обработчиков, Go 1.22+ ServeMux method routing
- Эндпоинты: tasks, subagents, pipelines, executions, mcp-servers, dashboard
- SSE-стриминг для live-вывода, login/refresh

**Фаза 7: Web UI**
- React 19 + Vite + TypeScript + Tailwind CSS 4 + TanStack Query
- Компоненты: Dashboard, TaskList, SubAgentList, PipelineList, ExecutionHistory, Login
- API-клиент с авторизацией, SSE helper

**Фаза 8: MCP-серверы (stubs)**
- JSON-RPC 2.0 stdio, определены схемы инструментов для каждого сервера

**Фаза 9: Пайплайны**
- Sequential: цикл с `{{.PrevOutput}}`, stop_signal
- Parallel: errgroup + опциональный collector task

**Удалено:**
- `cmd/orchestrator` → заменён на `cmd/server`
- Пакеты верхнего уровня `agent/`, `config/`, `pipeline/`, `scheduler/`, `watcher/` → перенесены в `internal/`

### 2026-03-07 — v3: Agents wiring + MCP реализация + Leads Pipeline

**Исправление бага: wiring поля `agents`**
- Создан `internal/task/resolve.go` — функция `ResolveRunOptions()` связывает `config.Task.Agents` → `RunOptions.AgentsJSON` через `subagent.Manager.Get()` + `subagent.ToAgentsJSON()`, и `config.Task.MCPServers` → `RunOptions.MCPConfigPath` через `mcpmanager.Manager.GenerateConfigFile()`
- Обновлены все 9 call sites: `cmd/server/main.go`, `internal/api/tasks.go` (2), `internal/api/pipelines.go`, `internal/pipeline/sequential.go`, `internal/pipeline/parallel.go` (3), `internal/scheduler/scheduler.go`, `internal/watcher/watcher.go`
- Добавлена инъекция `subMgr`/`mcpMgr` в конструкторы `pipeline.NewRunner()`, `scheduler.New()`, `watcher.New()`

**MCP-серверы — реализация**
- **mcp-excel** — полная реализация через `github.com/xuri/excelize/v2`: `create_spreadsheet`, `write_spreadsheet`, `read_spreadsheet` + новый `add_styled_table` (жирные заголовки, цветные строки, auto-filter)
- **mcp-email** — реализация `send_email` через `gopkg.in/gomail.v2`: SMTP с TLS, поддержка `attachments`, `html_body`. Конфиг через env vars: `SMTP_HOST`, `SMTP_PORT`, `SMTP_USER`, `SMTP_PASSWORD`, `SMTP_FROM`
- **mcp-telegram** — новый MCP-сервер через `gopkg.in/telebot.v4`: `send_message` (текст + parse_mode), `send_document` (файл + caption). Конфиг: `TELEGRAM_BOT_TOKEN`, `TELEGRAM_CHAT_ID`
- **mcp-filesystem** — полная реализация: `read_file`, `write_file`, `list_directory`, `search_files` + новый `copy_file`

**Leads Pipeline — пайплайн для CEO**
- 3 новых задачи: `find-leads` (поиск лидов → JSON), `compile-leads-excel` (JSON → Excel-отчёт), `deliver-leads-report` (email + file share + Telegram)
- Пайплайн `leads-to-ceo` (sequential, 3 шага, single-pass)
- 2 суб-агента: `leads-report-compiler`, `delivery-agent`
- 4 MCP-сервера сконфигурированы в `tasks.yaml`: excel, email, telegram, filesystem

**Прочее**
- Ослаблена валидация: `stop_signal` не требуется для sequential пайплайнов с `max_iterations: 1`
- Новые зависимости: `excelize/v2`, `gomail.v2`, `telebot.v4`

### 2026-03-08 — Поддержка .env

- Встроенный парсер `.env` без внешних зависимостей (`internal/config/dotenv.go`)
- Загрузка `.env` при старте сервера (до парсинга `tasks.yaml`)
- Подстановка `${VAR}` ссылок в конфигурации: auth, MCP servers env, task prompts, work_dir
- Приоритет: `.env` < переменные окружения (реальные env vars не перезаписываются)
- `.env.example` с описанием всех переменных
- `.env` в `.gitignore`
- Unit-тесты: `LoadDotEnv`, `ExpandEnvVars`, приоритет, отсутствующий файл

### 2026-03-08 — Email/webhook уведомления

- Новый пакет `internal/notify/` — обработчик уведомлений при завершении задач
- Структура `NotifyConfig` в `config.Task`: поля `email` ([]string), `webhook` (string), `trigger` (on_success | on_failure | always)
- Email через SMTP (gomail): HTML-шаблон с результатом + plain-text fallback, использует те же `SMTP_*` env vars что и mcp-email
- Webhook: HTTP POST с JSON-телом (event, task, status, execution_id, output, error, timestamp)
- Подписка на события `task.completed` и `pipeline.completed` через event bus
- Валидация `notify.trigger` в `config.Validate()`
- Подстановка `${VAR}` в полях `notify.email` и `notify.webhook`
- TypeScript-тип `NotifyConfig` в Web UI
- Unit-тесты: trigger-логика, webhook-доставка, фильтрация по статусу, шаблоны

### 2026-03-08 — Docker Compose

- Multi-stage `Dockerfile`: Node (React) → Go (бинарники) → Alpine (runtime + Claude CLI)
- Единый образ содержит server + все MCP-серверы (mcpmanager запускает их как дочерние процессы)
- `docker-compose.yml`: сервис server с health check (`/api/v1/dashboard`), volumes для config/data/agents, `.env`
- `.dockerignore` для чистой сборки
- Makefile: `docker-build`, `docker-up`, `docker-down`

### 2026-03-08 — Workspace volumes и Docker I/O

- Добавлен volume `workspace` для файлового ввода-вывода задач в Docker
- Переменная `WORKSPACE_DIR` в `.env` (по умолчанию `./workspace`)
- Маппинг: `${WORKSPACE_DIR:-./workspace}:/app/workspace`
- Структура каталогов: `workspace/leads/`, `workspace/ceo/reports/`
- `.gitignore` в workspace — данные не попадают в git, структура сохраняется через `.gitkeep`
- Убран `:ro` с `tasks.yaml` и `.claude/agents/` — Web UI может сохранять изменения конфигурации
- Обновлены пути в задачах `compile-leads-excel` и `deliver-leads-report` на workspace-relative

### 2026-03-08 — Прочее

- Порт по умолчанию изменён с `:8080` на `:3580`
- Go 1.26.0 → 1.26.1

### 2026-03-08 — Systemd daemon

- Systemd user service (`deploy/claude-ecosystem.service`) для запуска сервера как Linux-демона
- Unit-файл: `Type=simple`, graceful shutdown через SIGTERM, `Restart=on-failure`, логирование в journald
- `EnvironmentFile` загружает `.env`, `PATH` включает `~/.local/bin` для доступа к `claude` CLI
- Makefile targets: `daemon-install`, `daemon-uninstall`, `daemon-start`, `daemon-stop`, `daemon-restart`, `daemon-status`, `daemon-logs`
- User-level systemd (`systemctl --user`) — не требует `sudo`
- `daemon-install` автоматически собирает бинарники, устанавливает service и включает автозапуск

---

## Бэклог

### Фаза 10: Обновление hook-системы
- [ ] Оценить миграцию на хуки суб-агентов или MCP-сервер
- [ ] Расширить список опасных паттернов

### MCP-серверы — доработки
- [x] mcp-filesystem: полная реализация tools/call + copy_file
- [x] mcp-excel: интеграция с excelize + add_styled_table
- [ ] mcp-word: интеграция с docx-библиотекой
- [ ] mcp-pdf: интеграция с pdfcpu
- [x] mcp-email: SMTP-отправка с вложениями через gomail
- [ ] mcp-email: IMAP-чтение (read_inbox, search_emails)
- [ ] mcp-google: Google Docs/Sheets API
- [ ] mcp-database: SQL-драйверы (postgres, mysql, sqlite)
- [x] mcp-telegram: отправка сообщений и файлов через Telegram Bot API

### Web UI — доработки
- [ ] Детальный просмотр execution с SSE-стримингом
- [ ] Редактирование задач через UI
- [ ] Конфигурация scheduler/watcher через UI
- [ ] Управление MCP-серверами через UI
- [ ] Тёмная тема

### Инфраструктура
- [ ] Unit-тесты для всех internal/ пакетов
- [ ] CI/CD pipeline (GitHub Actions)
- [x] Docker-образ (Dockerfile + docker-compose.yml)
- [ ] Docker: авторизация Claude Code Max (OAuth) — `claude login` из контейнера не подключается к api.anthropic.com (ERR_BAD_REQUEST). Варианты: host network, DNS-fix, или ANTHROPIC_API_KEY
- [ ] Документация API (OpenAPI/Swagger)
