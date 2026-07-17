package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/zrma/sema/internal/lab"
)

func TestRunListsCanonicalWorkloads(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if exitCode := run([]string{"-list"}, &stdout, &stderr); exitCode != 0 {
		t.Fatalf("exit code = %d, stderr = %q", exitCode, stderr.String())
	}
	lines := strings.Split(strings.TrimSpace(stdout.String()), "\n")
	if len(lines) != len(lab.Workloads()) {
		t.Fatalf("listed workloads = %d; want %d", len(lines), len(lab.Workloads()))
	}
	if !strings.Contains(stdout.String(), "team-2v2-solo") || !strings.Contains(stdout.String(), "battle-royale-squad") {
		t.Fatalf("reference workloads missing from list:\n%s", stdout.String())
	}
}

func TestRunPrintsVersion(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if exitCode := run([]string{"-version"}, &stdout, &stderr); exitCode != 0 {
		t.Fatalf("exit code = %d, stderr = %q", exitCode, stderr.String())
	}
	if stdout.String() != "sema-lab dev\n" {
		t.Fatalf("version output = %q", stdout.String())
	}
}

func TestRunWritesDeterministicTextAndDetails(t *testing.T) {
	args := []string{"-details", "team-2v2-mixed"}
	var first, second, stderr bytes.Buffer
	if exitCode := run(args, &first, &stderr); exitCode != 0 {
		t.Fatalf("first exit code = %d, stderr = %q", exitCode, stderr.String())
	}
	stderr.Reset()
	if exitCode := run(args, &second, &stderr); exitCode != 0 {
		t.Fatalf("second exit code = %d, stderr = %q", exitCode, stderr.String())
	}
	if first.String() != second.String() {
		t.Fatalf("text output is not deterministic:\nfirst:\n%s\nsecond:\n%s", first.String(), second.String())
	}
	for _, expected := range []string{"scenario=team-2v2-mixed", "fingerprint=", "matched_players=4", "proposal=", "team=0 tickets="} {
		if !strings.Contains(first.String(), expected) {
			t.Fatalf("text output does not contain %q:\n%s", expected, first.String())
		}
	}
}

func TestRunWritesJSONReport(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if exitCode := run([]string{"-format", "json", "battle-royale-duo"}, &stdout, &stderr); exitCode != 0 {
		t.Fatalf("exit code = %d, stderr = %q", exitCode, stderr.String())
	}
	var report lab.Report
	if err := json.Unmarshal(stdout.Bytes(), &report); err != nil {
		t.Fatal(err)
	}
	if report.SchemaVersion != lab.SchemaVersion || len(report.Scenarios) != 1 {
		t.Fatalf("report = %#v", report)
	}
	if result := report.Scenarios[0]; result.Outcome.MatchedPlayers != 100 || len(result.Proposals) != 1 {
		t.Fatalf("battle royale result = %#v", result)
	}
}

func TestRunRejectsUnknownInputs(t *testing.T) {
	tests := [][]string{{"-format", "yaml"}, {"unknown-workload"}}
	for _, args := range tests {
		var stdout, stderr bytes.Buffer
		if exitCode := run(args, &stdout, &stderr); exitCode != 2 {
			t.Fatalf("args %v exit code = %d; want 2", args, exitCode)
		}
		if stderr.Len() == 0 {
			t.Fatalf("args %v produced no diagnostic", args)
		}
	}
}
