package discovery_test

import (
	"testing"

	"github.com/zrma/sema/internal/discovery"
	"github.com/zrma/sema/internal/domain"
)

func TestSelectWindowUsesOldestFittingPrefix(t *testing.T) {
	tickets := []domain.MatchTicket{
		party("too-large", 3),
		party("first", 1),
		party("second", 2),
		party("truncated", 1),
	}
	window := discovery.SelectWindow(tickets, []int{1, 2}, 2)
	if !window.Truncated || len(window.Tickets) != 2 {
		t.Fatalf("window = %#v", window)
	}
	if window.Tickets[0].ID != "first" || window.Tickets[1].ID != "second" {
		t.Fatalf("selected tickets = %#v", window.Tickets)
	}
}

func TestSelectWindowKeepsUnboundedInput(t *testing.T) {
	tickets := []domain.MatchTicket{party("large", 3), party("small", 1)}
	window := discovery.SelectWindow(tickets, []int{1, 1}, 0)
	if window.Truncated || len(window.Tickets) != len(tickets) || &window.Tickets[0] != &tickets[0] {
		t.Fatalf("unbounded window = %#v", window)
	}
}

func party(id domain.TicketID, size int) domain.MatchTicket {
	return domain.MatchTicket{ID: id, Players: make([]domain.Player, size)}
}
