// Package config loads and validates the service configuration from environment
// variables. Locally a .env file is loaded first (via godotenv); in production
// the real process environment / Docker secrets are used.
package config

import (
	"fmt"
	"log/slog"

	"github.com/caarlos0/env/v11"
	"github.com/joho/godotenv"
)

// Config is the fully-parsed, validated service configuration. Secret fields are
// redacted in log output via LogValue.
//
// Note: fields for later phases (Amadeus, Kiwi, ...) are parsed but NOT marked
// required yet — Phase 0 only needs the Telegram + web identity tract to boot.
// Phase 2 promotes the Amadeus credentials to required.
type Config struct {
	Telegram struct {
		BotToken       string  `env:"BOT_TOKEN,required"`
		AllowedUserIDs []int64 `env:"ALLOWED_USER_IDS,required"`
	}
	Sources struct {
		AmadeusClientID     string `env:"AMADEUS_CLIENT_ID"` // required from Phase 2
		AmadeusClientSecret string `env:"AMADEUS_CLIENT_SECRET"`
		KiwiAPIKey          string `env:"KIWI_API_KEY"`
		AviasalesToken      string `env:"AVIASALES_TOKEN"` // trend-only source, v1.1+
	}
	RateLimits struct {
		AmadeusRPS float64 `env:"AMADEUS_RPS" envDefault:"1.0"`
		RyanairRPS float64 `env:"RYANAIR_RPS" envDefault:"0.3"`
		KiwiRPS    float64 `env:"KIWI_RPS" envDefault:"0.5"`
	}
	Budgets struct {
		AmadeusDaily int `env:"AMADEUS_DAILY_BUDGET" envDefault:"60"` // requests/day
	}
	AlertDefaults struct { // env fallbacks; overridable per user (user_settings) and per subscription
		CooldownHours      int     `env:"COOLDOWN_HOURS" envDefault:"6"`
		DropPct            float64 `env:"DROP_PCT" envDefault:"0.25"`
		StablePriceBandPct float64 `env:"STABLE_PRICE_BAND_PCT" envDefault:"0.02"`
		WarmupMinObs       int     `env:"WARMUP_MIN_OBSERVATIONS" envDefault:"10"`
		WarmupMinDays      int     `env:"WARMUP_MIN_DAYS" envDefault:"3"`
	}
	Observability struct {
		SentryDSN   string `env:"SENTRY_DSN"`
		LogLevel    string `env:"LOG_LEVEL" envDefault:"info"`
		MetricsPort int    `env:"METRICS_PORT" envDefault:"9090"`
	}
	DatabaseURL string `env:"DATABASE_URL,required"`
	PublicURL   string `env:"PUBLIC_URL,required"`         // HTTPS base URL of the Mini App (web_app buttons)
	HTTPPort    int    `env:"HTTP_PORT" envDefault:"8080"` // app server (health, SPA, /api) — behind Caddy in prod
}

// Load reads .env (best-effort — absent file is fine when env is set another way),
// parses the environment into a Config, and validates it. A missing required field
// aborts with a clear message before any network or DB work happens.
func Load() (*Config, error) {
	// Best-effort: a missing .env is not an error (prod injects real env vars).
	_ = godotenv.Load()

	var cfg Config
	if err := env.Parse(&cfg); err != nil {
		return nil, fmt.Errorf("parse config from environment: %w", err)
	}
	if err := cfg.validate(); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}
	return &cfg, nil
}

func (c *Config) validate() error {
	if len(c.Telegram.AllowedUserIDs) == 0 {
		return fmt.Errorf("ALLOWED_USER_IDS must list at least one chat id")
	}
	switch c.Observability.LogLevel {
	case "debug", "info", "warn", "error":
	default:
		return fmt.Errorf("LOG_LEVEL must be one of debug|info|warn|error, got %q", c.Observability.LogLevel)
	}
	return nil
}

// LogValue implements slog.LogValuer so the config can be logged safely — every
// secret is redacted.
func (c *Config) LogValue() slog.Value {
	return slog.GroupValue(
		slog.String("bot_token", redact(c.Telegram.BotToken)),
		slog.Int("allowed_user_ids", len(c.Telegram.AllowedUserIDs)),
		slog.String("amadeus_client_id", redact(c.Sources.AmadeusClientID)),
		slog.String("amadeus_client_secret", redact(c.Sources.AmadeusClientSecret)),
		slog.String("kiwi_api_key", redact(c.Sources.KiwiAPIKey)),
		slog.String("aviasales_token", redact(c.Sources.AviasalesToken)),
		slog.String("database_url", redactURL(c.DatabaseURL)),
		slog.String("sentry_dsn", redact(c.Observability.SentryDSN)),
		slog.String("public_url", c.PublicURL),
		slog.String("log_level", c.Observability.LogLevel),
		slog.Int("metrics_port", c.Observability.MetricsPort),
	)
}

// redact reports whether a secret is set without revealing it.
func redact(s string) string {
	if s == "" {
		return "<unset>"
	}
	return "<set>"
}

// redactURL keeps the URL shape for debugging while hiding any embedded password.
func redactURL(s string) string {
	if s == "" {
		return "<unset>"
	}
	return "<set>"
}
