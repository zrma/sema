// Package operational executes bounded end-to-end service validation workloads.
package operational

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"sync"
	"time"

	api "github.com/zrma/sema/internal/api/v0alpha1"
)

const ReportSchema = "sema-ops-v1"

type Config struct {
	BaseURL         string
	Cycles          int
	TicketsPerCycle int
	Concurrency     int
	Client          *http.Client
	Now             func() time.Time
}

type LatencySummary struct {
	P50Millis float64 `json:"p50_millis"`
	P95Millis float64 `json:"p95_millis"`
	P99Millis float64 `json:"p99_millis"`
	MaxMillis float64 `json:"max_millis"`
}

type Report struct {
	SchemaVersion   string         `json:"schema_version"`
	Cycles          int            `json:"cycles"`
	Tickets         int            `json:"tickets"`
	Proposals       int            `json:"proposals"`
	Assignments     int            `json:"assignments"`
	Operations      int            `json:"operations"`
	AuditRecords    int            `json:"audit_records"`
	DurationMillis  float64        `json:"duration_millis"`
	Latency         LatencySummary `json:"latency"`
	MetricsVerified bool           `json:"metrics_verified"`
	Recovery        RecoveryReport `json:"recovery"`

	AssignmentIDs []string `json:"-"`
}

type RecoveryReport struct {
	RestartVerified   bool `json:"restart_verified"`
	TornTailRecovered bool `json:"torn_tail_recovered"`
}

