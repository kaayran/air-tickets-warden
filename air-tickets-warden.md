# Air Tickets Warden — Design Document

**Version:** 0.3 (draft)
**Date:** 2026-05-23
**Status:** Design

---

## 1. Overview

A Telegram bot for personal monitoring of air ticket prices from Serbia (primary airport: Belgrade, BEG) to any European country. The bot operates on a subscription model: the user creates monitoring rules (route + date range + alert conditions), and the bot regularly polls several data sources, aggregates results, maintains a price history, and sends notifications when conditions are met.

### Key principles

- **Multi-source coverage.** No single API covers the whole market (especially because of the low-cost carriers Wizz Air and Ryanair). The bot polls several sources in parallel.
- **History matters more than the spot price.** "Cheap" is defined relative to historical data for the specific route, not against an absolute threshold.
- **Airport flexibility.** From Belgrade it is often cheaper to fly via Budapest, Sofia, Timișoara, or Zagreb — the bot accounts for alternative departure airports.
- **Anti-spam.** The bot does not fire notifications on every minor price wiggle.

### What the bot does NOT do (out of scope for MVP)

- Does not book tickets automatically (only sends links).
- Does not handle payments.
- Does not work with multi-segment itineraries split across separate tickets (open-jaw, multi-city).
- Does not factor in baggage / seat fees yet.

---

## 2. Context and assumptions

### Target user

A single person (the bot owner) or a small circle of acquaintances. Not a public service, therefore:

- No need for paid-tier authorization, billing, or multi-tenancy.
- API keys on minimal / free tiers are acceptable.
- Semi-official APIs are acceptable (e.g., Ryanair endpoints that are not formally intended for public use).

### Routes

- **Origin:** Belgrade (BEG), plus nearby alternative airports — Budapest (BUD), Sofia (SOF), Timișoara (TSR), Zagreb (ZAG).
- **Destination:** any European airport.
- **Carriers that matter:** Air Serbia, Wizz Air, Ryanair, Lufthansa Group (LH/OS/LX), Turkish, easyJet, Vueling, Pegasus, AJet.

### Technology assumptions

- **Python 3.12+** (for compatibility with `ryanair-py` and the async ecosystem).
- **Async-first**: aiogram 3.x, httpx, aiosqlite / asyncpg. A single event loop, no worker pool.
- A single bot instance, no horizontal scaling.
- **SQLite (via `aiosqlite`) for MVP**, migration to **PostgreSQL (via `asyncpg`)** when volume grows. Alembic migrations from day one.
- Deployment on a VPS — **Hetzner Cloud (~€4–5/month)** as the reference. Railway / Fly.io are possible, but free tiers are unreliable for 24/7 operation.
- Containerization — **Docker + docker-compose** from MVP; simplifies migration and local dev.

The full stack with rationale — see §9 "Technology stack".

---

## 3. Architecture

### 3.1. High-level diagram

```
┌─────────────────────────────────────────────────────────────────┐
│                      Telegram Bot Layer                          │
│  (commands, inline buttons, notification formatting)             │
└──────────────────────────┬──────────────────────────────────────┘
                           │
                           ▼
            ┌──────────────────────────┐
            │  Subscription Manager    │ ←──→  ┌──────────┐
            │  (CRUD over rules)       │       │   DB     │
            └──────────────┬───────────┘       └──────────┘
                           │
                           ▼
            ┌──────────────────────────┐
            │      Scheduler           │
            │  (cron + priorities)     │
            └──────────────┬───────────┘
                           │
                           ▼
            ┌──────────────────────────┐
            │ Multi-Airport Expander   │
            │ (route expansion)        │
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
            │ (threshold / drop / min) │
            └──────────────┬───────────┘
                           │
                           ▼
            ┌──────────────────────────┐
            │   Notification Layer     │ ──→ Telegram
            └──────────────────────────┘
```

### 3.2. Components

#### Telegram Bot Layer

The user entry point. Implemented on **`aiogram` 3.x** (async-first, built-in FSM for the step-by-step `/new` dialog, Pydantic validation of updates).

**Transport:** long-polling (simpler for a personal bot — no ingress, no TLS, identical between local dev and prod). Webhook only worth considering if traffic grows.

**User whitelist:** a startup middleware checks `chat_id` against `ALLOWED_USER_IDS` from the config. Anything else is dropped without a reply. The bot is formally public on Telegram, and without a filter random users can burn external API quotas.

**Commands:**

- `/new` — dialog to create a new subscription (origin/destination/date range/flexibility/threshold).
- `/list` — list of active subscriptions with brief status.
- `/pause <id>`, `/resume <id>`, `/delete <id>` — manage subscriptions.
- `/search <id>` — ad-hoc manual check.
- `/stats <id>` — price history for a subscription: current minimum, average, min over 30/60 days, trend.
- `/help` — help text.

**Inline buttons in notifications:**

- "View details" (expands segments, layovers, duration)
- "Buy" (deep link to the source, optionally with a referral code)
- "Mute alert for this route"
- "Lower threshold" / "Ignore for N days"

