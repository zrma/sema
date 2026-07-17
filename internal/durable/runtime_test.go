//go:build darwin || linux

package durable_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/zrma/sema/internal/domain"
	"github.com/zrma/sema/internal/durable"
)

var fixtureNow = time.Date(2026, time.July, 17, 12, 0, 0, 0, time.UTC)

func TestRuntimeRecoversReservationAssignmentAndIdempotency(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state", "sema.journal")
	runtime := openRuntime(t, path)
	policy := testPolicy()
	if _, err := runtime.RegisterPolicy(policy); err != nil {
		t.Fatal(err)
	}
	for _, ticket := range soloTickets(4) {
		if err := runtime.SubmitMatchTicket(ticket); err != nil {
			t.Fatal(err)
		}
	}
	batch, err := runtime.Plan("snapshot-durable", fixtureNow, policy.Version)
	if err != nil {
		t.Fatal(err)
	}
	if len(batch.Proposals) != 1 {
		t.Fatalf("proposal count = %d; want 1", len(batch.Proposals))
	}
	proposal := batch.Proposals[0]
	reservation, err := runtime.Reserve(proposal, "reservation-durable", fixtureNow)
	if err != nil {
		t.Fatal(err)
	}
	closeRuntime(t, runtime)

	runtime = openRuntime(t, path)
	storedPolicy, fingerprint, exists, err := runtime.Policy(policy.Version)
	if err != nil {
		t.Fatal(err)
	}
	if !exists || fingerprint != proposal.PolicyFingerprint || !reflect.DeepEqual(storedPolicy, policy) {
		t.Fatalf("recovered policy = %#v, %q, %t", storedPolicy, fingerprint, exists)
	}
	whileReserved, err := runtime.Plan("snapshot-while-reserved", fixtureNow.Add(time.Second), policy.Version)
	if err != nil {
		t.Fatal(err)
	}
	if len(whileReserved.Proposals) != 0 {
		t.Fatalf("recovered reservation did not exclude tickets: %#v", whileReserved.Proposals)
	}
	assignment, err := runtime.Confirm(reservation.ID, "assignment-durable", fixtureNow.Add(2*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	closeRuntime(t, runtime)

	runtime = openRuntime(t, path)
	retried, err := runtime.Confirm(reservation.ID, assignment.ID, fixtureNow.Add(3*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(assignment, retried) {
		t.Fatalf("confirmed assignment retry = %#v; want %#v", retried, assignment)
	}
	request := domain.AssignmentAcknowledgmentRequest{
		OperationID: "operation-durable", Outcome: domain.AssignmentCompleted,
	}
	completed, err := runtime.AcknowledgeAssignment(assignment.ID, request, fixtureNow.Add(4*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	closeRuntime(t, runtime)

	runtime = openRuntime(t, path)
	recovered, exists, err := runtime.Assignment(assignment.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !exists || !reflect.DeepEqual(recovered, completed) {
		t.Fatalf("recovered assignment = %#v, %t; want %#v", recovered, exists, completed)
	}
	retried, err = runtime.AcknowledgeAssignment(assignment.ID, request, fixtureNow.Add(5*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(retried, completed) {
		t.Fatalf("acknowledgment retry = %#v; want %#v", retried, completed)
	}
	closeRuntime(t, runtime)
}

func TestRuntimePersistsOrderedDecisionAudit(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sema.journal")
	runtime := openRuntime(t, path)
	policy := testPolicy()
	if _, err := runtime.RegisterPolicy(policy); err != nil {
		t.Fatal(err)
	}
	for _, ticket := range soloTickets(4) {
		if err := runtime.SubmitMatchTicket(ticket); err != nil {
			t.Fatal(err)
		}
	}
	batch, err := runtime.Plan("snapshot-audit", fixtureNow, policy.Version)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.Reserve(batch.Proposals[0], "reservation-audit", fixtureNow); err != nil {
		t.Fatal(err)
	}

	records, err := runtime.Audit(0, 100)
	if err != nil {
		t.Fatal(err)
	}
	wantKinds := []string{
		"runtime_configured",
		"policy_registered",
		"match_ticket_submitted", "match_ticket_submitted", "match_ticket_submitted", "match_ticket_submitted",
		"plan_completed", "proposal_reserved",
	}
	if len(records) != len(wantKinds) {
		t.Fatalf("audit record count = %d; want %d", len(records), len(wantKinds))
	}
	summaries, err := runtime.AuditSummaries(0, 100)
	if err != nil {
		t.Fatal(err)
	}
	encodedSummaries, err := json.Marshal(summaries)
	if err != nil {
		t.Fatal(err)
	}
	for _, privateValue := range []string{"a-ticket", "a-player", "snapshot-audit", "reservation-audit"} {
		if bytes.Contains(encodedSummaries, []byte(privateValue)) {
			t.Fatalf("redacted audit contains %q: %s", privateValue, encodedSummaries)
		}
	}
	if summaries[6].Counts["proposals"] != 1 || summaries[6].Counts["unmatched_tickets"] != 0 {
		t.Fatalf("plan audit summary = %#v", summaries[6])
	}
	for index, record := range records {
		if record.Sequence != uint64(index+1) || record.Kind != wantKinds[index] || record.Checksum == "" {
			t.Fatalf("audit record %d = %#v", index, record)
		}
	}
	if !bytes.Contains(records[6].Payload, []byte(`"proposals"`)) ||
		!bytes.Contains(records[6].Payload, []byte(`"unmatched_digest"`)) ||
		!bytes.Contains(records[6].Payload, []byte(batch.Proposals[0].ID)) {
		t.Fatalf("plan audit omits decision output: %s", records[6].Payload)
	}
	records[0].Payload[0] = '!'
	again, err := runtime.Audit(0, 1)
	if err != nil {
		t.Fatal(err)
	}
	if again[0].Payload[0] == '!' {
		t.Fatal("audit payload mutation leaked into runtime")
	}
	page, err := runtime.Audit(6, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(page) != 1 || page[0].Kind != "plan_completed" {
		t.Fatalf("audit page = %#v", page)
	}
	expected, err := runtime.Audit(0, 100)
	if err != nil {
		t.Fatal(err)
	}
	closeRuntime(t, runtime)

	runtime = openRuntime(t, path)
	recovered, err := runtime.Audit(0, 100)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(recovered, expected) {
		t.Fatalf("recovered audit differs: got=%#v want=%#v", recovered, expected)
	}
	if len(recovered) != len(wantKinds) {
		t.Fatalf("recovered audit count = %d; want %d", len(recovered), len(wantKinds))
	}
	proposal, exists, err := runtime.Proposal(batch.Proposals[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if !exists || !reflect.DeepEqual(proposal, batch.Proposals[0]) {
		t.Fatalf("recovered planned proposal = %#v, %t; want %#v", proposal, exists, batch.Proposals[0])
	}
	proposal.Teams[0].Tickets[0].ID = "mutated"
	againProposal, _, err := runtime.Proposal(batch.Proposals[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if againProposal.Teams[0].Tickets[0].ID == "mutated" {
		t.Fatal("planned proposal mutation leaked into runtime")
	}
	closeRuntime(t, runtime)
}

func TestRuntimeRecoversIncompleteTailAndRejectsCorruption(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sema.journal")
	runtime := openRuntime(t, path)
	if _, err := runtime.RegisterPolicy(testPolicy()); err != nil {
		t.Fatal(err)
	}
	closeRuntime(t, runtime)

	file, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.WriteString(`{"schema":"torn-tail"`); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	runtime = openRuntime(t, path)
	if _, _, exists, err := runtime.Policy(testPolicy().Version); err != nil || !exists {
		t.Fatalf("policy after tail recovery: exists=%t err=%v", exists, err)
	}
	closeRuntime(t, runtime)
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(contents) == 0 || contents[len(contents)-1] != '\n' || bytes.Contains(contents, []byte("torn-tail")) {
		t.Fatalf("incomplete tail was not removed: %q", contents)
	}

	file, err = os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.WriteString("{}\n"); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := durable.Open(path, time.Minute); err == nil || !strings.Contains(err.Error(), "schema") {
		t.Fatalf("complete journal corruption error = %v", err)
	}
}

func TestRuntimeEnforcesSingleWriterAndPrivateFileMode(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "sema.journal")
	first := openRuntime(t, path)
	if _, err := durable.Open(path, time.Minute); err == nil || !strings.Contains(err.Error(), "lock journal") {
		t.Fatalf("second writer error = %v", err)
	}
	stat, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if stat.Mode().Perm() != 0o600 {
		t.Fatalf("journal mode = %o; want 600", stat.Mode().Perm())
	}
	directory, err := os.Stat(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	if directory.Mode().Perm() != 0o700 {
		t.Fatalf("journal directory mode = %o; want 700", directory.Mode().Perm())
	}
	closeRuntime(t, first)
	second := openRuntime(t, path)
	closeRuntime(t, second)
}

func TestRuntimeRejectsReservationTTLDrift(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sema.journal")
	runtime := openRuntime(t, path)
	closeRuntime(t, runtime)
	if _, err := durable.Open(path, 2*time.Minute); err == nil || !strings.Contains(err.Error(), "journal requires") {
		t.Fatalf("reservation TTL drift error = %v", err)
	}
}

func TestRuntimeReplaysCancellationEventKinds(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sema.journal")
	runtime := openRuntime(t, path)
	policy := testPolicy()
	if _, err := runtime.RegisterPolicy(policy); err != nil {
		t.Fatal(err)
	}
	cancelledTicket := domain.MatchTicket{
		ID: "cancelled-ticket", Revision: 1, EnqueuedAt: fixtureNow.Add(-time.Minute),
		Players: []domain.Player{{ID: "cancelled-player", Skill: 1000, LatencyMillis: 20}},
	}
	if err := runtime.SubmitMatchTicket(cancelledTicket); err != nil {
		t.Fatal(err)
	}
	if err := runtime.CancelMatchTicket(cancelledTicket.ID, cancelledTicket.Revision); err != nil {
		t.Fatal(err)
	}
	backfill := domain.BackfillTicket{
		ID: "cancelled-backfill", Revision: 1, SessionID: "cancelled-session", RosterVersion: 7,
		OpenSlotsByTeam: []int{1, 1}, EnqueuedAt: fixtureNow.Add(-time.Minute),
		ExistingTeams: []domain.RosterTeamSummary{
			{PlayerCount: 1, SkillTotal: 1_000, RoleCounts: []domain.RoleCount{{Role: "healer", Count: 1}}, MaxLatencyMillis: 40},
			{PlayerCount: 1, SkillTotal: 1_000, RoleCounts: []domain.RoleCount{{Role: "dps", Count: 1}}, MaxLatencyMillis: 50},
		},
	}
	if err := runtime.SubmitBackfillTicket(backfill); err != nil {
		t.Fatal(err)
	}
	if err := runtime.CancelBackfillTicket(backfill.ID, backfill.Revision, backfill.RosterVersion); err != nil {
		t.Fatal(err)
	}
	for _, ticket := range soloTickets(4) {
		if err := runtime.SubmitMatchTicket(ticket); err != nil {
			t.Fatal(err)
		}
	}
	batch, err := runtime.Plan("snapshot-before-reservation-cancel", fixtureNow, policy.Version)
	if err != nil {
		t.Fatal(err)
	}
	reservation, err := runtime.Reserve(batch.Proposals[0], "reservation-cancel-replay", fixtureNow)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.CancelReservation(reservation.ID, fixtureNow.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	closeRuntime(t, runtime)

	runtime = openRuntime(t, path)
	retried, err := runtime.CancelReservation(reservation.ID, fixtureNow.Add(2*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if retried.Status != domain.ReservationCancelled {
		t.Fatalf("recovered reservation status = %q", retried.Status)
	}
	replanned, err := runtime.Plan("snapshot-after-reservation-cancel", fixtureNow.Add(3*time.Second), policy.Version)
	if err != nil {
		t.Fatal(err)
	}
	if len(replanned.Proposals) != 1 || len(replanned.Proposals[0].Tickets) != 4 {
		t.Fatalf("replayed cancellations produced %#v", replanned)
	}
	records, err := runtime.Audit(0, 100)
	if err != nil {
		t.Fatal(err)
	}
	wantKinds := []string{"match_ticket_cancelled", "backfill_ticket_submitted", "backfill_ticket_cancelled", "reservation_cancelled"}
	for _, want := range wantKinds {
		if !hasRecordKind(records, want) {
			t.Fatalf("audit omits %q: %#v", want, records)
		}
	}
	closeRuntime(t, runtime)
}

func TestPlanSnapshotIDIsDurableIdempotencyKey(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sema.journal")
	runtime := openRuntime(t, path)
	policy := testPolicy()
	if _, err := runtime.RegisterPolicy(policy); err != nil {
		t.Fatal(err)
	}
	for _, ticket := range soloTickets(4) {
		if err := runtime.SubmitMatchTicket(ticket); err != nil {
			t.Fatal(err)
		}
	}
	first, err := runtime.Plan("snapshot-idempotent", fixtureNow, policy.Version)
	if err != nil {
		t.Fatal(err)
	}
	before, err := runtime.Audit(0, 100)
	if err != nil {
		t.Fatal(err)
	}
	closeRuntime(t, runtime)

	runtime = openRuntime(t, path)
	retried, err := runtime.Plan("snapshot-idempotent", fixtureNow.Add(time.Hour), policy.Version)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(retried, first) {
		t.Fatalf("plan retry changed durable batch: first=%#v retry=%#v", first, retried)
	}
	after, err := runtime.Audit(0, 100)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(after, before) {
		t.Fatalf("plan retry appended another decision: before=%d after=%d", len(before), len(after))
	}
	_, err = runtime.Plan("snapshot-idempotent", fixtureNow, "different-policy")
	if code, ok := domain.FailureCodeOf(err); !ok || code != domain.FailureIdempotencyConflict {
		t.Fatalf("snapshot policy conflict = %v; want %s", err, domain.FailureIdempotencyConflict)
	}
	closeRuntime(t, runtime)
}

func openRuntime(t *testing.T, path string) *durable.Runtime {
	t.Helper()
	runtime, err := durable.Open(path, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	return runtime
}

func closeRuntime(t *testing.T, runtime *durable.Runtime) {
	t.Helper()
	if err := runtime.Close(); err != nil {
		t.Fatal(err)
	}
}

func testPolicy() domain.MatchmakingPolicy {
	return domain.MatchmakingPolicy{
		Version: "durable-v1", TeamCount: 2, TeamSize: 2, MaxLatencyMillis: 200,
		MaxSearchNodes: 100_000, MaxCandidatesPerProposal: 64,
	}
}

func soloTickets(count int) []domain.MatchTicket {
	tickets := make([]domain.MatchTicket, count)
	for index := range tickets {
		tickets[index] = domain.MatchTicket{
			ID: domain.TicketID(string(rune('a'+index)) + "-ticket"), Revision: 1,
			EnqueuedAt: fixtureNow.Add(-time.Duration(count-index) * time.Second),
			Players: []domain.Player{{
				ID:    domain.PlayerID(string(rune('a'+index)) + "-player"),
				Skill: 1000 + index, LatencyMillis: 20,
			}},
		}
	}
	return tickets
}

func hasRecordKind(records []durable.Record, kind string) bool {
	for _, record := range records {
		if record.Kind == kind {
			return true
		}
	}
	return false
}
