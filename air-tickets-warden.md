# Air Tickets Warden — Design Document

**Версия:** 0.2 (draft)
**Дата:** 2026-05-23
**Статус:** Design

---

## 1. Обзор

Telegram-бот для персонального мониторинга цен на авиабилеты из Сербии (основной аэропорт — Белград, BEG) в любую страну Европы. Бот работает по подписочной модели: пользователь создаёт правила мониторинга (маршрут + диапазон дат + условия алерта), а бот регулярно опрашивает несколько источников данных, агрегирует результаты, ведёт историю цен и присылает уведомления при выполнении условий.

### Ключевые принципы

- **Многоисточниковость.** Ни один отдельный API не покрывает весь рынок (особенно из-за лоукостеров Wizz Air и Ryanair). Бот опрашивает несколько источников параллельно.
- **История важнее моментальной цены.** «Дёшево» определяется относительно исторических данных по конкретному маршруту, а не по абсолютному порогу.
- **Гибкость по аэропортам.** Из Белграда часто выгоднее лететь через Будапешт, Софию, Тимишоару или Загреб — бот учитывает альтернативные точки вылета.
- **Антиспам.** Бот не дёргает уведомлениями по любому шевелению цены.

### Что бот НЕ делает (out of scope для MVP)

- Не бронирует билеты автоматически (только присылает ссылки).
- Не управляет платежами.
- Не работает с многосегментными маршрутами на разных билетах (open-jaw, multi-city).
- Не учитывает багаж/места в расчёте (пока).

---

## 2. Контекст и допущения

### Целевой пользователь

Один человек (владелец бота) или узкий круг знакомых. Не публичный сервис, поэтому:

- Не нужна авторизация по платным тарифам, биллинг, multi-tenancy.
- Можно использовать API-ключи на минимальном/бесплатном тарифе.
- Допустимо использовать полу-официальные API (например, эндпоинты Ryanair, которые формально не для публичного использования).

### Маршруты

- **Источник:** Белград (BEG), плюс альтернативные близкие аэропорты — Будапешт (BUD), София (SOF), Тимишоара (TSR), Загреб (ZAG).
- **Назначение:** любые европейские аэропорты.
- **Перевозчики, которые важны:** Air Serbia, Wizz Air, Ryanair, Lufthansa Group (LH/OS/LX), Turkish, easyJet, Vueling, Pegasus, AJet.

### Технологические допущения

- **Python 3.12+** (для совместимости с `ryanair-py` и async-экосистемой).
- **Async-first**: aiogram 3.x, httpx, aiosqlite / asyncpg. Один event loop, без worker pool.
- Один экземпляр бота, без horizontal scaling.
- **SQLite (через `aiosqlite`) на MVP**, миграция на **PostgreSQL (через `asyncpg`)** при росте объёма. Миграции — Alembic с первого дня.
- Деплой на VPS — **Hetzner Cloud (~€4–5/мес)** как референс. Railway / Fly.io возможны, но free-tier ненадёжен для 24/7 жизни.
- Контейнеризация — **Docker + docker-compose** от MVP, упрощает миграцию и локальный dev.

Полный стек с обоснованиями выбора — см. §9 «Технологический стек».

---

## 3. Архитектура

### 3.1. Высокоуровневая схема

```
┌─────────────────────────────────────────────────────────────────┐
│                      Telegram Bot Layer                          │
│  (команды, inline-кнопки, форматирование уведомлений)            │
└──────────────────────────┬──────────────────────────────────────┘
                           │
                           ▼
            ┌──────────────────────────┐
            │  Subscription Manager    │ ←──→  ┌──────────┐
            │  (CRUD над правилами)    │       │   DB     │
            └──────────────┬───────────┘       └──────────┘
                           │
                           ▼
            ┌──────────────────────────┐
            │      Scheduler           │
            │  (cron + приоритеты)     │
            └──────────────┬───────────┘
                           │
                           ▼
            ┌──────────────────────────┐
            │ Multi-Airport Expander   │
            │ (расширение маршрутов)   │
            └──────────────┬───────────┘
                           │
        ┌──────────────────┼──────────────────┐
        ▼                  ▼                  ▼
┌──────────────┐  ┌──────────────┐  ┌──────────────┐
│  Aviasales   │  │     Kiwi     │  │   Ryanair    │
│   adapter    │  │   adapter    │  │   adapter    │
└──────┬───────┘  └──────┬───────┘  └──────┬───────┘
       └──────────────────┼──────────────────┘
                          ▼
            ┌──────────────────────────┐
            │  Aggregator / Dedup      │
            └──────────────┬───────────┘
                           │
                           ▼
            ┌──────────────────────────┐
            │   Price History Store    │
            └──────────────┬───────────┘
                           │
                           ▼
            ┌──────────────────────────┐
            │     Alert Engine         │
            │ (порог / drop / минимум) │
            └──────────────┬───────────┘
                           │
                           ▼
            ┌──────────────────────────┐
            │   Notification Layer     │ ──→ Telegram
            └──────────────────────────┘
```

### 3.2. Компоненты

#### Telegram Bot Layer

Входная точка пользователя. Реализуется на **`aiogram` 3.x** (async-first, встроенный FSM для пошагового диалога `/new`, Pydantic-валидация апдейтов).

**Транспорт:** long-polling (для personal-бота это проще — нет ingress, нет TLS, идентично между локальным dev и продом). Webhook рассматривать только при росте трафика.

**Whitelist пользователей:** middleware на старте проверяет `chat_id` против `ALLOWED_USER_IDS` из конфига. Всё лишнее отбрасывается без ответа. Бот формально публичен в Telegram, и без фильтра случайные люди могут сжечь квоты внешних API.

**Команды:**

- `/new` — диалог создания новой подписки (откуда/куда/диапазон дат/гибкость/порог).
- `/list` — список активных подписок с кратким статусом.
- `/pause <id>`, `/resume <id>`, `/delete <id>` — управление подписками.
- `/search <id>` — внеплановый ручной запуск проверки.
- `/stats <id>` — история цен по подписке: текущий минимум, среднее, мин за 30/60 дней, тренд.
- `/help` — справка.

