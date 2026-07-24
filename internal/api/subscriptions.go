package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/kaayran/air-tickets-warden/internal/domain"
	"github.com/kaayran/air-tickets-warden/internal/services/subscriptions"
)

// subscriptionResponse is the client-facing shape. user_chat_id is deliberately
// absent — identity is implicit in the authenticated request. Money fields are
// integer EUR minor units.
type subscriptionResponse struct {
	ID                 string     `json:"id"`
	Origin             string     `json:"origin"`
	OriginAlternatives []string   `json:"origin_alternatives"`
	Destinations       []string   `json:"destinations"`
	DateFrom           Date       `json:"date_from"`
	DateTo             Date       `json:"date_to"`
	ReturnDateFrom     *Date      `json:"return_date_from"`
	ReturnDateTo       *Date      `json:"return_date_to"`
	TripLengthMin      *int32     `json:"trip_length_min"`
	TripLengthMax      *int32     `json:"trip_length_max"`
	MaxPriceMinor      *int64     `json:"max_price_minor"`
	MaxStops           *int32     `json:"max_stops"`
	MaxDurationMinutes *int32     `json:"max_duration_minutes"`
	AirlinesWhitelist  []string   `json:"airlines_whitelist"`
	AirlinesBlacklist  []string   `json:"airlines_blacklist"`
	AlertStrategy      string     `json:"alert_strategy"`
	CooldownHours      *int32     `json:"cooldown_hours"`
	DropPct            *float64   `json:"drop_pct"`
	StablePriceBandPct *float64   `json:"stable_price_band_pct"`
	MutedUntil         *time.Time `json:"muted_until"`
	Status             string     `json:"status"`
	NextCheckAt        time.Time  `json:"next_check_at"`
	CreatedAt          time.Time  `json:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at"`
}

func toSubscriptionResponse(s domain.Subscription) subscriptionResponse {
	return subscriptionResponse{
		ID:                 s.ID,
		Origin:             s.Origin,
		OriginAlternatives: emptyNotNil(s.OriginAlternatives),
		Destinations:       emptyNotNil(s.Destinations),
		DateFrom:           Date{s.DateFrom},
		DateTo:             Date{s.DateTo},
		ReturnDateFrom:     dateOrNil(s.ReturnDateFrom),
		ReturnDateTo:       dateOrNil(s.ReturnDateTo),
		TripLengthMin:      s.TripLengthMin,
		TripLengthMax:      s.TripLengthMax,
		MaxPriceMinor:      s.MaxPriceMinor,
		MaxStops:           s.MaxStops,
		MaxDurationMinutes: s.MaxDurationMinutes,
		AirlinesWhitelist:  emptyNotNil(s.AirlinesWhitelist),
		AirlinesBlacklist:  emptyNotNil(s.AirlinesBlacklist),
		AlertStrategy:      string(s.AlertStrategy),
		CooldownHours:      s.CooldownHours,
		DropPct:            s.DropPct,
		StablePriceBandPct: s.StablePriceBandPct,
		MutedUntil:         s.MutedUntil,
		Status:             string(s.Status),
		NextCheckAt:        s.NextCheckAt,
		CreatedAt:          s.CreatedAt,
		UpdatedAt:          s.UpdatedAt,
	}
}

// emptyNotNil keeps list fields marshalling as [] instead of null.
func emptyNotNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

// normCode uppercases one IATA-ish code ("beg " -> "BEG"); real validation is
// the domain's job.
func normCode(c string) string { return strings.ToUpper(strings.TrimSpace(c)) }

// normCodes normalizes a code list, dropping blank entries.
func normCodes(codes []string) []string {
	out := make([]string, 0, len(codes))
	for _, c := range codes {
		if n := normCode(c); n != "" {
			out = append(out, n)
		}
	}
	return out
}

