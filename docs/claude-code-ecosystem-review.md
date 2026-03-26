# Обзор экосистемы Claude Code: плагины, фреймворки и инструменты

> Дата: 2026-03-26
> Контекст: анализ популярных open-source проектов для Claude Code — что полезно, что избыточно, что можно адаптировать.

---

## 1. claude-mem — Persistent Memory System

**Репозиторий:** https://github.com/thedotmack/claude-mem
**Лицензия:** AGPL-3.0

### Что делает
Автоматически записывает всё, что Claude Code делает в каждой сессии (tool use, наблюдения, решения), хранит в SQLite + Chroma (vector DB), и выдаёт контекст через semantic search при следующих сессиях.

### Архитектура
- 5 хуков (SessionStart, UserPromptSubmit, PostToolUse, Stop, SessionEnd)
- HTTP-сервис на порту 37777 (Bun runtime)
- SQLite + Chroma vector DB для гибридного поиска
- 3-уровневая прогрессивная загрузка (index → timeline → full details)

### Оценка

| Плюсы | Минусы |
|-------|--------|
| Ничего не теряется между сессиями | Зависимости: Node.js 18+, Bun, Python (uv) |
| Semantic search по истории | 90% записей — шум ("прочитал файл X") |
| Заявленная экономия 10x по токенам | Overhead на каждом tool use |

### Рекомендация
**Не использовать.** Claude Code уже имеет встроенную memory-систему (`.claude/projects/.../memory/`), которая работает без доп. зависимостей и не тратит токены на запись/поиск. Встроенная система — осознанное сохранение важного, а не автозахват всего подряд.

### Что можно взять
- **Идея:** если встроенная memory регулярно упускает важный контекст, можно добавить хук на SessionEnd, который генерирует summary сессии. Но это лучше реализовать через tasks.yaml задачу в claude-ecosystem, а не через внешний плагин.

---

## 2. everything-claude-code — Mega-Collection

**Репозиторий:** https://github.com/affaan-m/everything-claude-code
**Звёзды:** 109k+
**Лицензия:** MIT

### Что делает
Огромная коллекция готовых конфигов для Claude Code:
- **28 субагентов** (code-reviewer, security-reviewer, architect, tdd-guide, go-reviewer и др.)
- **125+ skills** по технологиям (React, Go, Django, Spring Boot, etc.)
- **60+ slash-команд** (/plan, /tdd, /code-review, /build-fix)
- **Rules** для 12 языков (Go, TypeScript, Python, Rust, etc.)
- **Hooks** с профильной системой (minimal/standard/strict)

### Архитектура
- Устанавливается как Claude Code plugin (marketplace) + ручная установка rules через `./install.sh [languages]`
- Хуки идут через `scripts/hooks/run-with-flags.js` с профилями
- Каждый загруженный rule/skill/agent читается в system prompt = токены

### Оценка

| Плюсы | Минусы |
|-------|--------|
| Готовые Go rules (coding-style, testing, security, patterns) | 125 skills — 99% нерелевантны конкретному проекту |
| Хороший code-reviewer агент | Каждый файл в .claude/ ест токены |
| Security-reviewer с OWASP Top 10 | Заточен под JS/React, Go — вторичен |
| Профильная система хуков | Node.js/Bun зависимость для хуков |

### Рекомендация
**Не ставить целиком. Cherry-pick конкретные файлы.**

### Что взято в проект (2026-03-26)
На основе анализа этого репозитория реализовано:
1. **`internal/safepath/`** — shared пакет валидации путей (извлечён из mcp-filesystem, применён к mcp-openapi, mcp-email, mcp-telegram)
2. **Санитизация ошибок в mcp-database** — `safeError()` хелпер, 11 мест исправлено
3. **Path traversal fixes** в 3 MCP серверах с тестами

### Что ещё можно взять
- **Go rules** → `.claude/rules/go.md` — coding style, error handling, testing conventions
- **code-reviewer агент** → `.claude/agents/code-reviewer.md` — адаптированный под Go
- **security-reviewer агент** → `.claude/agents/security-reviewer.md` — для ревью MCP серверов
- **PostToolUse hook: gofmt** — автоформат после Edit

Пример Go rule (адаптированный):
```markdown
---
paths:
  - "**/*.go"
  - "**/go.mod"
---
# Go Rules

## Style
- gofmt/goimports mandatory
- Accept interfaces, return structs
- Keep interfaces small (1-3 methods)

## Errors
- Always wrap: `fmt.Errorf("context: %w", err)`

## Testing
- Table-driven tests, always `go test -race ./...`

## Security
- context.WithTimeout for external calls
- No hardcoded secrets — os.Getenv only
```

---

## 3. superpowers — Agentic Skills Framework

**Репозиторий:** https://github.com/obra/superpowers
**Звёзды:** 115k+
**Лицензия:** MIT

### Что делает
Методологический фреймворк: заставляет Claude Code **сначала думать, потом кодить**. Автоматически активирует workflow:
1. Brainstorming → уточняющие вопросы
2. Git Worktrees → изоляция работы
3. Planning → разбивка на куски по 2-5 минут
4. Subagent-Driven Development → специализированные агенты на каждый кусок
5. TDD → RED-GREEN-REFACTOR цикл
6. Code Review → ревью по плану
7. Branch Completion → merge/cleanup

### Архитектура
- 20+ skills (testing, debugging, collaboration, meta)
- Поддержка Claude Code, Cursor, Codex, OpenCode, Gemini CLI
- 57% Shell, 30% JavaScript

### Оценка