#### Subscription Manager

A CRUD layer over monitoring rules. A subscription consists of:

| Field | Description |
|-------|-------------|
| `id` | UUID |
| `origin` | IATA code of the primary airport (e.g., `BEG`) |
| `origin_alternatives` | List of alternative departure airports |
| `destination` | IATA code or list (e.g., `[BCN, MAD, VLC]` for Spain) |
| `date_from`, `date_to` | Allowed departure date range |
| `return_date_from`, `return_date_to` | Optional, for round-trip |
| `trip_length_min`, `trip_length_max` | Trip length in days (if round-trip) |
| `max_price` | Absolute price threshold (optional) |
| `max_stops` | Maximum number of layovers |
| `max_duration_minutes` | Maximum total trip duration |
| `airlines_whitelist`, `airlines_blacklist` | Carrier filters |
| `alert_strategy` | Alert strategy (see Alert Engine) |
| `cooldown_hours` | Anti-spam between notifications |
| `status` | active / paused / archived |

#### Scheduler

Runs subscription check jobs. Not a flat cron — prioritized by date proximity:

- **High-priority** (departure within 14 days) — every hour
- **Medium** (15–60 days) — every 4 hours
- **Low** (60+ days) — once a day

**Implementation:** **APScheduler 3.x** in `AsyncIOScheduler` mode. Sufficient for a single instance and does not pull in Redis. Moving to `arq` only makes sense if Redis appears for caching.

**Rate limiting:** each source gets its own **`aiolimiter.AsyncLimiter`** (token bucket) at the adapter level. The scheduler does not know about external API limits — it only triggers jobs; quota handling is delegated to adapters.

**Jittering:** a random 0–60 second offset is added when scheduling jobs, so that all subscriptions don't fire at the same minute in a burst.

**Job persistence:** SQLAlchemyJobStore over the same DB — jobs survive restart.

#### Multi-Airport Expander

Before dispatching requests to adapters, expands the route according to user flexibility:

- If a subscription allows alternative airports, it queues requests for each.
- Maintains a lookup table: for each pair (primary airport, alternative airport) — the approximate cost and duration of ground transfer (bus, car).
- At the aggregation stage, this cost is added to the ticket price for a fair comparison. For example, a ticket from Budapest at €40 + €25 transfer = an effective price of €65 vs. a €70 ticket from Belgrade — the latter is cheaper.

Transfer reference table (draft):

| From BG to | Mode | Price | Time |
|------------|------|-------|------|
| BUD | bus/car | €25–40 | ~7 h |
| SOF | bus | €20 | ~6 h |
| TSR | bus | €15 | ~2.5 h |
| ZAG | bus | €30 | ~6 h |

#### Source Adapters

Each source is a separate module with the same interface:

```
search(origin, destination, date_from, date_to, options) -> List[Flight]
```

Where `Flight` is a normalized object:

```
Flight {
  source: str            # 'aviasales' | 'kiwi' | 'ryanair'
  price: float
  currency: str
  origin: str            # IATA
  destination: str       # IATA
  departure_at: datetime # in the airport's timezone
  arrival_at: datetime
  segments: List[Segment]
  airline: str           # primary carrier code
  flight_number: str
  stops: int
  duration_minutes: int
  booking_url: str
  fetched_at: datetime
}
```

**Initial set of adapters:**

1. **Aviasales / Travelpayouts adapter** — the foundation. Free API, good coverage of Air Serbia, traditional carriers, partially Wizz Air. The affiliate program provides referral links.
2. **Kiwi (Tequila) adapter** — better for low-cost carriers, supports virtual interlining (Kiwi stitches together flights from different carriers, which traditional GDSs do not). Important for non-standard routes.
3. **Ryanair adapter** — via the `ryanair-py` library, which uses the semi-official `services-api.ryanair.com` endpoint. Covers only Ryanair, but critical when that carrier is not visible in Aviasales.

**Optional extended set:**

4. **Amadeus self-service adapter** — 2000 free requests per month. Backup and additional price validation via GDS.
5. **Wizz Air monitoring** — via site scraping or subscribing to promo mailings (no official API).

**Standard adapter implementation:**

- HTTP client — **`httpx.AsyncClient`** with a persistent connection pool.
- Retry — **`tenacity`** with exponential backoff on 429/5xx (3 attempts, jitter).
- Rate limit — **`aiolimiter`** (token bucket), parameters per source come from the config.
- **Circuit breaker** — `pybreaker` (or a custom counter): after N consecutive failures a source is "tripped" for a cooldown period. Logged to `api_call_log`, separate metric. Protects against quota burn and cycle hangs.
- **Sanity check at the adapter output:** price < `MIN_REASONABLE_PRICE` (e.g., €10) or > `MAX_REASONABLE_PRICE` (€5000) → flagged, not passed to the aggregator, written to the log. Defends against "broken €1 prices" and Wizz parsing outliers.
- Every request is logged to `api_call_log` (endpoint, status, latency, remaining quota, error).
- A single adapter failure does not crash the whole cycle — `asyncio.gather(..., return_exceptions=True)` at the Aggregator level.

