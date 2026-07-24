// Package web wires the HTTP surface of the service: liveness/readiness
// (/health), Prometheus (/metrics), and — from Phase 0 step 4 — the embedded
// Mini App SPA plus the /api/v1 JSON API.
package web

import (
	"context"
	"encoding/json"
	"io/fs"
	"log/slog"
	"net/http"
	"path"
	"strings"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	webui "github.com/kaayran/air-tickets-warden/web"
)

// HealthCheck is a named readiness probe (e.g. a DB ping). It should respect the
// context deadline and return a non-nil error when the dependency is unhealthy.
type HealthCheck struct {
	Name  string
	Check func(context.Context) error
}

// Server assembles the HTTP handler tree.
type Server struct {
	log    *slog.Logger
	reg    *prometheus.Registry
	checks []HealthCheck
}

// New constructs a Server. Additional readiness checks (DB, etc.) are attached
// later via WithHealthCheck as the corresponding subsystems come online.
func New(log *slog.Logger, reg *prometheus.Registry) *Server {
	return &Server{log: log, reg: reg}
}

// WithHealthCheck registers a readiness probe surfaced by /health.
func (s *Server) WithHealthCheck(name string, check func(context.Context) error) *Server {
	s.checks = append(s.checks, HealthCheck{Name: name, Check: check})
	return s
}

// Handler builds the public-facing app handler (served behind Caddy in prod):
// /health plus the embedded Mini App SPA (with history-fallback to index.html).
// The /api/v1 subtree is mounted alongside this by the caller.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", s.handleHealth)
	mux.Handle("/", s.spaHandler())
	return mux
}

// spaHandler serves the embedded Vite build. Unknown paths fall back to
// index.html so client-side routing works. When the frontend has not been built
// (only the .gitkeep placeholder is embedded) it returns a clear 503 notice.
func (s *Server) spaHandler() http.Handler {
	dist, err := fs.Sub(webui.DistFS, "dist")
	if err != nil {
		s.log.Error("mount embedded dist", "err", err)
		return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "frontend unavailable", http.StatusInternalServerError)
		})
	}
	if _, err := fs.Stat(dist, "index.html"); err != nil {
		s.log.Warn("mini app not built; serving notice", "err", err)
	}
	return spaHandlerFor(dist)
}

// spaHandlerFor implements the SPA serving logic over any filesystem — split
// from spaHandler so tests can exercise it with an fstest.MapFS instead of the
// build-dependent embedded dist.
func spaHandlerFor(dist fs.FS) http.Handler {
	index, err := fs.ReadFile(dist, "index.html")
	if err != nil {
		return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "Mini App not built. Run `make web-build`.", http.StatusServiceUnavailable)
		})
	}

	fileServer := http.FileServerFS(dist)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := path.Clean(strings.TrimPrefix(r.URL.Path, "/"))
		if p == "." || p == "" {
			serveIndex(w, index)
			return
		}
		if _, statErr := fs.Stat(dist, p); statErr != nil {
			serveIndex(w, index) // SPA history fallback
			return
		}
		fileServer.ServeHTTP(w, r)
	})
}

func serveIndex(w http.ResponseWriter, index []byte) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(index)
}

// MetricsHandler serves the internal (non-public) port: Prometheus /metrics
// and the detailed /health with per-check errors. The public /health carries no
// dependency details — those would leak host/user names to anyone with the URL.
func (s *Server) MetricsHandler() http.Handler {
	mux := http.NewServeMux()
	mux.Handle("GET /metrics", promhttp.HandlerFor(s.reg, promhttp.HandlerOpts{}))
	mux.HandleFunc("GET /health", s.handleHealthDetailed)
	return mux
}

// runChecks executes every registered readiness probe, logging failures so a
// broken dependency is visible in logs regardless of who polls which endpoint.
func (s *Server) runChecks(ctx context.Context) (results map[string]string, ok bool) {
	results = make(map[string]string, len(s.checks))
	ok = true
	for _, c := range s.checks {
		if err := c.Check(ctx); err != nil {
			ok = false
			results[c.Name] = err.Error()
			s.log.Warn("health check failed", "check", c.Name, "err", err)
			continue
		}
		results[c.Name] = "ok"
	}
	return results, ok
}

// handleHealth is the public probe (uptime monitoring, deploy smoke tests):
// status only, no dependency details.
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	_, ok := s.runChecks(r.Context())
	writeHealth(w, ok, nil)
}

// handleHealthDetailed is the internal-port variant with per-check results.
func (s *Server) handleHealthDetailed(w http.ResponseWriter, r *http.Request) {
	results, ok := s.runChecks(r.Context())
	writeHealth(w, ok, results)
}

func writeHealth(w http.ResponseWriter, ok bool, checks map[string]string) {
	status := http.StatusOK
	if !ok {
		status = http.StatusServiceUnavailable
	}
	body := map[string]any{
		"status": map[bool]string{true: "ok", false: "unavailable"}[ok],
	}
	if checks != nil {
		body["checks"] = checks
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
