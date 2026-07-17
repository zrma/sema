package flowui

import (
	"strings"
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

func TestRatingDensityExpandsBandsAcrossAvailableHeight(t *testing.T) {
	now := time.Date(2026, time.January, 1, 0, 1, 0, 0, time.UTC)
	options := DefaultOptions()
	options.Color = false
	model := &Model{
		options: options,
		trends: []trendSample{{
			at: now, population: 100,
			ratingHistogram: [9]int{0, 0, 0, 0, 100},
		}},
	}

	lines := model.ratingDensityLines(model.glyphs(), 60, 18)
	if len(lines) != 18 {
		t.Fatalf("rating density lines = %d; want full height 18", len(lines))
	}
	joined := strings.Join(lines, "\n")
	labelCounts := make(map[string]int)
	for _, line := range lines {
		label, _, _ := strings.Cut(line, "│")
		if label = strings.TrimSpace(label); label != "" {
			labelCounts[label]++
		}
	}
	for _, label := range []string{"<1400", "1400", "1450", "1475", "1500", "1501", "1526", "1551", ">1600"} {
		if count := labelCounts[label]; count != 1 {
			t.Fatalf("rating label %q appeared %d times; want once:\n%s", label, count, joined)
		}
	}
	centerRows := 0
	for _, line := range lines {
		if strings.Contains(line, "█") {
			centerRows++
		}
	}
	if centerRows != 2 {
		t.Fatalf("1500 density occupied %d rows; want 2 expanded rows:\n%s", centerRows, joined)
	}

	model.options.Unicode = false
	asciiLines := model.ratingDensityLines(model.glyphs(), 60, 18)
	if len(asciiLines) != 18 || strings.Count(strings.Join(asciiLines, "\n"), "%") != 2 {
		t.Fatalf("ASCII density did not use the same vertical allocation:\n%s", strings.Join(asciiLines, "\n"))
	}

	model.options.Unicode = true
	model.trends[0].ratingHistogram = [9]int{0, 0, 0, 100}
	emptyCenter := strings.Join(model.ratingDensityLines(model.glyphs(), 60, 18), "\n")
	if axisRows := strings.Count(emptyCenter, "─"); axisRows != 1 {
		t.Fatalf("expanded empty 1500 bucket rendered %d reference axes; want one:\n%s", axisRows, emptyCenter)
	}
}
