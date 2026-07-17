// Package performance evaluates sanitized Go benchmark output against the reference profile.
package performance

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"slices"
	"strconv"
	"strings"
)

const (
	ReportSchema = "sema-performance-v1"
	ProfileID    = "sema-reference-container-v1"
)

type Budget struct {
	MaxNanosPerOp  float64 `json:"max_nanos_per_op"`
	MaxBytesPerOp  float64 `json:"max_bytes_per_op"`
	MaxAllocsPerOp float64 `json:"max_allocs_per_op"`
}

type BenchmarkResult struct {
	Name           string  `json:"name"`
	Samples        int     `json:"samples"`
	MaxNanosPerOp  float64 `json:"max_nanos_per_op"`
	MaxBytesPerOp  float64 `json:"max_bytes_per_op"`
	MaxAllocsPerOp float64 `json:"max_allocs_per_op"`
	Budget         Budget  `json:"budget"`
	WithinBudget   bool    `json:"within_budget"`
}

type Report struct {
	SchemaVersion  string            `json:"schema_version"`
	Profile        string            `json:"profile"`
	MinimumSamples int               `json:"minimum_samples"`
	Benchmarks     []BenchmarkResult `json:"benchmarks"`
}

type sample struct {
	nanosPerOp  float64
	bytesPerOp  float64
	allocsPerOp float64
}

var referenceBudgets = map[string]Budget{
	"BenchmarkPlanReferenceWorkloads/50v50-solo": {
		MaxNanosPerOp: 5_000_000, MaxBytesPerOp: 1_000_000, MaxAllocsPerOp: 1_500,
	},
	"BenchmarkPlanLargeQueues/5v5/window-256/queue-100000": {
		MaxNanosPerOp: 200_000_000, MaxBytesPerOp: 60_000_000, MaxAllocsPerOp: 120_000,
	},
	"BenchmarkEngineQueueSizes/5v5/queue-1000": {
		MaxNanosPerOp: 20_000_000, MaxBytesPerOp: 2_000_000, MaxAllocsPerOp: 5_000,
	},
	"BenchmarkOpenReplay/events-1002": {
		MaxNanosPerOp: 200_000_000, MaxBytesPerOp: 5_000_000, MaxAllocsPerOp: 40_000,
	},
}

// Evaluate parses Go benchmark lines and returns a redacted aggregate report.
func Evaluate(reader io.Reader, minimumSamples int) (Report, error) {
	report := Report{SchemaVersion: ReportSchema, Profile: ProfileID, MinimumSamples: minimumSamples}
	if minimumSamples <= 0 {
		return report, fmt.Errorf("minimum samples must be positive")
	}
	samples := make(map[string][]sample, len(referenceBudgets))
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 64<<10), 1<<20)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) == 0 || !strings.HasPrefix(fields[0], "Benchmark") {
			continue
		}
		name := canonicalBenchmarkName(fields[0])
		if _, exists := referenceBudgets[name]; !exists {
			continue
		}
		parsed, err := parseSample(fields)
		if err != nil {
			return report, fmt.Errorf("parse %s: %w", name, err)
		}
		samples[name] = append(samples[name], parsed)
	}
	if err := scanner.Err(); err != nil {
		return report, fmt.Errorf("read benchmark output: %w", err)
	}

	names := make([]string, 0, len(referenceBudgets))
	for name := range referenceBudgets {
		names = append(names, name)
	}
	slices.Sort(names)
	var failures []error
	for _, name := range names {
		observed := samples[name]
		result := BenchmarkResult{Name: name, Samples: len(observed), Budget: referenceBudgets[name]}
		for _, current := range observed {
			result.MaxNanosPerOp = max(result.MaxNanosPerOp, current.nanosPerOp)
			result.MaxBytesPerOp = max(result.MaxBytesPerOp, current.bytesPerOp)
			result.MaxAllocsPerOp = max(result.MaxAllocsPerOp, current.allocsPerOp)
		}
		result.WithinBudget = len(observed) >= minimumSamples &&
			result.MaxNanosPerOp <= result.Budget.MaxNanosPerOp &&
			result.MaxBytesPerOp <= result.Budget.MaxBytesPerOp &&
			result.MaxAllocsPerOp <= result.Budget.MaxAllocsPerOp
		if len(observed) < minimumSamples {
			failures = append(failures, fmt.Errorf("%s has %d samples; want at least %d", name, len(observed), minimumSamples))
		} else if !result.WithinBudget {
			failures = append(failures, fmt.Errorf("%s exceeded the reference budget", name))
		}
		report.Benchmarks = append(report.Benchmarks, result)
	}
	return report, errors.Join(failures...)
}

func canonicalBenchmarkName(name string) string {
	if _, exists := referenceBudgets[name]; exists {
		return name
	}
	separator := strings.LastIndexByte(name, '-')
	if separator < 0 {
		return name
	}
	if _, err := strconv.Atoi(name[separator+1:]); err != nil {
		return name
	}
	return name[:separator]
}

func parseSample(fields []string) (sample, error) {
	values := make(map[string]float64, 3)
	for index, field := range fields {
		if field != "ns/op" && field != "B/op" && field != "allocs/op" {
			continue
		}
		if index == 0 {
			return sample{}, fmt.Errorf("metric %s has no value", field)
		}
		value, err := strconv.ParseFloat(fields[index-1], 64)
		if err != nil {
			return sample{}, fmt.Errorf("metric %s: %w", field, err)
		}
		values[field] = value
	}
	for _, metric := range []string{"ns/op", "B/op", "allocs/op"} {
		if _, exists := values[metric]; !exists {
			return sample{}, fmt.Errorf("metric %s is missing", metric)
		}
	}
	return sample{
		nanosPerOp: values["ns/op"], bytesPerOp: values["B/op"], allocsPerOp: values["allocs/op"],
	}, nil
}
