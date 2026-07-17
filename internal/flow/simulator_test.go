package flow_test

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/zrma/sema/internal/flow"
)

func TestSimulatorRunsClosedHTTPLeagueLifecycle(t *testing.T) {
	simulator := openSimulator(t, 73)
	initial := simulator.Snapshot()
	if initial.Population.Players != 40 || initial.QueuePlayers != 0 || initial.QueueTickets != 0 ||
		initial.IdlePlayers != 40 || initial.InGamePlayers != 0 || initial.CooldownPlayers != 0 {
		t.Fatalf("initial state = %#v", initial)
	}

	seen := make(map[flow.EventKind]int)
	var completedRange int
	var returnedRevision uint64
	var returnedRatingChanged bool
	maximumConcurrent := 0
	sawIdleAndQueue := false
	sawQueueWhilePlaying := false
	sawCooldown := false
	for range 500 {
		event, err := simulator.Step(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		seen[event.Kind]++
		if total := event.IdlePlayers + event.QueuePlayers + event.InGamePlayers + event.CooldownPlayers; total != 40 {
			t.Fatalf("population conservation = %d after %s: %#v", total, event.Kind, event)
		}
		maximumConcurrent = max(maximumConcurrent, event.ActiveMatches)
		sawIdleAndQueue = sawIdleAndQueue || event.IdlePlayers > 0 && event.QueuePlayers > 0
		sawQueueWhilePlaying = sawQueueWhilePlaying || event.QueuePlayers > 0 && event.InGamePlayers > 0
		sawCooldown = sawCooldown || event.CooldownPlayers > 0
		if event.Kind == flow.EventPlanCompleted && (event.Batch == nil || len(event.Batch.Proposals) != 2) {
			t.Fatalf("plan event = %#v", event)
		}
		if event.Kind == flow.EventMatchCompleted {
			if event.Result == nil || event.Result.RatingDelta[0]+event.Result.RatingDelta[1] != 0 {
				t.Fatalf("match result = %#v", event.Result)
			}
			completedRange = event.Population.Maximum - event.Population.Minimum
		}
		if event.Kind == flow.EventTicketReturned && event.Ticket != nil {
			returnedRevision = event.Ticket.Revision
			for _, player := range event.Ticket.Players {
				returnedRatingChanged = returnedRatingChanged || player.Skill != 1500
			}
		}
		if seen[flow.EventMatchCompleted] >= 2 && returnedRevision >= 2 && returnedRatingChanged {
			break
		}
	}

	for _, kind := range []flow.EventKind{
		flow.EventPlanCompleted,
		flow.EventTicketQueued,
		flow.EventProposalReserved,
		flow.EventAssignmentConfirmed,
		flow.EventTimeAdvanced,
		flow.EventMatchCompleted,
		flow.EventTicketReturned,
	} {
		if seen[kind] == 0 {
			t.Fatalf("lifecycle event %q was not emitted: %#v", kind, seen)
		}
	}
	if completedRange == 0 || returnedRevision < 2 || !returnedRatingChanged || maximumConcurrent < 2 ||
		!sawIdleAndQueue || !sawQueueWhilePlaying || !sawCooldown {
		t.Fatalf(
			"rating range = %d, returned revision = %d, changed = %v, concurrent = %d, idle+queue = %v, queue+playing = %v, cooldown = %v",
			completedRange,
			returnedRevision,
			returnedRatingChanged,
			maximumConcurrent,
			sawIdleAndQueue,
			sawQueueWhilePlaying,
			sawCooldown,
		)
	}
}

func TestSimulatorMatchSequenceIsDeterministic(t *testing.T) {
	first := openSimulator(t, 91)
	second := openSimulator(t, 91)

	left := nextResult(t, first)
	right := nextResult(t, second)
	if !reflect.DeepEqual(left, right) {
		t.Fatalf("deterministic results differ:\nleft=%#v\nright=%#v", left, right)
	}
}

func TestSimulatorProcessesDueArrivalWithoutAdvancingClock(t *testing.T) {
	configuration := flow.DefaultConfig()
	simulator, err := flow.Open(configuration)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := simulator.Close(); err != nil {
			t.Error(err)
		}
	})

	event, err := simulator.Step(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if event.Kind != flow.EventTicketQueued || !event.At.Equal(configuration.Start) || event.Ticket == nil || !event.Ticket.EnqueuedAt.Equal(configuration.Start) {
		t.Fatalf("first scheduled arrival = %#v", event)
	}
}

func TestSimulatorRejectsInvalidConfiguration(t *testing.T) {
	if _, err := flow.Open(flow.Config{Seed: -1}); err == nil {
		t.Fatal("negative seed was accepted")
	}
	if _, err := flow.Open(flow.Config{PopulationSize: 10, MatchesPerCycle: 2}); err == nil {
		t.Fatal("workload larger than the population was accepted")
	}
}

func openSimulator(t *testing.T, seed int64) *flow.Simulator {
	t.Helper()
	configuration := flow.DefaultConfig()
	configuration.Seed = seed
	configuration.PopulationSize = 40
	configuration.MatchesPerCycle = 2
	configuration.MaxConcurrentMatches = 4
	configuration.GameDuration = 20 * time.Second
	configuration.ArrivalInterval = time.Second
	configuration.PlanningInterval = 2 * time.Second
	configuration.MaxReturnDelay = 10 * time.Second
	configuration.TickDuration = time.Second
	simulator, err := flow.Open(configuration)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := simulator.Close(); err != nil {
			t.Error(err)
		}
	})
	return simulator
}

func nextResult(t *testing.T, simulator *flow.Simulator) any {
	t.Helper()
	for range 300 {
		event, err := simulator.Step(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if event.Result != nil {
			return *event.Result
		}
	}
	t.Fatal("simulator did not complete a match")
	return nil
}
