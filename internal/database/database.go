package database

import (
	"context"
	"fmt"
	"io/fs"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/jackc/pgx/v5/pgxpool"
)

// NewPool creates a pgx connection pool. MaxConns is set to 10 when the DSN
// does not specify pool_max_conns, providing enough headroom for the 5 bar
// workers, the trade-update consumer, and background refresh goroutines.
func NewPool(ctx context.Context, databaseURL string) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse database URL: %w", err)
	}

	const minMaxConns = 10
	if cfg.MaxConns < minMaxConns {
		cfg.MaxConns = minMaxConns
	}

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create database pool: %w", err)
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("failed to connect to the database: %w", err)
	}

	return pool, nil
}

// Migrate applies all pending up-migrations from migrationsFS and returns an
// error if any step fails. ErrNoChange is treated as success.
func Migrate(databaseURL string, migrationsFS fs.FS) error {
	src, err := iofs.New(migrationsFS, ".")
	if err != nil {
		return fmt.Errorf("migrations: create iofs source: %w", err)
	}

	m, err := migrate.NewWithSourceInstance("iofs", src, databaseURL)
	if err != nil {
		return fmt.Errorf("migrations: create migrate instance: %w", err)
	}

	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		return fmt.Errorf("migrations: apply: %w", err)
	}

	return nil
}
