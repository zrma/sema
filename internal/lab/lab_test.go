package lab_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/zrma/sema/internal/domain"
	"github.com/zrma/sema/internal/evaluation"
	"github.com/zrma/sema/internal/lab"
)

func TestRunIsDeterministicAndCanonical(t *testing.T) {
	first, err := lab.Run([]string{"team-5v5-mixed", "team-2v2-solo", "team-5v5-mixed"})
	if err != nil {
		t.Fatal(err)
	}
	second, err := lab.Run([]string{"team-2v2-solo", "team-5v5-mixed"})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("lab report is not deterministic: first=%#v second=%#v", first, second)
	}
	if first.SchemaVersion != lab.SchemaVersion || len(first.Scenarios) != 2 {
		t.Fatalf("report envelope = %#v", first)
	}
	if first.Scenarios[0].ID != "team-2v2-solo" || first.Scenarios[1].ID != "team-5v5-mixed" {
		t.Fatalf("scenario order = %#v", first.Scenarios)
	}
}

func TestRunCoversReferenceOutcomes(t *testing.T) {
	tests := []struct {
		id               string
		proposals        int
		matchedPlayers   int
		unmatchedPlayers int
		kind             domain.ProposalKind
	}{
		{id: "team-50v50-solo", proposals: 1, matchedPlayers: 100, kind: domain.ProposalNewMatch},
		{id: "battle-royale-duo", proposals: 1, matchedPlayers: 100, kind: domain.ProposalNewMatch},
		{id: "battle-royale-squad", proposals: 1, matchedPlayers: 100, kind: domain.ProposalNewMatch},
		{id: "backfill-2v2-two-slots", proposals: 1, matchedPlayers: 2, unmatchedPlayers: 2, kind: domain.ProposalBackfill},
		{id: "no-match-insufficient-capacity", unmatchedPlayers: 3},
		{id: "no-match-latency-hard-limit", unmatchedPlayers: 4},
		{id: "quality-role-balanced-2v2", proposals: 1, matchedPlayers: 4, unmatchedPlayers: 2, kind: domain.ProposalNewMatch},
		{id: "quality-wait-relaxed-2v2", proposals: 1, matchedPlayers: 4, kind: domain.ProposalNewMatch},
	}

	for _, test := range tests {
		t.Run(test.id, func(t *testing.T) {
			report, err := lab.Run([]string{test.id})
			if err != nil {
				t.Fatal(err)
			}
			result := report.Scenarios[0]
			if result.Outcome.ProposalCount != test.proposals ||
				result.Outcome.MatchedPlayers != test.matchedPlayers ||
				result.Outcome.UnmatchedPlayers != test.unmatchedPlayers {
				t.Fatalf("outcome = %#v", result.Outcome)
			}
			if test.proposals > 0 && result.Proposals[0].Kind != test.kind {
				t.Fatalf("proposal kind = %q; want %q", result.Proposals[0].Kind, test.kind)
			}
		})
	}
}

func TestRunFullCorpusPreservesCoverageAndOrdering(t *testing.T) {
	report, err := lab.Run(nil)
	if err != nil {
		t.Fatal(err)
	}
	workloads := lab.Workloads()
	if len(report.Scenarios) != len(workloads) {
		t.Fatalf("scenario count = %d; want %d", len(report.Scenarios), len(workloads))
	}
	for index, result := range report.Scenarios {
		if result.ID != workloads[index].ID {
			t.Fatalf("scenario %d ID = %q; want %q", index, result.ID, workloads[index].ID)
		}
		if result.Outcome.MatchedPlayers+result.Outcome.UnmatchedPlayers != result.Demand.Players {
			t.Fatalf("scenario %q loses player coverage: demand=%d outcome=%#v", result.ID, result.Demand.Players, result.Outcome)
		}
		if strings.HasPrefix(result.ID, "team-") || strings.HasPrefix(result.ID, "battle-royale-") {
			if result.Outcome.MatchedPlayers != result.Demand.Players || result.Outcome.UnmatchedPlayers != 0 {
				t.Fatalf("reference workload %q is not fully matched: %#v", result.ID, result.Outcome)
			}
		}
	}
}

func TestRunReportsObjectiveAndHardConstraintEvidence(t *testing.T) {
	report, err := lab.Run([]string{
		"quality-role-balanced-2v2",
		"quality-wait-relaxed-2v2",
		"no-match-latency-hard-limit",
	})
	if err != nil {
		t.Fatal(err)
	}
	byID := make(map[string]lab.ScenarioResult, len(report.Scenarios))
	for _, result := range report.Scenarios {
		byID[result.ID] = result
	}
	if result := byID["quality-role-balanced-2v2"]; result.Outcome.Search.TotalRolePenalty != 0 {
		t.Fatalf("role-quality result = %#v", result.Outcome)
	}
	if result := byID["quality-wait-relaxed-2v2"]; result.Outcome.Search.MaxRelaxationLevel != 1 ||
		result.Outcome.Search.TotalRolePenalty != 2 || result.Outcome.Search.WaitPriorityProposals != 1 {
		t.Fatalf("wait-relaxed result = %#v", result.Outcome)
	}
	latency := byID["no-match-latency-hard-limit"]
	if len(latency.Outcome.UnmatchedReasons) != 2 || latency.Outcome.UnmatchedReasons[0].Reason != domain.UnmatchedHardConstraint {
		t.Fatalf("latency hard-limit reasons = %#v", latency.Outcome.UnmatchedReasons)
	}
}

