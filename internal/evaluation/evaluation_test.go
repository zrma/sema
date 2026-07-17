package evaluation_test

import (
	"reflect"
	"testing"
	"time"

	"github.com/zrma/sema/internal/domain"
	"github.com/zrma/sema/internal/evaluation"
	"github.com/zrma/sema/internal/planner"
	"github.com/zrma/sema/internal/simulation"
)

var fixtureNow = time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)

func TestGenerateIsDeterministicAndBounded(t *testing.T) {
	model := workloadModel(42)
	first, err := evaluation.Generate(model)
	if err != nil {
		t.Fatal(err)
	}
	second, err := evaluation.Generate(model)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("same model generated different scenarios: first=%#v second=%#v", first, second)
	}
	model.Seed++
	different, err := evaluation.Generate(model)
	if err != nil {
		t.Fatal(err)
	}
	if reflect.DeepEqual(first, different) {
		t.Fatal("different seed generated the same scenario")
	}
	if len(first.MatchTickets) != 20 {
		t.Fatalf("ticket count = %d; want 20", len(first.MatchTickets))
	}
	seenPlayers := make(map[domain.PlayerID]struct{})
	for _, ticket := range first.MatchTickets {
		if len(ticket.Players) < 1 || len(ticket.Players) > 4 {
			t.Fatalf("party size = %d", len(ticket.Players))
		}
		if ticket.EnqueuedAt.After(fixtureNow) || ticket.EnqueuedAt.Before(fixtureNow.Add(-2*time.Minute)) {
			t.Fatalf("enqueue time = %s", ticket.EnqueuedAt)
		}
		for _, player := range ticket.Players {
			if player.Skill < 700 || player.Skill > 1300 || player.LatencyMillis < 20 || player.LatencyMillis > 120 {
				t.Fatalf("player is outside model bounds: %#v", player)
			}
			if _, duplicate := seenPlayers[player.ID]; duplicate {
				t.Fatalf("duplicate player %q", player.ID)
			}
			seenPlayers[player.ID] = struct{}{}
		}
	}
}

func TestGenerateRejectsInvalidModels(t *testing.T) {
	tests := []evaluation.WorkloadModel{
		{},
		func() evaluation.WorkloadModel {
			model := workloadModel(1)
			model.PartySizes[0].Weight = 0
			return model
		}(),
		func() evaluation.WorkloadModel { model := workloadModel(1); model.SkillSpread = 2_000; return model }(),
		func() evaluation.WorkloadModel { model := workloadModel(1); model.MaxWait = -time.Second; return model }(),
	}
	for _, model := range tests {
		if _, err := evaluation.Generate(model); err == nil {
			t.Fatalf("invalid model was accepted: %#v", model)
		}
	}
}

