package api

import "net/http"

// airportSearchLimit bounds an autocomplete response — a dropdown never shows
// more anyway.
const airportSearchLimit = 8

type airportResponse struct {
	IATA    string `json:"iata"`
	Name    string `json:"name"`
	City    string `json:"city"`
	Country string `json:"country"`
}

// handleSearchAirports serves the airport picker: GET /api/v1/airports?q=belg.
// Sub-2-character queries yield an empty list (mirrors the service rule).
func (a *API) handleSearchAirports(w http.ResponseWriter, r *http.Request) {
	results := a.airports.Search(r.URL.Query().Get("q"), airportSearchLimit)
	resp := make([]airportResponse, len(results))
	for i, ap := range results {
		resp[i] = airportResponse{IATA: ap.IATA, Name: ap.Name, City: ap.City, Country: ap.Country}
	}
	writeJSON(w, http.StatusOK, resp)
}
