package main

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func TestRunPrintsVersionWithoutDatabaseEnvironment(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run(context.Background(), []string{"-version"}, func(string) string { return "" }, &stdout, &stderr)
	if code != 0 || stdout.String() != "sema-service-workload dev\n" || stderr.Len() != 0 {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
}

func TestRunRequiresExplicitTestDatabase(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run(context.Background(), nil, func(string) string { return "" }, &stdout, &stderr)
	if code != 2 || !strings.Contains(stderr.String(), postgresTestDSNEnvironment) {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
}

func TestRunRejectsInvalidWorkloadFlagsBeforeDatabaseAccess(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run(
		context.Background(),
		[]string{"-runs", "0"},
		func(string) string { return "postgres://example.invalid/sema" },
		&stdout,
		&stderr,
	)
	if code != 2 || !strings.Contains(stderr.String(), "invalid workload configuration") {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
}
