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

var demandFixtureNow = time.Date(2026, time.July, 18, 4, 0, 0, 0, time.UTC)

func TestDemandIdentityClaimSerializesMatchAndBackfillCompetition(t *testing.T) {
	backend := repository.NewMemoryBackend()
	matchOwner, err := service.NewMatchTickets(repository.OpenMemory(backend), func() time.Time { return demandFixtureNow })
	if err != nil {
		t.Fatal(err)
	}
	backfillOwner, err := service.NewBackfillTickets(repository.OpenMemory(backend), func() time.Time { return demandFixtureNow })
	if err != nil {
		t.Fatal(err)
	}
	start := make(chan struct{})
	results := make(chan error, 2)
	var group sync.WaitGroup
	group.Add(2)
	go func() {
		defer group.Done()
		<-start
		_, putErr := matchOwner.Put(context.Background(), "tenant-a", "match-create", domain.MatchTicket{
			ID: "shared-ticket", Revision: 1, EnqueuedAt: demandFixtureNow,
			Players: []domain.Player{{ID: "player-a", Skill: 1500, LatencyMillis: 20}},
		})
		results <- putErr
	}()
	go func() {
		defer group.Done()
		<-start
		_, putErr := backfillOwner.Put(
			context.Background(), "tenant-a", "backfill-create", backfillTicket("shared-ticket", "session-a"),
		)
		results <- putErr
	}()
	close(start)
	group.Wait()
	close(results)
	successes, conflicts := 0, 0
	for result := range results {
		if result == nil {
			successes++
			continue
		}
		if failureCode(result) == domain.FailureInvalidRevision {
			conflicts++
			continue
		}
		t.Fatalf("competition error = %v", result)
	}
	if successes != 1 || conflicts != 1 {
		t.Fatalf("competition successes=%d conflicts=%d; want 1/1", successes, conflicts)
	}

	snapshot, err := repository.OpenMemory(backend).Snapshot(context.Background(), "tenant-a")
	if err != nil {
		t.Fatal(err)
	}
	liveDemands := 0
	for _, resource := range snapshot.Resources {
		if !resource.Deleted && (resource.Key.Kind == string(service.ResourceMatchTicket) ||
			resource.Key.Kind == string(service.ResourceBackfillTicket)) {
			liveDemands++
		}
	}
	if liveDemands != 1 {
		t.Fatalf("live demand resources = %d; snapshot=%#v", liveDemands, snapshot)
	}
}

func TestBackfillSessionClaimSerializesCompetitionAndReleasesOnCancel(t *testing.T) {
	backend := repository.NewMemoryBackend()
	left, err := service.NewBackfillTickets(repository.OpenMemory(backend), func() time.Time { return demandFixtureNow })
	if err != nil {
		t.Fatal(err)
	}
	right, err := service.NewBackfillTickets(repository.OpenMemory(backend), func() time.Time { return demandFixtureNow })
	if err != nil {
		t.Fatal(err)
	}
	start := make(chan struct{})
	results := make(chan struct {
		id  domain.TicketID
		err error
	}, 2)
	var group sync.WaitGroup
	for index, owner := range []*service.BackfillTickets{left, right} {
		index, owner := index, owner
		group.Add(1)
		go func() {
			defer group.Done()
			<-start
			id := domain.TicketID("backfill-left")
			if index == 1 {
				id = "backfill-right"
			}
			_, putErr := owner.Put(
				context.Background(), "tenant-a", domain.OperationID("create-"+id),
				backfillTicket(id, "session-shared"),
			)
			results <- struct {
				id  domain.TicketID
				err error
			}{id: id, err: putErr}
		}()
	}
	close(start)
	group.Wait()
	close(results)
	var winner domain.TicketID
	conflicts := 0
	for result := range results {
		if result.err == nil {
			winner = result.id
			continue
		}
		if failureCode(result.err) == domain.FailureInvalidInput {
			conflicts++
			continue
		}
		t.Fatalf("session competition error = %v", result.err)
	}
	if winner == "" || conflicts != 1 {
		t.Fatalf("session winner=%q conflicts=%d", winner, conflicts)
	}
	if _, err := left.Cancel(
		context.Background(), "tenant-a", "cancel-winner", winner, 1, 7,
	); err != nil {
		t.Fatal(err)
	}
	if _, err := right.Put(
		context.Background(), "tenant-a", "create-after-cancel",
		backfillTicket("backfill-after", "session-shared"),
	); err != nil {
		t.Fatalf("reuse released session: %v", err)
	}
}

func TestBackfillReplacementRequiresMonotonicRosterFreshness(t *testing.T) {
	owner, err := service.NewBackfillTickets(repository.NewMemory(), func() time.Time { return demandFixtureNow })
	if err != nil {
		t.Fatal(err)
	}
	initial := backfillTicket("backfill-a", "session-a")
	if _, err := owner.Put(context.Background(), "tenant-a", "create-a", initial); err != nil {
		t.Fatal(err)
	}
	changedWithoutRosterAdvance := initial
	changedWithoutRosterAdvance.Revision = 2
	changedWithoutRosterAdvance.OpenSlotsByTeam = []int{2, 0}
	if _, err := owner.Put(
		context.Background(), "tenant-a", "replace-invalid-context", changedWithoutRosterAdvance,
	); failureCode(err) != domain.FailureInvalidRevision {
		t.Fatalf("same-roster replacement = %v; want %s", err, domain.FailureInvalidRevision)
	}
	advanced := changedWithoutRosterAdvance
	advanced.RosterVersion = 8
	if _, err := owner.Put(context.Background(), "tenant-a", "replace-valid", advanced); err != nil {
		t.Fatal(err)
	}
	backwards := advanced
	backwards.Revision = 3
	backwards.RosterVersion = 7
	if _, err := owner.Put(
		context.Background(), "tenant-a", "replace-backwards", backwards,
	); failureCode(err) != domain.FailureInvalidRevision {
		t.Fatalf("backwards roster replacement = %v; want %s", err, domain.FailureInvalidRevision)
	}
}

func backfillTicket(id domain.TicketID, sessionID domain.SessionID) domain.BackfillTicket {
	return domain.BackfillTicket{
		ID: id, Revision: 1, SessionID: sessionID, RosterVersion: 7,
		OpenSlotsByTeam: []int{1, 1},
		ExistingTeams: []domain.RosterTeamSummary{
			{PlayerCount: 1, SkillTotal: 1500, RoleCounts: []domain.RoleCount{{Role: "front", Count: 1}}, MaxLatencyMillis: 30},
			{PlayerCount: 1, SkillTotal: 1500, RoleCounts: []domain.RoleCount{{Role: "back", Count: 1}}, MaxLatencyMillis: 40},
		},
		EnqueuedAt: demandFixtureNow,
	}
}
