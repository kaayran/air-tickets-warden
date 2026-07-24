package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	initdata "github.com/telegram-mini-apps/init-data-golang"

	"github.com/kaayran/air-tickets-warden/internal/domain"
	"github.com/kaayran/air-tickets-warden/internal/services/airports"
	"github.com/kaayran/air-tickets-warden/internal/services/subscriptions"
	"github.com/kaayran/air-tickets-warden/internal/telemetry"
)

const (
	testBotToken       = "12345:test-token"
	ownerID      int64 = 100
	friendID     int64 = 200 // whitelisted, but must not see owner's data
	strangerID   int64 = 999 // not whitelisted
)

// signedInitData builds a Telegram-shaped initData query string signed with
// token — the same HMAC scheme the middleware validates.
func signedInitData(t *testing.T, token string, userID int64, authDate time.Time) string {
	t.Helper()
	user := fmt.Sprintf(`{"id":%d,"first_name":"Test","username":"test"}`, userID)
	payload := map[string]string{"user": user, "query_id": "AAtest"}
	hash := initdata.Sign(payload, token, authDate)

	v := url.Values{}
	v.Set("user", user)
	v.Set("query_id", "AAtest")
	v.Set("auth_date", strconv.FormatInt(authDate.Unix(), 10))
	v.Set("hash", hash)
	return v.Encode()
}

// fakeSubs is an in-memory SubscriptionStore mirroring the manager's
// ownership contract: foreign or unknown ids read as ErrNotFound.
type fakeSubs struct {
	mu   sync.Mutex
	rows map[string]domain.Subscription
	seq  int
}

func newFakeSubs() *fakeSubs { return &fakeSubs{rows: map[string]domain.Subscription{}} }

func (f *fakeSubs) Create(_ context.Context, sub domain.Subscription) (domain.Subscription, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.seq++
	sub.ID = fmt.Sprintf("00000000-0000-0000-0000-%012d", f.seq)
	now := time.Now()
	sub.NextCheckAt, sub.CreatedAt, sub.UpdatedAt = now, now, now
	f.rows[sub.ID] = sub
	return sub, nil
}

func (f *fakeSubs) List(_ context.Context, chatID int64) ([]domain.Subscription, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []domain.Subscription
	for _, s := range f.rows {
		if s.UserChatID == chatID {
			out = append(out, s)
		}
	}
	return out, nil
}

func (f *fakeSubs) Get(_ context.Context, chatID int64, id string) (domain.Subscription, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	s, ok := f.rows[id]
	if !ok || s.UserChatID != chatID {
		return domain.Subscription{}, subscriptions.ErrNotFound
	}
	return s, nil
}

func (f *fakeSubs) Update(_ context.Context, sub domain.Subscription) (domain.Subscription, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	old, ok := f.rows[sub.ID]
	if !ok || old.UserChatID != sub.UserChatID {
		return domain.Subscription{}, subscriptions.ErrNotFound
	}
	sub.UpdatedAt = time.Now()
	f.rows[sub.ID] = sub
	return sub, nil
}

func (f *fakeSubs) Delete(_ context.Context, chatID int64, id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	s, ok := f.rows[id]
	if !ok || s.UserChatID != chatID {
		return subscriptions.ErrNotFound
	}
	delete(f.rows, id)
	return nil
}

// newTestAPI wires the handler stack with the real airports dataset and an
// in-memory subscription store. The storage.Store is nil — these tests never
// touch the /me handlers.
func newTestAPI(t *testing.T) http.Handler {
	t.Helper()
	air, err := airports.New()
	if err != nil {
		t.Fatalf("airports.New(): %v", err)
	}
	a := New(nil, newFakeSubs(), air, testBotToken, []int64{ownerID, friendID}, telemetry.Logger("error"))
	return a.Handler()
}

// call sends an authenticated request as userID and decodes the response.
func call(t *testing.T, h http.Handler, userID int64, method, path, body string) (*httptest.ResponseRecorder, map[string]any) {
	t.Helper()
	rec := callRaw(t, h, "tma "+signedInitData(t, testBotToken, userID, time.Now()), method, path, body)
	var decoded map[string]any
	if rec.Body.Len() > 0 && strings.HasPrefix(rec.Body.String(), "{") {
		if err := json.Unmarshal(rec.Body.Bytes(), &decoded); err != nil {
			t.Fatalf("%s %s: bad JSON response %q", method, path, rec.Body.String())
		}
	}
	return rec, decoded
}

