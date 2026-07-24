package api

import (
	"encoding/json"
	"testing"
)

// TestOptionalUnmarshal pins the three-state contract of the PATCH body:
// absent key → untouched, explicit null → clear, value → set.
func TestOptionalUnmarshal(t *testing.T) {
	tests := []struct {
		name      string
		body      string
		wantSet   bool
		wantValue *int32
	}{
		{name: "absent key", body: `{}`, wantSet: false, wantValue: nil},
		{name: "explicit null", body: `{"cooldown_hours": null}`, wantSet: true, wantValue: nil},
		{name: "value", body: `{"cooldown_hours": 12}`, wantSet: true, wantValue: ptr(int32(12))},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var req mePatchRequest
			if err := json.Unmarshal([]byte(tt.body), &req); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			got := req.CooldownHours
			if got.Set != tt.wantSet {
				t.Errorf("Set = %v, want %v", got.Set, tt.wantSet)
			}
			switch {
			case tt.wantValue == nil && got.Value != nil:
				t.Errorf("Value = %d, want nil", *got.Value)
			case tt.wantValue != nil && (got.Value == nil || *got.Value != *tt.wantValue):
				t.Errorf("Value = %v, want %d", got.Value, *tt.wantValue)
			}
		})
	}
}

// TestOptionalUnmarshalPartialBody checks that fields absent from a partial
// PATCH stay unset while present ones are captured — the exact bug the old
// PUT-like semantics had.
func TestOptionalUnmarshalPartialBody(t *testing.T) {
	var req mePatchRequest
	if err := json.Unmarshal([]byte(`{"drop_pct": 0.3}`), &req); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if req.CooldownHours.Set || req.StablePriceBandPct.Set {
		t.Error("absent fields must remain Set=false")
	}
	if !req.DropPct.Set || req.DropPct.Value == nil || *req.DropPct.Value != 0.3 {
		t.Errorf("drop_pct = %+v, want Set with 0.3", req.DropPct)
	}
}

func TestOptionalUnmarshalTypeMismatch(t *testing.T) {
	var req mePatchRequest
	if err := json.Unmarshal([]byte(`{"cooldown_hours": "twelve"}`), &req); err == nil {
		t.Error("expected error for wrong JSON type, got nil")
	}
}

func ptr[T any](v T) *T { return &v }
