// Package flowmatrix compares deterministic Flow planning-batch profiles across seeds.
package flowmatrix

import (
	"context"
	"fmt"
	"slices"
	"sync"
	"time"

	"github.com/zrma/sema/internal/flow"
)

const SchemaVersion = "sema.flow.capacity-matrix.v0alpha2"

type Profile struct {
	MatchesPerCycle int `json:"matches_per_cycle"`
}

func (profile Profile) Label() string {
	return fmt.Sprintf("b%d", profile.MatchesPerCycle)
}

type Config struct {
	Base        flow.Config
	Duration    time.Duration
	Seeds       []int64
	Profiles    []Profile
	Parallelism int
}

func DefaultConfig() Config {
	return Config{
		Base:        flow.DefaultConfig(),
		Duration:    10 * time.Minute,
		Seeds:       []int64{42, 73, 101},
		Profiles:    []Profile{{MatchesPerCycle: 2}, {MatchesPerCycle: 8}, {MatchesPerCycle: 32}},
		Parallelism: 3,
	}
}

type Summary struct {
	Minimum int64 `json:"minimum"`
	Median  int64 `json:"median"`
	Maximum int64 `json:"maximum"`
}

type WorkloadConfiguration struct {
	ReservationTTLMillis   int64 `json:"reservation_ttl_millis"`
	GameDurationMillis     int64 `json:"game_duration_millis"`
	ArrivalIntervalMillis  int64 `json:"arrival_interval_millis"`
	PlanningIntervalMillis int64 `json:"planning_interval_millis"`
	MaxReturnDelayMillis   int64 `json:"max_return_delay_millis"`
	TickDurationMillis     int64 `json:"tick_duration_millis"`
}

type ProfileReport struct {
	Name                           string  `json:"name"`
	MatchesPerCycle                int     `json:"matches_per_cycle"`
	Runs                           int     `json:"runs"`
	InitialTickets                 Summary `json:"initial_tickets"`
	MaxArrivalLagMillis            Summary `json:"max_arrival_lag_millis"`
	FinalIngressBacklogPlayers     Summary `json:"final_ingress_backlog_players"`
	AssignmentYieldBasisPoints     Summary `json:"assignment_yield_basis_points"`
	ConfirmedMatchesPerMinuteMilli Summary `json:"confirmed_matches_per_minute_milli"`
	CompletedMatchesPerMinuteMilli Summary `json:"completed_matches_per_minute_milli"`
	WaitP50Millis                  Summary `json:"wait_p50_millis"`
	WaitP90Millis                  Summary `json:"wait_p90_millis"`
	QueueMeanPlayers               Summary `json:"queue_mean_players"`
	QueueP95Players                Summary `json:"queue_p95_players"`
	TeamSkillGapP90                Summary `json:"team_skill_gap_p90"`
}

type Report struct {
	SchemaVersion     string                `json:"schema_version"`
	DurationMillis    int64                 `json:"duration_millis"`
	PopulationPlayers int                   `json:"population_players"`
	Seeds             []int64               `json:"seeds"`
	Workload          WorkloadConfiguration `json:"workload"`
	DemandComparable  bool                  `json:"demand_comparable"`
	Profiles          []ProfileReport       `json:"profiles"`
}

type measurer func(context.Context, flow.Config, time.Duration) (flow.MeasurementReport, error)

func Run(ctx context.Context, configuration Config) (Report, error) {
	return run(ctx, configuration, flow.Measure)
}

func run(ctx context.Context, configuration Config, measure measurer) (Report, error) {
	normalized, err := normalize(configuration)
	if err != nil {
		return Report{}, err
	}

	tasks := len(normalized.Profiles) * len(normalized.Seeds)
	raw := make([]flow.MeasurementReport, tasks)
	workerContext, cancel := context.WithCancel(ctx)
	defer cancel()

	jobs := make(chan int)
	var workers sync.WaitGroup
	var firstError error
	var errorOnce sync.Once
	for range min(normalized.Parallelism, tasks) {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for index := range jobs {
				profileIndex := index / len(normalized.Seeds)
				seedIndex := index % len(normalized.Seeds)
				profile := normalized.Profiles[profileIndex]
				seed := normalized.Seeds[seedIndex]
				flowConfiguration := normalized.Base
				flowConfiguration.Seed = seed
				flowConfiguration.MatchesPerCycle = profile.MatchesPerCycle
				report, measureErr := measure(workerContext, flowConfiguration, normalized.Duration)
				if measureErr != nil {
					errorOnce.Do(func() {
						firstError = fmt.Errorf("measure seed %d profile %s: %w", seed, profile.Label(), measureErr)
						cancel()
					})
					continue
				}
				raw[index] = report
			}
		}()
	}

