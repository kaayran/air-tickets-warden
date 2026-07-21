# Air Tickets Warden — Development Plan (Go + Mini App)

Companion to the [design document](air-tickets-warden.md). Phases are ordered so that every phase ends in a runnable, testable state and later phases never require redoing earlier work.

Stack decisions (fixed): Go 1.24+, PostgreSQL 16 + `pgx/v5` + `sqlc` + `goose`, hand-rolled DB-driven scheduler, `failsafe-go` + `x/time/rate`, `slog` + Prometheus + Sentry; **frontend — Telegram Mini App on React 19 + TS + Vite** (`@telegram-apps/sdk-react`, `telegram-ui`, TanStack Query), embedded into the Go binary via `go:embed`; **bot (`go-telegram/bot`) — entry point + notifications only**; API auth via initData HMAC (`init-data-golang`); TLS via Caddy sidecar. Tests: testify/httptest/clockwork/testcontainers + vitest.

---

## Phase 0 — Skeleton & infrastructure (3–4 days)

Goal: an empty but production-shaped service: Go binary serving an embedded React shell + `/health`, bot answering `/start` with an "Open App" button, everything in Docker and CI.

1. **Module & layout.** `go mod init`, package tree from §10.5 (incl. `internal/api`, `internal/web`); `web/` scaffold via Vite (React + TS template + `@telegram-apps/sdk-react` + `telegram-ui`). `Makefile`: `run`, `test`, `lint`, `fmt`, `migrate`, `sqlc`, `web-build`, `docker`, `tunnel`.
2. **Config.** `internal/config`: `caarlos0/env` + `godotenv`, startup validation, secret redaction, `PUBLIC_URL`.
3. **Telemetry.** slog JSON, Prometheus registry, Sentry (no-op on empty DSN).
4. **Storage bootstrap.** pgxpool, goose runner with embedded `db/migrations`, migration `0001` (`subscriptions` with `next_check_at`), `sqlc.yaml`, `warden migrate` subcommand.
5. **Web server.** `internal/web`: serves embedded `web/dist` (SPA fallback to `index.html`), `/health`, `/metrics`; `internal/api`: `/api/v1` mux + initData auth middleware (validate HMAC, check `auth_date`, extract user, enforce whitelist) + a first `GET /api/v1/me`.
6. **Bot shell.** Long-polling, whitelist middleware, `/start` with `web_app` button → `PUBLIC_URL`, `/help`; menu button configured via `setChatMenuButton` at startup.
7. **Dev loop.** Vite dev server proxying `/api` to the Go server; cloudflared/ngrok tunnel target documented in README (Telegram must reach a HTTPS URL to open the Mini App).
8. **Lifecycle.** `signal.NotifyContext`, ordered graceful shutdown (bot → HTTP server → in-flight work → pool → Sentry flush).
9. **Docker & CI.** Multi-stage Dockerfile (node → go → distroless), compose: app + `postgres:16-alpine` + Caddy (Caddyfile with the chosen domain); `ci.yml`: golangci-lint, `go vet`, `go test -short`, `npm lint/test/build`, sqlc drift check, image build.

**Exit criteria:** `docker compose up` → Mini App shell opens from the bot's button and shows data from `/api/v1/me`; non-whitelisted users are rejected in both bot and API; CI green.

## Phase 1 — Subscriptions: API + Mini App CRUD (3–4 days)

Goal: full subscription lifecycle through the Mini App, stored in Postgres.

1. **Domain.** `internal/domain`: `Subscription`, validation rules (IATA against dataset, date ranges, enums), status enum. Pure package.
2. **Airports service.** `go:embed` OurAirports CSV; lookup by IATA, prefix/city search, TZ resolution. Unit tests.
3. **Subscription Manager.** sqlc-backed CRUD, scoped by `user_chat_id`.
4. **API.** `GET/POST /subscriptions`, `GET/PATCH/DELETE /subscriptions/{id}`, `GET /airports?q=`. Handler tests with signed initData fixtures (valid / expired / tampered / foreign object).
5. **Mini App screens.** Subscriptions list (status badges, pause/resume/delete actions); create/edit form: airport autocomplete over `/airports`, date-range picker, threshold stepper. TanStack Query wiring, optimistic updates where cheap.
6. **Tests.** Domain + airports unit; manager integration (testcontainers); API handler suite; vitest for form validation logic.

**Exit criteria:** create → see → edit → pause → delete a subscription entirely from the phone; rows visible in Postgres; foreign-ownership requests 404.

## Phase 2 — First adapter + FX (2–3 days)

Goal: real prices from Aviasales, normalized to EUR, visible in the Mini App.

1. **Adapter contract.** `Adapter` interface, `Flight`/`Segment`/`Query` types, shared resilient HTTP client (failsafe-go retry, `x/time/rate`, `api_call_log`, metrics).
2. **Aviasales adapter.** Typed decoding → `Flight`, sanity check (€10–€5000), booking deep link. Contract tests on recorded fixtures in `testdata/aviasales/`.
3. **FX service.** ECB XML fetch, `fx_rates` migration + daily refresh, stale fallback, EUR minor-units normalization.
4. **Ad-hoc check.** `POST /subscriptions/{id}/check` (202 + async run) and a results view in the subscription detail screen — first end-to-end proof of real data.

