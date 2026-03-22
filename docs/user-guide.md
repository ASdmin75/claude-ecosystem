# Claude Ecosystem — Руководство пользователя

## Содержание

1. [Установка](#установка)
2. [Конфигурация](#конфигурация)
3. [Запуск](#запуск)
4. [Задачи (Tasks)](#задачи-tasks)
5. [Суб-агенты](#суб-агенты)
6. [Пайплайны](#пайплайны)
7. [Домены](#домены)
8. [MCP-серверы](#mcp-серверы)
9. [REST API](#rest-api)
10. [Web UI](#web-ui)
11. [Аутентификация](#аутентификация)
12. [Hook-система](#hook-система)
13. [Docker](#docker)
14. [Systemd daemon](#systemd-daemon)

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

# Установка Whisper.cpp для транскрипции аудио (опционально)
make setup-whisper
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
| `domain` | Привязка к домену данных (см. [Домены](#домены)) |
| `permission_mode` | Режим разрешений Claude CLI: `default`, `dontAsk` |
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

# Удаление (с проверкой зависимостей и бэкапом)
curl -X DELETE -H "Authorization: Bearer <token>" \
  http://localhost:3580/api/v1/subagents/reviewer

# Предварительный анализ зависимостей
curl -H "Authorization: Bearer <token>" \
  http://localhost:3580/api/v1/subagents/reviewer/delete-info
```

### Безопасное удаление

Удаление задач, пайплайнов и суб-агентов выполняется с проверкой зависимостей и автоматическим бэкапом:

- **Задача:** нельзя удалить если используется в пайплайне. Сначала удалите пайплайн.
- **Суб-агент:** нельзя удалить если используется в задаче. Сначала уберите ссылку из задачи.
- **Пайплайн:** при удалении каскадно удаляются задачи и суб-агенты, которые принадлежат **только** этому пайплайну. Задачи, используемые в других пайплайнах, не удаляются.
- **Домен:** автоматически удаляется из `tasks.yaml` когда все его ссылки (tasks, pipelines, agents, mcp_servers) очищены в результате удаления. MCP-серверы очищаются из домена только если они не используются выжившими задачами. Директория `data_dir` домена сохраняется на диске для ручной очистки.

Перед удалением всегда создаётся бэкап в `data/backup/{id}/`. Каждая каскадно удалённая сущность получает свой бэкап-вход. Содержимое бэкапа на диске:
- `tasks.yaml` — полный снимок конфигурации на момент удаления (для задач и пайплайнов)
- `agents/*.md` — файлы суб-агентов (для каскадно удалённых агентов)

Восстановление через API:

```bash
# Список бэкапов
curl -H "Authorization: Bearer <token>" http://localhost:3580/api/v1/backups

# Восстановление (пайплайн восстановит каскадные задачи, суб-агенты и домен)
curl -X POST -H "Authorization: Bearer <token>" \
  http://localhost:3580/api/v1/backups/<backup-id>/restore
```

При восстановлении из бэкапа автоматически восстанавливаются связанные домены, если они были удалены как осиротевшие.

В Web UI удаление доступно через кнопку Delete — модальное окно покажет зависимости и каскадные элементы перед подтверждением.

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

### Правила передачи данных между шагами

Каждый шаг pipeline **обязан** вывести всю информацию, необходимую следующему шагу. `{{.PrevOutput}}` — это единственный канал данных между шагами.

**Важно:**
- **Не инструктируйте шаг "вернуть только путь к файлу"** — следующий шаг потеряет контекст (SESSION_ID, счётчики, списки)
- **Delivery-шаги** должны брать данные (ID, количества, списки) из `{{.PrevOutput}}`, а не читать бинарные файлы (xlsx, pdf) через filesystem — `mcp__filesystem__read_file` работает только с текстовыми файлами
- **Бинарные файлы** (xlsx, pdf) передавайте по пути для отправки через `mcp__telegram__send_document` или `mcp__email__send_email`

### Детекция сбоев (output validation)

Pipeline автоматически проверяет вывод каждого шага на паттерны, указывающие что Claude не смог выполнить задачу (несмотря на exit code 0):
- Запрос permissions ("Запрашиваю permission", "need permission")
- Недоступность инструментов ("tool is not available", "not in allowed_tools")
- Запрос данных вместо выполнения ("предоставь эти данные", "please provide")

При обнаружении таких паттернов шаг помечается как `failed` и pipeline останавливается.

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

### Session chaining (сохранение контекста между шагами)

По умолчанию каждый шаг pipeline запускает новый `claude -p` процесс без контекста предыдущего. С `session_chain: true` каждый следующий шаг продолжает разговор предыдущего через `--resume`:

```yaml
pipelines:
  - name: research-and-report
    mode: sequential
    session_chain: true
    steps:
      - task: research-data       # запускает claude -p, получает session_id
      - task: compile-report      # --resume <session_id> — помнит весь контекст research
      - task: deliver-report      # --resume <session_id> — помнит и research, и report
    max_iterations: 1
```

**Преимущества:**
- Агент помнит весь контекст без передачи `{{.PrevOutput}}`
- Экономия токенов — Claude не парсит заново предыдущий вывод
- Качественнее — полный контекст вместо сжатого текста

**Ограничения:**
- Только `mode: sequential`
- Все шаги должны иметь **одинаковый `work_dir`** (сессия привязана к проекту)
- `--resume` может не учитывать разные `model`, `agents`, `mcp_servers` между шагами

Без `session_chain: true` (по умолчанию) pipeline работает как раньше — каждый шаг получает вывод предыдущего через `{{.PrevOutput}}`.

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

## Домены

Домены — это механизм привязки бизнес-данных (SQLite БД, файлы, документация) к задачам и пайплайнам. Данные домена отделены от системной БД сервера.

### Конфигурация

```yaml
domains:
  vet-manufacturers-belarus:
    description: Database of veterinary drug manufacturers in Belarus
    data_dir: data/vet-manufacturers-belarus
    db: vet-manufacturers.db
    schema: |-
      CREATE TABLE IF NOT EXISTS manufacturers (
        id INTEGER PRIMARY KEY AUTOINCREMENT,
        name TEXT NOT NULL,
        exports_abroad INTEGER DEFAULT 0,
        air_export INTEGER DEFAULT 0,
        session_id TEXT,
        first_seen TEXT DEFAULT (date('now')),
        updated_at TEXT DEFAULT (datetime('now')),
        UNIQUE(name)
      );
      CREATE TABLE IF NOT EXISTS sync_log (
        id INTEGER PRIMARY KEY AUTOINCREMENT,
        run_at TEXT DEFAULT (datetime('now')),
        session_id TEXT,
        manufacturers_added INTEGER DEFAULT 0,
        manufacturers_updated INTEGER DEFAULT 0,
        notes TEXT
      );
    domain_doc: DOMAIN.md
    tasks:
      - research-vet-manufacturers
    pipelines:
      - vet-manufacturers-sync
    agents:
      - vet-manufacturers-researcher
    mcp_servers:
      - database
```

### Параметры домена

| Параметр | Описание |
|----------|----------|
| `description` | Описание домена |
| `data_dir` | Директория для данных (создаётся автоматически) |
| `db` | Имя файла SQLite БД |
| `schema` | SQL-схема (применяется при инициализации) |
| `domain_doc` | Имя файла DOMAIN.md (инжектируется в system prompt агента) |
| `tasks` | Связанные задачи |
| `pipelines` | Связанные пайплайны |
| `agents` | Связанные суб-агенты |
| `mcp_servers` | Связанные MCP-серверы |

### Как это работает

1. При старте сервера `domain.Manager.Init()` создаёт `data_dir`, применяет SQL-схему, генерирует шаблон `DOMAIN.md`
2. При запуске задачи с `domain: <name>` автоматически:
   - Env vars (`DOMAIN_DB_PATH`, `DOMAIN_DATA_DIR`, `DOMAIN_NAME`) инжектируются в MCP-серверы
   - Содержимое `DOMAIN.md` добавляется в `--append-system-prompt`
3. MCP-сервер `mcp-database` читает `DOMAIN_DB_PATH` из env и работает с БД домена

### Привязка задач к домену

```yaml
tasks:
  - name: research-vet-manufacturers
    domain: vet-manufacturers-belarus
    prompt: "Найди производителей..."
    mcp_servers: [database]
    allowed_tools:
      - mcp__database__query
      - mcp__database__insert
      - mcp__database__check_exists
```

### Session_id — фильтрация по запуску

Для пайплайнов, которые запускаются многократно (вручную или по cron), важно отличать данные текущего сеанса от ранее найденных. Паттерн `session_id`:

1. **Research-задача** генерирует уникальный `session_id` (например `vet-20260318-143052`) и проставляет его на все INSERT/UPDATE
2. Выводит `SESSION_ID: vet-20260318-143052` первой строкой → передаётся через `{{.PrevOutput}}`
3. **Следующие шаги** парсят session_id из `{{.PrevOutput}}` и фильтруют: `WHERE session_id = '...'`

Это надёжнее фильтрации по дате — при нескольких запусках в день каждый получит уникальный session_id.

---

## MCP-серверы

MCP-серверы предоставляют дополнительные инструменты для Claude через протокол MCP (stdio). Все серверы построены на библиотеке [mcp-go](https://github.com/mark3labs/mcp-go) — типобезопасное извлечение аргументов, полное соответствие MCP-спецификации, поддержка параллельных tool calls.

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
| **mcp-word** | `read_document`, `write_document`, `create_document` | Реализован |
| **mcp-pdf** | `read_pdf`, `extract_text`, `extract_tables` | Реализован |
| **mcp-openapi** | Динамические (из OpenAPI-спеки) | Реализован |
| **mcp-whisper** | `transcribe_audio`, `batch_transcribe`, `list_models`, `download_model` | Реализован |
| **mcp-google** | — | Stub |
| **mcp-database** | `query`, `execute`, `list_tables`, `describe_table`, `check_exists`, `insert`, `batch_insert` | Реализован |
| **mcp-exportby** | `sync_catalog`, `get_unanalyzed`, `check_new`, `get_stats`, `get_pending_count`, `export_leads_excel`, `mark_exported`, `reject_companies` | Реализован |

### mcp-openapi — интеграция внешних API

`mcp-openapi` — MCP-сервер, который динамически генерирует инструменты из OpenAPI v2/v3 спецификации. Каждый эндпоинт API становится отдельным MCP-инструментом.

#### Конфигурация

```yaml
mcp_servers:
  - name: crm-api
    command: ./bin/mcp-openapi
    env:
      OPENAPI_SPEC_PATH: specs/crm-openapi.yaml
      OPENAPI_BASE_URL: https://api.crm.example.com/v2
      OPENAPI_AUTH_TYPE: bearer
      OPENAPI_AUTH_TOKEN: ${CRM_API_TOKEN}
      OPENAPI_INCLUDE_TAGS: contacts,deals
```

Множественные API — отдельные записи в `mcp_servers` с разными env vars:

```yaml
mcp_servers:
  - name: crm-api
    command: ./bin/mcp-openapi
    env:
      OPENAPI_SPEC_PATH: specs/crm.yaml
      OPENAPI_AUTH_TYPE: bearer
      OPENAPI_AUTH_TOKEN: ${CRM_TOKEN}

  - name: billing-api
    command: ./bin/mcp-openapi
    env:
      OPENAPI_SPEC_PATH: specs/billing.yaml
      OPENAPI_AUTH_TYPE: apikey
      OPENAPI_API_KEY: ${BILLING_KEY}
```

#### Переменные окружения

**Основные:**

| Переменная | Обяз. | Описание |
|---|---|---|
| `OPENAPI_SPEC_PATH` | **Да** | Путь к OpenAPI-спецификации (JSON/YAML) |
| `OPENAPI_BASE_URL` | Нет | Переопределение base URL из спеки |
| `OPENAPI_TLS_INSECURE` | Нет | `true` — отключить проверку TLS-сертификата (самоподписанные) |
| `OPENAPI_EXTRA_HEADERS` | Нет | Доп. заголовки `Key:Value,Key2:Value2` |
| `OPENAPI_TIMEOUT` | Нет | HTTP timeout (default: `30s`) |
| `OPENAPI_MAX_TOOLS` | Нет | Лимит инструментов (default: 50) |

**Аутентификация — простые типы:**

| Переменная | Обяз. | Описание |
|---|---|---|
| `OPENAPI_AUTH_TYPE` | Нет | `bearer`, `apikey`, `basic`, `oauth2`, `oauth2_client_credentials` |
| `OPENAPI_AUTH_TOKEN` | Нет | Bearer token |
| `OPENAPI_API_KEY` | Нет | API key |
| `OPENAPI_API_KEY_NAME` | Нет | Имя заголовка/параметра (default: `X-API-Key`) |
| `OPENAPI_API_KEY_IN` | Нет | `header` (default) или `query` |
| `OPENAPI_BASIC_USER` / `OPENAPI_BASIC_PASS` | Нет | Basic auth |

**Аутентификация — OAuth2 client credentials (автоматическое управление токенами):**

| Переменная | Обяз. | Описание | Default |
|---|---|---|---|
| `OPENAPI_OAUTH2_TOKEN_URL` | **Да*** | URL получения токена (POST) | — |
| `OPENAPI_OAUTH2_CLIENT_ID` | **Да*** | Client ID / логин | — |
| `OPENAPI_OAUTH2_CLIENT_SECRET` | **Да*** | Client secret / пароль | — |
| `OPENAPI_OAUTH2_REFRESH_URL` | Нет | URL refresh токена (fallback: re-auth) | — |
| `OPENAPI_OAUTH2_ID_FIELD` | Нет | Имя поля логина в JSON body | `client_id` |
| `OPENAPI_OAUTH2_SECRET_FIELD` | Нет | Имя поля пароля в JSON body | `client_secret` |
| `OPENAPI_OAUTH2_GRANT_TYPE` | Нет | Значение grant_type (пустая строка = не включать) | `client_credentials` |
| `OPENAPI_OAUTH2_TOKEN_IN` | Нет | Куда инжектировать токен: `header` или `query` | `header` |
| `OPENAPI_OAUTH2_TOKEN_PARAM` | Нет | Имя query param при `TOKEN_IN=query` | `access_token` |

*\* Обязательны при `OPENAPI_AUTH_TYPE=oauth2` или `oauth2_client_credentials`*

**Фильтрация:**

| Переменная | Обяз. | Описание |
|---|---|---|
| `OPENAPI_INCLUDE_TAGS` | Нет | Фильтр по тегам (через запятую) |
| `OPENAPI_INCLUDE_PATHS` | Нет | Фильтр по path-префиксам |
| `OPENAPI_INCLUDE_OPS` | Нет | Фильтр по operationId |
| `OPENAPI_EXCLUDE_OPS` | Нет | Исключить по operationId |

#### OAuth2 (client credentials flow)

Для API с динамической авторизацией. MCP-сервер автоматически получает токен при старте, обновляет при истечении и инжектирует в каждый запрос — Claude не тратит ходы на аутентификацию.

**Стандартный OAuth2** (token в Authorization: Bearer):

```yaml
mcp_servers:
  - name: my-api
    command: ./bin/mcp-openapi
    env:
      OPENAPI_SPEC_PATH: specs/my-api.yaml
      OPENAPI_AUTH_TYPE: oauth2
      OPENAPI_OAUTH2_TOKEN_URL: https://api.example.com/auth/token
      OPENAPI_OAUTH2_CLIENT_ID: ${MY_API_KEY}
      OPENAPI_OAUTH2_CLIENT_SECRET: ${MY_API_SECRET}
      OPENAPI_OAUTH2_REFRESH_URL: https://api.example.com/auth/refresh
```

**Нестандартный OAuth2** (Yeastar PBX: username/password body, token в query param):

```yaml
mcp_servers:
  - name: yeastar-api
    command: ./bin/mcp-openapi
    env:
      OPENAPI_SPEC_PATH: specs/yeastar-pseries.yaml
      OPENAPI_BASE_URL: ${YEASTAR_BASE_URL}/openapi/v1.0
      OPENAPI_AUTH_TYPE: oauth2_client_credentials
      OPENAPI_OAUTH2_TOKEN_URL: ${YEASTAR_BASE_URL}/openapi/v1.0/get_token
      OPENAPI_OAUTH2_CLIENT_ID: ${YEASTAR_CLIENT_ID}
      OPENAPI_OAUTH2_CLIENT_SECRET: ${YEASTAR_CLIENT_PASSWORD}
      OPENAPI_OAUTH2_ID_FIELD: username
      OPENAPI_OAUTH2_SECRET_FIELD: password
      OPENAPI_OAUTH2_GRANT_TYPE: ""
      OPENAPI_OAUTH2_TOKEN_IN: query
      OPENAPI_OAUTH2_TOKEN_PARAM: access_token
      OPENAPI_TLS_INSECURE: "true"
      OPENAPI_EXTRA_HEADERS: "User-Agent:OpenAPI"
```

Логика работы:
1. При старте — POST на `TOKEN_URL` с настраиваемыми полями body → получение `access_token`
2. Проактивный refresh — если токен истекает через < 30 сек, обновляет заранее
3. Retry на 401 — автоматический refresh + повтор запроса
4. Fallback — если refresh не удался, полная повторная авторизация
5. Token injection — `header` (Authorization: Bearer) или `query` (?param=token) в зависимости от `TOKEN_IN`

**Важно:** при использовании OAuth2 аутентификация прозрачна для Claude — не нужно включать auth-эндпоинты в OpenAPI спеку и передавать токены в промпте.

#### Встроенные инструменты download_file и batch_download

Каждый mcp-openapi сервер предоставляет инструменты для скачивания файлов:

**`download_file`** — скачивание одного файла:

| Параметр | Описание |
|---|---|
| `url` | Полный URL или относительный путь (автоматически дополняется base URL) |
| `path` | Локальный путь для сохранения |

**`batch_download`** — скачивание множества файлов за один вызов (экономит токены):

| Параметр | Описание |
|---|---|
| `files` | Массив объектов `{url, path}` |

Возвращает per-file результаты (OK с количеством байт или ошибка). Auth и extra headers применяются автоматически. Родительские директории создаются при необходимости. Имена инструментов: `mcp__{server_name}__download_file`, `mcp__{server_name}__batch_download`.

#### Именование инструментов

- Если в спеке задан `operationId: getPetById` → инструмент `getpetbyid`
- Без operationId → `{method}_{path}`: `GET /users/{id}/orders` → `get_users_id_orders`
- В `allowed_tools`: `mcp__{server_name}__{tool_name}`, например `mcp__crm-api__getcontacts`

#### Пример использования в задаче

```yaml
tasks:
  - name: sync-crm-leads
    prompt: "Fetch new leads from CRM and sync to local database."
    mcp_servers:
      - crm-api
      - database
    allowed_tools:
      - mcp__crm-api__list_contacts
      - mcp__crm-api__get_contact
      - mcp__database__insert
    permission_mode: dontAsk
    timeout: "5m"
```

#### Ручная проверка

```bash
# Список инструментов
echo '{"jsonrpc":"2.0","id":1,"method":"tools/list"}' | \
  OPENAPI_SPEC_PATH=specs/api.yaml ./bin/mcp-openapi 2>/dev/null | python3 -m json.tool

# Вызов инструмента
echo '{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"getpetbyid","arguments":{"petId":"1"}}}' | \
  OPENAPI_SPEC_PATH=specs/petstore.json OPENAPI_BASE_URL=https://petstore3.swagger.io/api/v3 \
  ./bin/mcp-openapi 2>/dev/null | python3 -m json.tool
```

### mcp-whisper — транскрипция аудио

`mcp-whisper` — MCP-сервер для транскрипции аудиофайлов через [whisper.cpp](https://github.com/ggml-org/whisper.cpp). Работает локально, без внешних API.

#### Установка

```bash
make setup-whisper
```

Клонирует whisper.cpp, компилирует бинарник, скачивает модель `ggml-small.bin`. Требуется cmake и C++ компилятор. Для не-WAV форматов (MP3, FLAC, OGG, M4A) нужен ffmpeg.

Для лучшего качества русского языка рекомендуется `large-v3-turbo` (скачивается через `download_model` или вручную).

#### Конфигурация

```yaml
mcp_servers:
  - name: whisper
    command: ./bin/mcp-whisper
    env:
      WHISPER_BIN: ./data/whisper/bin/whisper-cli
      WHISPER_MODEL: ./data/whisper/models/ggml-large-v3-turbo.bin
      WHISPER_MODELS_DIR: ./data/whisper/models
      WHISPER_THREADS: "8"
```

#### Переменные окружения

| Переменная | Обяз. | Описание |
|---|---|---|
| `WHISPER_BIN` | **Да** | Путь к бинарнику whisper-cli |
| `WHISPER_MODEL` | **Да** | Путь к модели по умолчанию (ggml-*.bin) |
| `WHISPER_MODELS_DIR` | **Да** | Директория с моделями |
| `WHISPER_THREADS` | Нет | Количество потоков (default: 4) |

#### Инструменты

| Инструмент | Описание |
|---|---|
| `transcribe_audio` | Транскрипция одного аудиофайла. Параметры: `path`, `language` (auto/ru/en/...), `translate`, `output_format` (txt/srt/vtt/json). Для srt/vtt/json — файл сохраняется рядом с входным, путь возвращается как `file:<path>` в первой строке |
| `batch_transcribe` | Транскрипция множества файлов за один вызов. Параметры: `files` (массив путей), `output_format`, `language`. Идемпотентно: пропускает файлы с существующим выходным файлом на диске (status: `skipped`). Возвращает JSON с per-file результатами. Экономит токены: 1 tool-call вместо N |
| `list_models` | Список доступных моделей с отметкой скачанных |
| `download_model` | Скачивание модели с Hugging Face. Модели: tiny, base, small, medium, large-v3, large-v3-turbo |

#### Пример: пайплайн для заметок встречи

```yaml
tasks:
  - name: transcribe-audio
    prompt: |
      Транскрибируй аудиофайл: {{.AudioFile}}
      Используй инструмент transcribe_audio с языком "ru".
    mcp_servers: [whisper]
    allowed_tools: [mcp__whisper__transcribe_audio, mcp__whisper__list_models]
    timeout: 60m
    model: haiku

  - name: summarize-transcription
    prompt: |
      Проанализируй транскрипцию и создай:
      1. Краткое резюме
      2. Ключевые темы и решения
      3. Action items
      4. Участники и их позиции
      Транскрипция: {{.PrevOutput}}
    timeout: 10m
    model: haiku

pipelines:
  - name: meeting-notes
    mode: sequential
    session_chain: true
    steps:
      - task: transcribe-audio
      - task: summarize-transcription
    max_iterations: 1
```

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
| POST | `/tasks` | Создать задачу |
| PUT | `/tasks/:name` | Обновить задачу |
| DELETE | `/tasks/:name` | Удалить (с проверкой зависимостей и бэкапом) |
| GET | `/tasks/:name/delete-info` | Анализ зависимостей перед удалением |
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
| DELETE | `/subagents/:name` | Удалить (с проверкой зависимостей и бэкапом) |
| GET | `/subagents/:name/delete-info` | Анализ зависимостей перед удалением |

### Пайплайны

| Метод | Путь | Описание |
|-------|------|----------|
| GET | `/pipelines` | Список |
| GET | `/pipelines/:name` | Получить |
| POST | `/pipelines` | Создать |
| PUT | `/pipelines/:name` | Обновить |
| DELETE | `/pipelines/:name` | Удалить (каскад + бэкап) |
| GET | `/pipelines/:name/delete-info` | Анализ каскадного удаления |
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

### Бэкапы

| Метод | Путь | Описание |
|-------|------|----------|
| GET | `/backups` | Список бэкапов (фильтр: `entity_type`) |
| GET | `/backups/:id` | Детали бэкапа |
| POST | `/backups/:id/restore` | Восстановить из бэкапа |

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
- **Wizard** — генерация конфигураций из текстового описания (задачи, пайплайны, домены, агенты)

### Wizard

Wizard позволяет создавать полные конфигурации (задачи, пайплайны, домены, суб-агенты) из текстового описания на естественном языке.

При применении плана выполняются автоматические проверки:
- Все ссылки валидны (задачи → MCP серверы, агенты; пайплайны → задачи)
- `permission_mode: dontAsk` обязателен для задач с MCP-инструментами или агентами
- Если задача использует агентов, `"Agent"` должен быть в `allowed_tools`
- `allowed_tools` должен содержать хотя бы один инструмент от каждого MCP-сервера задачи (предупреждение)
- `stop_signal` не должен появляться в промптах не-финальных шагов пайплайна

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
