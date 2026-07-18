package targetapi

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"reflect"
	"sync/atomic"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	api "github.com/zrma/sema/internal/api/v0alpha2"
	postgresrepository "github.com/zrma/sema/internal/repository/postgres"
)

const postgresTestDSNEnvironment = "SEMA_POSTGRES_TEST_DSN"

var targetSchemaSequence atomic.Uint64

func TestTargetAPIPostgresComposition(t *testing.T) {
	dsn := os.Getenv(postgresTestDSNEnvironment)
	if dsn == "" {
		t.Skip(postgresTestDSNEnvironment + " is not set")
	}
	schema := fmt.Sprintf("sema_target_api_test_%d", targetSchemaSequence.Add(1))
	admin, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(admin.Close)
	if _, err := admin.Exec(
		context.Background(),
		"CREATE SCHEMA "+pgx.Identifier{schema}.Sanitize(),
	); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = admin.Exec(
			context.Background(),
			"DROP SCHEMA IF EXISTS "+pgx.Identifier{schema}.Sanitize()+" CASCADE",
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
	handler := newTestHandler(t, owner)
	created := requestData[api.MatchTicketMutation](
		t, handler, "tenant-a", "postgres-create", http.MethodPut,
		"/v0alpha2/match-tickets/ticket-postgres", matchTicket("ticket-postgres", 1), http.StatusOK,
	)
	polled := requestData[api.MatchTicketResource](
		t, handler, "tenant-a", "", http.MethodGet,
		"/v0alpha2/match-tickets/ticket-postgres", nil, http.StatusOK,
	)
	if !reflect.DeepEqual(polled.Ticket, created.Resource.Ticket) || polled.StorageVersion != created.Resource.StorageVersion {
		t.Fatalf("PostgreSQL poll = %#v; created=%#v", polled, created)
	}
}
