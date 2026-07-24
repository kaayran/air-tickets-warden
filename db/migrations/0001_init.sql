-- +goose Up
-- Migration 0001: per-user settings (the only table Phase 0 uses).
-- The subscriptions schema is deliberately NOT here: it is born in Phase 1
-- (migration 0002), derived from the domain types — money columns are
-- bigint EUR minor units, never numeric/float (see PLAN.md working agreements).
-- Migrations become append-only from the first production deploy.

CREATE TABLE user_settings (
    chat_id               bigint PRIMARY KEY,
    cooldown_hours        int,
    drop_pct              double precision,   -- ratio (e.g. 0.25); not money, float is fine
    stable_price_band_pct double precision,   -- ratio (e.g. 0.02)
    updated_at            timestamptz NOT NULL DEFAULT now()
);

-- +goose Down
DROP TABLE user_settings;
