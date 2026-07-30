// Package serviceworkload executes the bounded PostgreSQL target-service
// workload used to calibrate admission, pool, timeout, and latency budgets.
package serviceworkload

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	api "github.com/zrma/sema/internal/api/v0alpha2"
	postgresrepository "github.com/zrma/sema/internal/repository/postgres"
	"github.com/zrma/sema/internal/targetapi"
	"github.com/zrma/sema/internal/targetruntime"
	"github.com/zrma/sema/internal/wireconformance"
)

const (
	ReportSchema  = "sema.service-workload.v1"
	ProfileID     = "sema-standard-postgres-v1"
	workloadToken = "service-workload-token"
)

type Options struct {
	PostgresDSN     string
	Runs            int
	Cycles          int
	TicketsPerCycle int
	Concurrency     int
	MaxInFlight     int
	RequestTimeout  time.Duration
	Pool            postgresrepository.PoolOptions
	Random          io.Reader
	Now             func() time.Time
}

type Budget struct {
	MaxP95Millis         float64 `json:"max_p95_millis"`
	MaxRequestMillis     float64 `json:"max_request_millis"`
	MaxRunDurationMillis float64 `json:"max_run_duration_millis"`
}

type LatencySummary struct {
	P50Millis float64 `json:"p50_millis"`
	P95Millis float64 `json:"p95_millis"`
	P99Millis float64 `json:"p99_millis"`
	MaxMillis float64 `json:"max_millis"`
}

type PoolProfile struct {
	MaxConnections     int32   `json:"max_connections"`
	MinIdleConnections int32   `json:"min_idle_connections"`
	TotalAfterRun      int32   `json:"total_after_run"`
	AcquireCount       int64   `json:"acquire_count"`
	AcquireWaitMillis  float64 `json:"acquire_wait_millis"`
	EmptyAcquireCount  int64   `json:"empty_acquire_count"`
	EmptyWaitMillis    float64 `json:"empty_wait_millis"`
	CanceledAcquires   int64   `json:"canceled_acquires"`
}

type RunReport struct {
	Tickets          int            `json:"tickets"`
	Matches          int            `json:"matches"`
	Assignments      int            `json:"assignments"`
	Operations       int            `json:"operations"`
	ResourceRejected int            `json:"resource_exhausted"`
	DurationMillis   float64        `json:"duration_millis"`
	MatchesPerSecond float64        `json:"matches_per_second"`
	MetricsVerified  bool           `json:"metrics_verified"`
	Latency          LatencySummary `json:"latency"`
	Pool             PoolProfile    `json:"pool"`
}

type Report struct {
	Schema            string      `json:"schema"`
	Profile           string      `json:"profile"`
	Authentication    string      `json:"authentication"`
	Runs              int         `json:"runs"`
	CyclesPerRun      int         `json:"cycles_per_run"`
	TicketsPerCycle   int         `json:"tickets_per_cycle"`
	ClientConcurrency int         `json:"client_concurrency"`
	MaxInFlight       int         `json:"max_in_flight"`
	RequestTimeoutMS  int64       `json:"request_timeout_millis"`
	Budget            Budget      `json:"budget"`
	Results           []RunReport `json:"results"`
	WorstP95Millis    float64     `json:"worst_p95_millis"`
	WorstRequestMS    float64     `json:"worst_request_millis"`
	WorstRunMS        float64     `json:"worst_run_millis"`
	WithinBudget      bool        `json:"within_budget"`
}

func ReferenceBudget() Budget {
	return Budget{
		MaxP95Millis:         750,
		MaxRequestMillis:     2_000,
		MaxRunDurationMillis: 30_000,
	}
}

func DefaultOptions() Options {
	return Options{
		Runs: 3, Cycles: 3, TicketsPerCycle: 100, Concurrency: 32,
		MaxInFlight: 64, RequestTimeout: 5 * time.Second,
		Pool:   postgresrepository.DefaultPoolOptions(),
		Random: rand.Reader, Now: time.Now,
	}
}

