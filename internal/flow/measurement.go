package flow

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/zrma/sema/internal/league"
)

const MeasurementSchemaVersion = "sema.flow.measurement.v0alpha1"

// MeasurementReport is a deterministic aggregate of one bounded Flow run.
type MeasurementReport struct {
	SchemaVersion              string                   `json:"schema_version"`
	Seed                       int64                    `json:"seed"`
	DurationMillis             int64                    `json:"duration_millis"`
	PopulationPlayers          int                      `json:"population_players"`
	Configuration              MeasurementConfiguration `json:"configuration"`
	Steps                      int                      `json:"steps"`
	Cycles                     int                      `json:"cycles"`
	QueueEntries               EntryCounts              `json:"queue_entries"`
	Assignments                MatchCounts              `json:"assignments"`
	Completions                MatchCounts              `json:"completions"`
	AssignmentYieldBasisPoints int                      `json:"assignment_yield_basis_points"`
	Throughput                 Throughput               `json:"throughput"`
	Wait                       DurationDistribution     `json:"wait"`
	Queue                      QueueMeasurement         `json:"queue"`
	Quality                    QualityMeasurement       `json:"quality"`
	Final                      FinalMeasurement         `json:"final"`
}

type MeasurementConfiguration struct {
	MatchesPerCycle        int   `json:"matches_per_cycle"`
	MaxConcurrentMatches   int   `json:"max_concurrent_matches"`
	ReservationTTLMillis   int64 `json:"reservation_ttl_millis"`
	GameDurationMillis     int64 `json:"game_duration_millis"`
	ArrivalIntervalMillis  int64 `json:"arrival_interval_millis"`
	PlanningIntervalMillis int64 `json:"planning_interval_millis"`
	MaxReturnDelayMillis   int64 `json:"max_return_delay_millis"`
	TickDurationMillis     int64 `json:"tick_duration_millis"`
}

type EntryCounts struct {
	Tickets         int `json:"tickets"`
	Players         int `json:"players"`
	InitialTickets  int `json:"initial_tickets"`
	ReturnedTickets int `json:"returned_tickets"`
}

type MatchCounts struct {
	Matches int `json:"matches"`
	Tickets int `json:"tickets"`
	Players int `json:"players"`
}

type Throughput struct {
	ConfirmedMatchesPerMinuteMilli int64 `json:"confirmed_matches_per_minute_milli"`
	CompletedMatchesPerMinuteMilli int64 `json:"completed_matches_per_minute_milli"`
}

type DurationDistribution struct {
	SamplesPlayers int   `json:"samples_players"`
	P50Millis      int64 `json:"p50_millis"`
	P90Millis      int64 `json:"p90_millis"`
	P99Millis      int64 `json:"p99_millis"`
	MaxMillis      int64 `json:"max_millis"`
}

type IntegerDistribution struct {
	Samples int `json:"samples"`
	P50     int `json:"p50"`
	P90     int `json:"p90"`
	P99     int `json:"p99"`
	Max     int `json:"max"`
}

type QueueMeasurement struct {
	MeanPlayers               int `json:"mean_players"`
	P95Players                int `json:"p95_players"`
	PeakPlayers               int `json:"peak_players"`
	MeanSaturationBasisPoints int `json:"mean_saturation_basis_points"`
	P95SaturationBasisPoints  int `json:"p95_saturation_basis_points"`
	PeakSaturationBasisPoints int `json:"peak_saturation_basis_points"`
}

type QualityMeasurement struct {
	TeamSkillGap     IntegerDistribution `json:"team_skill_gap"`
	MaxLatencyMillis IntegerDistribution `json:"max_latency_millis"`
}

type FinalMeasurement struct {
	IdlePlayers     int               `json:"idle_players"`
	QueuedPlayers   int               `json:"queued_players"`
	InGamePlayers   int               `json:"in_game_players"`
	CooldownPlayers int               `json:"cooldown_players"`
	Rating          RatingMeasurement `json:"rating"`
}

