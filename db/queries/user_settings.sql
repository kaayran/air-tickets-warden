-- name: GetUserSettings :one
SELECT * FROM user_settings WHERE chat_id = $1;

-- name: EnsureUserSettings :one
-- Lazily creates a default settings row on first access (GET /api/v1/me).
INSERT INTO user_settings (chat_id)
VALUES ($1)
ON CONFLICT (chat_id) DO UPDATE SET chat_id = EXCLUDED.chat_id
RETURNING *;

-- name: UpdateUserSettings :one
-- PATCH /api/v1/me — true PATCH semantics: each column is written only when its
-- set_* flag is true (JSON key present); a true flag with a NULL value clears
-- the column back to the cascade default. Flags false → column untouched.
INSERT INTO user_settings (chat_id, cooldown_hours, drop_pct, stable_price_band_pct, updated_at)
VALUES (sqlc.arg(chat_id), sqlc.narg(cooldown_hours), sqlc.narg(drop_pct), sqlc.narg(stable_price_band_pct), now())
ON CONFLICT (chat_id) DO UPDATE SET
    cooldown_hours        = CASE WHEN sqlc.arg(set_cooldown_hours)::bool THEN excluded.cooldown_hours ELSE user_settings.cooldown_hours END,
    drop_pct              = CASE WHEN sqlc.arg(set_drop_pct)::bool THEN excluded.drop_pct ELSE user_settings.drop_pct END,
    stable_price_band_pct = CASE WHEN sqlc.arg(set_stable_price_band_pct)::bool THEN excluded.stable_price_band_pct ELSE user_settings.stable_price_band_pct END,
    updated_at            = now()
RETURNING *;
