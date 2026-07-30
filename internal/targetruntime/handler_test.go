package targetruntime

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	api "github.com/zrma/sema/internal/api/v0alpha2"
	"github.com/zrma/sema/internal/observability"
	"github.com/zrma/sema/internal/repository"
	"github.com/zrma/sema/internal/targetapi"
)

func TestHandlerExposesHealthWithoutWeakeningTargetAuthentication(t *testing.T) {
	handler := newTestHandler(t, repository.NewMemory())

	for _, path := range []string{"/livez", "/readyz"} {
		request := httptest.NewRequest(http.MethodGet, "https://sema.example"+path, nil)
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusOK {
			t.Fatalf("GET %s status = %d, body = %s", path, recorder.Code, recorder.Body.String())
		}
	}

	for path, expectedVersion := range map[string]string{
		"/v0alpha2/match-tickets": api.Version,
		"/v1/match-tickets":       targetapi.StableAPIVersion,
	} {
		request := httptest.NewRequest(http.MethodGet, "https://sema.example"+path, nil)
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, request)
		var envelope api.Envelope
		if err := json.Unmarshal(recorder.Body.Bytes(), &envelope); err != nil {
			t.Fatal(err)
		}
		if recorder.Code != http.StatusUnauthorized ||
			envelope.APIVersion != expectedVersion ||
			envelope.Error == nil ||
			envelope.Error.Code != "Unauthenticated" {
			t.Fatalf(
				"target API path=%s status=%d envelope=%#v",
				path,
				recorder.Code,
				envelope,
			)
		}
	}
}

func TestReadinessFailsClosedWithoutRepositoryDetails(t *testing.T) {
	handler, err := New(repository.NewMemory(), targetapi.AuthenticatorFunc(func(*http.Request) (targetapi.Principal, error) {
		return targetapi.Principal{}, targetapi.ErrUnauthenticated
	}), Options{
		CursorKey: make([]byte, 32), ReservationTTL: time.Minute,
		MaxInFlight: 2, RequestTimeout: time.Second, ReadinessTimeout: time.Second,
		ReadinessCheck: func(context.Context) error {
			return errors.New("private database endpoint is unavailable")
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodGet, "https://sema.example/readyz", nil)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusServiceUnavailable || recorder.Body.String() != "{\"status\":\"unavailable\"}\n" {
		t.Fatalf("readiness status = %d, body = %q", recorder.Code, recorder.Body.String())
	}
}

func TestAdmissionRejectsExcessWithoutBlockingHealth(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})
	var once sync.Once
	next := http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		once.Do(func() { close(entered) })
		<-release
	})
	handler := bounded(next, 1)

	finished := make(chan struct{})
	go func() {
		handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "https://sema.example", nil))
		close(finished)
	}()
	<-entered

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "https://sema.example", nil))
	if recorder.Code != http.StatusServiceUnavailable ||
		recorder.Header().Get("Retry-After") != "1" ||
		recorder.Header().Get("X-Sema-Error-Code") != "ResourceExhausted" {
		t.Fatalf(
			"overload status = %d, retry-after = %q, code = %q",
			recorder.Code,
			recorder.Header().Get("Retry-After"),
			recorder.Header().Get("X-Sema-Error-Code"),
		)
	}
	close(release)
	<-finished
}