type RatingMeasurement struct {
	GamesPlayed  int    `json:"games_played"`
	Minimum      int    `json:"minimum"`
	Percentile10 int    `json:"percentile_10"`
	Median       int    `json:"median"`
	Percentile90 int    `json:"percentile_90"`
	Maximum      int    `json:"maximum"`
	Mean         int    `json:"mean"`
	StdDev       int    `json:"std_dev"`
	Histogram    [9]int `json:"histogram"`
}

type queuedMeasurement struct {
	enqueuedAt time.Time
	players    int
}

type weightedValue struct {
	value        int
	weightMillis int64
}

type measurementRecorder struct {
	configuration Config
	start         time.Time
	end           time.Time
	lastAt        time.Time
	lastQueue     int
	final         FinalMeasurement

	steps             int
	cycles            int
	entries           EntryCounts
	assignments       MatchCounts
	completions       MatchCounts
	queued            map[string]queuedMeasurement
	activePlayers     map[string]int
	waits             []int64
	skillGaps         []int
	latencies         []int
	queueOccupancy    []weightedValue
	queuePlayerMillis int64
	peakQueuePlayers  int
}

// Measure runs one isolated Flow simulation for a fixed amount of simulated time.
func Measure(ctx context.Context, configuration Config, duration time.Duration) (report MeasurementReport, resultErr error) {
	if duration < time.Second {
		return MeasurementReport{}, fmt.Errorf("measurement duration must be at least one second")
	}
	normalized, err := normalizeConfig(configuration)
	if err != nil {
		return MeasurementReport{}, err
	}
	simulator, err := Open(normalized)
	if err != nil {
		return MeasurementReport{}, err
	}
	defer func() {
		resultErr = errors.Join(resultErr, simulator.Close())
	}()

	initial := simulator.Snapshot()
	recorder := newMeasurementRecorder(normalized, duration, initial)
	now := initial.Now
	for now.Before(recorder.end) {
		if err := ctx.Err(); err != nil {
			return MeasurementReport{}, err
		}
		event, err := simulator.Step(ctx)
		if err != nil {
			return MeasurementReport{}, err
		}
		if err := recorder.observe(event); err != nil {
			return MeasurementReport{}, err
		}
		now = event.At
	}
	return recorder.report(), nil
}

func newMeasurementRecorder(configuration Config, duration time.Duration, initial State) *measurementRecorder {
	recorder := &measurementRecorder{
		configuration: configuration,
		start:         initial.Now,
		end:           initial.Now.Add(duration),
		lastAt:        initial.Now,
		lastQueue:     initial.QueuePlayers,
		queued:        make(map[string]queuedMeasurement),
		activePlayers: make(map[string]int),
	}
	recorder.final = finalMeasurement(initial.IdlePlayers, initial.QueuePlayers, initial.InGamePlayers, initial.CooldownPlayers, initial.Population)
	recorder.peakQueuePlayers = initial.QueuePlayers
	return recorder
}

