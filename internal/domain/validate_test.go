package domain

import (
	"errors"
	"testing"
	"time"
)

// fakeAirports is a set-backed AirportLookup for tests.
type fakeAirports map[string]bool

func (f fakeAirports) Exists(iata string) bool { return f[iata] }

var testAirports = fakeAirports{"BEG": true, "BCN": true, "MAD": true, "VLC": true, "BUD": true, "SOF": true}

func ptr[T any](v T) *T { return &v }

func date(y int, m time.Month, d int) time.Time {
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

// validSub returns a subscription that passes Validate; cases mutate it.
func validSub() Subscription {
	return Subscription{
		UserChatID:         42,
		Origin:             "BEG",
		OriginAlternatives: []string{"BUD"},
		Destinations:       []string{"BCN", "MAD"},
		DateFrom:           date(2030, 7, 1),
		DateTo:             date(2030, 7, 15),
		MaxPriceMinor:      ptr[int64](15000),
		AlertStrategy:      StrategyAbsoluteThreshold,
		Status:             StatusActive,
	}
}

func TestValidateOK(t *testing.T) {
	s := validSub()
	if err := s.Validate(testAirports); err != nil {
		t.Fatalf("valid subscription rejected: %v", err)
	}
}

func TestValidateRoundTripOK(t *testing.T) {
	s := validSub()
	s.ReturnDateFrom = ptr(date(2030, 7, 5))
	s.ReturnDateTo = ptr(date(2030, 7, 20))
	s.TripLengthMin = ptr[int32](3)
	s.TripLengthMax = ptr[int32](10)
	if err := s.Validate(testAirports); err != nil {
		t.Fatalf("valid round-trip rejected: %v", err)
	}
	if !s.RoundTrip() {
		t.Error("RoundTrip() = false for a subscription with a return window")
	}
}

func TestValidateFailures(t *testing.T) {
	tests := []struct {
		name      string
		mutate    func(*Subscription)
		wantField string
	}{
		{"bad origin format", func(s *Subscription) { s.Origin = "beg" }, "origin"},
		{"unknown origin", func(s *Subscription) { s.Origin = "XXX" }, "origin"},
		{"alt duplicates origin", func(s *Subscription) { s.OriginAlternatives = []string{"BEG"} }, "origin_alternatives"},
		{"duplicate alternatives", func(s *Subscription) { s.OriginAlternatives = []string{"BUD", "BUD"} }, "origin_alternatives"},
		{"too many alternatives", func(s *Subscription) {
			s.OriginAlternatives = []string{"BUD", "SOF", "BCN", "MAD", "VLC", "BUD"}
		}, "origin_alternatives"},
		{"no destinations", func(s *Subscription) { s.Destinations = nil }, "destinations"},
		{"unknown destination", func(s *Subscription) { s.Destinations = []string{"ZZZ"} }, "destinations"},
		{"destination equals origin", func(s *Subscription) { s.Destinations = []string{"BEG"} }, "destinations"},
		{"duplicate destinations", func(s *Subscription) { s.Destinations = []string{"BCN", "BCN"} }, "destinations"},
		{"missing dates", func(s *Subscription) { s.DateFrom, s.DateTo = time.Time{}, time.Time{} }, "date_from"},
		{"inverted date range", func(s *Subscription) { s.DateTo = date(2030, 6, 1) }, "date_to"},
		{"half return window", func(s *Subscription) { s.ReturnDateFrom = ptr(date(2030, 7, 5)) }, "return_date_from"},
		{"inverted return range", func(s *Subscription) {
			s.ReturnDateFrom = ptr(date(2030, 7, 20))
			s.ReturnDateTo = ptr(date(2030, 7, 5))
		}, "return_date_to"},
		{"return before departure window", func(s *Subscription) {
			s.ReturnDateFrom = ptr(date(2030, 6, 1))
			s.ReturnDateTo = ptr(date(2030, 7, 20))
		}, "return_date_from"},
		{"zero trip length", func(s *Subscription) { s.TripLengthMin = ptr[int32](0) }, "trip_length_min"},
		{"inverted trip length", func(s *Subscription) {
			s.TripLengthMin = ptr[int32](7)
			s.TripLengthMax = ptr[int32](3)
		}, "trip_length_max"},
		{"non-positive price", func(s *Subscription) { s.MaxPriceMinor = ptr[int64](0) }, "max_price_minor"},
		{"negative stops", func(s *Subscription) { s.MaxStops = ptr[int32](-1) }, "max_stops"},
		{"non-positive duration", func(s *Subscription) { s.MaxDurationMinutes = ptr[int32](0) }, "max_duration_minutes"},
		{"bad carrier code", func(s *Subscription) { s.AirlinesWhitelist = []string{"w6"} }, "airlines_whitelist"},
		{"duplicate carrier", func(s *Subscription) { s.AirlinesBlacklist = []string{"FR", "FR"} }, "airlines_blacklist"},
		{"carrier in both lists", func(s *Subscription) {
			s.AirlinesWhitelist = []string{"W6"}
			s.AirlinesBlacklist = []string{"W6"}
		}, "airlines_blacklist"},
		{"unknown strategy", func(s *Subscription) { s.AlertStrategy = "psychic" }, "alert_strategy"},
		{"absolute threshold without ceiling", func(s *Subscription) { s.MaxPriceMinor = nil }, "max_price_minor"},
		{"unknown status", func(s *Subscription) { s.Status = "zombie" }, "status"},
		{"negative cooldown", func(s *Subscription) { s.CooldownHours = ptr[int32](-1) }, "cooldown_hours"},
		{"drop pct out of range", func(s *Subscription) { s.DropPct = ptr(1.5) }, "drop_pct"},
		{"stable band out of range", func(s *Subscription) { s.StablePriceBandPct = ptr(-0.1) }, "stable_price_band_pct"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := validSub()
			tt.mutate(&s)
			err := s.Validate(testAirports)
			if err == nil {
				t.Fatal("Validate() = nil, want error")
			}
			var verr ValidationError
			if !errors.As(err, &verr) {
				t.Fatalf("error is %T, want ValidationError", err)
			}
			found := false
			for _, fe := range verr {
				if fe.Field == tt.wantField {
					found = true
				}
			}
			if !found {
				t.Errorf("no error for field %q in %v", tt.wantField, verr)
			}
		})
	}
}

func TestValidateFresh(t *testing.T) {
	now := time.Date(2030, 8, 1, 13, 45, 0, 0, time.UTC)

	s := validSub() // window ends 2030-07-15, before now
	if err := s.ValidateFresh(now); err == nil {
		t.Error("past window accepted by ValidateFresh")
	}

	s.DateTo = date(2030, 8, 1) // today is still fresh
	if err := s.ValidateFresh(now); err != nil {
		t.Errorf("window ending today rejected: %v", err)
	}

	// Validate itself must stay time-independent: a stale existing
	// subscription still validates structurally (pause/edit must not 400).
	stale := validSub()
	if err := stale.Validate(testAirports); err != nil {
		t.Errorf("stale subscription failed structural validation: %v", err)
	}
}

func TestIsMuted(t *testing.T) {
	now := time.Date(2030, 7, 1, 12, 0, 0, 0, time.UTC)
	s := validSub()
	if s.IsMuted(now) {
		t.Error("unmuted subscription reports muted")
	}
	s.MutedUntil = ptr(now.Add(time.Hour))
	if !s.IsMuted(now) {
		t.Error("future muted_until not detected")
	}
	s.MutedUntil = ptr(now.Add(-time.Hour))
	if s.IsMuted(now) {
		t.Error("expired mute still reported as muted")
	}
}