**Inline-кнопки в уведомлениях:**

- «Посмотреть детали» (раскрывает сегменты, пересадки, длительность)
- «Купить» (deep link на источник, опционально с реферальным кодом)
- «Отключить алерт на этот маршрут»
- «Снизить порог» / «Игнорировать на N дней»

#### Subscription Manager

CRUD-слой над правилами мониторинга. Подписка состоит из:

| Поле | Описание |
|------|----------|
| `id` | UUID |
| `origin` | IATA-код основного аэропорта (например, `BEG`) |
| `origin_alternatives` | Список альтернативных аэропортов вылета |
| `destination` | IATA-код или список (например, `[BCN, MAD, VLC]` для Испании) |
| `date_from`, `date_to` | Диапазон допустимых дат вылета |
| `return_date_from`, `return_date_to` | Опционально, для round-trip |
| `trip_length_min`, `trip_length_max` | Длительность поездки в днях (если round-trip) |
| `max_price` | Абсолютный порог цены (опционально) |
| `max_stops` | Максимум пересадок |
| `max_duration_minutes` | Максимум общей длительности |
| `airlines_whitelist`, `airlines_blacklist` | Фильтры по перевозчикам |
| `alert_strategy` | Стратегия алерта (см. Alert Engine) |
| `cooldown_hours` | Антиспам между уведомлениями |
| `status` | active / paused / archived |

#### Scheduler

Запускает задачи проверки подписок. Не уравнительный cron — приоритезация по близости дат:

- **High-priority** (даты вылета в пределах 14 дней) — каждый час
- **Medium** (15-60 дней) — каждые 4 часа
- **Low** (60+ дней) — раз в день

**Реализация:** **APScheduler 3.x** в режиме `AsyncIOScheduler`. Достаточно для одного инстанса и не тянет за собой Redis. Переход на `arq` имеет смысл, только если Redis уже появится для кэша.

**Rate limiting:** для каждого источника — отдельный **`aiolimiter.AsyncLimiter`** (token bucket) на уровне адаптера. Scheduler не знает про лимиты внешних API — он только триггерит задачи; разруливание квот делегируется адаптерам.

**Jittering:** при постановке задач добавляется случайный сдвиг 0–60 секунд, чтобы не пулять залпом по всем подпискам на одной минуте.

**Persistence джобов:** SQLAlchemyJobStore поверх той же БД — переживает рестарт.

#### Multi-Airport Expander

Перед отправкой запроса в адаптеры расширяет маршрут с учётом гибкости пользователя:

- Если подписка разрешает альтернативные аэропорты, добавляет в очередь запросы для каждого.
- Хранит таблицу-справочник: для каждой пары аэропортов (основной, альтернативный) — примерная стоимость и длительность наземного трансфера (автобус, машина).
- На этапе агрегации эта стоимость прибавляется к цене билета для честного сравнения. Например, билет из Будапешта €40 + трансфер €25 = эффективная цена €65 vs. билет из Белграда €70 — последний выгоднее.

Справочник трансферов (черновик):

| Из БГ в | Способ | Цена | Время |
|---------|--------|------|-------|
| BUD | автобус/машина | €25-40 | ~7 ч |
| SOF | автобус | €20 | ~6 ч |
| TSR | автобус | €15 | ~2.5 ч |
| ZAG | автобус | €30 | ~6 ч |

#### Source Adapters

Каждый источник — отдельный модуль с одинаковым интерфейсом:

```
search(origin, destination, date_from, date_to, options) -> List[Flight]
```

Где `Flight` — нормализованный объект:

```
Flight {
  source: str            # 'aviasales' | 'kiwi' | 'ryanair'
  price: float
  currency: str
  origin: str            # IATA
  destination: str       # IATA
  departure_at: datetime # с timezone аэропорта
  arrival_at: datetime
  segments: List[Segment]
  airline: str           # код основного перевозчика
  flight_number: str
  stops: int
  duration_minutes: int
  booking_url: str
  fetched_at: datetime
}
```

**Стартовый набор адаптеров:**

1. **Aviasales / Travelpayouts adapter** — основа. Бесплатный API, хорошо покрывает Air Serbia, классических перевозчиков, частично Wizz Air. Партнёрская программа даёт реферальные ссылки.
2. **Kiwi (Tequila) adapter** — лучше с лоукостерами, поддерживает виртуальные пересадки (Kiwi сам стыкует рейсы разных перевозчиков, чего обычные GDS не делают). Важно для нестандартных маршрутов.
3. **Ryanair adapter** — через библиотеку `ryanair-py`, которая использует полу-официальный эндпоинт `services-api.ryanair.com`. Покрывает только Ryanair, но критично, если этот перевозчик не виден в Aviasales.

**Опциональный расширенный набор:**

4. **Amadeus self-service adapter** — 2000 бесплатных запросов в месяц. Резерв и дополнительная валидация цен через GDS.
5. **Wizz Air monitoring** — через парсинг сайта или подписку на промо-рассылку (без официального API).

**Стандартная реализация адаптера:**

- HTTP-клиент — **`httpx.AsyncClient`** с persistent connection pool.
- Retry — **`tenacity`** с экспоненциальным backoff на 429/5xx (3 попытки, jitter).
- Rate limit — **`aiolimiter`** (token bucket), параметры из конфига на источник.
- **Circuit breaker** — `pybreaker` (или собственный счётчик): после N последовательных провалов источник «выключается» на cooldown-период. Логируется в `api_call_log`, отдельный metric. Защищает от траты квоты и зависаний всего цикла.
- **Sanity check на выходе адаптера:** цена < `MIN_REASONABLE_PRICE` (например, €10) или > `MAX_REASONABLE_PRICE` (€5000) → flag, не пропускаем в агрегатор, пишем в лог. Защита от «битой цены €1» и outlier-ов парсинга Wizz.
- Каждый запрос логируется в `api_call_log` (endpoint, status, latency, остаток квоты, error).
- Не валит общий цикл при своей ошибке — `asyncio.gather(..., return_exceptions=True)` на уровне Aggregator.

