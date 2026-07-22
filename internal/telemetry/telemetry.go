// Package telemetry wires up the cross-cutting observability primitives: a
// structured slog logger, a Prometheus registry, and Sentry error tracking
// (a no-op when the DSN is empty).
package telemetry

import (
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/getsentry/sentry-go"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
)

// Logger builds a JSON slog logger at the given level and installs it as the
// process default.
func Logger(level string) *slog.Logger {
	var lvl slog.Level
	switch level {
	case "debug":
		lvl = slog.LevelDebug
	case "warn":
		lvl = slog.LevelWarn
	case "error":
		lvl = slog.LevelError
	default:
		lvl = slog.LevelInfo
	}
	h := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: lvl})
	l := slog.New(h)
	slog.SetDefault(l)
	return l
}

// NewRegistry returns a Prometheus registry with the standard Go/process
// collectors registered.
func NewRegistry() *prometheus.Registry {
	reg := prometheus.NewRegistry()
	reg.MustRegister(collectors.NewGoCollector())
	reg.MustRegister(collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}))
	return reg
}

// InitSentry initialises Sentry when a DSN is provided. An empty DSN yields a
// no-op client, so callers can always call sentry.CaptureException safely.
// The returned flush function should be deferred and given a deadline.
func InitSentry(dsn string) (flush func(), err error) {
	if dsn == "" {
		return func() {}, nil
	}
	if err := sentry.Init(sentry.ClientOptions{Dsn: dsn}); err != nil {
		return nil, fmt.Errorf("init sentry: %w", err)
	}
	return func() { sentry.Flush(2 * time.Second) }, nil
}
