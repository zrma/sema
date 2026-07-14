package engine_test

import (
	"reflect"
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

	first, err := runtime.Plan("snapshot-new-match", fixtureNow, policy)
	if err != nil {
		t.Fatal(err)
	}
	second, err := runtime.Plan("snapshot-new-match", fixtureNow, policy)
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
	batch, err := runtime.Plan("snapshot-backfill", fixtureNow, testPolicy(2, 2))
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
	first, err := runtime.Plan("snapshot-before-cancel", fixtureNow, policy)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.Reserve(first.Proposals[0], "reservation-cancel-engine", fixtureNow); err != nil {
		t.Fatal(err)
	}
	reserved, err := runtime.Plan("snapshot-while-reserved", fixtureNow.Add(time.Second), policy)
	if err != nil {
		t.Fatal(err)
	}
	if len(reserved.Proposals) != 0 {
		t.Fatalf("reserved tickets were replanned: %#v", reserved.Proposals)
	}
	if _, err := runtime.CancelReservation("reservation-cancel-engine", fixtureNow.Add(2*time.Second)); err != nil {
		t.Fatal(err)
	}
	retried, err := runtime.Plan("snapshot-after-cancel", fixtureNow.Add(3*time.Second), policy)
	if err != nil {
		t.Fatal(err)
	}
	if len(retried.Proposals) != 1 {
		t.Fatalf("cancelled reservation did not return tickets: %#v", retried)
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

func submitSoloTickets(t *testing.T, runtime *engine.Engine, count int) {
	t.Helper()
	for index := range count {
		ticket := domain.MatchTicket{
			ID:         domain.TicketID(string(rune('a' + index))),
			Revision:   1,
			EnqueuedAt: fixtureNow.Add(-time.Duration(count-index) * time.Second),
			Players: []domain.Player{{
				ID:            domain.PlayerID(string(rune('A' + index))),
				Skill:         1000 + index%2,
				LatencyMillis: 20,
			}},
		}
		if err := runtime.SubmitMatchTicket(ticket); err != nil {
			t.Fatal(err)
		}
	}
}

func testPolicy(teamCount, teamSize int) domain.MatchmakingPolicy {
	return domain.MatchmakingPolicy{
		Version:                  "engine-test-v1",
		TeamCount:                teamCount,
		TeamSize:                 teamSize,
		MaxLatencyMillis:         200,
		MaxSearchNodes:           100_000,
		MaxCandidatesPerProposal: 64,
	}
}
