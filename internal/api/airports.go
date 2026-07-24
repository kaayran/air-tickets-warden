package api

import (
	"net/http"
	"strings"

	"github.com/kaayran/air-tickets-warden/internal/services/airports"
)

// airportSearchLimit bounds an autocomplete response — a dropdown never shows
// more anyway. The browse list (empty query) is longer: it is the only way to
// scroll around before typing.
const (
	airportSearchLimit = 8
	airportBrowseLimit = 20
)

type airportResponse struct {
	IATA    string `json:"iata"`
	Name    string `json:"name"`
	City    string `json:"city"`
	Country string `json:"country"`
}

// handleSearchAirports serves the airport picker: GET /api/v1/airports?q=belg.
// An empty query returns the browse list (major hubs, city-alphabetical) so
// the picker has content before the user types.
func (a *API) handleSearchAirports(w http.ResponseWriter, r *http.Request) {
	var results []airports.Airport
	if q := strings.TrimSpace(r.URL.Query().Get("q")); q == "" {
		results = a.airports.Popular(airportBrowseLimit)
	} else {
		results = a.airports.Search(q, airportSearchLimit)
	}
	resp := make([]airportResponse, len(results))
	for i, ap := range results {
		resp[i] = airportResponse{IATA: ap.IATA, Name: ap.Name, City: ap.City, Country: ap.Country}
	}
	writeJSON(w, http.StatusOK, resp)
}
