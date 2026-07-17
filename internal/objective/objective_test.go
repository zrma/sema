package objective_test

import (
	"testing"
	"time"

	"github.com/zrma/sema/internal/domain"
	"github.com/zrma/sema/internal/objective"
)

var fixtureNow = time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC)

func TestEvaluateAppliesWaitRelaxation(t *testing.T) {
	policy := referencePolicy(false)
	shortTeams := dpsTeams(fixtureNow.Add(-10 * time.Second))
	short := objective.Evaluate(fixtureNow, shortTeams, policy, domain.ProposalNewMatch)
	if short.Admissible || short.Evidence.RolePenalty != 2 || short.Evidence.RelaxationLevel != 0 {
		t.Fatalf("short evaluation = %#v; want a quality-threshold rejection", short)
	}

	longTeams := dpsTeams(fixtureNow.Add(-time.Minute))
	long := objective.Evaluate(fixtureNow, longTeams, policy, domain.ProposalNewMatch)
	if !long.Admissible || long.Evidence.RelaxationLevel != 1 || !long.Evidence.WaitPriority {
		t.Fatalf("long evaluation = %#v; want relaxed wait-first admission", long)
	}
}

func TestEvaluateNeverRelaxesHardRole(t *testing.T) {
	policy := referencePolicy(true)
	evaluation := objective.Evaluate(fixtureNow, dpsTeams(fixtureNow.Add(-time.Minute)), policy, domain.ProposalNewMatch)
	if evaluation.Admissible || !evaluation.HardViolation {
		t.Fatalf("evaluation = %#v; hard role was relaxed", evaluation)
	}
}

func TestEvaluateBackfillDefersRosterQuality(t *testing.T) {
	policy := referencePolicy(true)
	evaluation := objective.Evaluate(fixtureNow, dpsTeams(fixtureNow.Add(-time.Second)), policy, domain.ProposalBackfill)
	if !evaluation.Admissible || evaluation.Evidence.RolePenalty != 0 || evaluation.Evidence.TeamSkillGap != 0 {
		t.Fatalf("backfill evaluation = %#v; roster quality should be deferred", evaluation)
	}
}

func TestCompareSwitchesFromQualityToWait(t *testing.T) {
	quality := objective.Evaluation{Admissible: true, Evidence: domain.ScoreEvidence{
		RolePenalty:      0,
		TeamSkillGap:     5,
		OldestWaitMillis: 10_000,
		TotalWaitMillis:  20_000,
		MaxLatencyMillis: 40,
	}}
	waiting := objective.Evaluation{Admissible: true, Evidence: domain.ScoreEvidence{
		WaitPriority:     true,
		RolePenalty:      1,
		TeamSkillGap:     30,
		OldestWaitMillis: 60_000,
		TotalWaitMillis:  120_000,
		MaxLatencyMillis: 50,
	}}

	if objective.Compare(waiting, quality) >= 0 {
		t.Fatal("a wait-first candidate should outrank a short-wait quality candidate")
	}
	waiting.Evidence.WaitPriority = false
	if objective.Compare(quality, waiting) >= 0 {
		t.Fatal("quality-first comparison should prefer the lower role penalty and skill gap")
	}
}

func TestTicketWaitPriorityUsesEachDemandAge(t *testing.T) {
	policy := referencePolicy(false)
	if priority, wait := objective.TicketWaitPriority(fixtureNow, fixtureNow.Add(-29*time.Second), policy); priority || wait != 29_000 {
		t.Fatalf("short demand priority=%t wait=%d; want false, 29000", priority, wait)
	}
	if priority, wait := objective.TicketWaitPriority(fixtureNow, fixtureNow.Add(-30*time.Second), policy); !priority || wait != 30_000 {
		t.Fatalf("aged demand priority=%t wait=%d; want true, 30000", priority, wait)
	}
}

func referencePolicy(hardRole bool) domain.MatchmakingPolicy {
	return domain.MatchmakingPolicy{
		Version:          "objective-v1",
		TeamCount:        2,
		TeamSize:         2,
		MaxLatencyMillis: 200,
		RoleRequirements: []domain.RoleRequirement{{Role: "healer", MinPerTeam: 1, Hard: hardRole}},
		RelaxationSteps: []domain.RelaxationStep{
			{AfterWait: 0, MaxTeamSkillGap: 50, MaxRolePenalty: 0},
			{AfterWait: 30 * time.Second, MaxTeamSkillGap: 200, MaxRolePenalty: 2, PrioritizeWait: true},
		},
	}
}

func dpsTeams(enqueuedAt time.Time) [][]domain.MatchTicket {
	return [][]domain.MatchTicket{
		{
			{ID: "ticket-a", Revision: 1, EnqueuedAt: enqueuedAt, Players: []domain.Player{{ID: "player-a", Skill: 1000, Role: "dps", LatencyMillis: 20}}},
			{ID: "ticket-b", Revision: 1, EnqueuedAt: enqueuedAt, Players: []domain.Player{{ID: "player-b", Skill: 1000, Role: "dps", LatencyMillis: 20}}},
		},
		{
			{ID: "ticket-c", Revision: 1, EnqueuedAt: enqueuedAt, Players: []domain.Player{{ID: "player-c", Skill: 1000, Role: "dps", LatencyMillis: 20}}},
			{ID: "ticket-d", Revision: 1, EnqueuedAt: enqueuedAt, Players: []domain.Player{{ID: "player-d", Skill: 1000, Role: "dps", LatencyMillis: 20}}},
		},
	}
}
