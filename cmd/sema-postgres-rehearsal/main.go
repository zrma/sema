package main

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/zrma/sema/internal/domain"
	"github.com/zrma/sema/internal/durable"
	"github.com/zrma/sema/internal/repository"
	postgresrepository "github.com/zrma/sema/internal/repository/postgres"
	"github.com/zrma/sema/internal/service"
)

const (
	connectionEnvironment = "SEMA_POSTGRES_TEST_DSN"
	manifestSchema        = "sema.postgres-rehearsal-manifest.v1"
	reportSchema          = "sema.postgres-rehearsal-report.v1"
	rehearsalScope        = "rehearsal-tenant"
	rehearsalImportID     = "rehearsal-import"
	rehearsalAssignmentID = domain.AssignmentID("rehearsal-assignment")
)

var schemaNamePattern = regexp.MustCompile(`^[a-z][a-z0-9_]{0,62}$`)

type configuration struct {
	phase        string
	schema       string
	journalPath  string
	manifestPath string
	timeout      time.Duration
}

type manifest struct {
	Schema          string         `json:"schema"`
	SourceDigest    string         `json:"source_digest"`
	SourceRecords   int            `json:"source_records"`
	SnapshotVersion uint64         `json:"snapshot_version"`
	ResourceCount   int            `json:"resource_count"`
	ResourceDigest  string         `json:"resource_digest"`
	AuditCount      int            `json:"audit_count"`
	AuditDigest     string         `json:"audit_digest"`
	AuthorityDigest string         `json:"authority_digest"`
	TableRows       map[string]int `json:"table_rows"`
}

type report struct {
	Schema          string `json:"schema"`
	Phase           string `json:"phase"`
	SourceRecords   int    `json:"source_records"`
	SnapshotVersion uint64 `json:"snapshot_version,omitempty"`
	ResourceCount   int    `json:"resource_count,omitempty"`
	AuditCount      int    `json:"audit_count,omitempty"`
}

func main() {
	os.Exit(run(context.Background(), os.Args[1:], os.Stdout, os.Stderr))
}

func run(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	configuration, err := parseConfiguration(args, stderr)
	if err != nil {
		return 2
	}
	validationContext, cancel := context.WithTimeout(ctx, configuration.timeout)
	defer cancel()

	var result report
	switch configuration.phase {
	case "seed":
		result, err = seed(validationContext, configuration)
	case "verify":
		result, err = verify(validationContext, configuration)
	case "rollback":
		result, err = rollback(configuration)
	default:
		err = fmt.Errorf("unsupported rehearsal phase")
	}
	if err != nil {
		fmt.Fprintf(stderr, "sema-postgres-rehearsal: %v\n", err)
		return 1
	}
	if err := json.NewEncoder(stdout).Encode(result); err != nil {
		fmt.Fprintf(stderr, "sema-postgres-rehearsal: write report: %v\n", err)
		return 1
	}
	return 0
}

