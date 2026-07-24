// Package domain holds the pure business types and rules: no I/O, no storage,
// no HTTP. Outer layers (api, storage, services) depend on this package, never
// the other way around; the subscriptions schema (migration 0002) is derived
// from these types.
package domain

import "time"

// Status is the subscription lifecycle state. Paused subscriptions are not
// checked by the scheduler; archived ones are hidden from the default list but
// keep their history.
type Status string

const (
	StatusActive   Status = "active"
	StatusPaused   Status = "paused"
	StatusArchived Status = "archived"
)

// ValidStatus reports whether s is a known lifecycle state.
func ValidStatus(s Status) bool {
	switch s {
	case StatusActive, StatusPaused, StatusArchived:
		return true
	}
	return false
}

// AlertStrategy names the rule the Alert Engine applies to decide whether a
// price is worth a notification. Only the MVP strategies exist for now; the
// v1.0 set (relative_drop, sudden_drop, combined) is added when implemented.
type AlertStrategy string

const (
	// StrategyAbsoluteThreshold alerts when the price drops below MaxPriceMinor.
	StrategyAbsoluteThreshold AlertStrategy = "absolute_threshold"
	// StrategyHistoricalMinimum alerts when the price undercuts the observed
	// historical minimum (parameters resolve through the settings cascade).
	StrategyHistoricalMinimum AlertStrategy = "historical_minimum"
)

// ValidAlertStrategy reports whether s is a known strategy.
func ValidAlertStrategy(s AlertStrategy) bool {
	switch s {
	case StrategyAbsoluteThreshold, StrategyHistoricalMinimum:
		return true
	}
	return false
}

// Subscription is a monitoring rule: where from, where to, when, and what
// price movement deserves a notification.
//
// Money is always integer EUR minor units (cents) — never floats. Date fields
// (DateFrom/DateTo/Return*) carry date-only semantics: UTC midnight of the
// civil date; the time-of-day component is meaningless and must be zero.
// Optional fields are pointers: nil means "not set", which for the alert
// parameters (CooldownHours, DropPct, StablePriceBandPct) means "resolve
// through the cascade" (subscription → user settings → env defaults).
type Subscription struct {
	ID         string // UUID assigned by storage; empty until first persisted
	UserChatID int64

	Origin             string   // IATA code of the primary departure airport
	OriginAlternatives []string // optional alternative departure airports
	Destinations       []string // one or more IATA codes (e.g. BCN, MAD, VLC)

	DateFrom       time.Time // allowed departure date range (inclusive)
	DateTo         time.Time
	ReturnDateFrom *time.Time // both set → round-trip with a return window
	ReturnDateTo   *time.Time
	TripLengthMin  *int32 // trip length bounds in days (round-trip only)
	TripLengthMax  *int32

	MaxPriceMinor      *int64 // absolute price ceiling, EUR minor units
	MaxStops           *int32
	MaxDurationMinutes *int32
	AirlinesWhitelist  []string // two-letter IATA carrier codes
	AirlinesBlacklist  []string

	AlertStrategy      AlertStrategy
	CooldownHours      *int32   // nil → cascade
	DropPct            *float64 // nil → cascade; dimensionless ratio, not money
	StablePriceBandPct *float64 // nil → cascade

	MutedUntil  *time.Time // notifications suppressed until then; checks continue
	Status      Status
	NextCheckAt time.Time // drives the DB scheduler (Phase 3)

	CreatedAt time.Time
	UpdatedAt time.Time
}

// IsMuted reports whether notifications are currently suppressed. Monitoring
// and history collection continue while muted.
func (s *Subscription) IsMuted(now time.Time) bool {
	return s.MutedUntil != nil && now.Before(*s.MutedUntil)
}

// RoundTrip reports whether the subscription describes a round-trip search
// (an explicit return window and/or trip-length bounds).
func (s *Subscription) RoundTrip() bool {
	return s.ReturnDateFrom != nil || s.ReturnDateTo != nil ||
		s.TripLengthMin != nil || s.TripLengthMax != nil
}
