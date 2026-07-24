package domain

import (
	"fmt"
	"regexp"
	"strings"
	"time"
)

// AirportLookup answers whether an IATA code names a real airport. The
// airports service implements it; the domain only declares the need so it
// stays free of the dataset.
type AirportLookup interface {
	Exists(iata string) bool
}

// FieldError pins a validation failure to the field that caused it, so the API
// can return actionable 400 bodies and the form can highlight the input.
type FieldError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

func (e FieldError) Error() string { return e.Field + ": " + e.Message }

// ValidationError is the full set of field failures from one validation pass.
type ValidationError []FieldError

func (v ValidationError) Error() string {
	msgs := make([]string, len(v))
	for i, e := range v {
		msgs[i] = e.Error()
	}
	return "invalid subscription: " + strings.Join(msgs, "; ")
}

// Caps keep a single subscription from fanning out into an unbounded number of
// adapter queries (each origin×destination pair is a separate search).
const (
	maxOriginAlternatives = 5
	maxDestinations       = 10
	maxAirlineFilters     = 20
)

var (
	airportCodeRe = regexp.MustCompile(`^[A-Z]{3}$`)
	carrierCodeRe = regexp.MustCompile(`^[A-Z0-9]{2}$`)
)

// Validate checks the structural rules: IATA codes against the dataset, range
// consistency, enums, parameter bounds. It is time-independent — an existing
// subscription whose travel window has already passed still validates, so
// editing or pausing old subscriptions never trips on stale dates. Create-time
// freshness is ValidateFresh.
func (s *Subscription) Validate(airports AirportLookup) error {
	var errs ValidationError
	add := func(field, format string, args ...any) {
		errs = append(errs, FieldError{Field: field, Message: fmt.Sprintf(format, args...)})
	}

	checkAirport := func(field, code string) {
		switch {
		case !airportCodeRe.MatchString(code):
			add(field, "%q is not a valid IATA airport code", code)
		case !airports.Exists(code):
			add(field, "unknown airport %q", code)
		}
	}

	checkAirport("origin", s.Origin)
	if len(s.OriginAlternatives) > maxOriginAlternatives {
		add("origin_alternatives", "at most %d alternative airports", maxOriginAlternatives)
	}
	seenAlt := map[string]bool{s.Origin: true}
	for _, code := range s.OriginAlternatives {
		checkAirport("origin_alternatives", code)
		if seenAlt[code] {
			add("origin_alternatives", "duplicate airport %q", code)
		}
		seenAlt[code] = true
	}

	switch {
	case len(s.Destinations) == 0:
		add("destinations", "at least one destination is required")
	case len(s.Destinations) > maxDestinations:
		add("destinations", "at most %d destinations", maxDestinations)
	}
	seenDst := map[string]bool{}
	for _, code := range s.Destinations {
		checkAirport("destinations", code)
		if code == s.Origin {
			add("destinations", "destination equals origin %q", code)
		}
		if seenDst[code] {
			add("destinations", "duplicate destination %q", code)
		}
		seenDst[code] = true
	}

	switch {
	case s.DateFrom.IsZero() || s.DateTo.IsZero():
		add("date_from", "departure date range is required")
	case s.DateTo.Before(s.DateFrom):
		add("date_to", "date_to is before date_from")
	}

	switch {
	case (s.ReturnDateFrom == nil) != (s.ReturnDateTo == nil):
		add("return_date_from", "return window needs both return_date_from and return_date_to")
	case s.ReturnDateFrom != nil:
		if s.ReturnDateTo.Before(*s.ReturnDateFrom) {
			add("return_date_to", "return_date_to is before return_date_from")
		}
		if s.ReturnDateFrom.Before(s.DateFrom) {
			add("return_date_from", "return window starts before the departure window")
		}
	}

	if s.TripLengthMin != nil && *s.TripLengthMin < 1 {
		add("trip_length_min", "trip length must be at least 1 day")
	}
	if s.TripLengthMax != nil {
		if *s.TripLengthMax < 1 {
			add("trip_length_max", "trip length must be at least 1 day")
		}
		if s.TripLengthMin != nil && *s.TripLengthMax < *s.TripLengthMin {
			add("trip_length_max", "trip_length_max is below trip_length_min")
		}
	}

	if s.MaxPriceMinor != nil && *s.MaxPriceMinor <= 0 {
		add("max_price_minor", "price ceiling must be positive")
	}
	if s.MaxStops != nil && *s.MaxStops < 0 {
		add("max_stops", "cannot be negative")
	}
	if s.MaxDurationMinutes != nil && *s.MaxDurationMinutes <= 0 {
		add("max_duration_minutes", "must be positive")
	}

	validateCarriers := func(field string, codes []string) map[string]bool {
		seen := map[string]bool{}
		if len(codes) > maxAirlineFilters {
			add(field, "at most %d carriers", maxAirlineFilters)
		}
		for _, code := range codes {
			if !carrierCodeRe.MatchString(code) {
				add(field, "%q is not a valid IATA carrier code", code)
			}
			if seen[code] {
				add(field, "duplicate carrier %q", code)
			}
			seen[code] = true
		}
		return seen
	}
	white := validateCarriers("airlines_whitelist", s.AirlinesWhitelist)
	for _, code := range s.AirlinesBlacklist {
		if white[code] {
			add("airlines_blacklist", "carrier %q is both whitelisted and blacklisted", code)
		}
	}
	validateCarriers("airlines_blacklist", s.AirlinesBlacklist)

	if !ValidAlertStrategy(s.AlertStrategy) {
		add("alert_strategy", "unknown strategy %q", s.AlertStrategy)
	}
	if s.AlertStrategy == StrategyAbsoluteThreshold && s.MaxPriceMinor == nil {
		add("max_price_minor", "absolute_threshold needs a price ceiling")
	}
	if !ValidStatus(s.Status) {
		add("status", "unknown status %q", s.Status)
	}

	if s.CooldownHours != nil && *s.CooldownHours < 0 {
		add("cooldown_hours", "cannot be negative")
	}
	if s.DropPct != nil && (*s.DropPct <= 0 || *s.DropPct >= 1) {
		add("drop_pct", "must be a ratio strictly between 0 and 1")
	}
	if s.StablePriceBandPct != nil && (*s.StablePriceBandPct < 0 || *s.StablePriceBandPct >= 1) {
		add("stable_price_band_pct", "must be a ratio in [0, 1)")
	}

	if len(errs) > 0 {
		return errs
	}
	return nil
}

// ValidateFresh rejects a travel window that is already entirely in the past —
// a rule for newly created subscriptions only (see Validate for why).
func (s *Subscription) ValidateFresh(now time.Time) error {
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	if !s.DateTo.IsZero() && s.DateTo.Before(today) {
		return ValidationError{{Field: "date_to", Message: "departure window is entirely in the past"}}
	}
	return nil
}
