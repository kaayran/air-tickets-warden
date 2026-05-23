# Air Tickets Warden

Telegram bot that monitors air ticket prices and sends alerts when prices drop.

Polls Aviasales, Kiwi, and Ryanair in parallel. Tracks price history per route and supports alternative departure airports with ground transfer cost factored in for fair comparison.

**Stack:** Python 3.12, aiogram 3.x, SQLAlchemy 2.x + Alembic, APScheduler, SQLite → PostgreSQL, Docker.

**Design document:** [air-tickets-warden.md](air-tickets-warden.md)

## Run

```bash
cp .env.example .env  # fill in bot token and API keys
docker compose up --build
```