// Run executes independent schemas and returns aggregate-only evidence.
func Run(ctx context.Context, options Options, budget Budget) (Report, error) {
	if err := Validate(options, budget); err != nil {
		return Report{}, err
	}
	report := Report{
		Schema: ReportSchema, Profile: ProfileID,
		Authentication: "deterministic-valid-principal",
		Runs:           options.Runs, CyclesPerRun: options.Cycles,
		TicketsPerCycle:   options.TicketsPerCycle,
		ClientConcurrency: options.Concurrency, MaxInFlight: options.MaxInFlight,
		RequestTimeoutMS: options.RequestTimeout.Milliseconds(),
		Budget:           budget, WithinBudget: true,
	}
	for range options.Runs {
		current, err := runOnce(ctx, options)
		if err != nil {
			return Report{}, err
		}
		report.Results = append(report.Results, current)
		report.WorstP95Millis = max(report.WorstP95Millis, current.Latency.P95Millis)
		report.WorstRequestMS = max(report.WorstRequestMS, current.Latency.MaxMillis)
		report.WorstRunMS = max(report.WorstRunMS, current.DurationMillis)
		if !current.MetricsVerified ||
			current.ResourceRejected != 0 || current.Pool.CanceledAcquires != 0 ||
			current.Latency.P95Millis > budget.MaxP95Millis ||
			current.Latency.MaxMillis > budget.MaxRequestMillis ||
			current.DurationMillis > budget.MaxRunDurationMillis {
			report.WithinBudget = false
		}
	}
	if !report.WithinBudget {
		return report, fmt.Errorf("standard service workload exceeded its reference budget")
	}
	return report, nil
}

// Validate rejects unsafe or non-reference-compatible workload bounds without
// opening a database connection.
func Validate(options Options, budget Budget) error {
	if options.PostgresDSN == "" {
		return fmt.Errorf("PostgreSQL test DSN is required")
	}
	if options.Runs <= 0 || options.Runs > 20 || options.Cycles <= 0 || options.Cycles > 100 ||
		options.TicketsPerCycle <= 0 || options.TicketsPerCycle > 250 ||
		options.TicketsPerCycle%10 != 0 || options.Concurrency <= 0 ||
		options.Concurrency > 256 || options.MaxInFlight <= 0 ||
		options.MaxInFlight > 4096 || options.RequestTimeout <= 0 {
		return fmt.Errorf("service workload configuration is outside supported bounds")
	}
	if options.Random == nil || options.Now == nil {
		return fmt.Errorf("service workload random source and clock are required")
	}
	if budget.MaxP95Millis <= 0 || budget.MaxRequestMillis <= 0 ||
		budget.MaxRunDurationMillis <= 0 {
		return fmt.Errorf("service workload budgets must be positive")
	}
	return nil
}

func runOnce(ctx context.Context, options Options) (RunReport, error) {
	suffix, err := randomSuffix(options.Random)
	if err != nil {
		return RunReport{}, err
	}
	schema := "sema_service_workload_" + suffix
	admin, err := pgxpool.New(ctx, options.PostgresDSN)
	if err != nil {
		return RunReport{}, fmt.Errorf("open workload administration pool: %w", err)
	}
	defer admin.Close()
	if _, err := admin.Exec(ctx, "CREATE SCHEMA "+pgx.Identifier{schema}.Sanitize()); err != nil {
		return RunReport{}, fmt.Errorf("create workload schema: %w", err)
	}
	defer func() {
		cleanupContext, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_, _ = admin.Exec(
			cleanupContext,
			"DROP SCHEMA IF EXISTS "+pgx.Identifier{schema}.Sanitize()+" CASCADE",
		)
	}()
	dsn, err := withSearchPath(options.PostgresDSN, schema)
	if err != nil {
		return RunReport{}, err
	}
	migrationPool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return RunReport{}, fmt.Errorf("open workload migration pool: %w", err)
	}
	if err := postgresrepository.Migrate(ctx, migrationPool); err != nil {
		migrationPool.Close()
		return RunReport{}, fmt.Errorf("migrate workload schema: %w", err)
	}
	migrationPool.Close()

	store, err := postgresrepository.OpenWithOptions(ctx, dsn, options.Pool)
	if err != nil {
		return RunReport{}, err
	}
	defer store.Close()
	handler, err := targetruntime.New(store, workloadAuthenticator(), targetruntime.Options{
		CursorKey:      bytes.Repeat([]byte{0x51}, 32),
		ReservationTTL: time.Minute, MaxInFlight: options.MaxInFlight,
		RequestTimeout: options.RequestTimeout, ReadinessTimeout: time.Second,
		ReadinessCheck: store.Ready,
	})
	if err != nil {
		return RunReport{}, err
	}
	server := httptest.NewServer(handler)
	defer server.Close()
	recorder := &latencyTransport{next: http.DefaultTransport}
	client := &http.Client{
		Transport: recorder,
		Timeout:   options.RequestTimeout + time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	started := time.Now()
	runID := "load-" + suffix
	result, err := executeLifecycle(ctx, client, server.URL, runID, options)
	if err != nil {
		return RunReport{}, err
	}
	if err := verifyMetrics(ctx, server.URL, runID, options.RequestTimeout); err != nil {
		return RunReport{}, err
	}
	result.MetricsVerified = true
	result.DurationMillis = milliseconds(time.Since(started))
	if result.DurationMillis > 0 {
		result.MatchesPerSecond = float64(result.Matches) / (result.DurationMillis / 1_000)
	}
	result.Operations, result.ResourceRejected, result.Latency = recorder.Summary()
	result.Pool = poolProfile(options.Pool, store.Stats())
	return result, nil
}

