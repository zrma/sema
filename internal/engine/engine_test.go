package engine_test

import (
	"fmt"
	"reflect"
	"sync"
	"testing"
	"time"

	"sema/internal/domain"
	"sema/internal/engine"
)

var fixtureNow = time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC)

func TestNewMatchLifecycleThroughEngine(t *testing.T) {
	runtime := newEngine(t)
	submitSoloTickets(t, runtime, 4)
	policy := testPolicy(2, 2)
	registerPolicy(t, runtime, policy)

	first, err := runtime.Plan("snapshot-new-match", fixtureNow, policy.Version)
	if err != nil {
		t.Fatal(err)
	}
	second, err := runtime.Plan("snapshot-new-match", fixtureNow, policy.Version)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first, second) || len(first.Proposals) != 1 {
		t.Fatalf("repeated side-effect-free plan differs: first=%#v second=%#v", first, second)
	}

	proposal := first.Proposals[0]
	if _, err := runtime.Reserve(proposal, "reservation-engine", fixtureNow); err != nil {
		t.Fatal(err)
	}
	assignment, err := runtime.Confirm("reservation-engine", "assignment-engine", fixtureNow.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if assignment.Status != domain.AssignmentPending {
		t.Fatalf("assignment status = %q; want pending", assignment.Status)
	}
	completed, err := runtime.AcknowledgeAssignment(
		assignment.ID,
		domain.AssignmentAcknowledgmentRequest{OperationID: "operation-engine", Outcome: domain.AssignmentCompleted},
		fixtureNow.Add(2*time.Second),
	)
	if err != nil {
		t.Fatal(err)
	}
	if completed.Status != domain.AssignmentCompleted {
		t.Fatalf("assignment status = %q; want completed", completed.Status)
	}
	stored, exists := runtime.Assignment(assignment.ID)
	if !exists || !reflect.DeepEqual(completed, stored) {
		t.Fatalf("assignment read model = %#v, %v; want %#v", stored, exists, completed)
	}
}

func TestPolicyCatalogControlsPlanning(t *testing.T) {
	runtime := newEngine(t)
	policy := testPolicy(2, 2)
	fingerprint, err := runtime.RegisterPolicy(policy)
	if err != nil {
		t.Fatal(err)
	}
	stored, storedFingerprint, exists := runtime.Policy(policy.Version)
	if !exists || storedFingerprint != fingerprint || !reflect.DeepEqual(stored, policy) {
		t.Fatalf("registered policy = %#v, %q, %v", stored, storedFingerprint, exists)
	}
	stored.MaxLatencyMillis = 1
	again, _, _ := runtime.Policy(policy.Version)
	if again.MaxLatencyMillis != policy.MaxLatencyMillis {
		t.Fatal("policy read mutation leaked into catalog")
	}

	conflicting := policy
	conflicting.MaxLatencyMillis++
	_, err = runtime.RegisterPolicy(conflicting)
	if code, ok := domain.FailureCodeOf(err); !ok || code != domain.FailurePolicyConflict {
		t.Fatalf("policy conflict = %v; want %s", err, domain.FailurePolicyConflict)
	}
	submitSoloTickets(t, runtime, 4)
	batch, err := runtime.Plan("snapshot-registered-policy", fixtureNow, policy.Version)
	if err != nil {
		t.Fatal(err)
	}
	if len(batch.Proposals) != 1 || batch.Proposals[0].PolicyFingerprint != fingerprint {
		t.Fatalf("plan did not use registered policy: %#v", batch)
	}

	_, err = runtime.Plan("snapshot-unregistered-policy", fixtureNow, "missing-policy")
	if code, ok := domain.FailureCodeOf(err); !ok || code != domain.FailureInvalidInput {
		t.Fatalf("unregistered policy error = %v; want %s", err, domain.FailureInvalidInput)
	}
	if _, _, exists := newEngine(t).Policy(policy.Version); exists {
		t.Fatal("fresh process retained policy catalog")
	}
}

