// Package observability provides bounded-cardinality metrics and redacted HTTP traces.
package observability

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

var durationBuckets = []float64{0.001, 0.005, 0.01, 0.05, 0.1, 0.5, 1, 5}

type metricKey struct {
	method string
	route  string
	status int
	code   string
}

type metricValue struct {
	count   uint64
	buckets []uint64
	sum     float64
}

// Recorder owns in-process HTTP metrics and a redacted JSON trace sink.
type Recorder struct {
	mu      sync.Mutex
	metrics map[metricKey]*metricValue

	traceMu     sync.Mutex
	traceWriter io.Writer
	now         func() time.Time
}

type traceSpan struct {
	Timestamp    time.Time `json:"timestamp"`
	TraceID      string    `json:"trace_id"`
	SpanID       string    `json:"span_id"`
	ParentSpanID string    `json:"parent_span_id,omitempty"`
	Method       string    `json:"method"`
	Route        string    `json:"route"`
	Status       int       `json:"status"`
	FailureCode  string    `json:"failure_code,omitempty"`
	DurationMS   float64   `json:"duration_ms"`
}

func New(traceWriter io.Writer, now func() time.Time) *Recorder {
	if traceWriter == nil {
		traceWriter = io.Discard
	}
	if now == nil {
		now = time.Now
	}
	return &Recorder{metrics: make(map[metricKey]*metricValue), traceWriter: traceWriter, now: now}
}

func (recorder *Recorder) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		started := recorder.now()
		traceID, parentSpanID := incomingTrace(request.Header.Get("traceparent"))
		if traceID == "" {
			traceID = randomIdentifier(16)
		}
		spanID := randomIdentifier(8)
		writer.Header().Set("traceparent", "00-"+traceID+"-"+spanID+"-01")
		observed := &responseWriter{ResponseWriter: writer, status: http.StatusOK}
		next.ServeHTTP(observed, request)

		finished := recorder.now()
		duration := finished.Sub(started)
		if duration < 0 {
			duration = 0
		}
		route := request.Pattern
		if route == "" {
			route = "unmatched"
		}
		failureCode := observed.Header().Get("X-Sema-Error-Code")
		recorder.observe(metricKey{
			method: request.Method, route: route, status: observed.status, code: failureCode,
		}, duration)
		recorder.writeTrace(traceSpan{
			Timestamp: started.UTC(), TraceID: traceID, SpanID: spanID, ParentSpanID: parentSpanID,
			Method: request.Method, Route: route, Status: observed.status,
			FailureCode: failureCode, DurationMS: float64(duration) / float64(time.Millisecond),
		})
	})
}

func (recorder *Recorder) ServeMetrics(writer http.ResponseWriter, _ *http.Request) {
	writer.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	writer.Header().Set("Cache-Control", "no-store")
	_, _ = io.WriteString(writer, recorder.renderMetrics())
}

func (recorder *Recorder) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	recorder.ServeMetrics(writer, request)
}

func (recorder *Recorder) observe(key metricKey, duration time.Duration) {
	seconds := duration.Seconds()
	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	value := recorder.metrics[key]
	if value == nil {
		value = &metricValue{buckets: make([]uint64, len(durationBuckets))}
		recorder.metrics[key] = value
	}
	value.count++
	value.sum += seconds
	for index, boundary := range durationBuckets {
		if seconds <= boundary {
			value.buckets[index]++
		}
	}
}