func verifyMetrics(
	ctx context.Context,
	baseURL string,
	privateRunID string,
	timeout time.Duration,
) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/metrics", nil)
	if err != nil {
		return fmt.Errorf("build metrics request: %w", err)
	}
	client := &http.Client{Timeout: timeout}
	response, err := client.Do(request)
	if err != nil {
		return fmt.Errorf("scrape workload metrics: %w", err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		return fmt.Errorf("read workload metrics: %w", err)
	}
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("workload metrics returned HTTP %d", response.StatusCode)
	}
	text := string(body)
	for _, route := range []string{
		"PUT /v0alpha2/match-tickets/{ticket_id}",
		"POST /v0alpha2/planning-runs/{run_id}",
		"POST /v0alpha2/reservations/{reservation_id}/confirm",
		"POST /v0alpha2/assignments/{assignment_id}/acknowledgments",
	} {
		if !strings.Contains(text, `route="`+route+`"`) {
			return fmt.Errorf("workload metrics omit bounded route %q", route)
		}
	}
	for _, forbidden := range []string{privateRunID, workloadToken, "service-workload"} {
		if strings.Contains(text, forbidden) {
			return fmt.Errorf("workload metrics exposed private run identity")
		}
	}
	return nil
}

func executeLifecycle(
	ctx context.Context,
	client *http.Client,
	baseURL string,
	runID string,
	options Options,
) (RunReport, error) {
	policyVersion := runID + "-policy"
	policy := api.MatchmakingPolicy{
		Version: policyVersion, TeamCount: 2, TeamSize: 5, MaxLatencyMillis: 100,
		MaxProposals: options.TicketsPerCycle / 10, MaxSearchNodes: 1_000_000,
		MaxCandidateTickets: 256, MaxCandidatesPerProposal: 128,
		MaxBatchCandidates: 512, MaxBatchSearchNodes: 1_000_000,
		RelaxationSteps: []api.RelaxationStep{{AfterWaitMillis: 0, MaxTeamSkillGap: 200}},
	}
	if _, err := measuredRequest[api.PolicyMutation](
		ctx, client, baseURL, runID+"-policy-put", http.MethodPut,
		"/v0alpha2/policies/"+policyVersion, policy,
	); err != nil {
		return RunReport{}, fmt.Errorf("register workload policy: %w", err)
	}

	result := RunReport{}
	for cycle := range options.Cycles {
		now := options.Now().UTC()
		if err := runParallel(options.TicketsPerCycle, options.Concurrency, func(index int) error {
			id := fmt.Sprintf("%s-c%03d-t%03d", runID, cycle, index)
			ticket := api.MatchTicket{
				ID: id, Revision: 1,
				EnqueuedAt: now.Add(-time.Duration(options.TicketsPerCycle-index) * time.Millisecond),
				Players: []api.Player{{
					ID: id + "-player", Skill: 1450 + index%100,
					Role: "flex", LatencyMillis: 20 + index%40,
				}},
			}
			_, err := measuredRequest[api.MatchTicketMutation](
				ctx, client, baseURL, id+"-put", http.MethodPut,
				"/v0alpha2/match-tickets/"+id, ticket,
			)
			return err
		}); err != nil {
			return RunReport{}, fmt.Errorf("cycle %d ticket ingestion: %w", cycle, err)
		}
		result.Tickets += options.TicketsPerCycle

		planningID := fmt.Sprintf("%s-c%03d-plan", runID, cycle)
		planning, err := measuredRequest[api.PlanningRunMutation](
			ctx, client, baseURL, planningID+"-execute", http.MethodPost,
			"/v0alpha2/planning-runs/"+planningID,
			api.PlanningRunRequest{PolicyVersion: policyVersion},
		)
		if err != nil {
			return RunReport{}, fmt.Errorf("cycle %d planning: %w", cycle, err)
		}
		expectedMatches := options.TicketsPerCycle / 10
		if planning.Resource.Status != "completed" ||
			planning.Resource.ProposalCount != expectedMatches ||
			planning.Resource.UnmatchedCount != 0 {
			return RunReport{}, fmt.Errorf("cycle %d planning result violated the workload fixture", cycle)
		}
		proposals, err := measuredRequest[api.ProposalPage](
			ctx, client, baseURL, "", http.MethodGet,
			"/v0alpha2/planning-runs/"+planningID+"/proposals", nil,
		)
		if err != nil {
			return RunReport{}, fmt.Errorf("cycle %d proposal read: %w", cycle, err)
		}
		if len(proposals.Items) != expectedMatches {
			return RunReport{}, fmt.Errorf("cycle %d returned %d proposals; want %d", cycle, len(proposals.Items), expectedMatches)
		}

		reservations := make([]api.ReservationMutation, len(proposals.Items))
		if err := runParallel(len(proposals.Items), options.Concurrency, func(index int) error {
			id := fmt.Sprintf("%s-c%03d-r%03d", runID, cycle, index)
			current, err := measuredRequest[api.ReservationMutation](
				ctx, client, baseURL, id+"-reserve", http.MethodPost,
				"/v0alpha2/reservations/"+id,
				api.ReservationRequest{ProposalID: proposals.Items[index].Proposal.ID},
			)
			if err == nil {
				reservations[index] = current
			}
			return err
		}); err != nil {
			return RunReport{}, fmt.Errorf("cycle %d reservation: %w", cycle, err)
		}
		assignments := make([]api.AssignmentMutation, len(reservations))
		if err := runParallel(len(reservations), options.Concurrency, func(index int) error {
			assignmentID := fmt.Sprintf("%s-c%03d-a%03d", runID, cycle, index)
			current, err := measuredRequest[api.AssignmentMutation](
				ctx, client, baseURL, assignmentID+"-confirm", http.MethodPost,
				"/v0alpha2/reservations/"+reservations[index].Resource.Reservation.ID+"/confirm",
				api.ConfirmReservationRequest{AssignmentID: assignmentID},
			)
			if err == nil {
				assignments[index] = current
			}
			return err
		}); err != nil {
			return RunReport{}, fmt.Errorf("cycle %d confirmation: %w", cycle, err)
		}
		if err := runParallel(len(assignments), options.Concurrency, func(index int) error {
			assignmentID := assignments[index].Resource.Assignment.ID
			completed, err := measuredRequest[api.AssignmentMutation](
				ctx, client, baseURL, assignmentID+"-complete", http.MethodPost,
				"/v0alpha2/assignments/"+assignmentID+"/acknowledgments",
				api.AcknowledgeAssignmentRequest{Outcome: "completed"},
			)
			if err == nil && completed.Resource.Assignment.Status != "completed" {
				return fmt.Errorf("assignment did not become terminal")
			}
			return err
		}); err != nil {
			return RunReport{}, fmt.Errorf("cycle %d acknowledgment: %w", cycle, err)
		}
		result.Matches += len(assignments)
		result.Assignments += len(assignments)
	}
	return result, nil
}