**Замечание по Kiwi Tequila:** API проходил реструктуризацию, доступ к бесплатному tier-у ограничен. **Перед закладыванием в код проверить актуальность ключей.** Альтернативы: **Duffel** (хороший API, есть test mode), **FlightAPI.io**, **SerpAPI Google Flights** (платный, но покрывает Wizz/Ryanair).

#### Cache Layer

Между Source Adapters и Aggregator — кэш ответов адаптеров. На MVP — **`aiocache` (in-memory)**, при росте — **Redis**.

**Зачем нужен:** при 5 альтернативных аэропортах × 3 источника × N подписок одна и та же пара (BEG→BCN, 10-20 июля) запрашивается многократно за час. Без кэша лимиты бесплатных API сгорят за день.

**Ключ:** `(source, origin, destination, date_from, date_to, options_hash)`.
**TTL:** 15 минут для high-priority подписок, 60 минут для остальных. Конфигурируется per-source.
**Инвалидация:** только по TTL. Принудительный сброс — командой `/refresh <id>` (та же, что `/search`, но игнорирует кэш).

Запись в `price_observations` идёт **всегда** (даже на cache hit), чтобы история не имела дыр от кэша. Но `api_call_log` пополняется только при реальном HTTP-запросе.

#### Currency Normalizer

Адаптеры могут отдавать цены в разных валютах: Aviasales — в зависимости от `currency` параметра (RUB/USD/EUR), Kiwi — в EUR, Ryanair — в локальной валюте маршрута (EUR/GBP/RON/...).

Все цены в системе приводятся к **базовой валюте (EUR)** перед записью в Price History Store и сравнением в Alert Engine. Иначе `$87 < €100` → ложный алерт.

**Источник курсов:** **ECB daily reference rates** (`https://www.ecb.europa.eu/stats/eurofxref/eurofxref-daily.xml`). Бесплатно, обновляется в будний день ~16:00 CET.
**Кэш курсов:** таблица `fx_rates(date, currency, rate_to_eur)`, обновление раз в сутки.
**Fallback:** если ECB недоступен, используется последний известный курс из БД (с пометкой `stale_rate=true` в логах).

В каждом `price_observation` сохраняем **обе цены** — оригинальную (`price`, `currency`) и нормализованную (`price_eur`). Это нужно для отображения пользователю и для аудита.

#### Aggregator / Deduplication

Собирает результаты со всех адаптеров, дедуплицирует и сортирует.

**Ключ дедупликации:** `(airline, flight_number, departure_at_date)`. Один и тот же физический рейс может прилететь от 3 адаптеров с разными ценами — оставляем минимальную, но сохраняем все источники в метаданных (для отладки и отображения «доступно на: Aviasales, Kiwi»).

**Особый случай — multi-segment рейсы.** Дедупликация по составному ключу из всех сегментов. Если хоть один сегмент отличается — это разные варианты.

**Сортировка:** по эффективной цене (`price_eur + transfer_cost_eur`), не по сырой цене билета. Так Будапешт с трансфером сравнивается с Белградом по-честному.

**Pipeline в Aggregator:**

1. Сбор результатов из всех адаптеров (`asyncio.gather(return_exceptions=True)`).
2. Currency Normalizer: каждый `Flight.price` → `Flight.price_eur`.
3. Sanity check (вторая линия — после адаптера): отбрасываем рейсы с `price_eur < €10` или `> €5000` (логируем с пометкой `outlier=true`).
4. Прибавление стоимости трансфера для альтернативных аэропортов.
5. Дедупликация по `(airline, flight_number, departure_date)`.
6. Сортировка по эффективной цене.

Pipeline идёмпотентный — пересчёт того же набора Flight даёт тот же результат.

#### Price History Store

Хранилище временных рядов цен. Минимальная схема:

```
price_observations (
  id              int pk,
  route_key       str,        -- 'BEG-BCN-2026-07-15'
  subscription_id uuid,       -- nullable, для отслеживания, чьим запросом получено
  price           float,      -- оригинальная цена
  currency        str,        -- оригинальная валюта
  price_eur       float,      -- нормализованная к EUR
  source          str,
  flight_signature str,       -- airline+flight_number, для идентификации рейса
  departure_at    timestamp,  -- TZ-aware, аэропорт вылета
  observed_at     timestamp,  -- UTC
  outlier         bool default false,
  raw_payload     json
)

-- Идемпотентность: один и тот же рейс из одного источника в один час
-- не должен попадать в БД дважды (защита от retry-ов).
UNIQUE INDEX idx_obs_dedup ON price_observations (
  flight_signature, departure_at, source, date_trunc('hour', observed_at)
);

-- Индексы для быстрых агрегатов в Alert Engine
INDEX idx_obs_route_time ON price_observations (route_key, observed_at DESC);
INDEX idx_obs_route_signature ON price_observations (route_key, flight_signature, observed_at DESC);
```

**Политика хранения:**

- Близкие даты (< 30 дней до вылета): 1 точка в час
- Средние (30-90): 1 точка в сутки
- Дальние (> 90): 1 точка в сутки + дедупликация по дню
- Через 14 дней после даты вылета: удаление

Реализуется через периодический job (`apscheduler`, раз в сутки): downsampling + удаление протухших.

**Outlier handling:** при записи помечается `outlier=true` для цен, которые более чем в 3× ниже медианы по `route_key` за последние 30 дней (с учётом непустой выборки ≥ 20 точек). Такие записи **не участвуют** в агрегатах Alert Engine, но остаются в БД для аудита.

**При переходе на PostgreSQL** — рассмотреть расширение **TimescaleDB** для `price_observations`. Hypertable + автоматический downsampling из коробки. Но не для MVP.

Из этих данных Alert Engine считает агрегаты: скользящее среднее, медиана, минимум за N дней.

#### Alert Engine

Решает, отправлять ли уведомление. Поддерживает несколько стратегий, выбирается на уровне подписки:

| Стратегия | Логика |
|-----------|--------|
| `absolute_threshold` | Цена ≤ `max_price` |
| `relative_drop` | Цена ≤ среднее за 30 дней × (1 − `drop_pct`) |
| `historical_minimum` | Новый минимум за последние N дней (например, 60) |
| `sudden_drop` | Цена упала на ≥ X% по сравнению с предыдущей точкой |
| `combined` | Любой из условий выше срабатывает (OR) |

