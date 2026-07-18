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