| Плюсы | Минусы |
|-------|--------|
| TDD by default | Overhead: "поправь строку" → brainstorm → plan → TDD |
| Хорошие debugging workflows | Generic фреймворк, не адаптирован под Go |
| Дисциплина разработки | Claude Code уже имеет встроенный plan mode и subagents |

### Рекомендация
**Не ставить.** Claude Code уже умеет планировать (plan mode), запускать субагентов (Agent tool) и отслеживать задачи (tasks). Superpowers форсирует эту дисциплину всегда, даже когда она не нужна.

### Что можно взять
- **TDD skill** как вдохновение для `.claude/agents/tdd-guide.md`
- **Debugging skill** — 4-фазный root cause analysis:
  1. Reproduce → минимальный репродьюсер
  2. Isolate → git bisect / binary search
  3. Diagnose → root cause, не симптом
  4. Fix → тест на регрессию, потом фикс

---

## 4. ui-ux-pro-max-skill — UI/UX Design System Generator

**Репозиторий:** https://github.com/nextlevelbuilder/ui-ux-pro-max-skill
**Звёзды:** 51k+

### Что делает
Генерирует UI/UX дизайн-системы: 67 стилей, 161 палитра, 57 типографических пар, 99 UX-гайдлайнов. Поддержка React, Vue, SwiftUI, Flutter.

### Рекомендация
**Нерелевантен.** Инструмент для фронтенд-дизайнеров. Не имеет отношения к Go-бэкенду или оркестрации CLI.

---

## 5. GSD (Get Shit Done) — Context Engineering System

**Репозиторий:** https://github.com/gsd-build/get-shit-done
**Звёзды:** 42k+

### Что делает
Решает проблему **context rot** — деградации качества Claude Code при длинных сессиях. Подход:
1. **Initialize** — PROJECT.md, REQUIREMENTS.md, ROADMAP.md, STATE.md
2. **Discuss** — решения до планирования
3. **Plan** — атомарные задачи в XML-формате
4. **Execute** — каждая задача в **свежем контексте** (отдельный субагент с чистыми 200k токенов)
5. **Verify** — UAT + автоматический debugging
6. **Ship** — PR из верифицированной работы

### Ключевые идеи
- **Wave-based execution:** группировка по зависимостям (независимые → параллельно, зависимые → последовательно)
- **Atomic commits per task:** каждая задача = отдельный коммит (git bisect friendly)
- **STATE.md:** отслеживание прогресса между задачами

### Оценка

| Плюсы | Минусы |
|-------|--------|
| Свежий контекст на каждую задачу — решает реальную проблему | По сути конкурент claude-ecosystem |
| Wave-based параллелизм с dependency graph | Claude Code plugin, не внешний сервер |
| Atomic commits | Ещё один слой абстракции поверх Claude |
| STATE.md для отслеживания прогресса | |

### Рекомендация
**Не ставить — это конкурент, а не дополнение.** Claude-ecosystem уже делает оркестрацию `claude -p` с задачами, пайплайнами (sequential/parallel) и cron-расписанием.

### Что можно взять в claude-ecosystem
- **STATE.md паттерн** — автоматическое отслеживание состояния пайплайна между шагами. Сейчас в claude-ecosystem пайплайн передаёт `{{.PrevOutput}}`, но нет persistent state файла.
- **Dependency graph для пайплайнов** — сейчас только sequential/parallel, нет DAG (directed acyclic graph) с явными зависимостями между задачами.
- **Atomic commits** — встроить `git commit` после каждого шага пайплайна (опционально).

---

## Сводная таблица

| Проект | Звёзды | Суть | Для проекта | Для workflow |
|--------|--------|------|-------------|--------------|
| claude-mem | — | Auto-memory | Не нужен | Не нужен |
| everything-claude-code | 109k | Configs mega-pack | Cherry-pick rules/agents | Go rules, code-reviewer |
| superpowers | 115k | TDD methodology | Не нужен | TDD/debugging skills |
| ui-ux-pro-max-skill | 51k | UI/UX generator | Нерелевантен | Нерелевантен |
| GSD | 42k | Context orchestration | Конкурент | STATE.md, DAG pipelines |

---

## Общие выводы

### 1. Звёзды ≠ полезность для тебя
109k звёзд everything-claude-code не означают, что тебе нужны 125 skills. Популярные проекты решают generic задачи для широкой аудитории.

### 2. Не ставь фреймворки — бери идеи
Каждый плагин/фреймворк добавляет: зависимости, токены на чтение конфигов, потенциальные конфликты. Один хороший `.claude/agents/code-reviewer.md` > целый фреймворк.

### 3. Встроенные возможности Claude Code покрывают 80%
- Plan mode = brainstorming + planning (superpowers)
- Agent tool = subagents (GSD, superpowers)
- Tasks = progress tracking
- Memory = persistent context (claude-mem)

### 4. Что реально стоит внедрить
- **Go rules** в `.claude/rules/` — минимальный overhead, реальная польза
- **code-reviewer + security-reviewer агенты** — для ревью перед коммитами
- **gofmt hook** на PostToolUse — автоформат
- **STATE.md паттерн** в пайплайнах claude-ecosystem — persistent state между шагами

### 5. Принцип работы с чужими проектами
При оценке любого Claude Code плагина/фреймворка:
1. **Читай, что реально в репозитории**, а не README с маркетинговыми обещаниями
2. **Считай overhead**: сколько файлов загружается в контекст × стоимость токенов × количество сессий
3. **Проверяй overlap** со встроенными возможностями Claude Code
4. **Cherry-pick > install**: скопируй 1-2 файла вместо установки всего фреймворка
5. **Адаптируй** под свой стек вместо использования generic конфигов
