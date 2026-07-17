// Package planner implements the side-effect-free deterministic matching core.
package planner

import (
	"cmp"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/zrma/sema/internal/constraint"
	"github.com/zrma/sema/internal/discovery"
	"github.com/zrma/sema/internal/domain"
	"github.com/zrma/sema/internal/objective"
)

const (
	defaultMaxProposals             = 64
	defaultMaxSearchNodes           = 100_000
	defaultMaxCandidatesPerProposal = 64
	defaultMaxBatchCandidates       = 256
	defaultMaxBatchSearchNodes      = 100_000
	// The expanded path shares the exhaustive evaluator's small-snapshot boundary.
	smallQueueTicketLimit    = 12
	smallQueueBackfillLimit  = 2
	smallQueueTeamLimit      = 2
	smallQueueCandidateLimit = 4_096
)

// Plan returns a deterministic set of mutually disjoint proposals.
func Plan(snapshot domain.MatchmakingSnapshot) (domain.ProposalBatch, error) {
	if err := domain.ValidateSnapshot(snapshot); err != nil {
		return domain.ProposalBatch{}, err
	}
	policyFingerprint, err := domain.FingerprintPolicy(snapshot.Policy)
	if err != nil {
		return domain.ProposalBatch{}, err
	}

	available := make([]domain.MatchTicket, 0, len(snapshot.MatchTickets))
	hardRejected := make([]domain.MatchTicket, 0)
	for _, ticket := range snapshot.MatchTickets {
		if constraint.TicketAllowed(ticket, snapshot.Policy.TeamSize, snapshot.Policy.MaxLatencyMillis) {
			available = append(available, domain.CloneMatchTicket(ticket))
		} else {
			hardRejected = append(hardRejected, domain.CloneMatchTicket(ticket))
		}
	}
	sortTickets(available)

	backfills := make([]domain.BackfillTicket, len(snapshot.BackfillTickets))
	for index, ticket := range snapshot.BackfillTickets {
		backfills[index] = domain.CloneBackfillTicket(ticket)
	}
	sort.Slice(backfills, func(left, right int) bool {
		return compareQueueOrder(backfills[left].EnqueuedAt, string(backfills[left].ID), backfills[right].EnqueuedAt, string(backfills[right].ID)) < 0
	})

	maxProposals := snapshot.Policy.MaxProposals
	if maxProposals == 0 {
		maxProposals = defaultMaxProposals
	}
	maxSearchNodes := snapshot.Policy.MaxSearchNodes
	if maxSearchNodes == 0 {
		maxSearchNodes = defaultMaxSearchNodes
	}
	// Explicit policy limits always win; only zero-value defaults opt into the
	// wider correctness path for snapshots that remain cheap to compare exhaustively.
	smallQueueExpandedSearch := snapshot.Policy.MaxCandidatesPerProposal == 0 &&
		snapshot.Policy.MaxBatchCandidates == 0 &&
		len(available) <= smallQueueTicketLimit &&
		len(backfills) <= smallQueueBackfillLimit &&
		snapshot.Policy.TeamCount <= smallQueueTeamLimit
	maxCandidates := snapshot.Policy.MaxCandidatesPerProposal
	if maxCandidates == 0 {
		maxCandidates = defaultMaxCandidatesPerProposal
		if smallQueueExpandedSearch {
			maxCandidates = smallQueueCandidateLimit
		}
	}
	maxBatchCandidates := snapshot.Policy.MaxBatchCandidates
	if maxBatchCandidates == 0 {
		maxBatchCandidates = defaultMaxBatchCandidates
		if smallQueueExpandedSearch {
			maxBatchCandidates = smallQueueCandidateLimit
		}
	}
	maxBatchSearchNodes := snapshot.Policy.MaxBatchSearchNodes
	if maxBatchSearchNodes == 0 {
		maxBatchSearchNodes = defaultMaxBatchSearchNodes
	}

	generationBudget := searchBudget{max: maxSearchNodes}
	generation := generateProposalCandidates(
		available,
		backfills,
		snapshot,
		maxCandidates,
		maxBatchCandidates,
		maxProposals,
		snapshot.Policy.MaxCandidateTickets,
		&generationBudget,
		smallQueueExpandedSearch,
	)
	rankCandidateUtilities(generation.candidates, maxProposals)
	selection := selectProposalBatch(generation.candidates, maxProposals, maxBatchSearchNodes, smallQueueExpandedSearch)
	priorityEligible, oldestPriority := priorityDemandEvidence(generation.candidates)
	prioritySelected, oldestSelectedPriority := priorityDemandEvidence(selection.candidates)

	batch := domain.ProposalBatch{
		SnapshotID:      snapshot.ID,
		BudgetExhausted: generation.truncated || selection.truncated,
		Evidence: domain.BatchScoreEvidence{
			CandidateProposals:           len(generation.candidates),
			SelectedProposals:            len(selection.candidates),
			SelectedBackfills:            selection.selectedBackfills,
			WaitPriorityEligibleDemands:  priorityEligible,
			WaitPrioritySelectedDemands:  prioritySelected,
			OldestWaitPriorityMillis:     oldestPriority,
			OldestSelectedPriorityMillis: oldestSelectedPriority,
			TotalUtility:                 selection.totalUtility,
			CandidateGenerationNodes:     generationBudget.used,
			CandidateGenerationTruncated: generation.truncated,
			SelectionNodes:               selection.nodes,
			SelectionTruncated:           selection.truncated,
		},
	}
	selectedTickets := make(map[domain.TicketID]struct{})
	for index, candidate := range selection.candidates {
		candidate.evaluation.Evidence.SelectionUtility = candidate.utility
		search := placementSearch{
			placement: candidate.placement, evaluation: candidate.evaluation, found: true,
		}
		batch.Proposals = append(batch.Proposals, buildProposal(
			snapshot,
			search,
			index+1,
			policyFingerprint,
			candidate.kind,
			candidate.backfill,
		))
		for _, team := range candidate.placement {
			for _, ticket := range team {
				selectedTickets[ticket.ID] = struct{}{}
			}
		}
	}

	remaining := make([]domain.MatchTicket, 0, len(available))
	for _, ticket := range available {
		if _, selected := selectedTickets[ticket.ID]; !selected {
			remaining = append(remaining, ticket)
		}
	}
	reason := batchUnmatchedReason(len(batch.Proposals), maxProposals, generation, selection)
	reasons := make(map[domain.TicketID]domain.UnmatchedReason, len(remaining)+len(hardRejected))
	for _, ticket := range remaining {
		reasons[ticket.ID] = reason
	}
	for _, ticket := range hardRejected {
		reasons[ticket.ID] = domain.UnmatchedHardConstraint
	}
	remaining = append(remaining, hardRejected...)
	sortTickets(remaining)
	batch.Unmatched = make([]domain.UnmatchedTicket, len(remaining))
	for index, ticket := range remaining {
		batch.Unmatched[index] = domain.UnmatchedTicket{
			Ticket: domain.TicketReference(ticket),
			Reason: reasons[ticket.ID],
		}
	}
	return batch, nil
}

