package simulation_test

import (
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/zrma/sema/internal/domain"
	"github.com/zrma/sema/internal/simulation"
)

var fixtureNow = time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC)

func TestRunIsDeterministicAndInputOrderIndependent(t *testing.T) {
	firstPolicy := simulationPolicy("policy-a", 2, 2)
	secondPolicy := simulationPolicy("policy-b", 2, 2)
	secondPolicy.MaxLatencyMillis = 150
	firstScenario := scenarioWithParties("scenario-a", []int{1, 1, 1, 1})
	secondScenario := scenarioWithParties("scenario-b", []int{1, 1, 1})

	first, err := simulation.Run(
		[]domain.MatchmakingPolicy{secondPolicy, firstPolicy},
		[]simulation.Scenario{secondScenario, firstScenario},
	)
	if err != nil {
		t.Fatal(err)
	}
	second, err := simulation.Run(
		[]domain.MatchmakingPolicy{firstPolicy, secondPolicy},
		[]simulation.Scenario{firstScenario, secondScenario},
	)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("simulation depends on input order: first=%#v second=%#v", first, second)
	}
	if len(first.Policies) != 2 || first.Policies[0].Version != "policy-a" || first.Policies[1].Version != "policy-b" {
		t.Fatalf("policy results are not canonical: %#v", first.Policies)
	}
	for _, result := range first.Policies {
		if result.Scenarios[0].ScenarioID != "scenario-a" || result.Scenarios[1].ScenarioID != "scenario-b" {
			t.Fatalf("scenario results are not canonical: %#v", result.Scenarios)
		}
	}
}

func TestRunRejectsPolicyConflictBeforeSimulation(t *testing.T) {
	first := simulationPolicy("shared-version", 2, 2)
	second := first
	second.MaxLatencyMillis++

	report, err := simulation.Run(
		[]domain.MatchmakingPolicy{first, second},
		[]simulation.Scenario{scenarioWithParties("conflict", []int{1, 1, 1, 1})},
	)
	if code, ok := domain.FailureCodeOf(err); !ok || code != domain.FailurePolicyConflict {
		t.Fatalf("policy conflict = %v; want %s", err, domain.FailurePolicyConflict)
	}
	if len(report.Policies) != 0 {
		t.Fatalf("conflicting policy produced a partial report: %#v", report)
	}
}

func TestRunRejectsInvalidPolicyBeforeSimulation(t *testing.T) {
	invalid := simulationPolicy("", 2, 2)
	report, err := simulation.Run(
		[]domain.MatchmakingPolicy{invalid},
		[]simulation.Scenario{scenarioWithParties("invalid-policy", []int{1, 1, 1, 1})},
	)
	if code, ok := domain.FailureCodeOf(err); !ok || code != domain.FailureInvalidInput {
		t.Fatalf("invalid policy = %v; want %s", err, domain.FailureInvalidInput)
	}
	if len(report.Policies) != 0 {
		t.Fatalf("invalid policy produced a partial report: %#v", report)
	}
}

func TestRunCoversReferenceCorpus(t *testing.T) {
	tests := []struct {
		name       string
		policy     domain.MatchmakingPolicy
		scenario   simulation.Scenario
		assertions func(*testing.T, simulation.ScenarioResult)
	}{
		{
			name:     "team-2v2",
			policy:   simulationPolicy("team-2v2-v1", 2, 2),
			scenario: scenarioWithParties("team-2v2", []int{1, 1, 1, 1}),
			assertions: func(t *testing.T, result simulation.ScenarioResult) {
				if result.Summary.ProposalCount != 1 || result.Summary.MatchedTicketCount != 4 {
					t.Fatalf("team summary = %#v", result.Summary)
				}
			},
		},
		{
			name:     "battle-royale-duo",
			policy:   simulationPolicy("battle-royale-duo-v1", 1, 100),
			scenario: scenarioWithParties("battle-royale-duo", repeatedPartySize(50, 2)),
			assertions: func(t *testing.T, result simulation.ScenarioResult) {
				if result.Summary.ProposalCount != 1 || result.Summary.MatchedTicketCount != 50 {
					t.Fatalf("battle royale summary = %#v", result.Summary)
				}
			},
		},
		{
			name:     "backfill",
			policy:   simulationPolicy("backfill-v1", 2, 2),
			scenario: backfillScenario(),
			assertions: func(t *testing.T, result simulation.ScenarioResult) {
				if len(result.Batch.Proposals) != 1 || result.Batch.Proposals[0].Kind != domain.ProposalBackfill {
					t.Fatalf("backfill result = %#v", result)
				}
			},
		},
		{
			name:     "no-match",
			policy:   simulationPolicy("no-match-v1", 2, 2),
			scenario: scenarioWithParties("no-match", []int{1, 1, 1}),
			assertions: func(t *testing.T, result simulation.ScenarioResult) {
				if result.Summary.ProposalCount != 0 || result.Summary.UnmatchedTicketCount != 3 || len(result.Summary.Unmatched) != 1 {
					t.Fatalf("no-match summary = %#v", result.Summary)
				}
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			report, err := simulation.Run(
				[]domain.MatchmakingPolicy{test.policy},
				[]simulation.Scenario{test.scenario},
			)
			if err != nil {
				t.Fatal(err)
			}
			test.assertions(t, report.Policies[0].Scenarios[0])
		})
	}
}

func simulationPolicy(version string, teamCount, teamSize int) domain.MatchmakingPolicy {
	return domain.MatchmakingPolicy{
		Version:                  version,
		TeamCount:                teamCount,
		TeamSize:                 teamSize,
		MaxLatencyMillis:         200,
		MaxSearchNodes:           100_000,
		MaxCandidatesPerProposal: 64,
	}
}

func scenarioWithParties(id domain.SnapshotID, partySizes []int) simulation.Scenario {
	tickets := make([]domain.MatchTicket, 0, len(partySizes))
	for ticketIndex, partySize := range partySizes {
		players := make([]domain.Player, 0, partySize)
		for playerIndex := range partySize {
			players = append(players, domain.Player{
				ID:            domain.PlayerID(fmt.Sprintf("%s-player-%02d-%02d", id, ticketIndex, playerIndex)),
				Skill:         1000 + ticketIndex%3,
				LatencyMillis: 20,
			})
		}
		tickets = append(tickets, domain.MatchTicket{
			ID:         domain.TicketID(fmt.Sprintf("%s-ticket-%02d", id, ticketIndex)),
			Revision:   1,
			EnqueuedAt: fixtureNow.Add(-time.Duration(len(partySizes)-ticketIndex) * time.Second),
			Players:    players,
		})
	}
	return simulation.Scenario{ID: id, Now: fixtureNow, MatchTickets: tickets}
}

func backfillScenario() simulation.Scenario {
	scenario := scenarioWithParties("backfill", []int{1, 1})
	scenario.BackfillTickets = []domain.BackfillTicket{{
		ID:              "backfill-demand",
		Revision:        1,
		SessionID:       "session-backfill",
		RosterVersion:   7,
		OpenSlotsByTeam: []int{1, 1},
		EnqueuedAt:      fixtureNow.Add(-time.Minute),
	}}
	return scenario
}

func repeatedPartySize(count, partySize int) []int {
	partySizes := make([]int, count)
	for index := range partySizes {
		partySizes[index] = partySize
	}
	return partySizes
}
