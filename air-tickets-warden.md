# Air Tickets Warden — Design Document

**Version:** 0.7 (live-search sources, alert hardening)
**Date:** 2026-07-22
**Status:** Design

---

## 1. Overview

A Telegram bot for personal monitoring of air ticket prices on any route. The bot operates on a subscription model: the user creates monitoring rules (route + date range + alert conditions), and the bot regularly polls several data sources, aggregates results, maintains a price history, and sends notifications when conditions are met.

The service is written in **Go**: a single static binary, goroutine-based concurrency, and a small, explicit dependency set.

The user-facing UI is a **Telegram Mini App** — a React SPA opened inside Telegram, served over HTTPS by the same Go binary (embedded via `go:embed`). All management (subscriptions, stats, search) happens there. The **bot** is reduced to two roles: the entry point (`/start` + "Open App" button) and the **notification channel** — a Mini App cannot push anything while closed, so alerts are always delivered as bot messages.

### Key principles

- **Multi-source coverage.** No single API covers the whole market (especially because of the low-cost carriers Wizz Air and Ryanair). The bot polls several sources in parallel.
- **History matters more than the spot price.** "Cheap" is defined relative to historical data for the specific route, not against an absolute threshold.
- **Airport flexibility.** It is often cheaper to fly from a nearby airport — the bot accounts for alternative departure airports and adds ground transfer cost for a fair comparison.
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

- **Any airport in the world**, both origin and destination — nothing in the system is hardcoded to a region; Belgrade (BEG) appears below only as a running example. Alternative departure airports are configurable per subscription.
- **Carriers that matter:** Air Serbia, Wizz Air, Ryanair, Lufthansa Group (LH/OS/LX), Turkish, easyJet, Vueling, Pegasus, AJet.
- **Known coverage gap (accepted for v1.0):** Wizz Air has no API and no GDS presence — it is not covered by any v1.0 source. The UI states this honestly (see Mini App Frontend); a scraper is the top v1.1+ backlog candidate.

### Technology assumptions

