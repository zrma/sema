package v0alpha2

import (
	"fmt"
	"time"

	"github.com/zrma/sema/internal/domain"
)

func ToDomainMatchTicket(ticket MatchTicket) domain.MatchTicket {
	players := make([]domain.Player, len(ticket.Players))
	for index, player := range ticket.Players {
		players[index] = domain.Player{
			ID: domain.PlayerID(player.ID), Skill: player.Skill, Role: player.Role,
			LatencyMillis: player.LatencyMillis,
		}
	}
	return domain.MatchTicket{
		ID: domain.TicketID(ticket.ID), Revision: domain.Revision(ticket.Revision),
		EnqueuedAt: ticket.EnqueuedAt, Players: players,
	}
}

func FromDomainMatchTicket(ticket domain.MatchTicket) MatchTicket {
	players := make([]Player, len(ticket.Players))
	for index, player := range ticket.Players {
		players[index] = Player{
			ID: string(player.ID), Skill: player.Skill, Role: player.Role,
			LatencyMillis: player.LatencyMillis,
		}
	}
	return MatchTicket{
		ID: string(ticket.ID), Revision: uint64(ticket.Revision),
		EnqueuedAt: ticket.EnqueuedAt, Players: players,
	}
}

func ToDomainBackfillTicket(ticket BackfillTicket) domain.BackfillTicket {
	teams := make([]domain.RosterTeamSummary, len(ticket.ExistingTeams))
	for teamIndex, team := range ticket.ExistingTeams {
		roles := make([]domain.RoleCount, len(team.RoleCounts))
		for roleIndex, role := range team.RoleCounts {
			roles[roleIndex] = domain.RoleCount{Role: role.Role, Count: role.Count}
		}
		teams[teamIndex] = domain.RosterTeamSummary{
			PlayerCount: team.PlayerCount, SkillTotal: team.SkillTotal,
			RoleCounts: roles, MaxLatencyMillis: team.MaxLatencyMillis,
		}
	}
	return domain.BackfillTicket{
		ID: domain.TicketID(ticket.ID), Revision: domain.Revision(ticket.Revision),
		SessionID: domain.SessionID(ticket.SessionID), RosterVersion: domain.Revision(ticket.RosterVersion),
		OpenSlotsByTeam: append([]int(nil), ticket.OpenSlotsByTeam...), ExistingTeams: teams,
		EnqueuedAt: ticket.EnqueuedAt,
	}
}

func FromDomainBackfillTicket(ticket domain.BackfillTicket) BackfillTicket {
	teams := make([]RosterTeamSummary, len(ticket.ExistingTeams))
	for teamIndex, team := range ticket.ExistingTeams {
		roles := make([]RoleCount, len(team.RoleCounts))
		for roleIndex, role := range team.RoleCounts {
			roles[roleIndex] = RoleCount{Role: role.Role, Count: role.Count}
		}
		teams[teamIndex] = RosterTeamSummary{
			PlayerCount: team.PlayerCount, SkillTotal: team.SkillTotal,
			RoleCounts: roles, MaxLatencyMillis: team.MaxLatencyMillis,
		}
	}
	return BackfillTicket{
		ID: string(ticket.ID), Revision: uint64(ticket.Revision), SessionID: string(ticket.SessionID),
		RosterVersion:   uint64(ticket.RosterVersion),
		OpenSlotsByTeam: append([]int(nil), ticket.OpenSlotsByTeam...), ExistingTeams: teams,
		EnqueuedAt: ticket.EnqueuedAt,
	}
}