func TestRunReportsSyntheticMetricsAndOracleGap(t *testing.T) {
	report, err := lab.Run([]string{"synthetic-5v5-seeded-queue", "diagnostic-bounded-quality-gap", "diagnostic-candidate-window-gap"})
	if err != nil {
		t.Fatal(err)
	}
	byID := make(map[string]lab.ScenarioResult, len(report.Scenarios))
	for _, result := range report.Scenarios {
		byID[result.ID] = result
	}
	synthetic := byID["synthetic-5v5-seeded-queue"]
	if synthetic.Demand.MatchTickets != 40 || synthetic.Demand.Players != 65 || synthetic.Outcome.CoverageBasisPoints < 6000 {
		t.Fatalf("synthetic metrics = %#v", synthetic)
	}
	if synthetic.Outcome.Search.Nodes > 1000 || synthetic.Outcome.OldestUnmatchedWaitMillis == 0 {
		t.Fatalf("synthetic regression budget = %#v", synthetic.Outcome)
	}
	if synthetic.Oracle != nil {
		t.Fatalf("large synthetic queue unexpectedly ran exhaustive oracle: %#v", synthetic.Oracle)
	}
	if synthetic.Frontier != nil {
		t.Fatalf("large synthetic queue unexpectedly ran exhaustive batch frontier: %#v", synthetic.Frontier)
	}
	diagnostic := byID["diagnostic-bounded-quality-gap"]
	if diagnostic.Oracle == nil || diagnostic.Oracle.Relation != evaluation.QualityOraclePreferred {
		t.Fatalf("diagnostic oracle = %#v", diagnostic.Oracle)
	}
	if diagnostic.Oracle.PlannerQuality.TeamSkillGap != 500 || diagnostic.Oracle.OracleQuality.TeamSkillGap != 0 {
		t.Fatalf("diagnostic quality vectors = %#v", diagnostic.Oracle)
	}
	if diagnostic.Outcome.CoverageBasisPoints != 5000 || diagnostic.Outcome.Search.Nodes > 5 ||
		diagnostic.Outcome.Batch.CandidateGenerationNodes > 30 {
		t.Fatalf("diagnostic regression budget = %#v", diagnostic.Outcome)
	}
	window := byID["diagnostic-candidate-window-gap"]
	if window.Oracle == nil || window.Oracle.Relation != evaluation.QualityOraclePreferred {
		t.Fatalf("candidate window oracle = %#v", window.Oracle)
	}
	if window.Outcome.Search.CandidateTickets != 2 || window.Outcome.Search.TruncatedWindows != 1 {
		t.Fatalf("candidate window evidence = %#v", window.Outcome.Search)
	}
}

func TestRunReportsBatchQualityFrontier(t *testing.T) {
	report, err := lab.Run([]string{"batch-frontier-mixed-party-backfill", "diagnostic-batch-frontier-gap"})
	if err != nil {
		t.Fatal(err)
	}
	if report.SchemaVersion != "v0alpha5" {
		t.Fatalf("schema version = %q; want v0alpha5", report.SchemaVersion)
	}
	byID := make(map[string]lab.ScenarioResult, len(report.Scenarios))
	for _, result := range report.Scenarios {
		byID[result.ID] = result
	}

	mixed := byID["batch-frontier-mixed-party-backfill"]
	if mixed.Frontier == nil || mixed.Frontier.Relation != evaluation.BatchFrontierEquivalent {
		t.Fatalf("mixed frontier = %#v", mixed.Frontier)
	}
	if mixed.Frontier.Planner.SelectedBackfills != 1 || mixed.Frontier.Planner.ProposalCount != 2 ||
		mixed.Frontier.Planner.MatchedPlayers != 11 || mixed.Outcome.CoverageBasisPoints != 10_000 {
		t.Fatalf("mixed frontier outcome = %#v", mixed)
	}

	diagnostic := byID["diagnostic-batch-frontier-gap"]
	if diagnostic.Frontier == nil || diagnostic.Frontier.Relation != evaluation.BatchFrontierDominated {
		t.Fatalf("diagnostic frontier = %#v", diagnostic.Frontier)
	}
	if diagnostic.Frontier.Planner.ProposalCount != 1 || diagnostic.Frontier.Planner.MatchedPlayers != 2 ||
		diagnostic.Frontier.Dominating == nil || diagnostic.Frontier.Dominating.ProposalCount != 2 ||
		diagnostic.Frontier.Dominating.MatchedPlayers != 4 {
		t.Fatalf("diagnostic frontier witness = %#v", diagnostic.Frontier)
	}
}

func TestRunRejectsUnknownWorkload(t *testing.T) {
	report, err := lab.Run([]string{"unknown"})
	if code, ok := domain.FailureCodeOf(err); !ok || code != domain.FailureInvalidInput {
		t.Fatalf("unknown workload error = %v; want %s", err, domain.FailureInvalidInput)
	}
	if len(report.Scenarios) != 0 {
		t.Fatalf("unknown workload produced a partial report: %#v", report)
	}
}

func TestWorkloadsReturnsDefensiveCopies(t *testing.T) {
	first := lab.Workloads()
	first[0].Policy.Version = "mutated"
	first[0].Scenario.MatchTickets[0].Players[0].Skill = -1
	second := lab.Workloads()
	if second[0].Policy.Version == "mutated" || second[0].Scenario.MatchTickets[0].Players[0].Skill < 0 {
		t.Fatalf("built-in workload was mutated: %#v", second[0])
	}
}
