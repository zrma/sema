package planner_test

import (
	"fmt"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/zrma/sema/internal/domain"
	"github.com/zrma/sema/internal/planner"
)

func FuzzPlanInvariants(f *testing.F) {
	f.Add(uint8(2), uint8(2), uint8(0), uint8(2), false, []byte{1, 1, 1, 1})
	f.Add(uint8(2), uint8(3), uint8(4), uint8(3), true, []byte{2, 1, 2, 1, 1})
	f.Add(uint8(1), uint8(5), uint8(8), uint8(1), true, []byte{4, 1, 2, 3, 1, 1})

	f.Fuzz(func(t *testing.T, teamCountSeed, teamSizeSeed, windowSeed, proposalSeed uint8, withBackfill bool, data []byte) {
		teamCount := 1 + int(teamCountSeed%2)
		teamSize := 1 + int(teamSizeSeed%5)
		if len(data) > 20 {
			data = data[:20]
		}
		tickets := fuzzTickets(data, teamSize)
		configured := domain.MatchmakingPolicy{
			Version:                  "fuzz-v1",
			TeamCount:                teamCount,
			TeamSize:                 teamSize,
			MaxLatencyMillis:         200,
			MaxProposals:             1 + int(proposalSeed%4),
			MaxSearchNodes:           10_000,
			MaxCandidateTickets:      int(windowSeed % 13),
			MaxCandidatesPerProposal: 64,
		}
		snapshot := domain.MatchmakingSnapshot{
			ID: "fuzz", Now: fixtureNow, MatchTickets: tickets, Policy: configured,
		}
		if withBackfill {
			slots := make([]int, teamCount)
			existing := make([]domain.RosterTeamSummary, teamCount)
			for team := range teamCount {
				slots[team] = 1
				existing[team] = domain.RosterTeamSummary{
					PlayerCount: teamSize - 1, SkillTotal: (teamSize - 1) * (1_000 + team*100),
					RoleCounts: []domain.RoleCount{{Role: "dps", Count: teamSize - 1}}, MaxLatencyMillis: 40 + team,
				}
			}
			snapshot.BackfillTickets = []domain.BackfillTicket{{
				ID: "fuzz-backfill", Revision: 1, SessionID: "fuzz-session", RosterVersion: 1,
				OpenSlotsByTeam: slots, ExistingTeams: existing, EnqueuedAt: fixtureNow.Add(-time.Minute),
			}}
		}
		original := cloneTickets(tickets)
		originalBackfills := cloneBackfills(snapshot.BackfillTickets)
		first, err := planner.Plan(snapshot)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(snapshot.MatchTickets, original) {
			t.Fatal("planner mutated snapshot tickets")
		}
		if !reflect.DeepEqual(snapshot.BackfillTickets, originalBackfills) {
			t.Fatal("planner mutated snapshot backfills")
		}

		reversed := snapshot
		reversed.MatchTickets = slices.Clone(snapshot.MatchTickets)
		slices.Reverse(reversed.MatchTickets)
		second, err := planner.Plan(reversed)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(first, second) {
			t.Fatalf("input order changed plan: first=%#v second=%#v", first, second)
		}
		assertFuzzDisjointAndCapacity(t, first, tickets, snapshot.BackfillTickets, teamCount, teamSize)
		matched := 0
		for _, proposal := range first.Proposals {
			matched += len(proposal.Tickets)
		}
		if matched+len(first.Unmatched) != len(tickets) {
			t.Fatalf("ticket coverage: matched=%d unmatched=%d demand=%d", matched, len(first.Unmatched), len(tickets))
		}
		truncated := first.Evidence.CandidateGenerationTruncated || first.Evidence.SelectionTruncated
		if first.BudgetExhausted != truncated {
			t.Fatalf("budget evidence = %#v, exhausted=%t", first.Evidence, first.BudgetExhausted)
		}
	})
}