func callRaw(t *testing.T, h http.Handler, authHeader, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, bytes.NewReader([]byte(body)))
	if authHeader != "" {
		req.Header.Set("Authorization", authHeader)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

// validCreateBody is a well-formed subscription create payload.
const validCreateBody = `{
	"origin": "BEG",
	"destinations": ["BCN", "MAD"],
	"date_from": "2030-07-01",
	"date_to": "2030-07-15",
	"max_price_minor": 15000
}`

func TestAuth(t *testing.T) {
	h := newTestAPI(t)

	t.Run("valid initData passes", func(t *testing.T) {
		rec, _ := call(t, h, ownerID, http.MethodGet, "/api/v1/subscriptions", "")
		if rec.Code != http.StatusOK {
			t.Errorf("status = %d, want 200; body %s", rec.Code, rec.Body)
		}
	})

	t.Run("expired initData is rejected", func(t *testing.T) {
		expired := signedInitData(t, testBotToken, ownerID, time.Now().Add(-2*time.Hour))
		rec := callRaw(t, h, "tma "+expired, http.MethodGet, "/api/v1/subscriptions", "")
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("status = %d, want 401", rec.Code)
		}
	})

	t.Run("tampered initData is rejected", func(t *testing.T) {
		forged := signedInitData(t, "12345:wrong-token", ownerID, time.Now())
		rec := callRaw(t, h, "tma "+forged, http.MethodGet, "/api/v1/subscriptions", "")
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("status = %d, want 401", rec.Code)
		}
	})

	t.Run("non-whitelisted user is rejected", func(t *testing.T) {
		rec, _ := call(t, h, strangerID, http.MethodGet, "/api/v1/subscriptions", "")
		if rec.Code != http.StatusForbidden {
			t.Errorf("status = %d, want 403", rec.Code)
		}
	})

	t.Run("missing header is rejected", func(t *testing.T) {
		rec := callRaw(t, h, "", http.MethodGet, "/api/v1/subscriptions", "")
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("status = %d, want 401", rec.Code)
		}
	})
}