**Антиспам:**

- Cooldown между алертами по одной подписке (по умолчанию 6 часов).
- Дедупликация: если та же цена для того же рейса уже отправлялась — не алертим.
- «Stable price» защита: если цена колеблется в пределах ±2% от уже алертенной — не повторяем.

**Логирование решений:** для каждого срабатывания/несрабатывания пишется запись с входными данными и итогом. Это нужно для отладки («почему алерт не пришёл?»).

**Dry-run режим.** На уровне подписки — флаг `dry_run: bool`. В этом режиме Alert Engine проходит весь pipeline, но **не отправляет** уведомление в Telegram, только пишет в `alerts_sent` с пометкой `dry_run=true`. Используется для:

- Тюнинга новых стратегий на исторических данных (replay из CLI: `python -m warden.replay --subscription <id> --strategy ...`).
- Тихого тестирования перед включением нового маршрута.

**Тайм-зоны (важно):**

- В БД всё в UTC (`TIMESTAMP WITH TIME ZONE` на Postgres, ISO-8601 строки с явным `+00:00` на SQLite).
- `departure_at` / `arrival_at` — TZ-aware с зоной аэропорта.
- Резолв TZ аэропорта — через **`airportsdata`** package (offline-данные, без внешних запросов).
- Для отображения в Telegram — конвертация в TZ аэропорта вылета через `zoneinfo`.
- Все сравнения дат в Alert Engine — в UTC; форматирование — в локальной TZ непосредственно перед отправкой.

#### Notification Layer

Форматирует и отправляет уведомления в Telegram. Структура сообщения:

```
✈️ Дешёвый билет найден!

BEG → BCN
🗓 15 июля 2026, 09:30 → 12:45
💰 €87 (Wizz Air, прямой, 3ч 15м)

📊 Это −34% от среднего за 30 дней (€132)
📊 Новый минимум за последние 60 дней
📊 Доступно на: Aviasales, Kiwi

[Купить] [Детали] [Отключить алерт]
```

Если выгоднее лететь из альтернативного аэропорта — отдельная пометка:

```
💡 Из Будапешта (BUD) дешевле:
   €52 + €25 трансфер = €77 эффективно
```

#### Observability

Бот работает 24/7 без присмотра — без observability упавший адаптер или выжженная квота заметятся через неделю по молчанию уведомлений.

**Логи:** **`structlog`** в JSON-формате. Каждая запись — с `subscription_id`, `source`, `route_key`, `trace_id` (UUID на цикл проверки одной подписки). Уровень из конфига.

**Метрики:** **`prometheus_client`** в pull-режиме (endpoint `/metrics` на отдельном порту). Минимальный набор:

- `warden_adapter_requests_total{source, status}` — counter
- `warden_adapter_latency_seconds{source}` — histogram
- `warden_adapter_quota_remaining{source}` — gauge
- `warden_alerts_sent_total{strategy}` — counter
- `warden_alerts_suppressed_total{reason}` — counter (cooldown / dedup / stable_price)
- `warden_subscriptions_active` — gauge
- `warden_db_size_bytes` — gauge

Скрейпить можно из соседнего docker-сервиса (Grafana Cloud free tier поддерживает scrape по public URL через agent).

**Error tracking:** **Sentry** (`sentry-sdk` с интеграциями для httpx, aiogram, SQLAlchemy). Конфигурируется DSN из env. Бесплатный tier (5K errors/мес) с запасом покрывает personal-проект.

**Health endpoint:** `/health` отвечает 200 если бот жив и БД доступна, иначе 503. Используется для uptime-мониторинга (UptimeRobot free tier).

**Команда `/health`** в Telegram-боте — выводит текущее состояние: aliveness каждого адаптера (по `api_call_log` за последний час), размер БД, количество активных подписок, последний успешный цикл проверки.

#### Config & Secrets

Конфиг — через **`pydantic-settings`**: type-safe, валидация на старте, source = env-variables + `.env`-файл локально, secrets через env в проде.

Структурирован по группам:

```python
class TelegramSettings(BaseSettings):
    bot_token: SecretStr
    allowed_user_ids: list[int]

class SourcesSettings(BaseSettings):
    aviasales_token: SecretStr
    kiwi_api_key: SecretStr | None
    amadeus_client_id: SecretStr | None
    amadeus_client_secret: SecretStr | None

class RateLimitsSettings(BaseSettings):
    aviasales_rps: float = 1.0
    kiwi_rps: float = 0.5
    ryanair_rps: float = 0.3

class AlertDefaultsSettings(BaseSettings):
    cooldown_hours: int = 6
    drop_pct: float = 0.25
    stable_price_band_pct: float = 0.02

class ObservabilitySettings(BaseSettings):
    sentry_dsn: SecretStr | None
    log_level: str = "INFO"
    metrics_port: int = 9090
```

**Секреты в проде:** **`.env` смонтированный в контейнер**, либо **Docker secrets**. Не коммитятся; `.env.example` лежит в репо как референс.

**Проверка на старте:** все обязательные поля валидируются Pydantic-ом, при ошибке бот падает с понятным сообщением. Альтернатива «оно потом упадёт где-то глубоко» — недопустима для 24/7 сервиса.

---

## 4. Поток данных (end-to-end сценарий)

**Сценарий:** пользователь создал подписку BEG → Барселона (BCN), даты 10-20 июля 2026, гибкость по аэропортам вылета вкл., стратегия `combined` (порог €100 ИЛИ -25% от среднего).

