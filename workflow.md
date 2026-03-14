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
- [x] mcp-database: SQLite-реализация (query, execute, check_exists, insert, list_tables, describe_table) — интеграция через domain system
- [x] mcp-telegram: отправка сообщений и файлов через Telegram Bot API
- [x] mcp-exportby: каталог export.by — scan, analyze, reject, mark_exported

### Web UI — доработки
- [x] Динамическое обновление UI через SSE (real-time, без polling)
- [x] Toast-уведомления при завершении задач/пайплайнов
- [ ] Детальный просмотр execution с SSE-стримингом (live output)
- [x] Редактирование задач через UI (CSV-поля исправлены)
- [x] Удаление execution записей (с подтверждением)
- [x] Конфигурация allow_concurrent через UI (чекбокс в TaskEditor и PipelineEditor)
- [x] Hot reload tasks.yaml (fsnotify + debounce → scheduler/watcher перерегистрация)
- [ ] Управление MCP-серверами через UI
- [x] Тёмная тема

### Инфраструктура
- [ ] Unit-тесты для всех internal/ пакетов
- [ ] CI/CD pipeline (GitHub Actions)
- [x] Docker-образ (Dockerfile + docker-compose.yml)
- [ ] Docker: авторизация Claude Code Max (OAuth) — `claude login` из контейнера не подключается к api.anthropic.com (ERR_BAD_REQUEST). Варианты: host network, DNS-fix, или ANTHROPIC_API_KEY
- [ ] Документация API (OpenAPI/Swagger)
