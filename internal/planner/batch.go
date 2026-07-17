package planner

import (
	"cmp"
	"slices"
	"strings"

	"github.com/zrma/sema/internal/domain"
	"github.com/zrma/sema/internal/objective"
)

type proposalCandidate struct {
	placement        [][]domain.MatchTicket
	evaluation       objective.Evaluation
	kind             domain.ProposalKind
	backfill         *domain.BackfillTarget
	key              string
	utility          int64
	effectiveUtility int64
}

type batchSelection struct {
	candidates        []proposalCandidate
	totalUtility      int64
	selectedBackfills int
	nodes             int
	truncated         bool
}

func rankCandidateUtilities(candidates []proposalCandidate, maxProposals int) {
	slices.SortFunc(candidates, compareCandidateQuality)
	admissionUtility := int64(maxProposals)*int64(len(candidates)) + 1
	groupStart := 0
	for index := range candidates {
		if index > 0 && objective.Compare(candidates[index-1].evaluation, candidates[index].evaluation) != 0 {
			groupStart = index
		}
		candidates[index].utility = admissionUtility + int64(len(candidates)-groupStart)
	}
}

func compareCandidateQuality(left, right proposalCandidate) int {
	if result := objective.Compare(left.evaluation, right.evaluation); result != 0 {
		return result
	}
	return cmp.Compare(left.key, right.key)
}

func selectProposalBatch(candidates []proposalCandidate, maxProposals, maxNodes int) batchSelection {
	if maxProposals <= 0 || len(candidates) == 0 {
		return batchSelection{}
	}

	ordered := slices.Clone(candidates)
	maximumUtility := int64(0)
	for _, candidate := range ordered {
		if candidate.utility > maximumUtility {
			maximumUtility = candidate.utility
		}
	}
	backfillBonus := int64(maxProposals)*maximumUtility + 1
	for index := range ordered {
		ordered[index].effectiveUtility = ordered[index].utility
		if ordered[index].kind == domain.ProposalBackfill {
			ordered[index].effectiveUtility += backfillBonus
		}
	}
	slices.SortFunc(ordered, func(left, right proposalCandidate) int {
		if result := cmp.Compare(right.effectiveUtility, left.effectiveUtility); result != 0 {
			return result
		}
		return cmp.Compare(left.key, right.key)
	})

	bestIndexes, bestScore := greedySelection(ordered, maxProposals)
	selected := make([]int, 0, maxProposals)
	usedTickets := make(map[domain.TicketID]struct{})
	usedBackfills := make(map[domain.TicketID]struct{})
	nodes := 0
	truncated := false

	var search func(index int, score int64)
	search = func(index int, score int64) {
		if nodes >= maxNodes {
			truncated = true
			return
		}
		nodes++

		if index == len(ordered) || len(selected) == maxProposals {
			if betterSelection(ordered, selected, score, bestIndexes, bestScore) {
				bestIndexes = slices.Clone(selected)
				bestScore = score
			}
			return
		}
		if selectionUpperBound(ordered, index, maxProposals-len(selected), score) < bestScore {
			return
		}

		candidate := ordered[index]
		if candidate.utility > 0 && candidateAvailable(candidate, usedTickets, usedBackfills) {
			markCandidate(candidate, usedTickets, usedBackfills, true)
			selected = append(selected, index)
			search(index+1, score+candidate.effectiveUtility)
			selected = selected[:len(selected)-1]
			markCandidate(candidate, usedTickets, usedBackfills, false)
		}
		search(index+1, score)
	}
	search(0, 0)

	result := batchSelection{nodes: nodes, truncated: truncated}
	for _, index := range bestIndexes {
		candidate := ordered[index]
		result.candidates = append(result.candidates, candidate)
		result.totalUtility += candidate.utility
		if candidate.kind == domain.ProposalBackfill {
			result.selectedBackfills++
		}
	}
	slices.SortFunc(result.candidates, func(left, right proposalCandidate) int {
		if left.kind != right.kind {
			if left.kind == domain.ProposalBackfill {
				return -1
			}
			return 1
		}
		return compareCandidateQuality(left, right)
	})
	return result
}

func greedySelection(candidates []proposalCandidate, maxProposals int) ([]int, int64) {
	selected := make([]int, 0, maxProposals)
	usedTickets := make(map[domain.TicketID]struct{})
	usedBackfills := make(map[domain.TicketID]struct{})
	var score int64
	for index, candidate := range candidates {
		if len(selected) == maxProposals {
			break
		}
		if candidate.utility <= 0 || !candidateAvailable(candidate, usedTickets, usedBackfills) {
			continue
		}
		selected = append(selected, index)
		score += candidate.effectiveUtility
		markCandidate(candidate, usedTickets, usedBackfills, true)
	}
	return selected, score
}

func candidateAvailable(
	candidate proposalCandidate,
	usedTickets map[domain.TicketID]struct{},
	usedBackfills map[domain.TicketID]struct{},
) bool {
	for _, team := range candidate.placement {
		for _, ticket := range team {
			if _, used := usedTickets[ticket.ID]; used {
				return false
			}
		}
	}
	if candidate.backfill != nil {
		if _, used := usedBackfills[candidate.backfill.Ticket.ID]; used {
			return false
		}
	}
	return true
}

func markCandidate(
	candidate proposalCandidate,
	usedTickets map[domain.TicketID]struct{},
	usedBackfills map[domain.TicketID]struct{},
	selected bool,
) {
	for _, team := range candidate.placement {
		for _, ticket := range team {
			if selected {
				usedTickets[ticket.ID] = struct{}{}
			} else {
				delete(usedTickets, ticket.ID)
			}
		}
	}
	if candidate.backfill == nil {
		return
	}
	if selected {
		usedBackfills[candidate.backfill.Ticket.ID] = struct{}{}
	} else {
		delete(usedBackfills, candidate.backfill.Ticket.ID)
	}
}

func selectionUpperBound(candidates []proposalCandidate, index, remainingSlots int, score int64) int64 {
	for index < len(candidates) && remainingSlots > 0 {
		if candidates[index].utility > 0 {
			score += candidates[index].effectiveUtility
			remainingSlots--
		}
		index++
	}
	return score
}

func betterSelection(
	candidates []proposalCandidate,
	left []int,
	leftScore int64,
	right []int,
	rightScore int64,
) bool {
	if leftScore != rightScore {
		return leftScore > rightScore
	}
	if len(left) != len(right) {
		return len(left) > len(right)
	}
	return selectionKey(candidates, left) < selectionKey(candidates, right)
}

func selectionKey(candidates []proposalCandidate, indexes []int) string {
	keys := make([]string, len(indexes))
	for index, candidateIndex := range indexes {
		keys[index] = candidates[candidateIndex].key
	}
	slices.Sort(keys)
	return strings.Join(keys, "\x00")
}
