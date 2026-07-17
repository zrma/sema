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
	f.Add(uint8(2), uint8(2), uint8(0), []byte{1, 1, 1, 1})
	f.Add(uint8(2), uint8(3), uint8(4), []byte{2, 1, 2, 1, 1})
	f.Add(uint8(1), uint8(5), uint8(8), []byte{4, 1, 2, 3, 1, 1})

	f.Fuzz(func(t *testing.T, teamCountSeed, teamSizeSeed, windowSeed uint8, data []byte) {
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
			MaxProposals:             1,
			MaxSearchNodes:           10_000,
			MaxCandidateTickets:      int(windowSeed % 13),
			MaxCandidatesPerProposal: 64,
		}
		snapshot := domain.MatchmakingSnapshot{
			ID: "fuzz", Now: fixtureNow, MatchTickets: tickets, Policy: configured,
		}
		original := cloneTickets(tickets)
		first, err := planner.Plan(snapshot)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(snapshot.MatchTickets, original) {
			t.Fatal("planner mutated snapshot tickets")
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
		assertDisjointAndCapacity(t, first, tickets, teamCount, teamSize)
		matched := 0
		for _, proposal := range first.Proposals {
			matched += len(proposal.Tickets)
		}
		if matched+len(first.Unmatched) != len(tickets) {
			t.Fatalf("ticket coverage: matched=%d unmatched=%d demand=%d", matched, len(first.Unmatched), len(tickets))
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
