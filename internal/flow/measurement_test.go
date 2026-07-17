package flow

import (
	"context"
	"reflect"
	"strings"
	"testing"
	"time"

	api "github.com/zrma/sema/internal/api/v0alpha1"
	"github.com/zrma/sema/internal/league"
)

func TestMeasureIsDeterministicAndConservesPopulation(t *testing.T) {
	configuration := DefaultConfig()
	configuration.PopulationSize = 40
	configuration.MatchesPerCycle = 2
	configuration.GameDuration = 20 * time.Second
	configuration.PlanningInterval = 2 * time.Second
	configuration.MaxReturnDelay = 10 * time.Second

	first, err := Measure(context.Background(), configuration, 2*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	second, err := Measure(context.Background(), configuration, 2*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("deterministic reports differ:\nfirst=%#v\nsecond=%#v", first, second)
	}
	if first.SchemaVersion != MeasurementSchemaVersion || first.Assignments.Matches == 0 || first.Completions.Matches == 0 {
		t.Fatalf("measurement did not exercise the closed loop: %#v", first)
	}
	if first.Wait.SamplesPlayers != first.Assignments.Players {
		t.Fatalf("wait samples = %d, assigned players = %d", first.Wait.SamplesPlayers, first.Assignments.Players)
	}
	finalPlayers := first.Final.IdlePlayers + first.Final.QueuedPlayers + first.Final.IngressBacklogPlayers + first.Final.InGamePlayers + first.Final.CooldownPlayers
	if finalPlayers != configuration.PopulationSize {
		t.Fatalf("final population = %d, want %d", finalPlayers, configuration.PopulationSize)
	}
	if first.Queue.MeanPlayers <= 0 || first.Queue.P95Players < first.Queue.MeanPlayers || first.Queue.PeakPlayers < first.Queue.P95Players {
		t.Fatalf("queue measurement = %#v", first.Queue)
	}
	if first.Ingress != (IngressMeasurement{SamplesTickets: 96}) {
		t.Fatalf("ingress measurement = %#v", first.Ingress)
	}
	if first.Steps != 264 || first.Cycles != 8 || first.QueueEntries != (EntryCounts{Tickets: 96, Players: 160, InitialTickets: 24, ReturnedTickets: 72}) ||
		first.Assignments != (MatchCounts{Matches: 14, Tickets: 84, Players: 140}) || first.Completions != (MatchCounts{Matches: 12, Tickets: 72, Players: 120}) ||
		first.AssignmentYieldBasisPoints != 8_750 || first.Wait != (DurationDistribution{SamplesPlayers: 140, P50Millis: 5_000, P90Millis: 12_000, P99Millis: 13_000, MaxMillis: 13_000}) ||
		first.Queue != (QueueMeasurement{MeanPlayers: 7, P95Players: 20, PeakPlayers: 25, MeanSaturationBasisPoints: 1_866, P95SaturationBasisPoints: 5_000, PeakSaturationBasisPoints: 6_250}) {
		t.Fatalf("reference measurement changed: %#v", first)
	}
}

func TestMeasurementRecorderWeightsPlayersAndSimulatedDuration(t *testing.T) {
	start := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)
	configuration := DefaultConfig()
	configuration.PopulationSize = 4
	recorder := newMeasurementRecorder(configuration, 6*time.Second, State{
		Now: start, IdlePlayers: 4, Population: league.Stats{Players: 4, Parties: 2, Minimum: 1500, Maximum: 1500, Mean: 1500},
	})

	solo := api.MatchTicket{ID: "solo", EnqueuedAt: start.Add(time.Second), Players: []api.Player{{ID: "solo-player"}}}
	trio := api.MatchTicket{
		ID: "trio", EnqueuedAt: start.Add(2 * time.Second),
		Players: []api.Player{{ID: "trio-1"}, {ID: "trio-2"}, {ID: "trio-3"}},
	}
	events := []Event{
		{Kind: EventTicketQueued, At: start.Add(time.Second), Ticket: &solo, QueuePlayers: 1, IdlePlayers: 3},
		{Kind: EventTicketQueued, At: start.Add(2 * time.Second), Ticket: &trio, QueuePlayers: 4},
		{
			Kind: EventAssignmentConfirmed, At: start.Add(5 * time.Second), QueuePlayers: 0, InGamePlayers: 4,
			Proposal: &api.MatchProposal{
				ID: "match", Tickets: []api.TicketRef{{ID: solo.ID}, {ID: trio.ID}},
				Evidence: api.ScoreEvidence{TeamSkillGap: 12, MaxLatencyMillis: 48},
			},
		},
		{Kind: EventTimeAdvanced, At: start.Add(6 * time.Second), InGamePlayers: 4},
	}
	for _, event := range events {
		if err := recorder.observe(event); err != nil {
			t.Fatal(err)
		}
	}
	report := recorder.report()
	if report.Wait != (DurationDistribution{SamplesPlayers: 4, P50Millis: 3_000, P90Millis: 4_000, P99Millis: 4_000, MaxMillis: 4_000}) {
		t.Fatalf("wait distribution = %#v", report.Wait)
	}
	if report.Queue.MeanPlayers != 2 || report.Queue.P95Players != 4 || report.Queue.PeakPlayers != 4 || report.Queue.MeanSaturationBasisPoints != 5_416 {
		t.Fatalf("time-weighted queue = %#v", report.Queue)
	}
	if report.AssignmentYieldBasisPoints != 10_000 || report.Quality.TeamSkillGap.P50 != 12 || report.Quality.MaxLatencyMillis.Max != 48 {
		t.Fatalf("assignment and quality report = %#v", report)
	}
}

func TestMeasureRejectsShortDuration(t *testing.T) {
	_, err := Measure(context.Background(), DefaultConfig(), 999*time.Millisecond)
	if err == nil || !strings.Contains(err.Error(), "at least one second") {
		t.Fatalf("error = %v", err)
	}
}

func TestMeasureDrainsScheduledIngressAtHorizon(t *testing.T) {
	configuration := DefaultConfig()
	configuration.PopulationSize = 100
	configuration.MatchesPerCycle = 8
	configuration.ArrivalInterval = 100 * time.Millisecond
	configuration.PlanningInterval = time.Second

	report, err := Measure(context.Background(), configuration, 6*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if report.QueueEntries.InitialTickets != 60 || report.Ingress.SamplesTickets != 60 {
		t.Fatalf("scheduled initial ingress was not fully observed: %#v", report)
	}
	if report.Ingress.MaxArrivalLagMillis != 0 || report.Ingress.FinalBacklogTickets != 0 || report.Ingress.FinalBacklogPlayers != 0 {
		t.Fatalf("ingress scheduler lagged at the measurement horizon: %#v", report.Ingress)
	}
}
