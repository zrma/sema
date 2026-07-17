package main

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/zrma/sema/internal/flowmatrix"
)

func TestRunPrintsVersion(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := run(context.Background(), []string{"-version"}, &stdout, &stderr); code != 0 {
		t.Fatalf("exit code = %d, stderr=%q", code, stderr.String())
	}
	if stdout.String() != "sema-flow-matrix dev\n" {
		t.Fatalf("version output = %q", stdout.String())
	}
}

func TestRunWritesComparableReducedMatrix(t *testing.T) {
	args := []string{
		"-format", "json", "-duration", "3s", "-population", "40", "-seeds", "42,43",
		"-profiles", "2:2,4:2", "-parallel", "2", "-game-duration", "20s",
		"-arrival-interval", "100ms", "-planning-interval", "1s", "-max-return-delay", "10s",
	}
	var stdout, stderr bytes.Buffer
	if code := run(context.Background(), args, &stdout, &stderr); code != 0 {
		t.Fatalf("exit code = %d, stderr=%q", code, stderr.String())
	}
	var report flowmatrix.Report
	if err := json.Unmarshal(stdout.Bytes(), &report); err != nil {
		t.Fatalf("decode report: %v\n%s", err, stdout.String())
	}
	if report.SchemaVersion != flowmatrix.SchemaVersion || !report.DemandComparable || len(report.Profiles) != 2 {
		t.Fatalf("report = %#v", report)
	}
	for _, profile := range report.Profiles {
		if profile.InitialTickets != (flowmatrix.Summary{Minimum: 24, Median: 24, Maximum: 24}) ||
			profile.MaxArrivalLagMillis.Maximum != 0 || profile.FinalIngressBacklogPlayers.Maximum != 0 {
			t.Fatalf("profile demand = %#v", profile)
		}
	}
}

func TestWriteTextReportUsesRangeVocabulary(t *testing.T) {
	report := flowmatrix.Report{
		SchemaVersion: flowmatrix.SchemaVersion,
		Seeds:         []int64{1, 2, 3},
		Profiles: []flowmatrix.ProfileReport{{
			Name: "c8-b2", Runs: 3, InitialTickets: flowmatrix.Summary{Minimum: 598, Median: 600, Maximum: 600},
		}},
	}
	var output bytes.Buffer
	if err := writeTextReport(&output, report); err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{"demand_comparable=false", "profile name=c8-b2", "initial_tickets=598/600/600", "capacity name=c8-b2", "wait name=c8-b2", "queue name=c8-b2", "quality name=c8-b2"} {
		if !strings.Contains(output.String(), expected) {
			t.Fatalf("text report omitted %q:\n%s", expected, output.String())
		}
	}
}

func TestRunRejectsInvalidLists(t *testing.T) {
	for _, args := range [][]string{{"-seeds", "42,x"}, {"-seeds", "42,42"}, {"-seeds", "-1"}, {"-profiles", "8/2"}, {"-profiles", "2:3"}, {"-profiles", "8:2,8:2"}, {"-format", "csv"}} {
		var stdout, stderr bytes.Buffer
		if code := run(context.Background(), args, &stdout, &stderr); code != 2 {
			t.Fatalf("args=%v exit code=%d stderr=%q", args, code, stderr.String())
		}
	}
}
