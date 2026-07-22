package api

import (
	"encoding/json"
	"net/http"

	"github.com/kaayran/air-tickets-warden/internal/storage/sqlcgen"
)

// meResponse is the client-facing shape of user settings (avoids leaking the
// pgtype timestamp representation).
type meResponse struct {
	ChatID             int64    `json:"chat_id"`
	CooldownHours      *int32   `json:"cooldown_hours"`
	DropPct            *float64 `json:"drop_pct"`
	StablePriceBandPct *float64 `json:"stable_price_band_pct"`
}

func toMeResponse(s sqlcgen.UserSetting) meResponse {
	return meResponse{
		ChatID:             s.ChatID,
		CooldownHours:      s.CooldownHours,
		DropPct:            s.DropPct,
		StablePriceBandPct: s.StablePriceBandPct,
	}
}

// handleGetMe returns the caller's settings, lazily creating a default row on
// first access.
func (a *API) handleGetMe(w http.ResponseWriter, r *http.Request) {
	chatID, ok := chatIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthenticated")
		return
	}
	settings, err := a.store.Queries.EnsureUserSettings(r.Context(), chatID)
	if err != nil {
		a.log.Error("ensure user settings", "err", err, "chat_id", chatID)
		writeError(w, http.StatusInternalServerError, "could not load settings")
		return
	}
	writeJSON(w, http.StatusOK, toMeResponse(settings))
}

// mePatchRequest is the PATCH body. Each field replaces the stored value; an
// omitted field clears it (Phase 0 semantics — refined later if needed).
type mePatchRequest struct {
	CooldownHours      *int32   `json:"cooldown_hours"`
	DropPct            *float64 `json:"drop_pct"`
	StablePriceBandPct *float64 `json:"stable_price_band_pct"`
}

// handlePatchMe upserts the caller's settings.
func (a *API) handlePatchMe(w http.ResponseWriter, r *http.Request) {
	chatID, ok := chatIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthenticated")
		return
	}
	var body mePatchRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	settings, err := a.store.Queries.UpdateUserSettings(r.Context(), sqlcgen.UpdateUserSettingsParams{
		ChatID:             chatID,
		CooldownHours:      body.CooldownHours,
		DropPct:            body.DropPct,
		StablePriceBandPct: body.StablePriceBandPct,
	})
	if err != nil {
		a.log.Error("update user settings", "err", err, "chat_id", chatID)
		writeError(w, http.StatusInternalServerError, "could not update settings")
		return
	}
	writeJSON(w, http.StatusOK, toMeResponse(settings))
}
