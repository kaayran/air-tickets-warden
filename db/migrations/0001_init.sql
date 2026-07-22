-- +goose Up
-- Migration 0001: core subscription + per-user settings tables.
-- Migrations are append-only from the first deploy (see PLAN.md working agreements).

CREATE TABLE subscriptions (
    id                   uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_chat_id         bigint      NOT NULL,
    origin               text        NOT NULL,
    origin_alternatives  jsonb       NOT NULL DEFAULT '[]'::jsonb,
    destination          jsonb       NOT NULL,                       -- list of IATA codes
    date_from            date        NOT NULL,
    date_to              date        NOT NULL,
    return_date_from     date,
    return_date_to       date,
    trip_length_min      int,
    trip_length_max      int,
    max_price            numeric,
    max_stops            int,
    max_duration_minutes int,
    airlines_whitelist   jsonb       NOT NULL DEFAULT '[]'::jsonb,
    airlines_blacklist   jsonb       NOT NULL DEFAULT '[]'::jsonb,
    alert_strategy       text        NOT NULL,
    alert_params         jsonb       NOT NULL DEFAULT '{}'::jsonb,
    cooldown_hours       int,                                        -- null → user_settings → env default
    dry_run              boolean     NOT NULL DEFAULT false,
    muted_until          timestamptz,                                -- notifications suppressed; monitoring continues
    status               text        NOT NULL DEFAULT 'active',      -- active | paused | archived
    next_check_at        timestamptz NOT NULL DEFAULT now(),         -- drives the scheduler
    created_at           timestamptz NOT NULL DEFAULT now(),
    updated_at           timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX idx_subscriptions_user_chat_id ON subscriptions (user_chat_id);
CREATE INDEX idx_subscriptions_next_check_at ON subscriptions (next_check_at) WHERE status = 'active';

CREATE TABLE user_settings (
    chat_id               bigint PRIMARY KEY,
    cooldown_hours        int,
    drop_pct              double precision,   -- ratio (e.g. 0.25); not money, float is fine
    stable_price_band_pct double precision,   -- ratio (e.g. 0.02)
    updated_at            timestamptz NOT NULL DEFAULT now()
);

-- +goose Down
DROP TABLE user_settings;
DROP TABLE subscriptions;
