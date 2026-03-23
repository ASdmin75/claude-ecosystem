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

### 2026-03-08 — Исправления пайплайна leads-to-ceo

**Подключение суб-агента к задаче find-leads**
- Добавлено поле `agents: [eaeu-logistics-lead-finder]` в задачу `find-leads` — ранее суб-агент упоминался только в тексте промпта, но не передавался через `--agents`
- Без поля `agents` в конфиге `ResolveRunOptions()` не генерирует `--agents` JSON

**Исправление наследования переменных окружения в MCP-серверах**
- Баг: `mcpmanager.Start()` создавал `cmd.Env` с нуля — MCP-процессы получали только явно указанные env vars, теряя переменные из `.env` и системного окружения
- Фикс: добавлено `cmd.Env = os.Environ()` перед кастомными переменными — MCP-серверы наследуют всё окружение родительского процесса

**Удаление дублирования credentials из tasks.yaml**
- Убраны захардкоженные SMTP и Telegram credentials из секции `mcp_servers.env` в `tasks.yaml`
- Переменные теперь берутся из `.env` через наследование окружения
- Секреты больше не дублируются в двух местах

**Настройка Telegram-бота**
- Создан бот через @BotFather, токен и chat_id группы сохранены в `.env`
- Отключён Group Privacy для получения сообщений в группах
- Проверена отправка сообщений через `mcp-telegram`

### 2026-03-08 — Исправление пайплайна leads-to-ceo (v2)

**Блокировка инструмента Agent в задаче find-leads**
- Баг: `allowed_tools: [WebSearch, WebFetch]` передавалось как `--allowedTools WebSearch WebFetch`, что блокировало инструмент `Agent` — Claude не мог делегировать работу суб-агенту `eaeu-logistics-lead-finder`
- Фикс: добавлен `Agent` в `allowed_tools` задачи `find-leads`

**Недостаточный max_turns**
- Баг: `max_turns: 30` — слишком мало для поиска 25 лидов (каждый лид требует 2-3+ тёрна на поиск и обработку)
- Фикс: увеличено до `max_turns: 100`

**Переменная `{{.Date}}` не передавалась в пайплайне**
- Баг: `compile-leads-excel` использовал `{{.Date}}` в имени файла, но `runPipeline()` передавал только `PrevOutput` — дата была пустой, файл назывался `leads-report-.xlsx`
- Фикс: добавлено `"Date": time.Now().Format("2006-01-02")` в template vars пайплайна (`internal/api/pipelines.go`)

**Отсутствие per-task таймаута в пайплайнах**
- Баг: `runPipeline()` использовал общий контекст пайплайна без per-task таймаутов — если шаг зависал, весь пайплайн блокировался бесконечно (в отличие от `handleRunTask`, который применяет `t.ParsedTimeout()`)
- Фикс: добавлен `context.WithTimeout(ctx, t.ParsedTimeout())` для каждого шага пайплайна

### 2026-03-08 — Execution History: отображение пайплайнов + тёмная тема

**Отображение имени пайплайна в Execution History**
- Баг: при запуске пайплайна в таблице Execution History колонка Task была пустой — `pipeline_name` записывался в БД, но UI отображал только `task_name`
- Фикс: `ExecutionHistory.tsx` — fallback на `pipeline_name` когда `task_name` пустой
- Визуальное различие: задачи отмечены синим значком ▶, пайплайны — фиолетовым значком ⛓
- В панели деталей — бейдж "task" или "pipeline" соответствующего цвета

**Тёмная тема (Dark Mode)**
- CSS: `@custom-variant dark` в `index.css` — class-based dark mode для Tailwind CSS 4
- `App.tsx`: хук `useTheme()` — переключение через кнопку в sidebar, состояние в `localStorage('theme')`, класс `dark` на `<html>`
- Все 6 компонентов обновлены с `dark:` вариантами Tailwind:
  - Фоны: `gray-900` (main), `gray-950` (root), `gray-800` (карточки/панели)
  - Инпуты: `gray-700` с `gray-600` бордерами
  - Статус-бейджи: полупрозрачные `dark:bg-*/40` варианты
  - Markdown-вывод: `dark:prose-invert` для корректной инверсии типографики
  - Тени: `dark:shadow-gray-950`
- Компоненты: Login, Dashboard, TaskList, SubAgentList, PipelineList, ExecutionHistory

### 2026-03-08 — Оптимизация задачи find-leads (таймаут 60 мин)

**Проблема:** пайплайн `leads-to-ceo` не укладывался в 60-минутный таймаут — шаг `find-leads` убивался по `signal: killed` после 3600 секунд.

**Причины:**
- `max_turns: 100` — Claude использовал все 100 ходов, бесконечно уточняя и перепроверяя результаты вместо быстрой выдачи
- `json_schema` — строгая JSON-схема вынуждала тратить дополнительные ходы на форматирование и валидацию вывода

**Изменения в `tasks.yaml`:**
- `max_turns`: 100 → **15** (достаточно для 10 лидов через субагента)
- `json_schema`: **убрана** (формат задаётся текстом промпта, без принудительной валидации)

**Вывод:** при использовании субагентов `max_turns` не должен быть избыточным — каждый ход с вызовом субагента порождает отдельный процесс `claude`, что кратно увеличивает время выполнения.

### 2026-03-08 — UI-фиксы и Makefile rebuild

**Исправление переполнения текста ошибки в Execution History**
- Добавлены `min-w-0`, `overflow-hidden` на контейнер деталей
- `<pre>` с ошибкой: `overflow-y-auto`, `whitespace-pre-wrap`, `break-all` — текст переносится вместо горизонтального скролла

**Исправление ввода запятой в CSV-полях форм**
- TaskList: поля Tags, Agents, MCP Servers, Allowed Tools — `filter(Boolean)` при каждом onChange мгновенно удалял запятую
- SubAgentList: поля Tools, Disallowed Tools, MCP Servers — аналогичная проблема
- Решение: сырой текст хранится в отдельном `csvFields` state, парсинг в массив — параллельно, но отображается raw-строка

**Makefile: таргет `rebuild`**
- Останавливает Docker (`docker compose down`), убивает процесс `server`, пересобирает UI + Go-бинарники, запускает systemd-демон
- `build-ui` теперь делает `touch internal/ui/embed.go` после копирования dist — инвалидирует Go build cache для `go:embed`

### 2026-03-09 — Исправление MCP config + allowed_tools + Telegram filename

**Исправление формата `--mcp-config` JSON**
- Баг: `GenerateConfigFile()` генерировал JSON с `"args": null` при отсутствии аргументов у MCP-сервера — Claude Code отклонял файл: `mcpServers.excel: Does not adhere to MCP server configuration schema`
- Фикс: `args` помечен `json:"args,omitempty"` — пустые args не включаются в JSON
- Убрано ранее добавленное поле `"type": "stdio"` — Claude Code его не требует для stdio-серверов
- Протестирован формат вручную: `claude -p "..." --mcp-config file.json` — подтверждён формат `{"mcpServers": {"name": {"command": "..."}}}`

**Разблокировка MCP-инструментов в режиме `dontAsk`**
- Баг: при `permission_mode: dontAsk` Claude Code блокировал MCP-инструменты — они требуют явного перечисления в `allowed_tools`
- Фикс: добавлены `allowed_tools` для двух задач:
  - `compile-leads-excel`: `mcp__excel__create_spreadsheet`, `mcp__excel__add_styled_table`, `mcp__excel__write_spreadsheet`, `mcp__excel__read_spreadsheet`
  - `deliver-leads-report`: `mcp__email__send_email`, `mcp__filesystem__copy_file`, `mcp__telegram__send_document`

**Исправление имени файла в Telegram**
- Баг: `mcp-telegram` отправлял документы без имени файла — в Telegram файл приходил как `file` без расширения
- Фикс: добавлено `FileName: filepath.Base(filePath)` в `handleSendDocument()` — теперь файл приходит с оригинальным именем (напр. `leads-report-2026-03-09.xlsx`)

### 2026-03-09 — Динамическое обновление UI через SSE

**Бэкенд: глобальный SSE-эндпоинт**
- Новый эндпоинт `GET /api/v1/events` — общий поток Server-Sent Events для всех событий системы
- Типы событий: `task.started`, `task.completed`, `pipeline.started`, `pipeline.completed`, `task.cancelled`
- Публикация `task.started` при запуске задач (sync и async handlers)
- Публикация `pipeline.started` при запуске пайплайнов (sync и async handlers)
- Auth middleware: добавлена поддержка `?token=` query param для SSE (EventSource не умеет ставить заголовки)

**Фронтенд: SSE-клиент**
- Новый хук `useSSE` (`web/src/hooks/useSSE.ts`) — подключение к `/api/v1/events` с автопереподключением (exponential backoff: 1s → 30s)
- При получении любого события — автоматическая инвалидация TanStack Query кешей (`executions`, `dashboard`, `execution` detail)
- Callback `onEvent` для кастомной обработки событий

**Фронтенд: toast-уведомления**
- Новый компонент `Toast` (`web/src/components/Toast.tsx`) — хук `useToast()` + контейнер `ToastContainer`
- Три типа: `success` (зелёный), `error` (красный), `info` (синий)
- Автоскрытие через 5 секунд, ручное закрытие по кнопке
- Slide-in анимация (CSS keyframes в `index.css`)

**Фронтенд: real-time обновления**
- Dashboard: счётчики обновляются в реальном времени (через SSE → query invalidation)
- Execution History: список и детали обновляются в реальном времени
- Убраны `refetchInterval: 5000` (список) и `refetchInterval: 3000` (детали при status=running)
- `App.tsx`: SSE подключается при наличии токена, toast-уведомления при старте/завершении задач и пайплайнов

### 2026-03-09 — Система доменов + дедупликация лидов

**Проблема:** субагент `eaeu-logistics-lead-finder` при повторных запусках находил дублирующихся лидов. Agent memory (`.claude/agent-memory/`) ненадёжна для дедупликации: MEMORY.md ограничен 200 строками, LLM может пропустить дубликат. Нужно структурированное хранилище с точной проверкой.

**Решение: секция `domains` в `tasks.yaml`**

Домен — реестр, связывающий бизнес-данные (SQLite DB, файлы, документацию) с задачами, агентами и пайплайнами. Бизнес-данные отделены от системной БД `claude-ecosystem.db`.

