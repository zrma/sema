package observability_test

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestReferenceAlertsUseBoundedMetricsWithoutResourceLabels(t *testing.T) {
	_, source, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve alert test source")
	}
	content, err := os.ReadFile(filepath.Join(filepath.Dir(source), "..", "..", "deploy", "prometheus-rules.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	rules := string(content)
	for _, required := range []string{
		"alert: SemaReadinessUnavailable",
		`route="GET /readyz",status="503"`,
		"alert: SemaAdmissionExhausted",
		`code="ResourceExhausted"`,
		"alert: SemaDependencyUnavailable",
		`code=~"Unavailable|AuthenticationUnavailable"`,
		"alert: SemaStandardServiceP95Latency",
		`route=~".* /v0alpha2/.*"`,
		") > 0.75",
	} {
		if !strings.Contains(rules, required) {
			t.Fatalf("reference alert contract omits %q", required)
		}
	}
	for _, forbidden := range []string{
		"tenant=", "ticket_id=", "assignment_id=", "reservation_id=", "subject=",
	} {
		if strings.Contains(rules, forbidden) {
			t.Fatalf("reference alert contract uses resource identity label %q", forbidden)
		}
	}
}
