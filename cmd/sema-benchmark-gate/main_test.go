package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestRunPrintsVersion(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := run([]string{"-version"}, strings.NewReader(""), &stdout, &stderr); code != 0 {
		t.Fatalf("exit code = %d, stderr=%q", code, stderr.String())
	}
	if stdout.String() != "sema-benchmark-gate dev\n" {
		t.Fatalf("version output = %q", stdout.String())
	}
}

func TestRunEmitsFailureReport(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run(nil, strings.NewReader(""), &stdout, &stderr)
	if code != 1 || !strings.Contains(stdout.String(), `"schema_version": "sema-performance-v1"`) {
		t.Fatalf("exit code = %d, stdout=%q, stderr=%q", code, stdout.String(), stderr.String())
	}
}
