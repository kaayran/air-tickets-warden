// Package subscriptions is the CRUD layer over monitoring rules. Every
// operation is scoped by the owner's chat id — a foreign or unknown
// subscription id is indistinguishable from a missing one (ErrNotFound), which
// the API surfaces as 404, never 403.
package subscriptions

import (
	"context"
	"errors"
	"fmt"
	"regexp"

	"github.com/jackc/pgx/v5"

	"github.com/kaayran/air-tickets-warden/internal/domain"
	"github.com/kaayran/air-tickets-warden/internal/storage/sqlcgen"
)

// ErrNotFound covers both "no such subscription" and "not yours".
var ErrNotFound = errors.New("subscription not found")

// uuidRe pre-screens ids so a malformed path segment reads as "not found"
// instead of surfacing a pgx uuid-encoding error as a 500.
var uuidRe = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// Manager persists domain.Subscription rows through the sqlc query layer.
type Manager struct {
	q *sqlcgen.Queries
}

func New(q *sqlcgen.Queries) *Manager { return &Manager{q: q} }

// Create persists a new subscription. ID, timestamps, and next_check_at come
// from column defaults; the caller is responsible for prior validation.
func (m *Manager) Create(ctx context.Context, sub domain.Subscription) (domain.Subscription, error) {
	row, err := m.q.CreateSubscription(ctx, sqlcgen.CreateSubscriptionParams{
		UserChatID:         sub.UserChatID,
		Origin:             sub.Origin,
		OriginAlternatives: emptyNotNil(sub.OriginAlternatives),
		Destinations:       sub.Destinations,
		DateFrom:           sub.DateFrom,
		DateTo:             sub.DateTo,
		ReturnDateFrom:     sub.ReturnDateFrom,
		ReturnDateTo:       sub.ReturnDateTo,
		TripLengthMin:      sub.TripLengthMin,
		TripLengthMax:      sub.TripLengthMax,
		MaxPriceMinor:      sub.MaxPriceMinor,
		MaxStops:           sub.MaxStops,
		MaxDurationMinutes: sub.MaxDurationMinutes,
		AirlinesWhitelist:  emptyNotNil(sub.AirlinesWhitelist),
		AirlinesBlacklist:  emptyNotNil(sub.AirlinesBlacklist),
		AlertStrategy:      string(sub.AlertStrategy),
		CooldownHours:      sub.CooldownHours,
		DropPct:            sub.DropPct,
		StablePriceBandPct: sub.StablePriceBandPct,
		MutedUntil:         sub.MutedUntil,
		Status:             string(sub.Status),
	})
	if err != nil {
		return domain.Subscription{}, fmt.Errorf("create subscription: %w", err)
	}
	return toDomain(row), nil
}

// List returns the owner's subscriptions, newest first.
func (m *Manager) List(ctx context.Context, chatID int64) ([]domain.Subscription, error) {
	rows, err := m.q.ListSubscriptions(ctx, chatID)
	if err != nil {
		return nil, fmt.Errorf("list subscriptions: %w", err)
	}
	subs := make([]domain.Subscription, len(rows))
	for i, row := range rows {
		subs[i] = toDomain(row)
	}
	return subs, nil
}

// Get returns one subscription owned by chatID, or ErrNotFound.
func (m *Manager) Get(ctx context.Context, chatID int64, id string) (domain.Subscription, error) {
	if !uuidRe.MatchString(id) {
		return domain.Subscription{}, ErrNotFound
	}
	row, err := m.q.GetSubscription(ctx, sqlcgen.GetSubscriptionParams{ID: id, UserChatID: chatID})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Subscription{}, ErrNotFound
	}
	if err != nil {
		return domain.Subscription{}, fmt.Errorf("get subscription: %w", err)
	}
	return toDomain(row), nil
}

// Update writes the full mutable state of sub (PATCH merging happens in the
// API layer before this call), keyed and scoped by sub.ID + sub.UserChatID.
func (m *Manager) Update(ctx context.Context, sub domain.Subscription) (domain.Subscription, error) {
	if !uuidRe.MatchString(sub.ID) {
		return domain.Subscription{}, ErrNotFound
	}
	row, err := m.q.UpdateSubscription(ctx, sqlcgen.UpdateSubscriptionParams{
		ID:                 sub.ID,
		UserChatID:         sub.UserChatID,
		Origin:             sub.Origin,
		OriginAlternatives: emptyNotNil(sub.OriginAlternatives),
		Destinations:       sub.Destinations,
		DateFrom:           sub.DateFrom,
		DateTo:             sub.DateTo,
		ReturnDateFrom:     sub.ReturnDateFrom,
		ReturnDateTo:       sub.ReturnDateTo,
		TripLengthMin:      sub.TripLengthMin,
		TripLengthMax:      sub.TripLengthMax,
		MaxPriceMinor:      sub.MaxPriceMinor,
		MaxStops:           sub.MaxStops,
		MaxDurationMinutes: sub.MaxDurationMinutes,
		AirlinesWhitelist:  emptyNotNil(sub.AirlinesWhitelist),
		AirlinesBlacklist:  emptyNotNil(sub.AirlinesBlacklist),
		AlertStrategy:      string(sub.AlertStrategy),
		CooldownHours:      sub.CooldownHours,
		DropPct:            sub.DropPct,
		StablePriceBandPct: sub.StablePriceBandPct,
		MutedUntil:         sub.MutedUntil,
		Status:             string(sub.Status),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Subscription{}, ErrNotFound
	}
	if err != nil {
		return domain.Subscription{}, fmt.Errorf("update subscription: %w", err)
	}
	return toDomain(row), nil
}

// Delete removes the subscription, or reports ErrNotFound.
func (m *Manager) Delete(ctx context.Context, chatID int64, id string) error {
	if !uuidRe.MatchString(id) {
		return ErrNotFound
	}
	n, err := m.q.DeleteSubscription(ctx, sqlcgen.DeleteSubscriptionParams{ID: id, UserChatID: chatID})
	if err != nil {
		return fmt.Errorf("delete subscription: %w", err)
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// toDomain is a plain field copy — the sqlc type overrides keep both sides
// shape-identical.
func toDomain(row sqlcgen.Subscription) domain.Subscription {
	return domain.Subscription{
		ID:                 row.ID,
		UserChatID:         row.UserChatID,
		Origin:             row.Origin,
		OriginAlternatives: row.OriginAlternatives,
		Destinations:       row.Destinations,
		DateFrom:           row.DateFrom,
		DateTo:             row.DateTo,
		ReturnDateFrom:     row.ReturnDateFrom,
		ReturnDateTo:       row.ReturnDateTo,
		TripLengthMin:      row.TripLengthMin,
		TripLengthMax:      row.TripLengthMax,
		MaxPriceMinor:      row.MaxPriceMinor,
		MaxStops:           row.MaxStops,
		MaxDurationMinutes: row.MaxDurationMinutes,
		AirlinesWhitelist:  row.AirlinesWhitelist,
		AirlinesBlacklist:  row.AirlinesBlacklist,
		AlertStrategy:      domain.AlertStrategy(row.AlertStrategy),
		CooldownHours:      row.CooldownHours,
		DropPct:            row.DropPct,
		StablePriceBandPct: row.StablePriceBandPct,
		MutedUntil:         row.MutedUntil,
		Status:             domain.Status(row.Status),
		NextCheckAt:        row.NextCheckAt,
		CreatedAt:          row.CreatedAt,
		UpdatedAt:          row.UpdatedAt,
	}
}

// emptyNotNil keeps NOT NULL text[] columns happy when the domain slice is nil.
func emptyNotNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}
