package runtimevalidation

import (
	"context"
	"os"
	"testing"
	"time"
)

const postgresTestDSNEnvironment = "SEMA_POSTGRES_TEST_DSN"

func TestRunRejectsMissingOrUnsafeConfiguration(t *testing.T) {
	for name, options := range map[string]Options{
		"missing DSN": {Timeout: time.Second},
		"zero timeout": {
			PostgresDSN: "postgres://database.example.invalid/sema",
		},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := Run(context.Background(), options); err == nil {
				t.Fatal("unsafe runtime validation configuration was accepted")
			}
		})
	}
}

func TestPostgreSQLRuntimeMatrix(t *testing.T) {
	dsn := os.Getenv(postgresTestDSNEnvironment)
	if dsn == "" {
		t.Skip(postgresTestDSNEnvironment + " is not set")
	}
	report, err := Run(context.Background(), Options{
		PostgresDSN: dsn,
		Timeout:     30 * time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	if report.Schema != reportSchema ||
		report.ReplicaCount != 2 ||
		report.ReservationWinnerCount != 1 ||
		report.ReservationConflictCount != 1 ||
		!report.TerminalAgreement ||
		!report.ReplicaRestartRecovered ||
		!report.DependencyReadinessFailedClosed ||
		!report.DependencyRequestFailedRetryable ||
		!report.LivenessMaintained ||
		!report.DependencyRecoveryComplete {
		t.Fatalf("runtime matrix report = %#v", report)
	}
}