**Note on Kiwi Tequila:** the API has been undergoing restructuring; free-tier access has been restricted. **Verify key availability before wiring it into code.** Alternatives: **Duffel** (good API, has a test mode), **FlightAPI.io**, **SerpAPI Google Flights** (paid, but covers Wizz/Ryanair).

#### Cache Layer

Sits between Source Adapters and Aggregator — an adapter-response cache. MVP — **`aiocache` (in-memory)**, growing into **Redis**.

**Why it's needed:** with 5 alternative airports × 3 sources × N subscriptions, the same pair (BEG→BCN, 10–20 July) is queried many times per hour. Without a cache, free-tier limits burn out within a day.

**Key:** `(source, origin, destination, date_from, date_to, options_hash)`.
**TTL:** 15 minutes for high-priority subscriptions, 60 minutes for the rest. Configurable per source.
**Invalidation:** TTL only. Forced refresh — via the `/refresh <id>` command (same as `/search`, but bypasses the cache).

A write to `price_observations` happens **always** (even on a cache hit) so that history has no cache-induced gaps. But `api_call_log` is appended only on a real HTTP request.

#### Currency Normalizer

Adapters can return prices in different currencies: Aviasales — depending on the `currency` parameter (RUB/USD/EUR), Kiwi — in EUR, Ryanair — in the route's local currency (EUR/GBP/RON/...).

All prices in the system are converted to a **base currency (EUR)** before being written to the Price History Store and compared in the Alert Engine. Otherwise `$87 < €100` → false alert.

**Source of rates:** **ECB daily reference rates** (`https://www.ecb.europa.eu/stats/eurofxref/eurofxref-daily.xml`). Free, updated around 16:00 CET on business days.
**Rate cache:** the `fx_rates(date, currency, rate_to_eur)` table, refreshed once per day.
**Fallback:** if the ECB is unreachable, the last known rate from the DB is used (tagged `stale_rate=true` in logs).

In each `price_observation` we store **both prices** — the original (`price`, `currency`) and the normalized one (`price_eur`). This is needed for user-facing display and for auditing.

#### Aggregator / Deduplication

Collects results from all adapters, deduplicates, and sorts.

**Dedup key:** `(airline, flight_number, departure_at_date)`. The same physical flight may come back from 3 adapters at different prices — we keep the minimum, but preserve all sources in metadata (for debugging and for displaying "available on: Aviasales, Kiwi").

**Special case — multi-segment flights.** Deduplication uses a composite key across all segments. If any segment differs, these are distinct itineraries.

**Sorting:** by effective price (`price_eur + transfer_cost_eur`), not by raw ticket price. This way Budapest with transfer is compared to Belgrade on a level playing field.

**Aggregator pipeline:**

1. Collect results from all adapters (`asyncio.gather(return_exceptions=True)`).
2. Currency Normalizer: each `Flight.price` → `Flight.price_eur`.
3. Sanity check (second line — after the adapter): drop flights with `price_eur < €10` or `> €5000` (logged with `outlier=true`).
4. Add transfer cost for alternative airports.
5. Deduplicate by `(airline, flight_number, departure_date)`.
6. Sort by effective price.

The pipeline is idempotent — recomputing on the same Flight set yields the same result.

#### Price History Store

Storage for the price time series. Minimal schema:

```
price_observations (
  id              int pk,
  route_key       str,        -- 'BEG-BCN-2026-07-15'
  subscription_id uuid,       -- nullable, tracks which subscription's query produced it
  price           float,      -- original price
  currency        str,        -- original currency
  price_eur       float,      -- normalized to EUR
  source          str,
  flight_signature str,       -- airline+flight_number, for flight identification
  departure_at    timestamp,  -- TZ-aware, departure airport
  observed_at     timestamp,  -- UTC
  outlier         bool default false,
  raw_payload     json
)

-- Idempotency: the same flight from the same source within one hour
-- must not enter the DB twice (defense against retries).
UNIQUE INDEX idx_obs_dedup ON price_observations (
  flight_signature, departure_at, source, date_trunc('hour', observed_at)
);

-- Indexes for fast aggregates in the Alert Engine
INDEX idx_obs_route_time ON price_observations (route_key, observed_at DESC);
INDEX idx_obs_route_signature ON price_observations (route_key, flight_signature, observed_at DESC);
```

**Retention policy:**

- Near-term dates (< 30 days to departure): 1 point per hour
- Medium-term (30–90): 1 point per day
- Long-term (> 90): 1 point per day + dedup by day
- 14 days past departure: deletion

Implemented via a periodic job (`apscheduler`, once per day): downsampling + deletion of expired entries.

**Outlier handling:** at write time, a record is flagged `outlier=true` if its price is more than 3× below the `route_key` median over the last 30 days (provided a sample of at least 20 points exists). Such records **do not participate** in Alert Engine aggregates but stay in the DB for auditing.

