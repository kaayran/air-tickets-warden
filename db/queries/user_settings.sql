-- name: GetUserSettings :one
SELECT * FROM user_settings WHERE chat_id = $1;

-- name: EnsureUserSettings :one
-- Lazily creates a default settings row on first access (GET /api/v1/me).
INSERT INTO user_settings (chat_id)
VALUES ($1)
ON CONFLICT (chat_id) DO UPDATE SET chat_id = EXCLUDED.chat_id
RETURNING *;

-- name: UpdateUserSettings :one
-- PATCH /api/v1/me — overwrites the mutable settings columns.
INSERT INTO user_settings (chat_id, cooldown_hours, drop_pct, stable_price_band_pct, updated_at)
VALUES ($1, $2, $3, $4, now())
ON CONFLICT (chat_id) DO UPDATE SET
    cooldown_hours        = EXCLUDED.cooldown_hours,
    drop_pct              = EXCLUDED.drop_pct,
    stable_price_band_pct = EXCLUDED.stable_price_band_pct,
    updated_at            = now()
RETURNING *;