func ToDomainPolicy(policy MatchmakingPolicy) (domain.MatchmakingPolicy, error) {
	requirements := make([]domain.RoleRequirement, len(policy.RoleRequirements))
	for index, requirement := range policy.RoleRequirements {
		requirements[index] = domain.RoleRequirement{
			Role: requirement.Role, MinPerTeam: requirement.MinPerTeam, Hard: requirement.Hard,
		}
	}
	steps := make([]domain.RelaxationStep, len(policy.RelaxationSteps))
	for index, step := range policy.RelaxationSteps {
		if step.AfterWaitMillis < 0 || step.AfterWaitMillis > int64((1<<63-1)/time.Millisecond) {
			return domain.MatchmakingPolicy{}, fmt.Errorf(
				"relaxation step %d has an invalid after_wait_millis",
				index,
			)
		}
		steps[index] = domain.RelaxationStep{
			AfterWait:       time.Duration(step.AfterWaitMillis) * time.Millisecond,
			MaxTeamSkillGap: step.MaxTeamSkillGap, MaxRolePenalty: step.MaxRolePenalty,
			PrioritizeWait: step.PrioritizeWait,
		}
	}
	return domain.MatchmakingPolicy{
		Version: policy.Version, TeamCount: policy.TeamCount, TeamSize: policy.TeamSize,
		MaxLatencyMillis: policy.MaxLatencyMillis, MaxProposals: policy.MaxProposals,
		MaxSearchNodes: policy.MaxSearchNodes, MaxCandidateTickets: policy.MaxCandidateTickets,
		MaxCandidatesPerProposal: policy.MaxCandidatesPerProposal,
		MaxBatchCandidates:       policy.MaxBatchCandidates, MaxBatchSearchNodes: policy.MaxBatchSearchNodes,
		RoleRequirements: requirements, RelaxationSteps: steps,
	}, nil
}

func FromDomainPolicy(policy domain.MatchmakingPolicy) MatchmakingPolicy {
	requirements := make([]RoleRequirement, len(policy.RoleRequirements))
	for index, requirement := range policy.RoleRequirements {
		requirements[index] = RoleRequirement{
			Role: requirement.Role, MinPerTeam: requirement.MinPerTeam, Hard: requirement.Hard,
		}
	}
	steps := make([]RelaxationStep, len(policy.RelaxationSteps))
	for index, step := range policy.RelaxationSteps {
		steps[index] = RelaxationStep{
			AfterWaitMillis: step.AfterWait.Milliseconds(), MaxTeamSkillGap: step.MaxTeamSkillGap,
			MaxRolePenalty: step.MaxRolePenalty, PrioritizeWait: step.PrioritizeWait,
		}
	}
	return MatchmakingPolicy{
		Version: policy.Version, TeamCount: policy.TeamCount, TeamSize: policy.TeamSize,
		MaxLatencyMillis: policy.MaxLatencyMillis, MaxProposals: policy.MaxProposals,
		MaxSearchNodes: policy.MaxSearchNodes, MaxCandidateTickets: policy.MaxCandidateTickets,
		MaxCandidatesPerProposal: policy.MaxCandidatesPerProposal,
		MaxBatchCandidates:       policy.MaxBatchCandidates, MaxBatchSearchNodes: policy.MaxBatchSearchNodes,
		RoleRequirements: requirements, RelaxationSteps: steps,
	}
}

func FromDomainProposal(proposal domain.MatchProposal) MatchProposal {
	teams := make([]TeamAssignment, len(proposal.Teams))
	for index, team := range proposal.Teams {
		teams[index] = TeamAssignment{Team: team.Team, Tickets: fromDomainTicketRefs(team.Tickets)}
	}
	return MatchProposal{
		ID: string(proposal.ID), Kind: string(proposal.Kind), PolicyVersion: proposal.PolicyVersion,
		PolicyFingerprint: string(proposal.PolicyFingerprint), Teams: teams,
		Tickets: fromDomainTicketRefs(proposal.Tickets), Backfill: fromDomainBackfillTarget(proposal.Backfill),
		Evidence: ScoreEvidence{
			RelaxationLevel: proposal.Evidence.RelaxationLevel, WaitPriority: proposal.Evidence.WaitPriority,
			RolePenalty: proposal.Evidence.RolePenalty, TeamSkillGap: proposal.Evidence.TeamSkillGap,
			OldestWaitMillis:         proposal.Evidence.OldestWaitMillis,
			TotalWaitMillis:          proposal.Evidence.TotalWaitMillis,
			MaxLatencyMillis:         proposal.Evidence.MaxLatencyMillis,
			CandidateTickets:         proposal.Evidence.CandidateTickets,
			CandidatesEvaluated:      proposal.Evidence.CandidatesEvaluated,
			SearchNodes:              proposal.Evidence.SearchNodes,
			CandidateWindowTruncated: proposal.Evidence.CandidateWindowTruncated,
			SearchTruncated:          proposal.Evidence.SearchTruncated,
			SelectionUtility:         proposal.Evidence.SelectionUtility,
		},
	}
}

func FromDomainUnmatched(unmatched domain.UnmatchedTicket) UnmatchedTicket {
	return UnmatchedTicket{
		Ticket: TicketRef{ID: string(unmatched.Ticket.ID), Revision: uint64(unmatched.Ticket.Revision)},
		Reason: string(unmatched.Reason),
	}
}

