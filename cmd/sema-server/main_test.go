//go:build darwin || linux

package main

import (
	"bytes"
	"context"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunPrintsVersion(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := run(context.Background(), []string{"-version"}, &stdout, &stderr); code != 0 {
		t.Fatalf("exit code = %d, stderr=%q", code, stderr.String())
	}
	if stdout.String() != "sema-server dev\n" {
		t.Fatalf("version output = %q", stdout.String())
	}
}

func TestRunStartsAndGracefullyStopsLoopbackServer(t *testing.T) {
	journal := filepath.Join(t.TempDir(), "sema.journal")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var stdout, stderr bytes.Buffer
	code := run(ctx, []string{"-listen", "127.0.0.1:0", "-journal", journal}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit code = %d, stdout=%q, stderr=%q", code, stdout.String(), stderr.String())
	}
	if !strings.Contains(stdout.String(), "sema-server listening on 127.0.0.1:") {
		t.Fatalf("listen output = %q", stdout.String())
	}
}

func TestRunRejectsImplicitUnauthenticatedRemoteListener(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run(
		context.Background(),
		[]string{"-listen", ":0", "-journal", filepath.Join(t.TempDir(), "sema.journal")},
		&stdout,
		&stderr,
	)
	if code != 2 || !strings.Contains(stderr.String(), "requires -allow-unauthenticated-remote") {
		t.Fatalf("exit code = %d, stderr=%q", code, stderr.String())
	}
}

func TestLoopbackAddressValidation(t *testing.T) {
	tests := map[string]bool{
		"127.0.0.1:8080": true,
		"[::1]:8080":     true,
		"localhost:8080": true,
		":8080":          false,
		"0.0.0.0:8080":   false,
		"example:8080":   false,
		"invalid":        false,
	}
	for address, want := range tests {
		if got := isLoopbackAddress(address); got != want {
			t.Errorf("isLoopbackAddress(%q) = %t; want %t", address, got, want)
		}
	}
}
