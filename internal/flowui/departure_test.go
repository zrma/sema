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
	if lifecycle := strings.Join(model.activeLines(model.glyphs(), 6), "\n"); strings.Contains(lifecycle, "①") {
		t.Fatalf("lifecycle entry appeared without a motion frame:\n%s", lifecycle)
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
	if lifecycle := strings.Join(model.activeLines(model.glyphs(), 6), "\n"); strings.Contains(lifecycle, "①") {
		t.Fatalf("lifecycle entry started before the selected parties finished departing:\n%s", lifecycle)
	}
	if row := model.tickets["ticket-c"].queueRow; row != 1 {
		t.Fatalf("first compaction frame moved the following row to %d; want 1", row)
	}
	model.animate()
	if row := model.tickets["ticket-c"].queueRow; row != 0 {
		t.Fatalf("second compaction frame moved the following row to %d; want 0", row)
	}
	if lifecycle := strings.Join(model.activeLines(model.glyphs(), 6), "\n"); !strings.Contains(lifecycle, "①") {
		t.Fatalf("lifecycle does not preserve the waiting-pool match marker while entering:\n%s", lifecycle)
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
	for range lifecycleEntryDelay + 1 {
		model.animate()
	}
	for _, line := range model.activeLines(model.glyphs(), 6) {
		if expected := model.paint(ansi.Strip(line), accent); line != expected {
			t.Fatalf("lifecycle row does not use the waiting match accent: got %q want %q", line, expected)
		}
	}
}

func TestLifecycleEntriesCascadeFromTopAndPushExistingMatchesDown(t *testing.T) {
	model := departureFixture(false)
	existing := lifecycleProposal("existing", "ticket-c")
	model.active[existing.ID] = &matchView{
		proposal: existing, stage: stagePlaying, motion: 8, entryFrame: lifecycleEntryFrames,
		partySizes: map[string]int{"ticket-c": 1}, matchVisual: 0,
	}
	model.activeOrder = append(model.activeOrder, existing.ID)

	first := lifecycleProposal("new-first", "ticket-a")
	second := lifecycleProposal("new-second", "ticket-b")
	model.apply(flow.Event{
		Kind: flow.EventPlanCompleted, At: model.now, Cycle: 1,
		Batch: &api.ProposalBatch{Proposals: []api.MatchProposal{first, second}},
	})
	model.active[first.ID].stage = stagePlaying
	model.active[second.ID].stage = stagePlaying

	initial := strings.Join(model.activeLines(model.glyphs(), 20), "\n")
	if strings.Contains(initial, "new-first") || strings.Contains(initial, "new-second") {
		t.Fatalf("new lifecycle blocks popped into the initial frame:\n%s", initial)
	}
	initialExistingRow := lineIndex(initial, "existing")

	for range lifecycleEntryDelay {
		model.animate()
	}
	delayed := strings.Join(model.activeLines(model.glyphs(), 20), "\n")
	if strings.Contains(delayed, "new-first") || lineIndex(delayed, "existing") != initialExistingRow {
		t.Fatalf("lifecycle layout changed before queue departure completed:\n%s", delayed)
	}
	model.animate()
	firstFrame := strings.Join(model.activeLines(model.glyphs(), 20), "\n")
	if !strings.Contains(firstFrame, "new-first") || strings.Contains(firstFrame, "new-second") {
		t.Fatalf("first entry frame did not start the batch from the top:\n%s", firstFrame)
	}
	if row := lineIndex(firstFrame, "existing"); row <= initialExistingRow {
		t.Fatalf("existing lifecycle row did not move down: got %d after %d", row, initialExistingRow)
	}

	model.animate()
	model.animate()
	staggered := strings.Join(model.activeLines(model.glyphs(), 20), "\n")
	if !strings.Contains(staggered, "new-first") || !strings.Contains(staggered, "new-second") {
		t.Fatalf("second lifecycle block did not follow the staggered cascade:\n%s", staggered)
	}
	if row := lineIndex(staggered, "existing"); row <= lineIndex(firstFrame, "existing") {
		t.Fatalf("existing lifecycle row stopped moving down during cascade: got %d", row)
	}

	model.animateToEnd()
	completed := strings.Join(model.activeLines(model.glyphs(), 20), "\n")
	if firstRow, secondRow, existingRow := lineIndex(completed, "new-first"), lineIndex(completed, "new-second"), lineIndex(completed, "existing"); firstRow < 0 || secondRow <= firstRow || existingRow <= secondRow {
		t.Fatalf("completed lifecycle order is not top-to-bottom: first=%d second=%d existing=%d\n%s", firstRow, secondRow, existingRow, completed)
	}
}

func TestReducedMotionPlacesLifecycleEntriesImmediately(t *testing.T) {
	model := departureFixture(true)
	first := lifecycleProposal("new-first", "ticket-a")
	second := lifecycleProposal("new-second", "ticket-b")
	model.apply(flow.Event{
		Kind: flow.EventPlanCompleted, At: model.now, Cycle: 1,
		Batch: &api.ProposalBatch{Proposals: []api.MatchProposal{first, second}},
	})

	lifecycle := strings.Join(model.activeLines(model.glyphs(), 20), "\n")
	if firstRow, secondRow := lineIndex(lifecycle, "new-first"), lineIndex(lifecycle, "new-second"); firstRow < 0 || secondRow <= firstRow {
		t.Fatalf("reduced motion did not apply the final lifecycle layout immediately:\n%s", lifecycle)
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

func lifecycleProposal(identifier, ticketID string) api.MatchProposal {
	reference := api.TicketRef{ID: ticketID, Revision: 1}
	return api.MatchProposal{
		ID:      identifier,
		Tickets: []api.TicketRef{reference},
		Teams:   []api.TeamAssignment{{Team: 0, Tickets: []api.TicketRef{reference}}, {Team: 1}},
	}
}

func lineIndex(content, needle string) int {
	for index, line := range strings.Split(content, "\n") {
		if strings.Contains(line, needle) {
			return index
		}
	}
	return -1
}
