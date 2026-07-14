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

func assertFailureCode(t *testing.T, err error, expected domain.FailureCode) {
	t.Helper()
	code, ok := domain.FailureCodeOf(err)
	if !ok || code != expected {
		t.Fatalf("failure code = %q, %v; want %q", code, err, expected)
	}
}