1. Scheduler триггерит проверку подписки (дата вылета через ~50 дней → medium-priority, проверка каждые 4 часа). Назначается `trace_id` для сквозного логирования всего цикла.
2. Subscription Manager отдаёт правило → передаётся в Multi-Airport Expander.
3. Expander разворачивает в 5 пар: (BEG, BCN), (BUD, BCN), (SOF, BCN), (TSR, BCN), (ZAG, BCN). Для каждой пары + диапазон дат — формируются запросы.
4. **Cache Layer** проверяет каждый ключ `(source, origin, destination, date_from, date_to)`. На hit — возвращает кэшированный список Flight. На miss — идёт дальше.
5. На miss-запросы пуляются параллельно в Aviasales, Kiwi, Ryanair adapters. Каждый адаптер:
   - Соблюдает свой rate limit (`aiolimiter`).
   - Применяет retry/backoff (`tenacity`).
   - Проверяется circuit breaker — если адаптер «выключен», запрос пропускается.
   - Делает sanity check на ответе.
   - Возвращает нормализованный список Flight.
   - Кладёт результат в Cache Layer.
6. **Currency Normalizer** добавляет `price_eur` каждому Flight по актуальному курсу из `fx_rates`.
7. Aggregator сливает результаты, дедуплицирует. К `price_eur` из альтернативных аэропортов прибавляется стоимость трансфера → `effective_price_eur`.
8. Каждое наблюдение записывается в Price History Store (с outlier-проверкой).
9. Alert Engine для каждого Flight проверяет условия стратегии подписки:
   - Цена ≤ €100? → проверяет.
   - Цена ≤ среднее_за_30_дней × 0.75? → достаёт среднее из истории (исключая `outlier=true`), проверяет.
   - Сработала хоть одна — кандидат на алерт.
10. Кандидаты проходят антиспам (cooldown, дедупликация по уже отправленным, stable-price защита).
11. Если подписка в `dry_run` — запись в `alerts_sent` с пометкой, без отправки в Telegram.
12. Иначе — Notification Layer форматирует и отправляет в Telegram. Сохраняется `message_id` для возможной правки.
13. Метрики цикла (latency, кол-во найденных Flight, кол-во алертов) обновляются в Prometheus.

---

## 5. Структура данных (схема БД)

Миграции — **Alembic** с первой версии. SQLite-совместимый синтаксис на MVP, переход на Postgres — без переписывания DDL.

```
subscriptions
  id (uuid pk), user_chat_id (bigint, indexed),
  origin (str), origin_alternatives (json),
  destination (json — list of IATA),
  date_from, date_to, return_date_from, return_date_to,
  trip_length_min, trip_length_max,
  max_price, max_stops, max_duration_minutes,
  airlines_whitelist (json), airlines_blacklist (json),
  alert_strategy (str), alert_params (json),
  cooldown_hours, dry_run (bool default false),
  status (active/paused/archived),
  created_at, updated_at

price_observations
  id (bigint pk),
  route_key (str, indexed) — 'BEG-BCN-2026-07-15'
  subscription_id (uuid fk → subscriptions.id ON DELETE SET NULL),
  price (float), currency (str),
  price_eur (float),
  source (str),
  flight_signature (str) — 'W6-2643',
  departure_at (timestamptz),
  observed_at (timestamptz),
  outlier (bool default false),
  raw_payload (json)

  UNIQUE (flight_signature, departure_at, source, hour_bucket)
  INDEX (route_key, observed_at DESC)
  INDEX (route_key, flight_signature, observed_at DESC)

alerts_sent
  id (bigint pk),
  subscription_id (uuid fk → subscriptions.id ON DELETE CASCADE),
  flight_signature (str),
  price_eur (float),
  strategy_triggered (str),
  sent_at (timestamptz),
  message_id (bigint),
  dry_run (bool default false)

  INDEX (subscription_id, sent_at DESC)

api_call_log
  id (bigint pk),
  source (str), endpoint (str),
  status_code (int), duration_ms (int),
  rate_limit_remaining (int nullable),
  error (str nullable),
  called_at (timestamptz)

  INDEX (source, called_at DESC)

fx_rates
  date (date pk part),
  currency (str pk part) — 'USD', 'GBP', ...
  rate_to_eur (float),
  fetched_at (timestamptz)

  PRIMARY KEY (date, currency)

scheduler_runs                 -- для /health и метрик
  id (bigint pk),
  subscription_id (uuid fk),
  started_at, finished_at,
  trace_id (uuid),
  flights_found (int),
  alerts_generated (int),
  status (success/partial/failed),
  error (str nullable)
```

**Foreign keys:** `subscription_id` в `price_observations` — `ON DELETE SET NULL` (история переживает удаление подписки), в `alerts_sent` — `ON DELETE CASCADE` (алерты бессмысленны без подписки).

**JSON-поля** на SQLite работают через `JSON1` extension, на Postgres — нативный `jsonb`. SQLAlchemy 2.x абстрагирует.

---

## 6. План внедрения (MVP → Full)

### MVP (1-2 недели)

Цель: рабочий бот для одного маршрута, один источник, ручные алерты. **Уже с базовой инфраструктурой**, чтобы не переделывать.

**Функционал:**

- Telegram Bot Layer (aiogram 3.x): команды `/new`, `/list`, `/delete`, whitelist пользователей.
- Subscription Manager на SQLite, без альтернативных аэропортов.
- Один адаптер: **Aviasales / Travelpayouts**.
- Scheduler (APScheduler async): один общий cron, проверка раз в час.
- Currency Normalizer с ECB-курсами.
- Price History Store: запись + запрос «минимум за N дней».
- Alert Engine: `absolute_threshold` и `historical_minimum`, cooldown.
- Notification Layer: текстовое уведомление без inline-кнопок.

**Инфраструктура (с первого дня):**

- Alembic-миграции.
- pydantic-settings для конфига.
- structlog + Sentry.
- Docker + docker-compose.
- `pytest` с базовыми тестами адаптера (фикстуры через `respx`).
- GitHub Actions: lint (ruff) + типы (mypy) + tests.

### v1.0 (следующие 1-2 недели)

- Добавление Kiwi adapter, Ryanair adapter.
- Aggregator с дедупликацией и sanity check.
- Cache Layer (in-memory).
- Multi-Airport Expander с справочником трансферов.
- Circuit breaker для адаптеров.
- Alert Engine: `relative_drop`, `combined`, антиспам полностью (cooldown + dedup + stable-price).
- Inline-кнопки в уведомлениях.
- Команда `/stats` со статистикой по подписке.
- Команда `/health` с aliveness адаптеров.
- Prometheus-метрики на `/metrics`.
- Dry-run режим для подписок.
- Бэкап SQLite в отдельный том раз в сутки.

