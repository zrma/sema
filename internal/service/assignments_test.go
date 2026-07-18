package service_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/zrma/sema/internal/domain"
	"github.com/zrma/sema/internal/repository"
	"github.com/zrma/sema/internal/service"
)

func TestReservationConfirmConsumesDemandAndAssignmentAcknowledges(t *testing.T) {
	owner := repository.NewMemory()
	proposalID := seedReservationProposal(t, owner, "tenant-a")
	reservations, err := service.NewReservations(owner, func() time.Time { return demandFixtureNow }, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := reservations.Reserve(
		context.Background(), "tenant-a", "reserve-confirm", "reservation-confirm", proposalID,
	); err != nil {
		t.Fatal(err)
	}
	confirmed, err := reservations.Confirm(
		context.Background(), "tenant-a", "confirm-assignment", "reservation-confirm", "assignment-confirm",
	)
	if err != nil {
		t.Fatal(err)
	}
	if confirmed.Record.Assignment.Status != domain.AssignmentPending || confirmed.Replayed {
		t.Fatalf("confirmed assignment = %#v", confirmed)
	}
	reservation, exists, err := reservations.Get(context.Background(), "tenant-a", "reservation-confirm")
	if err != nil || !exists || reservation.Reservation.Status != domain.ReservationConfirmed {
		t.Fatalf("confirmed reservation = %#v exists=%t err=%v", reservation, exists, err)
	}
	tickets, err := service.NewMatchTickets(owner, func() time.Time { return demandFixtureNow })
	if err != nil {
		t.Fatal(err)
	}
	for _, reference := range confirmed.Record.Assignment.Teams[0].Tickets {
		if _, exists, err := tickets.Get(context.Background(), "tenant-a", reference.ID); err != nil || exists {
			t.Fatalf("consumed ticket %q exists=%t err=%v", reference.ID, exists, err)
		}
	}

	assignments, err := service.NewAssignments(owner, func() time.Time { return demandFixtureNow.Add(time.Second) })
	if err != nil {
		t.Fatal(err)
	}
	completed, err := assignments.Acknowledge(
		context.Background(), "tenant-a", "assignment-confirm",
		domain.AssignmentAcknowledgmentRequest{
			OperationID: "ack-complete", Outcome: domain.AssignmentCompleted,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if completed.Record.Assignment.Status != domain.AssignmentCompleted ||
		completed.Record.Assignment.Acknowledgment == nil {
		t.Fatalf("completed assignment = %#v", completed)
	}
	polled, exists, err := assignments.Get(context.Background(), "tenant-a", "assignment-confirm")
	if err != nil || !exists || polled.Assignment.Status != domain.AssignmentCompleted {
		t.Fatalf("polled assignment = %#v exists=%t err=%v", polled, exists, err)
	}

	replayedConfirm, err := reservations.Confirm(
		context.Background(), "tenant-a", "confirm-assignment", "reservation-confirm", "assignment-confirm",
	)
	if err != nil {
		t.Fatal(err)
	}
	if !replayedConfirm.Replayed || replayedConfirm.Record.StorageVersion != confirmed.Record.StorageVersion ||
		replayedConfirm.Record.Assignment.Status != domain.AssignmentPending {
		t.Fatalf("historical confirmation replay = %#v; confirmed=%#v", replayedConfirm, confirmed)
	}
	replayedAck, err := assignments.Acknowledge(
		context.Background(), "tenant-a", "assignment-confirm",
		domain.AssignmentAcknowledgmentRequest{
			OperationID: "ack-complete", Outcome: domain.AssignmentCompleted,
		},
	)
	if err != nil || !replayedAck.Replayed ||
		replayedAck.Record.StorageVersion != completed.Record.StorageVersion {
		t.Fatalf("acknowledgment replay = %#v err=%v; completed=%#v", replayedAck, err, completed)
	}
}

func TestConcurrentAssignmentTerminalTransitionHasOneWinnerInRepositoryService(t *testing.T) {
	backend := repository.NewMemoryBackend()
	owner := repository.OpenMemory(backend)
	proposalID := seedReservationProposal(t, owner, "tenant-a")
	reservations, err := service.NewReservations(owner, func() time.Time { return demandFixtureNow }, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := reservations.Reserve(
		context.Background(), "tenant-a", "reserve-race", "reservation-race", proposalID,
	); err != nil {
		t.Fatal(err)
	}
	if _, err := reservations.Confirm(
		context.Background(), "tenant-a", "confirm-race", "reservation-race", "assignment-race",
	); err != nil {
		t.Fatal(err)
	}

	left, err := service.NewAssignments(repository.OpenMemory(backend), func() time.Time { return demandFixtureNow.Add(time.Second) })
	if err != nil {
		t.Fatal(err)
	}
	right, err := service.NewAssignments(repository.OpenMemory(backend), func() time.Time { return demandFixtureNow.Add(time.Second) })
	if err != nil {
		t.Fatal(err)
	}
	requests := []domain.AssignmentAcknowledgmentRequest{
		{OperationID: "ack-race-complete", Outcome: domain.AssignmentCompleted},
		{OperationID: "ack-race-cancel", Outcome: domain.AssignmentCancelled, Reason: "consumer cancelled"},
	}
	start := make(chan struct{})
	results := make(chan error, 2)
	var group sync.WaitGroup
	for index, assignments := range []*service.Assignments{left, right} {
		index, assignments := index, assignments
		group.Add(1)
		go func() {
			defer group.Done()
			<-start
			_, acknowledgeErr := assignments.Acknowledge(
				context.Background(), "tenant-a", "assignment-race", requests[index],
			)
			results <- acknowledgeErr
		}()
	}
	close(start)
	group.Wait()
	close(results)
	successes, conflicts := 0, 0
	for result := range results {
		if result == nil {
			successes++
			continue
		}
		if failureCode(result) == domain.FailureInvalidTransition {
			conflicts++
			continue
		}
		t.Fatalf("terminal competition error = %v", result)
	}
	if successes != 1 || conflicts != 1 {
		t.Fatalf("terminal successes=%d conflicts=%d; want 1/1", successes, conflicts)
	}
	reopened, err := service.NewAssignments(repository.OpenMemory(backend), func() time.Time { return demandFixtureNow.Add(2 * time.Second) })
	if err != nil {
		t.Fatal(err)
	}
	assignment, exists, err := reopened.Get(context.Background(), "tenant-a", "assignment-race")
	if err != nil || !exists || assignment.Assignment.Status == domain.AssignmentPending {
		t.Fatalf("reopened terminal assignment = %#v exists=%t err=%v", assignment, exists, err)
	}
}

func TestStaleConfirmCancelsReservationAndReleasesClaims(t *testing.T) {
	owner := repository.NewMemory()
	proposalID := seedReservationProposal(t, owner, "tenant-a")
	reservations, err := service.NewReservations(owner, func() time.Time { return demandFixtureNow }, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := reservations.Reserve(
		context.Background(), "tenant-a", "reserve-stale-confirm", "reservation-stale-confirm", proposalID,
	); err != nil {
		t.Fatal(err)
	}
	tickets, err := service.NewMatchTickets(owner, func() time.Time { return demandFixtureNow })
	if err != nil {
		t.Fatal(err)
	}
	changed := planningTicket("ticket-0")
	changed.Revision = 2
	if _, err := tickets.Put(context.Background(), "tenant-a", "replace-before-confirm", changed); err != nil {
		t.Fatal(err)
	}
	if _, err := reservations.Confirm(
		context.Background(), "tenant-a", "confirm-stale", "reservation-stale-confirm", "assignment-stale",
	); failureCode(err) != domain.FailureStaleSnapshot {
		t.Fatalf("stale confirmation = %v; want %s", err, domain.FailureStaleSnapshot)
	}
	reservation, exists, err := reservations.Get(context.Background(), "tenant-a", "reservation-stale-confirm")
	if err != nil || !exists || reservation.Reservation.Status != domain.ReservationCancelled {
		t.Fatalf("invalidated reservation = %#v exists=%t err=%v", reservation, exists, err)
	}
	snapshot, err := owner.Snapshot(context.Background(), "tenant-a")
	if err != nil {
		t.Fatal(err)
	}
	for _, resource := range snapshot.Resources {
		if resource.Key.Kind == string(service.ResourceDemandReservation) && !resource.Deleted {
			t.Fatalf("live reservation claim remained after stale confirmation: %#v", resource.Key)
		}
	}
}

func TestBackfillConfirmationReleasesSessionAndValidatesAcknowledgment(t *testing.T) {
	owner := repository.NewMemory()
	policies, err := service.NewPolicies(owner, func() time.Time { return demandFixtureNow })
	if err != nil {
		t.Fatal(err)
	}
	policy := servicePolicy("policy-backfill-assignment")
	policy.RoleRequirements = nil
	policy.RelaxationSteps = []domain.RelaxationStep{{AfterWait: 0, MaxTeamSkillGap: 100}}
	if _, err := policies.Put(
		context.Background(), "tenant-a", "register-backfill-assignment-policy", policy,
	); err != nil {
		t.Fatal(err)
	}
	tickets, err := service.NewMatchTickets(owner, func() time.Time { return demandFixtureNow })
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []domain.TicketID{"backfill-incoming-a", "backfill-incoming-b"} {
		if _, err := tickets.Put(
			context.Background(), "tenant-a", domain.OperationID("create-"+id), planningTicket(id),
		); err != nil {
			t.Fatal(err)
		}
	}
	backfills, err := service.NewBackfillTickets(owner, func() time.Time { return demandFixtureNow })
	if err != nil {
		t.Fatal(err)
	}
	target := backfillTicket("backfill-assignment", "session-assignment")
	if _, err := backfills.Put(
		context.Background(), "tenant-a", "create-backfill-assignment", target,
	); err != nil {
		t.Fatal(err)
	}
	runs, err := service.NewPlanningRuns(owner, func() time.Time { return demandFixtureNow }, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := runs.Execute(
		context.Background(), "tenant-a", "execute-backfill-assignment-run",
		"backfill-assignment-run", policy.Version,
	); err != nil {
		t.Fatal(err)
	}
	proposals, err := runs.Proposals(context.Background(), "tenant-a", "backfill-assignment-run")
	if err != nil {
		t.Fatal(err)
	}
	var proposalID domain.ProposalID
	for _, record := range proposals.Records {
		if record.Proposal.Kind == domain.ProposalBackfill {
			proposalID = record.Proposal.ID
			break
		}
	}
	if proposalID == "" {
		t.Fatalf("backfill proposal is missing: %#v", proposals)
	}
	reservations, err := service.NewReservations(owner, func() time.Time { return demandFixtureNow }, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := reservations.Reserve(
		context.Background(), "tenant-a", "reserve-backfill-assignment",
		"reservation-backfill-assignment", proposalID,
	); err != nil {
		t.Fatal(err)
	}
	confirmed, err := reservations.Confirm(
		context.Background(), "tenant-a", "confirm-backfill-assignment",
		"reservation-backfill-assignment", "assignment-backfill",
	)
	if err != nil {
		t.Fatal(err)
	}
	if confirmed.Record.Assignment.Kind != domain.ProposalBackfill || confirmed.Record.Assignment.Backfill == nil {
		t.Fatalf("confirmed backfill assignment = %#v", confirmed)
	}
	if _, exists, err := backfills.Get(context.Background(), "tenant-a", target.ID); err != nil || exists {
		t.Fatalf("consumed backfill exists=%t err=%v", exists, err)
	}
	replacement := backfillTicket("backfill-after-assignment", target.SessionID)
	if _, err := backfills.Put(
		context.Background(), "tenant-a", "create-backfill-after-assignment", replacement,
	); err != nil {
		t.Fatalf("reuse released backfill session: %v", err)
	}
	assignments, err := service.NewAssignments(owner, func() time.Time { return demandFixtureNow.Add(time.Second) })
	if err != nil {
		t.Fatal(err)
	}
	backfill := confirmed.Record.Assignment.Backfill
	completed, err := assignments.Acknowledge(
		context.Background(), "tenant-a", "assignment-backfill",
		domain.AssignmentAcknowledgmentRequest{
			OperationID: "ack-backfill", Outcome: domain.AssignmentCompleted,
			SessionID: backfill.SessionID, ExpectedRosterVersion: backfill.RosterVersion,
			ResultingRosterVersion: backfill.RosterVersion + 1,
		},
	)
	if err != nil || completed.Record.Assignment.Status != domain.AssignmentCompleted {
		t.Fatalf("backfill acknowledgment = %#v err=%v", completed, err)
	}
}