type candidateGeneration struct {
	candidates        []proposalCandidate
	truncated         bool
	sawHardFailure    bool
	sawQualityFailure bool
}

func generateProposalCandidates(
	available []domain.MatchTicket,
	backfills []domain.BackfillTicket,
	snapshot domain.MatchmakingSnapshot,
	maxCandidatesPerSearch int,
	maxBatchCandidates int,
	maxSeedProposals int,
	maxCandidateTickets int,
	budget *searchBudget,
	expandShapeAlternatives bool,
) candidateGeneration {
	result := candidateGeneration{}
	indexes := make(map[string]int)
	singleSelectFastPath := maxCandidatesPerSearch >= defaultMaxCandidatesPerProposal &&
		(maxSeedProposals == 1 || len(backfills) == 0 && !canFillMultipleProposals(available, matchSlots(snapshot.Policy)))
	shapeAlternatives := 1
	if expandShapeAlternatives && !singleSelectFastPath {
		shapeAlternatives = maxCandidatesPerSearch
	}
	for _, backfill := range backfills {
		if budget.exhausted || len(result.candidates) >= maxBatchCandidates {
			result.truncated = true
			break
		}
		window := discovery.SelectWindow(available, backfill.OpenSlotsByTeam, maxCandidateTickets)
		target := domain.BackfillReference(backfill)
		collectShapeCandidates(
			&result, indexes, window.Tickets, backfill.OpenSlotsByTeam, snapshot,
			domain.ProposalBackfill, &target, backfill.EnqueuedAt, window.Truncated,
			!singleSelectFastPath,
			maxCandidatesPerSearch, shapeAlternatives, maxBatchCandidates, budget,
		)
	}

	newMatchSlots := matchSlots(snapshot.Policy)
	if !budget.exhausted && len(result.candidates) < maxBatchCandidates {
		collectGreedyCoverCandidates(
			&result, indexes, available, newMatchSlots, snapshot,
			maxCandidatesPerSearch, maxBatchCandidates, maxSeedProposals,
			maxCandidateTickets, budget,
		)
	}
	needsBatchAlternatives := !singleSelectFastPath
	if needsBatchAlternatives && !budget.exhausted && len(result.candidates) < maxBatchCandidates {
		window := discovery.SelectWindow(available, newMatchSlots, maxCandidateTickets)
		collectShapeCandidates(
			&result, indexes, window.Tickets, newMatchSlots, snapshot,
			domain.ProposalNewMatch, nil, time.Time{}, window.Truncated,
			true,
			maxCandidatesPerSearch, shapeAlternatives, maxBatchCandidates, budget,
		)
	} else if needsBatchAlternatives && len(available) > 0 {
		result.truncated = true
	}
	result.truncated = result.truncated || budget.exhausted
	return result
}

