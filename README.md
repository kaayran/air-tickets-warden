# Air Tickets Warden

Air ticket price monitor living inside Telegram: a **Mini App** for managing subscriptions and viewing price history, a **bot** that delivers price-drop alerts, and a Go backend that polls live flight-search sources (Amadeus, Ryanair, more later) in parallel.

Tracks price history per route and supports alternative departure airports with ground transfer cost factored in for fair comparison.

**Stack:** Go 1.24+, PostgreSQL 16 (pgx v5 + sqlc + goose), [go-telegram/bot](https://github.com/go-telegram/bot); frontend — React 19 + TypeScript + Vite ([@telegram-apps/sdk-react](https://github.com/Telegram-Mini-Apps/telegram-apps), telegram-ui) embedded into the Go binary; Caddy for TLS; Prometheus, Sentry, Docker.

**Design document:** [air-tickets-warden.md](air-tickets-warden.md) · **Development plan:** [PLAN.md](PLAN.md)

## Status

Design stage (v0.7 — live-search sources, alert hardening). The previous Python implementation was removed; it is available in git history up to commit `bbe5ceb`. Implementation follows [PLAN.md](PLAN.md), starting with Phase 0.

## Architecture in one paragraph

The Mini App (React SPA) is the only management UI — Telegram opens it over HTTPS and injects signed `initData`, which the backend validates (HMAC on the bot token) to authenticate every `/api/v1` request. The bot keeps just `/start` (with an "Open App" button), `/help`, and alert delivery — a Mini App can't push anything while closed, so notifications always arrive as bot messages. The Go binary serves the SPA (via `go:embed`), the JSON API, `/health`, `/metrics`, runs the scheduler and adapters; Postgres holds everything; Caddy terminates TLS.

## Run with Docker

```bash
cp .env.example .env  # fill in BOT_TOKEN, ALLOWED_USER_IDS, PUBLIC_URL
docker compose -f docker/docker-compose.yml up --build
```

Compose starts Postgres, the app, and Caddy. The app applies migrations, starts polling Telegram, and serves the Mini App + API behind Caddy; `/health` and `/metrics` are on port 9090.

## Development

### Prerequisites

- Go 1.24+ — https://go.dev/dl/
- Node.js 22+ (frontend build)
- Docker (Postgres, container build)
- Tools (installed via `go install`, pinned in the Makefile): `sqlc`, `goose`, `golangci-lint`
- A tunnel for local Mini App testing: [cloudflared](https://github.com/cloudflare/cloudflared) or ngrok — Telegram can only open the Mini App from a public HTTPS URL

### Project bootstrap

```bash
git clone <repo> && cd air-tickets-warden

cp .env.example .env              # fill in BOT_TOKEN, ALLOWED_USER_IDS
docker compose -f docker/docker-compose.yml up -d postgres

make migrate                      # goose up
make web-build                    # vite build → web/dist (embedded on next go build)
make run                          # start the backend + bot

# Frontend dev loop (hot reload, proxies /api to the Go server):
cd web && npm run dev

# Expose to Telegram for on-device testing:
make tunnel                       # cloudflared → prints an HTTPS URL; set it as PUBLIC_URL
```

### Telegram setup

1. [@BotFather](https://t.me/BotFather) → `/newbot` → copy the token to `BOT_TOKEN` in `.env`.
2. [@userinfobot](https://t.me/userinfobot) → copy your numeric id to `ALLOWED_USER_IDS` (comma-separated for several users).
3. BotFather → `/newapp` (or Bot Settings → Configure Mini App) → set the Mini App URL to your `PUBLIC_URL` (the tunnel URL during development).
4. Open your bot, hit **Start** → **Open App**.

### Common commands

| Make target | What it runs |
|-------------|--------------|
| `make run` | `go run ./cmd/warden` — backend + bot |
| `make test` | `go test -short ./...` with coverage |
| `make test-integration` | full tests incl. testcontainers Postgres |
| `make lint` | `golangci-lint run` + `go vet ./...` + `npm run lint` |
| `make fmt` | `gofumpt -w .` |
| `make web-build` | `npm run build` in `web/` → `web/dist` |
| `make sqlc` | regenerate query code from `db/queries/` |
| `make migrate` | `goose up` against `DATABASE_URL` |
| `make tunnel` | cloudflared tunnel to the local server |
| `make docker` | `docker compose up --build` |

### Repository layout

```
cmd/warden/         entrypoint (run / migrate / replay subcommands)
internal/
  bot/              /start, /help, alert delivery, callbacks, middlewares
  api/              /api/v1 handlers, initData auth middleware
  domain/           Subscription, Flight, alert strategies (pure)
  adapters/         amadeus, ryanair, ... + resilient HTTP client, registry
  services/         aggregator, alert engine, fx, expander, airports
  scheduler/        DB-driven ticker loop
  storage/          pgx pool, sqlc code, goose runner
  cache/            in-memory TTL cache
  telemetry/        slog, prometheus, sentry
  config/           env config
  web/              HTTP server: embedded SPA, /health, /metrics
web/                React Mini App (src/, vite.config.ts; dist/ is gitignored)
db/migrations/      goose SQL (embedded)
db/queries/         sqlc SQL
docker/             Dockerfile + docker-compose.yml + Caddyfile
```

### Observability

- **Logs** — JSON via `slog`. Each check cycle carries a `trace_id`.
- **Metrics** — Prometheus at `GET /metrics` (adapter latency/status, alerts sent/suppressed, active subscriptions).
- **Health** — `GET /health` returns 200 if the DB ping succeeds, 503 otherwise. Hook UptimeRobot to this URL. The Mini App shows a human-readable health panel from `/api/v1/health-summary`.
- **Errors** — Sentry (set `SENTRY_DSN` in `.env`). No-op when DSN is empty.

## License

MIT