func fuzzTickets(data []byte, teamSize int) []domain.MatchTicket {
	tickets := make([]domain.MatchTicket, len(data))
	playerSequence := 0
	for ticketIndex, value := range data {
		partySize := 1 + int(value%byte(teamSize))
		players := make([]domain.Player, partySize)
		for playerIndex := range players {
			players[playerIndex] = domain.Player{
				ID:            domain.PlayerID(fmt.Sprintf("fuzz-player-%03d", playerSequence)),
				Skill:         int(value)*10 + playerIndex,
				LatencyMillis: 20 + int(value%100),
			}
			playerSequence++
		}
		tickets[ticketIndex] = domain.MatchTicket{
			ID:         domain.TicketID(fmt.Sprintf("fuzz-ticket-%03d", ticketIndex)),
			Revision:   1,
			EnqueuedAt: fixtureNow.Add(-time.Duration(1+value%60) * time.Second),
			Players:    players,
		}
	}
	return tickets
}

func cloneTickets(tickets []domain.MatchTicket) []domain.MatchTicket {
	cloned := make([]domain.MatchTicket, len(tickets))
	for index, ticket := range tickets {
		cloned[index] = domain.CloneMatchTicket(ticket)
	}
	return cloned
}

func cloneBackfills(tickets []domain.BackfillTicket) []domain.BackfillTicket {
	if tickets == nil {
		return nil
	}
	cloned := make([]domain.BackfillTicket, len(tickets))
	for index, ticket := range tickets {
		cloned[index] = domain.CloneBackfillTicket(ticket)
	}
	return cloned
}

func assertFuzzDisjointAndCapacity(
	t *testing.T,
	batch domain.ProposalBatch,
	tickets []domain.MatchTicket,
	backfills []domain.BackfillTicket,
	teamCount int,
	teamSize int,
) {
	t.Helper()
	sizes := make(map[domain.TicketID]int, len(tickets))
	for _, ticket := range tickets {
		sizes[ticket.ID] = len(ticket.Players)
	}
	backfillByID := make(map[domain.TicketID]domain.BackfillTicket, len(backfills))
	for _, backfill := range backfills {
		backfillByID[backfill.ID] = backfill
	}
	seenTickets := make(map[domain.TicketID]struct{})
	seenBackfills := make(map[domain.TicketID]struct{})
	for _, proposal := range batch.Proposals {
		if err := domain.ValidateProposal(proposal); err != nil {
			t.Fatalf("invalid proposal: %v", err)
		}
		if len(proposal.Teams) != teamCount {
			t.Fatalf("teams = %d; want %d", len(proposal.Teams), teamCount)
		}
		var target *domain.BackfillTicket
		if proposal.Backfill != nil {
			backfill, exists := backfillByID[proposal.Backfill.Ticket.ID]
			if !exists {
				t.Fatalf("proposal references unknown backfill %q", proposal.Backfill.Ticket.ID)
			}
			target = &backfill
			if _, duplicate := seenBackfills[backfill.ID]; duplicate {
				t.Fatalf("backfill %q appears more than once", backfill.ID)
			}
			seenBackfills[backfill.ID] = struct{}{}
		}
		for _, team := range proposal.Teams {
			if team.Team < 0 || team.Team >= teamCount {
				t.Fatalf("team index %d is outside [0,%d)", team.Team, teamCount)
			}
			players := 0
			for _, reference := range team.Tickets {
				size, exists := sizes[reference.ID]
				if !exists {
					t.Fatalf("proposal references unknown ticket %q", reference.ID)
				}
				if _, duplicate := seenTickets[reference.ID]; duplicate {
					t.Fatalf("ticket %q appears more than once", reference.ID)
				}
				seenTickets[reference.ID] = struct{}{}
				players += size
			}
			expected := teamSize
			if target != nil {
				expected = target.OpenSlotsByTeam[team.Team]
			}
			if players != expected {
				t.Fatalf("team %d has %d players; want %d", team.Team, players, expected)
			}
		}
	}
}