func TestAdmissionEnvelopeUsesBoundStableVersion(t *testing.T) {
	recorder := &stableResponseRecorder{ResponseRecorder: httptest.NewRecorder()}
	writeEnvelope(recorder, http.StatusServiceUnavailable, api.Envelope{
		APIVersion: api.Version,
		Error: &api.Failure{
			Code: "ResourceExhausted", Message: "capacity is exhausted", Retryable: true,
		},
	})
	var envelope api.Envelope
	if err := json.Unmarshal(recorder.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.APIVersion != targetapi.StableAPIVersion {
		t.Fatalf("API version = %q; want %q", envelope.APIVersion, targetapi.StableAPIVersion)
	}
}

type stableResponseRecorder struct {
	*httptest.ResponseRecorder
}

func (*stableResponseRecorder) SemaAPIVersion() string {
	return targetapi.StableAPIVersion
}

func TestSharedAdmissionMetricsRetainRejectedEndpointPattern(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})
	var once sync.Once
	blocking := http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		once.Do(func() { close(entered) })
		<-release
		writer.WriteHeader(http.StatusNoContent)
	})
	var traces bytes.Buffer
	observer := observability.New(&traces, time.Now)
	admit := newAdmissionMiddleware(1)
	mux := http.NewServeMux()
	mux.Handle(
		"GET /v0alpha2/match-tickets/{ticket_id}",
		observer.Middleware(admit(blocking)),
	)
	mux.Handle(
		"GET /v0alpha2/policies/{version}",
		observer.Middleware(admit(blocking)),
	)

	finished := make(chan struct{})
	go func() {
		mux.ServeHTTP(
			httptest.NewRecorder(),
			httptest.NewRequest(http.MethodGet, "https://sema.example/v0alpha2/match-tickets/ticket-a", nil),
		)
		close(finished)
	}()
	<-entered

	rejected := httptest.NewRecorder()
	mux.ServeHTTP(
		rejected,
		httptest.NewRequest(http.MethodGet, "https://sema.example/v0alpha2/policies/policy-a", nil),
	)
	if rejected.Code != http.StatusServiceUnavailable {
		t.Fatalf("rejected status = %d", rejected.Code)
	}
	metrics := httptest.NewRecorder()
	observer.ServeMetrics(metrics, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if !strings.Contains(
		metrics.Body.String(),
		`route="GET /v0alpha2/policies/{version}",status="503",code="ResourceExhausted"`,
	) {
		t.Fatalf("admission metric lost endpoint pattern: %s", metrics.Body.String())
	}

	close(release)
	<-finished
}

func TestRequestTimeoutCancelsTargetOperation(t *testing.T) {
	cancelled := make(chan struct{})
	next := http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		<-request.Context().Done()
		close(cancelled)
		writer.WriteHeader(http.StatusServiceUnavailable)
	})
	handler := withTimeout(next, 10*time.Millisecond)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "https://sema.example", nil))
	select {
	case <-cancelled:
	case <-time.After(time.Second):
		t.Fatal("request context was not cancelled")
	}
	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("timeout status = %d", recorder.Code)
	}
}

