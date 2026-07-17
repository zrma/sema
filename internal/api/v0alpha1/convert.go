package v0alpha1

import (
	"fmt"
	"time"

	"github.com/zrma/sema/internal/domain"
)

func ToDomainMatchTicket(ticket MatchTicket) domain.MatchTicket {
	players := make([]domain.Player, len(ticket.Players))
	for index, player := range ticket.Players {
		players[index] = domain.Player{
			ID: domain.PlayerID(player.ID), Skill: player.Skill, Role: player.Role, LatencyMillis: player.LatencyMillis,
		}
	}
	return domain.MatchTicket{
		ID: domain.TicketID(ticket.ID), Revision: domain.Revision(ticket.Revision),
		EnqueuedAt: ticket.EnqueuedAt, Players: players,
	}
}

func ToDomainBackfillTicket(ticket BackfillTicket) domain.BackfillTicket {
	return domain.BackfillTicket{
		ID: domain.TicketID(ticket.ID), Revision: domain.Revision(ticket.Revision),
		SessionID: domain.SessionID(ticket.SessionID), RosterVersion: domain.Revision(ticket.RosterVersion),
		OpenSlotsByTeam: append([]int(nil), ticket.OpenSlotsByTeam...), EnqueuedAt: ticket.EnqueuedAt,
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
			return domain.MatchmakingPolicy{}, fmt.Errorf("relaxation step %d has an invalid after_wait_millis", index)
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
		RoleRequirements:         requirements, RelaxationSteps: steps,
	}, nil
}

func ToDomainAcknowledgment(request AcknowledgeAssignmentRequest) domain.AssignmentAcknowledgmentRequest {
	return domain.AssignmentAcknowledgmentRequest{
		OperationID: domain.OperationID(request.OperationID), Outcome: domain.AssignmentStatus(request.Outcome),
		SessionID: domain.SessionID(request.SessionID), ExpectedRosterVersion: domain.Revision(request.ExpectedRosterVersion),
		ResultingRosterVersion: domain.Revision(request.ResultingRosterVersion),
		FailureCode:            domain.FailureCode(request.FailureCode), Reason: request.Reason,
	}
}

func FromDomainProposalBatch(batch domain.ProposalBatch) ProposalBatch {
	proposals := make([]MatchProposal, len(batch.Proposals))
	for index, proposal := range batch.Proposals {
		proposals[index] = FromDomainProposal(proposal)
	}
	unmatched := make([]UnmatchedTicket, len(batch.Unmatched))
	for index, ticket := range batch.Unmatched {
		unmatched[index] = UnmatchedTicket{
			Ticket: fromDomainTicketRef(ticket.Ticket), Reason: string(ticket.Reason),
		}
	}
	return ProposalBatch{
		SnapshotID: string(batch.SnapshotID), Proposals: proposals,
		Unmatched: unmatched, BudgetExhausted: batch.BudgetExhausted,
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
			OldestWaitMillis: proposal.Evidence.OldestWaitMillis, TotalWaitMillis: proposal.Evidence.TotalWaitMillis,
			MaxLatencyMillis: proposal.Evidence.MaxLatencyMillis, CandidateTickets: proposal.Evidence.CandidateTickets,
			CandidatesEvaluated: proposal.Evidence.CandidatesEvaluated, SearchNodes: proposal.Evidence.SearchNodes,
			CandidateWindowTruncated: proposal.Evidence.CandidateWindowTruncated,
			SearchTruncated:          proposal.Evidence.SearchTruncated,
		},
	}
}

func FromDomainReservation(reservation domain.Reservation) Reservation {
	return Reservation{
		ID: string(reservation.ID), ProposalID: string(reservation.ProposalID),
		Tickets: fromDomainTicketRefs(reservation.Tickets), Backfill: fromDomainBackfillTarget(reservation.Backfill),
		ExpiresAt: reservation.ExpiresAt, Status: string(reservation.Status),
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
			OperationID: string(assignment.Acknowledgment.OperationID), Outcome: string(assignment.Acknowledgment.Outcome),
			SessionID:              string(assignment.Acknowledgment.SessionID),
			ExpectedRosterVersion:  uint64(assignment.Acknowledgment.ExpectedRosterVersion),
			ResultingRosterVersion: uint64(assignment.Acknowledgment.ResultingRosterVersion),
			FailureCode:            string(assignment.Acknowledgment.FailureCode), Reason: assignment.Acknowledgment.Reason,
			AcknowledgedAt: assignment.Acknowledgment.AcknowledgedAt,
		}
	}
	return Assignment{
		ID: string(assignment.ID), ReservationID: string(assignment.ReservationID), ProposalID: string(assignment.ProposalID),
		Kind: string(assignment.Kind), Teams: teams, Backfill: fromDomainBackfillTarget(assignment.Backfill),
		ConfirmedAt: assignment.ConfirmedAt, Status: string(assignment.Status), Acknowledgment: acknowledgment,
	}
}

func fromDomainTicketRefs(references []domain.TicketRef) []TicketRef {
	converted := make([]TicketRef, len(references))
	for index, reference := range references {
		converted[index] = fromDomainTicketRef(reference)
	}
	return converted
}

func fromDomainTicketRef(reference domain.TicketRef) TicketRef {
	return TicketRef{ID: string(reference.ID), Revision: uint64(reference.Revision)}
}

func fromDomainBackfillTarget(target *domain.BackfillTarget) *BackfillTarget {
	if target == nil {
		return nil
	}
	return &BackfillTarget{
		Ticket: fromDomainTicketRef(target.Ticket), SessionID: string(target.SessionID),
		RosterVersion: uint64(target.RosterVersion),
	}
}