**Конфигурация (`internal/config/`)**
- Новый тип `Domain` (`domain.go`): `description`, `data_dir`, `db`, `schema`, `domain_doc`, ссылки на `tasks/pipelines/agents/mcp_servers`
- Поле `Domain string` в `Task` — привязка задачи к домену
- `Config.Domains map[string]Domain` — секция `domains:` в `tasks.yaml` (опциональна)
- Валидация ссылок: task→domain, domain→tasks (`validate.go`)
- Подстановка `${VAR}` в `domain.DataDir` и `domain.DB` (`dotenv.go`)

**Менеджер доменов (`internal/domain/manager.go`)**
- `Init()`: создаёт `data_dir`, применяет SQL-схему через SQLite, генерирует шаблон `DOMAIN.md` (парсит CREATE TABLE → таблица колонок)
- `DomainEnvVars()`: возвращает `DOMAIN_DB_PATH`, `DOMAIN_DATA_DIR`, `DOMAIN_NAME`, `DOMAIN_DOC_PATH` для инжекции в MCP-серверы
- `DomainDocContent()`: читает `DOMAIN.md` для инжекции в system prompt агента

**MCP конфиг с env vars (`internal/mcpmanager/config.go`)**
- Новый метод `GenerateConfigFileWithEnv(serverNames, extraEnv)` — мержит domain env vars в `Env` каждого MCP-сервера при генерации JSON
- `GenerateConfigFile()` → делегирует в `GenerateConfigFileWithEnv(names, nil)`

**Резолвинг задач (`internal/task/resolve.go`)**
- `ResolveRunOptions()` принимает `*domain.Manager` (4-й параметр)
- Если `task.Domain != ""`: получает domain env vars → передаёт в `GenerateConfigFileWithEnv()`, читает `DOMAIN.md` → добавляет в `RunOptions.AppendSystemPrompt`
- `BuildArgs()` мержит `AppendSystemPrompt` из task config + opts (через `\n\n`)

**MCP Database Server (`cmd/mcp/mcp-database/main.go`) — полная реализация из стаба**
- 6 инструментов: `query` (SELECT + auto LIMIT 1000), `execute` (INSERT/UPDATE/DELETE), `list_tables`, `describe_table`, `check_exists` (дедупликация по column=value → bool), `insert` (table + JSON data → ID)
- Безопасность: `check_exists`/`insert` строят SQL параметризованно (без инъекций), `query`/`execute` отклоняют DROP/ALTER/ATTACH
- Валидация идентификаторов через regexp `^[a-zA-Z_][a-zA-Z0-9_]*$`
- Читает `DOMAIN_DB_PATH` из env (инжектируется domain manager → MCP config)

**Wiring (`cmd/server/main.go`)**
- `domain.New(cfg.Domains, logger)` + `Init()` при старте
- `domainMgr` прокинут во все компоненты: `pipeline.NewRunner`, `scheduler.New`, `watcher.New`, `api.NewServer`, `ResolveRunOptions()` (все 9 call sites)

**DOMAIN.md — документация домена для AI**
- Файл `data/leads/DOMAIN.md` автоматически инжектируется в `--append-system-prompt` задач с `domain: leads`
- Содержит: описание таблиц, правила дедупликации, примеры вызовов MCP-инструментов
- Агент получает полный контекст без хардкода в промптах задач

**Конфигурация `tasks.yaml`**
- Добавлена секция `domains.leads`: schema с таблицей leads (15 полей + unique index на tax_id)
- Добавлен MCP-сервер `database` (`./bin/mcp-database`)
- Задачи `find-leads`, `compile-leads-excel`, `deliver-leads-report` получили `domain: leads`
- `find-leads`: добавлен `mcp_servers: [database]`, расширен `allowed_tools` инструментами `mcp__database__*`
- `compile-leads-excel`: добавлен `mcp_servers: [database]` + `mcp__database__query` для чтения из БД вместо `{{.PrevOutput}}`

**Unit-тесты (38 тестов)**
- `internal/config/domain_test.go`: paths, config loading, validation, env expansion
- `internal/domain/manager_test.go`: Init, env vars, doc content, get domain
- `internal/mcpmanager/config_test.go`: env merging, nil env, delegation
- `cmd/mcp/mcp-database/main_test.go`: query, execute, check_exists, insert, list_tables, describe_table, SQL injection prevention, deduplication

### 2026-03-10 — Пайплайн export-by-aviation-to-ceo: sync + email + UI-фикс

**Объединение sync в пайплайн export-by-aviation-to-ceo**
- Добавлен `sync-export-by-catalog` как первый шаг пайплайна — данные всегда актуальны перед анализом
- Убран `schedule: 0 7 * * 1,3,5` у задачи `sync-export-by-catalog` — sync теперь запускается только как часть основного пайплайна
- Пайплайн `export-by-sync` оставлен без расписания для ручного запуска через API/CLI
- Порядок шагов: sync → analyze → compile → deliver

**Email-рассылка в deliver-шаге**
- Добавлен MCP-сервер `email` в задачу `deliver-export-by-aviation-report`
- Добавлен инструмент `mcp__email__send_report` в `allowed_tools`
- Промпт обновлён: после отправки в Telegram — email через `send_report` с Excel-файлом и сводкой

**Фикс: статус execution не обновлялся в реальном времени**
- Баг: после завершения пайплайна/задачи статус в Execution History оставался "running" до ручного обновления страницы
- Причина: `selected` state в `ExecutionHistory.tsx` — копия объекта на момент клика, не обновлялась при SSE-инвалидации кешей
- Фикс: `useEffect` синхронизирует `selected` с актуальными данными из `executions` query при рефетче
- Фикс: панель деталей использует `detailQuery.data ?? selected` — приоритет свежих данных с сервера

### 2026-03-12 — mcp-exportby: отклонение компаний + auto-filtering + улучшения пайплайнов

**mcp-exportby: новые инструменты и авто-фильтрация**
- Новый инструмент `mark_exported` — помечает все компании со статусом `new` как `reported` после отправки отчёта
- Новый инструмент `reject_companies` — помечает компании как отклонённые (импортёры, сервисные и т.д.) с указанием причины; они больше не появляются в `get_unanalyzed`
- Новая таблица `rejected_companies` (name UNIQUE, reason, rejected_at)
- Авто-фильтрация импортёров по ключевым словам в description (`containsImporterKeyword`) — дистрибьюторы, дилеры, салоны красоты, рестораны и т.д. автоматически отклоняются без участия LLM
- `get_unanalyzed`: запрашивает 3x лимит для компенсации авто-отклонённых, исключает `rejected_companies` через LEFT JOIN, группирует дубликаты по имени, добавлено поле `url` в ответ
- Обновлена схема в `tasks.yaml` (domains.export-by-aviation.schema): добавлена таблица `rejected_companies`

**Улучшение промптов задач**
- `analyze-export-by-catalog`: двухэтапная оценка (тип компании → приоритет авиаперевозки), явные критерии отклонения (импортёры, сервисные), обязательный вызов `reject_companies` для отклонённых, `max_turns: 20→50`, `max_budget_usd: 0.3→0.5`
- `compile-export-by-aviation-excel`: переписан в пошаговый формат, убран лист «Сводка» (сводка отправляется отдельно)
- `deliver-export-by-aviation-report`: пошаговый формат с точным шаблоном сводки, добавлен `mcp__exportby__mark_exported`, убраны `mcp__filesystem` и `mcp__database__execute`
- `deliver-leads-report`: аналогичный пошаговый формат с точным шаблоном сводки
- `compile-leads-excel`: `{{.Date}}` → `{{.DateTime}}` в имени файла
- Суб-агент `delivery-agent`: добавлены правила единого формата сводки (побайтовая идентичность TG и email)

**Шаблонная переменная `{{.DateTime}}`**
- Новая переменная `{{.DateTime}}` (формат `2006-01-02_15-04`) в пайплайнах и scheduler — для уникальных имён файлов при множественных запусках в день
- Добавлена в `internal/api/pipelines.go` и `internal/scheduler/scheduler.go`
- Scheduler теперь передаёт `Date` и `DateTime` в шаблоны (ранее передавал `nil`)

**Удаление execution через API и UI**
- Новый эндпоинт `DELETE /api/v1/executions/{id}` — удаление записи execution
- `internal/store/store.go`: метод `DeleteExecution` в интерфейсе `ExecutionStore`
- `internal/store/sqlite/queries.go`: реализация `DeleteExecution` с проверкой affected rows
- Web UI: кнопка удаления (✕) в таблице и панели деталей, компонент `ConfirmModal` для подтверждения

**Стабильность subprocess (task runner)**
- `setupCmdEnv()` — фильтрует `CLAUDECODE` из env дочерних процессов (предотвращает ошибку «nested session» в Claude CLI)
- Устанавливает `GIT_TERMINAL_PROMPT=0`, `GIT_SSH_COMMAND=ssh -o BatchMode=yes`, пустые `SSH_ASKPASS` и `DISPLAY` — подавление интерактивных SSH/git промптов в автоматических задачах
- Применяется в `Run()` и `RunStream()`

**Логирование пайплайнов**
- Детальное логирование каждого шага пайплайна: старт, завершение, ошибки, длительность, номер шага
- Логирование ошибок при resolve и task not found

**Кеширование и polling**
- `Cache-Control: no-store` на всех JSON-ответах API (`internal/api/helpers.go`)
- `cache: 'no-store'` в fetch-клиенте Web UI
- Восстановлен polling fallback: `refetchInterval: 5000` для списка (когда есть running), `refetchInterval: 3000` для деталей running execution — страховка на случай пропущенных SSE

**Логирование сервера**
- `server.log_file: logs/server.log` — логи пишутся в файл
- Автосоздание директории для лог-файла (`os.MkdirAll`)
- Исправлено переназначение logger после `setupLogger()` (ранее новый logger не использовался)

### 2026-03-13 — Контекстная фильтрация лидов export-by-aviation

**Проблема:** система оценивала приоритет авиаперевозки по категориям товаров (ключевые слова), а не по контексту. Замороженные продукты (ягоды, овощи, грибы) попадали в категорию «скоропортящееся» → aviation_priority=2, хотя реально перевозятся рефрижераторами, а не самолётами.

