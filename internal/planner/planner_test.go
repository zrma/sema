package planner_test

import (
	"fmt"
	"reflect"
	"slices"
	"testing"
	"time"

	"sema/internal/domain"
	"sema/internal/planner"
)

var fixtureNow = time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC)

func TestPlanReturnsDeterministicDisjointMatches(t *testing.T) {
	snapshot := snapshotWith("multi-match", policy(2, 2), partyTickets([]int{1, 1, 1, 1, 1, 1, 1, 1}))

	first, err := planner.Plan(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	reversed := snapshot
	reversed.MatchTickets = slices.Clone(snapshot.MatchTickets)
	slices.Reverse(reversed.MatchTickets)
	second, err := planner.Plan(reversed)
	if err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(first, second) {
		t.Fatalf("planning is not deterministic:\nfirst: %#v\nsecond: %#v", first, second)
	}
	if len(first.Proposals) != 2 || len(first.Unmatched) != 0 {
		t.Fatalf("proposals = %d, unmatched = %d; want 2, 0", len(first.Proposals), len(first.Unmatched))
	}
	assertDisjointAndCapacity(t, first, snapshot.MatchTickets, 2, 2)
}

func TestPlanPreservesParties(t *testing.T) {
	snapshot := snapshotWith("party-preservation", policy(2, 3), partyTickets([]int{2, 2, 1, 1}))

	batch, err := planner.Plan(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if len(batch.Proposals) != 1 {
		t.Fatalf("proposals = %d; want 1", len(batch.Proposals))
	}
	assertDisjointAndCapacity(t, batch, snapshot.MatchTickets, 2, 3)
}

func TestPlanCoversReferenceTeamWorkloads(t *testing.T) {
	for _, teamSize := range []int{2, 3, 5, 10, 16, 20, 50} {
		variants := map[string][]int{
			"all-solo":   repeatedPartySizes(teamSize*2, 1),
			"full-party": {teamSize, teamSize},
			"mixed-party": func() []int {
				if teamSize == 2 {
					return []int{2, 1, 1}
				}
				return []int{teamSize - 1, 1, teamSize - 1, 1}
			}(),
		}
		for name, sizes := range variants {
			t.Run(fmt.Sprintf("%dv%d/%s", teamSize, teamSize, name), func(t *testing.T) {
				tickets := partyTickets(sizes)
				batch, err := planner.Plan(snapshotWith(t.Name(), policy(2, teamSize), tickets))
				if err != nil {
					t.Fatal(err)
				}
				if len(batch.Proposals) != 1 || len(batch.Unmatched) != 0 {
					t.Fatalf("proposals = %d, unmatched = %d; want 1, 0", len(batch.Proposals), len(batch.Unmatched))
				}
				assertDisjointAndCapacity(t, batch, tickets, 2, teamSize)
			})
		}
	}
}

func TestPlanCoversBattleRoyalePartyEnvelope(t *testing.T) {
	for _, partySize := range []int{2, 4} {
		t.Run(fmt.Sprintf("party-%d", partySize), func(t *testing.T) {
			tickets := partyTickets(repeatedPartySizes(100/partySize, partySize))
			batch, err := planner.Plan(snapshotWith(t.Name(), policy(1, 100), tickets))
			if err != nil {
				t.Fatal(err)
			}
			if len(batch.Proposals) != 1 || len(batch.Unmatched) != 0 {
				t.Fatalf("proposals = %d, unmatched = %d; want 1, 0", len(batch.Proposals), len(batch.Unmatched))
			}
			assertDisjointAndCapacity(t, batch, tickets, 1, 100)
		})
	}
}

func TestPlanPrioritizesBackfill(t *testing.T) {
	snapshot := snapshotWith("backfill-first", policy(2, 2), partyTickets([]int{1, 1, 1, 1}))
	snapshot.BackfillTickets = []domain.BackfillTicket{
		{
			ID:              "backfill-a",
			Revision:        3,
			SessionID:       "session-a",
			RosterVersion:   7,
			OpenSlotsByTeam: []int{1, 1},
			EnqueuedAt:      fixtureNow.Add(-2 * time.Minute),
		},
	}

	batch, err := planner.Plan(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if len(batch.Proposals) != 1 || batch.Proposals[0].Kind != domain.ProposalBackfill {
		t.Fatalf("proposals = %#v; want one backfill", batch.Proposals)
	}
	if len(batch.Unmatched) != 2 {
		t.Fatalf("unmatched = %d; want 2", len(batch.Unmatched))
	}
	target := batch.Proposals[0].Backfill
	if target == nil || target.Ticket.Revision != 3 || target.RosterVersion != 7 {
		t.Fatalf("backfill target = %#v; freshness was not preserved", target)
	}
}

func TestPlanKeepsHardConstraintFailuresUnmatched(t *testing.T) {
	tickets := partyTickets([]int{3, 1, 1, 1})
	tickets[len(tickets)-1].Players[0].LatencyMillis = 201
	batch, err := planner.Plan(snapshotWith("hard-constraints", policy(2, 2), tickets))
	if err != nil {
		t.Fatal(err)
	}
	if len(batch.Proposals) != 0 || len(batch.Unmatched) != len(tickets) {
		t.Fatalf("proposals = %d, unmatched = %d; want 0, %d", len(batch.Proposals), len(batch.Unmatched), len(tickets))
	}
}

func TestPlanReportsSearchBudgetExhaustion(t *testing.T) {
	configured := policy(2, 2)
	configured.MaxSearchNodes = 1
	batch, err := planner.Plan(snapshotWith("bounded-search", configured, partyTickets([]int{1, 1, 1, 1})))
	if err != nil {
		t.Fatal(err)
	}
	if !batch.BudgetExhausted || len(batch.Proposals) != 0 || len(batch.Unmatched) != 4 {
		t.Fatalf("batch = %#v; want an explicit best-known no-match", batch)
	}
}

func BenchmarkPlanReferenceWorkloads(b *testing.B) {
	benchmarks := []struct {
		name    string
		policy  domain.MatchmakingPolicy
		parties []int
	}{
		{name: "2v2-solo", policy: policy(2, 2), parties: repeatedPartySizes(4, 1)},
		{name: "50v50-solo", policy: policy(2, 50), parties: repeatedPartySizes(100, 1)},
		{name: "battle-royale-duo", policy: policy(1, 100), parties: repeatedPartySizes(50, 2)},
	}
	for _, benchmark := range benchmarks {
		b.Run(benchmark.name, func(b *testing.B) {
			snapshot := snapshotWith(benchmark.name, benchmark.policy, partyTickets(benchmark.parties))
			b.ReportAllocs()
			for range b.N {
				if _, err := planner.Plan(snapshot); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func snapshotWith(id string, configured domain.MatchmakingPolicy, tickets []domain.MatchTicket) domain.MatchmakingSnapshot {
	return domain.MatchmakingSnapshot{
		ID:           domain.SnapshotID(id),
		Now:          fixtureNow,
		MatchTickets: tickets,
		Policy:       configured,
	}
}

func policy(teamCount, teamSize int) domain.MatchmakingPolicy {
	return domain.MatchmakingPolicy{
		Version:          "test-v1",
		TeamCount:        teamCount,
		TeamSize:         teamSize,
		MaxLatencyMillis: 200,
		MaxSearchNodes:   100_000,
	}
}

func partyTickets(sizes []int) []domain.MatchTicket {
	tickets := make([]domain.MatchTicket, len(sizes))
	playerSequence := 0
	for ticketIndex, size := range sizes {
		players := make([]domain.Player, size)
		for playerIndex := range players {
			players[playerIndex] = domain.Player{
				ID:            domain.PlayerID(fmt.Sprintf("player-%04d", playerSequence)),
				Skill:         1000 + playerSequence%7,
				LatencyMillis: 20 + playerSequence%5,
			}
			playerSequence++
		}
		tickets[ticketIndex] = domain.MatchTicket{
			ID:         domain.TicketID(fmt.Sprintf("ticket-%04d", ticketIndex)),
			Revision:   1,
			EnqueuedAt: fixtureNow.Add(-time.Duration(len(sizes)-ticketIndex) * time.Second),
			Players:    players,
		}
	}
	return tickets
}

func repeatedPartySizes(count, size int) []int {
	sizes := make([]int, count)
	for index := range sizes {
		sizes[index] = size
	}
	return sizes
}

func assertDisjointAndCapacity(
	t *testing.T,
	batch domain.ProposalBatch,
	tickets []domain.MatchTicket,
	teamCount int,
	teamSize int,
) {
	t.Helper()
	sizes := make(map[domain.TicketID]int, len(tickets))
	for _, ticket := range tickets {
		sizes[ticket.ID] = len(ticket.Players)
	}
	seen := make(map[domain.TicketID]struct{}, len(tickets))
	for _, proposal := range batch.Proposals {
		if err := domain.ValidateProposal(proposal); err != nil {
			t.Fatalf("invalid proposal: %v", err)
		}
		if len(proposal.Teams) != teamCount {
			t.Fatalf("teams = %d; want %d", len(proposal.Teams), teamCount)
		}
		for _, team := range proposal.Teams {
			players := 0
			for _, ref := range team.Tickets {
				if _, exists := seen[ref.ID]; exists {
					t.Fatalf("ticket %q appears more than once", ref.ID)
				}
				seen[ref.ID] = struct{}{}
				players += sizes[ref.ID]
			}
			if players != teamSize {
				t.Fatalf("team %d has %d players; want %d", team.Team, players, teamSize)
			}
		}
	}
}