func matchSlots(policy domain.MatchmakingPolicy) []int {
	slots := make([]int, policy.TeamCount)
	for index := range slots {
		slots[index] = policy.TeamSize
	}
	return slots
}

func collectGreedyCoverCandidates(
	result *candidateGeneration,
	indexes map[string]int,
	available []domain.MatchTicket,
	slots []int,
	snapshot domain.MatchmakingSnapshot,
	maxCandidatesPerSearch int,
	maxBatchCandidates int,
	maxSeedProposals int,
	maxCandidateTickets int,
	budget *searchBudget,
) {
	remaining := slices.Clone(available)
	for seed := 0; seed < maxSeedProposals && len(result.candidates) < maxBatchCandidates && !budget.exhausted; seed++ {
		window := discovery.SelectWindow(remaining, slots, maxCandidateTickets)
		search := findBestPlacement(
			window.Tickets, slots, snapshot, domain.ProposalNewMatch, "", maxCandidatesPerSearch, 1, budget,
		)
		prepareSearchEvidence(&search, len(window.Tickets), window.Truncated)
		recordGeneratedCandidate(result, indexes, search, snapshot, domain.ProposalNewMatch, nil, time.Time{})
		if !search.found {
			return
		}
		remaining = removePlaced(remaining, search.placement)
	}
	if budget.exhausted || len(result.candidates) >= maxBatchCandidates {
		result.truncated = true
	}
}