func (recorder *measurementRecorder) observe(event Event) error {
	if event.At.Before(recorder.lastAt) {
		return fmt.Errorf("measurement event time moved backwards")
	}
	intervalEnd := event.At
	if intervalEnd.After(recorder.end) {
		intervalEnd = recorder.end
	}
	if intervalEnd.After(recorder.lastAt) {
		weight := intervalEnd.Sub(recorder.lastAt).Milliseconds()
		recorder.queueOccupancy = append(recorder.queueOccupancy, weightedValue{value: recorder.lastQueue, weightMillis: weight})
		recorder.queuePlayerMillis += int64(recorder.lastQueue) * weight
		recorder.lastAt = intervalEnd
	}
	if event.At.After(recorder.end) {
		return nil
	}

	recorder.steps++
	recorder.cycles = max(recorder.cycles, event.Cycle)
	switch event.Kind {
	case EventTicketQueued, EventTicketReturned:
		if event.Ticket == nil {
			return fmt.Errorf("queue event omitted ticket")
		}
		if _, exists := recorder.queued[event.Ticket.ID]; exists {
			return fmt.Errorf("ticket %q entered measurement queue twice", event.Ticket.ID)
		}
		players := len(event.Ticket.Players)
		recorder.queued[event.Ticket.ID] = queuedMeasurement{enqueuedAt: event.Ticket.EnqueuedAt, players: players}
		recorder.entries.Tickets++
		recorder.entries.Players += players
		if event.Kind == EventTicketReturned {
			recorder.entries.ReturnedTickets++
		} else {
			recorder.entries.InitialTickets++
		}
	case EventAssignmentConfirmed:
		if event.Proposal == nil {
			return fmt.Errorf("assignment event omitted proposal")
		}
		matchedPlayers := 0
		for _, reference := range event.Proposal.Tickets {
			entry, exists := recorder.queued[reference.ID]
			if !exists {
				return fmt.Errorf("assigned ticket %q was not observed in queue", reference.ID)
			}
			wait := max(int64(0), event.At.Sub(entry.enqueuedAt).Milliseconds())
			for range entry.players {
				recorder.waits = append(recorder.waits, wait)
			}
			matchedPlayers += entry.players
			delete(recorder.queued, reference.ID)
		}
		recorder.assignments.Matches++
		recorder.assignments.Tickets += len(event.Proposal.Tickets)
		recorder.assignments.Players += matchedPlayers
		recorder.activePlayers[event.Proposal.ID] = matchedPlayers
		recorder.skillGaps = append(recorder.skillGaps, event.Proposal.Evidence.TeamSkillGap)
		recorder.latencies = append(recorder.latencies, event.Proposal.Evidence.MaxLatencyMillis)
	case EventMatchCompleted:
		if event.Proposal == nil {
			return fmt.Errorf("completion event omitted proposal")
		}
		players, exists := recorder.activePlayers[event.Proposal.ID]
		if !exists {
			return fmt.Errorf("completed proposal %q was not observed as assigned", event.Proposal.ID)
		}
		delete(recorder.activePlayers, event.Proposal.ID)
		recorder.completions.Matches++
		recorder.completions.Tickets += len(event.Proposal.Tickets)
		recorder.completions.Players += players
	}

	recorder.lastQueue = event.QueuePlayers
	recorder.peakQueuePlayers = max(recorder.peakQueuePlayers, event.QueuePlayers)
	recorder.final = finalMeasurement(event.IdlePlayers, event.QueuePlayers, event.InGamePlayers, event.CooldownPlayers, event.Population)
	return nil
}

func (recorder *measurementRecorder) report() MeasurementReport {
	durationMillis := recorder.end.Sub(recorder.start).Milliseconds()
	report := MeasurementReport{
		SchemaVersion:     MeasurementSchemaVersion,
		Seed:              recorder.configuration.Seed,
		DurationMillis:    durationMillis,
		PopulationPlayers: recorder.configuration.PopulationSize,
		Configuration: MeasurementConfiguration{
			MatchesPerCycle:        recorder.configuration.MatchesPerCycle,
			MaxConcurrentMatches:   recorder.configuration.MaxConcurrentMatches,
			ReservationTTLMillis:   recorder.configuration.ReservationTTL.Milliseconds(),
			GameDurationMillis:     recorder.configuration.GameDuration.Milliseconds(),
			ArrivalIntervalMillis:  recorder.configuration.ArrivalInterval.Milliseconds(),
			PlanningIntervalMillis: recorder.configuration.PlanningInterval.Milliseconds(),
			MaxReturnDelayMillis:   recorder.configuration.MaxReturnDelay.Milliseconds(),
			TickDurationMillis:     recorder.configuration.TickDuration.Milliseconds(),
		},
		Steps:        recorder.steps,
		Cycles:       recorder.cycles,
		QueueEntries: recorder.entries,
		Assignments:  recorder.assignments,
		Completions:  recorder.completions,
		Wait:         durationDistribution(recorder.waits),
		Quality: QualityMeasurement{
			TeamSkillGap:     integerDistribution(recorder.skillGaps),
			MaxLatencyMillis: integerDistribution(recorder.latencies),
		},
		Final: recorder.final,
	}
	if recorder.entries.Players > 0 {
		report.AssignmentYieldBasisPoints = recorder.assignments.Players * 10_000 / recorder.entries.Players
	}
	report.Throughput = Throughput{
		ConfirmedMatchesPerMinuteMilli: matchesPerMinuteMilli(recorder.assignments.Matches, durationMillis),
		CompletedMatchesPerMinuteMilli: matchesPerMinuteMilli(recorder.completions.Matches, durationMillis),
	}
	p95Queue := weightedPercentile(recorder.queueOccupancy, 95)
	report.Queue = QueueMeasurement{
		MeanPlayers:               int(recorder.queuePlayerMillis / durationMillis),
		P95Players:                p95Queue,
		PeakPlayers:               recorder.peakQueuePlayers,
		MeanSaturationBasisPoints: int(recorder.queuePlayerMillis * 10_000 / (durationMillis * int64(recorder.configuration.PopulationSize))),
		P95SaturationBasisPoints:  p95Queue * 10_000 / recorder.configuration.PopulationSize,
		PeakSaturationBasisPoints: recorder.peakQueuePlayers * 10_000 / recorder.configuration.PopulationSize,
	}
	return report
}

