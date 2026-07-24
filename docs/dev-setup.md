# Dev setup

How to run Air Tickets Warden locally and test the Mini App on your phone.

## Prerequisites

| Tool | Why | Install |
|------|-----|---------|
| Go 1.24+ | the service binary | `brew install go` |
| Node 20+ | Vite build of the Mini App | `brew install node` |
| Docker | local Postgres (compose) | Docker Desktop |
| `cloudflared` | HTTPS tunnel so Telegram can reach the local app | `brew install cloudflared` |

`goose` is used as a **library** (migrations embedded via `go:embed`), so no CLI is needed.
`sqlc` needs no install either: `make sqlc` runs a pinned version via `go run`
(`SQLC_VERSION` in the Makefile — the same pin CI uses for the drift check).

## Environment

Copy `.env.example` → `.env` and fill in:

- `BOT_TOKEN` — from @BotFather.
- `ALLOWED_USER_IDS` — comma-separated numeric chat ids allowed to use the bot/API
  (your own id for now; add test ids later — no code change).
- `DATABASE_URL` — matches the compose Postgres: `postgres://warden:warden@localhost:5432/warden?sslmode=disable`.
- `PUBLIC_URL` — set automatically by `scripts/dev.sh` from the tunnel; leave blank otherwise.
- `SENTRY_DSN` — **leave empty** unless you have a real DSN. A malformed value aborts
  startup with `DsnParseError: invalid scheme`.

Amadeus / Kiwi keys are not required until Phase 2.

## Why the bot is unavoidable

The bot is not just an entry button — it is the **identity provider**. Telegram injects a
signed `initData` string into the Mini App WebView; its HMAC is keyed by the bot token, and
the server validates it on every `/api/v1` request (`Authorization: tma <initData>`). There is
no valid `initData` outside Telegram, so the whole app runs behind the bot's trust chain.

The bot uses **long polling**, so it needs no tunnel — only the Mini App does.

## Run it (one command)

```sh
make dev        # = scripts/dev.sh
```

This starts Postgres, rebuilds `web/dist` (always — so the phone never sees a stale
frontend), opens a cloudflared quick tunnel, and runs
the app with `PUBLIC_URL` set to the tunnel URL. Then in Telegram:

1. Open your bot, send `/start`.
2. Tap **Open App** (or the chat menu button).
3. The Mini App opens and shows your settings from `GET /api/v1/me`.

The tunnel URL is **ephemeral** — it changes each run. That's fine: the `/start` `web_app`
button reads `PUBLIC_URL` at runtime, so a restart is enough; **no BotFather edits**. (We
deliberately don't use the direct `t.me/<bot>/<app>` link in dev, which would need re-registration.)

## Run the pieces manually

```sh
make db-up                    # Postgres only
make migrate                  # apply migrations (also runs automatically on `warden run`)
make web-build                # build the Mini App into web/dist
make tunnel                   # print a trycloudflare.com URL
PUBLIC_URL=<that-url> make run
```

## Frontend dev server (fast UI iteration)

For quick UI work without rebuilding the binary:

```sh
cd web && npm run dev         # Vite on :5173, proxies /api → :8080
```

Note: opening `:5173` in a plain browser has **no** `initData`, so the app shows a sign-in
hint — that's expected. Full auth only works inside Telegram via the tunnel.

## Useful commands

```sh
make test        # go test -short ./... + web tests
make sqlc        # regenerate internal/storage/sqlcgen from db/queries
make fmt         # gofmt + go mod tidy
docker compose exec postgres psql -U warden -d warden   # inspect the DB
```

## Full stack in Docker

`make dev` is the everyday loop. To run the production-shaped stack (embedded app behind a
Caddy TLS sidecar):

```sh
docker compose up --build         # app + postgres + caddy
```

Caddy serves `localhost` with an internal self-signed cert by default; set `WARDEN_DOMAIN` to a
real hostname in prod for automatic Let's Encrypt. The multi-stage `Dockerfile` builds the Mini
App (node) then the Go binary (with `web/dist` embedded) into a distroless image.

Note: with the defaults (`PUBLIC_URL=https://localhost`) the bot's button opens a URL your
phone cannot reach — the compose stack is only phone-testable with a real domain
(`WARDEN_DOMAIN` + `PUBLIC_URL`). For phone testing in dev, use `make dev`.

## Read-only DB access for the AI assistant

The AI coding assistant inspects the **dev** schema and data through plain `psql` under a
`SELECT`-only role — the security boundary is the role's server-side grants, no extra
tooling needed. (An MCP server was considered and dropped: the reference
`@modelcontextprotocol/server-postgres` package is archived, and `psql` does the same job.)
**Local dev only — never production.**

One-time role setup against the dev database (the compose Postgres):

```sql
CREATE ROLE warden_ro LOGIN PASSWORD 'warden_ro';
GRANT CONNECT ON DATABASE warden TO warden_ro;
GRANT USAGE ON SCHEMA public TO warden_ro;
GRANT SELECT ON ALL TABLES IN SCHEMA public TO warden_ro;
ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT SELECT ON TABLES TO warden_ro;
```

Query path (works without psql on the host — uses the container's local socket):

```sh
docker compose exec postgres psql -U warden_ro -d warden -c '\d'
```

With psql installed on the host, `psql "$WARDEN_RO_DATABASE_URL"` works too (see
`.env.example`). Schema changes always go through goose migrations under review, never
through this connection.