func Run(ctx context.Context, configuration Config) (Report, error) {
	if err := validateConfig(configuration); err != nil {
		return Report{}, err
	}
	if configuration.Client == nil {
		configuration.Client = &http.Client{Timeout: 30 * time.Second}
	}
	if configuration.Now == nil {
		configuration.Now = time.Now
	}
	configuration.BaseURL = strings.TrimRight(configuration.BaseURL, "/")
	recorder := &latencyRecorder{}
	started := time.Now()
	proposalsPerCycle := configuration.TicketsPerCycle / 10
	policy := api.MatchmakingPolicy{
		Version: "ops-policy-v1", TeamCount: 2, TeamSize: 5, MaxLatencyMillis: 200,
		MaxProposals: proposalsPerCycle, MaxSearchNodes: 100_000,
		MaxCandidateTickets: 256, MaxCandidatesPerProposal: 64,
	}
	if _, err := request[api.PolicyRegistration](
		ctx, configuration.Client, recorder, http.MethodPut,
		configuration.BaseURL+"/v0alpha1/policies/"+policy.Version,
		policy,
	); err != nil {
		return Report{}, err
	}

	report := Report{SchemaVersion: ReportSchema, Cycles: configuration.Cycles}
	for cycle := range configuration.Cycles {
		now := configuration.Now().UTC()
		if err := runParallel(configuration.TicketsPerCycle, configuration.Concurrency, func(index int) error {
			ticketID := fmt.Sprintf("ops-c%04d-t%06d", cycle, index)
			ticket := api.MatchTicket{
				ID: ticketID, Revision: 1,
				EnqueuedAt: now.Add(-time.Duration(configuration.TicketsPerCycle-index) * time.Millisecond),
				Players: []api.Player{{
					ID: ticketID + "-player", Skill: 1000 + index%20, LatencyMillis: 20,
				}},
			}
			_, err := request[api.MutationResult](
				ctx, configuration.Client, recorder, http.MethodPut,
				configuration.BaseURL+"/v0alpha1/match-tickets/"+ticket.ID,
				ticket,
			)
			return err
		}); err != nil {
			return Report{}, fmt.Errorf("cycle %d ticket ingestion: %w", cycle, err)
		}
		report.Tickets += configuration.TicketsPerCycle

		batch, err := request[api.ProposalBatch](
			ctx, configuration.Client, recorder, http.MethodPost,
			configuration.BaseURL+"/v0alpha1/plans",
			api.PlanRequest{SnapshotID: fmt.Sprintf("ops-snapshot-%04d", cycle), PolicyVersion: policy.Version},
		)
		if err != nil {
			return Report{}, fmt.Errorf("cycle %d plan: %w", cycle, err)
		}
		if len(batch.Proposals) != proposalsPerCycle || len(batch.Unmatched) != 0 {
			return Report{}, fmt.Errorf(
				"cycle %d produced %d proposals and %d unmatched; want %d and 0",
				cycle,
				len(batch.Proposals),
				len(batch.Unmatched),
				proposalsPerCycle,
			)
		}
		report.Proposals += len(batch.Proposals)

		reservations := make([]api.Reservation, len(batch.Proposals))
		if err := runParallel(len(batch.Proposals), configuration.Concurrency, func(index int) error {
			reservationID := fmt.Sprintf("ops-reservation-%04d-%04d", cycle, index)
			reservation, err := request[api.Reservation](
				ctx, configuration.Client, recorder, http.MethodPost,
				configuration.BaseURL+"/v0alpha1/reservations/"+reservationID,
				api.ReserveRequest{ProposalID: batch.Proposals[index].ID},
			)
			if err == nil {
				reservations[index] = reservation
			}
			return err
		}); err != nil {
			return Report{}, fmt.Errorf("cycle %d reserve: %w", cycle, err)
		}

		assignments := make([]api.Assignment, len(reservations))
		if err := runParallel(len(reservations), configuration.Concurrency, func(index int) error {
			assignmentID := fmt.Sprintf("ops-assignment-%04d-%04d", cycle, index)
			assignment, err := request[api.Assignment](
				ctx, configuration.Client, recorder, http.MethodPost,
				configuration.BaseURL+"/v0alpha1/reservations/"+reservations[index].ID+"/confirm",
				api.ConfirmRequest{AssignmentID: assignmentID},
			)
			if err == nil {
				assignments[index] = assignment
			}
			return err
		}); err != nil {
			return Report{}, fmt.Errorf("cycle %d confirm: %w", cycle, err)
		}

		if err := runParallel(len(assignments), configuration.Concurrency, func(index int) error {
			assignment, err := request[api.Assignment](
				ctx, configuration.Client, recorder, http.MethodPost,
				configuration.BaseURL+"/v0alpha1/assignments/"+assignments[index].ID+"/acknowledgments",
				api.AcknowledgeAssignmentRequest{
					OperationID: assignments[index].ID + "-complete", Outcome: "completed",
				},
			)
			if err == nil && assignment.Status != "completed" {
				return fmt.Errorf("assignment status is %q", assignment.Status)
			}
			return err
		}); err != nil {
			return Report{}, fmt.Errorf("cycle %d acknowledge: %w", cycle, err)
		}
		report.Assignments += len(assignments)
		for _, assignment := range assignments {
			report.AssignmentIDs = append(report.AssignmentIDs, assignment.ID)
		}
	}

	auditRecords, err := readAllAudit(ctx, configuration.Client, configuration.BaseURL)
	if err != nil {
		return Report{}, err
	}
	report.AuditRecords = auditRecords
	metrics, err := readMetrics(ctx, configuration.Client, configuration.BaseURL)
	if err != nil {
		return Report{}, err
	}
	report.MetricsVerified = strings.Contains(metrics, "sema_http_requests_total") &&
		strings.Contains(metrics, `route="POST /v0alpha1/plans"`)
	if !report.MetricsVerified {
		return Report{}, fmt.Errorf("metrics scrape omitted service request counters")
	}
	report.Operations, report.Latency = recorder.Summary()
	report.DurationMillis = milliseconds(time.Since(started))
	return report, nil
}

