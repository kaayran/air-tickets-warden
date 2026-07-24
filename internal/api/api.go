// Package api serves the Mini App JSON API under /api/v1. Identity comes only
// from validated Telegram initData (never the request body); every request is
// authenticated by the initData middleware and checked against the whitelist.
package api

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/kaayran/air-tickets-warden/internal/domain"
	"github.com/kaayran/air-tickets-warden/internal/services/airports"
	"github.com/kaayran/air-tickets-warden/internal/storage"
)

// SubscriptionStore is what the handlers need from the subscription manager.
// An interface so handler tests can run against an in-memory fake; the
// production implementation is services/subscriptions.Manager, whose scoping
// contract (foreign/unknown id -> ErrNotFound) the handlers rely on for 404s.
type SubscriptionStore interface {
	Create(ctx context.Context, sub domain.Subscription) (domain.Subscription, error)
	List(ctx context.Context, chatID int64) ([]domain.Subscription, error)
	Get(ctx context.Context, chatID int64, id string) (domain.Subscription, error)
	Update(ctx context.Context, sub domain.Subscription) (domain.Subscription, error)
	Delete(ctx context.Context, chatID int64, id string) error
}

// API holds the dependencies shared by the handlers.
type API struct {
	store    *storage.Store
	subs     SubscriptionStore
	airports *airports.Service
	log      *slog.Logger
	botToken string
	allowed  map[int64]struct{}
}

// New builds the API with the given whitelist.
func New(store *storage.Store, subs SubscriptionStore, air *airports.Service, botToken string, allowedUserIDs []int64, log *slog.Logger) *API {
	allowed := make(map[int64]struct{}, len(allowedUserIDs))
	for _, id := range allowedUserIDs {
		allowed[id] = struct{}{}
	}
	return &API{store: store, subs: subs, airports: air, log: log, botToken: botToken, allowed: allowed}
}

// Handler returns the /api/v1 subtree wrapped in the initData auth middleware.
func (a *API) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/me", a.handleGetMe)
	mux.HandleFunc("PATCH /api/v1/me", a.handlePatchMe)
	mux.HandleFunc("GET /api/v1/subscriptions", a.handleListSubscriptions)
	mux.HandleFunc("POST /api/v1/subscriptions", a.handleCreateSubscription)
	mux.HandleFunc("GET /api/v1/subscriptions/{id}", a.handleGetSubscription)
	mux.HandleFunc("PATCH /api/v1/subscriptions/{id}", a.handlePatchSubscription)
	mux.HandleFunc("DELETE /api/v1/subscriptions/{id}", a.handleDeleteSubscription)
	mux.HandleFunc("GET /api/v1/airports", a.handleSearchAirports)
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

// writeValidationError sends the field-level failures so the form can
// highlight the offending inputs.
func writeValidationError(w http.ResponseWriter, verr domain.ValidationError) {
	writeJSON(w, http.StatusBadRequest, map[string]any{
		"error":  "validation failed",
		"fields": verr,
	})
}
