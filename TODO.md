# TODO

## 1. Структурированное логирование (slog) <!-- Выполнено: 2026-03-08 -->
> Добавлено: 2026-03-07 22:11

- [x] Заменить все `fmt.Printf`/`fmt.Println` на `slog.Info`/`slog.Error`/`slog.Warn`
- [x] Добавить slog в task runner (начало/конец задачи, ошибки, таймауты)
- [x] Добавить slog в pipeline runner
- [x] Добавить slog в watcher и scheduler
- [x] Добавить request logging middleware (метод, путь, статус, длительность)
- [x] Настраиваемый уровень логирования через `tasks.yaml` (`server.log_level`)
- [x] Опциональный вывод в файл (`server.log_file`)

## 2. Email-уведомления с результатами задач <!-- Выполнено: 2026-03-08 -->
> Добавлено: 2026-03-07 22:11

- [x] Реализовать MCP сервер для отправки email (`cmd/mcp/email/`)
- [x] Поддержка SMTP конфигурации в `tasks.yaml` (`mcp_servers` секция)
- [x] Инструменты: `send_email(to, subject, body, attachments)`
- [x] Шаблоны писем с результатами выполнения (HTML + plain text fallback)
- [x] Настройка на уровне задачи: `notify.email` поле с адресом(ами)
- [x] Триггеры: on_success, on_failure, always
- [x] Альтернатива: webhook/callback URL для интеграции с внешними системами

## 3. Динамическое обновление UI через SSE <!-- Выполнено: 2026-03-09 -->
> Добавлено: 2026-03-07 22:11

- [x] Бэкенд: публиковать события через event bus при изменении статуса execution
- [x] Бэкенд: SSE endpoint `/api/v1/events` — общий поток событий (task.started, task.completed, task.failed, task.cancelled)
- [x] Фронтенд: SSE клиент с автопереподключением
- [x] Фронтенд: обновление списка executions в реальном времени (без polling)
- [x] Фронтенд: обновление Dashboard счётчиков в реальном времени
- [x] Фронтенд: toast-уведомления при завершении задач
- [x] Убрать `refetchInterval` polling после внедрения SSE

## 4. Конфигурация через .env <!-- Выполнено: 2026-03-08 -->
> Добавлено: 2026-03-07 22:15

- [x] Добавить загрузку `.env` файла при старте сервера (встроенный парсер `internal/config/dotenv.go`)
- [x] Вынести секреты из `tasks.yaml` в `.env`: `PASETO_KEY`, `BEARER_TOKENS`, bcrypt-хеши паролей
- [x] SMTP креды для email MCP: `SMTP_HOST`, `SMTP_PORT`, `SMTP_USER`, `SMTP_PASSWORD`
- [x] Поддержка переменных окружения в `tasks.yaml` через `${ENV_VAR}` синтаксис
- [x] `.env.example` с описанием всех переменных
- [x] `.env` добавить в `.gitignore`
- [x] Приоритет: `.env` < переменные окружения < `tasks.yaml` (явные значения)

## 5. Docker Compose для оркестрации <!-- Выполнено: 2026-03-08 -->
> Добавлено: 2026-03-07 22:15

- [x] `Dockerfile` для server (multi-stage: build Node + build Go → финальный alpine образ)
- [x] MCP серверы включены в единый образ (запускаются mcpmanager внутри контейнера)
- [x] `docker-compose.yml`: server сервис, volumes для data/config/agents
- [x] Volume mounts: `./tasks.yaml`, `./.env`, `./data/`, `./.claude/agents/`
- [x] Health check для server (`/api/v1/dashboard`)
- [x] `make docker-build`, `make docker-up`, `make docker-down` в Makefile
- [x] `.dockerignore` для чистой сборки

## 6. Источник cci.by (БелТПП) для pipeline авиа-лидов
> Добавлено: 2026-03-13

Добавить второй источник компаний — каталог членов БелТПП (https://www.cci.by/o-chlenstve/chleny-beltpp/, 2339 компаний, 117 страниц). Дедупликация с export.by по названию, сайту, телефону.

- [ ] Добавить таблицы `raw_companies_cci` и `cci_scan_progress` в схему домена `export-by-aviation`
- [ ] Добавить колонки `source`, `phone`, `website`, `address` в таблицу `companies`
- [ ] Создать задачу `sync-cci-catalog` (скрапинг через `chrome-devtools` с `initScript` для обхода бот-защиты `navigator.webdriver`)
- [ ] Создать pipeline `cci-sync`
- [ ] Обновить промпт `process-export-by-leads` — дедупликация между источниками (нормализация названий, доменов, телефонов)
- [ ] Обновить `DOMAIN.md` — документация новых таблиц, поля `source`, правил дедупликации
