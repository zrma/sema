package discovery_test

import (
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/zrma/sema/internal/discovery"
	"github.com/zrma/sema/internal/domain"
)

func TestIndexedWindowMatchesLinearSmallShapes(t *testing.T) {
	tickets := indexedTickets(96)
	index := discovery.BuildIndex(tickets)
	if stats := index.Stats(); stats.Tickets != len(tickets) || stats.Partitions < 4 {
		t.Fatalf("index stats = %#v", stats)
	}
	for _, slots := range [][]int{{1, 1}, {2, 1}, {5, 5}} {
		for _, limit := range []int{0, 1, 7, 32, 128} {
			linear := discovery.SelectWindow(tickets, slots, limit)
			indexed := index.SelectWindow(slots, limit)
			if !reflect.DeepEqual(linear, indexed) {
				t.Fatalf("slots=%v limit=%d\nlinear=%#v\nindexed=%#v", slots, limit, linear, indexed)
			}
		}
	}
}

func TestIndexedWindowMatchesLinearTenThousandTickets(t *testing.T) {
	tickets := indexedTickets(10_000)
	index := discovery.BuildIndex(tickets)
	for _, slots := range [][]int{{1, 1}, {2, 2}, {5, 5}} {
		linear := discovery.SelectWindow(tickets, slots, 256)
		indexed := index.SelectWindow(slots, 256)
		if !reflect.DeepEqual(linear, indexed) {
			t.Fatalf("large indexed window differs for slots=%v", slots)
		}
	}
}

func FuzzIndexedWindowEquivalent(f *testing.F) {
	f.Add(uint8(2), uint8(7), []byte{1, 2, 3, 4, 1, 3, 2})
	f.Add(uint8(5), uint8(0), []byte{4, 4, 2, 1, 3})
	f.Fuzz(func(t *testing.T, maxPartySeed, limitSeed uint8, data []byte) {
		if len(data) > 128 {
			data = data[:128]
		}
		base := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)
		tickets := make([]domain.MatchTicket, len(data))
		for index, value := range data {
			partySize := 1 + int(value%8)
			ticket := domain.MatchTicket{
				ID: domain.TicketID(fmt.Sprintf("fuzz-ticket-%03d", index)), Revision: 1,
				EnqueuedAt: base.Add(time.Duration(index) * time.Millisecond),
			}
			for player := range partySize {
				ticket.Players = append(ticket.Players, domain.Player{
					ID:    domain.PlayerID(fmt.Sprintf("fuzz-player-%03d-%02d", index, player)),
					Skill: int(value) * 10, Role: []string{"", "dps", "healer"}[int(value)%3],
					LatencyMillis: 20 + int(value%100),
				})
			}
			tickets[index] = ticket
		}
		maxParty := 1 + int(maxPartySeed%8)
		limit := int(limitSeed % 33)
		linear := discovery.SelectWindow(tickets, []int{maxParty}, limit)
		indexed := discovery.BuildIndex(tickets).SelectWindow([]int{maxParty}, limit)
		if !reflect.DeepEqual(linear, indexed) {
			t.Fatalf("maxParty=%d limit=%d linear=%#v indexed=%#v", maxParty, limit, linear, indexed)
		}
	})
}

func BenchmarkWindowSelectionReuse(b *testing.B) {
	tickets := indexedTickets(100_000)
	index := discovery.BuildIndex(tickets)
	shapes := [][]int{{1, 1}, {2, 1}, {2, 2}, {5, 5}}
	b.Run("linear", func(b *testing.B) {
		for iteration := 0; iteration < b.N; iteration++ {
			for _, slots := range shapes {
				discovery.SelectWindow(tickets, slots, 256)
			}
		}
	})
	b.Run("indexed", func(b *testing.B) {
		for iteration := 0; iteration < b.N; iteration++ {
			for _, slots := range shapes {
				index.SelectWindow(slots, 256)
			}
		}
	})
}

func BenchmarkBuildIndex(b *testing.B) {
	tickets := indexedTickets(100_000)
	b.ResetTimer()
	for range b.N {
		discovery.BuildIndex(tickets)
	}
}

func indexedTickets(count int) []domain.MatchTicket {
	base := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)
	tickets := make([]domain.MatchTicket, count)
	for index := range tickets {
		partySize := index%4 + 1
		ticket := domain.MatchTicket{
			ID: domain.TicketID(fmt.Sprintf("ticket-%05d", index)), Revision: 1,
			EnqueuedAt: base.Add(time.Duration(index) * time.Millisecond),
		}
		for player := 0; player < partySize; player++ {
			ticket.Players = append(ticket.Players, domain.Player{
				ID:    domain.PlayerID(fmt.Sprintf("player-%05d-%d", index, player)),
				Skill: 1_200 + index%700, Role: []string{"", "dps", "healer"}[(index+player)%3],
				LatencyMillis: 20 + index%80,
			})
		}
		tickets[index] = ticket
	}
	return tickets
}
