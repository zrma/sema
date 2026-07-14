// Package planner implements the side-effect-free deterministic matching core.
package planner

import (
	"cmp"
	"fmt"
	"slices"
	"sort"
	"time"

	"sema/internal/domain"
)

const (
	defaultMaxProposals   = 64
	defaultMaxSearchNodes = 100_000
)

// Plan returns a deterministic set of mutually disjoint proposals.
func Plan(snapshot domain.MatchmakingSnapshot) (domain.ProposalBatch, error) {
	if err := domain.ValidateSnapshot(snapshot); err != nil {
		return domain.ProposalBatch{}, err
	}

	available := make([]domain.MatchTicket, 0, len(snapshot.MatchTickets))
	hardRejected := make([]domain.MatchTicket, 0)
	for _, ticket := range snapshot.MatchTickets {
		if withinLatencyCap(ticket, snapshot.Policy.MaxLatencyMillis) {
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
	budget := searchBudget{max: maxSearchNodes}
	batch := domain.ProposalBatch{SnapshotID: snapshot.ID}

	for _, backfill := range backfills {
		if len(batch.Proposals) >= maxProposals || budget.exhausted {
			break
		}
		placement, found, searchNodes := findPlacement(available, backfill.OpenSlotsByTeam, &budget)
		if !found {
			continue
		}
		proposal := buildProposal(snapshot, placement, len(batch.Proposals)+1, searchNodes)
		proposal.Kind = domain.ProposalBackfill
		target := domain.BackfillReference(backfill)
		proposal.Backfill = &target
		batch.Proposals = append(batch.Proposals, proposal)
		available = removePlaced(available, placement)
	}

	newMatchSlots := make([]int, snapshot.Policy.TeamCount)
	for index := range newMatchSlots {
		newMatchSlots[index] = snapshot.Policy.TeamSize
	}
	for len(batch.Proposals) < maxProposals && !budget.exhausted {
		placement, found, searchNodes := findPlacement(available, newMatchSlots, &budget)
		if !found {
			break
		}
		batch.Proposals = append(batch.Proposals, buildProposal(snapshot, placement, len(batch.Proposals)+1, searchNodes))
		available = removePlaced(available, placement)
	}

	available = append(available, hardRejected...)
	sortTickets(available)
	batch.Unmatched = make([]domain.TicketRef, len(available))
	for index, ticket := range available {
		batch.Unmatched[index] = domain.TicketReference(ticket)
	}
	batch.BudgetExhausted = budget.exhausted
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

func findPlacement(
	tickets []domain.MatchTicket,
	slots []int,
	budget *searchBudget,
) ([][]domain.MatchTicket, bool, int) {
	startNodes := budget.used
	remaining := slices.Clone(slots)
	totalNeeded := 0
	for _, count := range remaining {
		totalNeeded += count
	}
	if totalNeeded == 0 {
		return nil, false, 0
	}

	suffixPlayers := make([]int, len(tickets)+1)
	for index := len(tickets) - 1; index >= 0; index-- {
		suffixPlayers[index] = suffixPlayers[index+1] + len(tickets[index].Players)
	}
	if suffixPlayers[0] < totalNeeded {
		return nil, false, 0
	}

	assigned := make([][]int, len(slots))
	teamSkills := make([]int, len(slots))
	var result [][]domain.MatchTicket
	var search func(index int, needed int) bool
	search = func(index int, needed int) bool {
		if !budget.visit() {
			return false
		}
		if needed == 0 {
			result = materializePlacement(tickets, assigned)
			return true
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
			if search(index+1, needed-partySize) {
				return true
			}
			assigned[team] = assigned[team][:len(assigned[team])-1]
			teamSkills[team] -= partySkill
			remaining[team] += partySize
			if budget.exhausted {
				return false
			}
		}

		return search(index+1, needed)
	}

	found := search(0, totalNeeded)
	return result, found, budget.used - startNodes
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
			placement[team][index] = domain.CloneMatchTicket(tickets[ticketIndex])
		}
	}
	return placement
}

func buildProposal(
	snapshot domain.MatchmakingSnapshot,
	placement [][]domain.MatchTicket,
	sequence int,
	searchNodes int,
) domain.MatchProposal {
	proposal := domain.MatchProposal{
		ID:            domain.ProposalID(fmt.Sprintf("%s/p%04d", snapshot.ID, sequence)),
		Kind:          domain.ProposalNewMatch,
		PolicyVersion: snapshot.Policy.Version,
		Teams:         make([]domain.TeamAssignment, len(placement)),
	}
	for team, tickets := range placement {
		assignment := domain.TeamAssignment{Team: team, Tickets: make([]domain.TicketRef, len(tickets))}
		for index, ticket := range tickets {
			assignment.Tickets[index] = domain.TicketReference(ticket)
			proposal.Tickets = append(proposal.Tickets, assignment.Tickets[index])
		}
		proposal.Teams[team] = assignment
	}
	proposal.Evidence = calculateEvidence(snapshot.Now, placement, searchNodes)
	return proposal
}

func calculateEvidence(now time.Time, placement [][]domain.MatchTicket, searchNodes int) domain.ScoreEvidence {
	evidence := domain.ScoreEvidence{SearchNodes: searchNodes}
	minimumAverage, maximumAverage := 0, 0
	hasAverage := false
	for _, tickets := range placement {
		teamSkill, teamPlayers := 0, 0
		for _, ticket := range tickets {
			wait := now.Sub(ticket.EnqueuedAt).Milliseconds()
			if wait > evidence.OldestWaitMillis {
				evidence.OldestWaitMillis = wait
			}
			for _, player := range ticket.Players {
				teamSkill += player.Skill
				teamPlayers++
				if player.LatencyMillis > evidence.MaxLatencyMillis {
					evidence.MaxLatencyMillis = player.LatencyMillis
				}
			}
		}
		if teamPlayers == 0 {
			continue
		}
		average := teamSkill / teamPlayers
		if !hasAverage || average < minimumAverage {
			minimumAverage = average
		}
		if !hasAverage || average > maximumAverage {
			maximumAverage = average
		}
		hasAverage = true
	}
	evidence.TeamSkillGap = maximumAverage - minimumAverage
	return evidence
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

func withinLatencyCap(ticket domain.MatchTicket, cap int) bool {
	for _, player := range ticket.Players {
		if player.LatencyMillis > cap {
			return false
		}
	}
	return true
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
