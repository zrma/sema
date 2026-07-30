// Package postgresrecovery provides the destructive, isolated recovery
// acceptance used for a new standard-service PostgreSQL installation.
package postgresrecovery

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	api "github.com/zrma/sema/internal/api/v0alpha2"
	"github.com/zrma/sema/internal/domain"
	"github.com/zrma/sema/internal/repository"
	postgresrepository "github.com/zrma/sema/internal/repository/postgres"
	"github.com/zrma/sema/internal/service"
	"github.com/zrma/sema/internal/targetapi"
	"github.com/zrma/sema/internal/targetruntime"
	"github.com/zrma/sema/internal/wireconformance"
)

const (
	ManifestSchema = "sema.postgres-recovery-manifest.v1"
	ReportSchema   = "sema.postgres-recovery.v1"

	recoveryScope         = "recovery-tenant"
	checkpointPolicy      = "checkpoint-policy"
	checkpointRun         = "checkpoint-run"
	checkpointReservation = domain.ReservationID("checkpoint-reservation")
	checkpointAssignment  = domain.AssignmentID("checkpoint-assignment")
	recoveryToken         = "recovery-test-token"
)

type Options struct {
	PostgresDSN string
	Schema      string
	Manifest    string
	Now         func() time.Time
}

type Report struct {
	Schema                     string `json:"schema"`
	Phase                      string `json:"phase"`
	CheckpointVersion          uint64 `json:"checkpoint_version"`
	CheckpointResources        int    `json:"checkpoint_resources"`
	CheckpointAuditRecords     int    `json:"checkpoint_audit_records"`
	CheckpointVerified         bool   `json:"checkpoint_verified,omitempty"`
	PostCheckpointExcluded     bool   `json:"post_checkpoint_excluded,omitempty"`
	OperationReplayVerified    bool   `json:"operation_replay_verified,omitempty"`
	TerminalAssignmentVerified bool   `json:"terminal_assignment_verified,omitempty"`
	StatelessRuntimeVerified   bool   `json:"stateless_runtime_verified,omitempty"`
	PostRestoreWriteVerified   bool   `json:"post_restore_write_verified,omitempty"`
	WithinAcceptance           bool   `json:"within_acceptance"`
}

type manifest struct {
	Schema          string         `json:"schema"`
	SnapshotVersion uint64         `json:"snapshot_version"`
	ResourceCount   int            `json:"resource_count"`
	ResourceDigest  string         `json:"resource_digest"`
	AuditCount      int            `json:"audit_count"`
	AuditDigest     string         `json:"audit_digest"`
	AuthorityDigest string         `json:"authority_digest"`
	TableRows       map[string]int `json:"table_rows"`
}

func Seed(ctx context.Context, options Options) (Report, error) {
	if err := validateOptions(options); err != nil {
		return Report{}, err
	}
	if _, err := os.Stat(options.Manifest); err == nil {
		return Report{}, fmt.Errorf("private recovery manifest already exists")
	} else if !errors.Is(err, os.ErrNotExist) {
		return Report{}, fmt.Errorf("inspect private recovery manifest: %w", err)
	}
	pool, owner, err := open(ctx, options, true)
	if err != nil {
		return Report{}, err
	}
	defer pool.Close()
	snapshot, err := owner.Snapshot(ctx, recoveryScope)
	if err != nil {
		return Report{}, err
	}
	if snapshot.Version != 0 || len(snapshot.Resources) != 0 {
		return Report{}, fmt.Errorf("recovery seed requires an empty tenant scope")
	}
	if err := seedLifecycle(ctx, owner, options.Now); err != nil {
		return Report{}, err
	}
	checkpoint, err := capture(ctx, pool, owner)
	if err != nil {
		return Report{}, err
	}
	if err := writeManifest(options.Manifest, checkpoint); err != nil {
		return Report{}, err
	}
	return reportFor("seed", checkpoint, false), nil
}

