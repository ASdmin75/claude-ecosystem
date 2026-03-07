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

# Установка hook-бинарника
make install
```

После `make build` в директории `bin/` появятся:
- `server` — основной сервер
- `hook` — hook для Claude Code
- `mcp-*` — MCP-серверы

---

## Конфигурация

Основной файл конфигурации — `tasks.yaml`. Пример:

```yaml
claude_bin: claude

server:
  addr: ":8080"
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
| `prompt` | Go-шаблон промпта (обязательно). Переменные: `{{.PrevOutput}}`, `{{.File}}`, `{{.Iteration}}` |
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

---

## Запуск

### Режим сервера (по умолчанию)

```bash
make run
# или
./bin/server -config tasks.yaml
```

Запускает HTTP-сервер на `:8080`, планировщик cron и watcher файлов.

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
curl -H "Authorization: Bearer <token>" http://localhost:8080/api/v1/subagents

# Создание
curl -X POST -H "Authorization: Bearer <token>" \
  -d '{"name":"reviewer","description":"Code reviewer","instructions":"..."}' \
  http://localhost:8080/api/v1/subagents

# Удаление
curl -X DELETE -H "Authorization: Bearer <token>" \
  http://localhost:8080/api/v1/subagents/reviewer
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
```

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

```yaml
mcp_servers:
  - name: filesystem
    command: ./bin/mcp-filesystem
  - name: excel
    command: ./bin/mcp-excel
    env:
      DATA_DIR: /path/to/data
```

### Управление

```bash
# Статус серверов
curl -H "Authorization: Bearer <token>" http://localhost:8080/api/v1/mcp-servers

# Запуск
curl -X POST -H "Authorization: Bearer <token>" \
  http://localhost:8080/api/v1/mcp-servers/filesystem/start

# Остановка
curl -X POST -H "Authorization: Bearer <token>" \
  http://localhost:8080/api/v1/mcp-servers/filesystem/stop
```

Серверы запускаются лениво — при первом выполнении задачи, которая на них ссылается.

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

Веб-интерфейс доступен по адресу `http://localhost:8080/` после запуска сервера.

### Сборка

```bash
make build-ui
```

Требует Node.js 18+. Собранные файлы встраиваются в Go-бинарник через `go:embed`.

### Для разработки

```bash
cd web
npm install
npm run dev   # Vite dev server на :5173, проксирует /api на :8080
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
curl -X POST http://localhost:8080/api/v1/auth/login \
  -d '{"username":"admin","password":"secret"}'

# Использование
curl -H "Authorization: Bearer v4.local.xxx..." \
  http://localhost:8080/api/v1/tasks
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
  http://localhost:8080/api/v1/tasks
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