func TestBackfillStaleOutcomeThroughEngine(t *testing.T) {
	runtime := newEngine(t)
	submitSoloTickets(t, runtime, 2)
	backfill := domain.BackfillTicket{
		ID:              "backfill-engine",
		Revision:        1,
		SessionID:       "session-engine",
		RosterVersion:   7,
		OpenSlotsByTeam: []int{1, 1},
		EnqueuedAt:      fixtureNow.Add(-time.Minute),
	}
	if err := runtime.SubmitBackfillTicket(backfill); err != nil {
		t.Fatal(err)
	}
	policy := testPolicy(2, 2)
	registerPolicy(t, runtime, policy)
	batch, err := runtime.Plan("snapshot-backfill", fixtureNow, policy.Version)
	if err != nil {
		t.Fatal(err)
	}
	proposal := batch.Proposals[0]
	if proposal.Kind != domain.ProposalBackfill {
		t.Fatalf("proposal kind = %q; want backfill", proposal.Kind)
	}
	if _, err := runtime.Reserve(proposal, "reservation-backfill-engine", fixtureNow); err != nil {
		t.Fatal(err)
	}
	assignment, err := runtime.Confirm("reservation-backfill-engine", "assignment-backfill-engine", fixtureNow.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	failed, err := runtime.AcknowledgeAssignment(
		assignment.ID,
		domain.AssignmentAcknowledgmentRequest{
			OperationID:            "operation-backfill-stale",
			Outcome:                domain.AssignmentFailed,
			SessionID:              assignment.Backfill.SessionID,
			ExpectedRosterVersion:  assignment.Backfill.RosterVersion,
			ResultingRosterVersion: assignment.Backfill.RosterVersion + 1,
			FailureCode:            domain.FailureStaleSnapshot,
			Reason:                 "session authority observed a newer roster",
		},
		fixtureNow.Add(2*time.Second),
	)
	if err != nil {
		t.Fatal(err)
	}
	if failed.Status != domain.AssignmentFailed || failed.Acknowledgment.FailureCode != domain.FailureStaleSnapshot {
		t.Fatalf("backfill failure = %#v", failed)
	}
}

func TestCancelledReservationReturnsTicketsToNextCycle(t *testing.T) {
	runtime := newEngine(t)
	submitSoloTickets(t, runtime, 4)
	policy := testPolicy(2, 2)
	registerPolicy(t, runtime, policy)
	first, err := runtime.Plan("snapshot-before-cancel", fixtureNow, policy.Version)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.Reserve(first.Proposals[0], "reservation-cancel-engine", fixtureNow); err != nil {
		t.Fatal(err)
	}
	reserved, err := runtime.Plan("snapshot-while-reserved", fixtureNow.Add(time.Second), policy.Version)
	if err != nil {
		t.Fatal(err)
	}
	if len(reserved.Proposals) != 0 {
		t.Fatalf("reserved tickets were replanned: %#v", reserved.Proposals)
	}
	if _, err := runtime.CancelReservation("reservation-cancel-engine", fixtureNow.Add(2*time.Second)); err != nil {
		t.Fatal(err)
	}
	retried, err := runtime.Plan("snapshot-after-cancel", fixtureNow.Add(3*time.Second), policy.Version)
	if err != nil {
		t.Fatal(err)
	}
	if len(retried.Proposals) != 1 {
		t.Fatalf("cancelled reservation did not return tickets: %#v", retried)
	}
}

func TestRestartDropsProcessLocalStateAndReplayRestoresActiveDemand(t *testing.T) {
	beforeRestart := newEngine(t)
	tickets := soloTickets(4)
	submitTickets(t, beforeRestart, tickets)
	policy := testPolicy(2, 2)
	registerPolicy(t, beforeRestart, policy)

	expected, err := beforeRestart.Plan("snapshot-restart-replay", fixtureNow, policy.Version)
	if err != nil {
		t.Fatal(err)
	}
	if len(expected.Proposals) != 1 {
		t.Fatalf("proposal count before restart = %d; want 1", len(expected.Proposals))
	}
	if _, err := beforeRestart.Reserve(expected.Proposals[0], "reservation-restart-replay", fixtureNow); err != nil {
		t.Fatal(err)
	}

	afterRestart := newEngine(t)
	registerPolicy(t, afterRestart, policy)
	empty, err := afterRestart.Plan("snapshot-empty-after-restart", fixtureNow, policy.Version)
	if err != nil {
		t.Fatal(err)
	}
	if len(empty.Proposals) != 0 || len(empty.Unmatched) != 0 {
		t.Fatalf("fresh engine retained process-local demand: %#v", empty)
	}

	submitTickets(t, afterRestart, tickets)
	replayed, err := afterRestart.Plan("snapshot-restart-replay", fixtureNow, policy.Version)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(expected, replayed) {
		t.Fatalf("producer replay changed deterministic plan: before=%#v after=%#v", expected, replayed)
	}
	if _, err := afterRestart.Reserve(replayed.Proposals[0], "reservation-restart-replay", fixtureNow); err != nil {
		t.Fatalf("fresh process did not reset idempotency scope: %v", err)
	}
}

func TestRestartDropsAssignmentReadModel(t *testing.T) {
	beforeRestart := newEngine(t)
	submitSoloTickets(t, beforeRestart, 4)
	policy := testPolicy(2, 2)
	registerPolicy(t, beforeRestart, policy)
	batch, err := beforeRestart.Plan("snapshot-assignment-restart", fixtureNow, policy.Version)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := beforeRestart.Reserve(batch.Proposals[0], "reservation-assignment-restart", fixtureNow); err != nil {
		t.Fatal(err)
	}
	assignment, err := beforeRestart.Confirm(
		"reservation-assignment-restart",
		"assignment-before-restart",
		fixtureNow.Add(time.Second),
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, exists := beforeRestart.Assignment(assignment.ID); !exists {
		t.Fatal("confirmed assignment is missing before restart")
	}

	afterRestart := newEngine(t)
	if _, exists := afterRestart.Assignment(assignment.ID); exists {
		t.Fatal("fresh engine retained assignment read model")
	}
}

func TestReservationExpiryThroughEngineReleasesWholeProposal(t *testing.T) {
	runtime, err := engine.New(time.Second)
	if err != nil {
		t.Fatal(err)
	}
	submitSoloTickets(t, runtime, 4)
	policy := testPolicy(2, 2)
	registerPolicy(t, runtime, policy)
	batch, err := runtime.Plan("snapshot-before-expiry", fixtureNow, policy.Version)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.Reserve(batch.Proposals[0], "reservation-expiry-engine", fixtureNow); err != nil {
		t.Fatal(err)
	}
	_, err = runtime.Confirm(
		"reservation-expiry-engine",
		"assignment-after-expiry",
		fixtureNow.Add(time.Second),
	)
	if code, ok := domain.FailureCodeOf(err); !ok || code != domain.FailureReservationExpired {
		t.Fatalf("confirm error = %v; want %s", err, domain.FailureReservationExpired)
	}

	replanned, err := runtime.Plan("snapshot-after-expiry", fixtureNow.Add(time.Second), policy.Version)
	if err != nil {
		t.Fatal(err)
	}
	if len(replanned.Proposals) != 1 || len(replanned.Proposals[0].Tickets) != 4 {
		t.Fatalf("expiry left partially reserved demand: %#v", replanned)
	}
}

func TestConcurrentTerminalAcknowledgmentThroughEngineHasOneWinner(t *testing.T) {
	runtime := newEngine(t)
	submitSoloTickets(t, runtime, 4)
	policy := testPolicy(2, 2)
	registerPolicy(t, runtime, policy)
	batch, err := runtime.Plan("snapshot-terminal-race", fixtureNow, policy.Version)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.Reserve(batch.Proposals[0], "reservation-terminal-race", fixtureNow); err != nil {
		t.Fatal(err)
	}
	assignment, err := runtime.Confirm(
		"reservation-terminal-race",
		"assignment-terminal-race",
		fixtureNow.Add(time.Second),
	)
	if err != nil {
		t.Fatal(err)
	}

	requests := []domain.AssignmentAcknowledgmentRequest{
		{OperationID: "operation-complete", Outcome: domain.AssignmentCompleted},
		{OperationID: "operation-cancel", Outcome: domain.AssignmentCancelled, Reason: "allocation rejected"},
	}
	start := make(chan struct{})
	results := make(chan error, len(requests))
	var wait sync.WaitGroup
	for _, request := range requests {
		request := request
		wait.Add(1)
		go func() {
			defer wait.Done()
			<-start
			_, err := runtime.AcknowledgeAssignment(assignment.ID, request, fixtureNow.Add(2*time.Second))
			results <- err
		}()
	}
	close(start)
	wait.Wait()
	close(results)

	successes, invalidTransitions := 0, 0
	for err := range results {
		if err == nil {
			successes++
			continue
		}
		if code, ok := domain.FailureCodeOf(err); ok && code == domain.FailureInvalidTransition {
			invalidTransitions++
			continue
		}
		t.Fatalf("unexpected acknowledgment result: %v", err)
	}
	if successes != 1 || invalidTransitions != 1 {
		t.Fatalf("successes = %d, invalid transitions = %d; want 1, 1", successes, invalidTransitions)
	}
	stored, exists := runtime.Assignment(assignment.ID)
	if !exists || stored.Acknowledgment == nil || stored.Status == domain.AssignmentPending {
		t.Fatalf("terminal assignment read model = %#v, %v", stored, exists)
	}
}

func newEngine(t *testing.T) *engine.Engine {
	t.Helper()
	runtime, err := engine.New(time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	return runtime
}

func registerPolicy(
	t *testing.T,
	runtime *engine.Engine,
	policy domain.MatchmakingPolicy,
) domain.PolicyFingerprint {
	t.Helper()
	fingerprint, err := runtime.RegisterPolicy(policy)
	if err != nil {
		t.Fatal(err)
	}
	return fingerprint
}

func submitSoloTickets(t *testing.T, runtime *engine.Engine, count int) {
	t.Helper()
	submitTickets(t, runtime, soloTickets(count))
}

func soloTickets(count int) []domain.MatchTicket {
	tickets := make([]domain.MatchTicket, 0, count)
	for index := range count {
		tickets = append(tickets, domain.MatchTicket{
			ID:         domain.TicketID(string(rune('a' + index))),
			Revision:   1,
			EnqueuedAt: fixtureNow.Add(-time.Duration(count-index) * time.Second),
			Players: []domain.Player{{
				ID:            domain.PlayerID(string(rune('A' + index))),
				Skill:         1000 + index%2,
				LatencyMillis: 20,
			}},
		})
	}
	return tickets
}

func submitTickets(t *testing.T, runtime *engine.Engine, tickets []domain.MatchTicket) {
	t.Helper()
	for _, ticket := range tickets {
		if err := runtime.SubmitMatchTicket(ticket); err != nil {
			t.Fatal(err)
		}
	}
}

func testPolicy(teamCount, teamSize int) domain.MatchmakingPolicy {
	return domain.MatchmakingPolicy{
		Version:                  fmt.Sprintf("engine-test-%dx%d-v1", teamCount, teamSize),
		TeamCount:                teamCount,
		TeamSize:                 teamSize,
		MaxLatencyMillis:         200,
		MaxSearchNodes:           100_000,
		MaxCandidatesPerProposal: 64,
	}
}
