// Package repository defines the adapter-neutral transactional storage contract.
package repository

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/zrma/sema/internal/domain"
)

type Version uint64

// Key is the tenant-scoped identity of one service resource.
type Key struct {
	Scope string
	Kind  string
	ID    string
}

// Resource is one immutable repository snapshot value. Deleted resources remain
// as tombstones so a stale create cannot silently reuse an identity.
type Resource struct {
	Key     Key
	Version Version
	Payload []byte
	Deleted bool
}

// Snapshot is a consistent, defensive view of all resources at one commit version.
type Snapshot struct {
	Version   Version
	Resources []Resource
}

// Operation identifies one retryable service command. Digest must cover the
// canonical command payload; At is supplied by the service clock authority.
type Operation struct {
	Scope  string
	ID     domain.OperationID
	Kind   string
	Digest [sha256.Size]byte
	At     time.Time
}

// Digest returns a stable operation digest for an already canonical payload.
func Digest(payload []byte) [sha256.Size]byte {
	return sha256.Sum256(payload)
}

// ValidateTransaction validates adapter-independent command semantics and
// returns defensive mutations in canonical resource order.
func ValidateTransaction(operation Operation, mutations []Mutation) ([]Mutation, error) {
	return validateAndNormalize(operation, mutations)
}

// Mutation replaces or tombstones one resource if ExpectedVersion still matches.
// Version zero means that no resource or tombstone may exist.
type Mutation struct {
	Key             Key
	ExpectedVersion Version
	Payload         []byte
	Delete          bool
}

type CommitResult struct {
	Version  Version
	Replayed bool
}

// AuditRecord is the redacted durable receipt for one committed operation.
type AuditRecord struct {
	Version        Version
	OperationKind  string
	At             time.Time
	ResourceCounts map[string]int
}

// Repository atomically applies resource-level compare-and-swap mutations.
// A successful operation receipt and all mutations share one commit version.
type Repository interface {
	Snapshot(context.Context, string) (Snapshot, error)
	Replay(context.Context, Operation) (CommitResult, bool, error)
	Commit(context.Context, Operation, []Mutation) (CommitResult, error)
	Audit(context.Context, string, Version, int) ([]AuditRecord, error)
}

// ValidateOperation validates the identity needed to resolve or commit one
// idempotent operation. A service resolves the receipt before validating
// mutable resource state so an older retry still returns its original result.
func ValidateOperation(operation Operation) error {
	if operation.Scope == "" || operation.ID == "" || operation.Kind == "" || operation.At.IsZero() {
		return domain.NewFailure(
			domain.FailureInvalidInput,
			"operation scope, identity, kind, and server time are required",
		)
	}
	if operation.Digest == ([sha256.Size]byte{}) {
		return domain.NewFailure(domain.FailureInvalidInput, "operation digest is required")
	}
	return nil
}

// Conflict reports a resource whose storage version changed after it was read.
type Conflict struct {
	Key      Key
	Expected Version
	Actual   Version
}

func (conflict *Conflict) Error() string {
	return fmt.Sprintf(
		"repository resource %s/%s/%s is at version %d; expected %d",
		conflict.Key.Scope,
		conflict.Key.Kind,
		conflict.Key.ID,
		conflict.Actual,
		conflict.Expected,
	)
}

func IsConflict(err error) bool {
	var conflict *Conflict
	return errors.As(err, &conflict)
}

func CloneSnapshot(snapshot Snapshot) Snapshot {
	cloned := Snapshot{Version: snapshot.Version, Resources: make([]Resource, len(snapshot.Resources))}
	for index, resource := range snapshot.Resources {
		cloned.Resources[index] = cloneResource(resource)
	}
	return cloned
}

func CloneMutations(mutations []Mutation) []Mutation {
	cloned := make([]Mutation, len(mutations))
	for index, mutation := range mutations {
		cloned[index] = mutation
		cloned[index].Payload = slices.Clone(mutation.Payload)
	}
	return cloned
}

func CloneAudit(records []AuditRecord) []AuditRecord {
	cloned := make([]AuditRecord, len(records))
	for index, record := range records {
		cloned[index] = record
		cloned[index].ResourceCounts = make(map[string]int, len(record.ResourceCounts))
		for kind, count := range record.ResourceCounts {
			cloned[index].ResourceCounts[kind] = count
		}
	}
	return cloned
}

func cloneResource(resource Resource) Resource {
	resource.Payload = slices.Clone(resource.Payload)
	return resource
}