**Изменения в промпте `analyze-export-by-aviation` (tasks.yaml):**
- Этап 1 (тип компании): вместо списков ключевых слов — контекстная задача: «определи по смыслу описания, производит ли компания сама или перепродаёт». Слова-сигналы оставлены как ориентиры, а не исчерпывающие правила
- Этап 1.5 (новый — обогащение): если description < 50 символов или неоднозначное, и есть URL — разрешён WebFetch для получения профиля компании с её сайта
- Этап 2 (приоритет): вместо категорий — контекстный критерий: «есть ли экономическая причина платить x5–x10 за авиадоставку?». Явно указано: замороженные продукты = priority 0 (заморозка исключает потребность в срочной авиадоставке)
- WebFetch добавлен в allowed_tools задачи
- Убрано ограничение «НЕ используй WebFetch» (заменено на «WebFetch разрешён ТОЛЬКО для обогащения профиля»)

**Фильтрация по стране (mcp-exportby + промпт):**
- Go-код (`handleGetUnanalyzed`): компании с `country != "BY"` автоматически отклоняются с причиной `auto:non_by_country` до передачи LLM
- Промпт: добавлен фильтр «иностранная компания → ОТКАЗ» как первый шаг этапа 1 (страховка для случаев когда country=BY, но компания реально из другой страны — напр. г. Кострома в описании)
- Причина: китайские и российские компании регистрируются на export.by, но не являются целевыми лидами

**Изменения в агенте `export-by-scraper.md`:**
- Критерии приоритетов синхронизированы с новым контекстным подходом
- Добавлено явное правило: замороженное ≠ скоропортящееся

### 2026-03-13 — Batch-накопление лидов и единый процесс обработки

**Проблема:** пайплайн из 4 шагов (sync → analyze → compile Excel → deliver) отправлял отчёт при каждом запуске, даже если найден всего 1 лид. Неэффективно для получателя и тратит ресурсы на генерацию/отправку малых порций.

**Новая архитектура: accumulate & flush**
- Лиды накапливаются в таблице `companies` (status='new') между запусками
- Отправка срабатывает только при достижении порога (15 лидов, настраивается в промпте задачи)
- Excel генерируется программно в Go (mcp-exportby), без отдельного вызова Claude

**Изменения в mcp-exportby (cmd/mcp/mcp-exportby/main.go):**
- Новый инструмент `get_pending_count` — возвращает количество лидов со status='new'
- Новый инструмент `export_leads_excel` — генерирует стилизованный Excel (excelize) из всех pending лидов, возвращает путь к файлу + статистику (total, high_priority, med_priority)
- `mark_exported` обновлён: status='new' → 'sent' (вместо 'reported'), записывает `sent_at`
- Миграция: `ALTER TABLE companies ADD COLUMN sent_at TEXT` в `ensureSchema()`

**Изменения в tasks.yaml:**
- Удалены 3 задачи: `analyze-export-by-aviation`, `compile-export-by-aviation-excel`, `deliver-export-by-aviation-report`
- Новая задача `process-export-by-leads` — единый процесс с условной логикой:
  1. Проверяет pending count
  2. Если >= порог → export Excel → отправка TG + email → mark sent
  3. Если < порог → анализ новой порции raw_companies → повторная проверка → отправка если >= порог
- Пайплайн `export-by-aviation-to-ceo` упрощён до 2 шагов: `sync-export-by-catalog` → `process-export-by-leads`
- Схема companies: добавлено поле `sent_at TEXT`
- Домен: убран `excel` из mcp_servers (Excel теперь генерит exportby), добавлен `telegram`

**Обновлена документация:**
- DOMAIN.md: новая архитектура, статусы new/sent, новые инструменты
- workflow.md, user-guide.md

### 2026-03-13 — Уточнение критериев aviation_priority + веб-исследование при сомнениях

**Проблема:** «Агрокомбинат Несвижский» (мясо, овощи, живая рыба) получил aviation_priority=2 — LLM трактовал пищевую продукцию как «скоропортящееся» и назначал высокий приоритет, хотя такие грузы перевозятся рефрижераторами.

**Уточнение критериев priority 2 (tasks.yaml + DOMAIN.md):**
- Убрано размытое «скоропортящееся» из priority 2
- Расширен список: живые растения (цветы, саженцы, рассада, декоративные, элитный семенной материал), биоматериалы (образцы, реагенты, культуры)
- Добавлено явное правило: пищевая/сельхоз продукция (мясо, рыба, птица, овощи, молочка, зерно, корма) = priority 0 (ОТКАЗ)
- Агрокомбинаты, мясокомбинаты, молокозаводы, птицефабрики, рыбхозы → ОТКАЗ
- Исключение из пищевого отказа: живые растения и элитный семенной материал (priority 2)

**Веб-исследование при сомнениях в классификации (tasks.yaml):**
- Этап 1.5 расширен: модель обязана исследовать компанию через веб не только при коротком description, но и при любых сомнениях (производитель vs импортёр, тип продукции, корректность aviation_priority)
- Порядок: WebFetch по URL компании → WebSearch по названию (если WebFetch не дал ясности)
- `WebSearch` добавлен в `allowed_tools` задачи `process-export-by-leads`

**Очистка данных:**
- «Агрокомбинат Несвижский» удалён из `companies`, перенесён в `rejected_companies` (reason: `food_agriculture_producer`)

### 2026-03-13 — Cron-расписание для пайплайнов

**Проблема:** пайплайны можно было запускать только вручную (через API, CLI или Web UI). Поле `schedule` было доступно только для задач (tasks), но не для пайплайнов.

**Изменения:**

**Backend:**
- `internal/config/pipeline.go`: добавлено поле `Schedule string` в структуру `Pipeline`
- `internal/scheduler/scheduler.go`: новый метод `RegisterPipeline(p, runFn)` — принимает pipeline config и callback-функцию для запуска; поддерживает pause/resume по имени пайплайна
- `internal/api/pipelines.go`: новый публичный метод `RunPipelineByName(ctx, name, trigger)` — создаёт execution record, публикует SSE-события, выполняет пайплайн; trigger записывается как "schedule"
- `cmd/server/main.go`: пайплайны с `schedule != ""` регистрируются в cron-шедулере после инициализации API-сервера

**Frontend:**
- `web/src/types/index.ts`: добавлено `schedule?: string` в интерфейс `Pipeline`
- `web/src/components/PipelineList.tsx`: поле "Schedule (cron)" в форме редактирования, отображение расписания в карточке пайплайна

**Использование в tasks.yaml:**
```yaml
pipelines:
  - name: export-by-aviation-to-ceo
    mode: sequential
    steps:
      - task: sync-export-by-catalog
      - task: process-export-by-leads
    max_iterations: 1
    schedule: "0 9 * * 1-5"   # каждый будний день в 9:00
```

### 2026-03-14 — Защита от параллельного запуска (allow_concurrent)

**Проблема:** при cron-расписании с коротким интервалом или ручном запуске задача/пайплайн могли запускаться повторно, пока предыдущий экземпляр ещё выполняется. Это приводило к двойной работе, конфликтам в БД и дублированию отчётов.

**Решение: флаг `allow_concurrent`**

Новое опциональное поле `allow_concurrent` (bool) для задач и пайплайнов. По умолчанию `true` (обратная совместимость). При `allow_concurrent: false` повторный запуск блокируется, если предыдущий ещё выполняется.

**Изменения:**

**Конфигурация (`internal/config/`):**
- `task.go`: новое поле `AllowConcurrent *bool` + метод `ConcurrentAllowed()` (default true)
- `pipeline.go`: аналогичное поле и метод

**Новый пакет `internal/runguard/`:**
- Mutex-based guard для отслеживания запущенных задач/пайплайнов
- `TryAcquire(name)` / `Release(name)` — атомарная проверка и блокировка
- Единый экземпляр `Guard` разделяется между scheduler, watcher и API
- Namespace ключей: `task:<name>`, `pipeline:<name>` — исключает коллизии

**Scheduler (`internal/scheduler/`):**
- Cron-запуск проверяет guard перед выполнением: если задача/пайплайн уже запущены — пропускает с логом `"already running, skipping"`

**API (`internal/api/`):**
- `POST /tasks/:name/run` и `/run-async`: возвращают `409 Conflict` с сообщением `"task is already running"` при попытке параллельного запуска
- `POST /pipelines/:name/run` и `/run-async`: аналогично для пайплайнов
- `RunPipelineByName()` (вызов из scheduler): пропускает с логом

**Watcher (`internal/watcher/`):**
- При file-change проверяет guard: если задача уже выполняется — пропускает

**Wiring (`cmd/server/main.go`):**
- Создание `runguard.New()` и передача в scheduler, watcher и API server

**Использование в `tasks.yaml`:**
```yaml
tasks:
  - name: long-running-task
    prompt: "..."
    allow_concurrent: false
    schedule: "*/5 * * * *"

pipelines:
  - name: daily-pipeline
    allow_concurrent: false
    schedule: "0 9 * * 1-5"
    steps:
      - task: step-one
      - task: step-two
```

### 2026-03-14 — Исправление cron-запуска пайплайнов с allow_concurrent: false

**Проблема:** пайплайн `export-by-aviation-to-ceo` с `allow_concurrent: false` и cron-расписанием `*/15 * * * *` никогда не выполнялся. В логах: `"scheduled pipeline starting"` → `"pipeline is already running, skipping"` — при каждом срабатывании cron.

**Причина:** двойной `TryAcquire` на один и тот же ключ `"pipeline:<name>"`. Guard (runguard) не реентерабельный (`map[string]bool`):
1. `scheduler.RegisterPipeline()` захватывал guard в cron-callback
2. Вызывал `RunPipelineByName()`, которая пыталась захватить тот же guard повторно → всегда `false` → skip

**Фикс:** убран guard из `RegisterPipeline` в `internal/scheduler/scheduler.go` — он избыточен, т.к. `RunPipelineByName` (в `internal/api/pipelines.go`) уже самостоятельно управляет concurrency guard. Для задач (tasks) проблемы не было: guard только в scheduler, `runner.Run()` его не дублирует.

### 2026-03-14 — UI: allow_concurrent + Hot reload конфига

