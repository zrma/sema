package planner

import (
	"testing"

	"github.com/zrma/sema/internal/domain"
)

func TestSelectProposalBatchBeatsGreedyWithHigherTotalUtility(t *testing.T) {
	candidates := []proposalCandidate{
		candidateFixture("a-top", 10, "a", "b"),
		candidateFixture("b-left", 6, "a", "c"),
		candidateFixture("c-right", 6, "b", "d"),
	}

	selection := selectProposalBatch(candidates, 2, 100)
	if selection.truncated {
		t.Fatal("exact fixture unexpectedly exhausted the selection budget")
	}
	if selection.totalUtility != 12 || len(selection.candidates) != 2 ||
		selection.candidates[0].key != "b-left" || selection.candidates[1].key != "c-right" {
		t.Fatalf("selection = %#v; want b-left + c-right with utility 12", selection)
	}
}

func TestRankCandidateUtilitiesValuesAdmittedMatchBeforeQualityRank(t *testing.T) {
	candidates := []proposalCandidate{
		candidateFixture("a", 0, "a", "b"),
		candidateFixture("b", 0, "a", "c"),
		candidateFixture("c", 0, "b", "d"),
	}

	rankCandidateUtilities(candidates, 2)
	minimum, maximum := candidates[0].utility, candidates[0].utility
	for _, candidate := range candidates[1:] {
		if candidate.utility < minimum {
			minimum = candidate.utility
		}
		if candidate.utility > maximum {
			maximum = candidate.utility
		}
	}
	if 2*minimum <= maximum {
		t.Fatalf("utilities=%#v; two admitted proposals must outrank any one proposal", candidates)
	}
}

func TestSelectProposalBatchTreatsMaximumAsUpperBound(t *testing.T) {
	candidates := []proposalCandidate{
		candidateFixture("a-top", 10, "a", "b"),
		candidateFixture("b-left", 4, "a", "c"),
		candidateFixture("c-right", 4, "b", "d"),
	}

	selection := selectProposalBatch(candidates, 2, 100)
	if selection.totalUtility != 10 || len(selection.candidates) != 1 || selection.candidates[0].key != "a-top" {
		t.Fatalf("selection = %#v; want one higher-utility proposal", selection)
	}
}

func TestSelectProposalBatchPreservesBackfillPriority(t *testing.T) {
	backfill := candidateFixture("backfill", 1, "a", "b")
	backfill.kind = domain.ProposalBackfill
	backfill.backfill = &domain.BackfillTarget{Ticket: domain.TicketRef{ID: "target", Revision: 1}}
	newMatch := candidateFixture("new-match", 100, "a", "c")

	selection := selectProposalBatch([]proposalCandidate{newMatch, backfill}, 1, 100)
	if len(selection.candidates) != 1 || selection.candidates[0].key != "backfill" || selection.selectedBackfills != 1 {
		t.Fatalf("selection = %#v; want the admissible backfill first", selection)
	}
}

func TestSelectProposalBatchReturnsGreedyIncumbentWhenTruncated(t *testing.T) {
	candidates := []proposalCandidate{
		candidateFixture("a-top", 10, "a", "b"),
		candidateFixture("b-left", 6, "a", "c"),
		candidateFixture("c-right", 6, "b", "d"),
	}

	selection := selectProposalBatch(candidates, 2, 1)
	if !selection.truncated || len(selection.candidates) == 0 {
		t.Fatalf("selection = %#v; want an explicit feasible incumbent", selection)
	}
}

func TestSelectProposalBatchMatchesExhaustiveOracle(t *testing.T) {
	candidates := []proposalCandidate{
		candidateFixture("a", 9, "a", "b"),
		candidateFixture("b", 6, "a", "c"),
		candidateFixture("c", 6, "b", "d"),
		candidateFixture("d", 5, "c", "e"),
		candidateFixture("e", 5, "d", "f"),
		candidateFixture("f", 3, "e", "g"),
		candidateFixture("g", 2, "f", "h"),
	}
	candidates[5].kind = domain.ProposalBackfill
	candidates[5].backfill = &domain.BackfillTarget{Ticket: domain.TicketRef{ID: "target", Revision: 1}}
	candidates[6].kind = domain.ProposalBackfill
	candidates[6].backfill = &domain.BackfillTarget{Ticket: domain.TicketRef{ID: "target", Revision: 1}}

	selection := selectProposalBatch(candidates, 3, 10_000)
	oracleBackfills, oracleUtility, oracleCount := exhaustiveBatchScore(candidates, 3)
	if selection.truncated {
		t.Fatal("small exact selection unexpectedly truncated")
	}
	if selection.selectedBackfills != oracleBackfills || selection.totalUtility != oracleUtility ||
		len(selection.candidates) != oracleCount {
		t.Fatalf("selection=(backfills=%d utility=%d count=%d), oracle=(%d %d %d)",
			selection.selectedBackfills, selection.totalUtility, len(selection.candidates),
			oracleBackfills, oracleUtility, oracleCount)
	}
}

func exhaustiveBatchScore(candidates []proposalCandidate, maxProposals int) (int, int64, int) {
	bestBackfills, bestUtility, bestCount := -1, int64(-1), -1
	for mask := 0; mask < 1<<len(candidates); mask++ {
		usedTickets := make(map[domain.TicketID]struct{})
		usedBackfills := make(map[domain.TicketID]struct{})
		backfills, count := 0, 0
		var utility int64
		valid := true
		for index, candidate := range candidates {
			if mask&(1<<index) == 0 {
				continue
			}
			count++
			if count > maxProposals || candidate.utility <= 0 ||
				!candidateAvailable(candidate, usedTickets, usedBackfills) {
				valid = false
				break
			}
			markCandidate(candidate, usedTickets, usedBackfills, true)
			utility += candidate.utility
			if candidate.kind == domain.ProposalBackfill {
				backfills++
			}
		}
		if !valid {
			continue
		}
		if backfills > bestBackfills ||
			(backfills == bestBackfills && utility > bestUtility) ||
			(backfills == bestBackfills && utility == bestUtility && count > bestCount) {
			bestBackfills, bestUtility, bestCount = backfills, utility, count
		}
	}
	return bestBackfills, bestUtility, bestCount
}

func candidateFixture(key string, utility int64, ticketIDs ...domain.TicketID) proposalCandidate {
	placement := make([][]domain.MatchTicket, 1)
	for _, id := range ticketIDs {
		placement[0] = append(placement[0], domain.MatchTicket{ID: id})
	}
	return proposalCandidate{
		placement: placement,
		kind:      domain.ProposalNewMatch,
		key:       key,
		utility:   utility,
	}
}