func finalMeasurement(idle, queued, inGame, cooldown int, stats league.Stats) FinalMeasurement {
	return FinalMeasurement{
		IdlePlayers: idle, QueuedPlayers: queued, InGamePlayers: inGame, CooldownPlayers: cooldown,
		Rating: RatingMeasurement{
			GamesPlayed: stats.GamesPlayed, Minimum: stats.Minimum, Percentile10: stats.Percentile10,
			Median: stats.Median, Percentile90: stats.Percentile90, Maximum: stats.Maximum,
			Mean: stats.Mean, StdDev: stats.StdDev, Histogram: stats.Histogram,
		},
	}
}

func durationDistribution(values []int64) DurationDistribution {
	return DurationDistribution{
		SamplesPlayers: len(values),
		P50Millis:      percentileInt64(values, 50),
		P90Millis:      percentileInt64(values, 90),
		P99Millis:      percentileInt64(values, 99),
		MaxMillis:      percentileInt64(values, 100),
	}
}

func integerDistribution(values []int) IntegerDistribution {
	return IntegerDistribution{
		Samples: len(values),
		P50:     percentileInt(values, 50),
		P90:     percentileInt(values, 90),
		P99:     percentileInt(values, 99),
		Max:     percentileInt(values, 100),
	}
}

func percentileInt64(values []int64, percentage int) int64 {
	if len(values) == 0 {
		return 0
	}
	ordered := slices.Clone(values)
	slices.Sort(ordered)
	return ordered[nearestRank(len(ordered), percentage)]
}

func percentileInt(values []int, percentage int) int {
	if len(values) == 0 {
		return 0
	}
	ordered := slices.Clone(values)
	slices.Sort(ordered)
	return ordered[nearestRank(len(ordered), percentage)]
}

func nearestRank(length, percentage int) int {
	rank := (length*percentage + 99) / 100
	return max(0, min(length-1, rank-1))
}

func weightedPercentile(values []weightedValue, percentage int) int {
	ordered := slices.Clone(values)
	slices.SortFunc(ordered, func(left, right weightedValue) int {
		return left.value - right.value
	})
	totalWeight := int64(0)
	for _, value := range ordered {
		totalWeight += value.weightMillis
	}
	if totalWeight == 0 {
		return 0
	}
	target := (totalWeight*int64(percentage) + 99) / 100
	seen := int64(0)
	for _, value := range ordered {
		seen += value.weightMillis
		if seen >= target {
			return value.value
		}
	}
	return ordered[len(ordered)-1].value
}

func matchesPerMinuteMilli(matches int, durationMillis int64) int64 {
	return int64(matches) * int64(time.Minute/time.Millisecond) * 1_000 / durationMillis
}
