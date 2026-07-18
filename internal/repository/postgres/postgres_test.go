package postgres

import (
	"context"
	"fmt"
	"os"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/zrma/sema/internal/domain"
	"github.com/zrma/sema/internal/repository"
	"github.com/zrma/sema/internal/repository/repositorytest"
)

const testDSNEnvironment = "SEMA_POSTGRES_TEST_DSN"

var testSchemaSequence atomic.Uint64

func TestPostgresRepositoryConformance(t *testing.T) {
	dsn := os.Getenv(testDSNEnvironment)
	if dsn == "" {
		t.Skip(testDSNEnvironment + " is not set")
	}
	repositorytest.Run(t, postgresFactory(dsn))
}

func TestPostgresSeparatePoolsShareOrderedAuthority(t *testing.T) {
	dsn := os.Getenv(testDSNEnvironment)
	if dsn == "" {
		t.Skip(testDSNEnvironment + " is not set")
	}
	left, reopen := postgresFactory(dsn)(t)
	right := reopen()
	keys := []repository.Key{
		{Scope: "tenant-a", Kind: "match_ticket", ID: "left"},
		{Scope: "tenant-a", Kind: "match_ticket", ID: "right"},
	}
	seeded, err := left.Commit(context.Background(), postgresTestOperation("seed"), []repository.Mutation{
		{Key: keys[0], Payload: []byte("left-r1")},
		{Key: keys[1], Payload: []byte("right-r1")},
	})
	if err != nil {
		t.Fatal(err)
	}

	start := make(chan struct{})
	results := make(chan repository.CommitResult, 2)
	errors := make(chan error, 2)
	var group sync.WaitGroup
	for index, owner := range []repository.Repository{left, right} {
		index, owner := index, owner
		group.Add(1)
		go func() {
			defer group.Done()
			<-start
			result, commitErr := owner.Commit(
				context.Background(),
				postgresTestOperation(domain.OperationID(fmt.Sprintf("advance-%d", index))),
				[]repository.Mutation{{
					Key: keys[index], ExpectedVersion: seeded.Version, Payload: []byte("advanced"),
				}},
			)
			if commitErr != nil {
				errors <- commitErr
				return
			}
			results <- result
		}()
	}
	close(start)
	group.Wait()
	close(results)
	close(errors)
	for err := range errors {
		t.Fatal(err)
	}
	versions := make([]repository.Version, 0, 2)
	for result := range results {
		versions = append(versions, result.Version)
	}
	slices.Sort(versions)
	if len(versions) != 2 || versions[0] != seeded.Version+1 || versions[1] != seeded.Version+2 {
		t.Fatalf("separate pool versions = %v; want consecutive versions after %d", versions, seeded.Version)
	}
	snapshot, err := left.Snapshot(context.Background(), "tenant-a")
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Version != versions[1] {
		t.Fatalf("snapshot version = %d; want %d", snapshot.Version, versions[1])
	}
	audit, err := right.Audit(context.Background(), "tenant-a", 0, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(audit) != 3 || audit[0].Version != seeded.Version ||
		audit[1].Version != versions[0] || audit[2].Version != versions[1] {
		t.Fatalf("ordered audit = %#v", audit)
	}
}

func BenchmarkPostgresRepository(b *testing.B) {
	dsn := os.Getenv(testDSNEnvironment)
	if dsn == "" {
		b.Skip(testDSNEnvironment + " is not set")
	}
	repositorytest.BenchmarkAdapter(b, postgresFactory(dsn))
}

func postgresFactory(dsn string) repositorytest.Factory {
	return func(t testing.TB) (repository.Repository, func() repository.Repository) {
		t.Helper()
		schema := fmt.Sprintf("sema_repository_test_%d", testSchemaSequence.Add(1))
		admin, err := pgxpool.New(context.Background(), dsn)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := admin.Exec(
			context.Background(),
			"CREATE SCHEMA "+pgx.Identifier{schema}.Sanitize(),
		); err != nil {
			admin.Close()
			t.Fatal(err)
		}
		t.Cleanup(func() {
			_, _ = admin.Exec(
				context.Background(),
				"DROP SCHEMA IF EXISTS "+pgx.Identifier{schema}.Sanitize()+" CASCADE",
			)
			admin.Close()
		})

		open := func() repository.Repository {
			pool := openSchemaPool(t, dsn, schema)
			if err := Migrate(context.Background(), pool); err != nil {
				t.Fatal(err)
			}
			owner, err := New(pool)
			if err != nil {
				t.Fatal(err)
			}
			return owner
		}
		return open(), open
	}
}

func openSchemaPool(t testing.TB, dsn, schema string) *pgxpool.Pool {
	t.Helper()
	config, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	config.ConnConfig.RuntimeParams["search_path"] = schema
	pool, err := pgxpool.NewWithConfig(context.Background(), config)
	if err != nil {
		t.Fatal(err)
	}
	if err := pool.Ping(context.Background()); err != nil {
		pool.Close()
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func postgresTestOperation(id domain.OperationID) repository.Operation {
	return repository.Operation{
		Scope: "tenant-a", ID: id, Kind: "postgres_test",
		Digest: repository.Digest([]byte(id)),
		At:     time.Date(2026, 7, 18, 0, 0, 0, 0, time.UTC),
	}
}
