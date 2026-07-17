package evaluation_test

import (
	"testing"
	"time"

	"github.com/zrma/sema/internal/domain"
	"github.com/zrma/sema/internal/evaluation"
	"github.com/zrma/sema/internal/planner"
)

func TestBatchFrontierDefaultBudgetDifferentialCorpus(t *testing.T) {
	for seed := uint64(1); seed <= 128; seed++ {
		scenario, err := evaluation.Generate(evaluation.WorkloadModel{
			ID: domain.SnapshotID("frontier-sweep"), Seed: seed, Now: fixtureNow,
			TicketCount: 8 + int(seed%3), MaxPartySize: 3,
			PartySizes: []evaluation.PartySizeWeight{
				{Size: 1, Weight: 55}, {Size: 2, Weight: 30}, {Size: 3, Weight: 15},
			},
			SkillCenter: 1500, SkillSpread: 250,
			Roles: []evaluation.RoleWeight{
				{Role: "support", Weight: 25}, {Role: "frontline", Weight: 25}, {Role: "damage", Weight: 50},
			},
			MinLatencyMS: 20, MaxLatencyMS: 100,
			MinWait: 0, MaxWait: 2 * time.Minute,
		})
		if err != nil {
			t.Fatal(err)
		}
		if seed%2 == 0 {
			scenario.BackfillTickets = []domain.BackfillTicket{{
				ID: "frontier-sweep-backfill", Revision: 1,
				SessionID: "frontier-sweep-session", RosterVersion: 1,
				OpenSlotsByTeam: []int{1 + int((seed/2)%2), 1}, EnqueuedAt: fixtureNow.Add(-time.Minute),
			}}
		}
		snapshot := domain.MatchmakingSnapshot{
			ID: scenario.ID, Now: scenario.Now,
			MatchTickets: scenario.MatchTickets, BackfillTickets: scenario.BackfillTickets,
			Policy: domain.MatchmakingPolicy{
				Version: "frontier-sweep-v1", TeamCount: 2, TeamSize: 3,
				MaxLatencyMillis: 120, MaxProposals: 3,
			},
		}
		batch, err := planner.Plan(snapshot)
		if err != nil {
			t.Fatalf("seed %d: plan: %v", seed, err)
		}
		comparison, err := evaluation.CompareBatchFrontier(snapshot, batch)
		if err != nil {
			t.Fatalf("seed %d: compare: %v", seed, err)
		}
		if comparison.Relation != evaluation.BatchFrontierEquivalent {
			t.Fatalf(
				"seed %d: relation=%s planner=%#v dominating=%#v candidates=%d batches=%d budget=%#v",
				seed, comparison.Relation, comparison.Planner, comparison.Dominating,
				comparison.Frontier.AdmissibleCandidates, comparison.Frontier.BatchesEvaluated, batch.Evidence,
			)
		}
		if batch.BudgetExhausted || batch.Evidence.CandidateGenerationTruncated || batch.Evidence.SelectionTruncated {
			t.Fatalf("seed %d: default small-queue search truncated: %#v", seed, batch.Evidence)
		}
	}
}
