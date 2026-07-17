//go:build darwin || linux

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/zrma/sema/internal/operational"
)

func TestRunPrintsVersion(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := run(context.Background(), []string{"-version"}, &stdout, &stderr); code != 0 {
		t.Fatalf("exit code = %d, stderr=%q", code, stderr.String())
	}
	if stdout.String() != "sema-ops-check dev\n" {
		t.Fatalf("version output = %q", stdout.String())
	}
}

func TestRunValidatesLoadAndRecovery(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run(
		context.Background(),
		[]string{"-cycles", "1", "-tickets-per-cycle", "20", "-concurrency", "4", "-timeout", "30s"},
		&stdout,
		&stderr,
	)
	if code != 0 {
		t.Fatalf("exit code = %d, stderr=%q", code, stderr.String())
	}
	var report operational.Report
	if err := json.Unmarshal(stdout.Bytes(), &report); err != nil {
		t.Fatalf("decode output: %v; output=%q", err, stdout.String())
	}
	if report.SchemaVersion != operational.ReportSchema || report.Tickets != 20 || report.Assignments != 2 {
		t.Fatalf("unexpected report: %+v", report)
	}
	if !report.MetricsVerified || !report.Recovery.RestartVerified || !report.Recovery.TornTailRecovered {
		t.Fatalf("validation evidence is incomplete: %+v", report)
	}
	if strings.Contains(stdout.String(), "ops-assignment") || strings.Contains(stdout.String(), "/sema-ops-") {
		t.Fatalf("output exposed internal resource identifiers or paths: %q", stdout.String())
	}
}

func TestRunRejectsInvalidConfiguration(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run(context.Background(), []string{"-timeout", "0s"}, &stdout, &stderr)
	if code != 2 || !strings.Contains(stderr.String(), "must be positive") {
		t.Fatalf("exit code = %d, stderr=%q", code, stderr.String())
	}
}

func TestRunRejectsInvalidWorkloadBounds(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run(context.Background(), []string{"-tickets-per-cycle", "11"}, &stdout, &stderr)
	if code != 2 || !strings.Contains(stderr.String(), "outside supported bounds") {
		t.Fatalf("exit code = %d, stderr=%q", code, stderr.String())
	}
}

func TestCheckBudget(t *testing.T) {
	report := operational.Report{
		DurationMillis: 100,
		Latency:        operational.LatencySummary{P95Millis: 10, MaxMillis: 20},
	}
	if err := checkBudget(report, config{maxP95: 11 * time.Millisecond, maxRequest: 21 * time.Millisecond, maxDuration: 101 * time.Millisecond}); err != nil {
		t.Fatal(err)
	}
	if err := checkBudget(report, config{maxP95: 9 * time.Millisecond}); err == nil || !strings.Contains(err.Error(), "p95") {
		t.Fatalf("error = %v", err)
	}
}
