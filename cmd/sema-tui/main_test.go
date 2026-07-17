package main

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func TestRunPrintsVersion(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := run(context.Background(), []string{"-version"}, strings.NewReader(""), &stdout, &stderr); code != 0 {
		t.Fatalf("exit code = %d, stderr=%q", code, stderr.String())
	}
	if stdout.String() != "sema-tui dev\n" {
		t.Fatalf("version output = %q", stdout.String())
	}
}

func TestRunRendersDeterministicASCIISnapshot(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run(
		context.Background(),
		[]string{
			"-snapshot", "-ascii", "-population", "40", "-concurrent-matches", "4",
			"-game-duration", "20s", "-arrival-interval", "1s", "-planning-interval", "2s",
			"-max-return-delay", "10s", "-steps", "80", "-width", "110", "-height", "36",
		},
		strings.NewReader(""),
		&stdout,
		&stderr,
	)
	if code != 0 {
		t.Fatalf("exit code = %d, stderr=%q", code, stderr.String())
	}
	for _, expected := range []string{
		"SEMA FLOW", "idle", "queued", "ready", "games", "cooldown", "WAITING POOL", "MATCH LIFECYCLE", "COMPLETED MATCHES", "rating", "team", "won",
	} {
		if !strings.Contains(stdout.String(), expected) {
			t.Fatalf("snapshot omitted %q:\n%s", expected, stdout.String())
		}
	}
}

func TestRunRejectsUnsafeSnapshotSize(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run(context.Background(), []string{"-snapshot", "-width", "20"}, strings.NewReader(""), &stdout, &stderr)
	if code != 2 || !strings.Contains(stderr.String(), "outside the supported range") {
		t.Fatalf("exit code = %d, stderr=%q", code, stderr.String())
	}
}
