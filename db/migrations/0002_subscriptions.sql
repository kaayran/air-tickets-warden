-- +goose Up
-- Migration 0002: subscriptions, derived from internal/domain.Subscription
-- (schema follows domain, never the other way around).
--
--   * money is bigint EUR minor units (max_price_minor <-> MaxPriceMinor
--     int64) — never numeric/float; drop_pct and stable_price_band_pct are
--     dimensionless ratios, so float is fine there;
--   * IATA lists are text[] mirroring the domain's []string — no jsonb
--     indirection for plain string lists;
--   * nullable alert parameters (cooldown_hours, drop_pct,
--     stable_price_band_pct) mean "resolve through the cascade
--     subscription -> user_settings -> env";
--   * alert_strategy has no CHECK: the domain validates the enum, and the
--     strategy set grows in later phases without a schema change. status is
--     CHECKed — the lifecycle is stable;
--   * next_check_at drives the Phase 3 DB scheduler; indexed for the
--     "select due" query, present from birth so scheduling needs no backfill.

CREATE TABLE subscriptions (
    id                    uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_chat_id          bigint NOT NULL,

    origin                text NOT NULL,
    origin_alternatives   text[] NOT NULL DEFAULT '{}',
    destinations          text[] NOT NULL,

    date_from             date NOT NULL,
    date_to               date NOT NULL,
    return_date_from      date,
    return_date_to        date,
    trip_length_min       int,
    trip_length_max       int,

    max_price_minor       bigint,
    max_stops             int,
    max_duration_minutes  int,
    airlines_whitelist    text[] NOT NULL DEFAULT '{}',
    airlines_blacklist    text[] NOT NULL DEFAULT '{}',

    alert_strategy        text NOT NULL DEFAULT 'absolute_threshold',
    cooldown_hours        int,
    drop_pct              double precision,
    stable_price_band_pct double precision,

    muted_until           timestamptz,
    status                text NOT NULL DEFAULT 'active'
                          CHECK (status IN ('active', 'paused', 'archived')),
    next_check_at         timestamptz NOT NULL DEFAULT now(),

    created_at            timestamptz NOT NULL DEFAULT now(),
    updated_at            timestamptz NOT NULL DEFAULT now()
);

-- Every API query is scoped by user_chat_id (ownership); list is newest-first.
CREATE INDEX subscriptions_user_chat_id_idx
    ON subscriptions (user_chat_id, created_at DESC);

-- The scheduler's "select due" scan: WHERE next_check_at <= now() AND active.
CREATE INDEX subscriptions_due_idx
    ON subscriptions (next_check_at)
    WHERE status = 'active';

-- +goose Down
DROP TABLE subscriptions;
