# Air Tickets Warden

Telegram bot that monitors air ticket prices and sends alerts when prices drop.

Polls Aviasales, Kiwi, and Ryanair in parallel. Tracks price history per route and supports alternative departure airports with ground transfer cost factored in for fair comparison.

**Stack:** Python 3.12, aiogram 3.x, SQLAlchemy 2.x + Alembic, APScheduler, SQLite → PostgreSQL, Docker.

**Design document:** [air-tickets-warden.md](air-tickets-warden.md)

## Status

Phase 0 — bot scaffolding only. All commands are stubs. Real adapters (Aviasales, Kiwi, Ryanair), Scheduler, Aggregator, and Alert Engine arrive in v1.0.

## Run with Docker

```bash
cp .env.example .env  # fill in BOT_TOKEN and ALLOWED_USER_IDS
docker compose -f docker/docker-compose.yml up --build
```

The bot starts polling Telegram and exposes `/health` and `/metrics` on port 9090.

## Development

### One-time setup (macOS)

```bash
# Homebrew (skip if installed)
/bin/bash -c "$(curl -fsSL https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh)"

# uv — manages Python and project dependencies
curl -LsSf https://astral.sh/uv/install.sh | sh
# restart shell or: source ~/.zshrc

# Docker Desktop — needed for `make docker`
brew install --cask docker
```

### Project bootstrap

```bash
git clone <repo> && cd air-tickets-warden

uv python install 3.12
uv sync                           # installs runtime + dev deps into .venv

cp .env.example .env              # fill in BOT_TOKEN, ALLOWED_USER_IDS
make migrate                      # alembic upgrade head
make run                          # start the bot

# In another terminal:
curl -s localhost:9090/health     # {"status":"ok","db":"ok"}
curl -s localhost:9090/metrics    # Prometheus exposition
```

### Telegram setup

1. Talk to [@BotFather](https://t.me/BotFather) → `/newbot` → copy the token to `BOT_TOKEN` in `.env`.
2. Talk to [@userinfobot](https://t.me/userinfobot) → copy your numeric id to `ALLOWED_USER_IDS` (comma-separated for several users).
3. Open your bot, hit **Start**, try `/help`.

### Common commands

| Make target  | What it runs                                                  |
|--------------|---------------------------------------------------------------|
| `make run`   | `uv run python -m warden.main` — starts the bot               |
| `make test`  | `pytest --cov` with 80% threshold                             |
| `make lint`  | `ruff check` + `ruff format --check` + `mypy --strict`        |
| `make fmt`   | `ruff format` + `ruff check --fix`                            |
| `make migrate` | `alembic upgrade head`                                      |
| `make revision m="add foo"` | autogenerate a new migration                   |
| `make docker`| `docker compose up --build`                                   |
| `make clean` | wipe caches and local SQLite                                  |

### Cursor / VS Code

`.vscode/settings.json`, `launch.json`, and `extensions.json` are committed. On first open Cursor will suggest the recommended extensions:

- **Python** + **Pylance**
- **Ruff** — format-on-save and lint fixes
- **Even Better TOML** — for `pyproject.toml`
- **Docker** — for the `docker/` folder

F5 runs the bot with the `.env` loaded. The "pytest: all" launch profile runs the full suite.

### Repository layout

```
src/warden/
  bot/              aiogram handlers, FSM, middlewares
  infrastructure/   db, web (/health, /metrics), telemetry
  config.py         pydantic-settings
  main.py           entrypoint (bot + web + graceful shutdown)
tests/
  unit/ integration/ e2e/
alembic/            migrations
docker/             Dockerfile + docker-compose.yml
```

### Observability

- **Logs** — JSON via `structlog` when `LOG_JSON=true` (default in prod). Each request carries `trace_id`, `user_id`, `command`, `duration_ms`.
- **Metrics** — Prometheus exposition at `GET /metrics`. Counters for updates received, dropped (whitelist), and a `warden_command_duration_seconds` histogram.
- **Health** — `GET /health` returns 200 if DB ping succeeds, 503 otherwise. Hook UptimeRobot to this URL.
- **Errors** — Sentry (set `SENTRY_DSN` in `.env`). No-op when DSN is empty.

## License

MIT
