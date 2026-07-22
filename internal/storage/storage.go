// Package storage owns the database: the pgx connection pool, embedded goose
// migrations, and the sqlc-generated query layer.
package storage

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib" // database/sql driver for goose
	"github.com/pressly/goose/v3"

	"github.com/kaayran/air-tickets-warden/db"
	"github.com/kaayran/air-tickets-warden/internal/storage/sqlcgen"
)

// Store bundles the connection pool and the typed query layer.
type Store struct {
	Pool    *pgxpool.Pool
	Queries *sqlcgen.Queries
}

// Connect opens the pgx pool and verifies connectivity.
func Connect(ctx context.Context, dsn string) (*Store, error) {
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, fmt.Errorf("create pgx pool: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}
	return &Store{Pool: pool, Queries: sqlcgen.New(pool)}, nil
}

// Close releases the pool.
func (s *Store) Close() { s.Pool.Close() }

// Ping is the readiness probe surfaced by /health.
func (s *Store) Ping(ctx context.Context) error { return s.Pool.Ping(ctx) }

// Migrate applies all pending goose migrations from the embedded filesystem.
// It uses a short-lived database/sql connection (goose's interface) independent
// of the app pool.
func Migrate(ctx context.Context, dsn string) error {
	sqlDB, err := sql.Open("pgx", dsn)
	if err != nil {
		return fmt.Errorf("open sql db for migrations: %w", err)
	}
	defer func() { _ = sqlDB.Close() }()

	goose.SetBaseFS(db.MigrationsFS)
	goose.SetLogger(goose.NopLogger())
	if err := goose.SetDialect("postgres"); err != nil {
		return fmt.Errorf("set goose dialect: %w", err)
	}
	if err := goose.UpContext(ctx, sqlDB, "migrations"); err != nil {
		return fmt.Errorf("apply migrations: %w", err)
	}
	return nil
}