func measuredRequest[T any](
	ctx context.Context,
	client *http.Client,
	baseURL string,
	operationID string,
	method string,
	path string,
	body any,
) (T, error) {
	return wireconformance.RequestData[T](
		ctx, client, baseURL, workloadToken, operationID, method, path, body,
	)
}

func runParallel(count int, concurrency int, task func(int) error) error {
	if count == 0 {
		return nil
	}
	workers := min(count, concurrency)
	jobs := make(chan int)
	errs := make(chan error, count)
	var group sync.WaitGroup
	group.Add(workers)
	for range workers {
		go func() {
			defer group.Done()
			for index := range jobs {
				if err := task(index); err != nil {
					errs <- err
				}
			}
		}()
	}
	for index := range count {
		jobs <- index
	}
	close(jobs)
	group.Wait()
	close(errs)
	var collected []error
	for err := range errs {
		collected = append(collected, err)
	}
	return errors.Join(collected...)
}

type latencyTransport struct {
	next http.RoundTripper
	mu   sync.Mutex
	all  []time.Duration
	over int
}

func (transport *latencyTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	started := time.Now()
	response, err := transport.next.RoundTrip(request)
	elapsed := time.Since(started)
	transport.mu.Lock()
	transport.all = append(transport.all, elapsed)
	if response != nil && response.Header.Get("X-Sema-Error-Code") == "ResourceExhausted" {
		transport.over++
	}
	transport.mu.Unlock()
	return response, err
}

