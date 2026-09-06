package postgres

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// migrationAdvisoryLockID serialises migration runs across replicas.
const migrationAdvisoryLockID int64 = 8202541

// migrationVersionPlaceholder is the psql variable that db/migrations/*.sql use to
// record themselves in schema_migrations inside their own transaction.
const migrationVersionPlaceholder = `:'migration_version'`

var migrationFileName = regexp.MustCompile(`^[0-9]{4}_[a-z0-9_]+\.sql$`)

// Migrate applies every not-yet-applied file in directory, in filename order, and
// returns the versions it applied. Each file carries its own BEGIN/COMMIT and its own
// schema_migrations insert, so it is sent as a single multi-statement command.
func Migrate(ctx context.Context, pool *pgxpool.Pool, directory string) ([]string, error) {
	versions, err := migrationVersions(directory)
	if err != nil {
		return nil, err
	}

	connection, err := pool.Acquire(ctx)
	if err != nil {
		return nil, fmt.Errorf("acquire migration connection: %w", err)
	}
	defer connection.Release()

	if _, err := connection.Exec(ctx, `SELECT pg_advisory_lock($1)`, migrationAdvisoryLockID); err != nil {
		return nil, fmt.Errorf("acquire migration lock: %w", err)
	}
	defer connection.Exec(context.WithoutCancel(ctx), `SELECT pg_advisory_unlock($1)`, migrationAdvisoryLockID)

	if _, err := connection.Exec(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
    version TEXT PRIMARY KEY,
    applied_at TIMESTAMPTZ NOT NULL DEFAULT now()
)`); err != nil {
		return nil, fmt.Errorf("create schema_migrations: %w", err)
	}

	var applied []string
	for _, version := range versions {
		var exists int
		err := connection.QueryRow(ctx, `SELECT 1 FROM schema_migrations WHERE version = $1`, version).Scan(&exists)
		if err == nil {
			continue
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return applied, fmt.Errorf("check migration %s: %w", version, err)
		}

		statements, err := os.ReadFile(filepath.Join(directory, version))
		if err != nil {
			return applied, fmt.Errorf("read migration %s: %w", version, err)
		}
		// The file name is constrained to migrationFileName, so it cannot carry a quote.
		sql := strings.ReplaceAll(string(statements), migrationVersionPlaceholder, "'"+version+"'")
		if _, err := connection.Exec(ctx, sql); err != nil {
			return applied, fmt.Errorf("apply migration %s: %w", version, err)
		}
		applied = append(applied, version)
	}
	return applied, nil
}

func migrationVersions(directory string) ([]string, error) {
	// os.ReadDir returns entries sorted by file name, which is the migration order.
	entries, err := os.ReadDir(directory)
	if err != nil {
		return nil, fmt.Errorf("read migration directory: %w", err)
	}
	var versions []string
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}
		if !migrationFileName.MatchString(entry.Name()) {
			return nil, fmt.Errorf("migration file name is not permitted: %s", entry.Name())
		}
		versions = append(versions, entry.Name())
	}
	if len(versions) == 0 {
		return nil, fmt.Errorf("no migrations found in %s", directory)
	}
	return versions, nil
}
