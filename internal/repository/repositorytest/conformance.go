// Package repositorytest provides the shared repository adapter conformance suite.
package repositorytest

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/zrma/sema/internal/domain"
	"github.com/zrma/sema/internal/repository"
)

type Factory func(testing.TB) (repository.Repository, func() repository.Repository)

func Run(t *testing.T, factory Factory) {
	t.Helper()
	t.Run("same ticket revision competition", func(t *testing.T) {
		owner, _ := factory(t)
		key := repository.Key{Scope: "tenant-a", Kind: "match_ticket", ID: "ticket-a"}
		create := transaction("create-ticket", "ticket-r1", mutation(key, 0, "revision-1"))
		created, err := owner.Commit(context.Background(), create.operation, create.mutations)
		if err != nil {
			t.Fatal(err)
		}

		attempts := []transactionFixture{
			transaction("replace-left", "ticket-r2-left", mutation(key, created.Version, "revision-2-left")),
			transaction("replace-right", "ticket-r2-right", mutation(key, created.Version, "revision-2-right")),
		}
		start := make(chan struct{})
		results := make(chan error, len(attempts))
		var group sync.WaitGroup
		for _, attempt := range attempts {
			attempt := attempt
			group.Add(1)
			go func() {
				defer group.Done()
				<-start
				_, commitErr := owner.Commit(context.Background(), attempt.operation, attempt.mutations)
				results <- commitErr
			}()
		}
		close(start)
		group.Wait()
		close(results)
		successes, conflicts := 0, 0
		for result := range results {
			switch {
			case result == nil:
				successes++
			case repository.IsConflict(result):
				conflicts++
			default:
				t.Fatalf("competition error = %v", result)
			}
		}
		if successes != 1 || conflicts != 1 {
			t.Fatalf("competition successes=%d conflicts=%d; want 1/1", successes, conflicts)
		}
	})

	t.Run("duplicate operation receipt", func(t *testing.T) {
		owner, _ := factory(t)
		key := repository.Key{Scope: "tenant-a", Kind: "policy", ID: "policy-a"}
		request := transaction("register-policy", "policy-a-v1", mutation(key, 0, "policy"))
		first, err := owner.Commit(context.Background(), request.operation, request.mutations)
		if err != nil {
			t.Fatal(err)
		}
		replayed, err := owner.Commit(context.Background(), request.operation, request.mutations)
		if err != nil {
			t.Fatal(err)
		}
		if !replayed.Replayed || replayed.Version != first.Version {
			t.Fatalf("replayed result = %#v; want version %d replay", replayed, first.Version)
		}

		conflicting := request
		conflicting.operation.Digest = repository.Digest([]byte("different-command"))
		if _, err := owner.Commit(context.Background(), conflicting.operation, conflicting.mutations); failureCode(err) != domain.FailureIdempotencyConflict {
			t.Fatalf("operation conflict = %v; want %s", err, domain.FailureIdempotencyConflict)
		}

		otherScope := transaction("register-policy", "tenant-b-policy", repository.Mutation{
			Key:     repository.Key{Scope: "tenant-b", Kind: "policy", ID: "policy-a"},
			Payload: []byte("tenant-b-policy"),
		})
		otherScope.operation.Scope = "tenant-b"
		otherCommitted, err := owner.Commit(context.Background(), otherScope.operation, otherScope.mutations)
		if err != nil {
			t.Fatalf("tenant-scoped operation ID = %v", err)
		}
		tenantA, err := owner.Snapshot(context.Background(), "tenant-a")
		if err != nil {
			t.Fatal(err)
		}
		tenantB, err := owner.Snapshot(context.Background(), "tenant-b")
		if err != nil {
			t.Fatal(err)
		}
		if tenantA.Version != first.Version || len(tenantA.Resources) != 1 ||
			tenantB.Version != otherCommitted.Version || len(tenantB.Resources) != 1 {
			t.Fatalf("tenant snapshots = A:%#v B:%#v", tenantA, tenantB)
		}

		later := transaction("replace-policy", "policy-a-v2", mutation(key, first.Version, "policy-v2"))
		if _, err := owner.Commit(context.Background(), later.operation, later.mutations); err != nil {
			t.Fatal(err)
		}
		resolved, exists, err := owner.Replay(context.Background(), request.operation)
		if err != nil || !exists || !resolved.Replayed || resolved.Version != first.Version {
			t.Fatalf("historical replay = %#v, exists=%t, err=%v", resolved, exists, err)
		}
		missing := transaction("missing-operation", "missing", mutation(key, first.Version, "unused"))
		if result, exists, err := owner.Replay(context.Background(), missing.operation); err != nil || exists || result != (repository.CommitResult{}) {
			t.Fatalf("missing replay = %#v, exists=%t, err=%v", result, exists, err)
		}
		conflicting.operation.At = later.operation.At
		if _, exists, err := owner.Replay(context.Background(), conflicting.operation); !exists || failureCode(err) != domain.FailureIdempotencyConflict {
			t.Fatalf("resolved operation conflict = exists=%t err=%v", exists, err)
		}
	})

	t.Run("atomic multi resource conflict", func(t *testing.T) {
		owner, _ := factory(t)
		left := repository.Key{Scope: "tenant-a", Kind: "match_ticket", ID: "left"}
		right := repository.Key{Scope: "tenant-a", Kind: "match_ticket", ID: "right"}
		seed := transaction(
			"seed-pair",
			"seed-pair",
			mutation(left, 0, "left-r1"),
			mutation(right, 0, "right-r1"),
		)
		seeded, err := owner.Commit(context.Background(), seed.operation, seed.mutations)
		if err != nil {
			t.Fatal(err)
		}
		advance := transaction("advance-right", "right-r2", mutation(right, seeded.Version, "right-r2"))
		advanced, err := owner.Commit(context.Background(), advance.operation, advance.mutations)
		if err != nil {
			t.Fatal(err)
		}
		reserve := transaction(
			"reserve-pair",
			"reserve-pair",
			mutation(left, seeded.Version, "left-reserved"),
			mutation(right, seeded.Version, "right-reserved"),
		)
		if _, err := owner.Commit(context.Background(), reserve.operation, reserve.mutations); !repository.IsConflict(err) {
			t.Fatalf("atomic conflict = %v; want repository conflict", err)
		}
		snapshot, err := owner.Snapshot(context.Background(), "tenant-a")
		if err != nil {
			t.Fatal(err)
		}
		leftResource := find(t, snapshot, left)
		rightResource := find(t, snapshot, right)
		if string(leftResource.Payload) != "left-r1" || leftResource.Version != seeded.Version {
			t.Fatalf("left partially mutated = %#v", leftResource)
		}
		if string(rightResource.Payload) != "right-r2" || rightResource.Version != advanced.Version {
			t.Fatalf("right resource = %#v", rightResource)
		}
	})

	t.Run("restart replay and audit", func(t *testing.T) {
		owner, reopen := factory(t)
		key := repository.Key{Scope: "tenant-a", Kind: "reservation", ID: "reservation-a"}
		request := transaction("reserve-a", "reserve-a", mutation(key, 0, "active"))
		committed, err := owner.Commit(context.Background(), request.operation, request.mutations)
		if err != nil {
			t.Fatal(err)
		}

		owner = reopen()
		snapshot, err := owner.Snapshot(context.Background(), "tenant-a")
		if err != nil {
			t.Fatal(err)
		}
		if snapshot.Version != committed.Version || string(find(t, snapshot, key).Payload) != "active" {
			t.Fatalf("reopened snapshot = %#v", snapshot)
		}
		replayed, err := owner.Commit(context.Background(), request.operation, request.mutations)
		if err != nil || !replayed.Replayed || replayed.Version != committed.Version {
			t.Fatalf("reopened replay = %#v, %v", replayed, err)
		}
		audit, err := owner.Audit(context.Background(), "tenant-a", 0, 10)
		if err != nil {
			t.Fatal(err)
		}
		if len(audit) != 1 || audit[0].OperationKind != request.operation.Kind ||
			audit[0].Version != committed.Version || audit[0].ResourceCounts["reservation"] != 1 {
			t.Fatalf("reopened audit = %#v", audit)
		}
		if other, err := owner.Audit(context.Background(), "tenant-b", 0, 10); err != nil || len(other) != 0 {
			t.Fatalf("cross-tenant audit = %#v, %v", other, err)
		}
	})

	t.Run("defensive snapshots and tombstones", func(t *testing.T) {
		owner, _ := factory(t)
		key := repository.Key{Scope: "tenant-a", Kind: "backfill_ticket", ID: "backfill-a"}
		create := transaction("create-backfill", "create-backfill", mutation(key, 0, "open"))
		created, err := owner.Commit(context.Background(), create.operation, create.mutations)
		if err != nil {
			t.Fatal(err)
		}
		first, err := owner.Snapshot(context.Background(), "tenant-a")
		if err != nil {
			t.Fatal(err)
		}
		find(t, first, key).Payload[0] = '!'
		again, err := owner.Snapshot(context.Background(), "tenant-a")
		if err != nil {
			t.Fatal(err)
		}
		if string(find(t, again, key).Payload) != "open" {
			t.Fatalf("snapshot mutation leaked: %#v", again)
		}

		remove := transaction("cancel-backfill", "cancel-backfill", repository.Mutation{
			Key: key, ExpectedVersion: created.Version, Delete: true,
		})
		removed, err := owner.Commit(context.Background(), remove.operation, remove.mutations)
		if err != nil {
			t.Fatal(err)
		}
		final, err := owner.Snapshot(context.Background(), "tenant-a")
		if err != nil {
			t.Fatal(err)
		}
		tombstone := find(t, final, key)
		if !tombstone.Deleted || tombstone.Version != removed.Version || len(tombstone.Payload) != 0 {
			t.Fatalf("tombstone = %#v", tombstone)
		}
	})

	t.Run("cancelled context has no side effect", func(t *testing.T) {
		owner, _ := factory(t)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		key := repository.Key{Scope: "tenant-a", Kind: "assignment", ID: "assignment-a"}
		request := transaction("assignment-a", "assignment-a", mutation(key, 0, "pending"))
		if _, err := owner.Commit(ctx, request.operation, request.mutations); !errors.Is(err, context.Canceled) {
			t.Fatalf("cancelled commit = %v; want context cancellation", err)
		}
		snapshot, err := owner.Snapshot(context.Background(), "tenant-a")
		if err != nil {
			t.Fatal(err)
		}
		if snapshot.Version != 0 || len(snapshot.Resources) != 0 {
			t.Fatalf("cancelled commit changed repository: %#v", snapshot)
		}
	})
}

type transactionFixture struct {
	operation repository.Operation
	mutations []repository.Mutation
}

func transaction(operationID, command string, mutations ...repository.Mutation) transactionFixture {
	return transactionFixture{
		operation: repository.Operation{
			Scope: "tenant-a", ID: domain.OperationID(operationID), Kind: operationID,
			Digest: repository.Digest([]byte(command)),
			At:     time.Date(2026, 7, 18, 0, 0, 0, 0, time.UTC),
		},
		mutations: mutations,
	}
}

func mutation(key repository.Key, expected repository.Version, payload string) repository.Mutation {
	return repository.Mutation{Key: key, ExpectedVersion: expected, Payload: []byte(payload)}
}

func find(t testing.TB, snapshot repository.Snapshot, key repository.Key) repository.Resource {
	t.Helper()
	for _, resource := range snapshot.Resources {
		if resource.Key == key {
			return resource
		}
	}
	t.Fatalf("resource %#v not found in %#v", key, snapshot)
	return repository.Resource{}
}

func failureCode(err error) domain.FailureCode {
	code, _ := domain.FailureCodeOf(err)
	return code
}