// handleListSubscriptions returns the caller's subscriptions, newest first.
func (a *API) handleListSubscriptions(w http.ResponseWriter, r *http.Request) {
	chatID, ok := chatIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthenticated")
		return
	}
	subs, err := a.subs.List(r.Context(), chatID)
	if err != nil {
		a.log.Error("list subscriptions", "err", err, "chat_id", chatID)
		writeError(w, http.StatusInternalServerError, "could not list subscriptions")
		return
	}
	resp := make([]subscriptionResponse, len(subs))
	for i, s := range subs {
		resp[i] = toSubscriptionResponse(s)
	}
	writeJSON(w, http.StatusOK, resp)
}

// subscriptionCreateRequest is the POST body. Alert parameters left null
// resolve through the cascade (user settings -> env defaults); alert_strategy
// defaults to absolute_threshold; the subscription is born active.
type subscriptionCreateRequest struct {
	Origin             string   `json:"origin"`
	OriginAlternatives []string `json:"origin_alternatives"`
	Destinations       []string `json:"destinations"`
	DateFrom           Date     `json:"date_from"`
	DateTo             Date     `json:"date_to"`
	ReturnDateFrom     *Date    `json:"return_date_from"`
	ReturnDateTo       *Date    `json:"return_date_to"`
	TripLengthMin      *int32   `json:"trip_length_min"`
	TripLengthMax      *int32   `json:"trip_length_max"`
	MaxPriceMinor      *int64   `json:"max_price_minor"`
	MaxStops           *int32   `json:"max_stops"`
	MaxDurationMinutes *int32   `json:"max_duration_minutes"`
	AirlinesWhitelist  []string `json:"airlines_whitelist"`
	AirlinesBlacklist  []string `json:"airlines_blacklist"`
	AlertStrategy      string   `json:"alert_strategy"`
	CooldownHours      *int32   `json:"cooldown_hours"`
	DropPct            *float64 `json:"drop_pct"`
	StablePriceBandPct *float64 `json:"stable_price_band_pct"`
}

func (a *API) handleCreateSubscription(w http.ResponseWriter, r *http.Request) {
	chatID, ok := chatIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthenticated")
		return
	}
	var body subscriptionCreateRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body: "+err.Error())
		return
	}

	strategy := domain.AlertStrategy(strings.TrimSpace(body.AlertStrategy))
	if strategy == "" {
		strategy = domain.StrategyAbsoluteThreshold
	}
	sub := domain.Subscription{
		UserChatID:         chatID,
		Origin:             normCode(body.Origin),
		OriginAlternatives: normCodes(body.OriginAlternatives),
		Destinations:       normCodes(body.Destinations),
		DateFrom:           body.DateFrom.Time,
		DateTo:             body.DateTo.Time,
		ReturnDateFrom:     timeOrNil(body.ReturnDateFrom),
		ReturnDateTo:       timeOrNil(body.ReturnDateTo),
		TripLengthMin:      body.TripLengthMin,
		TripLengthMax:      body.TripLengthMax,
		MaxPriceMinor:      body.MaxPriceMinor,
		MaxStops:           body.MaxStops,
		MaxDurationMinutes: body.MaxDurationMinutes,
		AirlinesWhitelist:  normCodes(body.AirlinesWhitelist),
		AirlinesBlacklist:  normCodes(body.AirlinesBlacklist),
		AlertStrategy:      strategy,
		CooldownHours:      body.CooldownHours,
		DropPct:            body.DropPct,
		StablePriceBandPct: body.StablePriceBandPct,
		Status:             domain.StatusActive,
	}
	if !a.validateSubscription(w, &sub, true) {
		return
	}
	created, err := a.subs.Create(r.Context(), sub)
	if err != nil {
		a.log.Error("create subscription", "err", err, "chat_id", chatID)
		writeError(w, http.StatusInternalServerError, "could not create subscription")
		return
	}
	writeJSON(w, http.StatusCreated, toSubscriptionResponse(created))
}

