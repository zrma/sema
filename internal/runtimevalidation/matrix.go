// Package runtimevalidation provides repository-owned multi-replica and
// dependency-recovery evidence for the standard PostgreSQL runtime.
package runtimevalidation

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
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
	reportSchema = "sema.runtime-failure-matrix.v1"
	matrixToken  = "matrix-full-lifecycle-token"
)

// Options configures one disposable runtime matrix.
type Options struct {
	PostgresDSN string
	Timeout     time.Duration
}

// Report contains aggregate pass evidence only. It intentionally excludes
// database addresses, schema names, tokens, resource IDs, and raw payloads.
type Report struct {
	Schema                           string `json:"schema"`
	ReplicaCount                     int    `json:"replica_count"`
	ReservationWinnerCount           int    `json:"reservation_winner_count"`
	ReservationConflictCount         int    `json:"reservation_conflict_count"`
	TerminalAgreement                bool   `json:"terminal_agreement"`
	ReplicaRestartRecovered          bool   `json:"replica_restart_recovered"`
	DependencyReadinessFailedClosed  bool   `json:"dependency_readiness_failed_closed"`
	DependencyRequestFailedRetryable bool   `json:"dependency_request_failed_retryable"`
	LivenessMaintained               bool   `json:"liveness_maintained"`
	DependencyRecoveryComplete       bool   `json:"dependency_recovery_complete"`
}