**UI: настройка allow_concurrent**
- Добавлено поле `allow_concurrent?: boolean` в TypeScript-интерфейсы `Task` и `Pipeline` (`web/src/types/index.ts`)
- Чекбокс "Allow concurrent runs" в `TaskEditor` (сетка настроек, рядом с Max Budget)
- Чекбокс "Allow concurrent runs" в `PipelineEditor` (после поля Schedule)
- Логика: включен по умолчанию (соответствует бэкенду где `nil` = `true`); при снятии галочки отправляется `allow_concurrent: false`; при установке — поле убирается из JSON (`undefined`)

**Hot reload tasks.yaml**
- `internal/scheduler/scheduler.go`: новый метод `Reset()` — останавливает текущий cron, создаёт новый, запускает; pause-состояния сохраняются
- `internal/watcher/watcher.go`: новый метод `Reset()` — очищает список задач и убирает все fsnotify-watch'и
- `cmd/server/main.go`: новая горутина `watchConfigFile()` — следит за директорией конфиг-файла через fsnotify, фильтрует по имени файла, debounce 1 секунда
- `cmd/server/main.go`: функция `reloadConfig()` — перечитывает `tasks.yaml` через `config.Load()` (с валидацией), обновляет `cfg.Tasks` и `cfg.Pipelines` на shared pointer, пересоздаёт scheduler (все cron-задачи и пайплайны перерегистрируются), пересоздаёт watcher (все file-watch задачи перерегистрируются)
- Публикуется SSE-событие `config.reloaded` с количеством задач и пайплайнов
- Следит за директорией (не за файлом) для корректной работы со стратегиями сохранения редакторов (vim: write-rename)
- Изменения через API (`POST /tasks`, `PUT /tasks/:name`) автоматически подхватываются: API сохраняет на диск → fsnotify → reload → scheduler/watcher обновлены

### 2026-03-16 — Git: переход на ветку main + исправление credential helper

**Переименование основной ветки `master` → `main`**
- Локальная ветка `master` переименована в `main` (`git branch -m master main`)
- `main` запушена на GitHub с force-with-lease (перезаписала старую `main` с единственным Initial commit)
- Ветка `master` удалена с remote (`git push origin --delete master`)
- Дефолтная ветка на GitHub — `main`
- Причина расхождения: GitHub создал `main` при инициализации репозитория, а локальный `git init` (без `init.defaultBranch`) создал `master`. Вся разработка шла в `master`, `main` на GitHub содержала только 1 коммит

**Исправление git credential helper**
- Баг: предупреждение `git: 'credential-!gh' is not a git command` при каждой git-операции с remote
- Причина: в `~/.gitconfig` была лишняя секция `[credential]` с `helper = \!gh auth git-credential` (без полного пути) — вероятно добавлена вручную
- Секции для `github.com` и `gist.github.com` уже содержали корректный `helper = !/usr/bin/gh auth git-credential`
- Фикс: удалена дублирующая глобальная секция `[credential]`

### 2026-03-15 — Дедупликация по export_by_id + SSE reconnect + оптимизация промпта

**Дедупликация компаний по export_by_id (mcp-exportby)**
- Баг: дедупликация лидов в `get_unanalyzed` работала только по имени компании (`r.name = c.name`). Если компания меняла название на export.by, она обрабатывалась повторно
- Фикс: добавлено поле `export_by_id INTEGER` в таблицу `companies` (миграция через `ALTER TABLE`)
- `get_unanalyzed`: JOIN расширен до `r.name = c.name OR r.export_by_id = c.export_by_id` — дедупликация и по имени, и по ID каталога
- Счётчик `totalUnanalyzed` использует тот же расширенный JOIN
- Промпт `process-export-by-leads`: INSERT включает `export_by_id` из ответа `get_unanalyzed`
- Схема домена `export-by-aviation` в `tasks.yaml`: добавлено поле `export_by_id INTEGER`

**SSE: обновление данных при reconnect и возврате на вкладку (useSSE.ts)**
- Баг: при разрыве SSE-соединения (потеря сети, спящий режим) события терялись — UI показывал устаревшие данные до следующего события
- Фикс 1: при reconnect (если `wasConnected = true`) — автоматическая инвалидация кешей `executions` и `dashboard`
- Фикс 2: `visibilitychange` listener — при возврате на вкладку браузера инвалидируются кеши (ловит случаи когда вкладка была неактивна)
- Рефакторинг: `invalidateAll()` вынесен в `useCallback` для переиспользования

**Оптимизация промпта process-export-by-leads**
- Ранний выход: шаг 1 одновременно запрашивает `get_pending_count` и `get_unanalyzed(limit=1)` — если pending < 15 и unanalyzed = 0, завершается сразу без лишних tool calls
- Точность имён: добавлена инструкция сохранять name точно как в `get_unanalyzed` (не сокращать «Открытое акционерное общество» до «ОАО»)
- Нумерация шагов выровнена (убрана путаница 3→4)

**Убрано cron-расписание пайплайна export-by-aviation-to-ceo**
- Удалён `schedule: '*/15 * * * *'` — пайплайн запускается вручную или по необходимости

### 2026-03-17 — MCP-серверы (PDF, Word) + Live streaming + MCP UI + Unit-тесты

**mcp-pdf — полная реализация**
- 3 инструмента: `read_pdf` (метаданные + текст), `extract_text` (извлечение текста с поддержкой page ranges), `extract_tables` (best-effort извлечение таблиц в JSON/CSV)
- Библиотека: `github.com/ledongthuc/pdf` (pure Go)
- Поддержка диапазонов страниц: `1-5`, `1,3,7`, пустой = все
- Page range парсер с валидацией

**mcp-word — полная реализация**
- 3 инструмента: `read_document` (чтение текста с опциональным форматированием), `write_document` (append/replace), `create_document` (создание с нуля)
- Реализация через stdlib: `archive/zip` + `encoding/xml` (без внешних зависимостей)
- Парсинг `word/document.xml`: извлечение `<w:t>`, определение bold/italic/heading стилей
- Создание минимального валидного .docx: `[Content_Types].xml`, `_rels/.rels`, `word/document.xml`

**Live output streaming в Execution History**
- Бэкенд: `handleRunTaskAsync` переключен на `RunStream()` — публикует `task.output` SSE-события с каждым чанком вывода Claude CLI
- Фронтенд: при выборе running execution — подключение к `/api/v1/executions/{id}/stream` через `EventSource`
- Live-панель: зелёный терминальный вывод с auto-scroll, пульсирующий индикатор "Live Output"
- `streamExecution()` обновлён: слушает именованные SSE-события (`task.output`, `task.completed`)
- Автоотключение при завершении задачи, очистка при смене выбранного execution

**Управление MCP-серверами через Web UI**
- Новый компонент `MCPServerList.tsx`: таблица серверов со статусом, PID, кнопками Start/Stop
- Статистика: общее количество, запущенные, остановленные
- Зелёный/серый индикатор статуса, авто-обновление каждые 10 секунд
- Новый пункт навигации "MCP Servers" в sidebar
- Маршрут `/mcp-servers` в App.tsx

**Unit-тесты (45+ новых тестов, 6 пакетов)**
- `internal/runguard/` (4 теста): acquire/release, namespace isolation, concurrent access
- `internal/events/` (6 тестов): pub/sub, multiple subscribers, panic recovery, Wait blocking
- `internal/auth/` (8 тестов): PASETO create/validate, expiration, tampering, key generation
- `internal/subagent/` (7 тестов): parse, serialize, roundtrip, ToAgentsJSON, CRUD manager
- `internal/task/` (16 тестов): BuildArgs (все флаги), renderPrompt, parseJSONOutput
- `internal/store/sqlite/` (9 тестов): execution CRUD, list с фильтрами, user CRUD, pipeline execution
- Makefile: `make test` расширен на `./cmd/...`

### 2026-03-17 — mcp-openapi: динамические MCP-инструменты из OpenAPI-спецификаций

**Новый MCP-сервер `mcp-openapi` (`cmd/mcp/mcp-openapi/main.go`)**
- Единый MCP-сервер, который при запуске читает OpenAPI v3 спецификацию (JSON/YAML) и динамически генерирует MCP-инструменты для каждого эндпоинта API
- Библиотека: `github.com/pb33f/libopenapi` v0.34.3 (поддержка OpenAPI v2/v3/v3.1, разрешение $ref)
- Именование инструментов: из `operationId` (lowercase, sanitize) или `method_path` если operationId отсутствует
- InputSchema генерируется из параметров операции: path → required, query → optional, header → prefix `header_`, body → свойство `body` с вложенной JSON Schema (глубина до 3 уровней)
- Фильтрация: по тегам (`OPENAPI_INCLUDE_TAGS`), path-префиксам (`OPENAPI_INCLUDE_PATHS`), operationId (`OPENAPI_INCLUDE_OPS`/`OPENAPI_EXCLUDE_OPS`)
- Auth: bearer, API key (header/query), basic auth — через env vars
- Лимиты: макс. 50 инструментов (настраивается), ответ до 1MB, HTTP timeout 30s
- Множественные API — отдельные инстансы с разными env vars в `mcp_servers`
- Makefile подхватывает автоматически (цикл `for dir in cmd/mcp/*/`)
- Протестировано на Petstore spec (19 инструментов, live HTTP вызовы)

**Wizard: создание mcp-openapi серверов**
- Новый тип `MCPServerPlan` в `internal/wizard/types.go` (name, command, args, env)
- Поле `MCPServers []MCPServerPlan` в `Plan` — wizard теперь может создавать новые MCP-серверы (не только ссылаться на существующие)
- JSON schema в prompt.go: новая секция `mcp_servers` для плана
- Applier: MCP-серверы создаются первым шагом (до доменов), валидация дубликатов, задачи могут ссылаться на серверы из того же плана
- Prompt: документация по mcp-openapi (env vars, паттерн именования инструментов, пример конфигурации), правило #2 обновлено
- Динамическое отображение env vars openapi-серверов в секции "Available MCP Servers"
- Web UI (`Wizard.tsx`): секция "MCP Servers" в preview с env vars, отображение созданных серверов в результатах
- TypeScript: `MCPServerPlan` и `mcp_servers_created` в `ApplyResult`

### 2026-03-17 — mcp-openapi: OAuth2 client credentials flow

**Проблема:** `mcp-openapi` поддерживал только статичные методы аутентификации (bearer token, API key, basic auth). API с динамической авторизацией (api_key + api_secret → access_token + refresh_token) не поддерживались.