func (a *API) handleGetSubscription(w http.ResponseWriter, r *http.Request) {
	chatID, ok := chatIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthenticated")
		return
	}
	sub, err := a.subs.Get(r.Context(), chatID, r.PathValue("id"))
	if err != nil {
		a.writeSubError(w, err, chatID, "get subscription")
		return
	}
	writeJSON(w, http.StatusOK, toSubscriptionResponse(sub))
}

// subscriptionPatchRequest is the PATCH body — true PATCH semantics via
// Optional: absent key = leave untouched, explicit null = clear (only
// meaningful for nullable fields; null on a required field is a 400).
// Pause/resume is status, mute is muted_until.
type subscriptionPatchRequest struct {
	Origin             Optional[string]    `json:"origin"`
	OriginAlternatives Optional[[]string]  `json:"origin_alternatives"`
	Destinations       Optional[[]string]  `json:"destinations"`
	DateFrom           Optional[Date]      `json:"date_from"`
	DateTo             Optional[Date]      `json:"date_to"`
	ReturnDateFrom     Optional[Date]      `json:"return_date_from"`
	ReturnDateTo       Optional[Date]      `json:"return_date_to"`
	TripLengthMin      Optional[int32]     `json:"trip_length_min"`
	TripLengthMax      Optional[int32]     `json:"trip_length_max"`
	MaxPriceMinor      Optional[int64]     `json:"max_price_minor"`
	MaxStops           Optional[int32]     `json:"max_stops"`
	MaxDurationMinutes Optional[int32]     `json:"max_duration_minutes"`
	AirlinesWhitelist  Optional[[]string]  `json:"airlines_whitelist"`
	AirlinesBlacklist  Optional[[]string]  `json:"airlines_blacklist"`
	AlertStrategy      Optional[string]    `json:"alert_strategy"`
	CooldownHours      Optional[int32]     `json:"cooldown_hours"`
	DropPct            Optional[float64]   `json:"drop_pct"`
	StablePriceBandPct Optional[float64]   `json:"stable_price_band_pct"`
	MutedUntil         Optional[time.Time] `json:"muted_until"`
	Status             Optional[string]    `json:"status"`
}

func (a *API) handlePatchSubscription(w http.ResponseWriter, r *http.Request) {
	chatID, ok := chatIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthenticated")
		return
	}
	var body subscriptionPatchRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body: "+err.Error())
		return
	}
	sub, err := a.subs.Get(r.Context(), chatID, r.PathValue("id"))
	if err != nil {
		a.writeSubError(w, err, chatID, "get subscription for patch")
		return
	}
	if verr := mergePatch(&sub, &body); verr != nil {
		writeValidationError(w, verr)
		return
	}
	if !a.validateSubscription(w, &sub, false) {
		return
	}
	updated, err := a.subs.Update(r.Context(), sub)
	if err != nil {
		a.writeSubError(w, err, chatID, "update subscription")
		return
	}
	writeJSON(w, http.StatusOK, toSubscriptionResponse(updated))
}

