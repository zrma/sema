package flowui

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"

	api "github.com/zrma/sema/internal/api/v0alpha1"
	"github.com/zrma/sema/internal/flow"
)

func TestSelectedPartiesHoldDepartTogetherAndCloseQueueGap(t *testing.T) {
	model := departureFixture(false)
	proposal := api.MatchProposal{
		ID: "flow-snapshot-0001/p0001/example",
		Tickets: []api.TicketRef{
			{ID: "ticket-a", Revision: 1},
			{ID: "ticket-b", Revision: 1},
		},
	}
	model.apply(flow.Event{
		Kind:  flow.EventPlanCompleted,
		At:    model.now,
		Cycle: 1,
		Batch: &api.ProposalBatch{Proposals: []api.MatchProposal{proposal}},
	})

	for _, identifier := range []string{"ticket-a", "ticket-b"} {
		ticket := model.tickets[identifier]
		if ticket.state != ticketProposed || !ticket.leaving || ticket.hidden || ticket.matchVisual != 0 {
			t.Fatalf("selected ticket %s did not enter the shared departure state: %#v", identifier, ticket)
		}
	}
	content := strings.Join(model.waitingLines(model.glyphs(), 6), "\n")
	if strings.Count(content, "①") != 2 || !strings.Contains(content, "ticket-a") || !strings.Contains(content, "ticket-b") {
		t.Fatalf("selected parties do not share the first match marker:\n%s", content)
	}
	if lifecycle := strings.Join(model.activeLines(model.glyphs(), 6), "\n"); !strings.Contains(lifecycle, "①") {
		t.Fatalf("lifecycle does not preserve the waiting-pool match marker:\n%s", lifecycle)
	}

	for range selectionHoldFrames {
		model.animate()
	}
	if lane := model.waitingLane(model.tickets["ticket-a"], model.glyphs()); !strings.Contains(lane, "······[●]▷") {
		t.Fatalf("selected party moved during its hold phase: %q", lane)
	}
	model.animate()
	if lane := model.waitingLane(model.tickets["ticket-a"], model.glyphs()); !strings.Contains(lane, "·······[●]▷") {
		t.Fatalf("selected party did not move right after the hold phase: %q", lane)
	}

	for model.tickets["ticket-a"].selectionFrame < selectionHoldFrames+departureTravelFrames-1 {
		model.animate()
	}
	if model.tickets["ticket-a"].hidden {
		t.Fatal("selected party disappeared before completing its horizontal departure")
	}
	model.animate()
	if !model.tickets["ticket-a"].hidden || !model.tickets["ticket-b"].hidden {
		t.Fatal("selected parties did not leave together")
	}
	if row := model.tickets["ticket-c"].queueRow; row != 1 {
		t.Fatalf("first compaction frame moved the following row to %d; want 1", row)
	}
	model.animate()
	if row := model.tickets["ticket-c"].queueRow; row != 0 {
		t.Fatalf("second compaction frame moved the following row to %d; want 0", row)
	}
}

func TestReducedMotionRemovesSelectionAndCompactsImmediately(t *testing.T) {
	model := departureFixture(true)
	model.apply(flow.Event{
		Kind:  flow.EventPlanCompleted,
		At:    model.now,
		Cycle: 1,
		Batch: &api.ProposalBatch{Proposals: []api.MatchProposal{{
			ID:      "flow-snapshot-0001/p0001/example",
			Tickets: []api.TicketRef{{ID: "ticket-a", Revision: 1}, {ID: "ticket-b", Revision: 1}},
		}}},
	})

	content := strings.Join(model.waitingLines(model.glyphs(), 6), "\n")
	if strings.Contains(content, "ticket-a") || strings.Contains(content, "ticket-b") {
		t.Fatalf("reduced motion retained departing rows:\n%s", content)
	}
	if row := model.tickets["ticket-c"].queueRow; row != 0 {
		t.Fatalf("reduced motion left the remaining row at %d; want 0", row)
	}
}

func TestWaitingAndLifecycleUseTheSameMatchAccent(t *testing.T) {
	model := departureFixture(false)
	model.options.Color = true
	model.apply(flow.Event{
		Kind:  flow.EventPlanCompleted,
		At:    model.now,
		Cycle: 1,
		Batch: &api.ProposalBatch{Proposals: []api.MatchProposal{{
			ID:      "flow-snapshot-0001/p0001/example",
			Tickets: []api.TicketRef{{ID: "ticket-a", Revision: 1}, {ID: "ticket-b", Revision: 1}},
		}}},
	})

	accent := matchVisualColor(model.active["flow-snapshot-0001/p0001/example"].matchVisual)
	waiting := model.waitingLines(model.glyphs(), 6)[0]
	if expected := model.paint(ansi.Strip(waiting), accent); waiting != expected {
		t.Fatalf("waiting row does not use the match accent: got %q want %q", waiting, expected)
	}
	for _, line := range model.activeLines(model.glyphs(), 6) {
		if expected := model.paint(ansi.Strip(line), accent); line != expected {
			t.Fatalf("lifecycle row does not use the waiting match accent: got %q want %q", line, expected)
		}
	}
}

func TestActiveMatchesReceiveUniqueVisualSlotsAndColors(t *testing.T) {
	model := departureFixture(false)
	model.active = make(map[string]*matchView)
	colors := make(map[string]struct{})
	markers := make(map[string]struct{})
	for index := range 80 {
		visual := model.allocateMatchVisual()
		color := matchVisualColor(visual)
		marker := matchVisualMarker(visual, true)
		if _, duplicated := colors[color]; duplicated {
			t.Fatalf("active visual %d reused color %s", visual, color)
		}
		if _, duplicated := markers[marker]; duplicated {
			t.Fatalf("active visual %d reused marker %s", visual, marker)
		}
		colors[color] = struct{}{}
		markers[marker] = struct{}{}
		model.active[fmt.Sprintf("match-%02d", index)] = &matchView{matchVisual: visual}
	}
}

func departureFixture(reducedMotion bool) *Model {
	now := time.Date(2026, time.January, 1, 0, 1, 0, 0, time.UTC)
	options := DefaultOptions()
	options.Color = false
	options.ReducedMotion = reducedMotion
	model := &Model{
		options: options,
		now:     now,
		tickets: make(map[string]*ticketView),
		active:  make(map[string]*matchView),
	}
	for row, identifier := range []string{"ticket-a", "ticket-b", "ticket-c"} {
		ticket := api.MatchTicket{
			ID: identifier, Revision: 1, EnqueuedAt: now.Add(-time.Minute),
			Players: []api.Player{{ID: identifier + "-player", Skill: 1500, LatencyMillis: 30}},
		}
		model.tickets[identifier] = &ticketView{
			ticket: ticket, state: ticketQueued, position: 6, queueRow: row, matchVisual: -1,
		}
		model.ticketOrder = append(model.ticketOrder, identifier)
	}
	return model
}
