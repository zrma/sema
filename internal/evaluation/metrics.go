package evaluation

import (
	"time"

	"github.com/zrma/sema/internal/domain"
	"github.com/zrma/sema/internal/simulation"
)

const basisPoints = 10_000

// Metrics separates player coverage and queue fairness from proposal score evidence.
type Metrics struct {
	DemandTickets             int
	DemandPlayers             int
	MatchedTickets            int
	MatchedPlayers            int
	UnmatchedTickets          int
	UnmatchedPlayers          int
	CoverageBasisPoints       int
	OldestMatchedWaitMillis   int64
	OldestUnmatchedWaitMillis int64
	ProposalCount             int
	TotalRolePenalty          int
	MaxTeamSkillGap           int
	MaxLatencyMillis          int
	CandidateTickets          int
	TruncatedCandidateWindows int
	SearchNodes               int
	BudgetExhausted           bool
}

// Measure derives deterministic queue and quality metrics from a scenario and its batch.
func Measure(scenario simulation.Scenario, batch domain.ProposalBatch) Metrics {
	tickets := make(map[domain.TicketID]domain.MatchTicket, len(scenario.MatchTickets))
	metrics := Metrics{
		DemandTickets:    len(scenario.MatchTickets),
		UnmatchedTickets: len(batch.Unmatched),
		ProposalCount:    len(batch.Proposals),
		BudgetExhausted:  batch.BudgetExhausted,
	}
	for _, ticket := range scenario.MatchTickets {
		tickets[ticket.ID] = ticket
		metrics.DemandPlayers += len(ticket.Players)
	}
	for _, proposal := range batch.Proposals {
		metrics.TotalRolePenalty += proposal.Evidence.RolePenalty
		metrics.MaxTeamSkillGap = max(metrics.MaxTeamSkillGap, proposal.Evidence.TeamSkillGap)
		metrics.MaxLatencyMillis = max(metrics.MaxLatencyMillis, proposal.Evidence.MaxLatencyMillis)
		metrics.CandidateTickets += proposal.Evidence.CandidateTickets
		if proposal.Evidence.CandidateWindowTruncated {
			metrics.TruncatedCandidateWindows++
		}
		metrics.SearchNodes += proposal.Evidence.SearchNodes
		for _, reference := range proposal.Tickets {
			ticket, exists := tickets[reference.ID]
			if !exists {
				continue
			}
			metrics.MatchedTickets++
			metrics.MatchedPlayers += len(ticket.Players)
			metrics.OldestMatchedWaitMillis = max(metrics.OldestMatchedWaitMillis, waitMillis(scenario.Now, ticket))
		}
	}
	for _, unmatched := range batch.Unmatched {
		ticket, exists := tickets[unmatched.Ticket.ID]
		if !exists {
			continue
		}
		metrics.UnmatchedPlayers += len(ticket.Players)
		metrics.OldestUnmatchedWaitMillis = max(metrics.OldestUnmatchedWaitMillis, waitMillis(scenario.Now, ticket))
	}
	if metrics.DemandPlayers > 0 {
		metrics.CoverageBasisPoints = metrics.MatchedPlayers * basisPoints / metrics.DemandPlayers
	}
	return metrics
}

func waitMillis(now time.Time, ticket domain.MatchTicket) int64 {
	wait := now.Sub(ticket.EnqueuedAt).Milliseconds()
	return max(wait, 0)
}