func Advance(ctx context.Context, options Options) (Report, error) {
	if err := validateOptions(options); err != nil {
		return Report{}, err
	}
	expected, err := readManifest(options.Manifest)
	if err != nil {
		return Report{}, err
	}
	pool, owner, err := open(ctx, options, false)
	if err != nil {
		return Report{}, err
	}
	defer pool.Close()
	before, err := capture(ctx, pool, owner)
	if err != nil {
		return Report{}, err
	}
	if !manifestsEqual(expected, before) {
		return Report{}, fmt.Errorf("database changed before post-checkpoint mutation")
	}
	tickets, err := service.NewMatchTickets(owner, options.Now)
	if err != nil {
		return Report{}, err
	}
	if _, err := tickets.Put(
		ctx,
		recoveryScope,
		"post-checkpoint-ticket-put",
		postCheckpointTicket(options.Now()),
	); err != nil {
		return Report{}, fmt.Errorf("write post-checkpoint mutation: %w", err)
	}
	return reportFor("advance", expected, false), nil
}

func Verify(ctx context.Context, options Options) (Report, error) {
	if err := validateOptions(options); err != nil {
		return Report{}, err
	}
	expected, err := readManifest(options.Manifest)
	if err != nil {
		return Report{}, err
	}
	pool, owner, err := open(ctx, options, true)
	if err != nil {
		return Report{}, err
	}
	defer pool.Close()
	actual, err := capture(ctx, pool, owner)
	if err != nil {
		return Report{}, err
	}
	if !manifestsEqual(expected, actual) {
		return Report{}, fmt.Errorf("restored checkpoint differs from its semantic manifest")
	}

	tickets, err := service.NewMatchTickets(owner, options.Now)
	if err != nil {
		return Report{}, err
	}
	if _, exists, err := tickets.Get(ctx, recoveryScope, "post-checkpoint-ticket"); err != nil {
		return Report{}, err
	} else if exists {
		return Report{}, fmt.Errorf("post-checkpoint mutation survived checkpoint restore")
	}
	assignments, err := service.NewAssignments(owner, options.Now)
	if err != nil {
		return Report{}, err
	}
	assignment, exists, err := assignments.Get(ctx, recoveryScope, checkpointAssignment)
	if err != nil {
		return Report{}, err
	}
	if !exists || assignment.Assignment.Status != domain.AssignmentCompleted ||
		assignment.Assignment.Acknowledgment == nil {
		return Report{}, fmt.Errorf("restored terminal assignment is incomplete")
	}
	replayed, err := tickets.Put(
		ctx,
		recoveryScope,
		"checkpoint-ticket-0-put",
		checkpointTicket(0, options.Now()),
	)
	if err != nil {
		return Report{}, fmt.Errorf("replay restored operation receipt: %w", err)
	}
	if !replayed.Replayed || uint64(replayed.StorageVersion) > expected.SnapshotVersion {
		return Report{}, fmt.Errorf("restored operation receipt did not replay")
	}
	if err := verifyStatelessRuntime(ctx, owner, options.Now, expected.SnapshotVersion); err != nil {
		return Report{}, err
	}
	report := reportFor("verify", expected, true)
	report.CheckpointVerified = true
	report.PostCheckpointExcluded = true
	report.OperationReplayVerified = true
	report.TerminalAssignmentVerified = true
	report.StatelessRuntimeVerified = true
	report.PostRestoreWriteVerified = true
	return report, nil
}

func validateOptions(options Options) error {
	if options.PostgresDSN == "" || options.Schema == "" || options.Manifest == "" {
		return fmt.Errorf("PostgreSQL DSN, isolated schema, and private manifest are required")
	}
	if options.Now == nil {
		return fmt.Errorf("recovery clock is required")
	}
	return nil
}

