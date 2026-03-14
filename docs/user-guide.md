# Claude Ecosystem — Руководство пользователя

## Содержание

1. [Установка](#установка)
2. [Конфигурация](#конфигурация)
3. [Запуск](#запуск)
4. [Задачи (Tasks)](#задачи-tasks)
5. [Суб-агенты](#суб-агенты)
6. [Пайплайны](#пайплайны)
7. [MCP-серверы](#mcp-серверы)
8. [REST API](#rest-api)
9. [Web UI](#web-ui)
10. [Аутентификация](#аутентификация)
11. [Hook-система](#hook-система)
12. [Docker](#docker)
13. [Systemd daemon](#systemd-daemon)

---

## Установка

### Требования

- Go 1.26+
- Claude Code CLI (`claude`) в PATH
- Node.js 18+ (для сборки Web UI)

### Сборка

```bash
git clone <repo-url>
cd claude-ecosystem

# Сборка серверных бинарников
make build

# Сборка Web UI (опционально)
make build-ui

# Пересборка всего + перезапуск сервера (UI + Go + restart)
make rebuild

# Установка hook-бинарника
make install
```

После `make build` в директории `bin/` появятся:
- `server` — основной сервер
- `hook` — hook для Claude Code
- `mcp-*` — MCP-серверы

---

## Конфигурация

### Файл .env

Секреты и environment-specific значения хранятся в файле `.env` (не коммитится в git):

```bash
cp .env.example .env
# Отредактируйте .env, заполнив свои значения
```

Приоритет значений:
1. Реальные переменные окружения (наивысший)
2. Значения из `.env`
3. Дефолты в `tasks.yaml`

В `tasks.yaml` можно ссылаться на переменные через `${VAR}`:

```yaml
mcp_servers:
  - name: email
    command: ./bin/mcp-email
    env:
      SMTP_HOST: ${SMTP_HOST}
      SMTP_PASSWORD: ${SMTP_PASSWORD}
```

Подстановка работает в полях: `auth`, `mcp_servers.env`, `server`, `tasks.prompt`, `tasks.work_dir`.

### Файл tasks.yaml

Основной файл конфигурации — `tasks.yaml`. Пример:

```yaml
claude_bin: claude

server:
  addr: ":3580"
  data_dir: "data"

auth:
  paseto_key: ""               # hex-encoded 32-byte key (оставьте пустым для авто-генерации)
  bearer_tokens:               # предустановленные API-токены
    - "my-secret-token"
  users:
    - username: admin
      password: "$2a$10$..."   # bcrypt-хеш

# mcp_servers:
#   - name: filesystem
#     command: ./bin/mcp-filesystem
#   - name: excel
#     command: ./bin/mcp-excel

tasks:
  - name: code-review
    prompt: |
      Review the Go code in the working directory...
    work_dir: .
    schedule: "0 9 * * 1-5"
    tags: [review, quality]
    model: sonnet
    timeout: "10m"
    # max_turns: 20
    # max_budget_usd: 1.00
    # agents: [reviewer]        # суб-агенты для --agents
    # mcp_servers: [filesystem]  # MCP-серверы для --mcp-config
    # allowed_tools: [Read, Grep, Glob]
    # json_schema: '{"type":"object",...}'
    # notify:
    #   email: [admin@company.com]
    #   webhook: https://hooks.example.com/task-done
    #   trigger: on_failure  # on_success | on_failure | always

pipelines:
  - name: review-fix
    mode: sequential
    steps:
      - task: code-review
      - task: code-fixer
    max_iterations: 10
    stop_signal: "LGTM"
```

### Параметры задачи

| Параметр | Описание |
|----------|----------|
| `name` | Уникальное имя задачи (обязательно) |
| `prompt` | Go-шаблон промпта (обязательно). Переменные: `{{.PrevOutput}}`, `{{.File}}`, `{{.Iteration}}`, `{{.Date}}`, `{{.DateTime}}` |
| `work_dir` | Рабочая директория для claude |
| `schedule` | Cron-выражение для автозапуска |
| `watch` | Настройки fsnotify: `paths`, `extensions`, `debounce` |
| `tags` | Метки для организации |
| `model` | Модель Claude (sonnet, opus, haiku) |
| `timeout` | Таймаут выполнения (по умолчанию "5m") |
| `agents` | Список суб-агентов для `--agents` |
| `mcp_servers` | Список MCP-серверов для `--mcp-config` |
| `allowed_tools` | Разрешённые инструменты для `--allowedTools` |
| `disallowed_tools` | Запрещённые инструменты для `--disallowedTools` |
| `json_schema` | JSON-схема для структурированного вывода |
| `max_turns` | Максимум итераций агента |
| `max_budget_usd` | Лимит бюджета в USD |
| `output_format` | Формат вывода: `json` (по умолчанию) или `stream-json` |
| `allow_concurrent` | Разрешить параллельный запуск (по умолчанию `true`). При `false` повторный запуск блокируется, если предыдущий ещё выполняется |
| `notify` | Настройки уведомлений (см. ниже) |

#### Уведомления (notify)

Задача может автоматически отправлять email и/или webhook при завершении:

```yaml
tasks:
  - name: nightly-report
    prompt: "Generate daily report..."
    work_dir: .
    schedule: "0 22 * * *"
    notify:
      email:
        - ceo@company.com
        - team@company.com
      webhook: https://hooks.slack.com/services/XXX
      trigger: on_failure  # on_success | on_failure | always (default)
```

| Поле | Описание |
|------|----------|
| `email` | Список адресов для email-уведомлений. Требует `SMTP_*` переменных в `.env` |
| `webhook` | URL для POST-запроса с JSON-телом результата |
| `trigger` | Когда отправлять: `on_success`, `on_failure`, `always` (по умолчанию) |

**Email** содержит HTML-версию с результатом выполнения и plain-text fallback.
**Webhook** отправляет JSON:
```json
{
  "event": "task.completed",
  "task": "nightly-report",
  "status": "completed",
  "execution_id": "uuid",
  "output": "...",
  "timestamp": "2026-03-08T22:00:00Z"
}
```

---

## Запуск

### Режим сервера (по умолчанию)

```bash
make run
# или
./bin/server -config tasks.yaml
```

Запускает HTTP-сервер на `:3580`, планировщик cron и watcher файлов.

### Разовый запуск задачи

```bash
make run-task TASK=code-review
# или
./bin/server -run code-review
```

### Запуск пайплайна

```bash
make run-pipeline PIPELINE=review-fix
# или
./bin/server -pipeline review-fix
```

---

## Задачи (Tasks)

Задача — это единичный вызов `claude -p` с настроенным промптом и параметрами. Задачи могут запускаться:

- **По расписанию** — через cron-выражение в поле `schedule`
- **По изменению файлов** — через настройку `watch`
- **Вручную** — через CLI (`-run`) или REST API (`POST /api/v1/tasks/:name/run`)
- **В составе пайплайна** — как шаг последовательности

### Шаблоны промптов

Промпты поддерживают Go `text/template`:

```yaml
prompt: |
  Review the file: {{.File}}
  {{if .PrevOutput}}
  Previous context: {{.PrevOutput}}
  {{end}}
```

Доступные переменные зависят от контекста запуска:
- `{{.File}}` — путь к изменённому файлу (watcher)
- `{{.PrevOutput}}` — вывод предыдущего шага (pipeline)
- `{{.Iteration}}` — номер итерации (pipeline)
- `{{.Date}}` — текущая дата `YYYY-MM-DD` (pipeline, scheduler)
- `{{.DateTime}}` — текущие дата и время `YYYY-MM-DD_HH-MM` (pipeline, scheduler) — удобно для уникальных имён файлов

---

## Суб-агенты

Суб-агенты — это файлы `.claude/agents/*.md` с YAML frontmatter:

```markdown
---
description: "Code reviewer focused on security"
tools:
  - Read
  - Grep
  - Glob
model: sonnet
maxTurns: 10
---

You are a security-focused code reviewer. Focus on:
- Input validation
- SQL injection
- XSS vulnerabilities
```

### Управление через API

```bash
# Список суб-агентов
curl -H "Authorization: Bearer <token>" http://localhost:3580/api/v1/subagents

# Создание
curl -X POST -H "Authorization: Bearer <token>" \
  -d '{"name":"reviewer","description":"Code reviewer","instructions":"..."}' \
  http://localhost:3580/api/v1/subagents

# Удаление
curl -X DELETE -H "Authorization: Bearer <token>" \
  http://localhost:3580/api/v1/subagents/reviewer
```

### Использование в задачах

```yaml
tasks:
  - name: security-review
    prompt: "Review the codebase for security issues."
    agents: [reviewer]   # ссылка на .claude/agents/reviewer.md
```

---

## Пайплайны

Пайплайны объединяют задачи в цепочки.

### Sequential (последовательный)

Задачи выполняются по кругу, вывод каждой передаётся следующей через `{{.PrevOutput}}`. Цикл останавливается при обнаружении `stop_signal` или достижении `max_iterations`.

```yaml
pipelines:
  - name: review-fix
    mode: sequential
    steps:
      - task: code-review
      - task: code-fixer
    max_iterations: 10
    stop_signal: "LGTM"
    # schedule: "0 9 * * 1-5"  # опционально: cron-расписание
```

### Sequential single-pass (линейная цепочка)

Для пайплайнов без цикла используйте `max_iterations: 1` — `stop_signal` не требуется:

```yaml
pipelines:
  - name: leads-to-ceo
    mode: sequential
    steps:
      - task: find-leads            # поиск → JSON
      - task: compile-leads-excel   # JSON → Excel
      - task: deliver-leads-report  # Excel → email + Telegram
    max_iterations: 1
```

Каждый шаг получает вывод предыдущего через `{{.PrevOutput}}`.

### Расписание (schedule)

Пайплайны, как и задачи, поддерживают cron-расписание для автоматического запуска:

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

Расписание можно задать через UI (поле "Schedule (cron)" в форме редактирования пайплайна) или в `tasks.yaml`. Формат — стандартный 5-полевой cron. Пайплайн по расписанию можно приостановить/возобновить через API pause/resume (аналогично задачам).

### Защита от параллельного запуска

Задачи и пайплайны поддерживают поле `allow_concurrent` для защиты от параллельного запуска одного и того же таска/пайплайна:

```yaml
tasks:
  - name: long-running-analysis
    prompt: "..."
    allow_concurrent: false   # блокировать повторный запуск
    schedule: "*/5 * * * *"

pipelines:
  - name: export-by-aviation-to-ceo
    mode: sequential
    allow_concurrent: false   # только один экземпляр одновременно
    schedule: "0 9 * * 1-5"
    steps:
      - task: sync-export-by-catalog
      - task: process-export-by-leads
    max_iterations: 1
```

| Значение | Поведение |
|----------|-----------|
| `true` (по умолчанию) | Параллельные запуски разрешены (обратная совместимость) |
| `false` | Если предыдущий запуск ещё выполняется: cron/watcher — пропускает с логом; API — возвращает `409 Conflict` |

Защита действует на все триггеры: cron-расписание, file watcher, REST API (sync и async). Guard разделяется между всеми компонентами — запуск по cron блокирует ручной запуск и наоборот.

### Parallel (параллельный)

Все шаги запускаются одновременно. Опциональный `collector` собирает результаты.

```yaml
pipelines:
  - name: multi-review
    mode: parallel
    steps:
      - task: security-review
      - task: performance-review
      - task: style-review
    collector: review-summarizer
    max_iterations: 1
```

---

## MCP-серверы

MCP-серверы предоставляют дополнительные инструменты для Claude через протокол JSON-RPC 2.0 по stdio.

### Конфигурация

MCP-серверы наследуют все переменные окружения родительского процесса (включая `.env`). Достаточно указать `command` — если нужные env vars уже в `.env`, дополнительно прописывать `env` не нужно:

```yaml
mcp_servers:
  - name: filesystem
    command: ./bin/mcp-filesystem
  - name: excel
    command: ./bin/mcp-excel
  - name: email
    command: ./bin/mcp-email
  - name: telegram
    command: ./bin/mcp-telegram
```

Поле `env` используется только для переопределения или добавления переменных сверх `.env`:

```yaml
mcp_servers:
  - name: email
    command: ./bin/mcp-email
    env:
      SMTP_HOST: custom-smtp.example.com  # переопределяет значение из .env
```

### Доступные серверы и инструменты

| Сервер | Инструменты | Статус |
|--------|------------|--------|
| **mcp-filesystem** | `read_file`, `write_file`, `list_directory`, `search_files`, `copy_file` | Реализован |
| **mcp-excel** | `create_spreadsheet`, `write_spreadsheet`, `read_spreadsheet`, `add_styled_table` | Реализован |
| **mcp-email** | `send_email` (с вложениями и HTML), `read_inbox`*, `search_emails`* | Частично (*stubs) |
| **mcp-telegram** | `send_message`, `send_document` | Реализован |
| **mcp-word** | — | Stub |
| **mcp-pdf** | — | Stub |
| **mcp-google** | — | Stub |
| **mcp-database** | `query`, `execute`, `list_tables`, `describe_table`, `check_exists`, `insert` | Реализован |
| **mcp-exportby** | `sync_catalog`, `get_unanalyzed`, `check_new`, `get_stats`, `get_pending_count`, `export_leads_excel`, `mark_exported`, `reject_companies` | Реализован |

### Привязка к задачам

Поле `mcp_servers` в конфигурации задачи автоматически генерирует `--mcp-config` JSON-файл и передаёт его Claude CLI.

**Важно:** при использовании `permission_mode: dontAsk` MCP-инструменты блокируются, если не указаны явно в `allowed_tools`. Имена инструментов следуют формату `mcp__<server>__<tool>`:

```yaml
tasks:
  - name: create-report
    prompt: "Create an Excel report..."
    mcp_servers: [excel, filesystem]
    allowed_tools:
      - mcp__excel__create_spreadsheet
      - mcp__excel__add_styled_table
      - mcp__filesystem__copy_file
    permission_mode: dontAsk
```

### Привязка суб-агентов

Поле `agents` в конфигурации задачи автоматически резолвит суб-агентов из `.claude/agents/*.md` и передаёт их через `--agents` JSON:

```yaml
tasks:
  - name: compile-report
    prompt: "Compile the report..."
    agents: [leads-report-compiler]
    mcp_servers: [excel]
```

### Управление

```bash
# Статус серверов
curl -H "Authorization: Bearer <token>" http://localhost:3580/api/v1/mcp-servers

# Запуск
curl -X POST -H "Authorization: Bearer <token>" \
  http://localhost:3580/api/v1/mcp-servers/filesystem/start

# Остановка
curl -X POST -H "Authorization: Bearer <token>" \
  http://localhost:3580/api/v1/mcp-servers/filesystem/stop
```

---

## REST API

Базовый путь: `/api/v1/`

### Аутентификация

| Метод | Путь | Описание |
|-------|------|----------|
| POST | `/auth/login` | Логин → PASETO-токен |
| POST | `/auth/refresh` | Обновление токена |

### Задачи

| Метод | Путь | Описание |
|-------|------|----------|
| GET | `/tasks` | Список задач |
| GET | `/tasks/:name` | Получить задачу |
| POST | `/tasks/:name/run` | Синхронный запуск |
| POST | `/tasks/:name/run-async` | Асинхронный запуск → `execution_id` |
| GET | `/tasks/:name/stream` | SSE-стрим вывода |

### Суб-агенты

| Метод | Путь | Описание |
|-------|------|----------|
| GET | `/subagents` | Список |
| GET | `/subagents/:name` | Получить |
| POST | `/subagents` | Создать |
| PUT | `/subagents/:name` | Обновить |
| DELETE | `/subagents/:name` | Удалить |

### Пайплайны

| Метод | Путь | Описание |
|-------|------|----------|
| GET | `/pipelines` | Список |
| GET | `/pipelines/:name` | Получить |
| POST | `/pipelines/:name/run` | Синхронный запуск |
| POST | `/pipelines/:name/run-async` | Асинхронный запуск |

### Исполнения

| Метод | Путь | Описание |
|-------|------|----------|
| GET | `/executions` | Список (фильтры: task, status, trigger, limit, offset) |
| GET | `/executions/:id` | Детали исполнения |
| DELETE | `/executions/:id` | Удалить запись |
| GET | `/executions/:id/stream` | SSE-стрим |

### MCP-серверы

| Метод | Путь | Описание |
|-------|------|----------|
| GET | `/mcp-servers` | Список + статус |
| POST | `/mcp-servers/:name/start` | Запустить |
| POST | `/mcp-servers/:name/stop` | Остановить |

### Dashboard

| Метод | Путь | Описание |
|-------|------|----------|
| GET | `/dashboard` | Агрегированная статистика |

---

## Web UI

Веб-интерфейс доступен по адресу `http://localhost:3580/` после запуска сервера.

### Сборка

```bash
make build-ui
```

Требует Node.js 18+. Собранные файлы встраиваются в Go-бинарник через `go:embed`.

### Для разработки

```bash
cd web
npm install
npm run dev   # Vite dev server на :5173, проксирует /api на :3580
```

### Страницы

- **Dashboard** — общая статистика: количество задач, исполнений, статусы
- **Tasks** — список задач с возможностью запуска
- **Sub-Agents** — CRUD суб-агентов
- **Pipelines** — список пайплайнов с запуском
- **Executions** — история исполнений с авто-обновлением

---

## Аутентификация

Система поддерживает два метода:

### PASETO v4.local

Основной метод. Токены создаются при логине, действуют 24 часа.

```bash
# Получение токена
curl -X POST http://localhost:3580/api/v1/auth/login \
  -d '{"username":"admin","password":"secret"}'

# Использование
curl -H "Authorization: Bearer v4.local.xxx..." \
  http://localhost:3580/api/v1/tasks
```

### Bearer-токены

Предустановленные в конфигурации для автоматизации:

```yaml
auth:
  bearer_tokens:
    - "my-ci-token"
```

```bash
curl -H "Authorization: Bearer my-ci-token" \
  http://localhost:3580/api/v1/tasks
```

---

## Hook-система

Hook-бинарник интегрируется с Claude Code для контроля инструментов.

### Установка

```bash
make install
```

### Настройка Claude Code

Добавьте в `~/.claude/settings.json`:

```json
{
  "hooks": {
    "PreToolUse": [
      {"matcher": "Bash", "command": "claude-hook"}
    ]
  }
}
```

### Что блокируется

- `rm -rf /`, `rm -rf /*`
- `DROP TABLE`, `DROP DATABASE`
- Fork-бомбы (`:(){ :|:`)
- Запись в `/dev/sda`
- `mkfs.`, `dd if=`
- `chmod -R 777 /`

Пример конфигурации: `claude-hooks.example.json`.

---

## Docker

### Быстрый старт

```bash
cp .env.example .env
# Заполните ANTHROPIC_API_KEY и другие переменные в .env

make docker-build
make docker-up
```

Сервер будет доступен на `http://localhost:3580`.

### Остановка

```bash
make docker-down
```

### Авторизация Claude Code в контейнере

Claude Code CLI внутри контейнера требует авторизации. Два варианта:

**Вариант 1: API ключ (pay-per-use)**

Задайте в `.env`:
```bash
ANTHROPIC_API_KEY=sk-ant-...
```

**Вариант 2: Claude Code Max (OAuth) — в разработке**

OAuth-токены привязаны к устройству. Команда `claude login` внутри контейнера пока не работает из-за ограничений Docker-сети. См. бэклог.

**Для локальной разработки без Docker** — используйте `make run`, авторизация через `claude login` на хосте работает без ограничений.

### Что входит в образ

Единый Docker-образ содержит:
- Go-сервер (HTTP API + scheduler + watcher + Web UI)
- Все MCP-серверы (`mcp-excel`, `mcp-email`, `mcp-telegram`, `mcp-filesystem` и др.)
- Claude Code CLI

MCP-серверы запускаются автоматически как дочерние процессы внутри контейнера — отдельные контейнеры для них не нужны.

### Volumes

| Mount | Описание |
|-------|----------|
| `./tasks.yaml` | Конфигурация задач (read-write, редактируется через Web UI) |
| `./.claude/agents/` | Суб-агенты (read-write, редактируются через Web UI) |
| `server-data` | SQLite БД и данные (named volume) |
| `${WORKSPACE_DIR:-./workspace}` | Общая директория для файлового I/O задач |

### Workspace — рабочая директория задач

Задачи, создающие или читающие файлы (отчёты, экспорты и т.д.), используют каталог `workspace/` внутри контейнера (`/app/workspace`). На хосте по умолчанию маппится в `./workspace`, можно переопределить через `.env`:

```bash
WORKSPACE_DIR=/data/claude-workspace
```

Структура каталогов создаётся автоматически. Пример:

```
workspace/
├── leads/              # отчёты по лидам (Excel, JSON)
├── ceo/reports/        # копии отчётов для CEO
└── ...                 # другие каталоги по мере необходимости
```

В промптах задач используйте относительные пути от `work_dir`:

```yaml
tasks:
  - name: compile-leads-excel
    prompt: |
      Создай Excel-отчёт: workspace/leads/leads-report-{{.Date}}.xlsx
    work_dir: .
```

### Переменные окружения

Все переменные из `.env` автоматически передаются в контейнер. Основные:

```bash
ANTHROPIC_API_KEY=sk-...       # обязательно
WORKSPACE_DIR=./workspace      # директория для I/O задач (по умолчанию ./workspace)
SERVER_PORT=3580               # порт сервера (по умолчанию 3580)
```

Порт можно переопределить при запуске:

```bash
SERVER_PORT=9090 make docker-up
```

---

## Systemd daemon

Альтернатива Docker — запуск сервера как systemd user daemon на хосте. Все MCP-серверы запускаются как дочерние процессы сервера.

### Установка и запуск

```bash
# Сборка бинарников + установка systemd service + включение автозапуска
make daemon-install

# Запуск демона
make daemon-start
```

Сервер будет доступен на `http://localhost:3580`.

### Управление

| Команда | Описание |
|---------|----------|
| `make daemon-install` | Собрать, установить service, включить автозапуск |
| `make daemon-start` | Запустить демон |
| `make daemon-stop` | Остановить демон |
| `make rebuild` | Пересобрать UI + бинарники и перезапустить (останавливает Docker и/или процесс) |
| `make daemon-restart` | Перезапустить (после обновления кода — `make build && make daemon-restart`) |
| `make daemon-status` | Показать статус |
| `make daemon-logs` | Показать логи в реальном времени |
| `make daemon-uninstall` | Остановить, отключить автозапуск, удалить service |

### Детали

- Используется **user-level systemd** (`systemctl --user`) — `sudo` не требуется
- Демон работает от текущего пользователя и использует его `claude` credentials (`~/.claude`)
- Конфигурация загружается из `tasks.yaml` в корне проекта
- Переменные окружения из `.env` подхватываются автоматически
- Логи пишутся в journald, просмотр: `make daemon-logs` или `journalctl --user-unit claude-ecosystem`
- Graceful shutdown при остановке (SIGTERM → 20 секунд таймаут)
- Автоматический перезапуск при падении (`Restart=on-failure`, пауза 5 секунд)

---

## Hot Reload конфигурации

Сервер автоматически отслеживает изменения в `tasks.yaml` и перезагружает конфигурацию без перезапуска. Это работает при:

- Ручном редактировании `tasks.yaml` в текстовом редакторе
- Изменениях через Web UI (создание/обновление задач и пайплайнов)
- Изменениях через REST API (`POST /tasks`, `PUT /tasks/:name`, и т.д.)

### Что обновляется при hot reload

- **Задачи (tasks):** добавление, удаление, изменение промптов, расписания, таймаутов, параметров
- **Пайплайны (pipelines):** добавление, удаление, изменение шагов, расписания
- **Cron-расписание:** задачи и пайплайны с `schedule` автоматически перерегистрируются в планировщике
- **File watchers:** задачи с `watch` автоматически перерегистрируются

### Что НЕ обновляется (требует перезапуск)

- Секция `server` (addr, data_dir, log_level)
- Секция `auth` (paseto_key, users)
- Секция `mcp_servers` (добавление/удаление серверов)
- Секция `domains` (схемы БД, data_dir)

### Технические детали

- Используется fsnotify для отслеживания директории конфиг-файла
- Debounce 1 секунда (для обработки множественных записей при сохранении)
- При ошибке валидации нового конфига — изменения не применяются, ошибка логируется
- SSE-событие `config.reloaded` публикуется при успешной перезагрузке
- Pause-состояния задач в планировщике сохраняются при reload