### v1.1+ (по необходимости)

- Переход SQLite → PostgreSQL (опционально TimescaleDB для price_observations).
- Redis для Cache Layer (вместо in-memory).
- Amadeus adapter / Duffel adapter как резерв.
- Графики истории цен — matplotlib → PNG → `send_photo` в Telegram (по команде `/stats`).
- Trend Analyzer — еженедельная сводка по подпискам.
- Calendar Heatmap картинкой.
- Smart suggestions («куда дёшево из БГ на эти выходные»).
- Поддержка round-trip с двумя независимыми билетами разных авиакомпаний (Kiwi-style virtual interlining) — требует переработки модели Flight в Itinerary.
- Wizz Air monitoring через headless browser (Playwright в отдельном контейнере).

---

## 7. Риски и митигации

| Риск | Митигация |
|------|-----------|
| Бан/блокировка адаптера Ryanair (неофициальный API) | Graceful fallback, circuit breaker, не валить общий цикл. Мониторинг доступности через `/health`. |
| Превышение лимитов бесплатных API | Cache Layer (15-60 мин), jittering scheduler-а, приоритезация по близости дат, `aiolimiter` per-source. Метрика `warden_adapter_quota_remaining`. |
| Изменение схемы ответов API | Pydantic-валидация на выходе адаптера, контрактные тесты с фикстурами (`respx` + записанные ответы), логирование `raw_payload` в БД. Sentry-алерт на пик ошибок парсинга. |
| Раздувание БД | Политика хранения с downsampling и удалением старых наблюдений (см. Price History Store). Метрика `warden_db_size_bytes`. |
| Спам уведомлениями | Cooldown, дедупликация, stable-price защита (±2%). Кнопка «отключить алерт». |
| Ложные срабатывания (битая цена €1, цена в копейках) | Двухуровневый sanity check: внутри адаптера (абсолютные пороги) + outlier-флаг в БД (относительно медианы маршрута). Outlier'ы не участвуют в агрегатах. |
| Часовые пояса | UTC в БД, TZ-aware datetime через `zoneinfo`. Резолв аэропорта → `airportsdata` offline. Цены в EUR после нормализации. Покрыто тестами с `freezegun`. |
| **Скачки валютных курсов / ECB недоступен** | Кэш курсов в БД, fallback на последний известный, лог `stale_rate=true`. Sentry-алерт если курс не обновляется > 3 дней. |
| **Deprecation внешних API (Kiwi Tequila переезд)** | Изоляция через интерфейс `BaseAdapter`. План замены — Duffel/FlightAPI/SerpAPI. Контрактный тест ловит breaking changes. |
| **Перегрев event loop при синхронной операции** | Все I/O через async, CPU-heavy (парсинг) — через `asyncio.to_thread`. Метрика latency цикла поможет заметить деградацию. |
| **Потеря данных при рестарте** | SQLAlchemyJobStore для APScheduler (джобы переживают рестарт). Бэкап БД раз в сутки. Идемпотентность записи в `price_observations`. |
| **Whitelist обход или утечка токена** | Все секреты в env / Docker secrets, не в git. ALLOWED_USER_IDS как первая линия. Логирование отброшенных апдейтов для аудита. |

---

## 8. Открытые вопросы

**Решено в v0.2:**

- ~~Где деплоить?~~ → Hetzner Cloud (~€4-5/мес). Free-tier Railway/Fly.io ненадёжен для 24/7.
- ~~Webhook или long-polling?~~ → Long-polling для MVP.
- ~~SQLite или Postgres с начала?~~ → SQLite на MVP, путь к Postgres расчищен (Alembic, SQLAlchemy 2.x async, `aiosqlite` → `asyncpg` сменой драйвера).

**Остаются открытыми:**

- **Статус Kiwi Tequila в 2026.** Нужна верификация: остался ли бесплатный tier, выдают ли новые ключи. План B — Duffel (есть test mode).
- **Round-trip с virtual interlining.** Требует переработки `Flight` → `Itinerary` (список Flight как один билет). Отложено в v1.1+; пока считаем round-trip как пара независимых one-way.
- **Наземный транспорт.** Статический справочник vs Omio API. Прикинуть после v1.0: если справочник часто ошибается (жалобы пользователя) — мигрировать.
- **Wizz Discount Club.** Headless Playwright в отдельном контейнере (запуск раз в день) — рабочий вариант, но требует поддержки. Альтернатива — email-forwarding через IMAP, но хрупко при изменении шаблонов писем. Решить в v1.1+.
- **Кому реально нужны графики истории.** Если только владелец — `matplotlib` → PNG достаточно. Если планируется расширить круг — мини-веб-морда с FastAPI + Chart.js. Решить по факту использования.
- **Telegram premium-фичи.** Бот может отправлять интерактивные графики через WebApp. Полезно для пристального анализа подписки, но требует HTTPS-endpoint — повышает порог входа. Отложено.

---

## 9. Технологический стек

Все выборы — с обоснованием. Альтернативы перечислены там, где они реально рассматривались.

### Язык и runtime

- **Python 3.12+** — `ryanair-py` использует свежий typing, и Pydantic v2 / SQLAlchemy 2.x работают эффективнее на 3.12.
- Asyncio как основная модель concurrency. Threads — только для CPU-heavy парсинга через `asyncio.to_thread`.

### Telegram-бот

- **`aiogram` 3.x** — async-first, FSM из коробки (нужен для пошагового `/new`), Pydantic-валидация апдейтов, активное развитие.
- Альтернатива `python-telegram-bot` — отвергнута: тяжёлая, более «классическая» API.

### HTTP-клиенты и устойчивость

- **`httpx.AsyncClient`** — стандарт async HTTP. Поддерживает HTTP/2, connection pool, удобная инжекция в тестах через `respx`.
- **`tenacity`** — retry с экспоненциальным backoff и jitter. Декоратор-based, читается лучше чем кастомные циклы.
- **`aiolimiter`** — token bucket для rate limiting per-source.
- **`pybreaker`** — circuit breaker. Альтернатива — собственный счётчик в Redis/БД, но `pybreaker` готовый.

