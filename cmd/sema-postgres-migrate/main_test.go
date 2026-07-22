package main

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
)

func TestRunPrintsVersionWithoutDatabaseEnvironment(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run(context.Background(), []string{"-version"}, emptyEnvironment, &stdout, &stderr, nil)
	if code != 0 || stdout.String() != "sema-postgres-migrate dev\n" {
		t.Fatalf("exit code = %d, stdout=%q, stderr=%q", code, stdout.String(), stderr.String())
	}
}

func TestRunInvokesMigrationWithoutLoggingConnectionString(t *testing.T) {
	const dsn = "postgres://service@database.example/sema"
	called := false
	var stdout, stderr bytes.Buffer
	code := run(context.Background(), nil, func(name string) (string, bool) {
		return dsn, name == postgresDSNEnvironment
	}, &stdout, &stderr, func(_ context.Context, actual string) error {
		called = true
		if actual != dsn {
			t.Fatalf("DSN = %q", actual)
		}
		return nil
	})
	if code != 0 || !called || stdout.String() != "sema-postgres-migrate completed\n" ||
		strings.Contains(stdout.String()+stderr.String(), dsn) {
		t.Fatalf("exit code = %d, called=%t, stdout=%q, stderr=%q", code, called, stdout.String(), stderr.String())
	}
}

func TestRunReportsRedactedMigrationFailure(t *testing.T) {
	const dsn = "postgres://service@database.example/sema"
	const sensitiveDetail = "database.example: schema version mismatch"
	var stdout, stderr bytes.Buffer
	code := run(context.Background(), nil, func(string) (string, bool) { return dsn, true }, &stdout, &stderr,
		func(context.Context, string) error { return errors.New(sensitiveDetail) },
	)
	if code != 1 || !strings.Contains(stderr.String(), "PostgreSQL migration failed") ||
		strings.Contains(stderr.String(), sensitiveDetail) || strings.Contains(stderr.String(), dsn) {
		t.Fatalf("exit code = %d, stderr=%q", code, stderr.String())
	}
}

func TestParseConfigurationRejectsMissingEnvironmentAndInvalidTimeout(t *testing.T) {
	for name, args := range map[string][]string{
		"missing DSN":  nil,
		"zero timeout": {"-timeout", "0s"},
	} {
		t.Run(name, func(t *testing.T) {
			lookup := emptyEnvironment
			if name == "zero timeout" {
				lookup = func(string) (string, bool) { return "postgres://service@database.example/sema", true }
			}
			var stderr bytes.Buffer
			if _, _, err := parseConfiguration(args, lookup, &stderr); err == nil {
				t.Fatal("invalid migration configuration was accepted")
			}
		})
	}
}

func emptyEnvironment(string) (string, bool) {
	return "", false
}
