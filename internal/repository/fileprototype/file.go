// Package fileprototype provides a crash-testable persistent reference adapter.
// It rewrites a full checksummed snapshot per commit and is not a production store.
package fileprototype

import (
	"cmp"
	"context"
	"crypto/sha256"
	"slices"
	"sync"

	"github.com/zrma/sema/internal/domain"
	"github.com/zrma/sema/internal/repository"
)

const maxAuditLimit = 1000

type faultPoint string

const (
	faultAfterTempSync faultPoint = "after_temp_sync"
	faultAfterCommit   faultPoint = "after_commit"
)

type faultFunc func(faultPoint)

type operationKey struct {
	scope string
	id    domain.OperationID
}

type operationReceipt struct {
	digest [sha256.Size]byte
	result repository.CommitResult
}

type scopedAuditRecord struct {
	scope  string
	record repository.AuditRecord
}

type state struct {
	version       repository.Version
	scopeVersions map[string]repository.Version
	resources     map[repository.Key]repository.Resource
	operations    map[operationKey]operationReceipt
	audit         []scopedAuditRecord
}

func newState() state {
	return state{
		scopeVersions: make(map[string]repository.Version),
		resources:     make(map[repository.Key]repository.Resource),
		operations:    make(map[operationKey]operationReceipt),
	}
}

// Store persists the complete reference state with atomic file replacement.
// Concurrent calls on one Store are serialized; multiple process writers are
// intentionally unsupported so this adapter remains a decision-gate prototype.
type Store struct {
	mu sync.Mutex

	path  string
	state state
	fault faultFunc
}

func Open(path string) (*Store, error) {
	return open(path, nil)
}

func open(path string, fault faultFunc) (*Store, error) {
	if path == "" {
		return nil, domain.NewFailure(domain.FailureInvalidInput, "repository path is required")
	}
	loaded, err := loadState(path)
	if err != nil {
		return nil, err
	}
	return &Store{path: path, state: loaded, fault: fault}, nil
}

func (store *Store) Replay(
	ctx context.Context,
	operation repository.Operation,
) (repository.CommitResult, bool, error) {
	if err := ctx.Err(); err != nil {
		return repository.CommitResult{}, false, err
	}
	if err := repository.ValidateOperation(operation); err != nil {
		return repository.CommitResult{}, false, err
	}
	store.mu.Lock()
	defer store.mu.Unlock()

	receipt, exists := store.state.operations[operationKey{scope: operation.Scope, id: operation.ID}]
	if !exists {
		return repository.CommitResult{}, false, nil
	}
	if receipt.digest != operation.Digest {
		return repository.CommitResult{}, true, domain.NewFailure(
			domain.FailureIdempotencyConflict,
			"operation ID %q was used for another command",
			operation.ID,
		)
	}
	result := receipt.result
	result.Replayed = true
	return result, true, nil
}

func (store *Store) Snapshot(ctx context.Context, scope string) (repository.Snapshot, error) {
	if err := ctx.Err(); err != nil {
		return repository.Snapshot{}, err
	}
	if scope == "" {
		return repository.Snapshot{}, domain.NewFailure(domain.FailureInvalidInput, "snapshot scope is required")
	}
	store.mu.Lock()
	defer store.mu.Unlock()

	keys := make([]repository.Key, 0, len(store.state.resources))
	for key := range store.state.resources {
		if key.Scope == scope {
			keys = append(keys, key)
		}
	}
	slices.SortFunc(keys, compareKey)
	snapshot := repository.Snapshot{
		Version: store.state.scopeVersions[scope], Resources: make([]repository.Resource, len(keys)),
	}
	for index, key := range keys {
		resource := store.state.resources[key]
		resource.Payload = slices.Clone(resource.Payload)
		snapshot.Resources[index] = resource
	}
	return snapshot, nil
}