**Решение: новый auth type `oauth2`**

Реализован полный OAuth2 client credentials flow с автоматическим управлением токенами:

**Новые env-переменные:**
| Переменная | Обяз. | Описание |
|---|---|---|
| `OPENAPI_AUTH_TYPE` | Да | `oauth2` |
| `OPENAPI_AUTH_ENDPOINT` | Да | URL для получения токена (POST) |
| `OPENAPI_REFRESH_ENDPOINT` | Нет | URL для refresh токена (POST) |
| `OPENAPI_CLIENT_ID` | Да | Client ID / API key |
| `OPENAPI_CLIENT_SECRET` | Да | Client secret / API secret |

**Логика работы:**
1. При старте — `POST auth_endpoint` с `client_id` + `client_secret` + `grant_type=client_credentials` → получение `access_token` + `refresh_token`
2. Проактивный refresh — если до истечения токена < 30 сек, обновляет заранее через `getToken()`
3. Retry на 401 — при получении HTTP 401 автоматически вызывает refresh и повторяет запрос один раз
4. Fallback — если refresh не удался (expired refresh token, 400/401 от refresh endpoint), делает полную повторную авторизацию через `auth_endpoint`
5. Если `OPENAPI_REFRESH_ENDPOINT` не задан — при необходимости refresh всегда использует полную re-авторизацию

**Реализация (`cmd/mcp/mcp-openapi/main.go`):**
- Тип `oauth2TokenManager` — потокобезопасный (sync.Mutex) менеджер токенов
- Методы: `authenticate()`, `refresh()`, `getToken()`, `doTokenRequest()`
- `applyAuth()` возвращает `error` (вместо void) — для корректной обработки ошибок OAuth2
- `executeOperation()` → `doExecute()` — разделение для поддержки retry на 401
- `doExecute()` возвращает `(toolResult, statusCode)` — для проверки 401

**Unit-тесты (`cmd/mcp/mcp-openapi/oauth2_test.go`, 6 тестов):**
- Authenticate (успешный token exchange)
- Refresh (успешный refresh_token → новый access_token)
- GetToken с проактивным refresh (near-expiry)
- GetToken с валидным токеном (без refresh)
- Authenticate с невалидными credentials (401)
- Refresh fallback на re-auth (expired refresh token → full re-auth)

**Пример конфигурации:**
```yaml
mcp_servers:
  - name: my-api
    command: ./bin/mcp-openapi
    env:
      OPENAPI_SPEC_PATH: specs/my-api.yaml
      OPENAPI_AUTH_TYPE: oauth2
      OPENAPI_AUTH_ENDPOINT: https://api.example.com/auth/token
      OPENAPI_REFRESH_ENDPOINT: https://api.example.com/auth/refresh
      OPENAPI_CLIENT_ID: ${MY_API_KEY}
      OPENAPI_CLIENT_SECRET: ${MY_API_SECRET}
```

### 2026-03-18 — Пайплайн vet-manufacturers-sync: оптимизация исследования и session_id

**Новый пайплайн vet-manufacturers-sync**
- 3-шаговый пайплайн: `research-vet-manufacturers` → `compile-vet-manufacturers-excel` → `deliver-vet-manufacturers-report`
- Домен `vet-manufacturers-belarus`: SQLite БД с таблицами `manufacturers` (30 полей вкл. экспорт, авиатранспорт) и `sync_log`
- Агент-исследователь `vet-manufacturers-researcher.md`: WebSearch/WebFetch по официальным реестрам РБ, реестрам компаний, торговым базам
- Отчёт в Telegram: текстовое резюме + Excel-файл (3 листа: все производители, экспортёры, авиаэкспортёры)

**Оптимизация бюджета и стратегии исследования**
- Бюджет: $2 → **$8** (первый запуск исчерпывал $2 до завершения сбора данных → 0 результатов в БД)
- `max_turns`: 40 → **25** (фокусировка вместо распыления на все источники)
- Инкрементальное сохранение: агент сохраняет каждого найденного производителя в БД **сразу** после обнаружения (не накапливает) — при исчерпании бюджета данные уже в БД
- Приоритизация источников в 3 фазы: 1) официальные реестры, 2) реестры компаний, 3) экспорт/авиа (если остался бюджет)

**Дедупликация при повторных запусках**
- Шаг 0 в промпте: `SELECT name FROM manufacturers` — агент загружает уже известных и не тратит бюджет на повторный поиск
- Фокус на поиске НОВЫХ компаний, обновление существующих только при новой информации об экспорте

**Session_id для фильтрации по запуску (не по дате)**
- Добавлена колонка `session_id TEXT` в таблицы `manufacturers` и `sync_log`
- Формат: `vet-YYYYMMDD-HHMMSS` (например `vet-20260318-143052`) — уникален для каждого запуска
- Research-задача генерирует session_id, проставляет на все INSERT/UPDATE, выводит `SESSION_ID: ...` первой строкой
- Excel-задача парсит session_id из `{{.PrevOutput}}` → `WHERE session_id = '...'` — выгружает только лиды текущего сеанса
- Deliver-задача отчитывается о новых за сеанс + итого в БД
- Решает проблему: при множественных ручных запусках в день фильтрация по дате давала бы дубли

**Результат:** 30 производителей ветпрепаратов РБ в БД (8 экспортёров, 1 авиаэкспортёр — ОАО «БелВитунифарм»)

### 2026-03-18 — Session chaining для sequential pipelines

**Новая возможность: `session_chain`**
- Поле `session_chain: true` в конфигурации sequential pipeline
- При включении каждый шаг получает `--resume <session_id>` от предыдущего шага
- Агент сохраняет полный контекст разговора между шагами — не нужно передавать весь `{{.PrevOutput}}`
- Opt-in: без `session_chain: true` поведение pipeline не меняется

**Затронутые файлы:**
- `config/pipeline.go` — поле `SessionChain bool`
- `config/validate.go` — валидация: `session_chain` только для `sequential` mode + все шаги должны иметь одинаковый `work_dir`
- `pipeline/sequential.go` — передача `prevSessionID` между шагами через `RunOptions.ResumeSessionID`
- `wizard/types.go` — поле `SessionChain` в `PipelinePlan`
- `wizard/prompt.go` — описание и JSON schema для wizard (Claude знает про `session_chain`)
- `wizard/applier.go` — передача `SessionChain` при создании pipeline из плана

**Ограничения:**
- Только `mode: sequential` (валидация блокирует parallel)
- Все шаги pipeline должны иметь одинаковый `work_dir` (сессия привязана к проекту)
- `--resume` может игнорировать `--model`, `--agents`, `--mcp-config` при смене между шагами

### 2026-03-18 — MCP-сервер mcp-whisper: транскрипция аудио через Whisper.cpp

**Новый MCP-сервер `mcp-whisper`** — транскрипция аудиофайлов через whisper.cpp (локально, без внешних API).

**Инструменты:**
- `transcribe_audio` — транскрипция аудио (WAV, MP3, FLAC, OGG, M4A). Параметры: `path`, `language` (auto/ru/en/...), `translate` (перевод на EN), `output_format` (text/srt/vtt). Не-WAV форматы конвертируются через ffmpeg
- `list_models` — список доступных и скачанных моделей (tiny, base, small, medium, large-v3)
- `download_model` — скачивание модели с Hugging Face

**Конфигурация:**
```yaml
mcp_servers:
  - name: whisper
    command: ./bin/mcp-whisper
    env:
      WHISPER_BIN: ./data/whisper/bin/whisper-cli
      WHISPER_MODEL: ./data/whisper/models/ggml-small.bin
      WHISPER_MODELS_DIR: ./data/whisper/models
      WHISPER_THREADS: "8"
```

**Установка Whisper.cpp через Makefile:**
```bash
make setup-whisper
```
- Клонирует whisper.cpp, компилирует через cmake, скачивает модель `ggml-small.bin`
- Бинарник: `data/whisper/bin/whisper-cli`, модели: `data/whisper/models/`

**Новые задачи и пайплайн:**
- `transcribe-audio` — транскрипция аудиофайла (model: haiku, timeout: 60m)
- `summarize-transcription` — анализ транскрипции: резюме, ключевые темы, action items, участники
- Пайплайн `meeting-notes` — sequential с `session_chain: true`: транскрипция → анализ

**Затронутые файлы:**
- `cmd/mcp/mcp-whisper/main.go` — реализация MCP-сервера (JSON-RPC 2.0, stdio)
- `Makefile` — target `setup-whisper` для сборки whisper.cpp и скачивания модели
- `tasks.yaml` — задачи `transcribe-audio`, `summarize-transcription`, пайплайн `meeting-notes`, MCP-сервер `whisper`
- `.gitignore` — добавлена директория `logs/`

### 2026-03-18 — Миграция MCP-серверов на mcp-go

**Проблема:** 11 MCP-серверов дублировали ~70 строк идентичного JSON-RPC 2.0 boilerplate каждый: типы `jsonRPCRequest`/`jsonRPCResponse`/`tool`, `bufio.Scanner` main loop с `switch` по methods (`initialize`, `tools/list`, `tools/call`), хелперы `textResult()`/`errorResult()`. Итого ~900 строк копипасты. Ручная реализация не обрабатывала edge cases (notifications без id, `ping`, `notifications/initialized`).