func open(
	ctx context.Context,
	options Options,
	migrate bool,
) (*pgxpool.Pool, *postgresrepository.Store, error) {
	config, err := pgxpool.ParseConfig(options.PostgresDSN)
	if err != nil {
		return nil, nil, fmt.Errorf("parse recovery PostgreSQL DSN: %w", err)
	}
	config.ConnConfig.RuntimeParams["search_path"] = options.Schema
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, nil, fmt.Errorf("open recovery PostgreSQL pool: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, nil, fmt.Errorf("ping recovery PostgreSQL: %w", err)
	}
	if migrate {
		if err := postgresrepository.Migrate(ctx, pool); err != nil {
			pool.Close()
			return nil, nil, fmt.Errorf("migrate recovery PostgreSQL schema: %w", err)
		}
	}
	owner, err := postgresrepository.New(pool)
	if err != nil {
		pool.Close()
		return nil, nil, err
	}
	if !migrate {
		if _, err := owner.Snapshot(ctx, recoveryScope); err != nil {
			pool.Close()
			return nil, nil, fmt.Errorf("open recovery repository state: %w", err)
		}
	}
	return pool, owner, nil
}

func seedLifecycle(
	ctx context.Context,
	owner repository.Repository,
	now func() time.Time,
) error {
	policies, err := service.NewPolicies(owner, now)
	if err != nil {
		return err
	}
	policy := domain.MatchmakingPolicy{
		Version: checkpointPolicy, TeamCount: 2, TeamSize: 2,
		MaxLatencyMillis: 100, MaxProposals: 1, MaxSearchNodes: 100_000,
		MaxCandidateTickets: 64, MaxCandidatesPerProposal: 64,
		MaxBatchCandidates: 64, MaxBatchSearchNodes: 100_000,
		RelaxationSteps: []domain.RelaxationStep{{AfterWait: 0, MaxTeamSkillGap: 100}},
	}
	if _, err := policies.Put(ctx, recoveryScope, "checkpoint-policy-put", policy); err != nil {
		return fmt.Errorf("seed recovery policy: %w", err)
	}
	tickets, err := service.NewMatchTickets(owner, now)
	if err != nil {
		return err
	}
	for index := range 4 {
		if _, err := tickets.Put(
			ctx,
			recoveryScope,
			domain.OperationID(fmt.Sprintf("checkpoint-ticket-%d-put", index)),
			checkpointTicket(index, now()),
		); err != nil {
			return fmt.Errorf("seed recovery ticket: %w", err)
		}
	}
	runs, err := service.NewPlanningRuns(owner, now, nil)
	if err != nil {
		return err
	}
	run, err := runs.Execute(
		ctx,
		recoveryScope,
		"checkpoint-planning-run",
		checkpointRun,
		checkpointPolicy,
	)
	if err != nil {
		return fmt.Errorf("seed recovery planning run: %w", err)
	}
	if run.Run.Status != service.PlanningRunCompleted || run.Run.ProposalCount != 1 {
		return fmt.Errorf("recovery planning fixture did not produce one proposal")
	}
	proposals, err := runs.Proposals(ctx, recoveryScope, checkpointRun)
	if err != nil || len(proposals.Records) != 1 {
		return fmt.Errorf("read recovery proposal: count=%d error=%w", len(proposals.Records), err)
	}
	reservations, err := service.NewReservations(owner, now, time.Minute)
	if err != nil {
		return err
	}
	if _, err := reservations.Reserve(
		ctx,
		recoveryScope,
		"checkpoint-reservation-create",
		checkpointReservation,
		proposals.Records[0].Proposal.ID,
	); err != nil {
		return fmt.Errorf("seed recovery reservation: %w", err)
	}
	if _, err := reservations.Confirm(
		ctx,
		recoveryScope,
		"checkpoint-reservation-confirm",
		checkpointReservation,
		checkpointAssignment,
	); err != nil {
		return fmt.Errorf("seed recovery assignment: %w", err)
	}
	assignments, err := service.NewAssignments(owner, now)
	if err != nil {
		return err
	}
	if _, err := assignments.Acknowledge(
		ctx,
		recoveryScope,
		checkpointAssignment,
		domain.AssignmentAcknowledgmentRequest{
			OperationID: "checkpoint-assignment-complete",
			Outcome:     domain.AssignmentCompleted,
		},
	); err != nil {
		return fmt.Errorf("complete recovery assignment: %w", err)
	}
	return nil
}

