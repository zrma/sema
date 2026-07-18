package service_test

import (
	"context"
	"fmt"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	postgresrepository "github.com/zrma/sema/internal/repository/postgres"
	"github.com/zrma/sema/internal/service"
)

const legacyImportPostgresDSNEnvironment = "SEMA_POSTGRES_TEST_DSN"

var legacyImportSchemaSequence atomic.Uint64

func TestLegacyImporterPostgresCompletionSurvivesReopen(t *testing.T) {
	dsn := os.Getenv(legacyImportPostgresDSNEnvironment)
	if dsn == "" {
		t.Skip(legacyImportPostgresDSNEnvironment + " is not set")
	}
	schema := fmt.Sprintf("sema_legacy_import_test_%d", legacyImportSchemaSequence.Add(1))
	admin, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(admin.Close)
	if _, err := admin.Exec(
		context.Background(), "CREATE SCHEMA "+pgx.Identifier{schema}.Sanitize(),
	); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = admin.Exec(
			context.Background(), "DROP SCHEMA IF EXISTS "+pgx.Identifier{schema}.Sanitize()+" CASCADE",
		)
	})

	configuration, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	configuration.ConnConfig.RuntimeParams["search_path"] = schema
	pool, err := pgxpool.NewWithConfig(context.Background(), configuration)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	if err := postgresrepository.Migrate(context.Background(), pool); err != nil {
		t.Fatal(err)
	}
	owner, err := postgresrepository.New(pool)
	if err != nil {
		t.Fatal(err)
	}
	path := createLegacyLifecycleJournal(t)
	importer, err := service.NewLegacyImporter(owner, service.LegacyImportOptions{
		Now: func() time.Time { return demandFixtureNow.Add(time.Hour) }, BatchSize: 2,
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := importer.Import(context.Background(), "tenant-a", "postgres-legacy-import", path)
	if err != nil {
		t.Fatal(err)
	}
	reopened, err := postgresrepository.New(pool)
	if err != nil {
		t.Fatal(err)
	}
	status, err := service.RequireLegacyImportCompleted(
		context.Background(), reopened, "tenant-a", "postgres-legacy-import", result.Status.SourceDigest,
	)
	if err != nil || status.State != service.LegacyImportCompleted ||
		status.StorageVersion != result.Status.StorageVersion {
		t.Fatalf("reopened PostgreSQL import status = %#v err=%v; result=%#v", status, err, result)
	}
	assignments, err := service.NewAssignments(reopened, func() time.Time { return demandFixtureNow })
	if err != nil {
		t.Fatal(err)
	}
	assignment, exists, err := assignments.Get(context.Background(), "tenant-a", "legacy-assignment")
	if err != nil || !exists || assignment.Assignment.Acknowledgment == nil {
		t.Fatalf("reopened PostgreSQL assignment = %#v exists=%t err=%v", assignment, exists, err)
	}
}