**Exit criteria:** "Check now" in the Mini App returns live Aviasales prices in EUR; contract tests pin the response schema.

## Phase 3 — History, scheduler, alerts → **MVP** (3–4 days)

Goal: the bot monitors autonomously and delivers real alerts to Telegram.

1. **Price History Store.** `price_observations` migration (idempotency UNIQUE + indexes), `ON CONFLICT DO NOTHING` writes, aggregate queries (min/avg over N days excl. outliers), outlier flagging.
2. **Scheduler.** Ticker loop → select due (`next_check_at <= now()`), semaphore-bounded cycles, jittered rescheduling, `scheduler_runs`, per-cycle trace_id + timeout. Single hourly tier for MVP. Clockwork + goleak in tests.
3. **Alert Engine.** `Strategy` interface; `absolute_threshold`, `historical_minimum`; cooldown; `alerts_sent` + dedup. Table-driven tests with fake clock.
4. **Notification delivery.** `text/template` formatting; bot sends alert messages (plain text for MVP), stores `message_id`.
5. **Wiring + E2E.** scheduler → adapter → FX → history → alerts → bot. E2E: fake Aviasales + fake Telegram + real Postgres → observation and alert rows appear, message hits the fake Telegram server.

**Exit criteria = MVP:** a subscription left running produces observations every hour and a Telegram alert when the price crosses the threshold; restart loses nothing.

## Phase 4 — Multi-source & aggregation (3–4 days)

1. **Ryanair adapter.** Hand-written client for `services-api.ryanair.com` (ryanair-py as behavioral reference). Circuit breaker mandatory here.
2. **Kiwi adapter** — after verifying 2026 API status (open question §8); otherwise start Duffel instead.
3. **Cache Layer.** TTL map keyed by `(source, route, dates, options_hash)`; observations always written; "check now" bypasses.
4. **Aggregator.** Dedup by `(airline, flight_number, departure_date)` incl. multi-segment keys; keep min price, preserve source list; effective-price sort.
5. **Multi-Airport Expander.** Transfer-cost lookup (per-subscription config in the edit form), effective price = ticket + transfer.
6. **Scheduler tiers.** High/medium/low intervals by departure proximity.

**Exit criteria:** one cycle fans out across sources and alternative airports, dedups, compares on effective price; a dead source degrades gracefully (circuit breaker + `partial` status).

## Phase 5 — Alert polish & Mini App v1.0 (3–4 days)

1. **Alert strategies.** `relative_drop`, `sudden_drop`, `combined`; full anti-spam (stable-price ±2%); decision logging; dry-run toggle in the edit form + `warden replay` subcommand.
2. **Rich notifications.** Inline buttons: Buy (deep link), Details/Stats (`web_app` button deep-linking into the subscription's screen via start parameter), Mute (one-tap callback).
3. **Stats screen.** Price history chart (recharts), min/avg/trend, recent alerts list (`GET /stats`, `GET /alerts`).
4. **Health panel + settings.** `GET /health-summary` (adapter aliveness, last cycle, DB size); settings screen for defaults (cooldown, drop %).
5. **Retention job.** Daily downsampling + expiry per design-doc policy.
6. **Full metrics set** from §3.2 Observability; Mini App polish (theme, haptics, BackButton navigation).

**Exit criteria = v1.0:** feature parity with the design doc's v1.0 scope; coverage ≥ 75–80% on domain logic.

## Phase 6 — Deployment & operations (1–2 days)

1. **Domain decision** (open question §8): buy a domain / sslip.io / cloudflared tunnel → fix `PUBLIC_URL`, Caddyfile, and the BotFather Mini App URL.
2. **Deploy pipeline.** `deploy.yml`: on merge to master — build (node+go), push GHCR, ssh-deploy, `/health` smoke test.
3. **Nightly.** `-tags integration` against real APIs, FX freshness check, pg_dump backup.
4. **VPS setup.** Hetzner instance, compose with `restart: always`, UptimeRobot on `/health`, Sentry project, self-alert Telegram messages (quota low, DB big, suspicious silence).

**Exit criteria:** unattended 24/7 operation; the Mini App opens from any device via the production URL; a broken adapter is noticed via alerts, not silence.

## Backlog (v1.1+, unordered)

TimescaleDB; Redis cache; Amadeus/Duffel backup adapter; weekly trend digest (bot message + Mini App screen); smart suggestions; Itinerary model for virtual-interlining round-trips; Wizz monitoring via chromedp; Mini App extras (calendar heatmap, share-a-fare card).

---

## Estimate

~4–5 weeks part-time to v1.0 (Phases 0–5), +2 days for ops. MVP (Phases 0–3) in ~2 weeks. (The Mini App adds roughly a week over the bot-only plan, most of it in Phases 0–1.)

## Working agreements

- Every phase merges to `master` green (lint + tests + build, Go and web).
- Migrations are append-only from the first deploy.
- Adapter fixtures are recorded from real responses and committed; schema drift must break contract tests.
- No `float64` money in alert comparisons — integer minor units.
- New external calls always get: context deadline, rate limiter, retry policy, `api_call_log` write, metrics.
- API identity comes only from validated initData; every mutation checks ownership. The Mini App validates for UX; the server validates for real.