func (transport *latencyTransport) Summary() (int, int, LatencySummary) {
	transport.mu.Lock()
	defer transport.mu.Unlock()
	ordered := slices.Clone(transport.all)
	slices.Sort(ordered)
	if len(ordered) == 0 {
		return 0, transport.over, LatencySummary{}
	}
	return len(ordered), transport.over, LatencySummary{
		P50Millis: milliseconds(percentile(ordered, 50)),
		P95Millis: milliseconds(percentile(ordered, 95)),
		P99Millis: milliseconds(percentile(ordered, 99)),
		MaxMillis: milliseconds(ordered[len(ordered)-1]),
	}
}

func percentile(ordered []time.Duration, percentile int) time.Duration {
	index := (len(ordered)*percentile + 99) / 100
	index = max(1, index)
	return ordered[index-1]
}

func poolProfile(options postgresrepository.PoolOptions, stats postgresrepository.PoolStats) PoolProfile {
	return PoolProfile{
		MaxConnections: options.MaxConnections, MinIdleConnections: options.MinIdleConnections,
		TotalAfterRun: stats.TotalConnections, AcquireCount: stats.AcquireCount,
		AcquireWaitMillis: milliseconds(stats.AcquireDuration),
		EmptyAcquireCount: stats.EmptyAcquireCount,
		EmptyWaitMillis:   milliseconds(stats.EmptyAcquireWaitTime),
		CanceledAcquires:  stats.CanceledAcquireCount,
	}
}

func workloadAuthenticator() targetapi.Authenticator {
	allPermissions := []targetapi.Permission{
		targetapi.PermissionMatchTicketsRead,
		targetapi.PermissionMatchTicketsWrite,
		targetapi.PermissionBackfillTicketsRead,
		targetapi.PermissionBackfillTicketsWrite,
		targetapi.PermissionPoliciesRead,
		targetapi.PermissionPoliciesWrite,
		targetapi.PermissionPlanningRunsRead,
		targetapi.PermissionPlanningRunsWrite,
		targetapi.PermissionReservationsRead,
		targetapi.PermissionReservationsWrite,
		targetapi.PermissionAssignmentsRead,
		targetapi.PermissionAssignmentsWrite,
	}
	permissions := make(map[targetapi.Permission]bool, len(allPermissions))
	for _, permission := range allPermissions {
		permissions[permission] = true
	}
	return targetapi.AuthenticatorFunc(func(request *http.Request) (targetapi.Principal, error) {
		if request.Header.Get("Authorization") != "Bearer "+workloadToken {
			return targetapi.Principal{}, targetapi.ErrUnauthenticated
		}
		return targetapi.Principal{
			Subject: "service-workload", Tenant: "service-workload",
			Permissions: permissions,
		}, nil
	})
}

func randomSuffix(source io.Reader) (string, error) {
	value := make([]byte, 8)
	if _, err := io.ReadFull(source, value); err != nil {
		return "", fmt.Errorf("generate workload identity: %w", err)
	}
	return hex.EncodeToString(value), nil
}

func withSearchPath(dsn string, schema string) (string, error) {
	config, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return "", fmt.Errorf("parse workload PostgreSQL DSN: %w", err)
	}
	config.ConnConfig.RuntimeParams["search_path"] = schema
	return config.ConnString(), nil
}

func milliseconds(value time.Duration) float64 {
	return float64(value) / float64(time.Millisecond)
}