func collectShapeCandidates(
	result *candidateGeneration,
	indexes map[string]int,
	tickets []domain.MatchTicket,
	slots []int,
	snapshot domain.MatchmakingSnapshot,
	kind domain.ProposalKind,
	backfill *domain.BackfillTarget,
	backfillEnqueuedAt time.Time,
	windowTruncated bool,
	includeAnchors bool,
	maxCandidatesPerSearch int,
	maxAlternativesPerSearch int,
	maxBatchCandidates int,
	budget *searchBudget,
) {
	anchors := make([]domain.TicketID, 1, len(tickets)+1)
	if includeAnchors {
		for _, ticket := range tickets {
			anchors = append(anchors, ticket.ID)
		}
	}
	for anchorIndex, anchor := range anchors {
		if budget.exhausted || len(result.candidates) >= maxBatchCandidates {
			if anchorIndex < len(anchors) {
				result.truncated = true
			}
			return
		}
		search := findBestPlacement(
			tickets, slots, snapshot, kind, anchor, maxCandidatesPerSearch,
			min(maxAlternativesPerSearch, maxBatchCandidates-len(result.candidates)), budget,
		)
		alternatives := expandPlacementAlternatives(search)
		if len(alternatives) == 0 {
			alternatives = []placementSearch{search}
		}
		for _, alternative := range alternatives {
			prepareSearchEvidence(&alternative, len(tickets), windowTruncated)
			recordGeneratedCandidate(result, indexes, alternative, snapshot, kind, backfill, backfillEnqueuedAt)
			if len(result.candidates) >= maxBatchCandidates {
				break
			}
		}
	}
}

func canFillMultipleProposals(tickets []domain.MatchTicket, slots []int) bool {
	needed := 0
	for _, count := range slots {
		needed += count
	}
	if needed == 0 {
		return false
	}
	available := 0
	for _, ticket := range tickets {
		available += len(ticket.Players)
	}
	return available >= 2*needed
}

func prepareSearchEvidence(search *placementSearch, candidateTickets int, windowTruncated bool) {
	search.candidateWindowTruncated = windowTruncated
	search.evaluation.Evidence.CandidateTickets = candidateTickets
	search.evaluation.Evidence.CandidateWindowTruncated = windowTruncated
	search.evaluation.Evidence.SearchTruncated = search.evaluation.Evidence.SearchTruncated || windowTruncated
}

func recordGeneratedCandidate(
	result *candidateGeneration,
	indexes map[string]int,
	search placementSearch,
	snapshot domain.MatchmakingSnapshot,
	kind domain.ProposalKind,
	backfill *domain.BackfillTarget,
	backfillEnqueuedAt time.Time,
) {
	result.truncated = result.truncated || search.truncated || search.candidateWindowTruncated
	result.sawHardFailure = result.sawHardFailure || search.sawHardFailure
	result.sawQualityFailure = result.sawQualityFailure || search.sawQualityFailure
	if !search.found {
		return
	}
	candidate := proposalCandidate{
		placement:  search.placement,
		evaluation: search.evaluation,
		kind:       kind,
		backfill:   domain.CloneBackfillTarget(backfill),
	}
	if len(search.evaluation.PriorityWaitMillis) > 0 {
		candidate.priorityDemands = placementPriorityDemands(snapshot, search.placement)
	}
	if backfill != nil {
		priority, waitMillis := objective.TicketWaitPriority(snapshot.Now, backfillEnqueuedAt, snapshot.Policy)
		candidate.evaluation.Evidence.OldestWaitMillis = max(candidate.evaluation.Evidence.OldestWaitMillis, waitMillis)
		candidate.evaluation.Evidence.TotalWaitMillis += waitMillis
		if priority {
			candidate.evaluation.Evidence.WaitPriority = true
			candidate.priorityDemands = append(candidate.priorityDemands, priorityDemand{
				key: "backfill:" + string(backfill.Ticket.ID), waitMillis: waitMillis,
			})
		}
	}
	slices.SortFunc(candidate.priorityDemands, comparePriorityDemand)
	candidate.key = canonicalCandidateKey(candidate)
	if existingIndex, exists := indexes[candidate.key]; exists {
		existing := result.candidates[existingIndex]
		if objective.Compare(candidate.evaluation, existing.evaluation) < 0 ||
			(objective.Compare(candidate.evaluation, existing.evaluation) == 0 &&
				comparePlacement(candidate.placement, existing.placement) < 0) {
			result.candidates[existingIndex] = candidate
		}
		return
	}
	indexes[candidate.key] = len(result.candidates)
	result.candidates = append(result.candidates, candidate)
}

