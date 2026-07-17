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
	if policy.MaxProposals < 0 || policy.MaxSearchNodes < 0 || policy.MaxCandidateTickets < 0 ||
		policy.MaxCandidatesPerProposal < 0 || policy.MaxBatchCandidates < 0 || policy.MaxBatchSearchNodes < 0 {
		return NewFailure(FailureInvalidInput, "planning limits cannot be negative")
	}
	roles := make(map[string]struct{}, len(policy.RoleRequirements))
	for _, requirement := range policy.RoleRequirements {
		if requirement.Role == "" || requirement.MinPerTeam <= 0 || requirement.MinPerTeam > policy.TeamSize {
			return NewFailure(FailureInvalidInput, "role requirements need a role and a feasible positive minimum")
		}
		if _, exists := roles[requirement.Role]; exists {
			return NewFailure(FailureInvalidInput, "role %q is configured more than once", requirement.Role)
		}
		roles[requirement.Role] = struct{}{}
	}
	if len(policy.RelaxationSteps) > 0 {
		previous := policy.RelaxationSteps[0]
		if previous.AfterWait != 0 {
			return NewFailure(FailureInvalidInput, "the first relaxation step must start at zero wait")
		}
		if previous.MaxTeamSkillGap < 0 || previous.MaxRolePenalty < 0 {
			return NewFailure(FailureInvalidInput, "relaxation limits cannot be negative")
		}
		for index := 1; index < len(policy.RelaxationSteps); index++ {
			current := policy.RelaxationSteps[index]
			if current.AfterWait <= previous.AfterWait {
				return NewFailure(FailureInvalidInput, "relaxation wait thresholds must increase")
			}
			if current.MaxTeamSkillGap < previous.MaxTeamSkillGap || current.MaxRolePenalty < previous.MaxRolePenalty {
				return NewFailure(FailureInvalidInput, "relaxation limits cannot become stricter")
			}
			if previous.PrioritizeWait && !current.PrioritizeWait {
				return NewFailure(FailureInvalidInput, "wait priority cannot be disabled by a later relaxation step")
			}
			previous = current
		}
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
	if len(ticket.ExistingTeams) > 0 && len(ticket.ExistingTeams) != len(ticket.OpenSlotsByTeam) {
		return NewFailure(FailureInvalidInput, "backfill ticket %q roster team count differs from vacancy shape", ticket.ID)
	}
	for teamIndex, team := range ticket.ExistingTeams {
		if team.PlayerCount < 0 || team.SkillTotal < 0 || team.MaxLatencyMillis < 0 {
			return NewFailure(FailureInvalidInput, "backfill ticket %q team %d has negative roster quality", ticket.ID, teamIndex)
		}
		roles := make(map[string]struct{}, len(team.RoleCounts))
		rolePlayers := 0
		for _, role := range team.RoleCounts {
			if role.Role == "" || role.Count < 0 {
				return NewFailure(FailureInvalidInput, "backfill ticket %q team %d has invalid role count", ticket.ID, teamIndex)
			}
			if _, exists := roles[role.Role]; exists {
				return NewFailure(FailureInvalidInput, "backfill ticket %q team %d repeats role %q", ticket.ID, teamIndex, role.Role)
			}
			roles[role.Role] = struct{}{}
			rolePlayers += role.Count
		}
		if rolePlayers > team.PlayerCount {
			return NewFailure(FailureInvalidInput, "backfill ticket %q team %d role counts exceed players", ticket.ID, teamIndex)
		}
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
		for teamIndex, slots := range ticket.OpenSlotsByTeam {
			if slots > snapshot.Policy.TeamSize {
				return NewFailure(FailureInvalidInput, "backfill ticket %q vacancy exceeds team capacity", ticket.ID)
			}
			if len(ticket.ExistingTeams) > 0 && ticket.ExistingTeams[teamIndex].PlayerCount+slots != snapshot.Policy.TeamSize {
				return NewFailure(FailureInvalidInput, "backfill ticket %q team %d roster and vacancy do not fill policy capacity", ticket.ID, teamIndex)
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
	if proposal.ID == "" || proposal.PolicyVersion == "" || proposal.PolicyFingerprint == "" {
		return NewFailure(FailureInvalidInput, "proposal identity, policy version, and fingerprint are required")
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
	ticket.ExistingTeams = slices.Clone(ticket.ExistingTeams)
	for index := range ticket.ExistingTeams {
		ticket.ExistingTeams[index].RoleCounts = slices.Clone(ticket.ExistingTeams[index].RoleCounts)
	}
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

func ClonePolicy(policy MatchmakingPolicy) MatchmakingPolicy {
	policy.RoleRequirements = slices.Clone(policy.RoleRequirements)
	policy.RelaxationSteps = slices.Clone(policy.RelaxationSteps)
	return policy
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