func TestSubscriptionLifecycle(t *testing.T) {
	h := newTestAPI(t)

	// Create — codes are normalized, defaults applied.
	rec, created := call(t, h, ownerID, http.MethodPost, "/api/v1/subscriptions",
		strings.Replace(validCreateBody, `"BEG"`, `" beg "`, 1))
	if rec.Code != http.StatusCreated {
		t.Fatalf("create: status = %d, body %s", rec.Code, rec.Body)
	}
	if created["origin"] != "BEG" {
		t.Errorf("origin not normalized: %v", created["origin"])
	}
	if created["status"] != "active" || created["alert_strategy"] != "absolute_threshold" {
		t.Errorf("defaults not applied: status=%v strategy=%v", created["status"], created["alert_strategy"])
	}
	if created["date_from"] != "2030-07-01" {
		t.Errorf("date_from = %v, want 2030-07-01", created["date_from"])
	}
	if created["max_price_minor"] != float64(15000) {
		t.Errorf("max_price_minor = %v", created["max_price_minor"])
	}
	id := created["id"].(string)

	// List shows it.
	rec, _ = call(t, h, ownerID, http.MethodGet, "/api/v1/subscriptions", "")
	var list []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil || len(list) != 1 {
		t.Fatalf("list: err=%v n=%d body=%s", err, len(list), rec.Body)
	}

	// Get by id.
	rec, got := call(t, h, ownerID, http.MethodGet, "/api/v1/subscriptions/"+id, "")
	if rec.Code != http.StatusOK || got["id"] != id {
		t.Fatalf("get: status=%d id=%v", rec.Code, got["id"])
	}

	// Pause via PATCH status.
	rec, patched := call(t, h, ownerID, http.MethodPatch, "/api/v1/subscriptions/"+id, `{"status":"paused"}`)
	if rec.Code != http.StatusOK || patched["status"] != "paused" {
		t.Fatalf("pause: status=%d body=%s", rec.Code, rec.Body)
	}

	// Mute via muted_until; other fields untouched.
	rec, patched = call(t, h, ownerID, http.MethodPatch, "/api/v1/subscriptions/"+id,
		`{"muted_until":"2030-06-01T12:00:00Z"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("mute: status=%d body=%s", rec.Code, rec.Body)
	}
	if patched["muted_until"] == nil || patched["status"] != "paused" {
		t.Errorf("mute merged wrong: muted_until=%v status=%v", patched["muted_until"], patched["status"])
	}

	// Unmute with explicit null; clear cooldown back to cascade with null.
	rec, patched = call(t, h, ownerID, http.MethodPatch, "/api/v1/subscriptions/"+id,
		`{"muted_until":null,"cooldown_hours":null,"status":"active"}`)
	if rec.Code != http.StatusOK || patched["muted_until"] != nil || patched["status"] != "active" {
		t.Fatalf("unmute: status=%d body=%s", rec.Code, rec.Body)
	}

	// Delete, then 404.
	rec, _ = call(t, h, ownerID, http.MethodDelete, "/api/v1/subscriptions/"+id, "")
	if rec.Code != http.StatusNoContent {
		t.Fatalf("delete: status=%d", rec.Code)
	}
	rec, _ = call(t, h, ownerID, http.MethodGet, "/api/v1/subscriptions/"+id, "")
	if rec.Code != http.StatusNotFound {
		t.Errorf("get after delete: status=%d, want 404", rec.Code)
	}
}

func TestForeignObjectIs404(t *testing.T) {
	h := newTestAPI(t)

	rec, created := call(t, h, ownerID, http.MethodPost, "/api/v1/subscriptions", validCreateBody)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create: status=%d body=%s", rec.Code, rec.Body)
	}
	id := created["id"].(string)

	// friendID is whitelisted but does not own the subscription: every verb
	// must answer 404 — never 403, which would confirm existence.
	for _, tc := range []struct{ method, body string }{
		{http.MethodGet, ""},
		{http.MethodPatch, `{"status":"paused"}`},
		{http.MethodDelete, ""},
	} {
		rec, _ := call(t, h, friendID, tc.method, "/api/v1/subscriptions/"+id, tc.body)
		if rec.Code != http.StatusNotFound {
			t.Errorf("%s as foreign user: status = %d, want 404", tc.method, rec.Code)
		}
	}

	// And the owner still has it, untouched.
	rec, got := call(t, h, ownerID, http.MethodGet, "/api/v1/subscriptions/"+id, "")
	if rec.Code != http.StatusOK || got["status"] != "active" {
		t.Errorf("owner lost access or row mutated: status=%d body=%s", rec.Code, rec.Body)
	}
}

func TestCreateValidation(t *testing.T) {
	h := newTestAPI(t)

	tests := []struct {
		name  string
		body  string
		field string
	}{
		{"unknown airport", `{"origin":"QQQ","destinations":["BCN"],"date_from":"2030-07-01","date_to":"2030-07-15","max_price_minor":15000}`, "origin"},
		{"no destinations", `{"origin":"BEG","destinations":[],"date_from":"2030-07-01","date_to":"2030-07-15","max_price_minor":15000}`, "destinations"},
		{"inverted dates", `{"origin":"BEG","destinations":["BCN"],"date_from":"2030-07-15","date_to":"2030-07-01","max_price_minor":15000}`, "date_to"},
		{"past window", `{"origin":"BEG","destinations":["BCN"],"date_from":"2020-07-01","date_to":"2020-07-15","max_price_minor":15000}`, "date_to"},
		{"threshold strategy without ceiling", `{"origin":"BEG","destinations":["BCN"],"date_from":"2030-07-01","date_to":"2030-07-15"}`, "max_price_minor"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec, resp := call(t, h, ownerID, http.MethodPost, "/api/v1/subscriptions", tt.body)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400; body %s", rec.Code, rec.Body)
			}
			if !strings.Contains(rec.Body.String(), tt.field) {
				t.Errorf("400 body does not mention field %q: %v", tt.field, resp)
			}
		})
	}

	t.Run("malformed date is a 400, not a 500", func(t *testing.T) {
		rec, _ := call(t, h, ownerID, http.MethodPost, "/api/v1/subscriptions",
			`{"origin":"BEG","destinations":["BCN"],"date_from":"01.07.2030","date_to":"2030-07-15"}`)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("status = %d, want 400", rec.Code)
		}
	})
}

func TestPatchValidation(t *testing.T) {
	h := newTestAPI(t)
	_, created := call(t, h, ownerID, http.MethodPost, "/api/v1/subscriptions", validCreateBody)
	id := created["id"].(string)

	t.Run("null on a required field", func(t *testing.T) {
		rec, _ := call(t, h, ownerID, http.MethodPatch, "/api/v1/subscriptions/"+id, `{"origin":null}`)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("status = %d, want 400", rec.Code)
		}
	})

	t.Run("merged result is revalidated", func(t *testing.T) {
		rec, _ := call(t, h, ownerID, http.MethodPatch, "/api/v1/subscriptions/"+id, `{"destinations":["QQQ"]}`)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("status = %d, want 400", rec.Code)
		}
	})

	t.Run("unknown status", func(t *testing.T) {
		rec, _ := call(t, h, ownerID, http.MethodPatch, "/api/v1/subscriptions/"+id, `{"status":"zombie"}`)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("status = %d, want 400", rec.Code)
		}
	})

	t.Run("malformed id reads as 404", func(t *testing.T) {
		rec, _ := call(t, h, ownerID, http.MethodPatch, "/api/v1/subscriptions/not-a-uuid", `{"status":"paused"}`)
		if rec.Code != http.StatusNotFound {
			t.Errorf("status = %d, want 404", rec.Code)
		}
	})
}

func TestAirportSearch(t *testing.T) {
	h := newTestAPI(t)

	rec := callRaw(t, h, "tma "+signedInitData(t, testBotToken, ownerID, time.Now()),
		http.MethodGet, "/api/v1/airports?q=belgra", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	var results []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &results); err != nil {
		t.Fatalf("bad JSON: %v", err)
	}
	if len(results) == 0 || results[0]["iata"] != "BEG" {
		t.Errorf("q=belgra: got %v, want BEG first", results)
	}

	rec = callRaw(t, h, "tma "+signedInitData(t, testBotToken, ownerID, time.Now()),
		http.MethodGet, "/api/v1/airports?q=b", "")
	if body := strings.TrimSpace(rec.Body.String()); body != "[]" {
		t.Errorf("short query: body = %s, want []", body)
	}
}