### Scheduler

- **`APScheduler` 3.x (AsyncIOScheduler)** — достаточно для одного инстанса, переживает рестарт через `SQLAlchemyJobStore`.
- Альтернатива `arq` — отвергнута на MVP (тянет Redis). Рассматривается, если Redis всё равно понадобится для кэша.

### База данных и ORM

- **SQLAlchemy 2.x в async-режиме** — современный API (`AsyncSession`, `select()`-style), хорошо типизирован.
- **`aiosqlite`** (MVP) → **`asyncpg`** (Postgres). Драйвер меняется в connection string без изменений кода.
- **Alembic** — миграции с первого дня, иначе переход SQLite → Postgres будет страданием.
- **Pydantic v2** — для нормализованных моделей домена (`Flight`, `Segment`, `AlertParams`).
- Альтернативы: SQLModel (Pydantic+SQLAlchemy, попроще) — годится, если хочется единого источника схемы. Tortoise ORM — отвергнута: меньше зрелости, хуже с миграциями.

### Кэш

- **MVP:** `aiocache` (in-memory backend) — без внешних зависимостей.
- **v1.1+:** Redis (`redis.asyncio`), если запустится несколько процессов или нужно делиться кэшем с replay-CLI.

### Конфиг

- **`pydantic-settings`** — типизированный конфиг с валидацией на старте. `.env` локально, env-vars / Docker secrets в проде.

### Логирование, метрики, ошибки

- **`structlog`** — структурированные JSON-логи.
- **`prometheus-client`** — pull-метрики на отдельном порту.
- **`sentry-sdk`** — error tracking, бесплатный tier.

### Время и FX

- **`zoneinfo`** (стандартная либа) для timezone-операций.
- **`airportsdata`** — offline-резолв TZ аэропортов по IATA.
- **ECB daily reference rates** — источник FX-курсов, кэшируется в БД.

### Тестирование

- **`pytest` + `pytest-asyncio`** — основа.
- **`respx`** — mock httpx-запросов, фикстуры с записанными ответами адаптеров.
- **`freezegun`** — фиксация времени в тестах Alert Engine.
- **`coverage.py`** — таргет 80% для domain-логики (адаптеры, агрегатор, alert engine), для I/O-обёрток — по факту.

### Линт и типы

- **`ruff`** — линтер + форматтер (заменяет flake8/black/isort).
- **`mypy --strict`** для domain-кода. Для адаптеров — лояльнее, т.к. внешние данные.

### Контейнеризация и деплой

- **Docker + docker-compose** с MVP.
- Базовый образ — `python:3.12-slim`.
- Деплой — **Hetzner Cloud** (~€4-5/мес). Docker Compose, `restart: always`. Простой watchtower для авто-обновлений из registry (опционально).
- CI/CD — **GitHub Actions**: lint → tests → docker build → push to GHCR → ssh-deploy (через `appleboy/ssh-action` или watchtower).

### Сводная таблица

| Слой | MVP | v1.1+ |
|------|-----|-------|
| Bot framework | aiogram 3.x | aiogram 3.x |
| HTTP | httpx + tenacity + aiolimiter | + pybreaker |
| Scheduler | APScheduler async | APScheduler / arq |
| DB | SQLite + aiosqlite | PostgreSQL + asyncpg (опц. TimescaleDB) |
| ORM | SQLAlchemy 2.x async + Alembic | то же |
| Cache | aiocache (in-memory) | Redis |
| Config | pydantic-settings | то же |
| Logs | structlog (JSON) | structlog + Grafana Loki |
| Metrics | prometheus-client | + Grafana Cloud |
| Errors | Sentry | Sentry |
| Tests | pytest + respx + freezegun | + integration с реальными API в nightly |
| Deploy | Docker Compose / Hetzner | то же |

---

## 10. Эксплуатация и качество

### 10.1. Observability — что и зачем мониторить

| Сигнал | Источник | Алерт |
|--------|----------|-------|
| Бот жив | `/health` 200/503 | UptimeRobot — алерт после 2 пропусков |
| Адаптер «потух» | Circuit breaker open + Sentry | Sentry email |
| Квота источника < 20% | `warden_adapter_quota_remaining` | Telegram-сообщение владельцу из самого бота |
| Цикл проверки > 5 мин | `scheduler_runs.finished_at - started_at` | Sentry-event |
| Алертов 0 за 48 часов на active-подписку | агрегат по `alerts_sent` | Telegram-сообщение «подозрительная тишина» |
| Размер БД > 1 GB | `warden_db_size_bytes` | Telegram-сообщение, проверить retention |
| FX-курс stale > 3 дня | `fx_rates.fetched_at` | Sentry |

«Self-alerts» через тот же Telegram — бот может писать сам себе. Удобно для personal-сервиса без отдельной alertmanager-инфры.

### 10.2. Тестовая стратегия

**Уровни:**

1. **Unit** — чистые функции: Alert Engine стратегии, Currency Normalizer, Aggregator dedup. Быстрые (< 1 сек на сюиту), без I/O. `freezegun` для time-зависимых.
2. **Контрактные тесты адаптеров** — фикстуры с записанными ответами реальных API в `tests/fixtures/{source}/`. `respx` подставляет их вместо httpx. Тест: «парсинг ответа → ожидаемый список `Flight`». Ломается, если внешний API меняет схему.
3. **Integration тесты** — на реальных API, но только в **nightly job** в CI (не на каждый PR), чтобы не сжигать квоты. Помечены `@pytest.mark.integration`.
4. **End-to-end** — мини-сценарий: создать подписку через aiogram-тест-клиент → дёрнуть один цикл → проверить, что в БД появилось наблюдение и (опц.) алерт. Использует **временную SQLite-БД**.

**Замены:**

- Telegram API — `aiogram.test` (fake bot).
- БД — отдельный файл per-test или in-memory SQLite.
- Время — `freezegun`.
- HTTP — `respx`.

### 10.3. CI/CD

**На каждый PR:**

- `ruff check` + `ruff format --check`
- `mypy src/`
- `pytest -m "not integration"` + coverage > 75%
- Build Docker-образа (без push)

