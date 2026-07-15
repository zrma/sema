package coordinator_test

import (
	"fmt"
	"reflect"
	"sync"
	"testing"
	"time"

	"sema/internal/coordinator"
	"sema/internal/domain"
	"sema/internal/planner"
)

var fixtureNow = time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC)

func TestReservationAndAssignmentLifecycleIsIdempotent(t *testing.T) {
	owner := newCoordinator(t, time.Minute)
	upsertSoloTickets(t, owner, 4)
	proposal := firstProposal(t, owner, testPolicy(2, 2))

	firstReservation, err := owner.Reserve(proposal, "reservation-1", fixtureNow)
	if err != nil {
		t.Fatal(err)
	}
	secondReservation, err := owner.Reserve(proposal, "reservation-1", fixtureNow.Add(10*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(firstReservation, secondReservation) {
		t.Fatalf("repeated reserve changed the reservation:\nfirst: %#v\nsecond: %#v", firstReservation, secondReservation)
	}
	reservedSnapshot, err := owner.Snapshot("while-reserved", fixtureNow.Add(10*time.Second), testPolicy(2, 2))
	if err != nil {
		t.Fatal(err)
	}
	if len(reservedSnapshot.MatchTickets) != 0 {
		t.Fatalf("reserved tickets leaked into a new snapshot: %#v", reservedSnapshot.MatchTickets)
	}

	firstAssignment, err := owner.Confirm("reservation-1", "assignment-1", fixtureNow.Add(20*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	secondAssignment, err := owner.Confirm("reservation-1", "assignment-1", fixtureNow.Add(30*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(firstAssignment, secondAssignment) {
		t.Fatalf("repeated confirm changed the assignment:\nfirst: %#v\nsecond: %#v", firstAssignment, secondAssignment)
	}
	_, err = owner.Confirm("reservation-1", "assignment-2", fixtureNow.Add(30*time.Second))
	assertFailureCode(t, err, domain.FailureIdempotencyConflict)

	snapshot, err := owner.Snapshot("after-confirm", fixtureNow.Add(40*time.Second), testPolicy(2, 2))
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.MatchTickets) != 0 {
		t.Fatalf("confirmed tickets remain active: %#v", snapshot.MatchTickets)
	}
}

func TestConfirmRepeatsCASAndReleasesStaleReservation(t *testing.T) {
	owner := newCoordinator(t, time.Minute)
	tickets := upsertSoloTickets(t, owner, 4)
	proposal := firstProposal(t, owner, testPolicy(2, 2))
	if _, err := owner.Reserve(proposal, "reservation-before-update", fixtureNow); err != nil {
		t.Fatal(err)
	}

	updatedRef := proposal.Tickets[0]
	updated := tickets[updatedRef.ID]
	updated.Revision++
	if err := owner.UpsertMatchTicket(updated); err != nil {
		t.Fatal(err)
	}
	_, err := owner.Confirm("reservation-before-update", "assignment-stale", fixtureNow.Add(time.Second))
	assertFailureCode(t, err, domain.FailureStaleSnapshot)

	unchanged := proposal.Tickets[1]
	if _, err := owner.Reserve(proposalFor("after-stale", unchanged), "reservation-after-stale", fixtureNow.Add(2*time.Second)); err != nil {
		t.Fatalf("stale confirm did not release all resources: %v", err)
	}
}

func TestReserveRejectsStaleTicketRevisionAtomically(t *testing.T) {
	owner := newCoordinator(t, time.Minute)
	tickets := upsertSoloTickets(t, owner, 4)
	proposal := firstProposal(t, owner, testPolicy(2, 2))

	staleID := proposal.Tickets[0].ID
	updated := tickets[staleID]
	updated.Revision++
	if err := owner.UpsertMatchTicket(updated); err != nil {
		t.Fatal(err)
	}
	_, err := owner.Reserve(proposal, "stale-reservation", fixtureNow)
	assertFailureCode(t, err, domain.FailureStaleSnapshot)

	freshProposal := proposalFor("fresh-single", domain.TicketRef{ID: proposal.Tickets[1].ID, Revision: 1})
	if _, err := owner.Reserve(freshProposal, "fresh-reservation", fixtureNow); err != nil {
		t.Fatalf("stale failure left a partial reservation: %v", err)
	}
}

func TestReserveConflictIsAtomicAndCancelAllowsRetry(t *testing.T) {
	owner := newCoordinator(t, time.Minute)
	tickets := upsertSoloTickets(t, owner, 3)
	firstRef := domain.TicketRef{ID: "ticket-00", Revision: tickets["ticket-00"].Revision}
	secondRef := domain.TicketRef{ID: "ticket-01", Revision: tickets["ticket-01"].Revision}

	first := proposalFor("proposal-first", firstRef)
	if _, err := owner.Reserve(first, "reservation-first", fixtureNow); err != nil {
		t.Fatal(err)
	}
	overlapping := proposalFor("proposal-overlap", firstRef, secondRef)
	_, err := owner.Reserve(overlapping, "reservation-overlap", fixtureNow)
	assertFailureCode(t, err, domain.FailureReservationConflict)

	second := proposalFor("proposal-second", secondRef)
	if _, err := owner.Reserve(second, "reservation-second", fixtureNow); err != nil {
		t.Fatalf("conflict left a partial reservation: %v", err)
	}
	if _, err := owner.Cancel("reservation-first", fixtureNow.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	if _, err := owner.Reserve(first, "reservation-retry", fixtureNow.Add(2*time.Second)); err != nil {
		t.Fatalf("cancel did not release the ticket: %v", err)
	}
}

func TestConfirmRejectsExpiredReservation(t *testing.T) {
	owner := newCoordinator(t, time.Second)
	upsertSoloTickets(t, owner, 4)
	proposal := firstProposal(t, owner, testPolicy(2, 2))
	if _, err := owner.Reserve(proposal, "reservation-expiring", fixtureNow); err != nil {
		t.Fatal(err)
	}
	_, err := owner.Confirm("reservation-expiring", "assignment-late", fixtureNow.Add(time.Second))
	assertFailureCode(t, err, domain.FailureReservationExpired)

	if _, err := owner.Reserve(proposal, "reservation-after-expiry", fixtureNow.Add(time.Second)); err != nil {
		t.Fatalf("expiry did not release proposal resources: %v", err)
	}
}

func TestReserveRejectsStaleBackfillRoster(t *testing.T) {
	owner := newCoordinator(t, time.Minute)
	upsertSoloTickets(t, owner, 2)
	backfill := domain.BackfillTicket{
		ID:              "backfill-1",
		Revision:        1,
		SessionID:       "session-1",
		RosterVersion:   7,
		OpenSlotsByTeam: []int{1, 1},
		EnqueuedAt:      fixtureNow.Add(-time.Minute),
	}
	if err := owner.UpsertBackfillTicket(backfill); err != nil {
		t.Fatal(err)
	}
	proposal := firstProposal(t, owner, testPolicy(2, 2))
	if proposal.Kind != domain.ProposalBackfill {
		t.Fatalf("proposal kind = %q; want backfill", proposal.Kind)
	}

	backfill.Revision = 2
	backfill.RosterVersion = 8
	if err := owner.UpsertBackfillTicket(backfill); err != nil {
		t.Fatal(err)
	}
	_, err := owner.Reserve(proposal, "stale-backfill", fixtureNow)
	assertFailureCode(t, err, domain.FailureStaleSnapshot)
}

func TestConcurrentReserveHasOneWinner(t *testing.T) {
	owner := newCoordinator(t, time.Minute)
	upsertSoloTickets(t, owner, 4)
	proposal := firstProposal(t, owner, testPolicy(2, 2))

	start := make(chan struct{})
	results := make(chan error, 2)
	var wait sync.WaitGroup
	for sequence := 1; sequence <= 2; sequence++ {
		wait.Add(1)
		go func(sequence int) {
			defer wait.Done()
			<-start
			_, err := owner.Reserve(proposal, domain.ReservationID(fmt.Sprintf("reservation-%d", sequence)), fixtureNow)
			results <- err
		}(sequence)
	}
	close(start)
	wait.Wait()
	close(results)

	successes, conflicts := 0, 0
	for err := range results {
		if err == nil {
			successes++
			continue
		}
		if code, ok := domain.FailureCodeOf(err); ok && code == domain.FailureReservationConflict {
			conflicts++
			continue
		}
		t.Fatalf("unexpected reserve result: %v", err)
	}
	if successes != 1 || conflicts != 1 {
		t.Fatalf("successes = %d, conflicts = %d; want 1, 1", successes, conflicts)
	}
}

func TestUpsertRevisionAndSnapshotCopies(t *testing.T) {
	owner := newCoordinator(t, time.Minute)
	tickets := upsertSoloTickets(t, owner, 1)
	ticket := tickets["ticket-00"]
	if err := owner.UpsertMatchTicket(ticket); err != nil {
		t.Fatalf("idempotent upsert failed: %v", err)
	}
	ticket.Players[0].Skill++
	err := owner.UpsertMatchTicket(ticket)
	assertFailureCode(t, err, domain.FailureInvalidRevision)

	snapshot, err := owner.Snapshot("copy-1", fixtureNow, testPolicy(1, 1))
	if err != nil {
		t.Fatal(err)
	}
	snapshot.MatchTickets[0].Players[0].Skill = 0
	second, err := owner.Snapshot("copy-2", fixtureNow, testPolicy(1, 1))
	if err != nil {
		t.Fatal(err)
	}
	if second.MatchTickets[0].Players[0].Skill == 0 {
		t.Fatal("snapshot mutation leaked into coordinator state")
	}
}

func TestPlayerOwnershipIndexUpdatesHigherRevisionAtomically(t *testing.T) {
	owner := newCoordinator(t, time.Minute)
	original := matchTicket("ticket-owner", 1, "player-old")
	blocked := matchTicket("ticket-blocked", 1, "player-blocked")
	if err := owner.UpsertMatchTicket(original); err != nil {
		t.Fatal(err)
	}
	if err := owner.UpsertMatchTicket(blocked); err != nil {
		t.Fatal(err)
	}

	rejected := matchTicket("ticket-owner", 2, "player-new", "player-blocked")
	err := owner.UpsertMatchTicket(rejected)
	assertFailureCode(t, err, domain.FailureInvalidInput)
	if err := owner.UpsertMatchTicket(matchTicket("ticket-new-owner", 1, "player-new")); err != nil {
		t.Fatalf("rejected replacement partially acquired new player: %v", err)
	}
	err = owner.UpsertMatchTicket(matchTicket("ticket-old-contender", 1, "player-old"))
	assertFailureCode(t, err, domain.FailureInvalidInput)

	if err := owner.CancelMatchTicket("ticket-new-owner", 1); err != nil {
		t.Fatal(err)
	}
	updated := matchTicket("ticket-owner", 2, "player-replacement")
	if err := owner.UpsertMatchTicket(updated); err != nil {
		t.Fatal(err)
	}
	if err := owner.UpsertMatchTicket(matchTicket("ticket-old-reused", 1, "player-old")); err != nil {
		t.Fatalf("successful replacement did not release removed player: %v", err)
	}
	err = owner.UpsertMatchTicket(matchTicket("ticket-replacement-contender", 1, "player-replacement"))
	assertFailureCode(t, err, domain.FailureInvalidInput)
}

func TestPlayerOwnershipIndexReleasesOnTerminalTicketTransitions(t *testing.T) {
	t.Run("cancel", func(t *testing.T) {
		owner := newCoordinator(t, time.Minute)
		if err := owner.UpsertMatchTicket(matchTicket("ticket-cancelled", 1, "player-reusable")); err != nil {
			t.Fatal(err)
		}
		if err := owner.CancelMatchTicket("ticket-cancelled", 1); err != nil {
			t.Fatal(err)
		}
		if err := owner.UpsertMatchTicket(matchTicket("ticket-after-cancel", 1, "player-reusable")); err != nil {
			t.Fatalf("cancel did not release player ownership: %v", err)
		}
	})

	t.Run("confirm", func(t *testing.T) {
		owner := newCoordinator(t, time.Minute)
		if err := owner.UpsertMatchTicket(matchTicket("ticket-confirmed", 1, "player-reusable")); err != nil {
			t.Fatal(err)
		}
		proposal := firstProposal(t, owner, testPolicy(1, 1))
		if _, err := owner.Reserve(proposal, "reservation-player-index", fixtureNow); err != nil {
			t.Fatal(err)
		}
		if _, err := owner.Confirm(
			"reservation-player-index",
			"assignment-player-index",
			fixtureNow.Add(time.Second),
		); err != nil {
			t.Fatal(err)
		}
		if err := owner.UpsertMatchTicket(matchTicket("ticket-after-confirm", 1, "player-reusable")); err != nil {
			t.Fatalf("confirm did not release player ownership: %v", err)
		}
	})
}

func TestAssignmentCompletionIsIdempotentAndReadable(t *testing.T) {
	owner := newCoordinator(t, time.Minute)
	upsertSoloTickets(t, owner, 4)
	proposal := firstProposal(t, owner, testPolicy(2, 2))
	if _, err := owner.Reserve(proposal, "reservation-complete", fixtureNow); err != nil {
		t.Fatal(err)
	}
	if _, err := owner.Confirm("reservation-complete", "assignment-complete", fixtureNow.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	request := domain.AssignmentAcknowledgmentRequest{
		OperationID: "operation-complete",
		Outcome:     domain.AssignmentCompleted,
	}
	first, err := owner.AcknowledgeAssignment("assignment-complete", request, fixtureNow.Add(2*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	second, err := owner.AcknowledgeAssignment("assignment-complete", request, fixtureNow.Add(3*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first, second) || first.Status != domain.AssignmentCompleted {
		t.Fatalf("idempotent completion changed: first=%#v second=%#v", first, second)
	}

	conflicting := request
	conflicting.Outcome = domain.AssignmentCancelled
	conflicting.Reason = "consumer cancelled"
	_, err = owner.AcknowledgeAssignment("assignment-complete", conflicting, fixtureNow.Add(3*time.Second))
	assertFailureCode(t, err, domain.FailureIdempotencyConflict)
	request.OperationID = "another-operation"
	_, err = owner.AcknowledgeAssignment("assignment-complete", request, fixtureNow.Add(3*time.Second))
	assertFailureCode(t, err, domain.FailureInvalidTransition)

	first.Acknowledgment.Reason = "mutated by caller"
	stored, exists := owner.Assignment("assignment-complete")
	if !exists || stored.Acknowledgment.Reason != "" {
		t.Fatal("assignment read model was not defensively copied")
	}
}

func TestAssignmentCancellationDoesNotResurrectTickets(t *testing.T) {
	owner := newCoordinator(t, time.Minute)
	upsertSoloTickets(t, owner, 4)
	proposal := firstProposal(t, owner, testPolicy(2, 2))
	if _, err := owner.Reserve(proposal, "reservation-cancel-assignment", fixtureNow); err != nil {
		t.Fatal(err)
	}
	if _, err := owner.Confirm("reservation-cancel-assignment", "assignment-cancel", fixtureNow.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	request := domain.AssignmentAcknowledgmentRequest{
		OperationID: "operation-cancel",
		Outcome:     domain.AssignmentCancelled,
		Reason:      "allocation rejected",
	}
	assignment, err := owner.AcknowledgeAssignment("assignment-cancel", request, fixtureNow.Add(2*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if assignment.Status != domain.AssignmentCancelled {
		t.Fatalf("assignment status = %q; want cancelled", assignment.Status)
	}
	snapshot, err := owner.Snapshot("after-assignment-cancel", fixtureNow.Add(3*time.Second), testPolicy(2, 2))
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.MatchTickets) != 0 {
		t.Fatalf("cancelled assignment resurrected tickets: %#v", snapshot.MatchTickets)
	}
}

func TestBackfillAcknowledgmentValidatesRosterCAS(t *testing.T) {
	t.Run("completed", func(t *testing.T) {
		owner, assignment := confirmedBackfillAssignment(t)
		request := backfillAcknowledgment(assignment, domain.AssignmentCompleted, "operation-backfill-complete")
		completed, err := owner.AcknowledgeAssignment(assignment.ID, request, fixtureNow.Add(2*time.Second))
		if err != nil {
			t.Fatal(err)
		}
		if completed.Status != domain.AssignmentCompleted || completed.Acknowledgment.ResultingRosterVersion != 8 {
			t.Fatalf("completed assignment = %#v", completed)
		}
	})

	t.Run("non-advancing-version", func(t *testing.T) {
		owner, assignment := confirmedBackfillAssignment(t)
		request := backfillAcknowledgment(assignment, domain.AssignmentCompleted, "operation-invalid-version")
		request.ResultingRosterVersion = request.ExpectedRosterVersion
		_, err := owner.AcknowledgeAssignment(assignment.ID, request, fixtureNow.Add(2*time.Second))
		assertFailureCode(t, err, domain.FailureInvalidInput)
		stored, _ := owner.Assignment(assignment.ID)
		if stored.Status != domain.AssignmentPending {
			t.Fatalf("invalid acknowledgment changed assignment status to %q", stored.Status)
		}
	})

	t.Run("stale-failure", func(t *testing.T) {
		owner, assignment := confirmedBackfillAssignment(t)
		request := backfillAcknowledgment(assignment, domain.AssignmentFailed, "operation-backfill-stale")
		request.FailureCode = domain.FailureStaleSnapshot
		request.Reason = "session authority observed a newer roster"
		failed, err := owner.AcknowledgeAssignment(assignment.ID, request, fixtureNow.Add(2*time.Second))
		if err != nil {
			t.Fatal(err)
		}
		if failed.Status != domain.AssignmentFailed || failed.Acknowledgment.FailureCode != domain.FailureStaleSnapshot {
			t.Fatalf("failed assignment = %#v", failed)
		}
	})
}

func TestConcurrentAssignmentTerminalTransitionHasOneWinner(t *testing.T) {
	owner := newCoordinator(t, time.Minute)
	upsertSoloTickets(t, owner, 4)
	proposal := firstProposal(t, owner, testPolicy(2, 2))
	if _, err := owner.Reserve(proposal, "reservation-terminal-race", fixtureNow); err != nil {
		t.Fatal(err)
	}
	if _, err := owner.Confirm("reservation-terminal-race", "assignment-terminal-race", fixtureNow.Add(time.Second)); err != nil {
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
			_, err := owner.AcknowledgeAssignment("assignment-terminal-race", request, fixtureNow.Add(2*time.Second))
			results <- err
		}()
	}
	close(start)
	wait.Wait()
	close(results)

	successes, transitions := 0, 0
	for err := range results {
		if err == nil {
			successes++
			continue
		}
		if code, ok := domain.FailureCodeOf(err); ok && code == domain.FailureInvalidTransition {
			transitions++
			continue
		}
		t.Fatalf("unexpected terminal transition result: %v", err)
	}
	if successes != 1 || transitions != 1 {
		t.Fatalf("successes = %d, invalid transitions = %d; want 1, 1", successes, transitions)
	}
}

func newCoordinator(t *testing.T, ttl time.Duration) *coordinator.Coordinator {
	t.Helper()
	owner, err := coordinator.New(ttl)
	if err != nil {
		t.Fatal(err)
	}
	return owner
}

func upsertSoloTickets(
	t *testing.T,
	owner *coordinator.Coordinator,
	count int,
) map[domain.TicketID]domain.MatchTicket {
	t.Helper()
	tickets := make(map[domain.TicketID]domain.MatchTicket, count)
	for index := range count {
		id := domain.TicketID(fmt.Sprintf("ticket-%02d", index))
		ticket := domain.MatchTicket{
			ID:         id,
			Revision:   1,
			EnqueuedAt: fixtureNow.Add(-time.Duration(count-index) * time.Second),
			Players: []domain.Player{
				{ID: domain.PlayerID(fmt.Sprintf("player-%02d", index)), Skill: 1000 + index, LatencyMillis: 20},
			},
		}
		if err := owner.UpsertMatchTicket(ticket); err != nil {
			t.Fatal(err)
		}
		tickets[id] = ticket
	}
	return tickets
}

func matchTicket(id domain.TicketID, revision domain.Revision, playerIDs ...domain.PlayerID) domain.MatchTicket {
	players := make([]domain.Player, 0, len(playerIDs))
	for _, playerID := range playerIDs {
		players = append(players, domain.Player{ID: playerID, Skill: 1000, LatencyMillis: 20})
	}
	return domain.MatchTicket{
		ID:         id,
		Revision:   revision,
		EnqueuedAt: fixtureNow.Add(-time.Minute),
		Players:    players,
	}
}

func firstProposal(
	t *testing.T,
	owner *coordinator.Coordinator,
	configured domain.MatchmakingPolicy,
) domain.MatchProposal {
	t.Helper()
	snapshot, err := owner.Snapshot("snapshot-1", fixtureNow, configured)
	if err != nil {
		t.Fatal(err)
	}
	batch, err := planner.Plan(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if len(batch.Proposals) == 0 {
		t.Fatalf("planner returned no proposal: %#v", batch)
	}
	return batch.Proposals[0]
}

func testPolicy(teamCount, teamSize int) domain.MatchmakingPolicy {
	return domain.MatchmakingPolicy{
		Version:          "test-v1",
		TeamCount:        teamCount,
		TeamSize:         teamSize,
		MaxLatencyMillis: 200,
		MaxSearchNodes:   100_000,
	}
}

func proposalFor(id domain.ProposalID, refs ...domain.TicketRef) domain.MatchProposal {
	return domain.MatchProposal{
		ID:            id,
		Kind:          domain.ProposalNewMatch,
		PolicyVersion: "test-v1",
		Teams:         []domain.TeamAssignment{{Team: 0, Tickets: append([]domain.TicketRef(nil), refs...)}},
		Tickets:       append([]domain.TicketRef(nil), refs...),
	}
}

func confirmedBackfillAssignment(t *testing.T) (*coordinator.Coordinator, domain.Assignment) {
	t.Helper()
	owner := newCoordinator(t, time.Minute)
	upsertSoloTickets(t, owner, 2)
	backfill := domain.BackfillTicket{
		ID:              "backfill-ack",
		Revision:        1,
		SessionID:       "session-ack",
		RosterVersion:   7,
		OpenSlotsByTeam: []int{1, 1},
		EnqueuedAt:      fixtureNow.Add(-time.Minute),
	}
	if err := owner.UpsertBackfillTicket(backfill); err != nil {
		t.Fatal(err)
	}
	proposal := firstProposal(t, owner, testPolicy(2, 2))
	if _, err := owner.Reserve(proposal, "reservation-backfill-ack", fixtureNow); err != nil {
		t.Fatal(err)
	}
	assignment, err := owner.Confirm("reservation-backfill-ack", "assignment-backfill-ack", fixtureNow.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	return owner, assignment
}

func backfillAcknowledgment(
	assignment domain.Assignment,
	outcome domain.AssignmentStatus,
	operationID domain.OperationID,
) domain.AssignmentAcknowledgmentRequest {
	return domain.AssignmentAcknowledgmentRequest{
		OperationID:            operationID,
		Outcome:                outcome,
		SessionID:              assignment.Backfill.SessionID,
		ExpectedRosterVersion:  assignment.Backfill.RosterVersion,
		ResultingRosterVersion: assignment.Backfill.RosterVersion + 1,
	}
}

func assertFailureCode(t *testing.T, err error, expected domain.FailureCode) {
	t.Helper()
	code, ok := domain.FailureCodeOf(err)
	if !ok || code != expected {
		t.Fatalf("failure code = %q, %v; want %q", code, err, expected)
	}
}
