package flowui

import (
	"testing"
	"time"

	api "github.com/zrma/sema/internal/api/v0alpha1"
	"github.com/zrma/sema/internal/league"
)

func TestAverageQueueWaitWeightsPlayersAndStopsAtConfirmation(t *testing.T) {
	now := time.Date(2026, time.January, 1, 0, 1, 0, 0, time.UTC)
	model := &Model{
		now: now,
		tickets: map[string]*ticketView{
			"solo": {
				ticket: api.MatchTicket{EnqueuedAt: now.Add(-10 * time.Second), Players: []api.Player{{ID: "solo"}}},
				state:  ticketQueued,
			},
			"trio": {
				ticket: api.MatchTicket{EnqueuedAt: now.Add(-4 * time.Second), Players: []api.Player{{ID: "a"}, {ID: "b"}, {ID: "c"}}},
				state:  ticketReserved,
			},
			"confirmed": {
				ticket: api.MatchTicket{EnqueuedAt: now.Add(-time.Minute), Players: []api.Player{{ID: "done"}}},
				state:  ticketConfirmed,
			},
		},
	}
	if wait := model.averageQueueWait(); wait != 5_500*time.Millisecond {
		t.Fatalf("average queue wait = %s; want 5.5s", wait)
	}
}

func TestTrendSampleReplacesSameTimestampAndUsesCenteredRatingHistogram(t *testing.T) {
	now := time.Date(2026, time.January, 1, 0, 1, 0, 0, time.UTC)
	model := &Model{now: now, tickets: make(map[string]*ticketView)}
	model.population = league.Stats{Players: 10, CenteredHistogram: [9]int{0, 0, 0, 5, 0, 5}}
	model.recordTrendSample()
	model.population.CenteredHistogram = [9]int{0, 0, 0, 4, 2, 4}
	model.recordTrendSample()
	if len(model.trends) != 1 || model.trends[0].ratingHistogram != model.population.CenteredHistogram {
		t.Fatalf("same-timestamp trend sample was not replaced: %#v", model.trends)
	}
	model.now = model.now.Add(time.Second)
	model.recordTrendSample()
	if len(model.trends) != 2 {
		t.Fatalf("new timestamp was not appended: %#v", model.trends)
	}
	for range maxTrendSamples {
		model.now = model.now.Add(time.Second)
		model.recordTrendSample()
	}
	if len(model.trends) != maxTrendSamples || !model.trends[0].at.Equal(now.Add(2*time.Second)) {
		t.Fatalf("trend history was not bounded to the newest %d samples", maxTrendSamples)
	}
}