func placementPriorityDemands(snapshot domain.MatchmakingSnapshot, placement [][]domain.MatchTicket) []priorityDemand {
	demands := make([]priorityDemand, 0)
	for _, team := range placement {
		for _, ticket := range team {
			if priority, waitMillis := objective.TicketWaitPriority(snapshot.Now, ticket.EnqueuedAt, snapshot.Policy); priority {
				demands = append(demands, priorityDemand{
					key: "match:" + string(ticket.ID), waitMillis: waitMillis,
				})
			}
		}
	}
	return demands
}

func canonicalCandidateKey(candidate proposalCandidate) string {
	references := make([]domain.TicketRef, 0)
	for _, team := range candidate.placement {
		for _, ticket := range team {
			references = append(references, domain.TicketReference(ticket))
		}
	}
	slices.SortFunc(references, func(left, right domain.TicketRef) int {
		if result := cmp.Compare(left.ID, right.ID); result != 0 {
			return result
		}
		return cmp.Compare(left.Revision, right.Revision)
	})
	key := string(candidate.kind)
	if candidate.backfill != nil {
		key += fmt.Sprintf("|%s@%d|%s@%d", candidate.backfill.Ticket.ID, candidate.backfill.Ticket.Revision,
			candidate.backfill.SessionID, candidate.backfill.RosterVersion)
	}
	for _, reference := range references {
		key += fmt.Sprintf("|%s@%d", reference.ID, reference.Revision)
	}
	return key
}

type searchBudget struct {
	max       int
	used      int
	exhausted bool
}

func (budget *searchBudget) visit() bool {
	if budget.used >= budget.max {
		budget.exhausted = true
		return false
	}
	budget.used++
	return true
}

type placementSearch struct {
	placement                [][]domain.MatchTicket
	evaluation               objective.Evaluation
	found                    bool
	exactCandidates          int
	searchNodes              int
	truncated                bool
	candidateWindowTruncated bool
	sawHardFailure           bool
	sawQualityFailure        bool
	alternatives             []placementAlternative
}

type placementAlternative struct {
	placement  [][]domain.MatchTicket
	evaluation objective.Evaluation
}

