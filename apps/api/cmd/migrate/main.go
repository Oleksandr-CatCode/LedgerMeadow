// Command migrate applies db/migrations against DATABASE_URL and exits.
// It is a separate binary from the API so migrations stay a deliberate deploy step.
package main

import (
	"context"
	"log/slog"
	"os"
	"time"

	"ledgermeadow/src/modules/platform/postgres"
)

const defaultMigrationsDirectory = "db/migrations"

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		logger.Error("DATABASE_URL is required", "operation", "migrate")
		os.Exit(1)
	}
	directory := os.Getenv("MIGRATIONS_DIR")
	if directory == "" {
		directory = defaultMigrationsDirectory
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	pool, err := postgres.Open(ctx, databaseURL)
	if err != nil {
		logger.Error("database connection failed", "operation", "migrate", "error_code", "DATABASE_CONNECTION_FAILED")
		os.Exit(1)
	}
	defer pool.Close()

	applied, err := postgres.Migrate(ctx, pool, directory)
	if err != nil {
		logger.Error("migration failed", "operation", "migrate", "applied", applied, "error_code", "MIGRATION_FAILED")
		os.Exit(1)
	}
	logger.Info("migrations up to date", "operation", "migrate", "applied", applied, "count", len(applied))
}