// Run executes a two-replica contention, restart, PostgreSQL outage, and
// recovery matrix against an isolated schema.
func Run(ctx context.Context, options Options) (Report, error) {
	if options.PostgresDSN == "" {
		return Report{}, fmt.Errorf("PostgreSQL test DSN is required")
	}
	if options.Timeout <= 0 {
		return Report{}, fmt.Errorf("runtime validation timeout must be positive")
	}
	ctx, cancel := context.WithTimeout(ctx, options.Timeout)
	defer cancel()

	suffix, err := randomSuffix()
	if err != nil {
		return Report{}, err
	}
	schema := "sema_runtime_matrix_" + suffix
	admin, err := pgxpool.New(ctx, options.PostgresDSN)
	if err != nil {
		return Report{}, fmt.Errorf("open matrix administration pool: %w", err)
	}
	defer admin.Close()
	if err := admin.Ping(ctx); err != nil {
		return Report{}, fmt.Errorf("ping matrix PostgreSQL: %w", err)
	}
	if _, err := admin.Exec(ctx, "CREATE SCHEMA "+pgx.Identifier{schema}.Sanitize()); err != nil {
		return Report{}, fmt.Errorf("create matrix schema: %w", err)
	}
	defer func() {
		cleanupContext, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		_, _ = admin.Exec(
			cleanupContext,
			"DROP SCHEMA IF EXISTS "+pgx.Identifier{schema}.Sanitize()+" CASCADE",
		)
	}()

	migratedDSN, err := withSearchPath(options.PostgresDSN, schema)
	if err != nil {
		return Report{}, err
	}
	migrationPool, err := pgxpool.New(ctx, migratedDSN)
	if err != nil {
		return Report{}, fmt.Errorf("open matrix migration pool: %w", err)
	}
	if err := postgresrepository.Migrate(ctx, migrationPool); err != nil {
		migrationPool.Close()
		return Report{}, fmt.Errorf("migrate matrix schema: %w", err)
	}
	migrationPool.Close()

	proxy, proxiedDSN, err := newPostgresProxy(migratedDSN)
	if err != nil {
		return Report{}, err
	}
	defer proxy.Close()

	authenticator := matrixAuthenticator()
	replicas := make([]*replica, 2)
	for index := range replicas {
		replicas[index], err = startReplica(ctx, proxiedDSN, authenticator)
		if err != nil {
			for previous := 0; previous < index; previous++ {
				replicas[previous].Close()
			}
			return Report{}, fmt.Errorf("start replica %d: %w", index+1, err)
		}
	}
	defer func() {
		for _, current := range replicas {
			if current != nil {
				current.Close()
			}
		}
	}()

	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}}
	runID := "matrix-" + suffix
	proposalID, err := seedProposal(ctx, client, replicas[0].URL(), runID)
	if err != nil {
		return Report{}, err
	}

	winnerReplica, winnerReservation, conflictCount, err := raceReservations(
		ctx,
		client,
		[]string{replicas[0].URL(), replicas[1].URL()},
		runID,
		proposalID,
	)
	if err != nil {
		return Report{}, err
	}
	report := Report{
		Schema:                   reportSchema,
		ReplicaCount:             len(replicas),
		ReservationWinnerCount:   1,
		ReservationConflictCount: conflictCount,
	}

	assignmentID := runID + "-assignment"
	confirmReplica := replicas[(winnerReplica+1)%len(replicas)]
	if _, err := wireconformance.RequestData[api.AssignmentMutation](
		ctx,
		client,
		confirmReplica.URL(),
		matrixToken,
		runID+"-confirm",
		http.MethodPost,
		"/v0alpha2/reservations/"+winnerReservation+"/confirm",
		api.ConfirmReservationRequest{AssignmentID: assignmentID},
	); err != nil {
		return Report{}, fmt.Errorf("confirm winning reservation through peer replica: %w", err)
	}
	if _, err := wireconformance.RequestData[api.AssignmentMutation](
		ctx,
		client,
		replicas[winnerReplica].URL(),
		matrixToken,
		runID+"-acknowledge",
		http.MethodPost,
		"/v0alpha2/assignments/"+assignmentID+"/acknowledgments",
		api.AcknowledgeAssignmentRequest{Outcome: "completed"},
	); err != nil {
		return Report{}, fmt.Errorf("acknowledge assignment through winning replica: %w", err)
	}
	if err := assertTerminalAgreement(ctx, client, replicas, assignmentID); err != nil {
		return Report{}, err
	}
	report.TerminalAgreement = true

	replicas[0].Close()
	replicas[0], err = startReplica(ctx, proxiedDSN, authenticator)
	if err != nil {
		return Report{}, fmt.Errorf("restart replica: %w", err)
	}
	if err := assertTerminalAgreement(ctx, client, replicas, assignmentID); err != nil {
		return Report{}, fmt.Errorf("read terminal assignment after replica restart: %w", err)
	}
	report.ReplicaRestartRecovered = true

	proxy.Pause()
	if err := waitForStatus(ctx, client, replicas, "/readyz", http.StatusServiceUnavailable); err != nil {
		return Report{}, fmt.Errorf("readiness during PostgreSQL outage: %w", err)
	}
	report.DependencyReadinessFailedClosed = true
	if err := waitForStatus(ctx, client, replicas, "/livez", http.StatusOK); err != nil {
		return Report{}, fmt.Errorf("liveness during PostgreSQL outage: %w", err)
	}
	report.LivenessMaintained = true
	_, outageErr := wireconformance.RequestData[api.AssignmentResource](
		ctx,
		client,
		replicas[0].URL(),
		matrixToken,
		"",
		http.MethodGet,
		"/v0alpha2/assignments/"+assignmentID,
		nil,
	)
	var responseErr *wireconformance.ResponseError
	if !errors.As(outageErr, &responseErr) ||
		responseErr.Status != http.StatusServiceUnavailable ||
		responseErr.Code != "Unavailable" ||
		!responseErr.Retryable {
		return Report{}, fmt.Errorf("dependency outage request did not fail with retryable Unavailable")
	}
	report.DependencyRequestFailedRetryable = true

	proxy.Resume()
	if err := waitForStatus(ctx, client, replicas, "/readyz", http.StatusOK); err != nil {
		return Report{}, fmt.Errorf("readiness after PostgreSQL recovery: %w", err)
	}
	if err := assertTerminalAgreement(ctx, client, replicas, assignmentID); err != nil {
		return Report{}, fmt.Errorf("terminal assignment after PostgreSQL recovery: %w", err)
	}
	report.DependencyRecoveryComplete = true
	return report, nil
}