**На merge в `main`:**

- Полный pipeline выше
- Push Docker-образа в GHCR с тегами `main` и `sha-XXX`
- SSH-деплой на VPS: `docker compose pull && docker compose up -d`
- Smoke-test: HTTP `/health` отвечает 200 в течение 30 сек после рестарта.

**Nightly:**

- `pytest -m integration` с реальными API.
- Проверка свежести FX-курсов.
- Backup SQLite в S3 / отдельный том.

### 10.4. Безопасность

- **Whitelist пользователей** через middleware aiogram.
- **Секреты** — только через env / Docker secrets, никогда не в git. `.env.example` без значений.
- **`.gitignore`** покрывает `.env`, `*.sqlite`, `*.db`, локальные кэши.
- **Telegram bot token** при компрометации регенерируется через @BotFather.
- **Input validation** — Pydantic-схемы для всех команд бота (IATA-коды по регулярке, даты через `date.fromisoformat`).
- **SQL injection** — не используется raw SQL в логике, всё через SQLAlchemy ORM/Core.
- **Dependency audit** — `pip-audit` в CI, алерт на CVE.
- **Backup** — раз в сутки SQLite copy в S3 (Hetzner Storage Box ~€3/мес за 1 ТБ).

### 10.5. Структура репозитория

```
warden/
  src/warden/
    bot/              — aiogram handlers, FSM dialogs
    domain/           — Subscription, Flight, AlertStrategy (Pydantic)
    adapters/         — Aviasales, Kiwi, Ryanair, ...
      base.py         — BaseAdapter ABC
    services/         — Aggregator, AlertEngine, CurrencyNormalizer
    infrastructure/   — DB (SQLAlchemy), cache, scheduler, telemetry
    config.py         — pydantic-settings
    main.py
  tests/
    unit/
    contract/
      fixtures/{source}/
    integration/
    e2e/
  alembic/
    versions/
  docker/
    Dockerfile
    docker-compose.yml
  .github/workflows/
    ci.yml
    deploy.yml
    nightly.yml
  pyproject.toml
  .env.example
  README.md
```

Hexagonal-структура (`domain` ничего не знает про БД и httpx, `adapters`/`infrastructure` — про детали).

---

## 11. Rate limits и квоты источников (черновик)

| Источник | Quota | Rate limit | Источник данных | Заметки |
|----------|-------|------------|-----------------|---------|
| **Aviasales / Travelpayouts** | без хард-лимита | ~1 RPS рекомендовано | docs.travelpayouts.com | Подтвердить актуальные числа при регистрации |
| **Kiwi Tequila** | (deprecated?) | 0.5 RPS | tequila.kiwi.com | **Проверить статус API в 2026** |
| **Ryanair (services-api)** | неофициальный | низкая нагрузка | reverse-engineered | Риск бана при злоупотреблении |
| **Amadeus self-service** | 2000 req/мес (test) | 10 RPS | developers.amadeus.com | Production tier — платный |
| **Duffel** | до 1000 req/час test | — | duffel.com/docs | Test environment бесплатно |
| **ECB FX rates** | без лимита | вежливо: 1 запрос/сутки | www.ecb.europa.eu | XML-фид, обновляется ~16:00 CET в будни |

**Все значения — ориентировочные.** Перед каждым запуском в прод — сверка с актуальной документацией источника. Запись фактических `rate_limit_remaining` в `api_call_log` даёт реальную картину.

---

## 12. Changelog

- **0.2 — 2026-05-23.** Уточнён стек (aiogram 3.x, SQLAlchemy 2.x async, Alembic, pydantic-settings, structlog, Sentry, Prometheus). Добавлены компоненты: Cache Layer, Currency Normalizer, Observability. Уточнены: idempotent UNIQUE в `price_observations`, outlier-detection, dry-run для подписок, TZ-обработка через `zoneinfo` + `airportsdata`. Добавлены разделы: §9 Стек, §10 Эксплуатация (observability/tests/CI/CD/security/структура репо), §11 Rate limits. Расширены риски (FX, deprecation Kiwi). Закрыты open questions по деплою (Hetzner) и webhook vs polling (polling).
- **0.1 — 2026-05-21.** Первоначальный draft: архитектура, схема БД, MVP-план, риски, open questions.

---

## 13. Глоссарий

- **GDS** — Global Distribution System. Amadeus, Sabre, Travelport. Системы, через которые работают традиционные авиакомпании.
- **OTA** — Online Travel Agency. Kiwi, Trip.com, Booking-style продавцы билетов.
- **IATA-код** — трёхбуквенный код аэропорта (BEG, BCN, BUD).
- **Round-trip** — туда-обратно одним билетом.
- **Open-jaw** — туда в один город, обратно из другого.
- **Multi-city** — несколько сегментов в разных направлениях.
- **Virtual interlining** — стыковка рейсов разных авиакомпаний, которые формально не связаны. Фича Kiwi.
- **Метапоиск** — агрегатор, который не продаёт билеты сам, а перенаправляет на источник (Aviasales, Skyscanner).
- **FSM** — Finite State Machine. В aiogram — механизм для пошаговых диалогов (создание подписки через несколько вопросов).
- **FX** — foreign exchange. В контексте бота — валютные курсы для нормализации цен в EUR.
- **Circuit breaker** — паттерн отказоустойчивости: после N подряд провалов внешнего сервиса автоматически «открывает цепь» на cooldown-период, не тратя запросы впустую.
- **Token bucket** — алгоритм rate limiting: токены пополняются с заданной скоростью, каждый запрос «съедает» токен.
- **Idempotency** — свойство операции, при котором повтор не меняет результата. Реализуется через UNIQUE-индексы в БД.
- **Outlier** — статистический выброс. В контексте — подозрительная цена, не участвующая в агрегатах истории.
- **Downsampling** — снижение частоты точек временного ряда (например, час → сутки) для экономии места.
- **Dry-run** — режим, при котором операция выполняется, но не имеет побочных эффектов. Для подписок — алерт пишется в БД, но не отправляется.
- **TZ-aware datetime** — момент времени с явной timezone-привязкой, в отличие от «голого» datetime.
