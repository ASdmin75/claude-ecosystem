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
- Go 1.26.0

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
- [ ] Docker-образ
- [ ] Документация API (OpenAPI/Swagger)
