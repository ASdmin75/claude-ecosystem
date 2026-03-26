Создай пайплайн аналогичный vet-manufacturers-sync, но для лидов производителей оборудования радиационного контроля из Республики Беларусь.

#############################################################################################################################################
 Вариант описания для wizard:

 Создай пайплайн для работы с транспортной CRM 4Logist.

 API спецификация: specs/4log.json
 API URL: ${4LOG_API_URL} (из .env)
 Авторизация: OAuth2 client_credentials, token URL: ${4LOG_API_URL}/oauth/v2/token,
 client_id из env: ${4LOG_CLIENT_ID}, client_secret из env: ${4LOG_CLIENT_SECRET}.

 Нужно два пайплайна:

 1. Пайплайн "4log-daily-report" (ежедневный отчёт):
    - Шаг 1: Загрузить заказы за вчера (post_api_order_list), клиентов (post_api_client_list),
      счета (post_api_invoices_list) — сохранить в локальную БД (домен 4logist)
    - Шаг 2: Сформировать Excel-отчёт с листами: заказы, клиенты, финансы
    - Шаг 3: Отправить отчёт в Telegram
    Расписание: каждый день в 9:00 (cron: 0 9 * * *)

 2. Пайплайн "4log-order-monitor" (мониторинг заказов):
    - Шаг 1: Получить активные заказы и их статусы (post_api_order_list, post_api_order_statuses_list)
    - Шаг 2: Сравнить с предыдущим состоянием в БД, найти изменения статусов
    - Шаг 3: При наличии изменений — отправить уведомление в Telegram
    Расписание: каждые 30 минут (cron: */30 * * * *)

 MCP серверы: openapi (для 4log API), database (SQLite), excel, telegram, filesystem.
 Домен: 4logist (БД для хранения заказов, клиентов, счетов, статусов).

#############################################################################################################################################

Автоматизация производства
Противопожарное оборудование
Системы противопожарной безопасности
Охрана окружающей среды и экология
Измерительные приборы, меры и измерительные комплексы
Лабораторное, технологическое, весовое оборудование
3D станки. Станки для лазерной обработки
Измерительные приборы, меры и измерительные комплексы 
Мониторинг, диагностика и наладка промышленного оборудования
Обрабатывающие центры и системы с ЧПУ 
Промышленная гидравлика 
Промышленная пневматика 
Специальное, нестандартное оборудование. Спецтехника 
Технологии. Технологические линии и оснастка 
Универсальные станки 
Медицинская техника и оборудование 
Медицинские препараты. Биологически активные добавки 
Фармацевтические материалы 
Химические волокна 
Оптические, оптико-электронные приборы и оборудование 
Производство и продажа электронных компонентов 
Электронное управление для мобильного и индустриального применения 
Электротехническое оборудование и изделия

#############################################################################################################################################

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
