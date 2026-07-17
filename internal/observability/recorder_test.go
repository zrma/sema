package observability_test

import (
	"bufio"
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/zrma/sema/internal/observability"
)

func TestRecorderPropagatesTraceAndUsesRoutePattern(t *testing.T) {
	clock := &testClock{times: []time.Time{
		time.Date(2026, time.July, 17, 0, 0, 0, 0, time.UTC),
		time.Date(2026, time.July, 17, 0, 0, 0, int(2*time.Millisecond), time.UTC),
	}}
	var traces bytes.Buffer
	recorder := observability.New(&traces, clock.Now)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /items/{id}", func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("X-Sema-Error-Code", "StaleSnapshot")
		writer.WriteHeader(http.StatusConflict)
	})
	handler := recorder.Middleware(mux)
	request := httptest.NewRequest(http.MethodGet, "/items/private-ticket-id", nil)
	request.Header.Set("traceparent", "00-0123456789abcdef0123456789abcdef-0123456789abcdef-01")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	traceparent := response.Header().Get("traceparent")
	if !strings.HasPrefix(traceparent, "00-0123456789abcdef0123456789abcdef-") || len(traceparent) != 55 {
		t.Fatalf("response traceparent = %q", traceparent)
	}
	var span struct {
		TraceID      string  `json:"trace_id"`
		ParentSpanID string  `json:"parent_span_id"`
		Route        string  `json:"route"`
		FailureCode  string  `json:"failure_code"`
		DurationMS   float64 `json:"duration_ms"`
	}
	if err := json.Unmarshal(bytes.TrimSpace(traces.Bytes()), &span); err != nil {
		t.Fatal(err)
	}
	if span.TraceID != "0123456789abcdef0123456789abcdef" ||
		span.ParentSpanID != "0123456789abcdef" ||
		span.Route != "GET /items/{id}" || span.FailureCode != "StaleSnapshot" || span.DurationMS != 2 {
		t.Fatalf("trace span = %#v", span)
	}
	if strings.Contains(traces.String(), "private-ticket-id") {
		t.Fatalf("trace leaked resource identity: %s", traces.String())
	}

	metrics := httptest.NewRecorder()
	recorder.ServeMetrics(metrics, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	body := metrics.Body.String()
	for _, expected := range []string{
		`sema_http_requests_total{method="GET",route="GET /items/{id}",status="409",code="StaleSnapshot"} 1`,
		`sema_http_request_duration_seconds_bucket{method="GET",route="GET /items/{id}",status="409",code="StaleSnapshot",le="0.005"} 1`,
		`sema_http_request_duration_seconds_sum{method="GET",route="GET /items/{id}",status="409",code="StaleSnapshot"} 0.002000000`,
	} {
		if !strings.Contains(body, expected) {
			t.Fatalf("metrics omit %q:\n%s", expected, body)
		}
	}
	if strings.Contains(body, "private-ticket-id") {
		t.Fatalf("metrics leaked resource identity: %s", body)
	}
}

func TestRecorderReplacesInvalidTraceContext(t *testing.T) {
	clock := &testClock{times: []time.Time{time.Now(), time.Now()}}
	var traces bytes.Buffer
	recorder := observability.New(&traces, clock.Now)
	handler := recorder.Middleware(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusNoContent)
	}))
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.Header.Set("traceparent", "invalid")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	traceparent := response.Header().Get("traceparent")
	if !strings.HasPrefix(traceparent, "00-") || len(traceparent) != 55 {
		t.Fatalf("generated traceparent = %q", traceparent)
	}
	scanner := bufio.NewScanner(&traces)
	if !scanner.Scan() || strings.Contains(scanner.Text(), `"parent_span_id"`) {
		t.Fatalf("invalid parent was retained: %s", traces.String())
	}
}

func TestRecorderIsConcurrencySafe(t *testing.T) {
	recorder := observability.New(nil, time.Now)
	handler := recorder.Middleware(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusOK)
	}))
	const requests = 32
	var wait sync.WaitGroup
	for range requests {
		wait.Add(1)
		go func() {
			defer wait.Done()
			handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))
		}()
	}
	wait.Wait()
	metrics := httptest.NewRecorder()
	recorder.ServeMetrics(metrics, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if !strings.Contains(metrics.Body.String(), `status="200",code=""} 32`) {
		t.Fatalf("concurrent request count is wrong:\n%s", metrics.Body.String())
	}
}

type testClock struct {
	mu    sync.Mutex
	times []time.Time
}

func (clock *testClock) Now() time.Time {
	clock.mu.Lock()
	defer clock.mu.Unlock()
	value := clock.times[0]
	clock.times = clock.times[1:]
	return value
}
