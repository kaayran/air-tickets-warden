package api

import (
	"context"
	"net/http"
	"strings"
	"time"

	initdata "github.com/telegram-mini-apps/init-data-golang"
)

// authScheme is the Authorization header scheme Telegram Mini Apps use:
//
//	Authorization: tma <initData>
const authScheme = "tma"

// initDataMaxAge bounds how stale an initData string may be (replay protection).
const initDataMaxAge = 24 * time.Hour

type ctxKey int

const chatIDKey ctxKey = iota

// authMiddleware validates the Telegram initData on every request: HMAC against
// the bot token, auth_date freshness, then whitelist enforcement. The
// authenticated chat id is stored in the context — the body is never trusted
// for identity.
func (a *API) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, ok := bearerInitData(r.Header.Get("Authorization"))
		if !ok {
			writeError(w, http.StatusUnauthorized, "missing or malformed Authorization header")
			return
		}
		if err := initdata.Validate(raw, a.botToken, initDataMaxAge); err != nil {
			writeError(w, http.StatusUnauthorized, "invalid initData")
			return
		}
		parsed, err := initdata.Parse(raw)
		if err != nil || parsed.User.ID == 0 {
			writeError(w, http.StatusUnauthorized, "initData missing user")
			return
		}
		if _, allowed := a.allowed[parsed.User.ID]; !allowed {
			a.log.Warn("rejected non-whitelisted api user", "user_id", parsed.User.ID)
			writeError(w, http.StatusForbidden, "not authorized")
			return
		}
		ctx := context.WithValue(r.Context(), chatIDKey, parsed.User.ID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// bearerInitData extracts the raw initData from a "tma <initData>" header.
func bearerInitData(header string) (string, bool) {
	scheme, value, found := strings.Cut(header, " ")
	if !found || !strings.EqualFold(scheme, authScheme) || value == "" {
		return "", false
	}
	return value, true
}

// chatIDFromContext returns the authenticated chat id set by authMiddleware.
func chatIDFromContext(ctx context.Context) (int64, bool) {
	id, ok := ctx.Value(chatIDKey).(int64)
	return id, ok
}
