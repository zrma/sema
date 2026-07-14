package domain

import (
	"fmt"
	"slices"
)

func ValidatePolicy(policy MatchmakingPolicy) error {
	if policy.Version == "" {
		return NewFailure(FailureInvalidInput, "policy version is required")
	}
	if policy.TeamCount <= 0 || policy.TeamSize <= 0 {
		return NewFailure(FailureInvalidInput, "team count and size must be positive")
	}
	if policy.MaxLatencyMillis <= 0 {
		return NewFailure(FailureInvalidInput, "maximum latency must be positive")
	}
	if policy.MaxProposals < 0 || policy.MaxSearchNodes < 0 {
		return NewFailure(FailureInvalidInput, "planning limits cannot be negative")
	}
	return nil
}

func ValidateMatchTicket(ticket MatchTicket) error {
	if ticket.ID == "" || ticket.Revision == 0 {
		return NewFailure(FailureInvalidInput, "match ticket identity and revision are required")
	}
	if ticket.EnqueuedAt.IsZero() {
		return NewFailure(FailureInvalidInput, "match ticket %q has no enqueue time", ticket.ID)
	}
	if len(ticket.Players) == 0 {
		return NewFailure(FailureInvalidInput, "match ticket %q has no players", ticket.ID)
	}
	seen := make(map[PlayerID]struct{}, len(ticket.Players))
	for _, player := range ticket.Players {
		if player.ID == "" {
			return NewFailure(FailureInvalidInput, "match ticket %q has a player without identity", ticket.ID)
		}
		if player.Skill < 0 || player.LatencyMillis < 0 {
			return NewFailure(FailureInvalidInput, "player %q has a negative skill or latency", player.ID)
		}
		if _, ok := seen[player.ID]; ok {
			return NewFailure(FailureInvalidInput, "player %q is duplicated in ticket %q", player.ID, ticket.ID)
		}
		seen[player.ID] = struct{}{}
	}
	return nil
}

func ValidateBackfillTicket(ticket BackfillTicket) error {
	if ticket.ID == "" || ticket.Revision == 0 {
		return NewFailure(FailureInvalidInput, "backfill ticket identity and revision are required")
	}
	if ticket.SessionID == "" || ticket.RosterVersion == 0 {
		return NewFailure(FailureInvalidInput, "backfill ticket %q has no session freshness", ticket.ID)
	}
	if ticket.EnqueuedAt.IsZero() {
		return NewFailure(FailureInvalidInput, "backfill ticket %q has no enqueue time", ticket.ID)
	}
	if len(ticket.OpenSlotsByTeam) == 0 {
		return NewFailure(FailureInvalidInput, "backfill ticket %q has no team shape", ticket.ID)
	}
	total := 0
	for _, slots := range ticket.OpenSlotsByTeam {
		if slots < 0 {
			return NewFailure(FailureInvalidInput, "backfill ticket %q has negative capacity", ticket.ID)
		}
		total += slots
	}
	if total == 0 {
		return NewFailure(FailureInvalidInput, "backfill ticket %q has no vacancy", ticket.ID)
	}
	return nil
}

func ValidateSnapshot(snapshot MatchmakingSnapshot) error {
	if snapshot.ID == "" || snapshot.Now.IsZero() {
		return NewFailure(FailureInvalidInput, "snapshot identity and time are required")
	}
	if err := ValidatePolicy(snapshot.Policy); err != nil {
		return err
	}
	tickets := make(map[TicketID]struct{}, len(snapshot.MatchTickets)+len(snapshot.BackfillTickets))
	players := make(map[PlayerID]struct{})
	for _, ticket := range snapshot.MatchTickets {
		if err := ValidateMatchTicket(ticket); err != nil {
			return err
		}
		if ticket.EnqueuedAt.After(snapshot.Now) {
			return NewFailure(FailureInvalidInput, "match ticket %q is enqueued after snapshot time", ticket.ID)
		}
		if _, ok := tickets[ticket.ID]; ok {
			return NewFailure(FailureInvalidInput, "ticket %q is duplicated", ticket.ID)
		}
		tickets[ticket.ID] = struct{}{}
		for _, player := range ticket.Players {
			if _, ok := players[player.ID]; ok {
				return NewFailure(FailureInvalidInput, "player %q appears in multiple tickets", player.ID)
			}
			players[player.ID] = struct{}{}
		}
	}
	for _, ticket := range snapshot.BackfillTickets {
		if err := ValidateBackfillTicket(ticket); err != nil {
			return err
		}
		if ticket.EnqueuedAt.After(snapshot.Now) {
			return NewFailure(FailureInvalidInput, "backfill ticket %q is enqueued after snapshot time", ticket.ID)
		}
		if len(ticket.OpenSlotsByTeam) != snapshot.Policy.TeamCount {
			return NewFailure(FailureInvalidInput, "backfill ticket %q team count differs from policy", ticket.ID)
		}
		for _, slots := range ticket.OpenSlotsByTeam {
			if slots > snapshot.Policy.TeamSize {
				return NewFailure(FailureInvalidInput, "backfill ticket %q vacancy exceeds team capacity", ticket.ID)
			}
		}
		if _, ok := tickets[ticket.ID]; ok {
			return NewFailure(FailureInvalidInput, "ticket %q is duplicated", ticket.ID)
		}
		tickets[ticket.ID] = struct{}{}
	}
	return nil
}