func (store *Store) Commit(
	ctx context.Context,
	operation repository.Operation,
	mutations []repository.Mutation,
) (repository.CommitResult, error) {
	if err := ctx.Err(); err != nil {
		return repository.CommitResult{}, err
	}
	normalized, err := repository.ValidateTransaction(operation, mutations)
	if err != nil {
		return repository.CommitResult{}, err
	}

	store.mu.Lock()
	defer store.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return repository.CommitResult{}, err
	}
	receiptKey := operationKey{scope: operation.Scope, id: operation.ID}
	if receipt, exists := store.state.operations[receiptKey]; exists {
		if receipt.digest != operation.Digest {
			return repository.CommitResult{}, domain.NewFailure(
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
		actual := repository.Version(0)
		if resource, exists := store.state.resources[mutation.Key]; exists {
			actual = resource.Version
		}
		if actual != mutation.ExpectedVersion {
			return repository.CommitResult{}, &repository.Conflict{
				Key: mutation.Key, Expected: mutation.ExpectedVersion, Actual: actual,
			}
		}
	}

	next := cloneState(store.state)
	next.version++
	version := next.version
	next.scopeVersions[operation.Scope] = version
	resourceCounts := make(map[string]int)
	for _, mutation := range normalized {
		resourceCounts[mutation.Key.Kind]++
		next.resources[mutation.Key] = repository.Resource{
			Key: mutation.Key, Version: version,
			Payload: slices.Clone(mutation.Payload), Deleted: mutation.Delete,
		}
	}
	result := repository.CommitResult{Version: version}
	next.operations[receiptKey] = operationReceipt{digest: operation.Digest, result: result}
	next.audit = append(next.audit, scopedAuditRecord{
		scope: operation.Scope,
		record: repository.AuditRecord{
			Version: version, OperationKind: operation.Kind, At: operation.At,
			ResourceCounts: resourceCounts,
		},
	})
	if err := persistState(store.path, next, store.fault); err != nil {
		return repository.CommitResult{}, err
	}
	store.state = next
	return result, nil
}

func (store *Store) Audit(
	ctx context.Context,
	scope string,
	after repository.Version,
	limit int,
) ([]repository.AuditRecord, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if scope == "" {
		return nil, domain.NewFailure(domain.FailureInvalidInput, "audit scope is required")
	}
	if limit <= 0 || limit > maxAuditLimit {
		return nil, domain.NewFailure(
			domain.FailureInvalidInput,
			"audit limit must be between 1 and %d",
			maxAuditLimit,
		)
	}
	store.mu.Lock()
	defer store.mu.Unlock()

	page := make([]repository.AuditRecord, 0, min(limit, len(store.state.audit)))
	for _, scoped := range store.state.audit {
		if scoped.scope != scope || scoped.record.Version <= after {
			continue
		}
		page = append(page, scoped.record)
		if len(page) == limit {
			break
		}
	}
	return repository.CloneAudit(page), nil
}

func cloneState(current state) state {
	cloned := newState()
	cloned.version = current.version
	for scope, version := range current.scopeVersions {
		cloned.scopeVersions[scope] = version
	}
	for key, resource := range current.resources {
		resource.Payload = slices.Clone(resource.Payload)
		cloned.resources[key] = resource
	}
	for key, receipt := range current.operations {
		cloned.operations[key] = receipt
	}
	cloned.audit = make([]scopedAuditRecord, len(current.audit))
	for index, scoped := range current.audit {
		cloned.audit[index] = scoped
		cloned.audit[index].record = repository.CloneAudit([]repository.AuditRecord{scoped.record})[0]
	}
	return cloned
}

func compareKey(left, right repository.Key) int {
	if result := cmp.Compare(left.Scope, right.Scope); result != 0 {
		return result
	}
	if result := cmp.Compare(left.Kind, right.Kind); result != 0 {
		return result
	}
	return cmp.Compare(left.ID, right.ID)
}

var _ repository.Repository = (*Store)(nil)