- **Go 1.24+**. A single static binary; goroutines + channels as the concurrency model, `context.Context` cancellation everywhere.
- A single bot instance, no horizontal scaling.
- **PostgreSQL 16 from day one** (via `pgx` v5). Running Postgres in Docker Compose costs nothing operationally, and skipping the SQLite stage removes a whole migration project later. Migrations with `goose` from the first revision.
- **Telegram Mini App as the only management UI.** Requires a public HTTPS URL (Telegram opens Mini Apps only over HTTPS) — TLS is terminated by a Caddy sidecar in docker-compose (automatic Let's Encrypt). For local development, a tunnel (cloudflared / ngrok) exposes the dev server to Telegram. The exact domain is an open question (§8).
- Frontend: **React 19 + TypeScript + Vite**, built to static assets and embedded into the Go binary with `go:embed` — one deploy artifact, no CORS, no separate frontend pipeline.
- Deployment on a VPS — **Hetzner Cloud (~€4–5/month)** as the reference.
- Containerization — **Docker + docker-compose** from MVP; the app image is a multi-stage build (node builder → go builder → distroless) with a single binary.

The full stack with rationale — see §9 "Technology stack".

---

## 3. Architecture

### 3.1. High-level diagram

```
┌───────────────────────────────┐   ┌─────────────────────────────┐
│    Telegram Mini App (React)   │   │  Telegram Bot               │
│  subscriptions CRUD, stats,    │   │  entry point ("Open App"),  │
│  search, settings              │   │  alert delivery, inline     │
│  (static files ← go:embed)     │   │  buttons on notifications   │
└───────────────┬───────────────┘   └──────────────┬──────────────┘
                │ JSON API (initData auth)          │
                ▼                                   │
┌───────────────────────────────────────────────────┴─────────────┐
│                        HTTP API Layer                            │
│      (REST /api/v1, initData validation, whitelist)              │
└──────────────────────────┬──────────────────────────────────────┘
                           │
                           ▼
            ┌──────────────────────────┐
            │  Subscription Manager    │ ←──→  ┌──────────┐
            │  (CRUD over rules)       │       │ Postgres │
            └──────────────┬───────────┘       └──────────┘
                           │
                           ▼
            ┌──────────────────────────┐
            │      Scheduler           │
            │  (ticker + priorities)   │
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
│   Amadeus    │  │   Ryanair    │  │ Kiwi/Duffel  │
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

All components live in one process. Concurrency boundaries: the bot's update loop, the HTTP server (API + static files), the scheduler loop, and per-check-cycle fan-out to adapters (an `errgroup` per cycle).

### 3.2. Components

#### Mini App Frontend

The management UI. A **React 19 + TypeScript** SPA built with **Vite**, opened inside Telegram as a Mini App (attachment-menu button / `/start` button / direct `t.me/<bot>/<app>` link).

- **`@telegram-apps/sdk-react`** — typed bindings to the Mini App platform: `initData`, theme params, viewport, BackButton, MainButton, haptic feedback.
- **`@telegram-apps/telegram-ui`** — Telegram-native UI kit (cells, lists, modals, tabbar); the app inherits the user's Telegram theme (light/dark, accent colors) automatically.
- **TanStack Query** for API state (fetch/cache/invalidate); no global state manager beyond it — the app is CRUD-shaped.
- Screens:
  - **Subscriptions** — list with status badges: active / paused / 🔇 muted until … / 📈 collecting price history (warm-up, see Alert Engine); swipe/menu actions (pause, resume, mute, delete).
  - **New/Edit subscription** — a real form at last: airport autocomplete (backed by `GET /api/v1/airports?q=`), native-feeling date-range picker, sliders/steppers for thresholds, toggles for alternative airports. Everything the old inline-keyboard FSM emulated poorly, done as normal web UI.
  - **Subscription detail / stats** — price history chart (lightweight charting lib, e.g. `recharts`), min/avg/trend, recent alerts, "check now" button (`POST .../check` → `202 {run_id}`, then the app polls `GET .../runs/{run_id}` every ~2 s until `done` and renders the results). A **source-coverage line** — "sources: Amadeus, Ryanair · not covered: Wizz Air" — computed from the adapter registry, so the user always sees the monitoring boundary.
  - **Settings** — per-user defaults (cooldown, drop %) backed by the `user_settings` table; mute overview.
- Built assets land in `web/dist/` and are **embedded into the Go binary via `go:embed`** — the same server serves `/` (SPA) and `/api/v1` (JSON). One artifact, no CORS, no separate frontend deploy.

#### HTTP API Layer

A JSON REST API under `/api/v1`, served by `net/http` (Go 1.22+ pattern routing — no router dependency) in the same process.

**Authentication — Telegram `initData`:** every request carries the raw `initData` string (an `Authorization: tma <initData>` header) that Telegram injects into the Mini App. The server validates its HMAC-SHA256 signature against the bot token (the documented Mini App validation scheme, via `init-data-golang`), checks `auth_date` freshness (≤ 24 h), and extracts the authenticated `user.id`. No sessions, no passwords — Telegram is the identity provider.

**Authorization:** the extracted user id must be in `ALLOWED_USER_IDS` — the same whitelist as the bot, enforced as API middleware. Every handler scopes queries by the authenticated `user_chat_id`; object ownership is checked on each mutation.

**Endpoints (v1):**

```
GET    /api/v1/me                        — profile + per-user defaults (user_settings)
PATCH  /api/v1/me                        — update per-user defaults
GET    /api/v1/subscriptions             — list (with brief status)
POST   /api/v1/subscriptions             — create
GET    /api/v1/subscriptions/{id}        — detail (incl. source-coverage info)
PATCH  /api/v1/subscriptions/{id}        — edit / pause / resume / mute (muted_until) / dry-run
DELETE /api/v1/subscriptions/{id}        — delete
POST   /api/v1/subscriptions/{id}/check  — ad-hoc check → 202 {run_id}
GET    /api/v1/subscriptions/{id}/runs/{run_id} — check-run status + results (polled by the app;
                                           backed by scheduler_runs)
GET    /api/v1/subscriptions/{id}/stats  — history aggregates + chart series
GET    /api/v1/subscriptions/{id}/alerts — recent alerts
GET    /api/v1/airports?q=<text>         — autocomplete over the embedded dataset
GET    /api/v1/health-summary            — adapter aliveness, last cycle (the old /health command)
```

An ad-hoc check runs through **the same code path** as a scheduled cycle (recorded in `scheduler_runs` with `triggered_by = 'manual'`) — no divergence between "check now" and the scheduler.

Input validation server-side as before: IATA codes must exist in the airports dataset, dates parsed with `time.Parse`, ranges checked in the domain layer. The Mini App is a convenience, not a trust boundary.

#### Telegram Bot Layer

Reduced to **entry point + notification channel**. Implemented on **`go-telegram/bot`** (zero-dependency, actively maintained, full Bot API coverage, middleware). No FSM, no dialog flows — all input UX lives in the Mini App.

**Transport:** long-polling for updates (simpler for a personal bot; the HTTPS ingress required by the Mini App serves the API/static only — bot updates don't need a webhook).

**User whitelist:** a middleware checks `chat_id` against `ALLOWED_USER_IDS`. Anything else is dropped without a reply.

**Commands (complete list):**

- `/start` — greeting + a `web_app` keyboard button "Open App"; the bot's menu button is also configured (via BotFather / `setChatMenuButton`) to open the Mini App.
- `/help` — one screen of text pointing to the Mini App.

**Notifications:** the bot's main job. Alert messages with inline buttons:

- "Buy" (deep link to the source, optionally with a referral code)
- "Details" / "Stats" — a `web_app` button opening the Mini App directly on the subscription's detail screen (start-parameter deep link)
- "Mute alert for this route" / "Ignore for N days" — one-tap callback handled by the bot (no need to open the app for the common reaction); writes `subscriptions.muted_until`

Self-alerts (quota low, DB big, suspicious silence — §10.1) go through the same channel.

#### Subscription Manager

A CRUD layer over monitoring rules, backed by `sqlc`-generated queries. A subscription consists of:

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
| `cooldown_hours` | Anti-spam between notifications (nullable — falls back to user/env default) |
| `muted_until` | Notifications suppressed until this time; monitoring and history continue |
| `status` | active / paused / archived |

Alert parameters resolve through a cascade: **subscription → `user_settings` → env defaults** — a value set at a more specific level wins.

#### Scheduler

Runs subscription check jobs. Not a flat cron — prioritized by date proximity. Intervals are deliberately conservative while the primary source (Amadeus) has a tight quota; they tighten later once free sources (Ryanair) carry more of the load:

- **High-priority** (departure within 14 days) — every 4 hours
- **Medium** (15–60 days) — every 12 hours
- **Low** (60+ days) — once a day

**Implementation:** a hand-rolled, **stateless, DB-driven scheduler** instead of a job-queue library. Each subscription row carries `next_check_at`; a single goroutine wakes on a `time.Ticker` (every minute), selects due subscriptions (`WHERE next_check_at <= now() AND status = 'active'`), runs a check cycle for each, and writes the next `next_check_at` according to the priority tier. This gives job persistence for free (state lives in Postgres, survives restart), needs no external queue, and is ~100 lines of Go. Libraries considered — `gocron`, `robfig/cron` — rejected: they add in-memory job state that would need re-syncing with the DB anyway.

**Concurrency:** check cycles for due subscriptions run concurrently, bounded by a semaphore (`golang.org/x/sync/semaphore`, e.g. max 3 concurrent cycles). Inside one cycle, adapter calls fan out via `errgroup.WithContext`.

**Double-fire guard:** a cycle can outlive the one-minute tick, and `next_check_at` is only advanced on completion — so an in-memory "in flight" set (subscription ids currently being checked) prevents the next tick from picking the same subscription twice. In-memory is sufficient for a single instance, and keeps the restart property: a process killed mid-cycle simply reruns the check on the next tick.

**Rate limiting:** each source gets its own **`golang.org/x/time/rate.Limiter`** (token bucket) at the adapter level. The scheduler does not know about external API limits — quota handling is delegated to adapters.

**Jittering:** a random 0–60 second offset is added to `next_check_at`, so subscriptions don't fire in a burst at the same minute.

#### Multi-Airport Expander

Before dispatching requests to adapters, expands the route according to user flexibility:

- If a subscription allows alternative airports, it queues requests for each.
- **Budget-aware ordering:** the primary pair is always queried; alternative pairs are queried in order only while the source's daily budget allows (see Source Adapters). Depth of coverage never starves the primary route.
- Maintains a lookup table: for each pair (primary airport, alternative airport) — the approximate cost and duration of ground transfer (bus, car).
- At the aggregation stage, this cost is added to the ticket price for a fair comparison. For example, a ticket from an alternative airport at a lower fare but with a €25 transfer may end up more expensive than a direct departure — the bot computes the effective price for each option.

The transfer reference table is configurable per subscription (mode, approximate cost, duration). Example entries for Belgrade (BEG):

| Alternative | Mode | Price | Time |
|-------------|------|-------|------|
| BUD | bus/car | €25–40 | ~7 h |
| SOF | bus | €20 | ~6 h |
| TSR | bus | €15 | ~2.5 h |
| ZAG | bus | €30 | ~6 h |

#### Source Adapters

Each source is a separate package implementing the same interface:

```go
type Adapter interface {
    Name() string
    Search(ctx context.Context, q Query) ([]Flight, error)
}

type Query struct {
    Origin, Destination string    // IATA
    DateFrom, DateTo    time.Time
    Options             SearchOptions
}
```

Where `Flight` is a normalized struct:

```go
type Flight struct {
    Source          string    // "amadeus" | "ryanair" | ...
    PriceMinor      int64     // integer minor units (cents) — never float64 for money
    Currency        string
    Origin          string    // IATA
    Destination     string    // IATA
    DepartureAt     time.Time // in the airport's timezone
    ArrivalAt       time.Time
    Segments        []Segment
    Airline         string    // primary carrier code
    FlightNumber    string
    Stops           int
    DurationMinutes int
    BookingURL      string
    FetchedAt       time.Time
}
```

**Initial set of adapters (all must return *live* prices — that is the point of the product):**

1. **Amadeus Self-Service adapter (Flight Offers Search)** — the foundation. Official, **live** search with global GDS coverage (Air Serbia, Lufthansa Group, Turkish, Pegasus, and most traditional carriers, any airport in the world). Free tier is quota-limited (~2000 req/month) — see the daily-budget mechanism below. A half-day **spike precedes the implementation**: live requests against test and prod tiers to confirm current quotas, response quality, and whether easyJet / Vueling / AJet appear in results.
2. **Ryanair adapter** — a direct client for the semi-official `services-api.ryanair.com` endpoint (no Go library exists — the `ryanair-py` request/response shapes serve as the reference). Live prices, free, no quota; covers only Ryanair. High priority: it takes load off the Amadeus budget.
3. **Kiwi (Tequila) or Duffel adapter** — LCC coverage and virtual interlining. Kiwi's 2026 status must be verified first (open question §8); Duffel is the fallback.

**Trend-only / extended set:**

4. **Aviasales / Travelpayouts adapter** — **demoted to a trend-only source (v1.1+)**: its free Data API serves *cached* prices (what other users searched recently), not live searches. Useful to cheaply densify `price_observations` for statistics; flagged `trend_only` in the adapter registry — **its prices never trigger alerts on their own** and never appear as "current price" in notifications.
5. **Wizz Air monitoring** — via site scraping (no official API, no GDS presence). v1.1+ backlog; until then Wizz is an acknowledged coverage gap surfaced in the UI.

**Adapter registry:** each adapter declares `Name()`, `LiveSource() bool`, and covered-carrier info; the Mini App renders the per-subscription coverage line ("sources: … · not covered: …") from this registry.

**Standard adapter implementation:**

- HTTP client — a shared `*http.Client` with a tuned `Transport` (connection pool, timeouts). No third-party HTTP wrapper.
- Retry + circuit breaker — **`failsafe-go`**: exponential backoff with jitter on 429/5xx (3 attempts), and a circuit breaker that "trips" a source after N consecutive failures for a cooldown period. Logged to `api_call_log`, separate metric. Protects against quota burn.
- Rate limit — **`x/time/rate`** token bucket, parameters per source come from the config.
- **Daily request budget** — an RPS limiter caps speed, not volume, so quota-limited sources (Amadeus) additionally get a configured daily budget (`AMADEUS_DAILY_BUDGET`). The spent count is derived from `api_call_log` (requests today); when exhausted, the adapter returns a typed `ErrBudgetExhausted` — the cycle is marked `partial`, a metric is bumped, and one self-alert per day is sent. No silent quota burn.
- **Sanity check at the adapter output:** price < `MIN_REASONABLE_PRICE` (e.g., €10) or > `MAX_REASONABLE_PRICE` (€5000) → flagged, not passed to the aggregator, written to the log. Defends against "broken €1 prices" and parsing outliers.
- Every request is logged to `api_call_log` (endpoint, status, latency, remaining quota, error).
- A single adapter failure does not crash the whole cycle — the Aggregator collects per-adapter results and errors independently (errgroup with errors captured per goroutine, not propagated as cancellation).
- Response decoding via typed structs + `encoding/json`; a decode error is an adapter error, not a panic.

**Note on Kiwi Tequila:** the API has been undergoing restructuring; free-tier access has been restricted. **Verify key availability before wiring it into code.** Alternatives: **Duffel** (good API, has a test mode), **FlightAPI.io**, **SerpAPI Google Flights** (paid, but covers Wizz/Ryanair).

#### Cache Layer

Sits between Source Adapters and Aggregator — an adapter-response cache. MVP — **in-memory TTL cache** (a small hand-rolled `map` + mutex + janitor goroutine, or `patrickmn/go-cache`; the requirements don't justify ristretto/otter), growing into **Redis** only if a second process ever appears.

**Why it's needed:** with multiple alternative airports × 3 sources × N subscriptions, the same pair (origin→destination, date range) is queried many times per hour. Without a cache, free-tier limits burn out within a day.

**Key:** `(source, origin, destination, date_from, date_to, options_hash)`.
**TTL:** 15 minutes for high-priority subscriptions, 60 minutes for the rest. Configurable per source.
**Invalidation:** TTL only. Forced refresh — the "check now" action in the Mini App (`POST /api/v1/subscriptions/{id}/check`) bypasses the cache.

A write to `price_observations` happens **always** (even on a cache hit) so that history has no cache-induced gaps. But `api_call_log` is appended only on a real HTTP request.

#### Currency Normalizer

Adapters can return prices in different currencies: Amadeus — in the requested currency (but not all carriers/markets honor it), Ryanair — in the route's local currency (EUR/GBP/RON/...), Kiwi — in EUR.

All prices in the system are converted to a **base currency (EUR)** before being written to the Price History Store and compared in the Alert Engine. Otherwise `$87 < €100` → false alert.

**Source of rates:** **ECB daily reference rates** (`https://www.ecb.europa.eu/stats/eurofxref/eurofxref-daily.xml`), parsed with `encoding/xml`. Free, updated around 16:00 CET on business days.
**Rate cache:** the `fx_rates(date, currency, rate_to_eur)` table, refreshed once per day.
**Fallback:** if the ECB is unreachable, the last known rate from the DB is used (tagged `stale_rate=true` in logs).

In each `price_observation` we store **both prices** — the original (`price`, `currency`) and the normalized one (`price_eur`). This is needed for user-facing display and for auditing.

#### Aggregator / Deduplication

Collects results from all adapters, deduplicates, and sorts.

**Dedup key:** `(airline, flight_number, departure_at_date)`. The same physical flight may come back from several adapters at different prices — we keep the minimum, but preserve all sources in metadata (for debugging and for displaying "available on: Amadeus, Ryanair").

**Special case — multi-segment flights.** Deduplication uses a composite key across all segments. If any segment differs, these are distinct itineraries.

**Sorting:** by effective price (`price_eur + transfer_cost_eur`), not by raw ticket price. This way an alternative airport with transfer is compared to the primary departure on a level playing field.

**Aggregator pipeline:**

1. Collect results from all adapters (concurrent fan-out, per-adapter error isolation).
2. Currency Normalizer: each `Flight.Price` → `PriceEUR`.
3. Sanity check (second line — after the adapter): drop flights with `price_eur < €10` or `> €5000` (logged with `outlier=true`).
4. Add transfer cost for alternative airports.
5. Deduplicate by `(airline, flight_number, departure_date)`.
6. Sort by effective price.

The pipeline is a pure function over the collected Flight slice — recomputing on the same input yields the same result.

#### Price History Store

Storage for the price time series. Minimal schema:

```
price_observations (
  id              bigint pk,
  route_key       text,        -- 'BEG-BCN-2026-07-15'
  subscription_id uuid,        -- nullable, tracks which subscription's query produced it
  price           numeric,     -- original price
  currency        text,        -- original currency
  price_eur       numeric,     -- normalized to EUR
  source          text,
  flight_signature text,       -- airline+flight_number, for flight identification
  departure_at    timestamptz,
  observed_at     timestamptz, -- UTC
  hour_bucket     timestamptz, -- observed_at truncated to the hour, set by the INSERT
  outlier         bool default false,
  raw_payload     jsonb
)

-- Idempotency: the same flight from the same source within one hour
-- must not enter the DB twice (defense against retries).
-- hour_bucket is a plain column (filled with date_trunc('hour', now()) in the
-- INSERT statement) because date_trunc over timestamptz is STABLE, not
-- IMMUTABLE, and cannot be used in an expression index.
UNIQUE INDEX idx_obs_dedup ON price_observations (
  flight_signature, departure_at, source, hour_bucket
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

Implemented via a periodic job (the same scheduler loop, once per day): downsampling + deletion of expired entries.

**Outlier handling:** at write time, a record is flagged `outlier=true` if its price is more than 3× below the `route_key` median over the last 30 days (provided a sample of at least 20 points exists). Such records **do not participate** in Alert Engine aggregates but stay in the DB for auditing.

**Later** — consider the **TimescaleDB** extension for `price_observations`. Hypertables + automatic downsampling out of the box. Not for MVP.

From this data the Alert Engine computes aggregates: moving average, median, minimum over N days — as SQL queries (sqlc), not in-process loops.

#### Alert Engine

Decides whether to send a notification. Supports several strategies, selected per subscription:

| Strategy | Logic |
|----------|-------|
| `absolute_threshold` | Price ≤ `max_price` |
| `relative_drop` | Price ≤ 30-day average × (1 − `drop_pct`) |
| `historical_minimum` | New minimum over the last N days (e.g., 60) |
| `sudden_drop` | Price dropped by ≥ X% compared to the previous point |
| `combined` | Any of the above triggers (OR) |

Each strategy is a value implementing a `Strategy` interface (`Evaluate(ctx, flight, history) (Verdict, error)`); `combined` composes them.

**Warm-up guard (cold start):** history-based strategies (`historical_minimum`, `relative_drop`, `sudden_drop`) are meaningless without history — the first observation on a fresh subscription is trivially a "new minimum". They may not fire until the `route_key` has accumulated at least **K observations across at least D distinct days** (defaults K=10, D=3, configurable). Until then they return a `warming_up` verdict, which is logged but never sent. `absolute_threshold` is exempt — the user set the threshold themselves; it works from the first observation. The Mini App shows warming-up subscriptions as "📈 collecting price history (2/3 days)", and the "suspicious silence" self-alert (§10.1) skips subscriptions still in warm-up.

**Mute check:** after strategy evaluation, a subscription with `muted_until > now()` suppresses sending (decision logged with reason `muted`). Monitoring and history writes continue — mute silences notifications only, unlike pause.

**Anti-spam:**

- Cooldown between alerts per subscription (default 6 hours).
- Deduplication: if the same price for the same flight has already been sent — no alert.
- "Stable price" guard: if the price oscillates within ±2% of an already-alerted value — do not repeat.

**Decision logging:** every trigger / non-trigger writes a record with input data and the outcome. This is needed for debugging ("why didn't an alert come?").

**Dry-run mode.** A subscription-level flag — `dry_run`. In this mode the Alert Engine runs the whole pipeline but **does not send** to Telegram; it only writes to `alerts_sent` with `dry_run=true`. Used for:

- Tuning new strategies against historical data (CLI replay: `warden replay --subscription <id> --strategy ...` — a subcommand of the same binary).
- Silent testing before enabling a new route.

**Time zones (important):**

- Everything in the DB is `timestamptz` (UTC).
- `DepartureAt` / `ArrivalAt` are `time.Time` values with the airport's `*time.Location` attached.
- Airport TZ resolution — from the embedded airports dataset (`go:embed`), locations loaded via `time.LoadLocation`; the `time/tzdata` package is imported so the binary carries its own zone database (works in scratch containers and on Windows).
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
📊 Available on: Amadeus, Ryanair

[Buy] [Details] [Mute alert]
```

If flying from an alternative airport is cheaper, a separate note:

```
💡 Cheaper from Budapest (BUD):
   €52 + €25 transfer = €77 effective
```

Formatting via `text/template` templates — testable without a live bot.

#### Observability

The bot runs 24/7 unattended — without observability, a downed adapter or a burnt-out quota gets noticed a week later by the silence of alerts.

**Logs:** stdlib **`log/slog`** with the JSON handler. Each record carries `subscription_id`, `source`, `route_key`, `trace_id` (a UUID for one subscription's check cycle, propagated via `context`). Log level from config.

**Metrics:** **`prometheus/client_golang`** in pull mode (endpoint `/metrics` served by `net/http` on a separate port). Minimal set:

- `warden_adapter_requests_total{source, status}` — counter
- `warden_adapter_latency_seconds{source}` — histogram
- `warden_adapter_quota_remaining{source}` — gauge
- `warden_alerts_sent_total{strategy}` — counter
- `warden_alerts_suppressed_total{reason}` — counter (cooldown / dedup / stable_price)
- `warden_subscriptions_active` — gauge
- `warden_cycle_duration_seconds` — histogram (whole check cycle)
- `warden_source_budget_remaining{source}` — gauge (daily budget)
- `warden_db_size_bytes` — gauge

**Error tracking:** **`sentry-go`**. DSN from env, no-op when empty. The free tier (5K errors/month) covers a personal project with room to spare.

**Health endpoint:** `/health` returns 200 if the bot is alive and the DB is reachable (`pool.Ping`), otherwise 503. Used for uptime monitoring (UptimeRobot free tier).

**Health in the Mini App** — a status panel backed by `GET /api/v1/health-summary`: aliveness of each adapter (from `api_call_log` over the last hour), DB size, number of active subscriptions, last successful check cycle.

#### Config & Secrets

Config is a plain struct populated from env variables via **`caarlos0/env`**, with `.env` loaded locally by **`godotenv`** (prod uses real env / Docker secrets). Validated at startup — every required field missing → the process exits with a clear message before touching Telegram or the DB.

```go
type Config struct {
    Telegram struct {
        BotToken       string  `env:"BOT_TOKEN,required"`
        AllowedUserIDs []int64 `env:"ALLOWED_USER_IDS,required"`
    }
    Sources struct {
        AmadeusClientID     string `env:"AMADEUS_CLIENT_ID,required"`
        AmadeusClientSecret string `env:"AMADEUS_CLIENT_SECRET,required"`
        KiwiAPIKey          string `env:"KIWI_API_KEY"`
        AviasalesToken      string `env:"AVIASALES_TOKEN"` // trend-only source, v1.1+
    }
    RateLimits struct {
        AmadeusRPS  float64 `env:"AMADEUS_RPS" envDefault:"1.0"`
        RyanairRPS  float64 `env:"RYANAIR_RPS" envDefault:"0.3"`
        KiwiRPS     float64 `env:"KIWI_RPS" envDefault:"0.5"`
    }
    Budgets struct {
        AmadeusDaily int `env:"AMADEUS_DAILY_BUDGET" envDefault:"60"` // requests/day
    }
    AlertDefaults struct { // env-level fallbacks; overridable per user (user_settings) and per subscription
        CooldownHours      int     `env:"COOLDOWN_HOURS" envDefault:"6"`
        DropPct            float64 `env:"DROP_PCT" envDefault:"0.25"`
        StablePriceBandPct float64 `env:"STABLE_PRICE_BAND_PCT" envDefault:"0.02"`
        WarmupMinObs       int     `env:"WARMUP_MIN_OBSERVATIONS" envDefault:"10"`
        WarmupMinDays      int     `env:"WARMUP_MIN_DAYS" envDefault:"3"`
    }
    Observability struct {
        SentryDSN   string `env:"SENTRY_DSN"`
        LogLevel    string `env:"LOG_LEVEL" envDefault:"info"`
        MetricsPort int    `env:"METRICS_PORT" envDefault:"9090"`
    }
    DatabaseURL string `env:"DATABASE_URL,required"`
    PublicURL   string `env:"PUBLIC_URL,required"` // HTTPS base URL of the Mini App (web_app buttons, deep links)
}
```

Secrets never appear in logs: the config's `String()`/`LogValue()` redacts token fields.

#### Graceful shutdown

`main` wires everything under a root `context` cancelled on SIGINT/SIGTERM (`signal.NotifyContext`). Shutdown order: stop accepting Telegram updates → let in-flight check cycles finish (with a deadline) → close the pgx pool → flush Sentry. The scheduler's DB-driven design means a killed-mid-cycle check simply reruns on the next tick.

---

## 4. Data flow (end-to-end scenario)

**Scenario:** the user created a subscription BEG → Barcelona (BCN), dates 10–20 July 2026, departure-airport flexibility enabled (BUD, SOF, TSR, ZAG as alternatives), `combined` strategy (threshold €100 OR −25% off the average).

1. The Scheduler tick finds the subscription due (`next_check_at <= now()`, not in the in-flight set; departure ~50 days away → medium tier, every 12 hours). A `trace_id` is generated and attached to the context for end-to-end logging of the whole cycle; a `scheduler_runs` row is opened with `triggered_by = 'schedule'`.
2. The Subscription Manager hands over the rule → passed to the Multi-Airport Expander.
3. The Expander unfolds into pairs for each configured alternative: (BEG, BCN), (BUD, BCN), (SOF, BCN), (TSR, BCN), (ZAG, BCN). The primary pair goes first; alternatives follow while source budgets allow.
4. **Cache Layer** checks each key `(source, origin, destination, date_from, date_to)`. On a hit, the cached Flight slice is returned. On a miss, the request continues.
5. Miss requests fan out concurrently (errgroup) into the Amadeus and Ryanair adapters. Each adapter:
   - Waits on its own rate limiter (`x/time/rate`) and checks its daily budget (`ErrBudgetExhausted` → skip, cycle marked `partial`).
   - Applies retry/backoff and checks the circuit breaker (`failsafe-go`) — if the adapter is "tripped", the request is skipped.
   - Performs a sanity check on the response.
   - Returns a normalized Flight slice.
   - Writes the result back to the Cache Layer.
6. **Currency Normalizer** adds `price_eur` to each Flight using the current rate from `fx_rates`.
7. The Aggregator merges results, deduplicates. For alternative airports, transfer cost is added → `effective_price_eur`.
8. Every observation is written to the Price History Store (with outlier check; conflicts on the idempotency index are ignored via `ON CONFLICT DO NOTHING`).
9. The Alert Engine checks the subscription's strategy conditions for every Flight:
   - Price ≤ €100? → checks (works from day one).
   - Price ≤ 30-day average × 0.75? → only if the route is past warm-up (≥ K observations over ≥ D days); fetches the average from history (excluding `outlier=true`), checks.
   - If any matches — alert candidate.
10. Candidates pass through anti-spam (mute check, cooldown, dedup against already sent, stable-price guard).
11. If the subscription is in `dry_run`, a row is written to `alerts_sent` with the flag, without sending to Telegram.
12. Otherwise, the Notification Layer formats and sends to Telegram. The `message_id` is stored for possible later edits.
13. `next_check_at` is advanced (tier interval + jitter); cycle metrics (latency, Flights found, alerts generated) are updated in Prometheus.

---

## 5. Data model (DB schema)

Migrations use **`goose`** (plain-SQL migration files, embedded via `go:embed` and applied automatically at startup or via `warden migrate`). Queries are written in SQL and compiled to typed Go by **`sqlc`**.

```
subscriptions
  id (uuid pk), user_chat_id (bigint, indexed),
  origin (text), origin_alternatives (jsonb),
  destination (jsonb — list of IATA),
  date_from, date_to, return_date_from, return_date_to (date),
  trip_length_min, trip_length_max (int),
  max_price (numeric), max_stops (int), max_duration_minutes (int),
  airlines_whitelist (jsonb), airlines_blacklist (jsonb),
  alert_strategy (text), alert_params (jsonb),
  cooldown_hours (int nullable),             -- null → user_settings → env default
  dry_run (bool default false),
  muted_until (timestamptz nullable),        -- notifications suppressed; monitoring continues
  status (text: active/paused/archived),
  next_check_at (timestamptz, indexed),      -- drives the scheduler
  created_at, updated_at (timestamptz)

user_settings                    -- per-user defaults, lazily created on first /start or /me
  chat_id (bigint pk),
  cooldown_hours (int),
  drop_pct (numeric),
  stable_price_band_pct (numeric),
  updated_at (timestamptz)

price_observations
  id (bigint pk),
  route_key (text, indexed) — 'BEG-BCN-2026-07-15'
  subscription_id (uuid fk → subscriptions.id ON DELETE SET NULL),
  price (numeric), currency (text),
  price_eur (numeric),
  source (text),
  flight_signature (text) — 'W6-2643',
  departure_at (timestamptz),
  observed_at (timestamptz),
  hour_bucket (timestamptz),   -- observed_at truncated to the hour, set by the INSERT
  outlier (bool default false),
  raw_payload (jsonb)

  UNIQUE (flight_signature, departure_at, source, hour_bucket)
  INDEX (route_key, observed_at DESC)
  INDEX (route_key, flight_signature, observed_at DESC)

alerts_sent
  id (bigint pk),
  subscription_id (uuid fk → subscriptions.id ON DELETE CASCADE),
  flight_signature (text),
  price_eur (numeric),
  strategy_triggered (text),
  sent_at (timestamptz),
  message_id (bigint),
  dry_run (bool default false)

  INDEX (subscription_id, sent_at DESC)

api_call_log
  id (bigint pk),
  source (text), endpoint (text),
  status_code (int), duration_ms (int),
  rate_limit_remaining (int nullable),
  error (text nullable),
  called_at (timestamptz)

  INDEX (source, called_at DESC)

fx_rates
  date (date pk part),
  currency (text pk part) — 'USD', 'GBP', ...
  rate_to_eur (numeric),
  fetched_at (timestamptz)

  PRIMARY KEY (date, currency)

scheduler_runs                 -- also the resource behind GET .../runs/{run_id} polling
  id (bigint pk),
  subscription_id (uuid fk),
  triggered_by (text: schedule/manual),
  started_at, finished_at (timestamptz),
  trace_id (uuid),
  flights_found (int),
  alerts_generated (int),
  status (text: success/partial/failed),
  error (text nullable)
```

**Foreign keys:** `subscription_id` in `price_observations` — `ON DELETE SET NULL` (history outlives subscription deletion); in `alerts_sent` — `ON DELETE CASCADE` (alerts are meaningless without a subscription).

**Money as `numeric`**, scanned into Go as `pgtype.Numeric` / converted to a minor-units `int64` in the domain — never `float64` arithmetic on money in comparisons that gate alerts.

---

## 6. Implementation roadmap (MVP → Full)

See **[PLAN.md](PLAN.md)** for the detailed phase-by-phase development plan. Summary:

### MVP

Goal: a working bot for a single route, one source, real alerts. **Already with the baseline infrastructure** so it doesn't need to be redone later.

**Functionality:**

- Bot: `/start` with "Open App" button, whitelist, plain-text alert delivery.
- Mini App: subscriptions list + create/delete form (airport autocomplete, date range); HTTP API with initData auth.
- Subscription Manager on Postgres (sqlc), without alternative airports; `user_settings` table.
- One adapter: **Amadeus Self-Service** (preceded by the coverage/quota spike), with the daily-budget mechanism.
- Scheduler: DB-driven ticker loop, single conservative tier (every 4 hours), in-flight guard.
- Currency Normalizer with ECB rates.
- Price History Store: write + simple "minimum over N days" query.
- Alert Engine: `absolute_threshold` and `historical_minimum` with the warm-up guard, cooldown.
- Notification Layer: text notifications without inline buttons.

**Infrastructure (from day one):**

- goose migrations, sqlc codegen.
- Typed env config with startup validation.
- slog JSON + Sentry.
- Docker + docker-compose (app + Postgres).
- Table-driven tests with `testify`; adapter tests against `httptest.Server` fixtures.
- GitHub Actions: `golangci-lint` + `go vet` + tests + build.

### v1.0

- Ryanair adapter (high priority — free live prices, relieves the Amadeus budget); Kiwi or Duffel adapter.
- Aggregator with deduplication and sanity check.
- Cache Layer (in-memory).
- Multi-Airport Expander with the transfer reference table.
- Circuit breaker for adapters.
- Alert Engine: `relative_drop`, `combined`, full anti-spam (cooldown + dedup + stable-price).
- Inline buttons in notifications (Buy / open-in-app deep link / Mute via `muted_until`).
- Mini App: edit/pause/resume/mute, stats screen with price chart, source-coverage line, health panel, settings; Prometheus `/metrics`.
- Priority tiers in the scheduler; dry-run mode; daily retention job.
- Postgres backup (pg_dump cron) to a separate volume.

### v1.1+ (as needed)

- **Wizz Air scraper** (chromedp) — top backlog candidate; prioritize based on real pain after a month of operation.
- Travelpayouts as a trend-only source (densifies history, never alerts).
- TimescaleDB for `price_observations`.
- Redis for the Cache Layer (only if a second process appears).
- Duffel adapter as a backup.
- Trend Analyzer — a weekly digest across subscriptions (bot message + Mini App screen).
- Smart suggestions ("where to fly cheaply from BG this weekend").
- Round-trip support with two independent tickets on different airlines — requires reworking the Flight model into an Itinerary.

---

## 7. Risks and mitigations

| Risk | Mitigation |
|------|------------|
| Ryanair adapter ban / block (unofficial API) | Graceful fallback, circuit breaker, do not crash the cycle. Availability monitoring via `/health`. |
| Free API limits exhausted (Amadeus quota) | **Daily budget per source** (hard stop with `ErrBudgetExhausted`, self-alert), Cache Layer, scheduler jittering, conservative check intervals, primary-pair-first expansion. Metrics: `warden_adapter_quota_remaining`, `warden_source_budget_remaining`. |
| Wizz Air not covered by any v1.0 source | Accepted consciously; the Mini App shows a per-subscription coverage line so the boundary is visible. Scraper is the top v1.1+ candidate. |
| Alert noise on fresh subscriptions (cold start) | Warm-up guard: history-based strategies silent until ≥ K observations over ≥ D days; UI shows "collecting history" status. |
| API response schema changes | Strict typed decoding at the adapter output, contract tests with recorded fixtures (`httptest`), `raw_payload` logged in the DB. Sentry alert on a spike in parsing errors. |
| DB bloat | Retention policy with downsampling and deletion of old observations. The `warden_db_size_bytes` metric. |
| Notification spam | Cooldown, dedup, stable-price guard (±2%). "Mute alert" button. |
| False triggers (broken €1 price, price in cents) | Two-layer sanity check: inside the adapter (absolute thresholds) + outlier flag in the DB (relative to the route median). Outliers excluded from aggregates. |
| Time zones | UTC in the DB (`timestamptz`), TZ-aware `time.Time` via embedded airport dataset + `time/tzdata`. Prices in EUR after normalization. Covered with fake-clock tests. |
| FX rate swings / ECB unreachable | Rate cache in the DB, fallback to last known, `stale_rate=true` log. Sentry alert if rates aren't refreshed for > 3 days. |
| External API deprecation (Kiwi Tequila move) | Isolation via the `Adapter` interface. Replacement plan — Duffel/FlightAPI/SerpAPI. Contract tests catch breaking changes. |
| Goroutine leaks / stuck cycles | Every external call bounded by `context` deadlines; per-cycle timeout; `warden_cycle_duration_seconds` metric; `goleak` in tests. |
| Data loss on restart | Scheduler state lives in Postgres (`next_check_at`) — nothing to lose. Daily pg_dump. Idempotent writes in `price_observations`. |
| Whitelist bypass or token leak | All secrets via env / Docker secrets, never in git. ALLOWED_USER_IDS enforced in both bot and API middleware. Logging of dropped updates/requests for auditing. Config redacts secrets in logs. |
| Forged / replayed Mini App requests | initData HMAC validation on every request (bot-token key, constant-time compare), `auth_date` freshness ≤ 24 h, identity only from validated initData, ownership checks per mutation. |

---

## 8. Open questions

**Resolved in 0.7 (grilling session):**

- ~~Cached or live prices for the foundation adapter?~~ → Live search is the point of the product. Amadeus Self-Service replaces Aviasales/Travelpayouts as the foundation; Travelpayouts demoted to trend-only (v1.1+). A spike verifies Amadeus quotas/coverage before implementation.
- ~~How does "check now" report back to the Mini App?~~ → Polling: `202 {run_id}` + `GET .../runs/{run_id}` over `scheduler_runs`.
- ~~Cold start of history-based strategies?~~ → Warm-up guard (K=10 observations / D=3 days), `absolute_threshold` exempt.
- ~~Quota protection?~~ → Per-source daily budgets + conservative tiers (4h/12h/24h) + primary-pair-first expansion.
- ~~Where do user settings and mutes live?~~ → `user_settings` table (cascade subscription → user → env) and `subscriptions.muted_until`.
- ~~Wizz Air coverage?~~ → Accepted gap for v1.0, surfaced in the UI; scraper is the top backlog item.

**Resolved in 0.6 (Mini App):**

- ~~Bot dialogs vs web UI?~~ → Telegram Mini App (React) is the only management UI; the bot keeps `/start`, `/help`, and alert delivery. The inline-keyboard FSM (`/new` pickers, datepicker) is dropped entirely.
- ~~Where to host the frontend?~~ → Embedded into the Go binary (`go:embed`); one artifact, no CORS. TLS via a Caddy sidecar.
- ~~Who actually needs price history charts~~ (open since 0.2) → charts render in the Mini App (`recharts`), no PNG generation needed.

**Resolved in the Go rewrite (0.5):**

- ~~SQLite or Postgres from the start?~~ → Postgres from day one (Docker Compose makes it free operationally; sqlc+pgx work best against real Postgres; removes a future migration project).
- ~~Job-queue library for the scheduler?~~ → None; DB-driven `next_check_at` + ticker.
- ~~Where to deploy?~~ → Hetzner Cloud (~€4–5/month), carried over from v0.2.
- ~~Webhook or long-polling?~~ → Long-polling, carried over.

**Still open:**

- **Domain for the Mini App HTTPS URL.** Options: buy a cheap domain (cleanest, ~€10/yr, Caddy + Let's Encrypt), a free host-based hack (`<ip>.sslip.io` + Let's Encrypt), or a permanent cloudflared tunnel. Architecture is unaffected; decide before the first deploy. Local dev uses a cloudflared/ngrok tunnel either way.
- **Kiwi Tequila status in 2026.** Needs verification: is the free tier still around, are new keys being issued? Plan B — Duffel (has a test mode).
- **Round-trip with virtual interlining.** Requires reworking `Flight` → `Itinerary` (a list of Flights treated as one ticket). Deferred to v1.1+; for now round-trip is treated as a pair of independent one-ways.
- **Ground transport.** A static lookup vs the Omio API. Reassess after v1.0.
- **Wizz Discount Club.** Headless browser (chromedp) in a separate container vs IMAP email forwarding. Decide in v1.1+.

---

## 9. Technology stack

Every choice comes with rationale. Alternatives are listed where they were actually considered.

### Language and runtime

- **Go 1.24+** — single static binary, tiny deploy artifact, goroutine concurrency maps naturally onto "N subscriptions × M adapters" fan-out, first-class `context` cancellation.
- Concurrency primitives: `errgroup` for fan-out, `x/sync/semaphore` for bounding, channels only where they genuinely help.

### Frontend (Mini App)

- **React 19 + TypeScript + Vite** — the standard Mini App stack; most examples and templates target it.
- **`@telegram-apps/sdk-react`** — typed Mini App platform bindings (initData, theme, BackButton/MainButton, haptics).
- **`@telegram-apps/telegram-ui`** — Telegram-native components; automatic theming.
- **TanStack Query** — server-state management; **`recharts`** — price history charts.
- Build output embedded via `go:embed`; hashed asset filenames make caching trivial.
- Alternatives: Svelte (lighter, poorer Telegram-widget ecosystem), Go templates + HTMX (no build step, but hand-rolled SDK integration) — rejected.

### HTTP API

- **`net/http`** with Go 1.22+ pattern routing (`GET /api/v1/subscriptions/{id}`) — no router dependency.
- **`telegram-mini-apps/init-data-golang`** — initData HMAC validation + parsing (the documented Telegram scheme; small, focused library).

### Telegram bot

- **`github.com/go-telegram/bot`** — zero-dependency, actively maintained, full Bot API coverage, middleware, prefix-routed callback handlers. Used for `/start`, `/help`, alert delivery with inline buttons.
- Alternatives: `tucnak/telebot` (popular but development slowed), `mymmrac/telego`.
- No FSM, no dialog widgets — that role moved to the Mini App.

### HTTP clients and resilience

- **`net/http`** with a tuned shared `Transport` — the stdlib client is the async HTTP standard in Go; no wrapper library.
- **`github.com/failsafe-go/failsafe-go`** — retry with exponential backoff + jitter, and circuit breaker, composed as policies. One library for both concerns.
- **`golang.org/x/time/rate`** — token bucket for per-source rate limiting.

### Scheduler

- **Hand-rolled DB-driven loop** — `time.Ticker` + `next_check_at` column. Persistence and restart-safety come from Postgres; no job-queue dependency.
- Alternatives: `gocron`, `robfig/cron` — rejected: in-memory job state duplicates what the DB already holds.

### Database

- **PostgreSQL 16** + **`jackc/pgx/v5`** (with `pgxpool`) — the canonical Go Postgres driver.
- **`sqlc`** — SQL-first: queries live in `.sql` files, compile to typed Go. No ORM magic, reviewable SQL, and the Alert Engine aggregates are plain SQL anyway.
- **`goose`** — plain-SQL migrations, embeddable, applied at startup or via CLI subcommand.
- Alternatives: GORM — rejected (reflection-heavy, hides SQL); `sqlx` — fine but sqlc gives compile-time checked queries; `golang-migrate` — fine too, goose chosen for embedded-migrations ergonomics.

### Cache

- **MVP:** in-memory TTL cache (`patrickmn/go-cache` or ~80 hand-rolled lines).
- **Later:** Redis (`redis/go-redis`) only if a second process needs the cache.

### Config

- **`caarlos0/env`** + **`joho/godotenv`** — typed struct from env vars, `.env` locally, validated at startup.
- Alternatives: `koanf`/`viper` — overkill for a flat env-only config.

### Logging, metrics, errors

- **`log/slog`** (stdlib) — structured JSON logs, `trace_id` via context.
- **`prometheus/client_golang`** — pull metrics on a separate port.
- **`getsentry/sentry-go`** — error tracking, free tier.

### Time, TZ, and airports

- **`time` + `time/tzdata`** (stdlib) — the binary carries its own zone database.
- **Embedded OurAirports dataset** (`go:embed` CSV, parsed once at startup) — offline IATA lookup, city search for the airport picker, and TZ resolution. Replaces Python's `airportsdata`.

### Money

- Prices stored as `numeric` in Postgres; in Go — integer minor units (`int64` cents) for comparisons, `float64` only at display edges.

### Testing

- **`testify`** (assert/require) + table-driven tests — the foundation.
- **`net/http/httptest`** — adapter contract tests against recorded fixture responses in `testdata/{source}/`.
- **`jonboulle/clockwork`** — injectable fake clock for Alert Engine / scheduler tests (replaces `freezegun`).
- **`testcontainers-go`** — real Postgres in integration tests (replaces "temp SQLite file"); `go test -short` skips them.
- **`uber-go/goleak`** — goroutine-leak detection in tests.
- Coverage target: 80% for domain logic (adapters, aggregator, alert engine); I/O wrappers — best effort.

### Lint and static analysis

- **`golangci-lint`** — govet, staticcheck, errcheck, revive, gofumpt, and friends in one runner.
- **`go vet`** in CI as a baseline.

### Containerization and deployment

- **Docker multi-stage build**: `node:22` (Vite build) → `golang:1.24` (embeds `web/dist`) → `gcr.io/distroless/static` runtime. Image ≈ 20 MB.
- **docker-compose**: app + `postgres:16-alpine` + **Caddy** (TLS termination, automatic Let's Encrypt for the Mini App HTTPS URL), `restart: always`.
- Deployment — **Hetzner Cloud** (~€4–5/month).
- CI/CD — **GitHub Actions**: lint → tests → docker build → push to GHCR → ssh-deploy (or watchtower).

### Summary table

| Layer | MVP | v1.1+ |
|-------|-----|-------|
| Frontend | React + TS + Vite, @telegram-apps/sdk-react, telegram-ui | same |
| API | net/http + init-data-golang | same |
| Bot framework | go-telegram/bot | same |
| HTTP (outbound) | net/http + failsafe-go + x/time/rate | same |
| Scheduler | DB-driven ticker loop | same |
| DB | PostgreSQL 16 + pgx v5 | + TimescaleDB (opt.) |
| Queries / migrations | sqlc + goose | same |
| Cache | in-memory TTL | Redis (opt.) |
| Config | caarlos0/env + godotenv | same |
| Logs | slog (JSON) | + Grafana Loki |
| Metrics | client_golang | + Grafana Cloud |
| Errors | sentry-go | same |
| Tests | testify + httptest + clockwork + testcontainers | + nightly integration vs real APIs |
| TLS / ingress | Caddy sidecar (Let's Encrypt) | same |
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

1. **Unit** — pure functions: Alert Engine strategies, Currency Normalizer, Aggregator dedup. Fast (< 1 s per suite), no I/O, table-driven. `clockwork` for time-sensitive tests.
2. **Adapter contract tests** — fixtures with recorded real API responses in `testdata/{source}/`. An `httptest.Server` replays them. Test: "parse response → expected Flight slice". Breaks when the external API changes its schema.
3. **API handler tests** — `httptest` against the real mux: signed initData fixtures (valid / expired / tampered / non-whitelisted), CRUD flows, ownership checks. The initData signer is a test helper (HMAC with a test bot token).
4. **Integration tests** — Postgres via `testcontainers-go` for the storage layer; real external APIs only in a **nightly CI job** (build tag `integration`), not per PR, to avoid burning quotas.
5. **End-to-end** — a mini scenario: create a subscription through `POST /api/v1/subscriptions` → run one scheduler cycle → assert that an observation (and optionally an alert sent to the fake Telegram server) appeared.
6. **Frontend** — `vitest` + Testing Library for form logic (validation, date-range constraints); no browser-automation suite for MVP — the API contract tests carry the weight.

**Doubles:**

- Telegram Bot API — `httptest.Server` impersonating `api.telegram.org` (go-telegram/bot accepts a custom server URL).
- Mini App auth — helper-signed initData strings; no Telegram involved.
- DB — testcontainers Postgres, one container per package, per-test schema.
- Time — `clockwork.FakeClock` injected where scheduling/cooldowns are computed.
- HTTP — `httptest.Server` per adapter.

### 10.3. CI/CD

**On every PR:**

- `golangci-lint run`
- `go vet ./...`
- `go test -short ./...` + coverage > 75%
- `npm run lint` (eslint + tsc) + `npm run test` (vitest) + `npm run build` in `web/`
- `go build ./...` (with the built `web/dist` embedded) + Docker image build (without push)
- `sqlc diff` / `sqlc vet` — generated code in sync with queries

**On merge to `master`:**

- The full pipeline above (including testcontainers integration tests)
- Push the Docker image to GHCR with tags `latest` and `sha-XXX`
- SSH deploy to the VPS: `docker compose pull && docker compose up -d`
- Smoke test: HTTP `/health` returns 200 within 30 s after restart.

**Nightly:**

- `go test -tags integration ./...` against real APIs.
- FX-rate freshness check.
- pg_dump backup to S3 / a separate volume.

### 10.4. Security

- **Mini App auth** — initData HMAC validation on **every** API request (constant-time compare, `auth_date` ≤ 24 h); the API trusts nothing from the client body about identity — `user_chat_id` always comes from validated initData.
- **User whitelist** — enforced twice: bot middleware (updates) and API middleware (requests). Object ownership checked per mutation.
- **Secrets** — only via env / Docker secrets, never in git. `.env.example` without values. Config redacts secrets in `String()`/`LogValue()`. The bot token doubles as the initData signing key — one more reason it never leaves env.
- **`.gitignore`** covers `.env`, local dumps, build artifacts, `node_modules`.
- **Telegram bot token** is rotated via @BotFather if compromised (this also invalidates outstanding initData).
- **Input validation** — server-side in the domain layer regardless of the Mini App's client-side checks: IATA codes must exist in the airports dataset, dates via `time.Parse`, ranges/enums validated before persisting. The Mini App is a convenience, not a trust boundary.
- **Standard web headers** on the SPA: CSP (self + Telegram SDK requirements), no third-party origins.
- **SQL injection** — impossible by construction: all queries are sqlc-generated with bound parameters; no string-built SQL.
- **Dependency audit** — `govulncheck` in CI, Dependabot for go.mod.
- **Backup** — once-a-day pg_dump to S3 (Hetzner Storage Box ~€3/month for 1 TB).

### 10.5. Repository layout

```
air-tickets-warden/
  cmd/warden/
    main.go             — entrypoint, subcommands: run (default), migrate, replay
  internal/
    bot/                — /start, /help, alert delivery, callback handlers, middlewares
    api/                — HTTP handlers /api/v1, initData auth middleware
    domain/             — Subscription, Flight, Segment, alert strategies (pure Go, no I/O)
    adapters/           — amadeus/, ryanair/, ... + shared resilient HTTP client, registry
    services/           — aggregator, alert engine, currency normalizer, expander,
                          subscription manager, airports (embedded dataset)
    scheduler/          — DB-driven ticker loop
    storage/            — pgx pool, sqlc-generated code, goose runner
    cache/              — in-memory TTL cache
    telemetry/          — slog setup, prometheus registry, sentry init
    config/             — env config struct + validation
    web/                — HTTP server wiring: SPA static (go:embed), /health, /metrics
  web/
    src/                — React app (screens, components, api client)
    dist/               — Vite build output (embedded; gitignored)
    package.json, vite.config.ts, tsconfig.json
  db/
    migrations/         — goose SQL files (embedded)
    queries/            — sqlc query files
  sqlc.yaml
  docker/
    Dockerfile          — node build → go build → distroless
    docker-compose.yml  — app + postgres + caddy
    Caddyfile
  .github/workflows/
    ci.yml
    deploy.yml
    nightly.yml
  go.mod
  Makefile
  .env.example
  README.md
  PLAN.md
```

The dependency rule mirrors the old hexagonal intent: `internal/domain` imports nothing from the outer packages; `adapters`/`storage`/`bot` depend inward. Everything under `internal/` — nothing is importable from outside the module.

---

## 11. Source rate limits and quotas (draft)

| Source | Live? | Quota | Rate limit | Documentation | Notes |
|--------|-------|-------|------------|---------------|-------|
| **Amadeus self-service** | **Yes** | ~2000 req/month (test); prod has small free quota, then per-request cents | 10 RPS | developers.amadeus.com | **Foundation.** Verify current tiers on the Phase 2 spike; guarded by `AMADEUS_DAILY_BUDGET` |
| **Ryanair (services-api)** | **Yes** | unofficial | low load | reverse-engineered | Ban risk on abuse; free, relieves the Amadeus budget |
| **Kiwi Tequila** | Yes | (deprecated?) | 0.5 RPS | tequila.kiwi.com | **Verify API status in 2026**; Duffel is the fallback |
| **Duffel** | Yes | up to 1000 req/hour test | — | duffel.com/docs | Test environment is free |
| **Aviasales / Travelpayouts** | **No — cached prices** | no hard limit | ~1 RPS recommended | docs.travelpayouts.com | Trend-only source (v1.1+): densifies history, never triggers alerts |
| **ECB FX rates** | — | unlimited | politely: 1 request/day | www.ecb.europa.eu | XML feed, refreshed around 16:00 CET on business days |

**All values are approximate.** Before each production launch — cross-check with the source's current documentation. Recording the actual `rate_limit_remaining` into `api_call_log` gives the real picture.

---

## 12. Changelog

- **0.7 — 2026-07-22.** **Design review (grilling session) — seven decisions.** (1) Foundation adapter switched Aviasales/Travelpayouts → **Amadeus Self-Service**: the Travelpayouts free API serves cached prices while the product requires live search; Ryanair promoted, Travelpayouts demoted to trend-only (v1.1+); an Amadeus coverage/quota spike precedes Phase 2. (2) "Check now" contract defined: `202 {run_id}` + polling `GET .../runs/{run_id}` over `scheduler_runs` (+ `triggered_by` column). (3) Warm-up guard for history-based alert strategies (K=10 obs / D=3 days; `absolute_threshold` exempt; "collecting history" status in the UI). (4) Per-source daily request budgets (`ErrBudgetExhausted`, `warden_source_budget_remaining`), conservative scheduler tiers (4h/12h/24h), primary-pair-first expansion. (5) New `user_settings` table (cascade subscription → user → env) and `subscriptions.muted_until` (mute silences notifications only). (6) Fixes: `Flight.PriceMinor int64` replaces `Price float64`; explicit `hour_bucket` column replaces the non-immutable `date_trunc` index expression; scheduler in-flight guard against double-fire; orphan metrics added. (7) Wizz Air gap accepted for v1.0 with an honest per-subscription source-coverage line in the UI. Time estimates removed from both documents. "Any airport in the world" principle made explicit.
- **0.6 — 2026-07-21.** **Telegram Mini App becomes the management UI.** New components: Mini App Frontend (React 19 + TS + Vite, `@telegram-apps/sdk-react`, `telegram-ui`, TanStack Query, recharts; embedded via `go:embed`) and HTTP API Layer (`/api/v1`, `net/http` pattern routing, initData HMAC auth via `init-data-golang`, whitelist middleware). The bot is reduced to entry point (`/start` + web_app button) and notification delivery with inline buttons; the inline-keyboard FSM, `/new` dialog, pickers, and management commands (`/new`, `/list`, `/pause`, `/search`, `/stats`) are removed — replaced by Mini App screens and API endpoints. `go-telegram/ui` dropped from the stack. Docker gains a node build stage and a Caddy TLS sidecar; the compose ingress serves the Mini App over HTTPS. New open question: domain for the HTTPS URL. Charts open question closed (recharts in-app).
- **0.5 — 2026-07-21.** **Full rewrite of the design for Go.** Stack replaced: aiogram → `go-telegram/bot` + `go-telegram/ui`; SQLAlchemy/Alembic/aiosqlite → PostgreSQL from day one with `pgx` + `sqlc` + `goose` (SQLite stage dropped); APScheduler → hand-rolled DB-driven ticker scheduler (`next_check_at` column); httpx/tenacity/aiolimiter/pybreaker → `net/http` + `failsafe-go` + `x/time/rate`; pydantic-settings → `caarlos0/env`; structlog → `log/slog`; airportsdata → embedded OurAirports dataset; pytest/respx/freezegun → testify/httptest/clockwork/testcontainers. Repository relaid out as `cmd/` + `internal/`. Money moved to `numeric`/minor units. Development plan extracted to PLAN.md. Python implementation removed (available in git history up to commit `bbe5ceb`).
- **0.4 — 2026-05-24.** Added "Input UX for the `/new` dialog": airport-picker dropdown and inline calendar.
- **0.3 — 2026-05-23.** Translated the document from Russian to English. No content changes.
- **0.2 — 2026-05-23.** Clarified the Python stack; added Cache Layer, Currency Normalizer, Observability; ops sections.
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
- **Mini App** — a web application opened inside Telegram (WebView) over HTTPS; Telegram supplies signed user identity (`initData`) and native chrome (theme, BackButton, MainButton).
- **initData** — the query-string payload Telegram injects into a Mini App: user info + `auth_date` + an HMAC-SHA256 signature keyed by the bot token. Validated server-side on every API request.
- **FX** — foreign exchange. In the bot's context — currency rates for normalizing prices to EUR.
- **Circuit breaker** — a resilience pattern: after N consecutive failures of an external service, it automatically "opens the circuit" for a cooldown period, sparing requests.
- **Token bucket** — a rate-limiting algorithm: tokens refill at a set rate, each request consumes one.
- **Idempotency** — the property of an operation where repeating it doesn't change the result. Implemented via UNIQUE indexes + `ON CONFLICT DO NOTHING`.
- **Outlier** — a statistical anomaly. In context — a suspicious price that does not participate in history aggregates.
- **Downsampling** — reducing the sampling frequency of a time series (e.g., hourly → daily) to save storage.
- **Dry-run** — a mode in which an operation runs but has no side effects. For subscriptions — an alert is written to the DB but not sent.
- **errgroup** — `golang.org/x/sync/errgroup`; structured concurrent fan-out with shared context cancellation.