**Решение: библиотека [mcp-go](https://github.com/mark3labs/mcp-go) v0.45.0**

Каждый сервер вместо ~70 строк протокольного кода использует 3-5 строк:
```go
s := server.NewMCPServer("mcp-filesystem", "0.1.0")
s.AddTool(mcp.NewTool("read_file",
    mcp.WithDescription("Read file contents"),
    mcp.WithString("path", mcp.Required(), mcp.Description("File path")),
), handleReadFile)
server.ServeStdio(s)
```

**Мигрированные серверы (11 из 11):**
- `mcp-filesystem`, `mcp-pdf`, `mcp-word` — простые, read/write инструменты
- `mcp-email`, `mcp-telegram` — write-only, env-dependent
- `mcp-excel`, `mcp-whisper` — средней сложности
- `mcp-google` — stub (not implemented)
- `mcp-database`, `mcp-exportby` — сложные, с package-level `*sql.DB` и `ensureSchema()`
- `mcp-openapi` — самый сложный: динамические tools из OpenAPI spec, OAuth2 token management, 401 retry. Использует `mcp.NewToolWithRawSchema()` для generated schemas + closure `makeToolHandler()` для per-tool dispatch

**Что удалено из каждого сервера:**
- Типы `jsonRPCRequest`, `jsonRPCResponse`, `tool` (и варианты `mcpTool`, `toolCallParams`, `contentItem`, `toolResult`)
- Хелперы `textResult()`, `errorResult()`
- Dispatcher `handleToolCall(params)`
- Main loop: `bufio.Scanner` + `json.Encoder` + `switch req.Method`
- Import `"bufio"` (и часто `"encoding/json"`)

**Что сохранено:** вся бизнес-логика каждого сервера без изменений.

**Обновлённые тесты:**
- `cmd/mcp/mcp-database/main_test.go` — адаптирован к новым сигнатурам handler'ов (`func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error)`), добавлены хелперы `makeReq()` и `resultText()`
- `cmd/mcp/mcp-openapi/oauth2_test.go` — без изменений (тестирует `oauth2TokenManager`, не затронут миграцией)

**Верификация:**
- `make build` — все 13 бинарников собираются (server, hook, 11 MCP)
- `make test` — все тесты проходят
- Ручное тестирование всех 11 серверов через stdin/stdout: `initialize` → `tools/list` → `tools/call` — корректные JSON-RPC ответы

**Новая зависимость:** `github.com/mark3labs/mcp-go v0.45.0` (транзитивные: jsonschema, cast, uritemplate)

**Бонусы mcp-go:**
- Полное соответствие MCP spec (поддержка `ping`, notifications, `protocolVersion` negotiation)
- Параллельная обработка tool calls (worker pool)
- Типобезопасное извлечение аргументов: `req.RequireString()`, `req.GetInt()`, `req.BindArguments()`
- Готовность к будущим MCP features (Resources, Prompts, Middleware)

### 2026-03-19 — mcp-openapi: OAuth2 с настраиваемыми полями, download_file, Yeastar интеграция, SSE fix

**Исправление SSE стриминга (500 на /api/v1/events и /executions/{id}/stream)**
- Баг: `requestLogger` middleware оборачивал `http.ResponseWriter` в `statusWriter`, который не реализовывал `http.Flusher` → SSE handlers всегда возвращали 500 "streaming not supported"
- Фикс: добавлен метод `Flush()` к `statusWriter` (`internal/api/router.go`) — делегирует вызов оригинальному `ResponseWriter`

**mcp-openapi: настраиваемый OAuth2 для нестандартных API**

Расширен OAuth2 flow для поддержки API с нестандартной авторизацией (например, Yeastar PBX: token в query param, username/password вместо client_id/client_secret):

| Новая переменная | Описание | Default |
|---|---|---|
| `OPENAPI_OAUTH2_TOKEN_URL` | URL получения токена (заменяет `OPENAPI_AUTH_ENDPOINT`) | — |
| `OPENAPI_OAUTH2_CLIENT_ID` | Логин (заменяет `OPENAPI_CLIENT_ID`) | — |
| `OPENAPI_OAUTH2_CLIENT_SECRET` | Пароль (заменяет `OPENAPI_CLIENT_SECRET`) | — |
| `OPENAPI_OAUTH2_REFRESH_URL` | URL refresh токена | — |
| `OPENAPI_OAUTH2_ID_FIELD` | Имя поля в JSON body для логина | `client_id` |
| `OPENAPI_OAUTH2_SECRET_FIELD` | Имя поля в JSON body для пароля | `client_secret` |
| `OPENAPI_OAUTH2_GRANT_TYPE` | Значение grant_type (пустая строка = не включать) | `client_credentials` |
| `OPENAPI_OAUTH2_TOKEN_IN` | Куда инжектировать токен: `header` или `query` | `header` |
| `OPENAPI_OAUTH2_TOKEN_PARAM` | Имя query param при `TOKEN_IN=query` | `access_token` |

- Auth type `oauth2_client_credentials` — alias для `oauth2` с теми же env vars
- Парсинг Yeastar-формата expiry: `access_token_expire_time` (в дополнение к `expires_in`)
- 401 retry упрощён: проверяет наличие `tokenManager` вместо `authType == "oauth2"`
- Обратная совместимость: старые env vars (`OPENAPI_AUTH_ENDPOINT`, `OPENAPI_CLIENT_ID`, `OPENAPI_CLIENT_SECRET`) работают как fallback

**mcp-openapi: TLS insecure mode**
- Новая переменная `OPENAPI_TLS_INSECURE=true` — отключает проверку TLS-сертификата (самоподписанные сертификаты)
- `crypto/tls` import, `http.Transport` с `InsecureSkipVerify`

**mcp-openapi: встроенный инструмент `download_file`**
- Новый tool `download_file` — скачивает файл по URL и сохраняет на диск (для бинарных файлов: аудио, изображения и т.д.)
- Параметры: `url` (полный или относительный — автоматически дополняется base URL), `path` (локальный путь)
- Автоматически применяет auth (Bearer/query token) и extra headers
- Создаёт родительские директории при необходимости
- Для относительных URL: strip `/openapi/...` suffix из base URL (для download URLs PBX)

**OpenAPI спецификация Yeastar P-Series (`specs/yeastar-pseries.yaml`)**
- 4 эндпоинта (get_token убран — auth автоматический): `searchCDR`, `listCDR`, `listRecordings`, `downloadRecording`
- Полные response schemas: CDRRecord (25+ полей), RecordingRecord, RecordingDownloadResponse
- Параметры access_token убраны из спеки — MCP сервер добавляет автоматически
- Данные из официальной документации Yeastar P-Series Cloud Edition (help.yeastar.com)

**Пайплайн daily-sip-processing**
- Новый домен `sip-calls`: таблицы `recordings` и `daily_reports`
- 4 задачи: `fetch-sip-recordings` (скачивание WAV), `transcribe-sip-recordings`, `summarize-sip-transcriptions`, `deliver-sip-report`
- Промпт `fetch-sip-recordings`: auth автоматический, `listrecordings` + `downloadrecording` + `download_file`
- `allowed_tools` для всех Yeastar API и filesystem/database инструментов

**Wizard: документация OAuth2 и download_file**
- `internal/wizard/prompt.go`: полная документация по OAuth2 env vars (core, simple auth, oauth2, filtering)
- Два примера mcp_servers в промпте: простой bearer и OAuth2 Yeastar-style
- `download_file` описан в секции openapi tool names
- Отображение `OPENAPI_AUTH_TYPE`, `OPENAPI_OAUTH2_TOKEN_IN`, `OPENAPI_TLS_INSECURE` в секции "Available MCP Servers"
- `internal/wizard/envspec.go`: все 22 env vars для mcp-openapi в реестре

### 2026-03-19 — Пайплайн daily-sip-processing: Word-отчёт, batch-инструменты, large-v3-turbo, SRT на диске

**Формат отчёта: Word-документ вместо текстового сообщения Telegram**
- Шаг `deliver-sip-report` полностью переработан: создаёт `.docx` файл с расшифровками в SRT-формате + краткое резюме на русском
- Структура документа: заголовок, статистика, дайджест дня, затем по каждому звонку — краткое содержание, темы, задачи, полная расшифровка SRT
- Отправка `.docx` через Telegram `send_document` вместо текстового `send_message`
- Добавлен MCP-сервер `word` (`./bin/mcp-word`) в конфигурацию и домен `sip-calls`

**Batch-инструменты для экономии токенов**
- `batch_download` в mcp-openapi — скачивание всех файлов за 1 tool-call вместо N отдельных `download_file`
- `batch_insert` в mcp-database — вставка всех записей в одной транзакции за 1 tool-call
- `batch_transcribe` в mcp-whisper — транскрибирование всех файлов за 1 tool-call. Идемпотентно: пропускает файлы с существующим `.srt` на диске (status: `skipped`)
- Промпт `fetch-sip-recordings` переписан: собирает URL через `downloadrecording`, скачивает все через `batch_download`, вставляет через `batch_insert`. ~15 tool-calls вместо ~130
- Промпт `transcribe-sip-recordings`: модель sonnet → **haiku**, бюджет $5 → **$0.50**, max_turns 200 → **30**. Claude делает 3 tool-call (SELECT → batch_transcribe → UPDATE) вместо ~100

**Хранение SRT-файлов на диске**
- Колонка `transcription TEXT` заменена на `transcription_path TEXT` (путь к `.srt` файлу)
- SRT-файлы хранятся рядом с WAV: `recording.wav` → `recording.wav.srt`
- whisper-cli создаёт `.srt` напрямую — MCP-сервер больше не удаляет файл (убран `os.Remove`)
- БД остаётся лёгкой (пути вместо мегабайтов текста), SRT-файлы доступны для любого плеера/редактора

**Модель whisper large-v3-turbo**
- Скачана модель `ggml-large-v3-turbo.bin` (1.6 GB) — значительно лучшее качество русской транскрипции
- Добавлена в `download_model` (валидные модели: tiny, base, small, medium, large-v3, large-v3-turbo)
- Конфигурация: `WHISPER_MODEL: ./data/whisper/models/ggml-large-v3-turbo.bin`

**Исправление авто-определения языка в whisper**
- Баг: whisper-cli без флага `-l` дефолтил в английский, а не auto-detect — русские разговоры транскрибировались на английском
- Фикс: MCP-сервер теперь всегда передаёт `-l auto` (или указанный язык). Ранее при `language="auto"` флаг `-l` пропускался

**Логирование output каждого шага pipeline**
- `internal/pipeline/sequential.go`: добавлен `output_preview` (первые 500 символов) в лог `step completed`

**Дедупликация записей**
- Добавлен UNIQUE индекс на `recordings.filename` — защита от дубликатов при разных форматах `call_id`
- Промпт fetch: INSERT только после проверки файла на диске

**Прочее**
- `DOMAIN.md` домена `sip-calls` обновлён: новая схема с `transcription_path`, описание файлов (wav, srt, docx)
- Summarize: добавлен `filesystem` MCP для чтения SRT-файлов, бюджет увеличен, таймаут 30 мин
- Deliver: таймаут 15 мин, добавлен `filesystem read_file` в allowed_tools
- Refactoring mcp-openapi: `downloadOneFile()` — общая функция для `download_file` и `batch_download`
- Refactoring mcp-whisper: `transcribeOne()` — общая функция для `transcribe_audio` и `batch_transcribe`

### 2026-03-21 — Параллельная транскрипция в batch_transcribe, увеличение таймаута

**Параллельный worker pool в mcp-whisper**
- `batch_transcribe` переписан с последовательного цикла на параллельный worker pool (`sync.WaitGroup` + каналы)
- Количество воркеров настраивается через `WHISPER_WORKERS` (по умолчанию 4)
- Порядок результатов сохраняется (индексированная запись в слайс)
- Пре-фильтрация (skip уже готовых файлов) выполняется до параллельной фазы
- Причина: `batch_transcribe` 36 файлов моделью large-v3-turbo на CPU не укладывался в таймаут 180 мин и убивался по `signal: killed`

**Конфигурация whisper для 8-ядерной машины**
- `WHISPER_THREADS: "4"` (было "8"), `WHISPER_WORKERS: "2"` — 2 файла параллельно × 4 потока = 8 = все ядра CPU
- Ранее 1 файл × 8 потоков = 8, теперь пропускная способность удвоена

**Таймаут transcribe-sip-recordings**
- Увеличен с `180m` до `360m` как запас на случай большого количества файлов

### 2026-03-22 — Безопасное удаление с бэкапом и проверкой зависимостей

**Проблема:** удаление задач, пайплайнов и суб-агентов не проверяло зависимости (удаление задачи, используемой в пайплайне, ломало конфигурацию), не создавало бэкапов, и у задач (tasks) вообще отсутствовал DELETE-эндпоинт.

**Новый пакет `internal/depcheck/` — анализ зависимостей**
- Чистые функции (без I/O) для анализа `*config.Config`
- `AnalyzeTaskDelete(cfg, name)` — блокирует удаление если задача используется в пайплайне
- `AnalyzePipelineDelete(cfg, name)` — определяет каскадные элементы: задачи эксклюзивные для данного пайплайна + их эксклюзивные суб-агенты
- `AnalyzeSubAgentDelete(cfg, name)` — блокирует удаление если агент используется в задаче
- `DeleteAnalysis`: `Entity`, `UsedBy`, `CanDelete`, `CascadeItems`, `Blocked`, `BlockReason`
- Unit-тесты: 14 тестов (блокировка, каскад, эксклюзивность, множественные ссылки, EntityFields, BlockReason, NoCascadeWhenAllShared, CascadeAgentsFromMultipleExclusiveTasks)

**Новый пакет `internal/backup/` — бэкап и восстановление**
- SQLite-таблица `backup_log` (миграция в `backup.New()`)
- Файловые бэкапы в `data/backup/{uuid}/` (копии `.md` файлов суб-агентов)
- `config_snap` — полный снимок `tasks.yaml` на момент удаления (для безопасного restore)
- Каскадное удаление: parent backup + child entries связанные через `parent_id`
- Методы: `CreateBackup`, `ListBackups`, `GetBackup`, `RestoreFiles`, `MarkRestored`, `GetChildren`
- Unit-тесты: 11 тестов (CRUD, файловый бэкап/восстановление, фильтрация, сортировка, parent/child каскад, MarkRestored, not found)
- Менеджер доменов: добавлен `RemoveDomain()` — удаление из in-memory map (data dir остаётся на диске)
- Менеджер суб-агентов: добавлены `CreateFromBytes()` (восстановление из бэкапа) и `GetFilePath()`
- Unit-тесты суб-агентов: 4 новых теста (CreateFromBytes, CreateFromBytes дубликат, GetFilePath, GetFilePath not found)

**Новые API-эндпоинты**
- `DELETE /api/v1/tasks/{name}` — удаление задачи с проверкой зависимостей + бэкап
- `GET /api/v1/tasks/{name}/delete-info` — предварительный анализ (фронтенд вызывает перед показом модального окна)
- `GET /api/v1/pipelines/{name}/delete-info` — анализ каскадного удаления пайплайна
- `GET /api/v1/subagents/{name}/delete-info` — анализ зависимостей суб-агента
- `GET /api/v1/backups` — список бэкапов (фильтр по `entity_type`)
- `GET /api/v1/backups/{id}` — детали бэкапа
- `POST /api/v1/backups/{id}/restore` — восстановление из бэкапа (с проверкой конфликтов имён и валидацией конфигурации)

**Улучшенные DELETE-эндпоинты**
- `DELETE /api/v1/pipelines/{name}` — каскадное удаление эксклюзивных задач + суб-агентов, бэкап конфигурации + файлов агентов, очистка ссылок в доменах
- `DELETE /api/v1/subagents/{name}` — проверка зависимостей (блокировка если используется в задачах), бэкап `.md` файла
- Все DELETE-эндпоинты возвращают `{backup_id, deleted: [...]}` вместо `204 No Content`
- Сериализация через `runguard.Guard("config:write")` — защита от конкурентных модификаций

**Web UI**
- Улучшенный `ConfirmModal` — отображение `DeleteAnalysis`: секция "Used by" с цветными бейджами типов, секция "Will also be deleted" для каскадных элементов, блокировка кнопки Delete при наличии зависимостей с объяснением причины, индикатор загрузки
- `TaskList` — новая кнопка Delete: prefetch `delete-info` → ConfirmModal → delete + toast
- `PipelineList` — замена `window.confirm` на ConfirmModal с отображением каскадных элементов
- `SubAgentList` — замена `window.confirm` на ConfirmModal с отображением зависимостей
- API-клиент: `deleteTask`, `getTaskDeleteInfo`, `getPipelineDeleteInfo`, `getSubAgentDeleteInfo`, `listBackups`, `restoreBackup`
- TypeScript-типы: `DeleteAnalysis`, `Dependency`, `DeleteResponse`, `BackupEntry`

**Логика зависимостей:**
| Сценарий | Поведение |
|---|---|
| Задача в 2+ пайплайнах | Блокировка — удалите сначала пайплайны |
| Задача эксклюзивна для 1 пайплайна | Каскадное удаление вместе с пайплайном |
| Суб-агент в задачах | Блокировка — удалите из задач |
| Суб-агент эксклюзивен для каскадной задачи | Каскадное удаление |
| Домен | Автоматически удаляется когда все ссылки (tasks, pipelines, agents) очищены |
| Восстановление при конфликте имён | 409 — удалите/переименуйте существующую сущность |

**Затронутые файлы:**
- Новые: `internal/depcheck/checker.go` (+test), `internal/backup/manager.go`, `internal/api/delete.go`, `internal/api/backups.go`
- Изменённые: `internal/api/router.go`, `internal/api/tasks.go`, `internal/api/pipelines.go`, `internal/api/subagents.go`, `internal/domain/manager.go`, `internal/subagent/manager.go`, `internal/store/sqlite/sqlite.go`, `cmd/server/main.go`
- Фронтенд: `web/src/types/index.ts`, `web/src/api/client.ts`, `web/src/components/ConfirmModal.tsx`, `web/src/components/TaskList.tsx`, `web/src/components/PipelineList.tsx`, `web/src/components/SubAgentList.tsx`

### 2026-03-22 — Исправление очистки доменов при каскадном удалении

**Проблема:** при удалении пайплайна `cleanDomainRefs()` очищала только ссылки на задачи и пайплайны из доменов, но не на суб-агентов. Осиротевшие домены (без задач, пайплайнов и агентов) оставались в `tasks.yaml`. При отдельном удалении суб-агента (`DELETE /api/v1/subagents/{name}`) очистка доменных ссылок вообще не вызывалась.

**Исправления в `internal/api/delete.go`:**
- `cleanDomainRefs()` принимает 3-й параметр `agentNames []string` — очистка ссылок на агентов из `Domain.Agents`
- Автоматическое удаление осиротевших доменов: если после очистки у домена нет `Tasks`, `Pipelines` и `Agents`, запись домена удаляется из конфигурации
- Unit-тесты: 6 тестов (`delete_test.go`) — очистка tasks/pipelines/agents, удаление осиротевших доменов, multi-domain сценарии

**Исправления в `internal/api/pipelines.go`:**
- `handleDeletePipeline` собирает имена каскадно удалённых агентов и передаёт в `cleanDomainRefs()`

**Исправления в `internal/api/subagents.go`:**
- `handleDeleteSubAgent` теперь вызывает `cleanDomainRefs(nil, nil, []string{name})` + `s.cfg.Save()`

**Очистка данных:**
- Удалён осиротевший домен `radiation-control-equipment` из `tasks.yaml` (агент и пайплайн были удалены ранее, но домен остался)

### 2026-03-22 — Полная логическая целостность удаления и восстановления

**Проблема:** после первого фикса остались баги: 1) каскадные бэкапы задач не содержали `config_snap` (невозможно восстановить отдельно), 2) домены не удалялись если содержали `mcp_servers` ссылки, 3) восстановление из бэкапа не восстанавливало домен, 4) восстановление суб-агентов всегда падало с ошибкой "backup has no config snapshot".