func findBestPlacement(
	tickets []domain.MatchTicket,
	slots []int,
	snapshot domain.MatchmakingSnapshot,
	kind domain.ProposalKind,
	requiredTicket domain.TicketID,
	maxCandidates int,
	maxAlternatives int,
	budget *searchBudget,
) placementSearch {
	startNodes := budget.used
	result := placementSearch{}
	remaining := slices.Clone(slots)
	totalNeeded := 0
	for _, count := range remaining {
		totalNeeded += count
	}
	if totalNeeded == 0 {
		return result
	}

	suffixPlayers := make([]int, len(tickets)+1)
	for index := len(tickets) - 1; index >= 0; index-- {
		suffixPlayers[index] = suffixPlayers[index+1] + len(tickets[index].Players)
	}
	if suffixPlayers[0] < totalNeeded {
		return result
	}
	requiredIndex := -1
	if requiredTicket != "" {
		for index, ticket := range tickets {
			if ticket.ID == requiredTicket {
				requiredIndex = index
				break
			}
		}
		if requiredIndex < 0 {
			return result
		}
	}

	assigned := make([][]int, len(slots))
	teamSkills := make([]int, len(slots))
	var alternativeIndexes map[string]int
	if maxAlternatives > 1 {
		alternativeIndexes = make(map[string]int)
	}
	var search func(index int, needed int) bool
	search = func(index int, needed int) bool {
		if !budget.visit() {
			return true
		}
		if needed == 0 {
			if requiredIndex >= 0 && !placementIncludesIndex(assigned, requiredIndex) {
				return false
			}
			placement := materializePlacement(tickets, assigned)
			evaluation := objective.Evaluate(snapshot.Now, placement, snapshot.Policy, kind)
			result.exactCandidates++
			switch {
			case evaluation.HardViolation:
				result.sawHardFailure = true
			case !evaluation.Admissible:
				result.sawQualityFailure = true
			default:
				if maxAlternatives == 1 {
					if !result.found || objective.Compare(evaluation, result.evaluation) < 0 ||
						(objective.Compare(evaluation, result.evaluation) == 0 && comparePlacement(placement, result.placement) < 0) {
						result.placement = placement
						result.evaluation = evaluation
						result.found = true
					}
				} else {
					recordPlacementAlternative(&result, alternativeIndexes, placement, evaluation)
				}
			}
			if result.exactCandidates >= maxCandidates {
				result.truncated = true
				return true
			}
			return false
		}
		if index == len(tickets) || suffixPlayers[index] < needed {
			return false
		}

		ticket := tickets[index]
		partySize := len(ticket.Players)
		partySkill := totalSkill(ticket)
		candidates := candidateTeams(remaining, teamSkills, partySize, partySkill)
		seenStates := make(map[[2]int]struct{}, len(candidates))
		for _, candidate := range candidates {
			team := candidate.team
			state := [2]int{remaining[team], teamSkills[team]}
			if _, seen := seenStates[state]; seen {
				continue
			}
			seenStates[state] = struct{}{}

			remaining[team] -= partySize
			teamSkills[team] += partySkill
			assigned[team] = append(assigned[team], index)
			stop := search(index+1, needed-partySize)
			assigned[team] = assigned[team][:len(assigned[team])-1]
			teamSkills[team] -= partySkill
			remaining[team] += partySize
			if stop {
				return true
			}
		}

		if index == requiredIndex {
			return false
		}
		return search(index+1, needed)
	}

	search(0, totalNeeded)
	result.searchNodes = budget.used - startNodes
	if maxAlternatives > 1 {
		slices.SortFunc(result.alternatives, comparePlacementAlternative)
		if len(result.alternatives) > maxAlternatives {
			result.alternatives = result.alternatives[:maxAlternatives]
		}
		if len(result.alternatives) > 0 {
			result.placement = result.alternatives[0].placement
			result.evaluation = result.alternatives[0].evaluation
			result.found = true
		}
	}
	result.evaluation.Evidence.CandidatesEvaluated = result.exactCandidates
	result.evaluation.Evidence.SearchNodes = result.searchNodes
	result.evaluation.Evidence.SearchTruncated = result.truncated || budget.exhausted
	return result
}

func recordPlacementAlternative(
	result *placementSearch,
	indexes map[string]int,
	placement [][]domain.MatchTicket,
	evaluation objective.Evaluation,
) {
	key := placementTicketKey(placement)
	if index, exists := indexes[key]; exists {
		existing := result.alternatives[index]
		if objective.Compare(evaluation, existing.evaluation) < 0 ||
			(objective.Compare(evaluation, existing.evaluation) == 0 && comparePlacement(placement, existing.placement) < 0) {
			result.alternatives[index] = placementAlternative{placement: placement, evaluation: evaluation}
		}
		return
	}
	indexes[key] = len(result.alternatives)
	result.alternatives = append(result.alternatives, placementAlternative{placement: placement, evaluation: evaluation})
}

func comparePlacementAlternative(left, right placementAlternative) int {
	if result := objective.Compare(left.evaluation, right.evaluation); result != 0 {
		return result
	}
	return comparePlacement(left.placement, right.placement)
}

func expandPlacementAlternatives(search placementSearch) []placementSearch {
	expanded := make([]placementSearch, len(search.alternatives))
	for index, alternative := range search.alternatives {
		expanded[index] = search
		expanded[index].placement = alternative.placement
		expanded[index].evaluation = alternative.evaluation
		expanded[index].evaluation.Evidence.CandidatesEvaluated = search.exactCandidates
		expanded[index].evaluation.Evidence.SearchNodes = search.searchNodes
		expanded[index].evaluation.Evidence.SearchTruncated = search.evaluation.Evidence.SearchTruncated
		expanded[index].alternatives = nil
	}
	return expanded
}

