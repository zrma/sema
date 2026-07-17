package main

import (
	"encoding/json"
	"log"
	"os"
	"time"

	"github.com/zrma/sema/alpha"
)

func main() {
	now := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)
	tickets := make([]alpha.MatchTicket, 4)
	for index := range tickets {
		tickets[index] = alpha.MatchTicket{
			ID: alpha.TicketID(string(rune('a'+index)) + "-ticket"), Revision: 1,
			EnqueuedAt: now.Add(-time.Duration(4-index) * time.Second),
			Players: []alpha.Player{{
				ID:    alpha.PlayerID(string(rune('a'+index)) + "-player"),
				Skill: 1000 + index, Role: "player", LatencyMillis: 20,
			}},
		}
	}
	batch, err := alpha.Compose(alpha.Snapshot{
		ID: "example-2v2", Now: now, MatchTickets: tickets,
		Policy: alpha.MatchmakingPolicy{
			Version: "example-v1", TeamCount: 2, TeamSize: 2, MaxLatencyMillis: 200,
		},
	})
	if err != nil {
		log.Fatal(err)
	}
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(batch); err != nil {
		log.Fatal(err)
	}
}
