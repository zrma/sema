// Package postgres provides the PostgreSQL repository adapter.
package postgres

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"slices"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/zrma/sema/internal/domain"
	"github.com/zrma/sema/internal/repository"
)

const maxAuditLimit = 1000

// Store uses PostgreSQL transactions as the only durable mutation authority.
// It does not require Redis, a process-local lease owner, or a global writer.
type Store struct {
	pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) (*Store, error) {
	if pool == nil {
		return nil, fmt.Errorf("postgres repository pool is required")
	}
	return &Store{pool: pool}, nil
}

func Open(ctx context.Context, connectionString string) (*Store, error) {
	if connectionString == "" {
		return nil, domain.NewFailure(domain.FailureInvalidInput, "postgres connection string is required")
	}
	pool, err := pgxpool.New(ctx, connectionString)
	if err != nil {
		return nil, fmt.Errorf("open postgres repository pool: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping postgres repository: %w", err)
	}
	if err := checkSchemaVersion(ctx, pool); err != nil {
		pool.Close()
		return nil, err
	}
	return &Store{pool: pool}, nil
}

func (store *Store) Close() {
	if store != nil && store.pool != nil {
		store.pool.Close()
	}
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
	var digest []byte
	var rawVersion int64
	err := store.pool.QueryRow(ctx, `
		SELECT digest, version
		FROM sema_repository_operations
		WHERE scope = $1 AND operation_id = $2`,
		operation.Scope, string(operation.ID)).Scan(&digest, &rawVersion)
	if errors.Is(err, pgx.ErrNoRows) {
		return repository.CommitResult{}, false, nil
	}
	if err != nil {
		return repository.CommitResult{}, false, fmt.Errorf("read postgres operation receipt: %w", err)
	}
	result, err := validateReplay(operation, digest, rawVersion)
	return result, true, err
}

