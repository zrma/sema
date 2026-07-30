package serviceworkload

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"
)

const postgresTestDSNEnvironment = "SEMA_POSTGRES_TEST_DSN"

func TestRunRejectsUnsafeOptionsWithoutDatabaseAccess(t *testing.T) {
	options := DefaultOptions()
	for name, mutate := range map[string]func(*Options){
		"missing DSN": func(*Options) {},
		"zero runs": func(options *Options) {
			options.PostgresDSN = "postgres://example.invalid/sema"
			options.Runs = 0
		},
		"unaligned tickets": func(options *Options) {
			options.PostgresDSN = "postgres://example.invalid/sema"
			options.TicketsPerCycle = 11
		},
		"zero timeout": func(options *Options) {
			options.PostgresDSN = "postgres://example.invalid/sema"
			options.RequestTimeout = 0
		},
	} {
		t.Run(name, func(t *testing.T) {
			current := options
			mutate(&current)
			if _, err := Run(context.Background(), current, ReferenceBudget()); err == nil {
				t.Fatal("unsafe workload options were accepted")
			}
		})
	}
}

func TestVerifyMetricsRequiresBoundedRoutesAndRejectsRunIdentity(t *testing.T) {
	valid := `sema_http_requests_total{method="PUT",route="PUT /v0alpha2/match-tickets/{ticket_id}",status="200",code=""} 10
sema_http_requests_total{method="POST",route="POST /v0alpha2/planning-runs/{run_id}",status="200",code=""} 1
sema_http_requests_total{method="POST",route="POST /v0alpha2/reservations/{reservation_id}/confirm",status="200",code=""} 1
sema_http_requests_total{method="POST",route="POST /v0alpha2/assignments/{assignment_id}/acknowledgments",status="200",code=""} 1
`
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		_, _ = writer.Write([]byte(valid))
	}))
	defer server.Close()
	if err := verifyMetrics(context.Background(), server.URL, "private-run", time.Second); err != nil {
		t.Fatal(err)
	}

	leaking := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		_, _ = writer.Write([]byte(valid + "private-run"))
	}))
	defer leaking.Close()
	if err := verifyMetrics(context.Background(), leaking.URL, "private-run", time.Second); err == nil {
		t.Fatal("metrics containing private run identity were accepted")
	}
}

func TestPostgreSQLServiceWorkload(t *testing.T) {
	dsn := os.Getenv(postgresTestDSNEnvironment)
	if dsn == "" {
		t.Skip(postgresTestDSNEnvironment + " is not set")
	}
	options := DefaultOptions()
	options.PostgresDSN = dsn
	options.Runs = 1
	options.Cycles = 1
	options.Random = bytes.NewReader(bytes.Repeat([]byte{9}, 8))
	options.Now = func() time.Time { return time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC) }
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	raceSafeBudget := ReferenceBudget()
	raceSafeBudget.MaxP95Millis = 1_000
	report, err := Run(ctx, options, raceSafeBudget)
	if err != nil {
		t.Fatalf("%v: %#v", err, report)
	}
	if report.Schema != ReportSchema || report.Profile != ProfileID || !report.WithinBudget ||
		len(report.Results) != 1 || report.Results[0].Tickets != 100 ||
		report.Results[0].Matches != 10 || report.Results[0].Assignments != 10 ||
		report.Results[0].ResourceRejected != 0 || !report.Results[0].MetricsVerified {
		t.Fatalf("report = %#v", report)
	}
	encoded, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"postgres://", "sema_service_workload_", "service-workload-token"} {
		if bytes.Contains(encoded, []byte(forbidden)) {
			t.Fatalf("report exposed %q: %s", forbidden, encoded)
		}
	}
}
