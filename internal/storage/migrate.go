package storage

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres" // postgres driver
	_ "github.com/golang-migrate/migrate/v4/source/file"       // file:// source
)

// migrationsDir resolves the migrations directory.
// Precedence: ZERELIO_MIGRATIONS_DIR env var → path relative to this source file.
func migrationsDir() string {
	if v := os.Getenv("ZERELIO_MIGRATIONS_DIR"); v != "" {
		return v
	}
	// When running from a built binary, use the env var.
	// In development (go run / go test), resolve relative to this file.
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		return "db/migrations"
	}
	return filepath.Join(filepath.Dir(file), "..", "..", "db", "migrations")
}

// Migrate runs all pending up migrations against the PGBackend's database.
// Safe to call on every startup — idempotent, skips already-applied versions.
// Set ZERELIO_MIGRATIONS_DIR to override the default migration path.
func Migrate(_ context.Context, pgb *PGBackend, logger *slog.Logger) error {
	dir := migrationsDir()

	m, err := migrate.New("file://"+dir, pgb.dsn)
	if err != nil {
		return fmt.Errorf("migrate init (dir=%s): %w", dir, err)
	}
	defer m.Close()

	before, _, _ := m.Version()

	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		return fmt.Errorf("schema migration failed: %w", err)
	}

	after, _, _ := m.Version()
	if after != before {
		logger.Info("schema migrations applied", "from", before, "to", after)
	} else {
		logger.Debug("schema up to date", "version", after)
	}
	return nil
}