func placementTicketKey(placement [][]domain.MatchTicket) string {
	references := make([]domain.TicketRef, 0)
	for _, team := range placement {
		for _, ticket := range team {
			references = append(references, domain.TicketReference(ticket))
		}
	}
	slices.SortFunc(references, func(left, right domain.TicketRef) int {
		if result := cmp.Compare(left.ID, right.ID); result != 0 {
			return result
		}
		return cmp.Compare(left.Revision, right.Revision)
	})
	var builder strings.Builder
	for _, reference := range references {
		fmt.Fprintf(&builder, "%s@%d\x00", reference.ID, reference.Revision)
	}
	return builder.String()
}

func placementIncludesIndex(assigned [][]int, required int) bool {
	for _, team := range assigned {
		if slices.Contains(team, required) {
			return true
		}
	}
	return false
}

type teamCandidate struct {
	team   int
	spread int
}

func candidateTeams(remaining, teamSkills []int, partySize, partySkill int) []teamCandidate {
	candidates := make([]teamCandidate, 0, len(remaining))
	for team, capacity := range remaining {
		if partySize > capacity {
			continue
		}
		candidates = append(candidates, teamCandidate{
			team:   team,
			spread: projectedSkillSpread(teamSkills, team, partySkill),
		})
	}
	slices.SortFunc(candidates, func(left, right teamCandidate) int {
		if result := cmp.Compare(left.spread, right.spread); result != 0 {
			return result
		}
		return cmp.Compare(left.team, right.team)
	})
	return candidates
}

func projectedSkillSpread(teamSkills []int, target int, added int) int {
	minimum, maximum := 0, 0
	for team, skill := range teamSkills {
		if team == target {
			skill += added
		}
		if team == 0 || skill < minimum {
			minimum = skill
		}
		if team == 0 || skill > maximum {
			maximum = skill
		}
	}
	return maximum - minimum
}

func materializePlacement(tickets []domain.MatchTicket, assigned [][]int) [][]domain.MatchTicket {
	placement := make([][]domain.MatchTicket, len(assigned))
	for team, indexes := range assigned {
		placement[team] = make([]domain.MatchTicket, len(indexes))
		for index, ticketIndex := range indexes {
			placement[team][index] = tickets[ticketIndex]
		}
	}
	return placement
}

func buildProposal(
	snapshot domain.MatchmakingSnapshot,
	search placementSearch,
	sequence int,
	policyFingerprint domain.PolicyFingerprint,
	kind domain.ProposalKind,
	backfill *domain.BackfillTarget,
) domain.MatchProposal {
	proposal := domain.MatchProposal{
		Kind:              kind,
		PolicyVersion:     snapshot.Policy.Version,
		PolicyFingerprint: policyFingerprint,
		Teams:             make([]domain.TeamAssignment, len(search.placement)),
		Backfill:          domain.CloneBackfillTarget(backfill),
	}
	for team, tickets := range search.placement {
		assignment := domain.TeamAssignment{Team: team, Tickets: make([]domain.TicketRef, len(tickets))}
		for index, ticket := range tickets {
			assignment.Tickets[index] = domain.TicketReference(ticket)
			proposal.Tickets = append(proposal.Tickets, assignment.Tickets[index])
		}
		proposal.Teams[team] = assignment
	}
	proposal.Evidence = search.evaluation.Evidence
	proposal.ID = proposalIdentity(snapshot.ID, sequence, proposal)
	return proposal
}

