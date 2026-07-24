// Package airports is the offline airport reference: an embedded, filtered
// OurAirports dataset (see scripts/prepare-airports for regeneration) parsed
// once at startup. It answers IATA lookups (the domain's AirportLookup),
// autocomplete searches for the Mini App form, and timezone resolution.
package airports

import (
	"embed"
	"encoding/csv"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"

	"golang.org/x/text/runes"
	"golang.org/x/text/transform"
	"golang.org/x/text/unicode/norm"
)

//go:embed data/airports.csv
var dataFS embed.FS

// Airport is one row of the reference dataset.
type Airport struct {
	IATA      string
	Type      string // large_airport | medium_airport | small_airport
	Name      string
	City      string // municipality
	Country   string // ISO 3166-1 alpha-2
	Lat, Lon  float64
	Scheduled bool // has scheduled airline service
	TZ        string
}

// indexed carries the precomputed fold keys used by Search.
type indexed struct {
	Airport
	cityFold string
	nameFold string
	rank     int // large=0 medium=1 small=2 — tie-break for search results
}

// Service is the in-memory dataset. Immutable after New; safe for concurrent use.
type Service struct {
	byIATA  map[string]Airport
	all     []indexed
	popular []Airport // large airports with scheduled service, city-alphabetical
}

// New parses the embedded dataset. Errors mean a broken embedded file — a
// build problem, not a runtime condition.
func New() (*Service, error) {
	f, err := dataFS.Open("data/airports.csv")
	if err != nil {
		return nil, fmt.Errorf("open embedded airports dataset: %w", err)
	}
	defer func() { _ = f.Close() }()

	r := csv.NewReader(f)
	rows, err := r.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("parse airports dataset: %w", err)
	}
	if len(rows) < 2 {
		return nil, fmt.Errorf("airports dataset is empty")
	}

	svc := &Service{byIATA: make(map[string]Airport, len(rows)-1)}
	for _, row := range rows[1:] { // rows[0] is the header
		if len(row) != 9 {
			return nil, fmt.Errorf("airports dataset row has %d columns, want 9", len(row))
		}
		lat, err1 := strconv.ParseFloat(row[5], 64)
		lon, err2 := strconv.ParseFloat(row[6], 64)
		if err1 != nil || err2 != nil {
			return nil, fmt.Errorf("airport %s: bad coordinates", row[0])
		}
		a := Airport{
			IATA:      row[0],
			Type:      row[1],
			Name:      row[2],
			City:      row[3],
			Country:   row[4],
			Lat:       lat,
			Lon:       lon,
			Scheduled: row[7] == "yes",
			TZ:        row[8],
		}
		svc.byIATA[a.IATA] = a
		svc.all = append(svc.all, indexed{
			Airport:  a,
			cityFold: fold(a.City),
			nameFold: fold(a.Name),
			rank:     typeRank(a.Type),
		})
	}
	for _, a := range svc.byIATA {
		if a.Type == "large_airport" && a.Scheduled {
			svc.popular = append(svc.popular, a)
		}
	}
	sort.Slice(svc.popular, func(i, j int) bool {
		a, b := svc.popular[i], svc.popular[j]
		if a.City != b.City {
			return a.City < b.City
		}
		return a.IATA < b.IATA
	})
	return svc, nil
}

// Popular returns up to limit major airports (large, with scheduled service)
// in city-alphabetical order — the airport picker's browse list before the
// user has typed anything.
func (s *Service) Popular(limit int) []Airport {
	if limit <= 0 || limit > len(s.popular) {
		limit = len(s.popular)
	}
	out := make([]Airport, limit)
	copy(out, s.popular[:limit])
	return out
}

// Exists implements domain.AirportLookup.
func (s *Service) Exists(iata string) bool {
	_, ok := s.byIATA[strings.ToUpper(iata)]
	return ok
}

// Get returns the airport for an IATA code.
func (s *Service) Get(iata string) (Airport, bool) {
	a, ok := s.byIATA[strings.ToUpper(iata)]
	return a, ok
}

// Location resolves the airport's IANA timezone. The binary carries its own
// zone database (time/tzdata is imported by the main package).
func (s *Service) Location(iata string) (*time.Location, error) {
	a, ok := s.Get(iata)
	if !ok {
		return nil, fmt.Errorf("unknown airport %q", iata)
	}
	loc, err := time.LoadLocation(a.TZ)
	if err != nil {
		return nil, fmt.Errorf("load timezone %q for %s: %w", a.TZ, a.IATA, err)
	}
	return loc, nil
}

// Search match tiers, best first: exact IATA, city word-prefix, name
// word-prefix, IATA prefix. Ties break on scheduled service, airport size,
// then city name — so "lon" puts Heathrow above small London strips.
const (
	matchIATAExact = iota
	matchCityPrefix
	matchNamePrefix
	matchIATAPrefix
	matchNone
)

// Search returns up to limit airports for an autocomplete query. Matching is
// case- and diacritic-insensitive ("timis" finds Timișoara) and starts from
// the first character; an empty query returns nil (the picker's browse list
// is Popular, not Search).
func (s *Service) Search(query string, limit int) []Airport {
	q := fold(strings.TrimSpace(query))
	if q == "" || limit <= 0 {
		return nil
	}
	qUpper := strings.ToUpper(strings.TrimSpace(query))

	type scored struct {
		indexed
		tier int
	}
	var hits []scored
	for _, a := range s.all {
		tier := matchNone
		switch {
		case a.IATA == qUpper:
			tier = matchIATAExact
		case wordPrefix(a.cityFold, q):
			tier = matchCityPrefix
		case wordPrefix(a.nameFold, q):
			tier = matchNamePrefix
		case strings.HasPrefix(a.IATA, qUpper):
			tier = matchIATAPrefix
		}
		if tier != matchNone {
			hits = append(hits, scored{indexed: a, tier: tier})
		}
	}
	sort.SliceStable(hits, func(i, j int) bool {
		a, b := hits[i], hits[j]
		if a.tier != b.tier {
			return a.tier < b.tier
		}
		if a.Scheduled != b.Scheduled {
			return a.Scheduled
		}
		if a.rank != b.rank {
			return a.rank < b.rank
		}
		return a.City < b.City
	})
	if len(hits) > limit {
		hits = hits[:limit]
	}
	out := make([]Airport, len(hits))
	for i, h := range hits {
		out[i] = h.Airport
	}
	return out
}

func typeRank(t string) int {
	switch t {
	case "large_airport":
		return 0
	case "medium_airport":
		return 1
	default:
		return 2
	}
}

// wordPrefix reports whether any word of s (split on spaces, hyphens, slashes)
// starts with q. s must already be folded.
func wordPrefix(s, q string) bool {
	if strings.HasPrefix(s, q) {
		return true
	}
	for _, sep := range []string{" ", "-", "/"} {
		rest := s
		for {
			i := strings.Index(rest, sep)
			if i < 0 {
				break
			}
			rest = rest[i+len(sep):]
			if strings.HasPrefix(rest, q) {
				return true
			}
		}
	}
	return false
}

// fold lowercases and strips diacritics for accent-insensitive matching. The
// transformer is built per call: chained transformers carry state and are not
// safe for concurrent use, and Search runs on concurrent HTTP requests.
func fold(s string) string {
	t := transform.Chain(norm.NFD, runes.Remove(runes.In(unicode.Mn)), norm.NFC)
	if out, _, err := transform.String(t, s); err == nil {
		s = out
	}
	return strings.ToLower(s)
}
