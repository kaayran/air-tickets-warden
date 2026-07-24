package airports

import (
	"testing"

	"github.com/kaayran/air-tickets-warden/internal/domain"
)

// The service is the production implementation of the domain's lookup port.
var _ domain.AirportLookup = (*Service)(nil)

// load parses the embedded dataset once for all tests.
func load(t *testing.T) *Service {
	t.Helper()
	svc, err := New()
	if err != nil {
		t.Fatalf("New(): %v", err)
	}
	return svc
}

func TestLookup(t *testing.T) {
	svc := load(t)

	if !svc.Exists("BEG") {
		t.Error("BEG missing from dataset")
	}
	if svc.Exists("ZZZ") {
		t.Error("ZZZ should not exist")
	}
	if !svc.Exists("beg") {
		t.Error("Exists must be case-insensitive")
	}

	a, ok := svc.Get("BEG")
	if !ok {
		t.Fatal("Get(BEG) not found")
	}
	if a.City != "Belgrade" || a.Country != "RS" || a.TZ != "Europe/Belgrade" || !a.Scheduled {
		t.Errorf("BEG data looks wrong: %+v", a)
	}
}

func TestLocation(t *testing.T) {
	svc := load(t)

	loc, err := svc.Location("BEG")
	if err != nil {
		t.Fatalf("Location(BEG): %v", err)
	}
	if loc.String() != "Europe/Belgrade" {
		t.Errorf("Location(BEG) = %s, want Europe/Belgrade", loc)
	}
	if _, err := svc.Location("ZZZ"); err == nil {
		t.Error("Location(ZZZ) should fail")
	}
}

func TestSearch(t *testing.T) {
	svc := load(t)

	t.Run("exact IATA wins", func(t *testing.T) {
		got := svc.Search("BEG", 5)
		if len(got) == 0 || got[0].IATA != "BEG" {
			t.Errorf("Search(BEG)[0] = %v, want BEG first", first(got))
		}
	})

	t.Run("lowercase IATA works", func(t *testing.T) {
		got := svc.Search("beg", 5)
		if len(got) == 0 || got[0].IATA != "BEG" {
			t.Errorf("Search(beg)[0] = %v, want BEG first", first(got))
		}
	})

	t.Run("city prefix", func(t *testing.T) {
		got := svc.Search("belgra", 5)
		if len(got) == 0 || got[0].IATA != "BEG" {
			t.Errorf("Search(belgra)[0] = %v, want BEG", first(got))
		}
	})

	t.Run("diacritic folding", func(t *testing.T) {
		got := svc.Search("timis", 5)
		if len(got) == 0 || got[0].IATA != "TSR" {
			t.Errorf("Search(timis)[0] = %v, want TSR (Timișoara)", first(got))
		}
	})

	t.Run("big hubs rank above small strips", func(t *testing.T) {
		got := svc.Search("london", 10)
		if len(got) == 0 {
			t.Fatal("Search(london) found nothing")
		}
		if got[0].Type != "large_airport" {
			t.Errorf("Search(london)[0] = %s (%s), want a large airport", got[0].IATA, got[0].Type)
		}
	})

	t.Run("limit respected", func(t *testing.T) {
		if got := svc.Search("san", 3); len(got) > 3 {
			t.Errorf("limit ignored: got %d results", len(got))
		}
	})

	t.Run("short queries return nothing", func(t *testing.T) {
		if got := svc.Search("b", 5); got != nil {
			t.Errorf("one-letter query returned %d results", len(got))
		}
		if got := svc.Search("  ", 5); got != nil {
			t.Error("whitespace query returned results")
		}
	})
}

func first(a []Airport) string {
	if len(a) == 0 {
		return "<empty>"
	}
	return a[0].IATA
}
