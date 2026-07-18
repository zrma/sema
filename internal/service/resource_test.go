package service_test

import (
	"reflect"
	"testing"
	"time"

	"github.com/zrma/sema/internal/discovery"
	"github.com/zrma/sema/internal/domain"
	"github.com/zrma/sema/internal/repository"
	"github.com/zrma/sema/internal/service"
)

func TestPlanningSnapshotAndCandidateIndexPreserveVersionedInput(t *testing.T) {
	input := serviceSnapshot()
	planning, err := service.NewPlanningSnapshot(7, input)
	if err != nil {
		t.Fatal(err)
	}
	input.MatchTickets[0].Players[0].ID = "mutated-caller"
	if planning.MatchmakingInput().MatchTickets[0].Players[0].ID == "mutated-caller" {
		t.Fatal("caller mutation leaked into planning snapshot")
	}

	index := service.BuildCandidateIndex(planning)
	window, err := index.SelectWindow(planning, []int{2, 2}, 2)
	if err != nil {
		t.Fatal(err)
	}
	want := discovery.SelectWindow(planning.MatchmakingInput().MatchTickets, []int{2, 2}, 2)
	if !reflect.DeepEqual(window, want) {
		t.Fatalf("indexed window = %#v; want %#v", window, want)
	}
	window.Tickets[0].Players[0].ID = "mutated-window"
	again, err := index.SelectWindow(planning, []int{2, 2}, 2)
	if err != nil {
		t.Fatal(err)
	}
	if again.Tickets[0].Players[0].ID == "mutated-window" {
		t.Fatal("window mutation leaked into candidate index")
	}
	if index.RepositoryVersion() != planning.RepositoryVersion() {
		t.Fatalf("index version = %d; want %d", index.RepositoryVersion(), planning.RepositoryVersion())
	}
}

func TestCandidateIndexRejectsAnotherRepositoryVersion(t *testing.T) {
	planning, err := service.NewPlanningSnapshot(7, serviceSnapshot())
	if err != nil {
		t.Fatal(err)
	}
	index := service.BuildCandidateIndex(planning)
	newer, err := service.NewPlanningSnapshot(8, planning.MatchmakingInput())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := index.SelectWindow(newer, []int{2, 2}, 2); failureCode(err) != domain.FailureStaleSnapshot {
		t.Fatalf("stale index error = %v; want %s", err, domain.FailureStaleSnapshot)
	}
}

func TestServiceResourceKindsMapToScopedRepositoryKeys(t *testing.T) {
	kinds := []service.ResourceKind{
		service.ResourcePolicy,
		service.ResourceMatchTicket,
		service.ResourceBackfillTicket,
		service.ResourcePlanningSnapshot,
		service.ResourcePlanningRun,
		service.ResourceProposal,
		service.ResourcePlanningUnmatched,
		service.ResourceReservation,
		service.ResourceAssignment,
		service.ResourceAcknowledgment,
		service.ResourceOperationResult,
		service.ResourceDemandIdentity,
		service.ResourceBackfillSessionClaim,
		service.ResourceDemandReservation,
		service.ResourceLegacyImport,
	}
	for _, kind := range kinds {
		if !kind.Valid() {
			t.Fatalf("resource kind %q is not valid", kind)
		}
		key := service.Key("tenant-a", kind, "resource-a")
		if key != (repository.Key{Scope: "tenant-a", Kind: string(kind), ID: "resource-a"}) {
			t.Fatalf("resource key = %#v", key)
		}
	}
	if service.ResourceKind("unknown").Valid() {
		t.Fatal("unknown resource kind is valid")
	}
}

func serviceSnapshot() domain.MatchmakingSnapshot {
	now := time.Date(2026, 7, 18, 0, 0, 0, 0, time.UTC)
	return domain.MatchmakingSnapshot{
		ID:  "snapshot-a",
		Now: now,
		Policy: domain.MatchmakingPolicy{
			Version: "policy-a", TeamCount: 2, TeamSize: 2, MaxLatencyMillis: 100,
		},
		MatchTickets: []domain.MatchTicket{
			{
				ID: "ticket-a", Revision: 1, EnqueuedAt: now.Add(-2 * time.Second),
				Players: []domain.Player{{ID: "player-a", Skill: 1500, Role: "flex", LatencyMillis: 20}},
			},
			{
				ID: "ticket-b", Revision: 1, EnqueuedAt: now.Add(-time.Second),
				Players: []domain.Player{{ID: "player-b", Skill: 1510, Role: "flex", LatencyMillis: 20}},
			},
		},
	}
}

func failureCode(err error) domain.FailureCode {
	code, _ := domain.FailureCodeOf(err)
	return code
}