// mergePatch applies the set fields of the PATCH body onto sub. It returns
// field errors for explicit nulls on required fields; everything else is left
// to domain validation of the merged result.
func mergePatch(sub *domain.Subscription, body *subscriptionPatchRequest) domain.ValidationError {
	var errs domain.ValidationError
	nullField := func(field string) {
		errs = append(errs, domain.FieldError{Field: field, Message: "cannot be null"})
	}

	if body.Origin.Set {
		if body.Origin.Value == nil {
			nullField("origin")
		} else {
			sub.Origin = normCode(*body.Origin.Value)
		}
	}
	if body.OriginAlternatives.Set { // null and [] both mean "no alternatives"
		sub.OriginAlternatives = normCodes(deref(body.OriginAlternatives.Value))
	}
	if body.Destinations.Set {
		if body.Destinations.Value == nil {
			nullField("destinations")
		} else {
			sub.Destinations = normCodes(*body.Destinations.Value)
		}
	}
	if body.DateFrom.Set {
		if body.DateFrom.Value == nil {
			nullField("date_from")
		} else {
			sub.DateFrom = body.DateFrom.Value.Time
		}
	}
	if body.DateTo.Set {
		if body.DateTo.Value == nil {
			nullField("date_to")
		} else {
			sub.DateTo = body.DateTo.Value.Time
		}
	}
	if body.ReturnDateFrom.Set {
		sub.ReturnDateFrom = timeOrNil(body.ReturnDateFrom.Value)
	}
	if body.ReturnDateTo.Set {
		sub.ReturnDateTo = timeOrNil(body.ReturnDateTo.Value)
	}
	if body.TripLengthMin.Set {
		sub.TripLengthMin = body.TripLengthMin.Value
	}
	if body.TripLengthMax.Set {
		sub.TripLengthMax = body.TripLengthMax.Value
	}
	if body.MaxPriceMinor.Set {
		sub.MaxPriceMinor = body.MaxPriceMinor.Value
	}
	if body.MaxStops.Set {
		sub.MaxStops = body.MaxStops.Value
	}
	if body.MaxDurationMinutes.Set {
		sub.MaxDurationMinutes = body.MaxDurationMinutes.Value
	}
	if body.AirlinesWhitelist.Set {
		sub.AirlinesWhitelist = normCodes(deref(body.AirlinesWhitelist.Value))
	}
	if body.AirlinesBlacklist.Set {
		sub.AirlinesBlacklist = normCodes(deref(body.AirlinesBlacklist.Value))
	}
	if body.AlertStrategy.Set {
		if body.AlertStrategy.Value == nil {
			nullField("alert_strategy")
		} else {
			sub.AlertStrategy = domain.AlertStrategy(strings.TrimSpace(*body.AlertStrategy.Value))
		}
	}
	if body.CooldownHours.Set { // null = back to the cascade default
		sub.CooldownHours = body.CooldownHours.Value
	}
	if body.DropPct.Set {
		sub.DropPct = body.DropPct.Value
	}
	if body.StablePriceBandPct.Set {
		sub.StablePriceBandPct = body.StablePriceBandPct.Value
	}
	if body.MutedUntil.Set { // null = unmute
		sub.MutedUntil = body.MutedUntil.Value
	}
	if body.Status.Set {
		if body.Status.Value == nil {
			nullField("status")
		} else {
			sub.Status = domain.Status(*body.Status.Value)
		}
	}
	return errs
}

func deref[T any](p *[]T) []T {
	if p == nil {
		return nil
	}
	return *p
}

func (a *API) handleDeleteSubscription(w http.ResponseWriter, r *http.Request) {
	chatID, ok := chatIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthenticated")
		return
	}
	if err := a.subs.Delete(r.Context(), chatID, r.PathValue("id")); err != nil {
		a.writeSubError(w, err, chatID, "delete subscription")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// validateSubscription runs domain validation (and, for creates, the
// freshness rule) and writes the 400 itself; reports whether to proceed.
func (a *API) validateSubscription(w http.ResponseWriter, sub *domain.Subscription, fresh bool) bool {
	err := sub.Validate(a.airports)
	if err == nil && fresh {
		err = sub.ValidateFresh(time.Now())
	}
	if err == nil {
		return true
	}
	var verr domain.ValidationError
	if errors.As(err, &verr) {
		writeValidationError(w, verr)
	} else {
		writeError(w, http.StatusBadRequest, err.Error())
	}
	return false
}

// writeSubError maps store errors: not-found (foreign or missing id — the two
// are indistinguishable by design) to 404, anything else to 500.
func (a *API) writeSubError(w http.ResponseWriter, err error, chatID int64, op string) {
	if errors.Is(err, subscriptions.ErrNotFound) {
		writeError(w, http.StatusNotFound, "subscription not found")
		return
	}
	a.log.Error(op, "err", err, "chat_id", chatID)
	writeError(w, http.StatusInternalServerError, "internal error")
}