func seedProposal(
	ctx context.Context,
	client *http.Client,
	baseURL string,
	runID string,
) (string, error) {
	now := time.Now().UTC().Add(-5 * time.Second)
	for index, skill := range []int{1490, 1510, 1495, 1505} {
		id := fmt.Sprintf("%s-ticket-%d", runID, index+1)
		ticket := api.MatchTicket{
			ID: id, Revision: 1, EnqueuedAt: now,
			Players: []api.Player{{
				ID: "player-" + id, Skill: skill, Role: "flex", LatencyMillis: 30,
			}},
		}
		if _, err := wireconformance.RequestData[api.MatchTicketMutation](
			ctx,
			client,
			baseURL,
			matrixToken,
			fmt.Sprintf("%s-put-ticket-%d", runID, index+1),
			http.MethodPut,
			"/v0alpha2/match-tickets/"+id,
			ticket,
		); err != nil {
			return "", fmt.Errorf("seed matrix ticket %d: %w", index+1, err)
		}
	}
	policyVersion := runID + "-policy"
	policy := api.MatchmakingPolicy{
		Version: policyVersion, TeamCount: 2, TeamSize: 2, MaxLatencyMillis: 100,
		MaxProposals: 1, MaxSearchNodes: 100_000,
		RelaxationSteps: []api.RelaxationStep{{AfterWaitMillis: 0, MaxTeamSkillGap: 100}},
	}
	if _, err := wireconformance.RequestData[api.PolicyMutation](
		ctx,
		client,
		baseURL,
		matrixToken,
		runID+"-put-policy",
		http.MethodPut,
		"/v0alpha2/policies/"+policyVersion,
		policy,
	); err != nil {
		return "", fmt.Errorf("seed matrix policy: %w", err)
	}
	planningRunID := runID + "-planning"
	planning, err := wireconformance.RequestData[api.PlanningRunMutation](
		ctx,
		client,
		baseURL,
		matrixToken,
		runID+"-plan",
		http.MethodPost,
		"/v0alpha2/planning-runs/"+planningRunID,
		api.PlanningRunRequest{PolicyVersion: policyVersion},
	)
	if err != nil {
		return "", fmt.Errorf("execute matrix planning run: %w", err)
	}
	if planning.Resource.Status != "completed" || planning.Resource.ProposalCount != 1 {
		return "", fmt.Errorf("matrix planning run did not produce one completed proposal")
	}
	proposals, err := wireconformance.RequestData[api.ProposalPage](
		ctx,
		client,
		baseURL,
		matrixToken,
		"",
		http.MethodGet,
		"/v0alpha2/planning-runs/"+planningRunID+"/proposals",
		nil,
	)
	if err != nil {
		return "", fmt.Errorf("read matrix proposal: %w", err)
	}
	if len(proposals.Items) != 1 {
		return "", fmt.Errorf("matrix planning result contains %d proposals; want 1", len(proposals.Items))
	}
	return proposals.Items[0].Proposal.ID, nil
}

func raceReservations(
	ctx context.Context,
	client *http.Client,
	baseURLs []string,
	runID string,
	proposalID string,
) (int, string, int, error) {
	type result struct {
		replica      int
		reservation  string
		mutation     api.ReservationMutation
		requestError error
	}
	start := make(chan struct{})
	results := make(chan result, len(baseURLs))
	for index, baseURL := range baseURLs {
		go func(replicaIndex int, endpoint string) {
			<-start
			reservationID := fmt.Sprintf("%s-reservation-%d", runID, replicaIndex+1)
			mutation, requestErr := wireconformance.RequestData[api.ReservationMutation](
				ctx,
				client,
				endpoint,
				matrixToken,
				fmt.Sprintf("%s-reserve-%d", runID, replicaIndex+1),
				http.MethodPost,
				"/v0alpha2/reservations/"+reservationID,
				api.ReservationRequest{ProposalID: proposalID},
			)
			results <- result{
				replica: replicaIndex, reservation: reservationID,
				mutation: mutation, requestError: requestErr,
			}
		}(index, baseURL)
	}
	close(start)

	winnerReplica := -1
	winnerReservation := ""
	conflicts := 0
	for range baseURLs {
		outcome := <-results
		if outcome.requestError == nil {
			if winnerReplica != -1 || outcome.mutation.Resource.Reservation.Status != "active" {
				return 0, "", 0, fmt.Errorf("reservation contention produced multiple or non-active winners")
			}
			winnerReplica = outcome.replica
			winnerReservation = outcome.reservation
			continue
		}
		var responseErr *wireconformance.ResponseError
		if !errors.As(outcome.requestError, &responseErr) ||
			responseErr.Status != http.StatusConflict ||
			responseErr.Code != "ReservationConflict" {
			return 0, "", 0, fmt.Errorf("reservation contention returned an unexpected loser outcome")
		}
		conflicts++
	}
	if winnerReplica == -1 || conflicts != len(baseURLs)-1 {
		return 0, "", 0, fmt.Errorf("reservation contention did not converge to one winner")
	}
	return winnerReplica, winnerReservation, conflicts, nil
}

