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

func TestCompetingReservationsClaimProposalDemandAtomically(t *testing.T) {
	backend := repository.NewMemoryBackend()
	owner := repository.OpenMemory(backend)
	proposalID := seedReservationProposal(t, owner, "tenant-a")
	left, err := service.NewReservations(
		repository.OpenMemory(backend), func() time.Time { return demandFixtureNow }, 30*time.Second,
	)
	if err != nil {
		t.Fatal(err)
	}
	right, err := service.NewReservations(
		repository.OpenMemory(backend), func() time.Time { return demandFixtureNow }, 30*time.Second,
	)
	if err != nil {
		t.Fatal(err)
	}

	type result struct {
		reservationID domain.ReservationID
		mutation      service.ReservationMutation
		err           error
	}
	start := make(chan struct{})
	results := make(chan result, 2)
	var group sync.WaitGroup
	for index, reservations := range []*service.Reservations{left, right} {
		index, reservations := index, reservations
		group.Add(1)
		go func() {
			defer group.Done()
			<-start
			reservationID := domain.ReservationID("reservation-left")
			if index == 1 {
				reservationID = "reservation-right"
			}
			mutation, reserveErr := reservations.Reserve(
				context.Background(), "tenant-a", domain.OperationID("reserve-"+reservationID),
				reservationID, proposalID,
			)
			results <- result{reservationID: reservationID, mutation: mutation, err: reserveErr}
		}()
	}
	close(start)
	group.Wait()
	close(results)

	var winner, loser domain.ReservationID
	var created service.ReservationMutation
	for result := range results {
		if result.err == nil {
			winner = result.reservationID
			created = result.mutation
			continue
		}
		if failureCode(result.err) != domain.FailureReservationConflict {
			t.Fatalf("competing reservation error = %v", result.err)
		}
		loser = result.reservationID
	}
	if winner == "" || loser == "" || created.Record.Reservation.Status != domain.ReservationActive {
		t.Fatalf("winner=%q loser=%q created=%#v", winner, loser, created)
	}

	cancelled, err := left.Cancel(context.Background(), "tenant-a", "cancel-winner", winner)
	if err != nil {
		t.Fatal(err)
	}
	if cancelled.Record.Reservation.Status != domain.ReservationCancelled {
		t.Fatalf("cancelled reservation = %#v", cancelled)
	}
	released, err := right.Reserve(
		context.Background(), "tenant-a", domain.OperationID("reserve-"+loser), loser, proposalID,
	)
	if err != nil {
		t.Fatalf("reserve released proposal demand: %v", err)
	}
	if released.Record.Reservation.ID != loser || released.Record.Reservation.Status != domain.ReservationActive {
		t.Fatalf("released reservation = %#v", released)
	}

	replayed, err := left.Reserve(
		context.Background(), "tenant-a", domain.OperationID("reserve-"+winner), winner, proposalID,
	)
	if err != nil {
		t.Fatal(err)
	}
	if !replayed.Replayed || replayed.Record.StorageVersion != created.Record.StorageVersion ||
		replayed.Record.Reservation.Status != domain.ReservationActive {
		t.Fatalf("historical reservation replay = %#v; created=%#v", replayed, created)
	}
}

func TestReservationExpiryDurablyReleasesClaims(t *testing.T) {
	owner := repository.NewMemory()
	proposalID := seedReservationProposal(t, owner, "tenant-a")
	now := demandFixtureNow
	reservations, err := service.NewReservations(owner, func() time.Time { return now }, 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := reservations.Reserve(
		context.Background(), "tenant-a", "reserve-expiring", "reservation-expiring", proposalID,
	); err != nil {
		t.Fatal(err)
	}
	now = now.Add(5 * time.Second)
	expired, exists, err := reservations.Get(context.Background(), "tenant-a", "reservation-expiring")
	if err != nil || !exists || expired.Reservation.Status != domain.ReservationExpired {
		t.Fatalf("expired reservation = %#v exists=%t err=%v", expired, exists, err)
	}
	if _, err := reservations.Reserve(
		context.Background(), "tenant-a", "reserve-after-expiry", "reservation-after-expiry", proposalID,
	); err != nil {
		t.Fatalf("reserve after expiry: %v", err)
	}
	if _, err := reservations.Cancel(
		context.Background(), "tenant-a", "cancel-expired", "reservation-expiring",
	); failureCode(err) != domain.FailureReservationExpired {
		t.Fatalf("cancel expired reservation = %v; want %s", err, domain.FailureReservationExpired)
	}
}

func TestReservationRejectsDemandChangedAfterPlanning(t *testing.T) {
	owner := repository.NewMemory()
	proposalID := seedReservationProposal(t, owner, "tenant-a")
	tickets, err := service.NewMatchTickets(owner, func() time.Time { return demandFixtureNow })
	if err != nil {
		t.Fatal(err)
	}
	changed := planningTicket("ticket-0")
	changed.Revision = 2
	if _, err := tickets.Put(context.Background(), "tenant-a", "replace-after-plan", changed); err != nil {
		t.Fatal(err)
	}
	reservations, err := service.NewReservations(owner, func() time.Time { return demandFixtureNow }, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := reservations.Reserve(
		context.Background(), "tenant-a", "reserve-stale", "reservation-stale", proposalID,
	); failureCode(err) != domain.FailureStaleSnapshot {
		t.Fatalf("stale reservation = %v; want %s", err, domain.FailureStaleSnapshot)
	}
	if _, exists, err := reservations.Get(context.Background(), "tenant-a", "reservation-stale"); err != nil || exists {
		t.Fatalf("stale reservation persisted: exists=%t err=%v", exists, err)
	}
}

func seedReservationProposal(t *testing.T, owner repository.Repository, scope string) domain.ProposalID {
	t.Helper()
	seedPlanningDemand(t, owner, scope, 4)
	runs, err := service.NewPlanningRuns(owner, func() time.Time { return demandFixtureNow }, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := runs.Execute(context.Background(), scope, "execute-reservation-run", "reservation-run", "policy-plan"); err != nil {
		t.Fatal(err)
	}
	proposals, err := runs.Proposals(context.Background(), scope, "reservation-run")
	if err != nil {
		t.Fatal(err)
	}
	if len(proposals.Records) != 1 {
		t.Fatalf("reservation proposal count = %d; want 1", len(proposals.Records))
	}
	return proposals.Records[0].Proposal.ID
}
