package performance

import (
	"fmt"
	"strings"
	"testing"
)

func TestEvaluateAcceptsRepeatedReferenceSamples(t *testing.T) {
	input := referenceOutput(3, false)
	report, err := Evaluate(strings.NewReader(input), 3)
	if err != nil {
		t.Fatal(err)
	}
	if report.SchemaVersion != ReportSchema || report.Profile != ProfileID || len(report.Benchmarks) != 4 {
		t.Fatalf("unexpected report: %+v", report)
	}
	for _, benchmark := range report.Benchmarks {
		if benchmark.Samples != 3 || !benchmark.WithinBudget {
			t.Fatalf("benchmark did not pass: %+v", benchmark)
		}
	}
}

func TestEvaluateRejectsBudgetRegression(t *testing.T) {
	report, err := Evaluate(strings.NewReader(referenceOutput(3, true)), 3)
	if err == nil || !strings.Contains(err.Error(), "exceeded") {
		t.Fatalf("error = %v", err)
	}
	failed := 0
	for _, benchmark := range report.Benchmarks {
		if !benchmark.WithinBudget {
			failed++
		}
	}
	if failed != 1 {
		t.Fatalf("failed benchmarks = %d; want 1", failed)
	}
}

func TestEvaluateRequiresEverySampleSet(t *testing.T) {
	_, err := Evaluate(strings.NewReader("BenchmarkPlanReferenceWorkloads/50v50-solo-8 3 1 ns/op 1 B/op 1 allocs/op\n"), 3)
	if err == nil || !strings.Contains(err.Error(), "samples") {
		t.Fatalf("error = %v", err)
	}
}

func referenceOutput(samples int, regress bool) string {
	names := []string{
		"BenchmarkPlanReferenceWorkloads/50v50-solo",
		"BenchmarkPlanLargeQueues/5v5/window-256/queue-100000",
		"BenchmarkEngineQueueSizes/5v5/queue-1000",
		"BenchmarkOpenReplay/events-1002",
	}
	var output strings.Builder
	for _, name := range names {
		for range samples {
			nanos := float64(1000)
			if regress && name == names[0] {
				nanos = referenceBudgets[name].MaxNanosPerOp + 1
			}
			fmt.Fprintf(&output, "%s-8 3 %.0f ns/op 100 B/op 10 allocs/op\n", name, nanos)
		}
	}
	return output.String()
}
