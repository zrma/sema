//go:build darwin || linux

package httpapi_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	api "github.com/zrma/sema/internal/api/v0alpha1"
	"github.com/zrma/sema/internal/durable"
	"github.com/zrma/sema/internal/httpapi"
	"github.com/zrma/sema/internal/observability"
)

var fixtureNow = time.Date(2026, time.July, 17, 12, 0, 0, 0, time.UTC)

func TestHTTPAPIFullLifecycleSurvivesRestart(t *testing.T) {
	journalPath := filepath.Join(t.TempDir(), "sema.journal")
	runtime := openRuntime(t, journalPath)
	handler := httpapi.NewWithClock(runtime, func() time.Time { return fixtureNow })
	policy := api.MatchmakingPolicy{
		Version: "http-v1", TeamCount: 2, TeamSize: 2, MaxLatencyMillis: 200,
		MaxSearchNodes: 100_000, MaxCandidatesPerProposal: 64,
		RelaxationSteps: []api.RelaxationStep{{AfterWaitMillis: 0, MaxTeamSkillGap: 100}},
	}
	registration := requestData[api.PolicyRegistration](t, handler, http.MethodPut, "/v0alpha1/policies/http-v1", policy, http.StatusOK)
	if registration.Fingerprint == "" || registration.Policy.Version != policy.Version {
		t.Fatalf("policy registration = %#v", registration)
	}
	stored := requestData[api.PolicyRegistration](t, handler, http.MethodGet, "/v0alpha1/policies/http-v1", nil, http.StatusOK)
	if stored.Fingerprint != registration.Fingerprint || stored.Policy.RelaxationSteps[0].AfterWaitMillis != 0 {
		t.Fatalf("stored policy = %#v", stored)
	}
	for index := range 4 {
		ticket := api.MatchTicket{
			ID: fmt.Sprintf("ticket-%d", index), Revision: 1,
			EnqueuedAt: fixtureNow.Add(-time.Duration(4-index) * time.Second),
			Players: []api.Player{{
				ID: fmt.Sprintf("player-%d", index), Skill: 1000 + index, LatencyMillis: 20,
			}},
		}
		result := requestData[api.MutationResult](
			t, handler, http.MethodPut, "/v0alpha1/match-tickets/"+ticket.ID, ticket, http.StatusOK,
		)
		if result.Status != "accepted" {
			t.Fatalf("ticket result = %#v", result)
		}
	}
	batch := requestData[api.ProposalBatch](t, handler, http.MethodPost, "/v0alpha1/plans", api.PlanRequest{
		SnapshotID: "snapshot-http", PolicyVersion: policy.Version,
	}, http.StatusOK)
	if len(batch.Proposals) != 1 || len(batch.Unmatched) != 0 {
		t.Fatalf("plan batch = %#v", batch)
	}
	if err := runtime.Close(); err != nil {
		t.Fatal(err)
	}
	runtime = openRuntime(t, journalPath)
	handler = httpapi.NewWithClock(runtime, func() time.Time { return fixtureNow.Add(10 * time.Second) })
	retriedBatch := requestData[api.ProposalBatch](t, handler, http.MethodPost, "/v0alpha1/plans", api.PlanRequest{
		SnapshotID: "snapshot-http", PolicyVersion: policy.Version,
	}, http.StatusOK)
	if !reflect.DeepEqual(retriedBatch, batch) {
		t.Fatalf("durable plan retry changed batch: first=%#v retry=%#v", batch, retriedBatch)
	}
	reservation := requestData[api.Reservation](
		t,
		handler,
		http.MethodPost,
		"/v0alpha1/reservations/reservation-http",
		api.ReserveRequest{ProposalID: batch.Proposals[0].ID},
		http.StatusOK,
	)
	assignment := requestData[api.Assignment](
		t,
		handler,
		http.MethodPost,
		"/v0alpha1/reservations/"+reservation.ID+"/confirm",
		api.ConfirmRequest{AssignmentID: "assignment-http"},
		http.StatusOK,
	)
	if assignment.Status != "pending" {
		t.Fatalf("confirmed assignment = %#v", assignment)
	}
	polled := requestData[api.Assignment](
		t, handler, http.MethodGet, "/v0alpha1/assignments/"+assignment.ID, nil, http.StatusOK,
	)
	if polled.ID != assignment.ID || polled.Status != "pending" {
		t.Fatalf("polled assignment = %#v", polled)
	}
	if err := runtime.Close(); err != nil {
		t.Fatal(err)
	}

	runtime = openRuntime(t, journalPath)
	handler = httpapi.NewWithClock(runtime, func() time.Time { return fixtureNow.Add(2 * time.Second) })
	completed := requestData[api.Assignment](
		t,
		handler,
		http.MethodPost,
		"/v0alpha1/assignments/"+assignment.ID+"/acknowledgments",
		api.AcknowledgeAssignmentRequest{
			OperationID: "operation-http", Outcome: "completed",
		},
		http.StatusOK,
	)
	if completed.Status != "completed" || completed.Acknowledgment == nil {
		t.Fatalf("completed assignment = %#v", completed)
	}
	retried := requestData[api.Assignment](
		t,
		handler,
		http.MethodPost,
		"/v0alpha1/assignments/"+assignment.ID+"/acknowledgments",
		api.AcknowledgeAssignmentRequest{
			OperationID: "operation-http", Outcome: "completed",
		},
		http.StatusOK,
	)
	if retried.Acknowledgment.AcknowledgedAt != completed.Acknowledgment.AcknowledgedAt {
		t.Fatalf("retry changed acknowledged_at: first=%s retry=%s", completed.Acknowledgment.AcknowledgedAt, retried.Acknowledgment.AcknowledgedAt)
	}
	if err := runtime.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestHTTPAPIBackfillIngestionAndCancellation(t *testing.T) {
	runtime := openRuntime(t, filepath.Join(t.TempDir(), "sema.journal"))
	defer runtime.Close()
	handler := httpapi.NewWithClock(runtime, func() time.Time { return fixtureNow })
	ticket := api.BackfillTicket{
		ID: "backfill-http", Revision: 1, SessionID: "session-http", RosterVersion: 7,
		OpenSlotsByTeam: []int{1, 1}, EnqueuedAt: fixtureNow,
	}
	requestData[api.MutationResult](
		t, handler, http.MethodPut, "/v0alpha1/backfill-tickets/"+ticket.ID, ticket, http.StatusOK,
	)
	requestData[api.MutationResult](
		t,
		handler,
		http.MethodDelete,
		"/v0alpha1/backfill-tickets/"+ticket.ID+"?revision=1&roster_version=7",
		nil,
		http.StatusOK,
	)
}

func TestHTTPAPIRejectsMalformedAndConflictingRequests(t *testing.T) {
	runtime := openRuntime(t, filepath.Join(t.TempDir(), "sema.journal"))
	defer runtime.Close()
	handler := httpapi.NewWithClock(runtime, func() time.Time { return fixtureNow })

	tests := []struct {
		name   string
		method string
		path   string
		body   string
		status int
		code   string
	}{
		{name: "unknown field", method: http.MethodPut, path: "/v0alpha1/policies/policy", body: `{"version":"policy","unknown":true}`, status: 400, code: "InvalidInput"},
		{name: "multiple values", method: http.MethodPost, path: "/v0alpha1/plans", body: `{}` + `{}`, status: 400, code: "InvalidInput"},
		{name: "path mismatch", method: http.MethodPut, path: "/v0alpha1/match-tickets/path", body: `{"id":"body"}`, status: 400, code: "InvalidInput"},
		{name: "invalid revision", method: http.MethodDelete, path: "/v0alpha1/match-tickets/ticket?revision=0", status: 400, code: "InvalidInput"},
		{name: "missing assignment", method: http.MethodGet, path: "/v0alpha1/assignments/missing", status: 404, code: "NotFound"},
		{name: "unknown proposal", method: http.MethodPost, path: "/v0alpha1/reservations/reservation", body: `{"proposal_id":"missing"}`, status: 404, code: "NotFound"},
		{name: "forged proposal body", method: http.MethodPost, path: "/v0alpha1/reservations/reservation", body: `{"proposal":{}}`, status: 400, code: "InvalidInput"},
		{name: "unknown endpoint", method: http.MethodGet, path: "/v0alpha1/unknown", status: 404, code: "NotFound"},
		{name: "wrong method", method: http.MethodPatch, path: "/v0alpha1/plans", status: 405, code: "MethodNotAllowed"},
		{name: "invalid audit limit", method: http.MethodGet, path: "/v0alpha1/audit?limit=0", status: 400, code: "InvalidInput"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(test.method, test.path, strings.NewReader(test.body))
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			if response.Code != test.status {
				t.Fatalf("status = %d, body=%s; want %d", response.Code, response.Body.String(), test.status)
			}
			var envelope api.Envelope
			if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
				t.Fatal(err)
			}
			if envelope.APIVersion != api.Version || envelope.Error == nil || envelope.Error.Code != test.code {
				t.Fatalf("error envelope = %#v", envelope)
			}
			if response.Header().Get("X-Sema-Error-Code") != test.code {
				t.Fatalf("error code header = %q; want %q", response.Header().Get("X-Sema-Error-Code"), test.code)
			}
		})
	}

	request := httptest.NewRequest(http.MethodPost, "/v0alpha1/plans", strings.NewReader(strings.Repeat(" ", (1<<20)+1)))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("oversized request status = %d; want 400", response.Code)
	}
}

