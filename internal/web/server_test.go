package web

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

func builtDist() fstest.MapFS {
	return fstest.MapFS{
		"index.html":    {Data: []byte("<html>app</html>")},
		"assets/app.js": {Data: []byte("console.log('hi')")},
	}
}

func get(t *testing.T, h http.Handler, path string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	return rec
}

func TestSPAHandler(t *testing.T) {
	h := spaHandlerFor(builtDist())

	tests := []struct {
		name       string
		path       string
		wantStatus int
		wantBody   string
	}{
		{name: "root serves index", path: "/", wantStatus: http.StatusOK, wantBody: "<html>app</html>"},
		{name: "existing asset served as-is", path: "/assets/app.js", wantStatus: http.StatusOK, wantBody: "console.log('hi')"},
		{name: "unknown path falls back to index (SPA history)", path: "/subscriptions/123", wantStatus: http.StatusOK, wantBody: "<html>app</html>"},
		{name: "dot-dot traversal falls back to index, no escape", path: "/../../etc/passwd", wantStatus: http.StatusOK, wantBody: "<html>app</html>"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := get(t, h, tt.path)
			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", rec.Code, tt.wantStatus)
			}
			body, _ := io.ReadAll(rec.Body)
			if !strings.Contains(string(body), tt.wantBody) {
				t.Errorf("body = %q, want it to contain %q", body, tt.wantBody)
			}
		})
	}
}

func TestSPAHandlerNotBuilt(t *testing.T) {
	h := spaHandlerFor(fstest.MapFS{".gitkeep": {Data: nil}})
	rec := get(t, h, "/")
	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusServiceUnavailable)
	}
}
