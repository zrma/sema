package v0alpha2

import "github.com/zrma/sema/internal/domain"

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