func (recorder *Recorder) renderMetrics() string {
	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	keys := make([]metricKey, 0, len(recorder.metrics))
	for key := range recorder.metrics {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(left, right int) bool {
		return metricKeyString(keys[left]) < metricKeyString(keys[right])
	})
	var output strings.Builder
	output.WriteString("# HELP sema_http_requests_total HTTP requests handled by method, route, status, and failure code.\n")
	output.WriteString("# TYPE sema_http_requests_total counter\n")
	for _, key := range keys {
		value := recorder.metrics[key]
		fmt.Fprintf(&output, "sema_http_requests_total%s %d\n", labels(key, ""), value.count)
	}
	output.WriteString("# HELP sema_http_request_duration_seconds HTTP request duration.\n")
	output.WriteString("# TYPE sema_http_request_duration_seconds histogram\n")
	for _, key := range keys {
		value := recorder.metrics[key]
		for index, boundary := range durationBuckets {
			fmt.Fprintf(
				&output,
				"sema_http_request_duration_seconds_bucket%s %d\n",
				labels(key, strconv.FormatFloat(boundary, 'f', -1, 64)),
				value.buckets[index],
			)
		}
		fmt.Fprintf(
			&output,
			"sema_http_request_duration_seconds_bucket%s %d\n",
			labels(key, "+Inf"),
			value.count,
		)
		fmt.Fprintf(
			&output,
			"sema_http_request_duration_seconds_sum%s %.9f\n",
			labels(key, ""),
			value.sum,
		)
		fmt.Fprintf(
			&output,
			"sema_http_request_duration_seconds_count%s %d\n",
			labels(key, ""),
			value.count,
		)
	}
	return output.String()
}

func (recorder *Recorder) writeTrace(span traceSpan) {
	recorder.traceMu.Lock()
	defer recorder.traceMu.Unlock()
	_ = json.NewEncoder(recorder.traceWriter).Encode(span)
}

type responseWriter struct {
	http.ResponseWriter
	status      int
	wroteHeader bool
}

func (writer *responseWriter) WriteHeader(status int) {
	if writer.wroteHeader {
		return
	}
	writer.wroteHeader = true
	writer.status = status
	writer.ResponseWriter.WriteHeader(status)
}

func (writer *responseWriter) Write(contents []byte) (int, error) {
	if !writer.wroteHeader {
		writer.WriteHeader(http.StatusOK)
	}
	return writer.ResponseWriter.Write(contents)
}

func (writer *responseWriter) Unwrap() http.ResponseWriter {
	return writer.ResponseWriter
}

func incomingTrace(value string) (string, string) {
	parts := strings.Split(value, "-")
	if len(parts) != 4 || parts[0] != "00" || len(parts[1]) != 32 || len(parts[2]) != 16 || len(parts[3]) != 2 {
		return "", ""
	}
	if !validHex(parts[1]) || !validHex(parts[2]) || !validHex(parts[3]) || allZero(parts[1]) || allZero(parts[2]) {
		return "", ""
	}
	return strings.ToLower(parts[1]), strings.ToLower(parts[2])
}

func validHex(value string) bool {
	_, err := hex.DecodeString(value)
	return err == nil
}

func allZero(value string) bool {
	return strings.Trim(value, "0") == ""
}

var fallbackSequence atomic.Uint64

func randomIdentifier(byteCount int) string {
	value := make([]byte, byteCount)
	if _, err := rand.Read(value); err == nil {
		return hex.EncodeToString(value)
	}
	digest := sha256.Sum256([]byte(fmt.Sprintf("%d", fallbackSequence.Add(1))))
	return hex.EncodeToString(digest[:byteCount])
}

func labels(key metricKey, bucket string) string {
	values := []string{
		`method="` + escapeLabel(key.method) + `"`,
		`route="` + escapeLabel(key.route) + `"`,
		`status="` + strconv.Itoa(key.status) + `"`,
		`code="` + escapeLabel(key.code) + `"`,
	}
	if bucket != "" {
		values = append(values, `le="`+bucket+`"`)
	}
	return "{" + strings.Join(values, ",") + "}"
}

func escapeLabel(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	value = strings.ReplaceAll(value, "\n", `\n`)
	return strings.ReplaceAll(value, `"`, `\"`)
}

func metricKeyString(key metricKey) string {
	return key.method + "\x00" + key.route + "\x00" + strconv.Itoa(key.status) + "\x00" + key.code
}