func validateConfig(configuration Config) error {
	parsed, err := url.ParseRequestURI(configuration.BaseURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" ||
		(parsed.Scheme != "http" && parsed.Scheme != "https") ||
		(parsed.Path != "" && parsed.Path != "/") || parsed.RawQuery != "" || parsed.Fragment != "" {
		return fmt.Errorf("valid base URL is required")
	}
	if configuration.Cycles <= 0 || configuration.Cycles > 10_000 {
		return fmt.Errorf("cycles must be between 1 and 10000")
	}
	if configuration.TicketsPerCycle <= 0 || configuration.TicketsPerCycle > 250 || configuration.TicketsPerCycle%10 != 0 {
		return fmt.Errorf("tickets per cycle must be a positive multiple of 10 and at most 250")
	}
	if configuration.Concurrency <= 0 || configuration.Concurrency > 256 {
		return fmt.Errorf("concurrency must be between 1 and 256")
	}
	return nil
}

func request[T any](
	ctx context.Context,
	client *http.Client,
	recorder *latencyRecorder,
	method string,
	endpoint string,
	body any,
) (T, error) {
	var zero T
	var encoded io.Reader
	if body != nil {
		contents, err := json.Marshal(body)
		if err != nil {
			return zero, err
		}
		encoded = bytes.NewReader(contents)
	}
	request, err := http.NewRequestWithContext(ctx, method, endpoint, encoded)
	if err != nil {
		return zero, err
	}
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	started := time.Now()
	response, err := client.Do(request)
	recorder.Record(time.Since(started))
	if err != nil {
		return zero, err
	}
	defer response.Body.Close()
	contents, err := io.ReadAll(response.Body)
	if err != nil {
		return zero, err
	}
	var envelope struct {
		APIVersion string          `json:"api_version"`
		Data       json.RawMessage `json:"data"`
		Error      *api.Failure    `json:"error"`
	}
	if err := json.Unmarshal(contents, &envelope); err != nil {
		return zero, fmt.Errorf("decode response: %w", err)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 || envelope.Error != nil {
		if envelope.Error != nil {
			return zero, fmt.Errorf("service %s: %s", envelope.Error.Code, envelope.Error.Message)
		}
		return zero, fmt.Errorf("service status %d", response.StatusCode)
	}
	if envelope.APIVersion != api.Version {
		return zero, fmt.Errorf("service API version is %q", envelope.APIVersion)
	}
	var data T
	if err := json.Unmarshal(envelope.Data, &data); err != nil {
		return zero, fmt.Errorf("decode response data: %w", err)
	}
	return data, nil
}

func readAllAudit(ctx context.Context, client *http.Client, baseURL string) (int, error) {
	count := 0
	var after uint64
	for {
		page, err := request[api.AuditPage](
			ctx,
			client,
			&latencyRecorder{},
			http.MethodGet,
			fmt.Sprintf("%s/v0alpha1/audit?after=%d&limit=1000", baseURL, after),
			nil,
		)
		if err != nil {
			return 0, fmt.Errorf("read audit: %w", err)
		}
		count += len(page.Records)
		if len(page.Records) < 1000 {
			return count, nil
		}
		if page.NextSequence <= after {
			return 0, fmt.Errorf("audit pagination did not advance")
		}
		after = page.NextSequence
	}
}

func readMetrics(ctx context.Context, client *http.Client, baseURL string) (string, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/metrics", nil)
	if err != nil {
		return "", err
	}
	response, err := client.Do(request)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	contents, err := io.ReadAll(response.Body)
	if err != nil {
		return "", err
	}
	if response.StatusCode != http.StatusOK {
		return "", fmt.Errorf("metrics status %d", response.StatusCode)
	}
	return string(contents), nil
}

func runParallel(count, concurrency int, operation func(int) error) error {
	semaphore := make(chan struct{}, concurrency)
	errors := make(chan error, count)
	var wait sync.WaitGroup
	for index := range count {
		wait.Add(1)
		go func() {
			defer wait.Done()
			semaphore <- struct{}{}
			defer func() { <-semaphore }()
			if err := operation(index); err != nil {
				errors <- err
			}
		}()
	}
	wait.Wait()
	close(errors)
	for err := range errors {
		return err
	}
	return nil
}

type latencyRecorder struct {
	mu        sync.Mutex
	durations []time.Duration
}

func (recorder *latencyRecorder) Record(duration time.Duration) {
	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	recorder.durations = append(recorder.durations, duration)
}

func (recorder *latencyRecorder) Summary() (int, LatencySummary) {
	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	durations := slices.Clone(recorder.durations)
	slices.Sort(durations)
	if len(durations) == 0 {
		return 0, LatencySummary{}
	}
	return len(durations), LatencySummary{
		P50Millis: milliseconds(percentile(durations, 0.50)),
		P95Millis: milliseconds(percentile(durations, 0.95)),
		P99Millis: milliseconds(percentile(durations, 0.99)),
		MaxMillis: milliseconds(durations[len(durations)-1]),
	}
}

func percentile(sorted []time.Duration, quantile float64) time.Duration {
	index := int(math.Ceil(quantile*float64(len(sorted)))) - 1
	if index < 0 {
		index = 0
	}
	return sorted[index]
}

func milliseconds(duration time.Duration) float64 {
	return float64(duration) / float64(time.Millisecond)
}
