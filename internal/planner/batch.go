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

func selectProposalBatch(candidates []proposalCandidate, maxProposals, maxNodes int, preferPareto bool) batchSelection {
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
	if preferPareto {
		bestIndexes, nodes, truncated := paretoSelection(ordered, maxProposals, maxNodes)
		return materializeBatchSelection(ordered, bestIndexes, nodes, truncated)
	}

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
			if betterSelection(ordered, selected, score, bestIndexes, bestScore, false) {
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

	return materializeBatchSelection(ordered, bestIndexes, nodes, truncated)
}

// paretoSelection enumerates feasible subsets directly so conflicts terminate a
// branch without walking every excluded suffix candidate.
func paretoSelection(candidates []proposalCandidate, maxProposals, maxNodes int) ([]int, int, bool) {
	bestIndexes, bestScore := greedySelection(candidates, maxProposals)
	selected := make([]int, 0, maxProposals)
	usedTickets := make(map[domain.TicketID]struct{})
	usedBackfills := make(map[domain.TicketID]struct{})
	nodes := 0
	truncated := false

	var search func(int, int64)
	search = func(start int, score int64) {
		if nodes >= maxNodes {
			truncated = true
			return
		}
		nodes++
		if betterSelection(candidates, selected, score, bestIndexes, bestScore, true) {
			bestIndexes = slices.Clone(selected)
			bestScore = score
		}
		if len(selected) == maxProposals {
			return
		}
		for index := start; index < len(candidates); index++ {
			candidate := candidates[index]
			if candidate.utility <= 0 || !candidateAvailable(candidate, usedTickets, usedBackfills) {
				continue
			}
			markCandidate(candidate, usedTickets, usedBackfills, true)
			selected = append(selected, index)
			search(index+1, score+candidate.effectiveUtility)
			selected = selected[:len(selected)-1]
			markCandidate(candidate, usedTickets, usedBackfills, false)
			if truncated {
				return
			}
		}
	}
	search(0, 0)
	return bestIndexes, nodes, truncated
}

func materializeBatchSelection(
	ordered []proposalCandidate,
	bestIndexes []int,
	nodes int,
	truncated bool,
) batchSelection {
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
	preferPareto bool,
) bool {
	if preferPareto {
		leftQuality := selectedBatchQuality(candidates, left)
		rightQuality := selectedBatchQuality(candidates, right)
		if leftQuality.selectedBackfills != rightQuality.selectedBackfills {
			return leftQuality.selectedBackfills > rightQuality.selectedBackfills
		}
		if leftQuality.proposalCount != rightQuality.proposalCount {
			return leftQuality.proposalCount > rightQuality.proposalCount
		}
		if leftQuality.matchedPlayers != rightQuality.matchedPlayers {
			return leftQuality.matchedPlayers > rightQuality.matchedPlayers
		}
		if batchQualityDominates(leftQuality, rightQuality) {
			return true
		}
		if batchQualityDominates(rightQuality, leftQuality) {
			return false
		}
	}
	if leftScore != rightScore {
		return leftScore > rightScore
	}
	if len(left) != len(right) {
		return len(left) > len(right)
	}
	return selectionKey(candidates, left) < selectionKey(candidates, right)
}

type selectedQuality struct {
	selectedBackfills     int
	proposalCount         int
	matchedPlayers        int
	maxRolePenalty        int
	meanRolePenaltyMilli  int
	totalRolePenalty      int
	maxTeamSkillGap       int
	meanTeamSkillGapMilli int
	totalTeamSkillGap     int
	maxLatencyMillis      int
	oldestWaitMillis      int64
	meanWaitMillis        int64
	totalWaitMillis       int64
	matchedTickets        int
}

func selectedBatchQuality(candidates []proposalCandidate, indexes []int) selectedQuality {
	quality := selectedQuality{proposalCount: len(indexes)}
	for _, index := range indexes {
		candidate := candidates[index]
		if candidate.kind == domain.ProposalBackfill {
			quality.selectedBackfills++
		}
		quality.maxRolePenalty = max(quality.maxRolePenalty, candidate.evaluation.Evidence.RolePenalty)
		quality.totalRolePenalty += candidate.evaluation.Evidence.RolePenalty
		quality.maxTeamSkillGap = max(quality.maxTeamSkillGap, candidate.evaluation.Evidence.TeamSkillGap)
		quality.totalTeamSkillGap += candidate.evaluation.Evidence.TeamSkillGap
		quality.maxLatencyMillis = max(quality.maxLatencyMillis, candidate.evaluation.Evidence.MaxLatencyMillis)
		quality.oldestWaitMillis = max(quality.oldestWaitMillis, candidate.evaluation.Evidence.OldestWaitMillis)
		quality.totalWaitMillis += candidate.evaluation.Evidence.TotalWaitMillis
		for _, team := range candidate.placement {
			quality.matchedTickets += len(team)
			for _, ticket := range team {
				quality.matchedPlayers += len(ticket.Players)
			}
		}
	}
	if quality.proposalCount > 0 {
		quality.meanRolePenaltyMilli = quality.totalRolePenalty * 1_000 / quality.proposalCount
		quality.meanTeamSkillGapMilli = quality.totalTeamSkillGap * 1_000 / quality.proposalCount
	}
	if quality.matchedTickets > 0 {
		quality.meanWaitMillis = quality.totalWaitMillis / int64(quality.matchedTickets)
	}
	return quality
}

func batchQualityDominates(left, right selectedQuality) bool {
	noWorse := left.oldestWaitMillis >= right.oldestWaitMillis &&
		left.meanWaitMillis >= right.meanWaitMillis &&
		left.maxRolePenalty <= right.maxRolePenalty &&
		left.meanRolePenaltyMilli <= right.meanRolePenaltyMilli &&
		left.maxTeamSkillGap <= right.maxTeamSkillGap &&
		left.meanTeamSkillGapMilli <= right.meanTeamSkillGapMilli &&
		left.maxLatencyMillis <= right.maxLatencyMillis
	if !noWorse {
		return false
	}
	return left.oldestWaitMillis > right.oldestWaitMillis ||
		left.meanWaitMillis > right.meanWaitMillis ||
		left.maxRolePenalty < right.maxRolePenalty ||
		left.meanRolePenaltyMilli < right.meanRolePenaltyMilli ||
		left.maxTeamSkillGap < right.maxTeamSkillGap ||
		left.meanTeamSkillGapMilli < right.meanTeamSkillGapMilli ||
		left.maxLatencyMillis < right.maxLatencyMillis
}

func selectionKey(candidates []proposalCandidate, indexes []int) string {
	keys := make([]string, len(indexes))
	for index, candidateIndex := range indexes {
		keys[index] = candidates[candidateIndex].key
	}
	slices.Sort(keys)
	return strings.Join(keys, "\x00")
}
