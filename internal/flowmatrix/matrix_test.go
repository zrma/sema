package flowmatrix

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/zrma/sema/internal/flow"
)

func TestRunIsDeterministicAcrossWorkerParallelism(t *testing.T) {
	configuration := DefaultConfig()
	configuration.Seeds = []int64{3, 1, 2}
	configuration.Profiles = []Profile{{MatchesPerCycle: 2}, {MatchesPerCycle: 1}}
	configuration.Duration = time.Minute
	configuration.Parallelism = 1
	first, err := run(context.Background(), configuration, deterministicMeasurement)
	if err != nil {
		t.Fatal(err)
	}
	configuration.Parallelism = 4
	second, err := run(context.Background(), configuration, deterministicMeasurement)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("parallel reports differ:\nfirst=%#v\nsecond=%#v", first, second)
	}
	firstJSON, err := json.Marshal(first)
	if err != nil {
		t.Fatal(err)
	}
	secondJSON, err := json.Marshal(second)
	if err != nil {
		t.Fatal(err)
	}
	if string(firstJSON) != string(secondJSON) {
		t.Fatalf("parallel JSON differs:\nfirst=%s\nsecond=%s", firstJSON, secondJSON)
	}
	if !reflect.DeepEqual(first.Seeds, []int64{1, 2, 3}) || first.Profiles[0].Name != "b1" || first.Profiles[1].Name != "b2" {
		t.Fatalf("canonical order = %#v", first)
	}
	if first.Profiles[0].AssignmentYieldBasisPoints != (Summary{Minimum: 101, Median: 201, Maximum: 301}) {
		t.Fatalf("aggregate = %#v", first.Profiles[0])
	}
	if !first.DemandComparable {
		t.Fatalf("comparable fixture = %#v", first)
	}
}

func TestDefaultConfigUsesBatchUpperBoundProfiles(t *testing.T) {
	configuration := DefaultConfig()
	want := []Profile{{MatchesPerCycle: 2}, {MatchesPerCycle: 8}, {MatchesPerCycle: 32}}
	if !reflect.DeepEqual(configuration.Profiles, want) {
		t.Fatalf("default profiles = %#v; want %#v", configuration.Profiles, want)
	}
}

func TestRunPreservesDemandMismatch(t *testing.T) {
	configuration := DefaultConfig()
	configuration.Seeds = []int64{42}
	configuration.Profiles = []Profile{{MatchesPerCycle: 1}, {MatchesPerCycle: 4}}
	configuration.Parallelism = 2
	report, err := run(context.Background(), configuration, func(ctx context.Context, configuration flow.Config, duration time.Duration) (flow.MeasurementReport, error) {
		report, err := deterministicMeasurement(ctx, configuration, duration)
		if configuration.MatchesPerCycle == 4 {
			report.QueueEntries.InitialTickets--
			report.Ingress.MaxArrivalLagMillis = 1
			report.Ingress.FinalBacklogPlayers = 2
		}
		return report, err
	})
	if err != nil {
		t.Fatal(err)
	}
	if report.DemandComparable {
		t.Fatalf("mismatched demand was accepted: %#v", report)
	}
}

func TestRunWrapsMeasurementFailure(t *testing.T) {
	configuration := DefaultConfig()
	configuration.Seeds = []int64{42}
	configuration.Profiles = []Profile{{MatchesPerCycle: 1}}
	configuration.Parallelism = 1
	_, err := run(context.Background(), configuration, func(context.Context, flow.Config, time.Duration) (flow.MeasurementReport, error) {
		return flow.MeasurementReport{}, errors.New("boom")
	})
	if err == nil || !strings.Contains(err.Error(), "seed 42 profile b1: boom") {
		t.Fatalf("error = %v", err)
	}
}

func TestNormalizeRejectsInvalidMatrix(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Config)
	}{
		{name: "short duration", mutate: func(configuration *Config) { configuration.Duration = time.Millisecond }},
		{name: "duplicate seed", mutate: func(configuration *Config) { configuration.Seeds = []int64{1, 1} }},
		{name: "negative seed", mutate: func(configuration *Config) { configuration.Seeds = []int64{-1} }},
		{name: "duplicate profile", mutate: func(configuration *Config) {
			configuration.Profiles = []Profile{{MatchesPerCycle: 1}, {MatchesPerCycle: 1}}
		}},
		{name: "invalid profile", mutate: func(configuration *Config) {
			configuration.Profiles = []Profile{{MatchesPerCycle: flow.MaximumMatchesPerCycle + 1}}
		}},
		{name: "zero parallelism", mutate: func(configuration *Config) { configuration.Parallelism = 0 }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			configuration := DefaultConfig()
			test.mutate(&configuration)
			if _, err := normalize(configuration); err == nil {
				t.Fatalf("invalid configuration was accepted: %#v", configuration)
			}
		})
	}
}

func TestSummarizeUsesIntegerMidpoint(t *testing.T) {
	if summary := summarize([]int64{9, 1, 5}); summary != (Summary{Minimum: 1, Median: 5, Maximum: 9}) {
		t.Fatalf("odd summary = %#v", summary)
	}
	if summary := summarize([]int64{1, 4}); summary != (Summary{Minimum: 1, Median: 2, Maximum: 4}) {
		t.Fatalf("even summary = %#v", summary)
	}
}

func deterministicMeasurement(_ context.Context, configuration flow.Config, duration time.Duration) (flow.MeasurementReport, error) {
	base := int(configuration.Seed)*100 + configuration.MatchesPerCycle
	return flow.MeasurementReport{
		DurationMillis:             duration.Milliseconds(),
		QueueEntries:               flow.EntryCounts{InitialTickets: 60},
		Ingress:                    flow.IngressMeasurement{},
		AssignmentYieldBasisPoints: base,
		Throughput: flow.Throughput{
			ConfirmedMatchesPerMinuteMilli: int64(base * 10),
			CompletedMatchesPerMinuteMilli: int64(base * 9),
		},
		Wait:    flow.DurationDistribution{P50Millis: int64(base * 20), P90Millis: int64(base * 30)},
		Queue:   flow.QueueMeasurement{MeanPlayers: base * 2, P95Players: base * 3},
		Quality: flow.QualityMeasurement{TeamSkillGap: flow.IntegerDistribution{P90: base}},
	}, nil
}
