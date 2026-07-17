// Package discovery bounds the ticket supply passed to placement enumeration.
package discovery

import "github.com/zrma/sema/internal/domain"

type Window struct {
	Tickets   []domain.MatchTicket
	Truncated bool
}

// SelectWindow returns the oldest fitting queue prefix from canonically ordered tickets.
func SelectWindow(tickets []domain.MatchTicket, slots []int, limit int) Window {
	if limit <= 0 {
		return Window{Tickets: tickets}
	}
	maxPartySize := 0
	for _, available := range slots {
		maxPartySize = max(maxPartySize, available)
	}
	selected := make([]domain.MatchTicket, 0, min(limit, len(tickets)))
	for _, ticket := range tickets {
		if len(ticket.Players) > maxPartySize {
			continue
		}
		if len(selected) == limit {
			return Window{Tickets: selected, Truncated: true}
		}
		selected = append(selected, ticket)
	}
	return Window{Tickets: selected}
}