func TestOperationalEndpointsExposeRedactedAuditMetricsAndTrace(t *testing.T) {
	runtime := openRuntime(t, filepath.Join(t.TempDir(), "sema.journal"))
	defer runtime.Close()
	var traces bytes.Buffer
	observer := observability.New(&traces, func() time.Time { return fixtureNow })
	handler := httpapi.NewWithOptions(runtime, httpapi.Options{
		Now: func() time.Time { return fixtureNow }, Observer: observer,
	})
	requestData[api.MutationResult](t, handler, http.MethodGet, "/livez", nil, http.StatusOK)
	requestData[api.MutationResult](t, handler, http.MethodGet, "/readyz", nil, http.StatusOK)
	ticket := api.MatchTicket{
		ID: "private-ticket-id", Revision: 1, EnqueuedAt: fixtureNow,
		Players: []api.Player{{ID: "private-player-id", Skill: 1000, LatencyMillis: 20}},
	}
	requestData[api.MutationResult](
		t, handler, http.MethodPut, "/v0alpha1/match-tickets/"+ticket.ID, ticket, http.StatusOK,
	)
	audit := requestData[api.AuditPage](t, handler, http.MethodGet, "/v0alpha1/audit?after=0&limit=100", nil, http.StatusOK)
	if len(audit.Records) != 2 || audit.Records[1].Counts["players"] != 1 || audit.NextSequence != 2 {
		t.Fatalf("audit page = %#v", audit)
	}
	encodedAudit, err := json.Marshal(audit)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(encodedAudit, []byte(ticket.ID)) || bytes.Contains(encodedAudit, []byte(ticket.Players[0].ID)) {
		t.Fatalf("audit leaked resource identities: %s", encodedAudit)
	}

	request := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	request.Header.Set("traceparent", "00-0123456789abcdef0123456789abcdef-0123456789abcdef-01")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK || !strings.HasPrefix(
		response.Header().Get("traceparent"),
		"00-0123456789abcdef0123456789abcdef-",
	) {
		t.Fatalf("ready trace response = %d, %q", response.Code, response.Header().Get("traceparent"))
	}

	metricsResponse := httptest.NewRecorder()
	handler.ServeHTTP(metricsResponse, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	metrics := metricsResponse.Body.String()
	if metricsResponse.Code != http.StatusOK || !strings.Contains(metrics, `route="GET /readyz"`) {
		t.Fatalf("metrics response = %d:\n%s", metricsResponse.Code, metrics)
	}
	if strings.Contains(metrics, ticket.ID) || strings.Contains(traces.String(), ticket.ID) {
		t.Fatalf("operational output leaked resource identity: metrics=%s traces=%s", metrics, traces.String())
	}
}

func requestData[T any](
	t *testing.T,
	handler http.Handler,
	method string,
	path string,
	body any,
	wantStatus int,
) T {
	t.Helper()
	var encoded []byte
	if body != nil {
		var err error
		encoded, err = json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
	}
	request := httptest.NewRequest(method, path, bytes.NewReader(encoded))
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != wantStatus {
		t.Fatalf("%s %s status = %d, body=%s; want %d", method, path, response.Code, response.Body.String(), wantStatus)
	}
	if response.Header().Get("Content-Type") != "application/json" || response.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("response headers = %#v", response.Header())
	}
	if len(response.Header().Get("traceparent")) != 55 {
		t.Fatalf("response traceparent = %q", response.Header().Get("traceparent"))
	}
	var wire struct {
		APIVersion string          `json:"api_version"`
		Data       json.RawMessage `json:"data"`
		Error      *api.Failure    `json:"error"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &wire); err != nil {
		t.Fatal(err)
	}
	if wire.APIVersion != api.Version || wire.Error != nil {
		t.Fatalf("response envelope = %#v", wire)
	}
	var data T
	if err := json.Unmarshal(wire.Data, &data); err != nil {
		t.Fatal(err)
	}
	return data
}

func openRuntime(t *testing.T, path string) *durable.Runtime {
	t.Helper()
	runtime, err := durable.Open(path, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	return runtime
}