func assertTerminalAgreement(
	ctx context.Context,
	client *http.Client,
	replicas []*replica,
	assignmentID string,
) error {
	var expectedVersion uint64
	for index, current := range replicas {
		resource, err := wireconformance.RequestData[api.AssignmentResource](
			ctx,
			client,
			current.URL(),
			matrixToken,
			"",
			http.MethodGet,
			"/v0alpha2/assignments/"+assignmentID,
			nil,
		)
		if err != nil {
			return fmt.Errorf("read terminal assignment from replica %d: %w", index+1, err)
		}
		if resource.Assignment.Status != "completed" ||
			resource.Assignment.Acknowledgment == nil ||
			resource.Assignment.Acknowledgment.Outcome != "completed" {
			return fmt.Errorf("replica %d returned a non-terminal assignment", index+1)
		}
		if index == 0 {
			expectedVersion = resource.StorageVersion
		} else if resource.StorageVersion != expectedVersion {
			return fmt.Errorf("replicas returned different terminal storage versions")
		}
	}
	return nil
}

type replica struct {
	store  *postgresrepository.Store
	server *httptest.Server
}

func startReplica(
	ctx context.Context,
	dsn string,
	authenticator targetapi.Authenticator,
) (*replica, error) {
	store, err := postgresrepository.Open(ctx, dsn)
	if err != nil {
		return nil, err
	}
	handler, err := targetruntime.New(store, authenticator, targetruntime.Options{
		CursorKey:        []byte("runtime-matrix-cursor-key-32-byte"),
		ReservationTTL:   time.Minute,
		MaxInFlight:      32,
		ReadinessTimeout: 250 * time.Millisecond,
		ReadinessCheck:   store.Ready,
	})
	if err != nil {
		store.Close()
		return nil, err
	}
	return &replica{store: store, server: httptest.NewServer(handler)}, nil
}

func (current *replica) URL() string {
	return current.server.URL
}

func (current *replica) Close() {
	if current == nil {
		return
	}
	if current.server != nil {
		current.server.Close()
		current.server = nil
	}
	if current.store != nil {
		current.store.Close()
		current.store = nil
	}
}

func matrixAuthenticator() targetapi.Authenticator {
	permissions := map[targetapi.Permission]bool{
		targetapi.PermissionMatchTicketsRead: true, targetapi.PermissionMatchTicketsWrite: true,
		targetapi.PermissionBackfillTicketsRead: true, targetapi.PermissionBackfillTicketsWrite: true,
		targetapi.PermissionPoliciesRead: true, targetapi.PermissionPoliciesWrite: true,
		targetapi.PermissionPlanningRunsRead: true, targetapi.PermissionPlanningRunsWrite: true,
		targetapi.PermissionReservationsRead: true, targetapi.PermissionReservationsWrite: true,
		targetapi.PermissionAssignmentsRead: true, targetapi.PermissionAssignmentsWrite: true,
	}
	return targetapi.AuthenticatorFunc(func(request *http.Request) (targetapi.Principal, error) {
		if request.Header.Get("Authorization") != "Bearer "+matrixToken {
			return targetapi.Principal{}, targetapi.ErrUnauthenticated
		}
		return targetapi.Principal{
			Subject: "runtime-matrix", Tenant: "runtime-matrix", Permissions: permissions,
		}, nil
	})
}

