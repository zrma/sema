package evaluation_test

import (
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/zrma/sema/internal/domain"
	"github.com/zrma/sema/internal/evaluation"
	"github.com/zrma/sema/internal/planner"
)

func TestBatchFrontierDetectsBoundedCandidateCoverageGap(t *testing.T) {
	snapshot := batchFrontierCoverageGapSnapshot()
	batch, err := planner.Plan(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	comparison, err := evaluation.CompareBatchFrontier(snapshot, batch)
	if err != nil {
		t.Fatal(err)
	}
	if comparison.Relation != evaluation.BatchFrontierDominated {
		t.Fatalf("comparison = %#v; want dominated", comparison)
	}
	if comparison.Planner.ProposalCount != 1 || comparison.Planner.MatchedPlayers != 2 {
		t.Fatalf("planner quality = %#v", comparison.Planner)
	}
	if comparison.Dominating == nil || comparison.Dominating.ProposalCount != 2 || comparison.Dominating.MatchedPlayers != 4 {
		t.Fatalf("dominating point = %#v", comparison.Dominating)
	}
}

func TestBatchFrontierMatchesMixedPartyAndBackfillBatch(t *testing.T) {
	snapshot := mixedBatchFrontierSnapshot()
	batch, err := planner.Plan(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	comparison, err := evaluation.CompareBatchFrontier(snapshot, batch)
	if err != nil {
		t.Fatal(err)
	}
	if comparison.Relation != evaluation.BatchFrontierEquivalent {
		t.Fatalf("batch = %#v; comparison = %#v; want equivalent", batch, comparison)
	}
	if comparison.Planner.SelectedBackfills != 1 || comparison.Planner.ProposalCount != 2 || comparison.Planner.MatchedPlayers != 11 {
		t.Fatalf("planner quality = %#v", comparison.Planner)
	}
	if comparison.Frontier.PlacementsEvaluated == 0 || comparison.Frontier.AdmissibleCandidates == 0 || comparison.Frontier.BatchesEvaluated == 0 {
		t.Fatalf("frontier counters = %#v", comparison.Frontier)
	}
}

func TestBatchFrontierUsesRosterAwareBackfillQuality(t *testing.T) {
	snapshot := domain.MatchmakingSnapshot{
		ID: "frontier-roster-backfill", Now: fixtureNow,
		Policy: domain.MatchmakingPolicy{
			Version: "frontier-roster-v1", TeamCount: 2, TeamSize: 2, MaxLatencyMillis: 200,
			MaxProposals: 1, MaxSearchNodes: 100_000,
			RoleRequirements: []domain.RoleRequirement{{Role: "healer", MinPerTeam: 1}},
			RelaxationSteps:  []domain.RelaxationStep{{MaxTeamSkillGap: 1_000, MaxRolePenalty: 2}},
		},
		MatchTickets: []domain.MatchTicket{
			{ID: "high-dps", Revision: 1, EnqueuedAt: fixtureNow.Add(-time.Second), Players: []domain.Player{{ID: "p-high", Skill: 1_500, Role: "dps", LatencyMillis: 20}}},
			{ID: "low-healer", Revision: 1, EnqueuedAt: fixtureNow.Add(-time.Second), Players: []domain.Player{{ID: "p-low", Skill: 1_000, Role: "healer", LatencyMillis: 30}}},
			{ID: "mid-dps", Revision: 1, EnqueuedAt: fixtureNow.Add(-time.Second), Players: []domain.Player{{ID: "p-mid-dps", Skill: 1_250, Role: "dps", LatencyMillis: 20}}},
			{ID: "mid-healer", Revision: 1, EnqueuedAt: fixtureNow.Add(-time.Second), Players: []domain.Player{{ID: "p-mid-healer", Skill: 1_250, Role: "healer", LatencyMillis: 30}}},
		},
		BackfillTickets: []domain.BackfillTicket{{
			ID: "backfill", Revision: 1, SessionID: "session", RosterVersion: 7,
			OpenSlotsByTeam: []int{1, 1}, EnqueuedAt: fixtureNow.Add(-time.Minute),
			ExistingTeams: []domain.RosterTeamSummary{
				{PlayerCount: 1, SkillTotal: 1_000, RoleCounts: []domain.RoleCount{{Role: "healer", Count: 1}}, MaxLatencyMillis: 40},
				{PlayerCount: 1, SkillTotal: 1_500, RoleCounts: []domain.RoleCount{{Role: "dps", Count: 1}}, MaxLatencyMillis: 60},
			},
		}},
	}
	batch, err := planner.Plan(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	comparison, err := evaluation.CompareBatchFrontier(snapshot, batch)
	if err != nil {
		t.Fatal(err)
	}
	if comparison.Relation != evaluation.BatchFrontierEquivalent || comparison.Planner.SelectedBackfills != 1 ||
		comparison.Planner.MaxTeamSkillGap != 0 || comparison.Planner.MaxRolePenalty != 0 || comparison.Planner.MaxLatencyMillis != 60 {
		t.Fatalf("roster-aware comparison = %#v", comparison)
	}
}

func TestBatchFrontierIsInvariantToInputPermutation(t *testing.T) {
	snapshot := mixedBatchFrontierSnapshot()
	snapshot.BackfillTickets = append(snapshot.BackfillTickets, domain.BackfillTicket{
		ID: "whole-match-backfill", Revision: 1, SessionID: "whole-match-session", RosterVersion: 1,
		OpenSlotsByTeam: []int{5, 5}, EnqueuedAt: fixtureNow.Add(-30 * time.Second),
	})
	batch, err := planner.Plan(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	first, err := evaluation.CompareBatchFrontier(snapshot, batch)
	if err != nil {
		t.Fatal(err)
	}

	permuted := snapshot
	permuted.MatchTickets = slices.Clone(snapshot.MatchTickets)
	permuted.BackfillTickets = slices.Clone(snapshot.BackfillTickets)
	slices.Reverse(permuted.MatchTickets)
	slices.Reverse(permuted.BackfillTickets)
	permutedBatch, err := planner.Plan(permuted)
	if err != nil {
		t.Fatal(err)
	}
	second, err := evaluation.CompareBatchFrontier(permuted, permutedBatch)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("permutation changed frontier:\nfirst=%#v\nsecond=%#v", first, second)
	}
}

func TestBatchFrontierRejectsConflictingPlannerBatch(t *testing.T) {
	t.Run("ticket", func(t *testing.T) {
		snapshot := batchFrontierCoverageGapSnapshot()
		batch, err := planner.Plan(snapshot)
		if err != nil {
			t.Fatal(err)
		}
		duplicate := batch.Proposals[0]
		duplicate.ID = "duplicate-proposal"
		batch.Proposals = append(batch.Proposals, duplicate)
		if _, err := evaluation.CompareBatchFrontier(snapshot, batch); err == nil {
			t.Fatal("frontier accepted a batch that reuses tickets")
		}
	})

	t.Run("backfill", func(t *testing.T) {
		snapshot := batchFrontierCoverageGapSnapshot()
		snapshot.BackfillTickets = []domain.BackfillTicket{{
			ID: "shared-backfill", Revision: 1, SessionID: "shared-session", RosterVersion: 1,
			OpenSlotsByTeam: []int{1, 0}, EnqueuedAt: fixtureNow.Add(-time.Minute),
		}}
		fingerprint, err := domain.FingerprintPolicy(snapshot.Policy)
		if err != nil {
			t.Fatal(err)
		}
		target := domain.BackfillReference(snapshot.BackfillTickets[0])
		proposal := func(id domain.ProposalID, ticketIndex int) domain.MatchProposal {
			reference := domain.TicketReference(snapshot.MatchTickets[ticketIndex])
			return domain.MatchProposal{
				ID: id, Kind: domain.ProposalBackfill, PolicyVersion: snapshot.Policy.Version,
				PolicyFingerprint: fingerprint, Backfill: &target,
				Teams: []domain.TeamAssignment{
					{Team: 0, Tickets: []domain.TicketRef{reference}},
					{Team: 1},
				},
				Tickets: []domain.TicketRef{reference},
			}
		}
		batch := domain.ProposalBatch{
			SnapshotID: snapshot.ID,
			Proposals: []domain.MatchProposal{
				proposal("backfill-1", 0),
				proposal("backfill-2", 1),
			},
		}
		if _, err := evaluation.CompareBatchFrontier(snapshot, batch); err == nil {
			t.Fatal("frontier accepted a batch that reuses a backfill target")
		}
	})
}

func TestBatchFrontierRejectsInputsOutsideExhaustiveBound(t *testing.T) {
	t.Run("tickets", func(t *testing.T) {
		snapshot := batchFrontierCoverageGapSnapshot()
		for len(snapshot.MatchTickets) <= evaluation.MaxBatchFrontierTickets {
			snapshot.MatchTickets = append(snapshot.MatchTickets, soloTicket(len(snapshot.MatchTickets), 1500, time.Second))
		}
		if _, err := evaluation.ExhaustiveBatchFrontier(snapshot); err == nil {
			t.Fatal("frontier accepted too many match tickets")
		}
	})

	t.Run("backfills", func(t *testing.T) {
		snapshot := batchFrontierCoverageGapSnapshot()
		for index := 0; index <= evaluation.MaxBatchFrontierBackfills; index++ {
			snapshot.BackfillTickets = append(snapshot.BackfillTickets, domain.BackfillTicket{
				ID: domain.TicketID("backfill-" + string(rune('a'+index))), Revision: 1,
				SessionID: domain.SessionID("session-" + string(rune('a'+index))), RosterVersion: 1,
				OpenSlotsByTeam: []int{1, 1}, EnqueuedAt: fixtureNow.Add(-time.Minute),
			})
		}
		if _, err := evaluation.ExhaustiveBatchFrontier(snapshot); err == nil {
			t.Fatal("frontier accepted too many backfill tickets")
		}
	})

	t.Run("teams", func(t *testing.T) {
		snapshot := batchFrontierCoverageGapSnapshot()
		snapshot.Policy.TeamCount = evaluation.MaxBatchFrontierTeams + 1
		if _, err := evaluation.ExhaustiveBatchFrontier(snapshot); err == nil {
			t.Fatal("frontier accepted too many teams")
		}
	})
}

func batchFrontierCoverageGapSnapshot() domain.MatchmakingSnapshot {
	policy := domain.MatchmakingPolicy{
		Version: "batch-frontier-gap-v1", TeamCount: 2, TeamSize: 1,
		MaxLatencyMillis: 200, MaxProposals: 2, MaxSearchNodes: 100_000,
		MaxCandidatesPerProposal: 64, MaxBatchCandidates: 1, MaxBatchSearchNodes: 100_000,
	}
	return domain.MatchmakingSnapshot{
		ID: "batch-frontier-gap", Now: fixtureNow, Policy: policy,
		MatchTickets: []domain.MatchTicket{
			soloTicket(0, 1500, time.Minute), soloTicket(1, 1500, time.Minute),
			soloTicket(2, 1500, time.Minute), soloTicket(3, 1500, time.Minute),
		},
	}
}

func mixedBatchFrontierSnapshot() domain.MatchmakingSnapshot {
	partySizes := []int{1, 2, 3, 2, 3}
	tickets := make([]domain.MatchTicket, 0, len(partySizes))
	playerSequence := 0
	for ticketSequence, size := range partySizes {
		ticket := domain.MatchTicket{
			ID: domain.TicketID("party-" + string(rune('a'+ticketSequence))), Revision: 1,
			EnqueuedAt: fixtureNow.Add(-time.Duration(ticketSequence+1) * time.Second),
		}
		for range size {
			ticket.Players = append(ticket.Players, domain.Player{
				ID:    domain.PlayerID("player-" + string(rune('a'+playerSequence))),
				Skill: 1500, LatencyMillis: 30,
			})
			playerSequence++
		}
		tickets = append(tickets, ticket)
	}
	return domain.MatchmakingSnapshot{
		ID: "batch-frontier-mixed", Now: fixtureNow,
		Policy: domain.MatchmakingPolicy{
			Version: "batch-frontier-mixed-v1", TeamCount: 2, TeamSize: 5,
			MaxLatencyMillis: 100, MaxProposals: 2, MaxSearchNodes: 500_000,
			MaxCandidatesPerProposal: 512, MaxBatchCandidates: 1024, MaxBatchSearchNodes: 500_000,
		},
		MatchTickets: tickets,
		BackfillTickets: []domain.BackfillTicket{{
			ID: "mixed-backfill", Revision: 1, SessionID: "mixed-session", RosterVersion: 1,
			OpenSlotsByTeam: []int{1, 0}, EnqueuedAt: fixtureNow.Add(-time.Minute),
		}},
	}
}