**Исправления бэкапов (`internal/api/pipelines.go`):**
- Каскадные бэкапы задач теперь включают `configSnap` — каждая задача может быть восстановлена индивидуально, а не только через родительский бэкап пайплайна

**Исправления очистки доменов (`internal/api/delete.go`, `pipelines.go`, `tasks.go`):**
- `cleanDomainRefs()` принимает 4-й вариадик-параметр `mcpServerNames ...[]string` — очистка ссылок на MCP-серверы из `Domain.MCPServers`
- Проверка осиротевшего домена теперь включает `MCPServers`: домен удаляется только если все 4 списка пусты (`Tasks`, `Pipelines`, `Agents`, `MCPServers`)
- `handleDeletePipeline`: перед удалением задач собирает их MCP-серверы, фильтрует те что используются выжившими задачами, передаёт эксклюзивные в `cleanDomainRefs()`
- `handleDeleteTask`: аналогично собирает MCP-серверы удаляемой задачи и очищает эксклюзивные из доменов
- Unit-тесты: 3 новых теста для MCPServers (полная очистка, частичная очистка, сохранение домена при наличии MCP-серверов)

**Исправления восстановления (`internal/api/backups.go`):**
- `handleRestoreBackup`: проверка `configSnap` перенесена внутрь case-блоков `task`/`pipeline` — суб-агенты не требуют configSnap и теперь успешно восстанавливаются
- `restoreTask`: при восстановлении задачи восстанавливается домен, который ссылался на эту задачу (если был удалён как осиротевший)
- `restorePipeline`: при восстановлении пайплайна восстанавливаются домены, ссылающиеся на пайплайн или его каскадные сущности