func checkpointTicket(index int, now time.Time) domain.MatchTicket {
	id := domain.TicketID(fmt.Sprintf("checkpoint-ticket-%d", index))
	return domain.MatchTicket{
		ID: id, Revision: 1, EnqueuedAt: now.UTC().Add(-time.Second),
		Players: []domain.Player{{
			ID: domain.PlayerID(id + "-player"), Skill: 1490 + index*5, Role: "flex",
			LatencyMillis: 30 + index,
		}},
	}
}

func postCheckpointTicket(now time.Time) domain.MatchTicket {
	return domain.MatchTicket{
		ID: "post-checkpoint-ticket", Revision: 1, EnqueuedAt: now.UTC(),
		Players: []domain.Player{{
			ID: "post-checkpoint-player", Skill: 1500, Role: "flex", LatencyMillis: 35,
		}},
	}
}

func verifyStatelessRuntime(
	ctx context.Context,
	owner *postgresrepository.Store,
	now func() time.Time,
	checkpointVersion uint64,
) error {
	permissions := map[targetapi.Permission]bool{
		targetapi.PermissionMatchTicketsWrite: true,
		targetapi.PermissionAssignmentsRead:   true,
	}
	authenticator := targetapi.AuthenticatorFunc(func(request *http.Request) (targetapi.Principal, error) {
		if request.Header.Get("Authorization") != "Bearer "+recoveryToken {
			return targetapi.Principal{}, targetapi.ErrUnauthenticated
		}
		return targetapi.Principal{
			Subject: "recovery-verifier", Tenant: recoveryScope, Permissions: permissions,
		}, nil
	})
	handler, err := targetruntime.New(owner, authenticator, targetruntime.Options{
		CursorKey: bytes.Repeat([]byte{0x72}, 32), ReservationTTL: time.Minute,
		MaxInFlight: 4, RequestTimeout: 2 * time.Second, ReadinessTimeout: time.Second,
		ReadinessCheck: owner.Ready,
	})
	if err != nil {
		return err
	}
	server := httptest.NewServer(handler)
	defer server.Close()
	client := &http.Client{Timeout: 3 * time.Second}

	readiness, err := client.Get(server.URL + "/readyz")
	if err != nil {
		return fmt.Errorf("read restored runtime readiness: %w", err)
	}
	_ = readiness.Body.Close()
	if readiness.StatusCode != http.StatusOK {
		return fmt.Errorf("restored runtime readiness returned HTTP %d", readiness.StatusCode)
	}
	assignment, err := wireconformance.RequestData[api.AssignmentResource](
		ctx,
		client,
		server.URL,
		recoveryToken,
		"",
		http.MethodGet,
		"/v0alpha2/assignments/"+string(checkpointAssignment),
		nil,
	)
	if err != nil {
		return fmt.Errorf("read restored assignment through stateless runtime: %w", err)
	}
	if assignment.Assignment.Status != string(domain.AssignmentCompleted) ||
		assignment.Assignment.Acknowledgment == nil {
		return fmt.Errorf("stateless runtime returned incomplete restored assignment")
	}
	newTicket := api.MatchTicket{
		ID: "post-restore-ticket", Revision: 1, EnqueuedAt: now().UTC(),
		Players: []api.Player{{
			ID: "post-restore-player", Skill: 1500, Role: "flex", LatencyMillis: 35,
		}},
	}
	written, err := wireconformance.RequestData[api.MatchTicketMutation](
		ctx,
		client,
		server.URL,
		recoveryToken,
		"post-restore-ticket-put",
		http.MethodPut,
		"/v0alpha2/match-tickets/"+newTicket.ID,
		newTicket,
	)
	if err != nil {
		return fmt.Errorf("write through restored stateless runtime: %w", err)
	}
	if written.Replayed || written.Resource.StorageVersion <= checkpointVersion {
		return fmt.Errorf("post-restore write did not advance repository authority")
	}
	return nil
}

