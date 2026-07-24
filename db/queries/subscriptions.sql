-- Every query is scoped by user_chat_id: identity comes only from the
-- initData middleware, and a foreign id must behave exactly like a missing
-- one (the API returns 404, never 403 — no existence oracle).

-- name: CreateSubscription :one
INSERT INTO subscriptions (
    user_chat_id,
    origin, origin_alternatives, destinations,
    date_from, date_to, return_date_from, return_date_to,
    trip_length_min, trip_length_max,
    max_price_minor, max_stops, max_duration_minutes,
    airlines_whitelist, airlines_blacklist,
    alert_strategy, cooldown_hours, drop_pct, stable_price_band_pct,
    muted_until, status
) VALUES (
    $1,
    $2, $3, $4,
    $5, $6, $7, $8,
    $9, $10,
    $11, $12, $13,
    $14, $15,
    $16, $17, $18, $19,
    $20, $21
)
RETURNING *;

-- name: ListSubscriptions :many
SELECT * FROM subscriptions
WHERE user_chat_id = $1
ORDER BY created_at DESC;

-- name: GetSubscription :one
SELECT * FROM subscriptions
WHERE id = $1 AND user_chat_id = $2;

-- name: UpdateSubscription :one
-- Full-row update: PATCH merging happens in Go (read -> apply Optional fields
-- -> validate -> write), so the query stays a plain SET list instead of the
-- set_* flag forest a 20-column partial update would need.
UPDATE subscriptions SET
    origin                = $3,
    origin_alternatives   = $4,
    destinations          = $5,
    date_from             = $6,
    date_to               = $7,
    return_date_from      = $8,
    return_date_to        = $9,
    trip_length_min       = $10,
    trip_length_max       = $11,
    max_price_minor       = $12,
    max_stops             = $13,
    max_duration_minutes  = $14,
    airlines_whitelist    = $15,
    airlines_blacklist    = $16,
    alert_strategy        = $17,
    cooldown_hours        = $18,
    drop_pct              = $19,
    stable_price_band_pct = $20,
    muted_until           = $21,
    status                = $22,
    updated_at            = now()
WHERE id = $1 AND user_chat_id = $2
RETURNING *;

-- name: DeleteSubscription :execrows
DELETE FROM subscriptions
WHERE id = $1 AND user_chat_id = $2;