**When moving to PostgreSQL** — consider the **TimescaleDB** extension for `price_observations`. Hypertables + automatic downsampling out of the box. Not for MVP, though.

From this data the Alert Engine computes aggregates: moving average, median, minimum over N days.

#### Alert Engine

Decides whether to send a notification. Supports several strategies, selected per subscription:

| Strategy | Logic |
|----------|-------|
| `absolute_threshold` | Price ≤ `max_price` |
| `relative_drop` | Price ≤ 30-day average × (1 − `drop_pct`) |
| `historical_minimum` | New minimum over the last N days (e.g., 60) |
| `sudden_drop` | Price dropped by ≥ X% compared to the previous point |
| `combined` | Any of the above triggers (OR) |

**Anti-spam:**

- Cooldown between alerts per subscription (default 6 hours).
- Deduplication: if the same price for the same flight has already been sent — no alert.
- "Stable price" guard: if the price oscillates within ±2% of an already-alerted value — do not repeat.

**Decision logging:** every trigger / non-trigger writes a record with input data and the outcome. This is needed for debugging ("why didn't an alert come?").

**Dry-run mode.** A subscription-level flag — `dry_run: bool`. In this mode the Alert Engine runs the whole pipeline but **does not send** to Telegram; it only writes to `alerts_sent` with `dry_run=true`. Used for:

- Tuning new strategies against historical data (CLI replay: `python -m warden.replay --subscription <id> --strategy ...`).
- Silent testing before enabling a new route.

**Time zones (important):**

- Everything in the DB is in UTC (`TIMESTAMP WITH TIME ZONE` on Postgres, ISO-8601 strings with explicit `+00:00` on SQLite).
- `departure_at` / `arrival_at` are TZ-aware in the airport's zone.
- Airport TZ resolution — via the **`airportsdata`** package (offline data, no external requests).
- For Telegram display — convert to the departure airport's TZ via `zoneinfo`.
- All date comparisons in the Alert Engine are in UTC; formatting happens in the local TZ immediately before sending.

#### Notification Layer

Formats and sends notifications to Telegram. Message structure:

```
✈️ Cheap ticket found!

BEG → BCN
🗓 15 July 2026, 09:30 → 12:45
💰 €87 (Wizz Air, direct, 3h 15m)

📊 That's −34% off the 30-day average (€132)
📊 New minimum over the last 60 days
📊 Available on: Aviasales, Kiwi

[Buy] [Details] [Mute alert]
```

If flying from an alternative airport is cheaper, a separate note:

```
💡 Cheaper from Budapest (BUD):
   €52 + €25 transfer = €77 effective
```

#### Observability

The bot runs 24/7 unattended — without observability, a downed adapter or a burnt-out quota gets noticed a week later by the silence of alerts.