sendJobs:
	for index := range tasks {
		select {
		case jobs <- index:
		case <-workerContext.Done():
			break sendJobs
		}
	}
	close(jobs)
	workers.Wait()
	if firstError != nil {
		return Report{}, firstError
	}
	if err := ctx.Err(); err != nil {
		return Report{}, err
	}

	report := Report{
		SchemaVersion:     SchemaVersion,
		DurationMillis:    normalized.Duration.Milliseconds(),
		PopulationPlayers: raw[0].PopulationPlayers,
		Seeds:             slices.Clone(normalized.Seeds),
		Workload: WorkloadConfiguration{
			ReservationTTLMillis:   raw[0].Configuration.ReservationTTLMillis,
			GameDurationMillis:     raw[0].Configuration.GameDurationMillis,
			ArrivalIntervalMillis:  raw[0].Configuration.ArrivalIntervalMillis,
			PlanningIntervalMillis: raw[0].Configuration.PlanningIntervalMillis,
			MaxReturnDelayMillis:   raw[0].Configuration.MaxReturnDelayMillis,
			TickDurationMillis:     raw[0].Configuration.TickDurationMillis,
		},
		DemandComparable: true,
		Profiles:         make([]ProfileReport, 0, len(normalized.Profiles)),
	}
	for profileIndex, profile := range normalized.Profiles {
		runs := raw[profileIndex*len(normalized.Seeds) : (profileIndex+1)*len(normalized.Seeds)]
		report.Profiles = append(report.Profiles, aggregate(profile, runs))
	}
	for seedIndex := range normalized.Seeds {
		initialTickets := raw[seedIndex].QueueEntries.InitialTickets
		for profileIndex := range normalized.Profiles {
			runReport := raw[profileIndex*len(normalized.Seeds)+seedIndex]
			if runReport.QueueEntries.InitialTickets != initialTickets || runReport.Ingress.MaxArrivalLagMillis != 0 ||
				runReport.Ingress.FinalBacklogTickets != 0 || runReport.Ingress.FinalBacklogPlayers != 0 {
				report.DemandComparable = false
			}
		}
	}
	return report, nil
}

func normalize(configuration Config) (Config, error) {
	if configuration.Duration < time.Second {
		return Config{}, fmt.Errorf("matrix duration must be at least one second")
	}
	if len(configuration.Seeds) == 0 || len(configuration.Profiles) == 0 {
		return Config{}, fmt.Errorf("matrix requires at least one seed and profile")
	}
	if configuration.Parallelism <= 0 {
		return Config{}, fmt.Errorf("matrix parallelism must be positive")
	}
	normalized := configuration
	normalized.Seeds = slices.Clone(configuration.Seeds)
	slices.Sort(normalized.Seeds)
	for index, seed := range normalized.Seeds {
		if seed < 0 || index > 0 && seed == normalized.Seeds[index-1] {
			return Config{}, fmt.Errorf("matrix seeds must be unique and non-negative")
		}
	}
	normalized.Profiles = slices.Clone(configuration.Profiles)
	slices.SortFunc(normalized.Profiles, func(left, right Profile) int {
		return left.MatchesPerCycle - right.MatchesPerCycle
	})
	for index, profile := range normalized.Profiles {
		if profile.MatchesPerCycle <= 0 || profile.MatchesPerCycle > flow.MaximumMatchesPerCycle {
			return Config{}, fmt.Errorf("matrix profile %q is invalid", profile.Label())
		}
		if index > 0 && profile == normalized.Profiles[index-1] {
			return Config{}, fmt.Errorf("matrix profiles must be unique")
		}
	}
	return normalized, nil
}

func aggregate(profile Profile, reports []flow.MeasurementReport) ProfileReport {
	values := func(selectValue func(flow.MeasurementReport) int64) Summary {
		collected := make([]int64, 0, len(reports))
		for _, report := range reports {
			collected = append(collected, selectValue(report))
		}
		return summarize(collected)
	}
	return ProfileReport{
		Name:                           profile.Label(),
		MatchesPerCycle:                profile.MatchesPerCycle,
		Runs:                           len(reports),
		InitialTickets:                 values(func(report flow.MeasurementReport) int64 { return int64(report.QueueEntries.InitialTickets) }),
		MaxArrivalLagMillis:            values(func(report flow.MeasurementReport) int64 { return report.Ingress.MaxArrivalLagMillis }),
		FinalIngressBacklogPlayers:     values(func(report flow.MeasurementReport) int64 { return int64(report.Ingress.FinalBacklogPlayers) }),
		AssignmentYieldBasisPoints:     values(func(report flow.MeasurementReport) int64 { return int64(report.AssignmentYieldBasisPoints) }),
		ConfirmedMatchesPerMinuteMilli: values(func(report flow.MeasurementReport) int64 { return report.Throughput.ConfirmedMatchesPerMinuteMilli }),
		CompletedMatchesPerMinuteMilli: values(func(report flow.MeasurementReport) int64 { return report.Throughput.CompletedMatchesPerMinuteMilli }),
		WaitP50Millis:                  values(func(report flow.MeasurementReport) int64 { return report.Wait.P50Millis }),
		WaitP90Millis:                  values(func(report flow.MeasurementReport) int64 { return report.Wait.P90Millis }),
		QueueMeanPlayers:               values(func(report flow.MeasurementReport) int64 { return int64(report.Queue.MeanPlayers) }),
		QueueP95Players:                values(func(report flow.MeasurementReport) int64 { return int64(report.Queue.P95Players) }),
		TeamSkillGapP90:                values(func(report flow.MeasurementReport) int64 { return int64(report.Quality.TeamSkillGap.P90) }),
	}
}

func summarize(values []int64) Summary {
	ordered := slices.Clone(values)
	slices.Sort(ordered)
	middle := len(ordered) / 2
	median := ordered[middle]
	if len(ordered)%2 == 0 {
		median = ordered[middle-1] + (ordered[middle]-ordered[middle-1])/2
	}
	return Summary{Minimum: ordered[0], Median: median, Maximum: ordered[len(ordered)-1]}
}
