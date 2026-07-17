//go:build darwin || linux

package operational

import (
	"context"
	"io"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/zrma/sema/internal/domain"
	"github.com/zrma/sema/internal/durable"
	"github.com/zrma/sema/internal/httpapi"
	"github.com/zrma/sema/internal/observability"
)

func TestRunExercisesServiceLifecycle(t *testing.T) {
	runtime, err := durable.Open(filepath.Join(t.TempDir(), "sema.journal"), 30*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := runtime.Close(); err != nil {
			t.Errorf("close runtime: %v", err)
		}
	})
	server := httptest.NewServer(httpapi.NewWithOptions(runtime, httpapi.Options{
		Observer: observability.New(io.Discard, time.Now),
	}))
	t.Cleanup(server.Close)

	report, err := Run(context.Background(), Config{
		BaseURL: server.URL, Cycles: 2, TicketsPerCycle: 20, Concurrency: 4,
	})
	if err != nil {
		t.Fatal(err)
	}
	if report.SchemaVersion != ReportSchema {
		t.Fatalf("schema = %q", report.SchemaVersion)
	}
	if report.Cycles != 2 || report.Tickets != 40 || report.Proposals != 4 || report.Assignments != 4 {
		t.Fatalf("unexpected aggregate report: %+v", report)
	}
	if report.Operations != 55 {
		t.Fatalf("operations = %d; want 55", report.Operations)
	}
	if report.AuditRecords != 56 {
		t.Fatalf("audit records = %d; want 56", report.AuditRecords)
	}
	if !report.MetricsVerified || report.DurationMillis <= 0 || report.Latency.MaxMillis <= 0 {
		t.Fatalf("measurement evidence is incomplete: %+v", report)
	}
	if len(report.AssignmentIDs) != 4 {
		t.Fatalf("assignment IDs = %d; want 4", len(report.AssignmentIDs))
	}
	for _, id := range report.AssignmentIDs {
		assignment, exists, err := runtime.Assignment(domain.AssignmentID(id))
		if err != nil {
			t.Fatal(err)
		}
		if !exists || assignment.Status != "completed" {
			t.Fatalf("assignment %q was not completed after the run", id)
		}
	}
}

func TestRunRejectsUnsafeBounds(t *testing.T) {
	tests := []struct {
		name   string
		config Config
		want   string
	}{
		{name: "base URL", config: Config{Cycles: 1, TicketsPerCycle: 10, Concurrency: 1}, want: "base URL"},
		{name: "cycles", config: Config{BaseURL: "http://example.test", TicketsPerCycle: 10, Concurrency: 1}, want: "cycles"},
		{name: "ticket multiple", config: Config{BaseURL: "http://example.test", Cycles: 1, TicketsPerCycle: 11, Concurrency: 1}, want: "tickets per cycle"},
		{name: "ticket bound", config: Config{BaseURL: "http://example.test", Cycles: 1, TicketsPerCycle: 260, Concurrency: 1}, want: "tickets per cycle"},
		{name: "concurrency", config: Config{BaseURL: "http://example.test", Cycles: 1, TicketsPerCycle: 10}, want: "concurrency"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := Run(context.Background(), test.config)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v; want substring %q", err, test.want)
			}
		})
	}
}