func (store *Store) Snapshot(ctx context.Context, scope string) (repository.Snapshot, error) {
	if err := ctx.Err(); err != nil {
		return repository.Snapshot{}, err
	}
	if scope == "" {
		return repository.Snapshot{}, domain.NewFailure(domain.FailureInvalidInput, "snapshot scope is required")
	}
	tx, err := store.pool.BeginTx(ctx, pgx.TxOptions{
		IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly,
	})
	if err != nil {
		return repository.Snapshot{}, fmt.Errorf("begin postgres repository snapshot: %w", err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()

	version, err := readScopeVersion(ctx, tx, scope)
	if err != nil {
		return repository.Snapshot{}, err
	}
	rows, err := tx.Query(ctx, `
		SELECT resource_kind, resource_id, version, payload, deleted
		FROM sema_repository_resources
		WHERE scope = $1
		ORDER BY resource_kind, resource_id`, scope)
	if err != nil {
		return repository.Snapshot{}, fmt.Errorf("query postgres repository snapshot: %w", err)
	}
	resources := make([]repository.Resource, 0)
	for rows.Next() {
		var kind, id string
		var rawVersion int64
		var payload []byte
		var deleted bool
		if err := rows.Scan(&kind, &id, &rawVersion, &payload, &deleted); err != nil {
			rows.Close()
			return repository.Snapshot{}, fmt.Errorf("scan postgres repository resource: %w", err)
		}
		if rawVersion <= 0 {
			rows.Close()
			return repository.Snapshot{}, fmt.Errorf("postgres repository resource has invalid version %d", rawVersion)
		}
		resources = append(resources, repository.Resource{
			Key:     repository.Key{Scope: scope, Kind: kind, ID: id},
			Version: repository.Version(rawVersion), Payload: slices.Clone(payload), Deleted: deleted,
		})
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return repository.Snapshot{}, fmt.Errorf("iterate postgres repository snapshot: %w", err)
	}
	rows.Close()
	if err := tx.Commit(ctx); err != nil {
		return repository.Snapshot{}, fmt.Errorf("commit postgres repository snapshot: %w", err)
	}
	return repository.Snapshot{Version: version, Resources: resources}, nil
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
	for _, mutation := range normalized {
		if mutation.ExpectedVersion > repository.Version(math.MaxInt64) {
			return repository.CommitResult{}, domain.NewFailure(
				domain.FailureInvalidInput,
				"expected repository version exceeds PostgreSQL bigint range",
			)
		}
	}

	tx, err := store.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return repository.CommitResult{}, fmt.Errorf("begin postgres repository commit: %w", err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()

	if _, err := tx.Exec(ctx, `
		INSERT INTO sema_repository_scopes (scope, version)
		VALUES ($1, 0)
		ON CONFLICT (scope) DO NOTHING`, operation.Scope); err != nil {
		return repository.CommitResult{}, fmt.Errorf("ensure postgres repository scope: %w", err)
	}
	inserted, err := insertOperationClaim(ctx, tx, operation)
	if err != nil {
		return repository.CommitResult{}, err
	}
	if !inserted {
		return replayOperation(ctx, tx, operation)
	}

	for _, mutation := range normalized {
		actual, err := lockResourceVersion(ctx, tx, mutation.Key)
		if err != nil {
			return repository.CommitResult{}, err
		}
		if actual != mutation.ExpectedVersion {
			return repository.CommitResult{}, &repository.Conflict{
				Key: mutation.Key, Expected: mutation.ExpectedVersion, Actual: actual,
			}
		}
	}

	version, err := nextScopeVersion(ctx, tx, operation.Scope)
	if err != nil {
		return repository.CommitResult{}, err
	}
	resourceCounts := make(map[string]int)
	for _, mutation := range normalized {
		resourceCounts[mutation.Key.Kind]++
		if err := applyMutation(ctx, tx, mutation, version); err != nil {
			return repository.CommitResult{}, err
		}
	}
	operationTag, err := tx.Exec(ctx, `
		UPDATE sema_repository_operations
		SET version = $3
		WHERE scope = $1 AND operation_id = $2`,
		operation.Scope, string(operation.ID), int64(version))
	if err != nil {
		return repository.CommitResult{}, fmt.Errorf("finalize postgres operation receipt: %w", err)
	}
	if operationTag.RowsAffected() != 1 {
		return repository.CommitResult{}, fmt.Errorf("postgres operation receipt disappeared before commit")
	}
	encodedCounts, err := json.Marshal(resourceCounts)
	if err != nil {
		return repository.CommitResult{}, fmt.Errorf("encode postgres audit resource counts: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO sema_repository_audit (
			scope, version, operation_kind, occurred_at, resource_counts
		) VALUES ($1, $2, $3, $4, $5::jsonb)`,
		operation.Scope, int64(version), operation.Kind, operation.At, encodedCounts); err != nil {
		return repository.CommitResult{}, fmt.Errorf("insert postgres audit receipt: %w", err)
	}
	scopeTag, err := tx.Exec(ctx, `
		UPDATE sema_repository_scopes
		SET version = $2
		WHERE scope = $1`, operation.Scope, int64(version))
	if err != nil {
		return repository.CommitResult{}, fmt.Errorf("advance postgres repository scope: %w", err)
	}
	if scopeTag.RowsAffected() != 1 {
		return repository.CommitResult{}, fmt.Errorf("postgres repository scope disappeared before commit")
	}
	if err := tx.Commit(ctx); err != nil {
		return repository.CommitResult{}, fmt.Errorf("commit postgres repository transaction: %w", err)
	}
	return repository.CommitResult{Version: version}, nil
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
	if after > repository.Version(math.MaxInt64) {
		return []repository.AuditRecord{}, nil
	}
	rows, err := store.pool.Query(ctx, `
		SELECT version, operation_kind, occurred_at, resource_counts
		FROM sema_repository_audit
		WHERE scope = $1 AND version > $2
		ORDER BY version
		LIMIT $3`, scope, int64(after), limit)
	if err != nil {
		return nil, fmt.Errorf("query postgres repository audit: %w", err)
	}
	defer rows.Close()
	records := make([]repository.AuditRecord, 0, limit)
	for rows.Next() {
		var rawVersion int64
		var record repository.AuditRecord
		var encodedCounts []byte
		if err := rows.Scan(&rawVersion, &record.OperationKind, &record.At, &encodedCounts); err != nil {
			return nil, fmt.Errorf("scan postgres repository audit: %w", err)
		}
		if rawVersion <= 0 {
			return nil, fmt.Errorf("postgres repository audit has invalid version %d", rawVersion)
		}
		if err := json.Unmarshal(encodedCounts, &record.ResourceCounts); err != nil {
			return nil, fmt.Errorf("decode postgres audit resource counts: %w", err)
		}
		record.Version = repository.Version(rawVersion)
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate postgres repository audit: %w", err)
	}
	return records, nil
}

func insertOperationClaim(ctx context.Context, tx pgx.Tx, operation repository.Operation) (bool, error) {
	result, err := tx.Exec(ctx, `
		INSERT INTO sema_repository_operations (
			scope, operation_id, digest, operation_kind, occurred_at, version
		) VALUES ($1, $2, $3, $4, $5, NULL)
		ON CONFLICT (scope, operation_id) DO NOTHING`,
		operation.Scope, string(operation.ID), operation.Digest[:], operation.Kind, operation.At)
	if err != nil {
		return false, fmt.Errorf("claim postgres operation receipt: %w", err)
	}
	return result.RowsAffected() == 1, nil
}

func replayOperation(
	ctx context.Context,
	tx pgx.Tx,
	operation repository.Operation,
) (repository.CommitResult, error) {
	var digest []byte
	var rawVersion int64
	if err := tx.QueryRow(ctx, `
		SELECT digest, version
		FROM sema_repository_operations
		WHERE scope = $1 AND operation_id = $2`,
		operation.Scope, string(operation.ID)).Scan(&digest, &rawVersion); err != nil {
		return repository.CommitResult{}, fmt.Errorf("read postgres operation receipt: %w", err)
	}
	return validateReplay(operation, digest, rawVersion)
}

func validateReplay(
	operation repository.Operation,
	digest []byte,
	rawVersion int64,
) (repository.CommitResult, error) {
	if !bytes.Equal(digest, operation.Digest[:]) {
		return repository.CommitResult{}, domain.NewFailure(
			domain.FailureIdempotencyConflict,
			"operation ID %q was used for another command",
			operation.ID,
		)
	}
	if rawVersion <= 0 {
		return repository.CommitResult{}, fmt.Errorf("postgres operation receipt has invalid version %d", rawVersion)
	}
	return repository.CommitResult{Version: repository.Version(rawVersion), Replayed: true}, nil
}

func lockResourceVersion(
	ctx context.Context,
	tx pgx.Tx,
	key repository.Key,
) (repository.Version, error) {
	var rawVersion int64
	err := tx.QueryRow(ctx, `
		SELECT version
		FROM sema_repository_resources
		WHERE scope = $1 AND resource_kind = $2 AND resource_id = $3
		FOR UPDATE`, key.Scope, key.Kind, key.ID).Scan(&rawVersion)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("lock postgres repository resource: %w", err)
	}
	if rawVersion <= 0 {
		return 0, fmt.Errorf("postgres repository resource has invalid version %d", rawVersion)
	}
	return repository.Version(rawVersion), nil
}

func nextScopeVersion(
	ctx context.Context,
	tx pgx.Tx,
	scope string,
) (repository.Version, error) {
	var current int64
	if err := tx.QueryRow(ctx, `
		SELECT version
		FROM sema_repository_scopes
		WHERE scope = $1
		FOR UPDATE`, scope).Scan(&current); err != nil {
		return 0, fmt.Errorf("lock postgres repository scope: %w", err)
	}
	if current < 0 || current == math.MaxInt64 {
		return 0, fmt.Errorf("postgres repository scope version is invalid or exhausted")
	}
	return repository.Version(current + 1), nil
}

func applyMutation(
	ctx context.Context,
	tx pgx.Tx,
	mutation repository.Mutation,
	version repository.Version,
) error {
	payload := mutation.Payload
	if payload == nil {
		payload = []byte{}
	}
	if mutation.ExpectedVersion == 0 {
		tag, err := tx.Exec(ctx, `
			INSERT INTO sema_repository_resources (
				scope, resource_kind, resource_id, version, payload, deleted
			) VALUES ($1, $2, $3, $4, $5, $6)
			ON CONFLICT (scope, resource_kind, resource_id) DO NOTHING`,
			mutation.Key.Scope, mutation.Key.Kind, mutation.Key.ID,
			int64(version), payload, mutation.Delete)
		if err != nil {
			return fmt.Errorf("insert postgres repository resource: %w", err)
		}
		if tag.RowsAffected() == 1 {
			return nil
		}
	} else {
		tag, err := tx.Exec(ctx, `
			UPDATE sema_repository_resources
			SET version = $4, payload = $5, deleted = $6
			WHERE scope = $1 AND resource_kind = $2 AND resource_id = $3 AND version = $7`,
			mutation.Key.Scope, mutation.Key.Kind, mutation.Key.ID,
			int64(version), payload, mutation.Delete, int64(mutation.ExpectedVersion))
		if err != nil {
			return fmt.Errorf("update postgres repository resource: %w", err)
		}
		if tag.RowsAffected() == 1 {
			return nil
		}
	}
	actual, err := lockResourceVersion(ctx, tx, mutation.Key)
	if err != nil {
		return err
	}
	return &repository.Conflict{
		Key: mutation.Key, Expected: mutation.ExpectedVersion, Actual: actual,
	}
}

func readScopeVersion(ctx context.Context, tx pgx.Tx, scope string) (repository.Version, error) {
	var rawVersion int64
	err := tx.QueryRow(
		ctx,
		"SELECT version FROM sema_repository_scopes WHERE scope = $1",
		scope,
	).Scan(&rawVersion)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("read postgres repository scope: %w", err)
	}
	if rawVersion < 0 {
		return 0, fmt.Errorf("postgres repository scope has invalid version %d", rawVersion)
	}
	return repository.Version(rawVersion), nil
}

var _ repository.Repository = (*Store)(nil)