**Исправления обработки ошибок (`internal/api/subagents.go`):**
- `handleDeleteSubAgent`: ошибка `cfg.Save()` теперь возвращает HTTP 500 клиенту вместо тихого логирования

### 2026-03-22 — Файловые бэкапы tasks.yaml на диск

**Проблема:** при удалении пайплайна конфигурационный снимок (`tasks.yaml`) сохранялся только в SQLite (поле `config_snap`), но не как физический файл на диске. В директории бэкапа `data/backup/{id}/` были видны только файлы суб-агентов (`.md`). Это создавало впечатление, что задачи и конфигурация не попадают в бэкап.

**Исправление (`internal/backup/manager.go`):**
- `CreateBackup()` теперь сохраняет `configSnap` как файл `tasks.yaml` в директорию бэкапа на диске
- Файл создаётся для всех бэкап-записей, имеющих configSnap (parent pipeline, cascade tasks, standalone tasks)
- Логика восстановления не изменена — по-прежнему использует `config_snap` из SQLite

**Результат — структура `data/backup/{id}/` после удаления пайплайна:**
```
data/backup/
  {parent-pipeline-id}/
    tasks.yaml              ← полный снимок конфигурации
  {cascade-task-id}/
    tasks.yaml              ← полный снимок конфигурации
  {cascade-agent-id}/
    agents/reviewer.md      ← файл суб-агента
```

### 2026-03-22 — Pipeline reliability: output validation + wizard guardrails

**Проблема 1:** пайплайн `rad-control-manufacturers-sync` помечен `completed`, хотя deliver-шаг не отправил файл в Telegram. Claude вернул "Запрашиваю permission для чтения файла xlsx" с exit code 0 — runner проверял только exit code, не содержимое вывода.

**Проблема 2:** wizard создал конфиг, скопировав баги из существующего пайплайна без валидации — `stop_signal: PIPELINE_DONE` в промпте первого шага, deliver-шаг читал бинарный xlsx через filesystem (невозможно), compile-шаг возвращал "только путь к файлу" без контекста для следующего шага.

**Детекция "мягких" сбоев — новый пакет `internal/outputcheck/`**
- Функция `CheckStepOutput()` сканирует вывод Claude на паттерны, указывающие что задача НЕ выполнена несмотря на exit code 0
- Паттерны (case-insensitive): permission requests (RU/EN), tool not available, not in allowed_tools, Claude asks for input instead of completing
- Подключена в трёх местах: `internal/api/pipelines.go`, `internal/pipeline/sequential.go`, `internal/pipeline/parallel.go`
- Теперь шаг с таким выводом помечается `failed`, а не `completed`
- Unit-тесты: 14 test cases (RU/EN/case-insensitive/false-positive проверки)

**Wizard validation — новые проверки (`internal/wizard/applier.go`)**
- Изменена сигнатура `validate()` → возвращает `(warnings []string, err error)`
- **HARD ERROR**: задача с `agents` без `"Agent"` в `allowed_tools` (если allowed_tools не пуст)
- **HARD ERROR**: задача с `mcp_servers`/`agents` с `permission_mode` != `"dontAsk"` (если указан)
- **WARNING**: задача с `mcp_servers` без matching tools (prefix `mcp__{server}__`) в `allowed_tools`
- **HARD ERROR** (ранее): stop_signal в промпте не-финального шага пайплайна

**Wizard prompt — новые правила (`internal/wizard/prompt.go`)**
- Rule 13: stop_signal safety — только последний шаг может выводить stop_signal; при `max_iterations: 1` лучше не использовать stop_signal
- Rule 14: pipeline data flow — каждый шаг ОБЯЗАН выводить все данные для следующего шага; НЕЛЬЗЯ инструктировать "верни только путь к файлу"; delivery-шаги берут данные из `{{.PrevOutput}}`, не из бинарных файлов

**Исправления промптов (`tasks.yaml`)**
- `research-rad-control-manufacturers`: убрано `PIPELINE_DONE` из промпта
- `compile-rad-control-manufacturers-excel`: "Верни только путь к файлу" → вывод SESSION_ID, NEW_COUNT, FILE, сводка
- `compile-vet-manufacturers-excel`: аналогичное дополнение вывода
- `deliver-rad-control-manufacturers-report`: явный запрет чтения xlsx, данные из PrevOutput
- `deliver-vet-manufacturers-report`: аналогичное исправление
- `rad-control-manufacturers-sync`: убран `stop_signal: PIPELINE_DONE` (не нужен при `max_iterations: 1`)
- `vet-manufacturers-sync`: аналогично убран `stop_signal`

### 2026-03-22 — Оптимизация пайплайна rad-control-manufacturers-sync

**Упрощение схемы БД домена `rad-control-manufacturers-belarus`:**
- Удалены избыточные колонки из таблицы `manufacturers`: `unp`, `product_categories`, `product_types`, `certifications`
- Оставлены поля, релевантные для отчёта: `license_no`, `license_issuer`, `exports_abroad`, `air_export`, `export_countries`, `air_export_notes`, `source`, `notes`
- Добавлена таблица `sync_sessions` (session_id, started_at, finished_at, new_count, status) — отслеживание запусков пайплайна

**Выделение суб-агента `rad-control-manufacturers-researcher`:**
- Инструкции исследователя вынесены из inline-промпта задачи `research-rad-control-manufacturers` в отдельный файл `.claude/agents/rad-control-manufacturers-researcher.md`
- Агент: поиск по реестрам Госстандарта, ТПП, аккредитации, export.by, профильным выставкам
- 4-шаговый процесс: инициализация сессии → поиск → квалификация и сохранение → финализация
- Встроенная дедупликация: SELECT known → check_exists → INSERT только новых
- Запись в `sync_sessions`: статус `running` → `done`, подсчёт `new_count`

**Оптимизация промптов:**
- `research-rad-control-manufacturers`: упрощён до делегирования агенту, убраны inline-инструкции
- `compile-rad-control-manufacturers-excel`: SQL-запрос обновлён под новую схему (убраны несуществующие колонки)
- `deliver-rad-control-manufacturers-report`: задача удалена — deliver-логика интегрирована в compile-шаг (пайплайн упрощён до 2 шагов: research → compile+deliver)

### 2026-03-23 — Навигация из Pipeline в Tasks

**Быстрый переход к задаче из редактора пайплайна:**
- В `PipelineEditor` рядом с каждым шагом (task name) добавлена иконка-ссылка `↗`, которая ведёт на страницу Tasks с автоматическим выбором соответствующей задачи
- Ссылка использует `react-router-dom` `<Link>` с query-параметром `?select=<taskName>`
- `TaskList` читает `useSearchParams` при загрузке — если есть `?select=`, автоматически открывает задачу в панели редактирования
- После выбора задачи query-параметр очищается (`replace: true`) для чистого URL

**Изменённые файлы:**
- `web/src/components/PipelineList.tsx` — импорт `Link`, иконка `↗` с навигацией
- `web/src/components/TaskList.tsx` — импорт `useSearchParams`/`useEffect`, автоселект задачи по query param

---

## Бэклог

### Фаза 10: Обновление hook-системы
- [ ] Оценить миграцию на хуки суб-агентов или MCP-сервер
- [ ] Расширить список опасных паттернов

### MCP-серверы — доработки
- [x] mcp-filesystem: полная реализация tools/call + copy_file
- [x] mcp-excel: интеграция с excelize + add_styled_table
- [x] mcp-word: чтение, запись и создание .docx через stdlib (archive/zip + encoding/xml)
- [x] mcp-pdf: чтение, извлечение текста и таблиц из PDF через ledongthuc/pdf
- [x] mcp-email: SMTP-отправка с вложениями через gomail
- [ ] mcp-email: IMAP-чтение (read_inbox, search_emails)
- [ ] mcp-google: Google Docs/Sheets API
- [x] mcp-database: SQLite-реализация (query, execute, check_exists, insert, list_tables, describe_table) — интеграция через domain system
- [x] mcp-telegram: отправка сообщений и файлов через Telegram Bot API
- [x] mcp-exportby: каталог export.by — scan, analyze, reject, mark_exported
- [x] mcp-openapi: динамические MCP-инструменты из OpenAPI-спецификаций (libopenapi)
- [x] mcp-whisper: транскрипция аудио через whisper.cpp (WAV/MP3/FLAC/OGG/M4A, multi-language)
- [x] Миграция всех 11 MCP-серверов на библиотеку mcp-go (устранение ~900 строк JSON-RPC boilerplate)

### Web UI — доработки
- [x] Динамическое обновление UI через SSE (real-time, без polling)
- [x] Toast-уведомления при завершении задач/пайплайнов
- [x] Детальный просмотр execution с SSE-стримингом (live output)
- [x] Редактирование задач через UI (CSV-поля исправлены)
- [x] Удаление execution записей (с подтверждением)
- [x] Конфигурация allow_concurrent через UI (чекбокс в TaskEditor и PipelineEditor)
- [x] Hot reload tasks.yaml (fsnotify + debounce → scheduler/watcher перерегистрация)
- [x] Управление MCP-серверами через UI
- [x] Тёмная тема
- [x] Безопасное удаление задач/пайплайнов/суб-агентов (зависимости, каскад, бэкап, ConfirmModal)
- [x] Восстановление удалённых элементов из бэкапа (REST API)
- [x] Навигация из PipelineEditor в TaskList (ссылка `↗` на шагах пайплайна)

### Инфраструктура
- [x] Unit-тесты: auth, events, runguard, subagent, task, store/sqlite, depcheck, backup (75+ тестов)
- [ ] CI/CD pipeline (GitHub Actions)
- [x] Docker-образ (Dockerfile + docker-compose.yml)
- [ ] Docker: авторизация Claude Code Max (OAuth) — `claude login` из контейнера не подключается к api.anthropic.com (ERR_BAD_REQUEST). Варианты: host network, DNS-fix, или ANTHROPIC_API_KEY
- [ ] Документация API (OpenAPI/Swagger)