func proposalIdentity(
	snapshotID domain.SnapshotID,
	sequence int,
	proposal domain.MatchProposal,
) domain.ProposalID {
	encoded := make([]byte, 0, 256)
	encoded = appendProposalString(encoded, string(snapshotID))
	encoded = binary.BigEndian.AppendUint64(encoded, uint64(sequence))
	encoded = appendProposalString(encoded, proposal.PolicyVersion)
	encoded = appendProposalString(encoded, string(proposal.PolicyFingerprint))
	encoded = appendProposalString(encoded, string(proposal.Kind))
	encoded = binary.BigEndian.AppendUint64(encoded, uint64(len(proposal.Teams)))
	for _, team := range proposal.Teams {
		encoded = binary.BigEndian.AppendUint64(encoded, uint64(team.Team))
		encoded = binary.BigEndian.AppendUint64(encoded, uint64(len(team.Tickets)))
		for _, ticket := range team.Tickets {
			encoded = appendProposalString(encoded, string(ticket.ID))
			encoded = binary.BigEndian.AppendUint64(encoded, uint64(ticket.Revision))
		}
	}
	if proposal.Backfill == nil {
		encoded = append(encoded, 0)
	} else {
		encoded = append(encoded, 1)
		encoded = appendProposalString(encoded, string(proposal.Backfill.Ticket.ID))
		encoded = binary.BigEndian.AppendUint64(encoded, uint64(proposal.Backfill.Ticket.Revision))
		encoded = appendProposalString(encoded, string(proposal.Backfill.SessionID))
		encoded = binary.BigEndian.AppendUint64(encoded, uint64(proposal.Backfill.RosterVersion))
	}
	digest := sha256.Sum256(encoded)
	return domain.ProposalID(fmt.Sprintf("%s/p%04d/%x", snapshotID, sequence, digest[:16]))
}

func appendProposalString(encoded []byte, value string) []byte {
	encoded = binary.BigEndian.AppendUint64(encoded, uint64(len(value)))
	return append(encoded, value...)
}

func comparePlacement(left, right [][]domain.MatchTicket) int {
	for team := 0; team < len(left) && team < len(right); team++ {
		for index := 0; index < len(left[team]) && index < len(right[team]); index++ {
			if result := cmp.Compare(left[team][index].ID, right[team][index].ID); result != 0 {
				return result
			}
			if result := cmp.Compare(left[team][index].Revision, right[team][index].Revision); result != 0 {
				return result
			}
		}
		if result := cmp.Compare(len(left[team]), len(right[team])); result != 0 {
			return result
		}
	}
	return cmp.Compare(len(left), len(right))
}

func batchUnmatchedReason(
	proposalCount int,
	maxProposals int,
	generation candidateGeneration,
	selection batchSelection,
) domain.UnmatchedReason {
	switch {
	case generation.truncated || selection.truncated:
		return domain.UnmatchedSearchBudget
	case proposalCount >= maxProposals:
		return domain.UnmatchedProposalLimit
	case len(generation.candidates) > len(selection.candidates):
		return domain.UnmatchedBatchObjective
	case generation.sawQualityFailure:
		return domain.UnmatchedQualityThreshold
	case generation.sawHardFailure:
		return domain.UnmatchedHardConstraint
	default:
		return domain.UnmatchedInsufficientCapacity
	}
}

func removePlaced(tickets []domain.MatchTicket, placement [][]domain.MatchTicket) []domain.MatchTicket {
	selected := make(map[domain.TicketID]struct{})
	for _, team := range placement {
		for _, ticket := range team {
			selected[ticket.ID] = struct{}{}
		}
	}
	remaining := make([]domain.MatchTicket, 0, len(tickets)-len(selected))
	for _, ticket := range tickets {
		if _, ok := selected[ticket.ID]; !ok {
			remaining = append(remaining, ticket)
		}
	}
	return remaining
}

func totalSkill(ticket domain.MatchTicket) int {
	total := 0
	for _, player := range ticket.Players {
		total += player.Skill
	}
	return total
}

func sortTickets(tickets []domain.MatchTicket) {
	sort.Slice(tickets, func(left, right int) bool {
		return compareQueueOrder(tickets[left].EnqueuedAt, string(tickets[left].ID), tickets[right].EnqueuedAt, string(tickets[right].ID)) < 0
	})
}

func compareQueueOrder(leftTime time.Time, leftID string, rightTime time.Time, rightID string) int {
	if result := leftTime.Compare(rightTime); result != 0 {
		return result
	}
	return cmp.Compare(leftID, rightID)
}
