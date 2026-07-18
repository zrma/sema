package repository

import (
	"cmp"
	"context"
	"crypto/sha256"
	"slices"
	"sync"

	"github.com/zrma/sema/internal/domain"
)

const maxAuditLimit = 1000

type operationReceipt struct {
	digest [sha256.Size]byte
	result CommitResult
}

type operationKey struct {
	scope string
	id    domain.OperationID
}

type scopedAuditRecord struct {
	scope  string
	record AuditRecord
}

// MemoryBackend owns in-memory repository state independently from a handle.
// Reopening a handle against the same backend exercises adapter restart semantics.
type MemoryBackend struct {
	mu sync.Mutex

	version       Version
	scopeVersions map[string]Version
	resources     map[Key]Resource
	operations    map[operationKey]operationReceipt
	audit         []scopedAuditRecord
}

func NewMemoryBackend() *MemoryBackend {
	return &MemoryBackend{
		scopeVersions: make(map[string]Version),
		resources:     make(map[Key]Resource),
		operations:    make(map[operationKey]operationReceipt),
	}
}

// Memory is the reference adapter for repository conformance tests.
type Memory struct {
	backend *MemoryBackend
}

func OpenMemory(backend *MemoryBackend) *Memory {
	if backend == nil {
		backend = NewMemoryBackend()
	}
	return &Memory{backend: backend}
}

func NewMemory() *Memory {
	return OpenMemory(NewMemoryBackend())
}

func (memory *Memory) Snapshot(ctx context.Context, scope string) (Snapshot, error) {
	if err := ctx.Err(); err != nil {
		return Snapshot{}, err
	}
	if scope == "" {
		return Snapshot{}, domain.NewFailure(domain.FailureInvalidInput, "snapshot scope is required")
	}
	memory.backend.mu.Lock()
	defer memory.backend.mu.Unlock()

	keys := make([]Key, 0, len(memory.backend.resources))
	for key := range memory.backend.resources {
		if key.Scope != scope {
			continue
		}
		keys = append(keys, key)
	}
	slices.SortFunc(keys, compareKey)
	snapshot := Snapshot{Version: memory.backend.scopeVersions[scope], Resources: make([]Resource, len(keys))}
	for index, key := range keys {
		snapshot.Resources[index] = cloneResource(memory.backend.resources[key])
	}
	return snapshot, nil
}

func (memory *Memory) Commit(
	ctx context.Context,
	operation Operation,
	mutations []Mutation,
) (CommitResult, error) {
	if err := ctx.Err(); err != nil {
		return CommitResult{}, err
	}
	normalized, err := validateAndNormalize(operation, mutations)
	if err != nil {
		return CommitResult{}, err
	}

	memory.backend.mu.Lock()
	defer memory.backend.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return CommitResult{}, err
	}
	receiptKey := operationKey{scope: operation.Scope, id: operation.ID}
	if receipt, exists := memory.backend.operations[receiptKey]; exists {
		if receipt.digest != operation.Digest {
			return CommitResult{}, domain.NewFailure(
				domain.FailureIdempotencyConflict,
				"operation ID %q was used for another command",
				operation.ID,
			)
		}
		result := receipt.result
		result.Replayed = true
		return result, nil
	}

	for _, mutation := range normalized {
		actual := Version(0)
		if resource, exists := memory.backend.resources[mutation.Key]; exists {
			actual = resource.Version
		}
		if actual != mutation.ExpectedVersion {
			return CommitResult{}, &Conflict{
				Key: mutation.Key, Expected: mutation.ExpectedVersion, Actual: actual,
			}
		}
	}

	memory.backend.version++
	version := memory.backend.version
	memory.backend.scopeVersions[operation.Scope] = version
	resourceCounts := make(map[string]int)
	for _, mutation := range normalized {
		resourceCounts[mutation.Key.Kind]++
		memory.backend.resources[mutation.Key] = Resource{
			Key: mutation.Key, Version: version,
			Payload: slices.Clone(mutation.Payload), Deleted: mutation.Delete,
		}
	}
	record := AuditRecord{
		Version: version, OperationKind: operation.Kind, At: operation.At, ResourceCounts: resourceCounts,
	}
	result := CommitResult{Version: version}
	memory.backend.operations[receiptKey] = operationReceipt{
		digest: operation.Digest, result: result,
	}
	memory.backend.audit = append(memory.backend.audit, scopedAuditRecord{scope: operation.Scope, record: record})
	return result, nil
}

func (memory *Memory) Audit(ctx context.Context, scope string, after Version, limit int) ([]AuditRecord, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if limit <= 0 || limit > maxAuditLimit {
		return nil, domain.NewFailure(
			domain.FailureInvalidInput,
			"audit limit must be between 1 and %d",
			maxAuditLimit,
		)
	}
	if scope == "" {
		return nil, domain.NewFailure(domain.FailureInvalidInput, "audit scope is required")
	}
	memory.backend.mu.Lock()
	defer memory.backend.mu.Unlock()

	page := make([]AuditRecord, 0, min(limit, len(memory.backend.audit)))
	for _, scoped := range memory.backend.audit {
		if scoped.scope != scope || scoped.record.Version <= after {
			continue
		}
		page = append(page, scoped.record)
		if len(page) == limit {
			break
		}
	}
	return CloneAudit(page), nil
}

func validateAndNormalize(operation Operation, mutations []Mutation) ([]Mutation, error) {
	if operation.Scope == "" || operation.ID == "" || operation.Kind == "" || operation.At.IsZero() {
		return nil, domain.NewFailure(
			domain.FailureInvalidInput,
			"operation scope, identity, kind, and server time are required",
		)
	}
	if operation.Digest == ([sha256.Size]byte{}) {
		return nil, domain.NewFailure(domain.FailureInvalidInput, "operation digest is required")
	}
	if len(mutations) == 0 {
		return nil, domain.NewFailure(domain.FailureInvalidInput, "transaction needs at least one mutation")
	}
	normalized := CloneMutations(mutations)
	slices.SortFunc(normalized, func(left, right Mutation) int {
		return compareKey(left.Key, right.Key)
	})
	for index, mutation := range normalized {
		if mutation.Key.Scope == "" || mutation.Key.Kind == "" || mutation.Key.ID == "" {
			return nil, domain.NewFailure(
				domain.FailureInvalidInput,
				"resource scope, kind, and identity are required",
			)
		}
		if mutation.Key.Scope != operation.Scope {
			return nil, domain.NewFailure(
				domain.FailureInvalidInput,
				"operation scope %q cannot mutate resource scope %q",
				operation.Scope,
				mutation.Key.Scope,
			)
		}
		if index > 0 && mutation.Key == normalized[index-1].Key {
			return nil, domain.NewFailure(
				domain.FailureInvalidInput,
				"resource %s/%s/%s is mutated more than once",
				mutation.Key.Scope,
				mutation.Key.Kind,
				mutation.Key.ID,
			)
		}
		if mutation.Delete && len(mutation.Payload) != 0 {
			return nil, domain.NewFailure(domain.FailureInvalidInput, "deleted resource cannot carry a payload")
		}
		if !mutation.Delete && len(mutation.Payload) == 0 {
			return nil, domain.NewFailure(domain.FailureInvalidInput, "live resource payload is required")
		}
	}
	return normalized, nil
}

func compareKey(left, right Key) int {
	if result := cmp.Compare(left.Scope, right.Scope); result != 0 {
		return result
	}
	if result := cmp.Compare(left.Kind, right.Kind); result != 0 {
		return result
	}
	return cmp.Compare(left.ID, right.ID)
}

var _ Repository = (*Memory)(nil)