func ValidateProposal(proposal MatchProposal) error {
	if proposal.ID == "" || proposal.PolicyVersion == "" {
		return NewFailure(FailureInvalidInput, "proposal identity and policy version are required")
	}
	if proposal.Kind != ProposalNewMatch && proposal.Kind != ProposalBackfill {
		return NewFailure(FailureInvalidInput, "proposal %q has unknown kind", proposal.ID)
	}
	if len(proposal.Teams) == 0 || len(proposal.Tickets) == 0 {
		return NewFailure(FailureInvalidInput, "proposal %q has no placement", proposal.ID)
	}
	if proposal.Kind == ProposalBackfill && proposal.Backfill == nil {
		return NewFailure(FailureInvalidInput, "backfill proposal %q has no target", proposal.ID)
	}
	if proposal.Kind == ProposalNewMatch && proposal.Backfill != nil {
		return NewFailure(FailureInvalidInput, "new-match proposal %q has a backfill target", proposal.ID)
	}

	flattened := make([]TicketRef, 0, len(proposal.Tickets))
	seen := make(map[TicketID]struct{}, len(proposal.Tickets))
	for index, team := range proposal.Teams {
		if team.Team != index {
			return NewFailure(FailureInvalidInput, "proposal %q team indexes are not canonical", proposal.ID)
		}
		for _, ref := range team.Tickets {
			if ref.ID == "" || ref.Revision == 0 {
				return NewFailure(FailureInvalidInput, "proposal %q has an invalid ticket reference", proposal.ID)
			}
			if _, ok := seen[ref.ID]; ok {
				return NewFailure(FailureInvalidInput, "proposal %q repeats ticket %q", proposal.ID, ref.ID)
			}
			seen[ref.ID] = struct{}{}
			flattened = append(flattened, ref)
		}
	}
	if !slices.Equal(flattened, proposal.Tickets) {
		return NewFailure(FailureInvalidInput, "proposal %q ticket order differs from team placement", proposal.ID)
	}
	if proposal.Backfill != nil {
		target := proposal.Backfill
		if target.Ticket.ID == "" || target.Ticket.Revision == 0 || target.SessionID == "" || target.RosterVersion == 0 {
			return NewFailure(FailureInvalidInput, "proposal %q has an invalid backfill target", proposal.ID)
		}
		if _, ok := seen[target.Ticket.ID]; ok {
			return NewFailure(FailureInvalidInput, "proposal %q reuses the backfill ticket as supply", proposal.ID)
		}
	}
	return nil
}

func CloneMatchTicket(ticket MatchTicket) MatchTicket {
	ticket.Players = slices.Clone(ticket.Players)
	return ticket
}

func CloneBackfillTicket(ticket BackfillTicket) BackfillTicket {
	ticket.OpenSlotsByTeam = slices.Clone(ticket.OpenSlotsByTeam)
	return ticket
}

func CloneBackfillTarget(target *BackfillTarget) *BackfillTarget {
	if target == nil {
		return nil
	}
	cloned := *target
	return &cloned
}

func CloneTeams(teams []TeamAssignment) []TeamAssignment {
	cloned := make([]TeamAssignment, len(teams))
	for index, team := range teams {
		cloned[index] = TeamAssignment{Team: team.Team, Tickets: slices.Clone(team.Tickets)}
	}
	return cloned
}

func CloneProposal(proposal MatchProposal) MatchProposal {
	proposal.Teams = CloneTeams(proposal.Teams)
	proposal.Tickets = slices.Clone(proposal.Tickets)
	proposal.Backfill = CloneBackfillTarget(proposal.Backfill)
	return proposal
}

func TicketReference(ticket MatchTicket) TicketRef {
	return TicketRef{ID: ticket.ID, Revision: ticket.Revision}
}

func BackfillReference(ticket BackfillTicket) BackfillTarget {
	return BackfillTarget{
		Ticket:        TicketRef{ID: ticket.ID, Revision: ticket.Revision},
		SessionID:     ticket.SessionID,
		RosterVersion: ticket.RosterVersion,
	}
}

func DescribeTicket(ref TicketRef) string {
	return fmt.Sprintf("%s@%d", ref.ID, ref.Revision)
}
