package flow_test

import (
	"context"
	"reflect"
	"slices"
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
		if total := event.IdlePlayers + event.QueuePlayers + event.IngressBacklogPlayers + event.InGamePlayers + event.CooldownPlayers; total != 40 {
			t.Fatalf("population conservation = %d after %s: %#v", total, event.Kind, event)
		}
		maximumConcurrent = max(maximumConcurrent, event.ActiveMatches)
		sawIdleAndQueue = sawIdleAndQueue || event.IdlePlayers > 0 && event.QueuePlayers > 0
		sawQueueWhilePlaying = sawQueueWhilePlaying || event.QueuePlayers > 0 && event.InGamePlayers > 0
		sawCooldown = sawCooldown || event.CooldownPlayers > 0
		if event.Kind == flow.EventPlanCompleted && (event.Batch == nil || len(event.Batch.Proposals) == 0 || len(event.Batch.Proposals) > 2) {
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

	advanced, err := simulator.Step(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	scheduledAt := configuration.Start.Add(configuration.ArrivalInterval)
	if advanced.Kind != flow.EventTimeAdvanced || !advanced.At.Equal(scheduledAt) || advanced.IngressBacklogTickets != 1 {
		t.Fatalf("arrival scheduling event = %#v", advanced)
	}
	event, err := simulator.Step(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if event.Kind != flow.EventTicketQueued || !event.At.Equal(scheduledAt) || !event.ArrivalScheduledAt.Equal(scheduledAt) ||
		event.Ticket == nil || !event.Ticket.EnqueuedAt.Equal(scheduledAt) || event.IngressBacklogTickets != 0 {
		t.Fatalf("first scheduled arrival = %#v", event)
	}
}

func TestDefaultConfigUsesBulkProposalUpperBound(t *testing.T) {
	configuration := flow.DefaultConfig()
	if configuration.MatchesPerCycle != 32 || configuration.PlanningInterval != 5*time.Second {
		t.Fatalf("default planning admission = %d proposals every %s", configuration.MatchesPerCycle, configuration.PlanningInterval)
	}
}

func TestSimulatorRejectsInvalidConfiguration(t *testing.T) {
	if _, err := flow.Open(flow.Config{Seed: -1}); err == nil {
		t.Fatal("negative seed was accepted")
	}
	if _, err := flow.Open(flow.Config{MatchesPerCycle: flow.MaximumMatchesPerCycle + 1}); err == nil {
		t.Fatal("unbounded planning batch was accepted")
	}
}

func TestSimulatorPlansPartialBatchBelowConfiguredLimit(t *testing.T) {
	configuration := flow.DefaultConfig()
	configuration.PopulationSize = 40
	configuration.MatchesPerCycle = 32
	configuration.ArrivalInterval = 100 * time.Millisecond
	configuration.PlanningInterval = time.Second
	configuration.GameDuration = 20 * time.Second
	configuration.MaxReturnDelay = 10 * time.Second
	simulator, err := flow.Open(configuration)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := simulator.Close(); err != nil {
			t.Error(err)
		}
	})

	for range 200 {
		event, stepErr := simulator.Step(context.Background())
		if stepErr != nil {
			t.Fatal(stepErr)
		}
		if event.Kind != flow.EventPlanCompleted {
			continue
		}
		if event.Batch == nil || len(event.Batch.Proposals) == 0 || len(event.Batch.Proposals) >= configuration.MatchesPerCycle {
			t.Fatalf("partial batch = %#v; want 1..31 proposals", event.Batch)
		}
		if event.MaxProposals != 32 || event.PlanningInterval != time.Second {
			t.Fatalf("planning contract = max %d interval %s", event.MaxProposals, event.PlanningInterval)
		}
		return
	}
	t.Fatal("simulator never planned a partial batch")
}

func TestSimulatorPlansDozensOfMatchesFromBacklog(t *testing.T) {
	configuration := flow.DefaultConfig()
	configuration.PopulationSize = 400
	configuration.MatchesPerCycle = 32
	configuration.ArrivalInterval = time.Millisecond
	configuration.PlanningInterval = 5 * time.Second
	simulator := openSimulatorWithConfig(t, configuration)

	for range 1_000 {
		event, err := simulator.Step(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if event.Kind != flow.EventPlanCompleted {
			continue
		}
		if event.Batch == nil {
			t.Fatal("backlogged plan omitted its batch")
		}
		if len(event.Batch.Proposals) == 1 {
			continue
		}
		if len(event.Batch.Proposals) != configuration.MatchesPerCycle {
			t.Fatalf("backlogged plan contained %d proposals; want %d", len(event.Batch.Proposals), configuration.MatchesPerCycle)
		}
		return
	}
	t.Fatal("simulator never planned a full backlogged batch")
}

func TestSimulatorPlanningContinuesAboveEightActiveGames(t *testing.T) {
	configuration := flow.DefaultConfig()
	configuration.PopulationSize = 100
	configuration.MatchesPerCycle = 2
	configuration.GameDuration = 30 * time.Second
	configuration.ArrivalInterval = 100 * time.Millisecond
	configuration.PlanningInterval = time.Second
	configuration.MaxReturnDelay = 10 * time.Second
	simulator, err := flow.Open(configuration)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := simulator.Close(); err != nil {
			t.Error(err)
		}
	})

	maximumActive := 0
	plannedAboveEight := false
	for range 600 {
		event, err := simulator.Step(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		maximumActive = max(maximumActive, event.ActiveMatches)
		plannedAboveEight = plannedAboveEight || event.Kind == flow.EventPlanCompleted && event.ActiveMatches >= 8
		if maximumActive > 8 && plannedAboveEight {
			break
		}
	}
	if maximumActive <= 8 || !plannedAboveEight {
		t.Fatalf("game execution still gated planning: max active=%d planned above eight=%v", maximumActive, plannedAboveEight)
	}
}

func TestSimulatorOrdersBatchStagesStablyAtOneTimestamp(t *testing.T) {
	configuration := flow.DefaultConfig()
	configuration.Seed = 101
	configuration.PopulationSize = 100
	configuration.MatchesPerCycle = 8
	configuration.GameDuration = 20 * time.Second
	configuration.ArrivalInterval = 100 * time.Millisecond
	configuration.PlanningInterval = 2 * time.Second
	configuration.MaxReturnDelay = 10 * time.Second
	configuration.TickDuration = time.Second
	simulator := openSimulatorWithConfig(t, configuration)
	groups := make(map[time.Time][]flow.Event)
	for range 400 {
		event, err := simulator.Step(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		groups[event.At] = append(groups[event.At], event)
	}

	sawArrivalBeforeStage := false
	sawMultiProposalStage := false
	for at, events := range groups {
		stageStarted := false
		hasArrival := false
		hasStage := false
		proposalIDs := make(map[flow.EventKind][]string)
		for _, event := range events {
			switch event.Kind {
			case flow.EventProposalReserved, flow.EventAssignmentConfirmed:
				stageStarted = true
				hasStage = true
				if event.Proposal == nil {
					t.Fatalf("%s stage at %s omitted proposal", event.Kind, at)
				}
				proposalIDs[event.Kind] = append(proposalIDs[event.Kind], event.Proposal.ID)
			case flow.EventTicketQueued, flow.EventTicketReturned:
				hasArrival = true
				if stageStarted {
					t.Fatalf("arrival %s followed a due batch stage at %s: %#v", event.Kind, at, events)
				}
			}
		}
		sawArrivalBeforeStage = sawArrivalBeforeStage || hasArrival && hasStage
		for kind, identifiers := range proposalIDs {
			if len(identifiers) > 1 {
				sawMultiProposalStage = true
			}
			if !slices.IsSorted(identifiers) {
				t.Fatalf("%s proposal order at %s = %v", kind, at, identifiers)
			}
		}
	}
	if !sawArrivalBeforeStage || !sawMultiProposalStage {
		t.Fatalf("ordering fixture did not exercise arrival and multi-proposal ties")
	}
}

func openSimulator(t *testing.T, seed int64) *flow.Simulator {
	t.Helper()
	configuration := flow.DefaultConfig()
	configuration.Seed = seed
	configuration.PopulationSize = 40
	configuration.MatchesPerCycle = 2
	configuration.GameDuration = 20 * time.Second
	configuration.ArrivalInterval = time.Second
	configuration.PlanningInterval = 2 * time.Second
	configuration.MaxReturnDelay = 10 * time.Second
	configuration.TickDuration = time.Second
	return openSimulatorWithConfig(t, configuration)
}

func openSimulatorWithConfig(t *testing.T, configuration flow.Config) *flow.Simulator {
	t.Helper()
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