func capture(
	ctx context.Context,
	pool *pgxpool.Pool,
	owner repository.Repository,
) (manifest, error) {
	snapshot, err := owner.Snapshot(ctx, recoveryScope)
	if err != nil {
		return manifest{}, fmt.Errorf("capture recovery snapshot: %w", err)
	}
	audit, err := allAudit(ctx, owner)
	if err != nil {
		return manifest{}, err
	}
	tableRows, err := repositoryTableRows(ctx, pool)
	if err != nil {
		return manifest{}, err
	}
	authority, err := repositoryAuthorityDigest(ctx, pool)
	if err != nil {
		return manifest{}, err
	}
	return manifest{
		Schema: ManifestSchema, SnapshotVersion: uint64(snapshot.Version),
		ResourceCount: len(snapshot.Resources), ResourceDigest: resourceDigest(snapshot),
		AuditCount: len(audit), AuditDigest: auditDigest(audit),
		AuthorityDigest: authority, TableRows: tableRows,
	}, nil
}

func allAudit(ctx context.Context, owner repository.Repository) ([]repository.AuditRecord, error) {
	var records []repository.AuditRecord
	var after repository.Version
	for {
		page, err := owner.Audit(ctx, recoveryScope, after, 1000)
		if err != nil {
			return nil, fmt.Errorf("capture recovery audit: %w", err)
		}
		if len(page) == 0 {
			return records, nil
		}
		records = append(records, page...)
		after = page[len(page)-1].Version
	}
}

func repositoryTableRows(ctx context.Context, pool *pgxpool.Pool) (map[string]int, error) {
	tables := []string{
		"sema_repository_metadata", "sema_repository_scopes", "sema_repository_operations",
		"sema_repository_resources", "sema_repository_audit",
	}
	counts := make(map[string]int, len(tables))
	for _, table := range tables {
		var count int
		if err := pool.QueryRow(ctx, "SELECT count(*) FROM "+table).Scan(&count); err != nil {
			return nil, fmt.Errorf("count recovery table: %w", err)
		}
		counts[table] = count
	}
	return counts, nil
}