func FromDomainBatchEvidence(evidence domain.BatchScoreEvidence) BatchScoreEvidence {
	return BatchScoreEvidence{
		CandidateProposals:           evidence.CandidateProposals,
		SelectedProposals:            evidence.SelectedProposals,
		SelectedBackfills:            evidence.SelectedBackfills,
		WaitPriorityEligibleDemands:  evidence.WaitPriorityEligibleDemands,
		WaitPrioritySelectedDemands:  evidence.WaitPrioritySelectedDemands,
		OldestWaitPriorityMillis:     evidence.OldestWaitPriorityMillis,
		OldestSelectedPriorityMillis: evidence.OldestSelectedPriorityMillis,
		TotalUtility:                 evidence.TotalUtility,
		CandidateGenerationNodes:     evidence.CandidateGenerationNodes,
		CandidateGenerationTruncated: evidence.CandidateGenerationTruncated,
		SelectionNodes:               evidence.SelectionNodes,
		SelectionTruncated:           evidence.SelectionTruncated,
	}
}

func FromDomainReservation(reservation domain.Reservation) Reservation {
	return Reservation{
		ID: string(reservation.ID), ProposalID: string(reservation.ProposalID),
		Tickets: fromDomainTicketRefs(reservation.Tickets), Backfill: fromDomainBackfillTarget(reservation.Backfill),
		ExpiresAt: reservation.ExpiresAt.UTC(), Status: string(reservation.Status),
	}
}

func FromDomainAssignment(assignment domain.Assignment) Assignment {
	teams := make([]TeamAssignment, len(assignment.Teams))
	for index, team := range assignment.Teams {
		teams[index] = TeamAssignment{Team: team.Team, Tickets: fromDomainTicketRefs(team.Tickets)}
	}
	var acknowledgment *AssignmentAcknowledgment
	if assignment.Acknowledgment != nil {
		acknowledgment = &AssignmentAcknowledgment{
			OperationID: string(assignment.Acknowledgment.OperationID),
			Outcome:     string(assignment.Acknowledgment.Outcome), SessionID: string(assignment.Acknowledgment.SessionID),
			ExpectedRosterVersion:  uint64(assignment.Acknowledgment.ExpectedRosterVersion),
			ResultingRosterVersion: uint64(assignment.Acknowledgment.ResultingRosterVersion),
			FailureCode:            string(assignment.Acknowledgment.FailureCode), Reason: assignment.Acknowledgment.Reason,
			AcknowledgedAt: assignment.Acknowledgment.AcknowledgedAt.UTC(),
		}
	}
	return Assignment{
		ID: string(assignment.ID), ReservationID: string(assignment.ReservationID),
		ProposalID: string(assignment.ProposalID), Kind: string(assignment.Kind),
		Teams: teams, Backfill: fromDomainBackfillTarget(assignment.Backfill),
		ConfirmedAt: assignment.ConfirmedAt.UTC(), Status: string(assignment.Status),
		Acknowledgment: acknowledgment,
	}
}

func ToDomainAcknowledgment(
	operationID string,
	request AcknowledgeAssignmentRequest,
) domain.AssignmentAcknowledgmentRequest {
	return domain.AssignmentAcknowledgmentRequest{
		OperationID: domain.OperationID(operationID), Outcome: domain.AssignmentStatus(request.Outcome),
		SessionID:              domain.SessionID(request.SessionID),
		ExpectedRosterVersion:  domain.Revision(request.ExpectedRosterVersion),
		ResultingRosterVersion: domain.Revision(request.ResultingRosterVersion),
		FailureCode:            domain.FailureCode(request.FailureCode), Reason: request.Reason,
	}
}

func fromDomainTicketRefs(references []domain.TicketRef) []TicketRef {
	converted := make([]TicketRef, len(references))
	for index, reference := range references {
		converted[index] = TicketRef{ID: string(reference.ID), Revision: uint64(reference.Revision)}
	}
	return converted
}

func fromDomainBackfillTarget(target *domain.BackfillTarget) *BackfillTarget {
	if target == nil {
		return nil
	}
	return &BackfillTarget{
		Ticket:    TicketRef{ID: string(target.Ticket.ID), Revision: uint64(target.Ticket.Revision)},
		SessionID: string(target.SessionID), RosterVersion: uint64(target.RosterVersion),
	}
}
