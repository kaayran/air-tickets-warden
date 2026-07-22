// Package api serves the Mini App JSON API under /api/v1. Identity comes only
// from validated Telegram initData (never the request body); every request is
// authenticated by the initData middleware and checked against the whitelist.
package api

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/kaayran/air-tickets-warden/internal/storage"
)

// API holds the dependencies shared by the handlers.
type API struct {
	store    *storage.Store
	log      *slog.Logger
	botToken string
	allowed  map[int64]struct{}
}

// New builds the API with the given whitelist.
func New(store *storage.Store, botToken string, allowedUserIDs []int64, log *slog.Logger) *API {
	allowed := make(map[int64]struct{}, len(allowedUserIDs))
	for _, id := range allowedUserIDs {
		allowed[id] = struct{}{}
	}
	return &API{store: store, log: log, botToken: botToken, allowed: allowed}
}

// Handler returns the /api/v1 subtree wrapped in the initData auth middleware.
func (a *API) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/me", a.handleGetMe)
	mux.HandleFunc("PATCH /api/v1/me", a.handlePatchMe)
	return a.authMiddleware(mux)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
