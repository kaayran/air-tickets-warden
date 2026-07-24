package api

import "testing"

func TestBearerInitData(t *testing.T) {
	tests := []struct {
		name   string
		header string
		want   string
		wantOK bool
	}{
		{name: "valid", header: "tma query_id=abc&user=def", want: "query_id=abc&user=def", wantOK: true},
		{name: "scheme is case-insensitive", header: "TMA data", want: "data", wantOK: true},
		{name: "empty header", header: "", wantOK: false},
		{name: "wrong scheme", header: "Bearer token", wantOK: false},
		{name: "scheme only", header: "tma", wantOK: false},
		{name: "scheme with empty value", header: "tma ", wantOK: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := bearerInitData(tt.header)
			if ok != tt.wantOK || got != tt.want {
				t.Errorf("bearerInitData(%q) = (%q, %v), want (%q, %v)", tt.header, got, ok, tt.want, tt.wantOK)
			}
		})
	}
}