func waitForStatus(
	ctx context.Context,
	client *http.Client,
	replicas []*replica,
	path string,
	want int,
) error {
	ticker := time.NewTicker(25 * time.Millisecond)
	defer ticker.Stop()
	for {
		allMatched := true
		for _, current := range replicas {
			request, err := http.NewRequestWithContext(ctx, http.MethodGet, current.URL()+path, nil)
			if err != nil {
				return err
			}
			response, err := client.Do(request)
			if err != nil {
				allMatched = false
				continue
			}
			_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 64<<10))
			_ = response.Body.Close()
			if response.StatusCode != want {
				allMatched = false
			}
		}
		if allMatched {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func randomSuffix() (string, error) {
	raw := make([]byte, 8)
	if _, err := io.ReadFull(rand.Reader, raw); err != nil {
		return "", fmt.Errorf("generate runtime matrix identity: %w", err)
	}
	return hex.EncodeToString(raw), nil
}

func withSearchPath(dsn, schema string) (string, error) {
	parsed, err := url.Parse(dsn)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", fmt.Errorf("PostgreSQL test DSN must be an absolute URL")
	}
	query := parsed.Query()
	query.Set("search_path", schema)
	parsed.RawQuery = query.Encode()
	return parsed.String(), nil
}

type postgresProxy struct {
	listener net.Listener
	target   string

	mu      sync.Mutex
	enabled bool
	active  map[*proxyConnection]struct{}
}

type proxyConnection struct {
	client  net.Conn
	backend net.Conn
}

func newPostgresProxy(dsn string) (*postgresProxy, string, error) {
	parsed, err := url.Parse(dsn)
	if err != nil || parsed.Scheme == "" || parsed.Hostname() == "" {
		return nil, "", fmt.Errorf("parse PostgreSQL proxy target")
	}
	port := parsed.Port()
	if port == "" {
		port = "5432"
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, "", fmt.Errorf("listen for PostgreSQL outage proxy: %w", err)
	}
	proxy := &postgresProxy{
		listener: listener,
		target:   net.JoinHostPort(parsed.Hostname(), port),
		enabled:  true,
		active:   make(map[*proxyConnection]struct{}),
	}
	go proxy.accept()

	parsed.Host = listener.Addr().String()
	return proxy, parsed.String(), nil
}

func (proxy *postgresProxy) accept() {
	for {
		client, err := proxy.listener.Accept()
		if err != nil {
			return
		}
		proxy.mu.Lock()
		enabled := proxy.enabled
		proxy.mu.Unlock()
		if !enabled {
			_ = client.Close()
			continue
		}
		backend, err := net.DialTimeout("tcp", proxy.target, time.Second)
		if err != nil {
			_ = client.Close()
			continue
		}
		connection := &proxyConnection{client: client, backend: backend}
		proxy.mu.Lock()
		if !proxy.enabled {
			proxy.mu.Unlock()
			_ = client.Close()
			_ = backend.Close()
			continue
		}
		proxy.active[connection] = struct{}{}
		proxy.mu.Unlock()
		go proxy.bridge(connection)
	}
}

func (proxy *postgresProxy) bridge(connection *proxyConnection) {
	finished := make(chan struct{}, 2)
	copyDirection := func(destination, source net.Conn) {
		_, _ = io.Copy(destination, source)
		finished <- struct{}{}
	}
	go copyDirection(connection.backend, connection.client)
	go copyDirection(connection.client, connection.backend)
	<-finished
	_ = connection.client.Close()
	_ = connection.backend.Close()
	<-finished
	proxy.mu.Lock()
	delete(proxy.active, connection)
	proxy.mu.Unlock()
}

func (proxy *postgresProxy) Pause() {
	proxy.mu.Lock()
	proxy.enabled = false
	active := make([]*proxyConnection, 0, len(proxy.active))
	for connection := range proxy.active {
		active = append(active, connection)
	}
	proxy.mu.Unlock()
	for _, connection := range active {
		_ = connection.client.Close()
		_ = connection.backend.Close()
	}
}

func (proxy *postgresProxy) Resume() {
	proxy.mu.Lock()
	proxy.enabled = true
	proxy.mu.Unlock()
}

func (proxy *postgresProxy) Close() {
	_ = proxy.listener.Close()
	proxy.Pause()
}