func TestMetricsAndTracesUseBoundedRouteWithoutResourceIdentity(t *testing.T) {
	var traces bytes.Buffer
	observer := observability.New(&traces, time.Now)
	handler, err := New(
		repository.NewMemory(),
		targetapi.AuthenticatorFunc(func(request *http.Request) (targetapi.Principal, error) {
			if request.Header.Get("Authorization") != "Bearer redacted-test-token" {
				return targetapi.Principal{}, targetapi.ErrUnauthenticated
			}
			return targetapi.Principal{
				Subject: "subject-private", Tenant: "tenant-private",
				Permissions: map[targetapi.Permission]bool{
					targetapi.PermissionMatchTicketsRead: true,
				},
			}, nil
		}),
		Options{
			CursorKey: make([]byte, 32), ReservationTTL: time.Minute,
			MaxInFlight: 2, RequestTimeout: time.Second, ReadinessTimeout: time.Second,
			ReadinessCheck: func(context.Context) error { return nil },
			Observer:       observer,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	const resourceID = "ticket-private-identity"
	request := httptest.NewRequest(
		http.MethodGet,
		"https://sema.example/v0alpha2/match-tickets/"+resourceID,
		nil,
	)
	request.Header.Set("Authorization", "Bearer redacted-test-token")
	request.Header.Set(
		"traceparent",
		"00-0123456789abcdef0123456789abcdef-0123456789abcdef-01",
	)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusNotFound ||
		response.Header().Get("traceparent") == "" {
		t.Fatalf("response status=%d traceparent=%q", response.Code, response.Header().Get("traceparent"))
	}

	metrics := httptest.NewRecorder()
	handler.ServeHTTP(
		metrics,
		httptest.NewRequest(http.MethodGet, "https://sema.example/metrics", nil),
	)
	metricBody := metrics.Body.String()
	if metrics.Code != http.StatusOK ||
		!strings.Contains(metricBody, `route="GET /v0alpha2/match-tickets/{ticket_id}"`) ||
		!strings.Contains(metricBody, `status="404",code="NotFound"`) {
		t.Fatalf("metrics status=%d body=%s", metrics.Code, metricBody)
	}
	combined := metricBody + traces.String()
	for _, forbidden := range []string{
		resourceID, "redacted-test-token", "tenant-private", "subject-private",
	} {
		if strings.Contains(combined, forbidden) {
			t.Fatalf("observability output exposed %q: %s", forbidden, combined)
		}
	}
	var span struct {
		TraceID     string `json:"trace_id"`
		ParentSpan  string `json:"parent_span_id"`
		Route       string `json:"route"`
		Status      int    `json:"status"`
		FailureCode string `json:"failure_code"`
	}
	decoder := json.NewDecoder(&traces)
	if err := decoder.Decode(&span); err != nil {
		t.Fatal(err)
	}
	if span.TraceID != "0123456789abcdef0123456789abcdef" ||
		span.ParentSpan != "0123456789abcdef" ||
		span.Route != "GET /v0alpha2/match-tickets/{ticket_id}" ||
		span.Status != http.StatusNotFound ||
		span.FailureCode != "NotFound" {
		t.Fatalf("span = %#v", span)
	}
}

func TestNewRejectsUnsafeOptions(t *testing.T) {
	owner := repository.NewMemory()
	authenticator := targetapi.AuthenticatorFunc(func(*http.Request) (targetapi.Principal, error) {
		return targetapi.Principal{}, targetapi.ErrUnauthenticated
	})
	for name, options := range map[string]Options{
		"missing cursor key": {
			ReservationTTL: time.Minute, MaxInFlight: 1, RequestTimeout: time.Second, ReadinessTimeout: time.Second,
			ReadinessCheck: func(context.Context) error { return nil },
		},
		"zero admission": {
			CursorKey: make([]byte, 32), ReservationTTL: time.Minute, RequestTimeout: time.Second, ReadinessTimeout: time.Second,
			ReadinessCheck: func(context.Context) error { return nil },
		},
		"zero request timeout": {
			CursorKey: make([]byte, 32), ReservationTTL: time.Minute,
			MaxInFlight: 1, ReadinessTimeout: time.Second,
			ReadinessCheck: func(context.Context) error { return nil },
		},
		"zero readiness": {
			CursorKey: make([]byte, 32), ReservationTTL: time.Minute, MaxInFlight: 1, RequestTimeout: time.Second,
			ReadinessCheck: func(context.Context) error { return nil },
		},
		"missing readiness check": {
			CursorKey: make([]byte, 32), ReservationTTL: time.Minute,
			MaxInFlight: 1, RequestTimeout: time.Second, ReadinessTimeout: time.Second,
		},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := New(owner, authenticator, options); err == nil {
				t.Fatal("unsafe runtime options were accepted")
			}
		})
	}
}

func newTestHandler(t *testing.T, owner repository.Repository) http.Handler {
	t.Helper()
	handler, err := New(owner, targetapi.AuthenticatorFunc(func(*http.Request) (targetapi.Principal, error) {
		return targetapi.Principal{}, targetapi.ErrUnauthenticated
	}), Options{
		CursorKey: make([]byte, 32), ReservationTTL: time.Minute,
		MaxInFlight: 2, RequestTimeout: time.Second, ReadinessTimeout: time.Second,
		ReadinessCheck: func(context.Context) error { return nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	return handler
}
