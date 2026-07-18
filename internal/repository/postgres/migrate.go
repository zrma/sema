package postgres

import (
	"context"
	_ "embed"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	schemaVersion   = "1"
	statementMarker = "-- sema:statement"
)

//go:embed schema.sql
var schemaSQL string

// Migrate installs the repository-owned schema. Service startup does not call
// it implicitly; deployment must run migrations before opening traffic.
func Migrate(ctx context.Context, pool *pgxpool.Pool) error {
	if pool == nil {
		return fmt.Errorf("postgres repository pool is required")
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin postgres repository migration: %w", err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()

	for _, statement := range strings.Split(schemaSQL, statementMarker) {
		statement = strings.TrimSpace(statement)
		if statement == "" {
			continue
		}
		if _, err := tx.Exec(ctx, statement); err != nil {
			return fmt.Errorf("apply postgres repository schema: %w", err)
		}
	}
	if err := checkSchemaVersion(ctx, tx); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit postgres repository migration: %w", err)
	}
	return nil
}

type schemaVersionReader interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

func checkSchemaVersion(ctx context.Context, reader schemaVersionReader) error {
	var actual string
	if err := reader.QueryRow(
		ctx,
		"SELECT value FROM sema_repository_metadata WHERE key = 'schema_version'",
	).Scan(&actual); err != nil {
		return fmt.Errorf("read postgres repository schema version: %w", err)
	}
	if actual != schemaVersion {
		return fmt.Errorf("postgres repository schema version is %q; want %q", actual, schemaVersion)
	}
	return nil
}