**Logs:** **`structlog`** in JSON. Each record carries `subscription_id`, `source`, `route_key`, `trace_id` (a UUID for one subscription's check cycle). Log level from config.

**Metrics:** **`prometheus_client`** in pull mode (endpoint `/metrics` on a separate port). Minimal set:

- `warden_adapter_requests_total{source, status}` — counter
- `warden_adapter_latency_seconds{source}` — histogram
- `warden_adapter_quota_remaining{source}` — gauge
- `warden_alerts_sent_total{strategy}` — counter
- `warden_alerts_suppressed_total{reason}` — counter (cooldown / dedup / stable_price)
- `warden_subscriptions_active` — gauge
- `warden_db_size_bytes` — gauge

Scraping can be done from a sidecar docker service (Grafana Cloud free tier supports scraping by public URL via the agent).

**Error tracking:** **Sentry** (`sentry-sdk` with integrations for httpx, aiogram, SQLAlchemy). DSN from env. The free tier (5K errors/month) covers a personal project with room to spare.

**Health endpoint:** `/health` returns 200 if the bot is alive and the DB is reachable, otherwise 503. Used for uptime monitoring (UptimeRobot free tier).

**The `/health` Telegram command** — outputs current state: aliveness of each adapter (from `api_call_log` over the last hour), DB size, number of active subscriptions, last successful check cycle.

#### Config & Secrets

Config goes through **`pydantic-settings`**: type-safe, validated at startup, source = env variables + `.env` file locally, secrets via env in prod.

Grouped by domain:

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

**Secrets in prod:** a **`.env` file mounted into the container** or **Docker secrets**. Not committed; `.env.example` lives in the repo as a reference.

**Startup validation:** every required field is validated by Pydantic; on failure the bot crashes with a clear message. The alternative — "it will explode somewhere deep later" — is unacceptable for a 24/7 service.

---

## 4. Data flow (end-to-end scenario)

**Scenario:** the user created a subscription BEG → Barcelona (BCN), dates 10–20 July 2026, departure-airport flexibility enabled, `combined` strategy (threshold €100 OR −25% off the average).

1. The Scheduler triggers a subscription check (departure ~50 days away → medium-priority, every 4 hours). A `trace_id` is assigned for end-to-end logging of the whole cycle.
2. The Subscription Manager hands over the rule → passed to the Multi-Airport Expander.
3. The Expander unfolds into 5 pairs: (BEG, BCN), (BUD, BCN), (SOF, BCN), (TSR, BCN), (ZAG, BCN). For each pair + date range, requests are formed.
4. **Cache Layer** checks each key `(source, origin, destination, date_from, date_to)`. On a hit, the cached Flight list is returned. On a miss, the request continues.
5. Miss requests are fired in parallel into the Aviasales, Kiwi, Ryanair adapters. Each adapter:
   - Respects its own rate limit (`aiolimiter`).
   - Applies retry/backoff (`tenacity`).
   - Checks the circuit breaker — if the adapter is "tripped", the request is skipped.
   - Performs a sanity check on the response.
   - Returns a normalized Flight list.
   - Writes the result back to the Cache Layer.
6. **Currency Normalizer** adds `price_eur` to each Flight using the current rate from `fx_rates`.
7. The Aggregator merges results, deduplicates. For alternative airports, transfer cost is added to `price_eur` → `effective_price_eur`.
8. Every observation is written to the Price History Store (with outlier check).
9. The Alert Engine checks the subscription's strategy conditions for every Flight:
   - Price ≤ €100? → checks.
   - Price ≤ 30-day average × 0.75? → fetches the average from history (excluding `outlier=true`), checks.
   - If any matches — alert candidate.
10. Candidates pass through anti-spam (cooldown, dedup against already sent, stable-price guard).
11. If the subscription is in `dry_run`, a row is written to `alerts_sent` with the flag, without sending to Telegram.
12. Otherwise, the Notification Layer formats and sends to Telegram. The `message_id` is stored for possible later edits.
13. Cycle metrics (latency, Flights found, alerts generated) are updated in Prometheus.

---

## 5. Data model (DB schema)

Migrations use **Alembic** from the very first revision. SQLite-compatible syntax on MVP — the path to Postgres requires no DDL rewrites.

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

scheduler_runs                 -- for /health and metrics
  id (bigint pk),
  subscription_id (uuid fk),
  started_at, finished_at,
  trace_id (uuid),
  flights_found (int),
  alerts_generated (int),
  status (success/partial/failed),
  error (str nullable)
```

**Foreign keys:** `subscription_id` in `price_observations` — `ON DELETE SET NULL` (history outlives subscription deletion); in `alerts_sent` — `ON DELETE CASCADE` (alerts are meaningless without a subscription).

**JSON fields** on SQLite work via the `JSON1` extension; on Postgres — native `jsonb`. SQLAlchemy 2.x abstracts the difference.

---

## 6. Implementation roadmap (MVP → Full)

### MVP (1–2 weeks)

Goal: a working bot for a single route, one source, manual alerts. **Already with the baseline infrastructure** so it doesn't need to be redone later.

**Functionality:**

- Telegram Bot Layer (aiogram 3.x): commands `/new`, `/list`, `/delete`, user whitelist.
- Subscription Manager on SQLite, without alternative airports.
- One adapter: **Aviasales / Travelpayouts**.
- Scheduler (APScheduler async): one shared cron, hourly check.
- Currency Normalizer with ECB rates.
- Price History Store: write + simple "minimum over N days" query.
- Alert Engine: `absolute_threshold` and `historical_minimum`, cooldown.
- Notification Layer: text notifications without inline buttons.

**Infrastructure (from day one):**

- Alembic migrations.
- pydantic-settings for config.
- structlog + Sentry.
- Docker + docker-compose.
- `pytest` with baseline adapter tests (fixtures via `respx`).
- GitHub Actions: lint (ruff) + types (mypy) + tests.

### v1.0 (next 1–2 weeks)

- Adding the Kiwi adapter and Ryanair adapter.
- Aggregator with deduplication and sanity check.
- Cache Layer (in-memory).
- Multi-Airport Expander with the transfer reference table.
- Circuit breaker for adapters.
- Alert Engine: `relative_drop`, `combined`, full anti-spam (cooldown + dedup + stable-price).
- Inline buttons in notifications.
- `/stats` command with subscription statistics.
- `/health` command with adapter aliveness.
- Prometheus metrics on `/metrics`.
- Dry-run mode for subscriptions.
- SQLite backup to a separate volume once a day.

### v1.1+ (as needed)

- Migrating SQLite → PostgreSQL (optionally TimescaleDB for price_observations).
- Redis for the Cache Layer (instead of in-memory).
- Amadeus adapter / Duffel adapter as a backup.
- Price history charts — matplotlib → PNG → `send_photo` to Telegram (via `/stats`).
- Trend Analyzer — a weekly digest across subscriptions.
- Calendar Heatmap as an image.
- Smart suggestions ("where to fly cheaply from BG this weekend").
- Round-trip support with two independent tickets on different airlines (Kiwi-style virtual interlining) — requires reworking the Flight model into an Itinerary.
- Wizz Air monitoring via a headless browser (Playwright in a separate container).

---

## 7. Risks and mitigations

| Risk | Mitigation |
|------|------------|
| Ryanair adapter ban / block (unofficial API) | Graceful fallback, circuit breaker, do not crash the cycle. Availability monitoring via `/health`. |
| Free API limits exhausted | Cache Layer (15–60 min), scheduler jittering, prioritization by departure proximity, per-source `aiolimiter`. The `warden_adapter_quota_remaining` metric. |
| API response schema changes | Pydantic validation at the adapter output, contract tests with fixtures (`respx` + recorded responses), `raw_payload` logged in the DB. Sentry alert on a spike in parsing errors. |
| DB bloat | Retention policy with downsampling and deletion of old observations (see Price History Store). The `warden_db_size_bytes` metric. |
| Notification spam | Cooldown, dedup, stable-price guard (±2%). "Mute alert" button. |
| False triggers (broken €1 price, price in cents) | Two-layer sanity check: inside the adapter (absolute thresholds) + outlier flag in the DB (relative to the route median). Outliers excluded from aggregates. |
| Time zones | UTC in the DB, TZ-aware datetimes via `zoneinfo`. Airport resolution via `airportsdata` offline. Prices in EUR after normalization. Covered with `freezegun` tests. |
| **FX rate swings / ECB unreachable** | Rate cache in the DB, fallback to last known, `stale_rate=true` log. Sentry alert if rates aren't refreshed for > 3 days. |
| **External API deprecation (Kiwi Tequila move)** | Isolation via the `BaseAdapter` interface. Replacement plan — Duffel/FlightAPI/SerpAPI. Contract tests catch breaking changes. |
| **Event loop stalls on a sync operation** | All I/O is async, CPU-heavy parsing offloaded via `asyncio.to_thread`. The cycle-latency metric helps spot degradation. |
| **Data loss on restart** | SQLAlchemyJobStore for APScheduler (jobs survive restart). Daily DB backup. Idempotent writes in `price_observations`. |
| **Whitelist bypass or token leak** | All secrets via env / Docker secrets, never in git. ALLOWED_USER_IDS as the first line. Logging of dropped updates for auditing. |

---

## 8. Open questions

**Resolved in v0.2:**

- ~~Where to deploy?~~ → Hetzner Cloud (~€4–5/month). Free-tier Railway/Fly.io is unreliable for 24/7.
- ~~Webhook or long-polling?~~ → Long-polling for MVP.
- ~~SQLite or Postgres from the start?~~ → SQLite on MVP, the path to Postgres is clear (Alembic, SQLAlchemy 2.x async, `aiosqlite` → `asyncpg` by driver swap).

**Still open:**

- **Kiwi Tequila status in 2026.** Needs verification: is the free tier still around, are new keys being issued? Plan B — Duffel (has a test mode).
- **Round-trip with virtual interlining.** Requires reworking `Flight` → `Itinerary` (a list of Flights treated as one ticket). Deferred to v1.1+; for now round-trip is treated as a pair of independent one-ways.
- **Ground transport.** A static lookup vs the Omio API. Reassess after v1.0: if the static lookup is often wrong (user complaints), migrate.
- **Wizz Discount Club.** A headless Playwright in a separate container (runs once a day) is a workable option but requires maintenance. The alternative — email forwarding via IMAP — is fragile to template changes. Decide in v1.1+.
- **Who actually needs price history charts.** If only the owner — `matplotlib` → PNG is enough. If a wider circle is planned — a small FastAPI + Chart.js web frontend. Decide based on real usage.
- **Telegram premium features.** The bot can send interactive charts via WebApp. Useful for close subscription analysis, but requires an HTTPS endpoint — raises the entry barrier. Deferred.

---

## 9. Technology stack

Every choice comes with rationale. Alternatives are listed where they were actually considered.

### Language and runtime

- **Python 3.12+** — `ryanair-py` uses modern typing, and Pydantic v2 / SQLAlchemy 2.x run more efficiently on 3.12.
- Asyncio as the primary concurrency model. Threads — only for CPU-heavy parsing via `asyncio.to_thread`.

### Telegram bot

- **`aiogram` 3.x** — async-first, FSM out of the box (needed for the step-by-step `/new`), Pydantic update validation, active development.
- The `python-telegram-bot` alternative — rejected: heavier, more "classical" API.

### HTTP clients and resilience

- **`httpx.AsyncClient`** — the async HTTP standard. Supports HTTP/2, connection pool, convenient injection in tests via `respx`.
- **`tenacity`** — retry with exponential backoff and jitter. Decorator-based, reads better than custom loops.
- **`aiolimiter`** — token bucket for per-source rate limiting.
- **`pybreaker`** — circuit breaker. The alternative — a custom counter in Redis/DB, but `pybreaker` is ready-made.

### Scheduler

- **`APScheduler` 3.x (AsyncIOScheduler)** — sufficient for a single instance, survives restart via `SQLAlchemyJobStore`.
- The `arq` alternative — rejected for MVP (pulls in Redis). Reconsidered if Redis is needed anyway for caching.

### Database and ORM

- **SQLAlchemy 2.x in async mode** — modern API (`AsyncSession`, `select()`-style), well typed.
- **`aiosqlite`** (MVP) → **`asyncpg`** (Postgres). The driver changes in the connection string without code changes.
- **Alembic** — migrations from day one, otherwise the SQLite → Postgres move is painful.
- **Pydantic v2** — for normalized domain models (`Flight`, `Segment`, `AlertParams`).
- Alternatives: SQLModel (Pydantic+SQLAlchemy, simpler) — fine if a single source of schema is preferred. Tortoise ORM — rejected: less mature, weaker migration story.

### Cache

- **MVP:** `aiocache` (in-memory backend) — no external dependencies.
- **v1.1+:** Redis (`redis.asyncio`) if multiple processes appear or the cache needs to be shared with the replay CLI.

### Config

- **`pydantic-settings`** — typed config with startup validation. `.env` locally, env vars / Docker secrets in prod.

### Logging, metrics, errors

- **`structlog`** — structured JSON logs.
- **`prometheus-client`** — pull metrics on a separate port.
- **`sentry-sdk`** — error tracking, free tier.

### Time and FX

- **`zoneinfo`** (stdlib) for timezone operations.
- **`airportsdata`** — offline TZ resolution by IATA.
- **ECB daily reference rates** — FX rate source, cached in the DB.

### Testing

- **`pytest` + `pytest-asyncio`** — the foundation.
- **`respx`** — mocking httpx requests, fixtures with recorded adapter responses.
- **`freezegun`** — frozen time in Alert Engine tests.
- **`coverage.py`** — 80% target for domain logic (adapters, aggregator, alert engine); for I/O wrappers — best effort.

### Lint and types

- **`ruff`** — linter + formatter (replaces flake8/black/isort).
- **`mypy --strict`** for domain code. Looser for adapters because of external data.

### Containerization and deployment

- **Docker + docker-compose** from MVP.
- Base image — `python:3.12-slim`.
- Deployment — **Hetzner Cloud** (~€4–5/month). Docker Compose, `restart: always`. A simple watchtower for auto-updates from the registry (optional).
- CI/CD — **GitHub Actions**: lint → tests → docker build → push to GHCR → ssh-deploy (via `appleboy/ssh-action` or watchtower).

### Summary table

| Layer | MVP | v1.1+ |
|-------|-----|-------|
| Bot framework | aiogram 3.x | aiogram 3.x |
| HTTP | httpx + tenacity + aiolimiter | + pybreaker |
| Scheduler | APScheduler async | APScheduler / arq |
| DB | SQLite + aiosqlite | PostgreSQL + asyncpg (opt. TimescaleDB) |
| ORM | SQLAlchemy 2.x async + Alembic | same |
| Cache | aiocache (in-memory) | Redis |
| Config | pydantic-settings | same |
| Logs | structlog (JSON) | structlog + Grafana Loki |
| Metrics | prometheus-client | + Grafana Cloud |
| Errors | Sentry | Sentry |
| Tests | pytest + respx + freezegun | + integration against real APIs in nightly |
| Deploy | Docker Compose / Hetzner | same |

---

## 10. Operations and quality

### 10.1. Observability — what and why to monitor

| Signal | Source | Alert |
|--------|--------|-------|
| Bot alive | `/health` 200/503 | UptimeRobot — alert after 2 missed checks |
| Adapter "down" | Circuit breaker open + Sentry | Sentry email |
| Source quota < 20% | `warden_adapter_quota_remaining` | Telegram message to the owner from the bot itself |
| Check cycle > 5 min | `scheduler_runs.finished_at - started_at` | Sentry event |
| 0 alerts in 48 hours on an active subscription | `alerts_sent` aggregate | Telegram "suspicious silence" message |
| DB size > 1 GB | `warden_db_size_bytes` | Telegram message, check retention |
| FX rate stale > 3 days | `fx_rates.fetched_at` | Sentry |

"Self-alerts" via the same Telegram chat — the bot can DM itself. Convenient for a personal service without a separate alertmanager infrastructure.

### 10.2. Testing strategy

**Levels:**

1. **Unit** — pure functions: Alert Engine strategies, Currency Normalizer, Aggregator dedup. Fast (< 1 s per suite), no I/O. `freezegun` for time-sensitive tests.
2. **Adapter contract tests** — fixtures with recorded real API responses in `tests/fixtures/{source}/`. `respx` swaps them in for httpx. Test: "parse response → expected Flight list". Breaks when the external API changes its schema.
3. **Integration tests** — against real APIs, but only in a **nightly CI job** (not per PR) to avoid burning quotas. Marked `@pytest.mark.integration`.
4. **End-to-end** — a mini scenario: create a subscription via an aiogram test client → run one cycle → assert that an observation (and optionally an alert) appeared in the DB. Uses a **temporary SQLite DB**.

**Doubles:**

- Telegram API — `aiogram.test` (fake bot).
- DB — per-test file or in-memory SQLite.
- Time — `freezegun`.
- HTTP — `respx`.

### 10.3. CI/CD

**On every PR:**

- `ruff check` + `ruff format --check`
- `mypy src/`
- `pytest -m "not integration"` + coverage > 75%
- Docker image build (without push)

**On merge to `main`:**

- The full pipeline above
- Push the Docker image to GHCR with tags `main` and `sha-XXX`
- SSH deploy to the VPS: `docker compose pull && docker compose up -d`
- Smoke test: HTTP `/health` returns 200 within 30 s after restart.

**Nightly:**

- `pytest -m integration` against real APIs.
- FX-rate freshness check.
- SQLite backup to S3 / a separate volume.

### 10.4. Security

- **User whitelist** via aiogram middleware.
- **Secrets** — only via env / Docker secrets, never in git. `.env.example` without values.
- **`.gitignore`** covers `.env`, `*.sqlite`, `*.db`, local caches.
- **Telegram bot token** is rotated via @BotFather if compromised.
- **Input validation** — Pydantic schemas for every bot command (IATA codes by regex, dates via `date.fromisoformat`).
- **SQL injection** — no raw SQL in business logic, everything goes through SQLAlchemy ORM/Core.
- **Dependency audit** — `pip-audit` in CI, alert on CVEs.
- **Backup** — once-a-day SQLite copy to S3 (Hetzner Storage Box ~€3/month for 1 TB).

### 10.5. Repository layout

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

A hexagonal layout (`domain` knows nothing about the DB or httpx; `adapters` / `infrastructure` carry the implementation details).

---

## 11. Source rate limits and quotas (draft)

| Source | Quota | Rate limit | Documentation | Notes |
|--------|-------|------------|---------------|-------|
| **Aviasales / Travelpayouts** | no hard limit | ~1 RPS recommended | docs.travelpayouts.com | Confirm current numbers at signup |
| **Kiwi Tequila** | (deprecated?) | 0.5 RPS | tequila.kiwi.com | **Verify API status in 2026** |
| **Ryanair (services-api)** | unofficial | low load | reverse-engineered | Ban risk on abuse |
| **Amadeus self-service** | 2000 req/month (test) | 10 RPS | developers.amadeus.com | Production tier is paid |
| **Duffel** | up to 1000 req/hour test | — | duffel.com/docs | Test environment is free |
| **ECB FX rates** | unlimited | politely: 1 request/day | www.ecb.europa.eu | XML feed, refreshed around 16:00 CET on business days |

**All values are approximate.** Before each production launch — cross-check with the source's current documentation. Recording the actual `rate_limit_remaining` into `api_call_log` gives the real picture.

---

## 12. Changelog

- **0.3 — 2026-05-23.** Translated the document from Russian to English. No content changes.
- **0.2 — 2026-05-23.** Clarified the stack (aiogram 3.x, SQLAlchemy 2.x async, Alembic, pydantic-settings, structlog, Sentry, Prometheus). Added components: Cache Layer, Currency Normalizer, Observability. Specified: idempotent UNIQUE on `price_observations`, outlier detection, dry-run for subscriptions, TZ handling via `zoneinfo` + `airportsdata`. Added sections: §9 Stack, §10 Operations (observability/tests/CI/CD/security/repo layout), §11 Rate limits. Risks expanded (FX, Kiwi deprecation). Open questions on deployment (Hetzner) and webhook vs polling (polling) closed.
- **0.1 — 2026-05-21.** Initial draft: architecture, DB schema, MVP plan, risks, open questions.

---

## 13. Glossary

- **GDS** — Global Distribution System. Amadeus, Sabre, Travelport. The systems through which traditional airlines operate.
- **OTA** — Online Travel Agency. Kiwi, Trip.com, Booking-style ticket sellers.
- **IATA code** — the three-letter airport code (BEG, BCN, BUD).
- **Round-trip** — there-and-back on a single ticket.
- **Open-jaw** — out into one city, back from another.
- **Multi-city** — several segments in different directions.
- **Virtual interlining** — stitching together flights on airlines that are not formally interlined. A Kiwi feature.
- **Metasearch** — an aggregator that does not sell tickets itself but redirects to the source (Aviasales, Skyscanner).
- **FSM** — Finite State Machine. In aiogram — the mechanism for step-by-step dialogs (creating a subscription across several questions).
- **FX** — foreign exchange. In the bot's context — currency rates for normalizing prices to EUR.
- **Circuit breaker** — a resilience pattern: after N consecutive failures of an external service, it automatically "opens the circuit" for a cooldown period, sparing requests.
- **Token bucket** — a rate-limiting algorithm: tokens refill at a set rate, each request consumes one.
- **Idempotency** — the property of an operation where repeating it doesn't change the result. Implemented via UNIQUE indexes in the DB.
- **Outlier** — a statistical anomaly. In context — a suspicious price that does not participate in history aggregates.
- **Downsampling** — reducing the sampling frequency of a time series (e.g., hourly → daily) to save storage.
- **Dry-run** — a mode in which an operation runs but has no side effects. For subscriptions — an alert is written to the DB but not sent.
- **TZ-aware datetime** — a point in time with an explicit timezone, as opposed to a "naive" datetime.
