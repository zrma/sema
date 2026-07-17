package alpha

import (
	"github.com/zrma/sema/internal/domain"
	"github.com/zrma/sema/internal/planner"
)

// Compose returns a deterministic, side-effect-free proposal batch.
func Compose(snapshot Snapshot) (ProposalBatch, error) {
	batch, err := planner.Plan(toDomainSnapshot(snapshot))
	if err != nil {
		return ProposalBatch{}, translateError(err)
	}
	return fromDomainBatch(batch), nil
}

func toDomainSnapshot(snapshot Snapshot) domain.MatchmakingSnapshot {
	matchTickets := make([]domain.MatchTicket, len(snapshot.MatchTickets))
	for index, ticket := range snapshot.MatchTickets {
		players := make([]domain.Player, len(ticket.Players))
		for playerIndex, player := range ticket.Players {
			players[playerIndex] = domain.Player{
				ID: domain.PlayerID(player.ID), Skill: player.Skill, Role: player.Role, LatencyMillis: player.LatencyMillis,
			}
		}
		matchTickets[index] = domain.MatchTicket{
			ID: domain.TicketID(ticket.ID), Revision: domain.Revision(ticket.Revision),
			EnqueuedAt: ticket.EnqueuedAt, Players: players,
		}
	}
	backfillTickets := make([]domain.BackfillTicket, len(snapshot.BackfillTickets))
	for index, ticket := range snapshot.BackfillTickets {
		backfillTickets[index] = domain.BackfillTicket{
			ID: domain.TicketID(ticket.ID), Revision: domain.Revision(ticket.Revision),
			SessionID: domain.SessionID(ticket.SessionID), RosterVersion: domain.Revision(ticket.RosterVersion),
			OpenSlotsByTeam: append([]int(nil), ticket.OpenSlotsByTeam...), EnqueuedAt: ticket.EnqueuedAt,
		}
	}
	policy := snapshot.Policy
	roleRequirements := make([]domain.RoleRequirement, len(policy.RoleRequirements))
	for index, requirement := range policy.RoleRequirements {
		roleRequirements[index] = domain.RoleRequirement{
			Role: requirement.Role, MinPerTeam: requirement.MinPerTeam, Hard: requirement.Hard,
		}
	}
	relaxationSteps := make([]domain.RelaxationStep, len(policy.RelaxationSteps))
	for index, step := range policy.RelaxationSteps {
		relaxationSteps[index] = domain.RelaxationStep{
			AfterWait: step.AfterWait, MaxTeamSkillGap: step.MaxTeamSkillGap,
			MaxRolePenalty: step.MaxRolePenalty, PrioritizeWait: step.PrioritizeWait,
		}
	}
	return domain.MatchmakingSnapshot{
		ID: domain.SnapshotID(snapshot.ID), Now: snapshot.Now,
		MatchTickets: matchTickets, BackfillTickets: backfillTickets,
		Policy: domain.MatchmakingPolicy{
			Version: policy.Version, TeamCount: policy.TeamCount, TeamSize: policy.TeamSize,
			MaxLatencyMillis: policy.MaxLatencyMillis, MaxProposals: policy.MaxProposals,
			MaxSearchNodes: policy.MaxSearchNodes, MaxCandidateTickets: policy.MaxCandidateTickets,
			MaxCandidatesPerProposal: policy.MaxCandidatesPerProposal,
			MaxBatchCandidates:       policy.MaxBatchCandidates, MaxBatchSearchNodes: policy.MaxBatchSearchNodes,
			RoleRequirements: roleRequirements, RelaxationSteps: relaxationSteps,
		},
	}
}

func fromDomainBatch(batch domain.ProposalBatch) ProposalBatch {
	proposals := make([]MatchProposal, len(batch.Proposals))
	for index, proposal := range batch.Proposals {
		teams := make([]TeamAssignment, len(proposal.Teams))
		for teamIndex, team := range proposal.Teams {
			teams[teamIndex] = TeamAssignment{Team: team.Team, Tickets: fromDomainRefs(team.Tickets)}
		}
		var backfill *BackfillTarget
		if proposal.Backfill != nil {
			backfill = &BackfillTarget{
				Ticket:    fromDomainRef(proposal.Backfill.Ticket),
				SessionID: SessionID(proposal.Backfill.SessionID), RosterVersion: Revision(proposal.Backfill.RosterVersion),
			}
		}
		evidence := proposal.Evidence
		proposals[index] = MatchProposal{
			ID: ProposalID(proposal.ID), Kind: ProposalKind(proposal.Kind),
			PolicyVersion: proposal.PolicyVersion, PolicyFingerprint: PolicyFingerprint(proposal.PolicyFingerprint),
			Teams: teams, Tickets: fromDomainRefs(proposal.Tickets), Backfill: backfill,
			Evidence: ScoreEvidence{
				RelaxationLevel: evidence.RelaxationLevel, WaitPriority: evidence.WaitPriority,
				RolePenalty: evidence.RolePenalty, TeamSkillGap: evidence.TeamSkillGap,
				OldestWaitMillis: evidence.OldestWaitMillis, TotalWaitMillis: evidence.TotalWaitMillis,
				MaxLatencyMillis: evidence.MaxLatencyMillis, CandidateTickets: evidence.CandidateTickets,
				CandidatesEvaluated: evidence.CandidatesEvaluated, SearchNodes: evidence.SearchNodes,
				CandidateWindowTruncated: evidence.CandidateWindowTruncated, SearchTruncated: evidence.SearchTruncated,
				SelectionUtility: evidence.SelectionUtility,
			},
		}
	}
	unmatched := make([]UnmatchedTicket, len(batch.Unmatched))
	for index, ticket := range batch.Unmatched {
		unmatched[index] = UnmatchedTicket{
			Ticket: fromDomainRef(ticket.Ticket), Reason: UnmatchedReason(ticket.Reason),
		}
	}
	return ProposalBatch{
		APIVersion: APIVersion, SnapshotID: SnapshotID(batch.SnapshotID),
		Proposals: proposals, Unmatched: unmatched, BudgetExhausted: batch.BudgetExhausted,
		Evidence: BatchScoreEvidence{
			CandidateProposals:           batch.Evidence.CandidateProposals,
			SelectedProposals:            batch.Evidence.SelectedProposals,
			SelectedBackfills:            batch.Evidence.SelectedBackfills,
			WaitPriorityEligibleDemands:  batch.Evidence.WaitPriorityEligibleDemands,
			WaitPrioritySelectedDemands:  batch.Evidence.WaitPrioritySelectedDemands,
			OldestWaitPriorityMillis:     batch.Evidence.OldestWaitPriorityMillis,
			OldestSelectedPriorityMillis: batch.Evidence.OldestSelectedPriorityMillis,
			TotalUtility:                 batch.Evidence.TotalUtility,
			CandidateGenerationNodes:     batch.Evidence.CandidateGenerationNodes,
			CandidateGenerationTruncated: batch.Evidence.CandidateGenerationTruncated,
			SelectionNodes:               batch.Evidence.SelectionNodes,
			SelectionTruncated:           batch.Evidence.SelectionTruncated,
		},
	}
}

func fromDomainRefs(references []domain.TicketRef) []TicketRef {
	converted := make([]TicketRef, len(references))
	for index, reference := range references {
		converted[index] = fromDomainRef(reference)
	}
	return converted
}

func fromDomainRef(reference domain.TicketRef) TicketRef {
	return TicketRef{ID: TicketID(reference.ID), Revision: Revision(reference.Revision)}
}
