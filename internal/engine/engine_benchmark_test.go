package engine_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/zrma/sema/internal/domain"
	"github.com/zrma/sema/internal/engine"
)

func BenchmarkEngineReferenceWorkloads(b *testing.B) {
	benchmarks := []struct {
		name       string
		policy     domain.MatchmakingPolicy
		partySizes []int
	}{
		{name: "2v2-solo", policy: testPolicy(2, 2), partySizes: repeatPartySize(4, 1)},
		{name: "50v50-solo", policy: testPolicy(2, 50), partySizes: repeatPartySize(100, 1)},
		{name: "battle-royale-duo", policy: testPolicy(1, 100), partySizes: repeatPartySize(50, 2)},
	}

	for _, benchmark := range benchmarks {
		b.Run(benchmark.name, func(b *testing.B) {
			tickets := benchmarkMatchTickets(benchmark.name, benchmark.partySizes)
			benchmarkEngineLifecycle(b, benchmark.policy, tickets)
		})
	}
}

func BenchmarkEngineQueueSizes(b *testing.B) {
	for _, queueSize := range []int{100, 500, 1000} {
		b.Run(fmt.Sprintf("5v5/queue-%d", queueSize), func(b *testing.B) {
			policy := testPolicy(2, 5)
			policy.MaxProposals = 1
			tickets := benchmarkMatchTickets(b.Name(), repeatPartySize(queueSize, 1))
			benchmarkEngineLifecycle(b, policy, tickets)
		})
	}
}

func benchmarkEngineLifecycle(
	b *testing.B,
	policy domain.MatchmakingPolicy,
	tickets []domain.MatchTicket,
) {
	b.Helper()
	b.ReportAllocs()
	b.ResetTimer()

	var proposals int
	var matchedTickets int
	var unmatchedTickets int
	unmatchedByReason := make(map[domain.UnmatchedReason]int)
	var searchNodes int
	var pendingAssignments int
	var exhaustedCycles int
	for range b.N {
		runtime, err := engine.New(time.Minute)
		if err != nil {
			b.Fatal(err)
		}
		if _, err := runtime.RegisterPolicy(policy); err != nil {
			b.Fatal(err)
		}
		for _, ticket := range tickets {
			if err := runtime.SubmitMatchTicket(ticket); err != nil {
				b.Fatal(err)
			}
		}

		batch, err := runtime.Plan(domain.SnapshotID("benchmark-"+b.Name()), fixtureNow, policy.Version)
		if err != nil {
			b.Fatal(err)
		}
		if batch.BudgetExhausted {
			exhaustedCycles++
		}
		proposals += len(batch.Proposals)
		unmatchedTickets += len(batch.Unmatched)
		for _, unmatched := range batch.Unmatched {
			unmatchedByReason[unmatched.Reason]++
		}
		for index, proposal := range batch.Proposals {
			matchedTickets += len(proposal.Tickets)
			searchNodes += proposal.Evidence.SearchNodes
			reservationID := domain.ReservationID(fmt.Sprintf("reservation-%d", index))
			assignmentID := domain.AssignmentID(fmt.Sprintf("assignment-%d", index))
			if _, err := runtime.Reserve(proposal, reservationID, fixtureNow); err != nil {
				b.Fatal(err)
			}
			assignment, err := runtime.Confirm(reservationID, assignmentID, fixtureNow.Add(time.Second))
			if err != nil {
				b.Fatal(err)
			}
			if assignment.Status != domain.AssignmentPending {
				b.Fatalf("assignment status = %q; want pending", assignment.Status)
			}
			pendingAssignments++
		}
	}

	operations := float64(b.N)
	b.ReportMetric(float64(proposals)/operations, "proposals/op")
	b.ReportMetric(float64(matchedTickets)/operations, "matched_tickets/op")
	b.ReportMetric(float64(unmatchedTickets)/operations, "unmatched_tickets/op")
	for _, reason := range []domain.UnmatchedReason{
		domain.UnmatchedHardConstraint,
		domain.UnmatchedInsufficientCapacity,
		domain.UnmatchedQualityThreshold,
		domain.UnmatchedSearchBudget,
		domain.UnmatchedProposalLimit,
	} {
		unit := fmt.Sprintf("unmatched_%s/op", reason)
		b.ReportMetric(float64(unmatchedByReason[reason])/operations, unit)
	}
	b.ReportMetric(float64(searchNodes)/operations, "search_nodes/op")
	b.ReportMetric(float64(pendingAssignments)/operations, "pending_assignments/op")
	b.ReportMetric(float64(exhaustedCycles)/operations, "budget_exhausted/op")
}

func benchmarkMatchTickets(prefix string, partySizes []int) []domain.MatchTicket {
	tickets := make([]domain.MatchTicket, 0, len(partySizes))
	for ticketIndex, partySize := range partySizes {
		players := make([]domain.Player, 0, partySize)
		for playerIndex := range partySize {
			players = append(players, domain.Player{
				ID:            domain.PlayerID(fmt.Sprintf("%s-player-%04d-%02d", prefix, ticketIndex, playerIndex)),
				Skill:         1000 + ticketIndex%5,
				LatencyMillis: 20,
			})
		}
		tickets = append(tickets, domain.MatchTicket{
			ID:         domain.TicketID(fmt.Sprintf("%s-ticket-%04d", prefix, ticketIndex)),
			Revision:   1,
			EnqueuedAt: fixtureNow.Add(-time.Duration(len(partySizes)-ticketIndex) * time.Second),
			Players:    players,
		})
	}
	return tickets
}

func repeatPartySize(count, partySize int) []int {
	partySizes := make([]int, count)
	for index := range partySizes {
		partySizes[index] = partySize
	}
	return partySizes
}
