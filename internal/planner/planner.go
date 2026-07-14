// Package planner implements the side-effect-free deterministic matching core.
package planner

import (
	"cmp"
	"fmt"
	"slices"
	"sort"
	"time"

	"sema/internal/constraint"
	"sema/internal/domain"
	"sema/internal/objective"
)

const (
	defaultMaxProposals             = 64
	defaultMaxSearchNodes           = 100_000
	defaultMaxCandidatesPerProposal = 64
)

// Plan returns a deterministic set of mutually disjoint proposals.
func Plan(snapshot domain.MatchmakingSnapshot) (domain.ProposalBatch, error) {
	if err := domain.ValidateSnapshot(snapshot); err != nil {
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
	maxCandidates := snapshot.Policy.MaxCandidatesPerProposal
	if maxCandidates == 0 {
		maxCandidates = defaultMaxCandidatesPerProposal
	}
	budget := searchBudget{max: maxSearchNodes}
	batch := domain.ProposalBatch{SnapshotID: snapshot.ID}
	lastSearch := placementSearch{}

	for _, backfill := range backfills {
		if len(batch.Proposals) >= maxProposals || budget.exhausted {
			break
		}
		lastSearch = findBestPlacement(
			available,
			backfill.OpenSlotsByTeam,
			snapshot,
			domain.ProposalBackfill,
			maxCandidates,
			&budget,
		)
		batch.BudgetExhausted = batch.BudgetExhausted || lastSearch.truncated
		if !lastSearch.found {
			continue
		}
		proposal := buildProposal(snapshot, lastSearch, len(batch.Proposals)+1)
		proposal.Kind = domain.ProposalBackfill
		target := domain.BackfillReference(backfill)
		proposal.Backfill = &target
		batch.Proposals = append(batch.Proposals, proposal)
		available = removePlaced(available, lastSearch.placement)
	}

	newMatchSlots := make([]int, snapshot.Policy.TeamCount)
	for index := range newMatchSlots {
		newMatchSlots[index] = snapshot.Policy.TeamSize
	}
	for len(batch.Proposals) < maxProposals && !budget.exhausted {
		lastSearch = findBestPlacement(
			available,
			newMatchSlots,
			snapshot,
			domain.ProposalNewMatch,
			maxCandidates,
			&budget,
		)
		batch.BudgetExhausted = batch.BudgetExhausted || lastSearch.truncated
		if !lastSearch.found {
			break
		}
		batch.Proposals = append(batch.Proposals, buildProposal(snapshot, lastSearch, len(batch.Proposals)+1))
		available = removePlaced(available, lastSearch.placement)
	}

	reason := unmatchedReason(len(batch.Proposals), maxProposals, budget, lastSearch)
	reasons := make(map[domain.TicketID]domain.UnmatchedReason, len(available)+len(hardRejected))
	for _, ticket := range available {
		reasons[ticket.ID] = reason
	}
	for _, ticket := range hardRejected {
		reasons[ticket.ID] = domain.UnmatchedHardConstraint
	}
	available = append(available, hardRejected...)
	sortTickets(available)
	batch.Unmatched = make([]domain.UnmatchedTicket, len(available))
	for index, ticket := range available {
		batch.Unmatched[index] = domain.UnmatchedTicket{
			Ticket: domain.TicketReference(ticket),
			Reason: reasons[ticket.ID],
		}
	}
	batch.BudgetExhausted = batch.BudgetExhausted || budget.exhausted
	return batch, nil
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
	placement         [][]domain.MatchTicket
	evaluation        objective.Evaluation
	found             bool
	exactCandidates   int
	searchNodes       int
	truncated         bool
	sawHardFailure    bool
	sawQualityFailure bool
}

func findBestPlacement(
	tickets []domain.MatchTicket,
	slots []int,
	snapshot domain.MatchmakingSnapshot,
	kind domain.ProposalKind,
	maxCandidates int,
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

	assigned := make([][]int, len(slots))
	teamSkills := make([]int, len(slots))
	var search func(index int, needed int) bool
	search = func(index int, needed int) bool {
		if !budget.visit() {
			return true
		}
		if needed == 0 {
			placement := materializePlacement(tickets, assigned)
			evaluation := objective.Evaluate(snapshot.Now, placement, snapshot.Policy, kind)
			result.exactCandidates++
			switch {
			case evaluation.HardViolation:
				result.sawHardFailure = true
			case !evaluation.Admissible:
				result.sawQualityFailure = true
			case !result.found || objective.Compare(evaluation, result.evaluation) < 0 ||
				(objective.Compare(evaluation, result.evaluation) == 0 && comparePlacement(placement, result.placement) < 0):
				result.placement = placement
				result.evaluation = evaluation
				result.found = true
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

		return search(index+1, needed)
	}

	search(0, totalNeeded)
	result.searchNodes = budget.used - startNodes
	result.evaluation.Evidence.CandidatesEvaluated = result.exactCandidates
	result.evaluation.Evidence.SearchNodes = result.searchNodes
	result.evaluation.Evidence.SearchTruncated = result.truncated || budget.exhausted
	return result
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
) domain.MatchProposal {
	proposal := domain.MatchProposal{
		ID:            domain.ProposalID(fmt.Sprintf("%s/p%04d", snapshot.ID, sequence)),
		Kind:          domain.ProposalNewMatch,
		PolicyVersion: snapshot.Policy.Version,
		Teams:         make([]domain.TeamAssignment, len(search.placement)),
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
	return proposal
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

func unmatchedReason(
	proposalCount int,
	maxProposals int,
	budget searchBudget,
	search placementSearch,
) domain.UnmatchedReason {
	switch {
	case proposalCount >= maxProposals:
		return domain.UnmatchedProposalLimit
	case budget.exhausted || search.truncated:
		return domain.UnmatchedSearchBudget
	case search.sawQualityFailure:
		return domain.UnmatchedQualityThreshold
	case search.sawHardFailure:
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