func parseConfiguration(args []string, stderr io.Writer) (configuration, error) {
	configuration := configuration{}
	flags := flag.NewFlagSet("sema-postgres-rehearsal", flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.StringVar(&configuration.phase, "phase", "", "rehearsal phase: seed, verify, or rollback")
	flags.StringVar(&configuration.schema, "schema", "", "isolated PostgreSQL schema")
	flags.StringVar(&configuration.journalPath, "journal", "", "stopped V0 journal path")
	flags.StringVar(&configuration.manifestPath, "manifest", "", "private temporary manifest path")
	flags.DurationVar(&configuration.timeout, "timeout", time.Minute, "whole phase timeout")
	flags.Usage = func() {
		fmt.Fprintln(stderr, "usage: sema-postgres-rehearsal -phase seed|verify|rollback -journal <path> -manifest <path> [-schema <name>]")
		flags.PrintDefaults()
	}
	if err := flags.Parse(args); err != nil {
		return configuration, err
	}
	if flags.NArg() != 0 {
		flags.Usage()
		return configuration, fmt.Errorf("unexpected positional arguments")
	}
	if configuration.phase != "seed" && configuration.phase != "verify" && configuration.phase != "rollback" {
		fmt.Fprintln(stderr, "sema-postgres-rehearsal: phase must be seed, verify, or rollback")
		return configuration, fmt.Errorf("invalid phase")
	}
	if configuration.timeout <= 0 || configuration.journalPath == "" || configuration.manifestPath == "" {
		fmt.Fprintln(stderr, "sema-postgres-rehearsal: timeout, journal, and manifest are required")
		return configuration, fmt.Errorf("invalid configuration")
	}
	if configuration.phase != "rollback" && !schemaNamePattern.MatchString(configuration.schema) {
		fmt.Fprintln(stderr, "sema-postgres-rehearsal: seed and verify require a safe schema name")
		return configuration, fmt.Errorf("invalid schema")
	}
	if configuration.phase != "rollback" && os.Getenv(connectionEnvironment) == "" {
		fmt.Fprintf(stderr, "sema-postgres-rehearsal: %s is required for seed and verify\n", connectionEnvironment)
		return configuration, fmt.Errorf("missing connection environment")
	}
	return configuration, nil
}

func seed(ctx context.Context, configuration configuration) (report, error) {
	source, err := createLegacyFixture(configuration.journalPath)
	if err != nil {
		return report{}, err
	}
	pool, owner, err := openRepository(ctx, configuration.schema, true)
	if err != nil {
		return report{}, err
	}
	defer pool.Close()
	importer, err := service.NewLegacyImporter(owner, service.LegacyImportOptions{
		Now: func() time.Time { return fixtureNow().Add(time.Hour) }, BatchSize: 3,
	})
	if err != nil {
		return report{}, err
	}
	imported, err := importer.Import(ctx, rehearsalScope, rehearsalImportID, configuration.journalPath)
	if err != nil {
		return report{}, fmt.Errorf("import stopped V0 fixture: %w", err)
	}
	if imported.Status.State != service.LegacyImportCompleted || imported.Status.SourceDigest != source.Digest {
		return report{}, fmt.Errorf("legacy import did not produce the expected completion marker")
	}
	evidence, err := captureManifest(ctx, pool, owner, source)
	if err != nil {
		return report{}, err
	}
	if err := writeManifest(configuration.manifestPath, evidence); err != nil {
		return report{}, err
	}
	return reportFromManifest("seed", evidence), nil
}

func verify(ctx context.Context, configuration configuration) (report, error) {
	expected, err := readManifest(configuration.manifestPath)
	if err != nil {
		return report{}, err
	}
	source, err := durable.ReadLegacyJournal(configuration.journalPath)
	if err != nil {
		return report{}, fmt.Errorf("read stopped V0 source: %w", err)
	}
	pool, owner, err := openRepository(ctx, configuration.schema, false)
	if err != nil {
		return report{}, err
	}
	defer pool.Close()
	actual, err := captureManifest(ctx, pool, owner, source)
	if err != nil {
		return report{}, err
	}
	if !manifestsEqual(expected, actual) {
		return report{}, fmt.Errorf("restored PostgreSQL state differs from the pre-backup manifest")
	}
	if _, err := service.RequireLegacyImportCompleted(
		ctx, owner, rehearsalScope, rehearsalImportID, expected.SourceDigest,
	); err != nil {
		return report{}, fmt.Errorf("verify restored import completion: %w", err)
	}
	assignments, err := service.NewAssignments(owner, fixtureNow)
	if err != nil {
		return report{}, err
	}
	assignment, exists, err := assignments.Get(ctx, rehearsalScope, rehearsalAssignmentID)
	if err != nil || !exists || assignment.Assignment.Status != domain.AssignmentCompleted ||
		assignment.Assignment.Acknowledgment == nil {
		return report{}, fmt.Errorf("restored terminal assignment is incomplete")
	}
	return reportFromManifest("verify", actual), nil
}

func rollback(configuration configuration) (report, error) {
	expected, err := readManifest(configuration.manifestPath)
	if err != nil {
		return report{}, err
	}
	before, err := durable.ReadLegacyJournal(configuration.journalPath)
	if err != nil {
		return report{}, fmt.Errorf("read V0 source before rollback: %w", err)
	}
	if before.Digest != expected.SourceDigest || before.RecordCount != expected.SourceRecords {
		return report{}, fmt.Errorf("V0 source differs from the imported source manifest")
	}
	runtime, err := durable.Open(configuration.journalPath, before.ReservationTTL)
	if err != nil {
		return report{}, fmt.Errorf("restart V0 runtime: %w", err)
	}
	assignment, exists, assignmentErr := runtime.Assignment(rehearsalAssignmentID)
	readyErr := runtime.Ready()
	closeErr := runtime.Close()
	if assignmentErr != nil || !exists || assignment.Status != domain.AssignmentCompleted || assignment.Acknowledgment == nil {
		return report{}, fmt.Errorf("restarted V0 terminal assignment is incomplete")
	}
	if readyErr != nil {
		return report{}, fmt.Errorf("restarted V0 runtime is not ready: %w", readyErr)
	}
	if closeErr != nil {
		return report{}, fmt.Errorf("close restarted V0 runtime: %w", closeErr)
	}
	after, err := durable.ReadLegacyJournal(configuration.journalPath)
	if err != nil {
		return report{}, fmt.Errorf("read V0 source after rollback: %w", err)
	}
	if after.Digest != before.Digest || after.RecordCount != before.RecordCount {
		return report{}, fmt.Errorf("V0 rollback restart changed the stopped source")
	}
	return report{Schema: reportSchema, Phase: "rollback", SourceRecords: after.RecordCount}, nil
}

func openRepository(
	ctx context.Context,
	schema string,
	migrate bool,
) (*pgxpool.Pool, repository.Repository, error) {
	config, err := pgxpool.ParseConfig(os.Getenv(connectionEnvironment))
	if err != nil {
		return nil, nil, fmt.Errorf("parse PostgreSQL connection configuration: %w", err)
	}
	config.ConnConfig.RuntimeParams["search_path"] = schema
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, nil, fmt.Errorf("open PostgreSQL rehearsal pool: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, nil, fmt.Errorf("ping PostgreSQL rehearsal pool: %w", err)
	}
	if migrate {
		if err := postgresrepository.Migrate(ctx, pool); err != nil {
			pool.Close()
			return nil, nil, fmt.Errorf("migrate PostgreSQL rehearsal schema: %w", err)
		}
	}
	owner, err := postgresrepository.New(pool)
	if err != nil {
		pool.Close()
		return nil, nil, err
	}
	if !migrate {
		if _, err := owner.Snapshot(ctx, rehearsalScope); err != nil {
			pool.Close()
			return nil, nil, fmt.Errorf("open restored repository state: %w", err)
		}
	}
	return pool, owner, nil
}

func createLegacyFixture(path string) (durable.LegacyJournal, error) {
	if _, err := os.Stat(path); err == nil {
		return durable.LegacyJournal{}, fmt.Errorf("V0 fixture path already exists")
	} else if !errors.Is(err, os.ErrNotExist) {
		return durable.LegacyJournal{}, fmt.Errorf("inspect V0 fixture path: %w", err)
	}
	runtime, err := durable.Open(path, time.Minute)
	if err != nil {
		return durable.LegacyJournal{}, fmt.Errorf("create V0 fixture: %w", err)
	}
	closeRuntime := func(cause error) (durable.LegacyJournal, error) {
		return durable.LegacyJournal{}, errors.Join(cause, runtime.Close())
	}
	policy := domain.MatchmakingPolicy{
		Version: "rehearsal-policy", TeamCount: 2, TeamSize: 2,
		MaxLatencyMillis: 100, MaxProposals: 8, MaxSearchNodes: 1000,
		RelaxationSteps: []domain.RelaxationStep{{AfterWait: 0, MaxTeamSkillGap: 100}},
	}
	if _, err := runtime.RegisterPolicy(policy); err != nil {
		return closeRuntime(err)
	}
	for index := range 4 {
		ticket := domain.MatchTicket{
			ID: domain.TicketID(fmt.Sprintf("rehearsal-ticket-%d", index)), Revision: 1,
			EnqueuedAt: fixtureNow().Add(-time.Second),
			Players: []domain.Player{{
				ID:    domain.PlayerID(fmt.Sprintf("rehearsal-player-%d", index)),
				Skill: 1500 + index, LatencyMillis: 30,
			}},
		}
		if err := runtime.SubmitMatchTicket(ticket); err != nil {
			return closeRuntime(err)
		}
	}
	batch, err := runtime.Plan("rehearsal-run", fixtureNow(), policy.Version)
	if err != nil {
		return closeRuntime(err)
	}
	if len(batch.Proposals) != 1 {
		return closeRuntime(fmt.Errorf("fixture proposal count is %d; want 1", len(batch.Proposals)))
	}
	if _, err := runtime.Reserve(batch.Proposals[0], "rehearsal-reservation", fixtureNow()); err != nil {
		return closeRuntime(err)
	}
	if _, err := runtime.Confirm("rehearsal-reservation", rehearsalAssignmentID, fixtureNow().Add(time.Second)); err != nil {
		return closeRuntime(err)
	}
	if _, err := runtime.AcknowledgeAssignment(
		rehearsalAssignmentID,
		domain.AssignmentAcknowledgmentRequest{
			OperationID: "rehearsal-acknowledgment", Outcome: domain.AssignmentCompleted,
		},
		fixtureNow().Add(2*time.Second),
	); err != nil {
		return closeRuntime(err)
	}
	if err := runtime.SubmitMatchTicket(domain.MatchTicket{
		ID: "rehearsal-active", Revision: 1, EnqueuedAt: fixtureNow().Add(3 * time.Second),
		Players: []domain.Player{{ID: "rehearsal-active-player", Skill: 1510, LatencyMillis: 35}},
	}); err != nil {
		return closeRuntime(err)
	}
	if err := runtime.Close(); err != nil {
		return durable.LegacyJournal{}, err
	}
	return durable.ReadLegacyJournal(path)
}

func captureManifest(
	ctx context.Context,
	pool *pgxpool.Pool,
	owner repository.Repository,
	source durable.LegacyJournal,
) (manifest, error) {
	snapshot, err := owner.Snapshot(ctx, rehearsalScope)
	if err != nil {
		return manifest{}, fmt.Errorf("capture repository snapshot: %w", err)
	}
	audit, err := allAudit(ctx, owner, rehearsalScope)
	if err != nil {
		return manifest{}, err
	}
	tableRows, err := repositoryTableRows(ctx, pool)
	if err != nil {
		return manifest{}, err
	}
	authorityDigest, err := repositoryAuthorityDigest(ctx, pool)
	if err != nil {
		return manifest{}, err
	}
	return manifest{
		Schema: manifestSchema, SourceDigest: source.Digest, SourceRecords: source.RecordCount,
		SnapshotVersion: uint64(snapshot.Version), ResourceCount: len(snapshot.Resources),
		ResourceDigest: resourceDigest(snapshot), AuditCount: len(audit), AuditDigest: auditDigest(audit),
		AuthorityDigest: authorityDigest, TableRows: tableRows,
	}, nil
}

func allAudit(ctx context.Context, owner repository.Repository, scope string) ([]repository.AuditRecord, error) {
	var records []repository.AuditRecord
	var after repository.Version
	for {
		page, err := owner.Audit(ctx, scope, after, 1000)
		if err != nil {
			return nil, fmt.Errorf("capture repository audit: %w", err)
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
			return nil, fmt.Errorf("count PostgreSQL rehearsal table: %w", err)
		}
		counts[table] = count
	}
	return counts, nil
}

func repositoryAuthorityDigest(ctx context.Context, pool *pgxpool.Pool) (string, error) {
	hash := sha256.New()
	metadata, err := pool.Query(ctx, `SELECT key, value FROM sema_repository_metadata ORDER BY key`)
	if err != nil {
		return "", fmt.Errorf("read PostgreSQL rehearsal metadata: %w", err)
	}
	for metadata.Next() {
		var key, value string
		if err := metadata.Scan(&key, &value); err != nil {
			metadata.Close()
			return "", fmt.Errorf("scan PostgreSQL rehearsal metadata: %w", err)
		}
		fmt.Fprintf(hash, "metadata\x00%s\x00%s\n", key, value)
	}
	if err := metadata.Err(); err != nil {
		metadata.Close()
		return "", fmt.Errorf("iterate PostgreSQL rehearsal metadata: %w", err)
	}
	metadata.Close()

	scopes, err := pool.Query(ctx, `SELECT scope, version FROM sema_repository_scopes ORDER BY scope`)
	if err != nil {
		return "", fmt.Errorf("read PostgreSQL rehearsal scopes: %w", err)
	}
	for scopes.Next() {
		var scope string
		var version int64
		if err := scopes.Scan(&scope, &version); err != nil {
			scopes.Close()
			return "", fmt.Errorf("scan PostgreSQL rehearsal scopes: %w", err)
		}
		fmt.Fprintf(hash, "scope\x00%s\x00%d\n", scope, version)
	}
	if err := scopes.Err(); err != nil {
		scopes.Close()
		return "", fmt.Errorf("iterate PostgreSQL rehearsal scopes: %w", err)
	}
	scopes.Close()

	operations, err := pool.Query(ctx, `
		SELECT scope, operation_id, digest, operation_kind, occurred_at, version
		FROM sema_repository_operations
		ORDER BY scope, operation_id`)
	if err != nil {
		return "", fmt.Errorf("read PostgreSQL rehearsal operation authority: %w", err)
	}
	for operations.Next() {
		var scope, operationID, operationKind string
		var digest []byte
		var occurredAt time.Time
		var version sql.NullInt64
		if err := operations.Scan(&scope, &operationID, &digest, &operationKind, &occurredAt, &version); err != nil {
			operations.Close()
			return "", fmt.Errorf("scan PostgreSQL rehearsal operation authority: %w", err)
		}
		fmt.Fprintf(hash, "operation\x00%s\x00%s\x00%s\x00%s\x00%s\x00%t\x00%d\n",
			scope, operationID, hex.EncodeToString(digest), operationKind,
			occurredAt.UTC().Format(time.RFC3339Nano), version.Valid, version.Int64,
		)
	}
	if err := operations.Err(); err != nil {
		operations.Close()
		return "", fmt.Errorf("iterate PostgreSQL rehearsal operation authority: %w", err)
	}
	operations.Close()
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func resourceDigest(snapshot repository.Snapshot) string {
	hash := sha256.New()
	for _, resource := range snapshot.Resources {
		payloadDigest := sha256.Sum256(resource.Payload)
		fmt.Fprintf(hash, "%s\x00%s\x00%s\x00%d\x00%t\x00%s\n",
			resource.Key.Scope, resource.Key.Kind, resource.Key.ID, resource.Version, resource.Deleted,
			hex.EncodeToString(payloadDigest[:]),
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
		fmt.Fprintf(hash, "%d\x00%s\x00%s", record.Version, record.OperationKind, record.At.UTC().Format(time.RFC3339Nano))
		for _, kind := range kinds {
			fmt.Fprintf(hash, "\x00%s=%d", kind, record.ResourceCounts[kind])
		}
		fmt.Fprintln(hash)
	}
	return hex.EncodeToString(hash.Sum(nil))
}

func writeManifest(path string, evidence manifest) error {
	directory := filepath.Dir(path)
	temporary, err := os.CreateTemp(directory, ".sema-postgres-rehearsal-*")
	if err != nil {
		return fmt.Errorf("create private rehearsal manifest: %w", err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0o600); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("protect private rehearsal manifest: %w", err)
	}
	encoder := json.NewEncoder(temporary)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(evidence); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("encode private rehearsal manifest: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("sync private rehearsal manifest: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close private rehearsal manifest: %w", err)
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return fmt.Errorf("install private rehearsal manifest: %w", err)
	}
	return nil
}

func readManifest(path string) (manifest, error) {
	file, err := os.Open(path)
	if err != nil {
		return manifest{}, fmt.Errorf("open private rehearsal manifest: %w", err)
	}
	defer file.Close()
	decoder := json.NewDecoder(file)
	decoder.DisallowUnknownFields()
	var evidence manifest
	if err := decoder.Decode(&evidence); err != nil {
		return manifest{}, fmt.Errorf("decode private rehearsal manifest: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return manifest{}, fmt.Errorf("private rehearsal manifest has trailing content")
	}
	if evidence.Schema != manifestSchema || evidence.SourceDigest == "" || evidence.SourceRecords <= 0 ||
		evidence.SnapshotVersion == 0 || evidence.ResourceCount <= 0 || evidence.ResourceDigest == "" ||
		evidence.AuditCount <= 0 || evidence.AuditDigest == "" || evidence.AuthorityDigest == "" ||
		len(evidence.TableRows) == 0 {
		return manifest{}, fmt.Errorf("private rehearsal manifest is incomplete")
	}
	return evidence, nil
}

func manifestsEqual(left, right manifest) bool {
	leftJSON, leftErr := json.Marshal(left)
	rightJSON, rightErr := json.Marshal(right)
	return leftErr == nil && rightErr == nil && string(leftJSON) == string(rightJSON)
}

func reportFromManifest(phase string, evidence manifest) report {
	return report{
		Schema: reportSchema, Phase: phase, SourceRecords: evidence.SourceRecords,
		SnapshotVersion: evidence.SnapshotVersion, ResourceCount: evidence.ResourceCount,
		AuditCount: evidence.AuditCount,
	}
}

func fixtureNow() time.Time {
	return time.Date(2026, 7, 18, 0, 0, 0, 0, time.UTC)
}
