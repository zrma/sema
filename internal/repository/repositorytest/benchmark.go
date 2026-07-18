package repositorytest

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/zrma/sema/internal/domain"
	"github.com/zrma/sema/internal/repository"
)

// BenchmarkAdapter measures the same sequential and contended mutation paths
// for every repository adapter. It is comparative evidence, not a product SLO.
func BenchmarkAdapter(b *testing.B, factory Factory) {
	b.Helper()
	b.Run("sequential", func(b *testing.B) {
		owner, _ := factory(b)
		key := repository.Key{Scope: "benchmark", Kind: "match_ticket", ID: "sequential"}
		version := seedBenchmarkResource(b, owner, key)
		b.ReportAllocs()
		b.ResetTimer()
		for index := 0; index < b.N; index++ {
			operation := benchmarkOperation(index + 1)
			result, err := owner.Commit(context.Background(), operation, []repository.Mutation{{
				Key: key, ExpectedVersion: version, Payload: []byte("updated"),
			}})
			if err != nil {
				b.Fatal(err)
			}
			version = result.Version
		}
	})

	b.Run("contended", func(b *testing.B) {
		owner, _ := factory(b)
		key := repository.Key{Scope: "benchmark", Kind: "match_ticket", ID: "contended"}
		seedBenchmarkResource(b, owner, key)
		var sequence atomic.Uint64
		var conflicts atomic.Uint64
		b.ReportAllocs()
		b.ResetTimer()
		b.RunParallel(func(parallel *testing.PB) {
			for parallel.Next() {
				id := sequence.Add(1)
				operation := benchmarkOperation(int(id))
				for {
					snapshot, err := owner.Snapshot(context.Background(), key.Scope)
					if err != nil {
						b.Error(err)
						return
					}
					version := benchmarkResourceVersion(b, snapshot, key)
					_, err = owner.Commit(context.Background(), operation, []repository.Mutation{{
						Key: key, ExpectedVersion: version, Payload: []byte("updated"),
					}})
					if err == nil {
						break
					}
					if !repository.IsConflict(err) {
						b.Error(err)
						return
					}
					conflicts.Add(1)
				}
			}
		})
		b.ReportMetric(float64(conflicts.Load())/float64(max(1, b.N)), "conflicts/op")
	})
}

func seedBenchmarkResource(
	b testing.TB,
	owner repository.Repository,
	key repository.Key,
) repository.Version {
	b.Helper()
	result, err := owner.Commit(context.Background(), benchmarkOperation(0), []repository.Mutation{{
		Key: key, Payload: []byte("seed"),
	}})
	if err != nil {
		b.Fatal(err)
	}
	return result.Version
}

func benchmarkOperation(index int) repository.Operation {
	identity := domain.OperationID(fmt.Sprintf("benchmark-%08d", index))
	return repository.Operation{
		Scope: "benchmark", ID: identity, Kind: "benchmark_commit",
		Digest: repository.Digest([]byte(identity)),
		At:     time.Date(2026, 7, 18, 0, 0, 0, 0, time.UTC),
	}
}

func benchmarkResourceVersion(
	b testing.TB,
	snapshot repository.Snapshot,
	key repository.Key,
) repository.Version {
	b.Helper()
	for _, resource := range snapshot.Resources {
		if resource.Key == key {
			return resource.Version
		}
	}
	b.Fatalf("benchmark resource %#v is missing", key)
	return 0
}
