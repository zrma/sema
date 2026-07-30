package serviceworkload

import (
	"bytes"
	"context"
	"encoding/json"
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
		report.Results[0].ResourceRejected != 0 {
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