func repositoryAuthorityDigest(ctx context.Context, pool *pgxpool.Pool) (string, error) {
	hash := sha256.New()
	metadata, err := pool.Query(ctx, `SELECT key, value FROM sema_repository_metadata ORDER BY key`)
	if err != nil {
		return "", fmt.Errorf("read recovery metadata: %w", err)
	}
	for metadata.Next() {
		var key, value string
		if err := metadata.Scan(&key, &value); err != nil {
			metadata.Close()
			return "", fmt.Errorf("scan recovery metadata: %w", err)
		}
		fmt.Fprintf(hash, "metadata\x00%s\x00%s\n", key, value)
	}
	if err := metadata.Err(); err != nil {
		metadata.Close()
		return "", fmt.Errorf("iterate recovery metadata: %w", err)
	}
	metadata.Close()

	scopes, err := pool.Query(ctx, `SELECT scope, version FROM sema_repository_scopes ORDER BY scope`)
	if err != nil {
		return "", fmt.Errorf("read recovery scopes: %w", err)
	}
	for scopes.Next() {
		var scope string
		var version int64
		if err := scopes.Scan(&scope, &version); err != nil {
			scopes.Close()
			return "", fmt.Errorf("scan recovery scope: %w", err)
		}
		fmt.Fprintf(hash, "scope\x00%s\x00%d\n", scope, version)
	}
	if err := scopes.Err(); err != nil {
		scopes.Close()
		return "", fmt.Errorf("iterate recovery scopes: %w", err)
	}
	scopes.Close()

	operations, err := pool.Query(ctx, `
		SELECT scope, operation_id, digest, operation_kind, occurred_at, version
		FROM sema_repository_operations
		ORDER BY scope, operation_id`)
	if err != nil {
		return "", fmt.Errorf("read recovery operation authority: %w", err)
	}
	for operations.Next() {
		var scope, operationID, operationKind string
		var digest []byte
		var occurredAt time.Time
		var version sql.NullInt64
		if err := operations.Scan(&scope, &operationID, &digest, &operationKind, &occurredAt, &version); err != nil {
			operations.Close()
			return "", fmt.Errorf("scan recovery operation authority: %w", err)
		}
		fmt.Fprintf(
			hash,
			"operation\x00%s\x00%s\x00%s\x00%s\x00%s\x00%t\x00%d\n",
			scope,
			operationID,
			hex.EncodeToString(digest),
			operationKind,
			occurredAt.UTC().Format(time.RFC3339Nano),
			version.Valid,
			version.Int64,
		)
	}
	if err := operations.Err(); err != nil {
		operations.Close()
		return "", fmt.Errorf("iterate recovery operations: %w", err)
	}
	operations.Close()
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func resourceDigest(snapshot repository.Snapshot) string {
	hash := sha256.New()
	for _, resource := range snapshot.Resources {
		payload := sha256.Sum256(resource.Payload)
		fmt.Fprintf(
			hash,
			"%s\x00%s\x00%s\x00%d\x00%t\x00%s\n",
			resource.Key.Scope,
			resource.Key.Kind,
			resource.Key.ID,
			resource.Version,
			resource.Deleted,
			hex.EncodeToString(payload[:]),
		)
	}
	return hex.EncodeToString(hash.Sum(nil))
}

func auditDigest(records []repository.AuditRecord) string {
	hash := sha256.New()
	for _, record := range records {
		kinds := make([]string, 0, len(record.ResourceCounts))
		for kind := range record.ResourceCounts {
			kinds = append(kinds, kind)
		}
		slices.Sort(kinds)
		fmt.Fprintf(
			hash,
			"%d\x00%s\x00%s",
			record.Version,
			record.OperationKind,
			record.At.UTC().Format(time.RFC3339Nano),
		)
		for _, kind := range kinds {
			fmt.Fprintf(hash, "\x00%s=%d", kind, record.ResourceCounts[kind])
		}
		fmt.Fprintln(hash)
	}
	return hex.EncodeToString(hash.Sum(nil))
}

func writeManifest(path string, evidence manifest) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create recovery manifest directory: %w", err)
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".sema-postgres-recovery-*")
	if err != nil {
		return fmt.Errorf("create private recovery manifest: %w", err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0o600); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("protect private recovery manifest: %w", err)
	}
	encoder := json.NewEncoder(temporary)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(evidence); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("encode private recovery manifest: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("sync private recovery manifest: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close private recovery manifest: %w", err)
	}
	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("private recovery manifest already exists")
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("inspect private recovery manifest destination: %w", err)
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return fmt.Errorf("install private recovery manifest: %w", err)
	}
	return nil
}

func readManifest(path string) (manifest, error) {
	file, err := os.Open(path)
	if err != nil {
		return manifest{}, fmt.Errorf("open private recovery manifest: %w", err)
	}
	defer file.Close()
	decoder := json.NewDecoder(file)
	decoder.DisallowUnknownFields()
	var evidence manifest
	if err := decoder.Decode(&evidence); err != nil {
		return manifest{}, fmt.Errorf("decode private recovery manifest: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return manifest{}, fmt.Errorf("private recovery manifest has trailing content")
	}
	if evidence.Schema != ManifestSchema || evidence.SnapshotVersion == 0 ||
		evidence.ResourceCount <= 0 || len(evidence.ResourceDigest) != sha256.Size*2 ||
		evidence.AuditCount <= 0 || len(evidence.AuditDigest) != sha256.Size*2 ||
		len(evidence.AuthorityDigest) != sha256.Size*2 || len(evidence.TableRows) == 0 {
		return manifest{}, fmt.Errorf("private recovery manifest is incomplete")
	}
	return evidence, nil
}

func manifestsEqual(left, right manifest) bool {
	leftJSON, leftErr := json.Marshal(left)
	rightJSON, rightErr := json.Marshal(right)
	return leftErr == nil && rightErr == nil && string(leftJSON) == string(rightJSON)
}

func reportFor(phase string, checkpoint manifest, accepted bool) Report {
	return Report{
		Schema: ReportSchema, Phase: phase,
		CheckpointVersion:      checkpoint.SnapshotVersion,
		CheckpointResources:    checkpoint.ResourceCount,
		CheckpointAuditRecords: checkpoint.AuditCount,
		WithinAcceptance:       accepted,
	}
}
