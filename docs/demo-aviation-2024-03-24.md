# Demo: AviationStack + Claude Ecosystem
**Дата:** 24 марта 2026 | **Аудитория:** Руководство компании

---

## Подготовка (сделано)

- [x] OpenAPI-спецификация: `specs/aviationstack.yaml`
- [x] API-ключ в `.env` (AVIATIONSTACK_API_KEY)
- [x] NO_PROXY обновлён
- [ ] `make build` перед демо

---

## Акт 1: Wizard — создание интеграции (5 мин)

**Открыть:** Web UI → Wizard

**Вставить текст:**

```
Мониторинг грузовых авиарейсов через AviationStack API.

MCP-сервер "aviationstack" (mcp-openapi):
- OPENAPI_SPEC_PATH: specs/aviationstack.yaml
- OPENAPI_BASE_URL: http://api.aviationstack.com
- OPENAPI_AUTH_TYPE: apikey
- OPENAPI_API_KEY: ${AVIATIONSTACK_API_KEY}
- OPENAPI_API_KEY_NAME: access_key
- OPENAPI_API_KEY_IN: query

API предоставляет инструменты: getflights (поиск рейсов), getairports (аэропорты), gettimetable (расписание на сегодня), getfutureflights (будущие рейсы).

Домен "aviation-monitoring" с БД flights.db:
- Таблица tracked_flights: flight_iata, airline_name, dep_iata, arr_iata, flight_date, status, dep_scheduled, dep_actual, arr_scheduled, arr_actual, delay_minutes, checked_at
- Таблица hub_snapshots: hub_iata, snapshot_date, total_flights, cargo_flights, delayed_flights, cancelled_flights, report_text, created_at

Задача 1: "aviation-flight-tracker" — найти текущие грузовые рейсы через getflights (фильтр по airline_iata для грузовых авиакомпаний: TK для Turkish Cargo, CV для Cargolux, EK для Emirates), сохранить в tracked_flights, вывести сводку. Использовать aviationstack + database. Timeout 5m. Limit запросов к API: не более 3. Модель: sonnet.

Задача 2: "aviation-hub-monitor" — проверить расписание (вылеты и прилёты) аэропорта FRA (Frankfurt) через gettimetable, выделить грузовые рейсы, посчитать статистику, сохранить снимок в hub_snapshots, вывести детальный отчёт. Использовать aviationstack + database. Timeout 5m. Limit запросов к API: не более 2. Модель: sonnet.

Задача 3: "aviation-compile-report" — на основе {{.PrevOutput}} прочитать данные из БД и создать Excel-отчёт в data/aviation-monitoring/reports/ с таблицей рейсов, задержек, статистикой. Использовать database + excel + filesystem. Timeout 5m. Модель: sonnet.

Задача 4: "aviation-deliver-report" — отправить в Telegram краткую сводку из {{.PrevOutput}} текстом + приложить Excel-файл. Использовать telegram + filesystem. Timeout 3m. Модель: haiku.

Пайплайн "aviation-cargo-monitor": sequential, шаги aviation-hub-monitor → aviation-compile-report → aviation-deliver-report, max_iterations 1, без stop_signal.
```

**Действия:**
1. "Generate Plan" → показать preview (MCP сервер, домен, 4 задачи, пайплайн)
2. "Apply" → всё создано
3. Показать SETUP.md

**Говорить:** "Мы не написали ни строчки кода. Текстовое описание на естественном языке — и Claude создал полную интеграцию с внешним API."

---

## Акт 2: CLI — единичный запрос (3 мин)

```bash
make run-task TASK=aviation-flight-tracker
```

**Что увидят:**
- Claude обнаруживает MCP-инструменты AviationStack
- Вызывает getflights с фильтром по грузовым авиакомпаниям
- Парсит JSON-ответ API
- Сохраняет данные в SQLite
- Выдаёт сводку: количество рейсов, задержки, статусы

**Расход API:** ~1-2 запроса

**Говорить:** "Claude сам понял какие инструменты вызвать, какие параметры передать, как обработать ответ и куда сохранить."

---

## Акт 3: Web UI — полный пайплайн (5 мин)

1. Открыть Web UI → Pipelines → `aviation-cargo-monitor` → Run
2. Показать SSE-стриминг в реальном времени

**Что увидят (3 шага):**
1. **Hub Monitor:** расписание FRA → статистика грузовых рейсов → SQLite
2. **Compile Report:** данные из БД → Excel с таблицами и статистикой
3. **Deliver Report:** сводка + Excel → Telegram

**Расход API:** ~2-3 запроса

**Говорить:** "Полная автоматизация: мониторинг → анализ → Excel → Telegram. Можно поставить на расписание — каждые 6 часов."

---

## Ключевые тезисы

| Для кого | Тезис |
|----------|-------|
| Руководство | "Подключаем любой API за 10 минут без программистов" |
| Руководство | "Пайплайн мониторинга: API → БД → Excel → Telegram — автоматически" |
| Руководство | "Масштабируется: таможня, порты, склады — любой API с документацией" |
| Техническим | "OpenAPI-спецификация → MCP-инструменты → Claude сам решает что вызвать" |
| Техническим | "Домены изолируют данные, SQLite-схемы применяются автоматически" |

---

## Бюджет API-запросов

| Этап | Запросов |
|------|----------|
| Тестирование до демо | ~5 |
| Акт 2 (flight tracker) | ~2 |
| Акт 3 (пайплайн) | ~3 |
| **Итого** | **~10 из 100** |

---

## Если что-то пойдёт не так

| Проблема | Решение |
|----------|---------|
| Wizard не генерирует план | Проверить что сервер запущен, `make run` |
| API возвращает ошибку | Проверить ключ: `curl "http://api.aviationstack.com/v1/flights?access_key=$AVIATIONSTACK_API_KEY&limit=1"` |
| Нет грузовых рейсов | Убрать фильтр airline — показать все рейсы FRA |
| Telegram не отправляет | Проверить TELEGRAM_BOT_TOKEN и CHAT_ID в .env |
| Proxy блокирует | Убедиться что api.aviationstack.com в NO_PROXY |
