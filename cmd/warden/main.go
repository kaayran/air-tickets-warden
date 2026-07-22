// Command warden is the Air Tickets Warden service binary.
//
// Subcommands:
//
//	warden run       start the service (default when no subcommand is given)
//	warden migrate   apply DB migrations and exit          (Phase 0 step 3)
//	warden replay    re-run alert strategies over history  (Phase 5)
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/kaayran/air-tickets-warden/internal/api"
	"github.com/kaayran/air-tickets-warden/internal/bot"
	"github.com/kaayran/air-tickets-warden/internal/config"
	"github.com/kaayran/air-tickets-warden/internal/storage"
	"github.com/kaayran/air-tickets-warden/internal/telemetry"
	"github.com/kaayran/air-tickets-warden/internal/web"
)

func main() {
	cmd := "run"
	if len(os.Args) > 1 {
		cmd = os.Args[1]
	}

	switch cmd {
	case "run":
		if err := run(); err != nil {
			slog.Error("service exited with error", "err", err)
			os.Exit(1)
		}
	case "migrate":
		if err := migrate(); err != nil {
			slog.Error("migrate failed", "err", err)
			os.Exit(1)
		}
	default:
		fmt.Fprintf(os.Stderr, "unknown subcommand %q (want: run|migrate)\n", cmd)
		os.Exit(2)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	log := telemetry.Logger(cfg.Observability.LogLevel)
	reg := telemetry.NewRegistry()

	flushSentry, err := telemetry.InitSentry(cfg.Observability.SentryDSN)
	if err != nil {
		return err
	}
	defer flushSentry()

	log.Info("starting warden", "config", cfg)

	// Root context cancelled on SIGINT/SIGTERM.
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// Apply pending migrations, then open the app pool.
	if err := storage.Migrate(ctx, cfg.DatabaseURL); err != nil {
		return err
	}
	store, err := storage.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer store.Close()
	log.Info("database connected and migrated")

	// Bot runs under its own context so shutdown can stop Telegram polling first
	// (design shutdown order: bot → HTTP → in-flight → pool → Sentry).
	tgBot, err := bot.New(cfg.Telegram.BotToken, cfg.PublicURL, cfg.Telegram.AllowedUserIDs, log)
	if err != nil {
		return err
	}
	botCtx, botCancel := context.WithCancel(context.Background())
	defer botCancel()
	botDone := make(chan struct{})
	go func() {
		defer close(botDone)
		tgBot.Start(botCtx)
	}()

	apiHandler := api.New(store, cfg.Telegram.BotToken, cfg.Telegram.AllowedUserIDs, log).Handler()
	srv := web.New(log, reg).WithHealthCheck("database", store.Ping)

	// Root mux: /api/v1/* → API (initData auth); everything else → app (health,
	// and the embedded SPA from step 4).
	root := http.NewServeMux()
	root.Handle("/api/v1/", apiHandler)
	root.Handle("/", srv.Handler())

	appServer := &http.Server{
		Addr:              fmt.Sprintf(":%d", cfg.HTTPPort),
		Handler:           root,
		ReadHeaderTimeout: 10 * time.Second,
	}
	metricsServer := &http.Server{
		Addr:              fmt.Sprintf(":%d", cfg.Observability.MetricsPort),
		Handler:           srv.MetricsHandler(),
		ReadHeaderTimeout: 10 * time.Second,
	}

	// Surface listen errors through this channel so run() returns cleanly.
	errCh := make(chan error, 2)
	go serve(appServer, "app", cfg.HTTPPort, log, errCh)
	go serve(metricsServer, "metrics", cfg.Observability.MetricsPort, log, errCh)

	// Ordered graceful shutdown will grow (bot → HTTP → in-flight → pool →
	// Sentry) as those subsystems land in later steps.
	select {
	case <-ctx.Done():
		log.Info("shutdown signal received")
	case err := <-errCh:
		return err
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	// 1. Stop accepting Telegram updates.
	botCancel()
	select {
	case <-botDone:
	case <-shutdownCtx.Done():
		log.Warn("bot did not stop within deadline")
	}

	// 2. Stop HTTP servers.
	if err := appServer.Shutdown(shutdownCtx); err != nil {
		log.Error("app server shutdown", "err", err)
	}
	if err := metricsServer.Shutdown(shutdownCtx); err != nil {
		log.Error("metrics server shutdown", "err", err)
	}
	log.Info("shutdown complete")
	return nil
}

// migrate applies pending DB migrations and exits (`warden migrate`).
func migrate() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	log := telemetry.Logger(cfg.Observability.LogLevel)
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	if err := storage.Migrate(ctx, cfg.DatabaseURL); err != nil {
		return err
	}
	log.Info("migrations applied")
	return nil
}

func serve(s *http.Server, name string, port int, log *slog.Logger, errCh chan<- error) {
	log.Info("http server listening", "server", name, "port", port)
	if err := s.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		errCh <- fmt.Errorf("%s server: %w", name, err)
	}
}
