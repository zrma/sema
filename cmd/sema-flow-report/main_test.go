package main

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/zrma/sema/internal/flow"
)

func TestRunPrintsVersion(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := run(context.Background(), []string{"-version"}, &stdout, &stderr); code != 0 {
		t.Fatalf("exit code = %d, stderr=%q", code, stderr.String())
	}
	if stdout.String() != "sema-flow-report dev\n" {
		t.Fatalf("version output = %q", stdout.String())
	}
}

func TestRunWritesDeterministicJSONReport(t *testing.T) {
	args := []string{
		"-format", "json", "-duration", "90s", "-population", "40", "-matches-per-cycle", "2",
		"-concurrent-matches", "4", "-game-duration", "20s", "-planning-interval", "2s", "-max-return-delay", "10s",
	}
	var stdout, stderr bytes.Buffer
	if code := run(context.Background(), args, &stdout, &stderr); code != 0 {
		t.Fatalf("exit code = %d, stderr=%q", code, stderr.String())
	}
	var report flow.MeasurementReport
	if err := json.Unmarshal(stdout.Bytes(), &report); err != nil {
		t.Fatalf("decode report: %v\n%s", err, stdout.String())
	}
	if report.SchemaVersion != flow.MeasurementSchemaVersion || report.Assignments.Matches == 0 || report.Wait.SamplesPlayers == 0 {
		t.Fatalf("report = %#v", report)
	}
}

func TestRunWritesTextMetricVocabulary(t *testing.T) {
	var stdout, stderr bytes.Buffer
	args := []string{"-duration", "60s", "-population", "40", "-concurrent-matches", "4", "-game-duration", "20s", "-max-return-delay", "10s"}
	if code := run(context.Background(), args, &stdout, &stderr); code != 0 {
		t.Fatalf("exit code = %d, stderr=%q", code, stderr.String())
	}
	for _, expected := range []string{"assignment_yield_bps=", "wait samples_players=", "throughput confirmed_matches_per_minute_milli=", "queue mean_players=", "ingress samples_tickets=", "quality samples=", "final idle="} {
		if !strings.Contains(stdout.String(), expected) {
			t.Fatalf("text report omitted %q:\n%s", expected, stdout.String())
		}
	}
}

func TestRunRejectsUnsupportedFormat(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := run(context.Background(), []string{"-format", "csv"}, &stdout, &stderr); code != 2 || !strings.Contains(stderr.String(), "unsupported output format") {
		t.Fatalf("exit code = %d, stderr=%q", code, stderr.String())
	}
}