func TestOracleDetectsBoundedPlannerQualityGap(t *testing.T) {
	snapshot := qualitySnapshot(1)
	batch, err := planner.Plan(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	comparison, err := evaluation.CompareBatch(snapshot, batch)
	if err != nil {
		t.Fatal(err)
	}
	if comparison.Relation != evaluation.QualityOraclePreferred {
		t.Fatalf("comparison = %#v; want oracle preferred", comparison)
	}
	if comparison.PlannerEvidence.TeamSkillGap != 500 || comparison.Oracle.BestEvidence.TeamSkillGap != 0 {
		t.Fatalf("quality evidence = %#v", comparison)
	}
	if comparison.Oracle.AdmissibleCandidates == 0 {
		t.Fatal("oracle evaluated no admissible candidates")
	}
}

func TestOracleMatchesPlannerWhenBudgetCoversSmallCase(t *testing.T) {
	snapshot := qualitySnapshot(64)
	batch, err := planner.Plan(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	comparison, err := evaluation.CompareBatch(snapshot, batch)
	if err != nil {
		t.Fatal(err)
	}
	if comparison.Relation != evaluation.QualityEquivalent {
		t.Fatalf("comparison = %#v; want equivalent", comparison)
	}
}

func TestOracleRejectsUnboundedOrBackfillInput(t *testing.T) {
	snapshot := qualitySnapshot(64)
	for len(snapshot.MatchTickets) <= evaluation.MaxOracleTickets {
		sequence := len(snapshot.MatchTickets)
		snapshot.MatchTickets = append(snapshot.MatchTickets, soloTicket(sequence, 500, time.Second))
	}
	if _, err := evaluation.ExhaustiveNewMatch(snapshot); err == nil {
		t.Fatal("oracle accepted an unbounded queue")
	}
	snapshot = qualitySnapshot(64)
	snapshot.BackfillTickets = []domain.BackfillTicket{{
		ID: "backfill", Revision: 1, SessionID: "session", RosterVersion: 1,
		OpenSlotsByTeam: []int{1, 1}, EnqueuedAt: fixtureNow.Add(-time.Minute),
	}}
	if _, err := evaluation.ExhaustiveNewMatch(snapshot); err == nil {
		t.Fatal("new-match oracle accepted backfill input")
	}
	snapshot = qualitySnapshot(64)
	snapshot.Policy.TeamCount = evaluation.MaxOracleTeams + 1
	if _, err := evaluation.ExhaustiveNewMatch(snapshot); err == nil {
		t.Fatal("oracle accepted an unbounded team count")
	}
}

func TestOracleComparisonRejectsDifferentBatchSnapshot(t *testing.T) {
	snapshot := qualitySnapshot(64)
	batch, err := planner.Plan(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	batch.SnapshotID = "different"
	if _, err := evaluation.CompareBatch(snapshot, batch); err == nil {
		t.Fatal("oracle accepted a batch from another snapshot")
	}
}

func TestMeasureSeparatesPlayerCoverageAndWait(t *testing.T) {
	snapshot := qualitySnapshot(1)
	batch, err := planner.Plan(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	metrics := evaluation.Measure(
		structToScenario(snapshot),
		batch,
	)
	if metrics.DemandPlayers != 4 || metrics.MatchedPlayers != 2 || metrics.UnmatchedPlayers != 2 {
		t.Fatalf("coverage metrics = %#v", metrics)
	}
	if metrics.CoverageBasisPoints != 5_000 || metrics.OldestMatchedWaitMillis != 60_000 || metrics.OldestUnmatchedWaitMillis != 60_000 {
		t.Fatalf("quality metrics = %#v", metrics)
	}
}

func workloadModel(seed uint64) evaluation.WorkloadModel {
	return evaluation.WorkloadModel{
		ID:           "synthetic-queue",
		Seed:         seed,
		Now:          fixtureNow,
		TicketCount:  20,
		MaxPartySize: 4,
		PartySizes: []evaluation.PartySizeWeight{
			{Size: 1, Weight: 60}, {Size: 2, Weight: 25}, {Size: 4, Weight: 15},
		},
		SkillCenter: 1000,
		SkillSpread: 300,
		Roles: []evaluation.RoleWeight{
			{Role: "healer", Weight: 15}, {Role: "tank", Weight: 20}, {Role: "dps", Weight: 65},
		},
		MinLatencyMS: 20,
		MaxLatencyMS: 120,
		MinWait:      0,
		MaxWait:      2 * time.Minute,
	}
}

func qualitySnapshot(maxCandidates int) domain.MatchmakingSnapshot {
	return domain.MatchmakingSnapshot{
		ID:  "quality-oracle",
		Now: fixtureNow,
		Policy: domain.MatchmakingPolicy{
			Version:                  "quality-oracle-v1",
			TeamCount:                2,
			TeamSize:                 1,
			MaxLatencyMillis:         200,
			MaxProposals:             1,
			MaxSearchNodes:           100_000,
			MaxCandidatesPerProposal: maxCandidates,
		},
		MatchTickets: []domain.MatchTicket{
			soloTicket(0, 0, time.Minute),
			soloTicket(1, 1000, time.Minute),
			soloTicket(2, 500, 10*time.Second),
			soloTicket(3, 500, 10*time.Second),
		},
	}
}

func soloTicket(sequence, skill int, wait time.Duration) domain.MatchTicket {
	return domain.MatchTicket{
		ID:         domain.TicketID(string(rune('a'+sequence)) + "-ticket"),
		Revision:   1,
		EnqueuedAt: fixtureNow.Add(-wait),
		Players: []domain.Player{{
			ID: domain.PlayerID(string(rune('a'+sequence)) + "-player"), Skill: skill, LatencyMillis: 20,
		}},
	}
}

func structToScenario(snapshot domain.MatchmakingSnapshot) simulation.Scenario {
	return simulation.Scenario{
		ID: snapshot.ID, Now: snapshot.Now, MatchTickets: snapshot.MatchTickets, BackfillTickets: snapshot.BackfillTickets,
	}
}
