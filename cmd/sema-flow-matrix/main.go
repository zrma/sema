package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/zrma/sema/internal/flowmatrix"
)

var version = "dev"

func main() {
	os.Exit(run(context.Background(), os.Args[1:], os.Stdout, os.Stderr))
}

func run(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	configuration := flowmatrix.DefaultConfig()
	flags := flag.NewFlagSet("sema-flow-matrix", flag.ContinueOnError)
	flags.SetOutput(stderr)
	format := flags.String("format", "text", "output format: text or json")
	seeds := flags.String("seeds", "42,73,101", "comma-separated deterministic seeds")
	batches := flags.String("batches", "2,4,8", "comma-separated matches-per-cycle profiles")
	showVersion := flags.Bool("version", false, "print version")
	flags.DurationVar(&configuration.Duration, "duration", configuration.Duration, "simulated duration per run")
	flags.IntVar(&configuration.Parallelism, "parallel", configuration.Parallelism, "maximum independent runs executed in parallel")
	flags.IntVar(&configuration.Base.PopulationSize, "population", configuration.Base.PopulationSize, "closed population player count")
	flags.DurationVar(&configuration.Base.ReservationTTL, "reservation-ttl", configuration.Base.ReservationTTL, "reservation TTL")
	flags.DurationVar(&configuration.Base.GameDuration, "game-duration", configuration.Base.GameDuration, "simulated game duration")
	flags.DurationVar(&configuration.Base.ArrivalInterval, "arrival-interval", configuration.Base.ArrivalInterval, "initial party arrival interval")
	flags.DurationVar(&configuration.Base.PlanningInterval, "planning-interval", configuration.Base.PlanningInterval, "minimum planning interval")
	flags.DurationVar(&configuration.Base.MaxReturnDelay, "max-return-delay", configuration.Base.MaxReturnDelay, "maximum post-game return delay")
	flags.DurationVar(&configuration.Base.TickDuration, "tick-duration", configuration.Base.TickDuration, "maximum idle clock step")
	flags.Usage = func() {
		fmt.Fprintln(stderr, "usage: sema-flow-matrix [flags]")
		flags.PrintDefaults()
	}
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if flags.NArg() != 0 {
		flags.Usage()
		return 2
	}
	if *showVersion {
		fmt.Fprintf(stdout, "sema-flow-matrix %s\n", version)
		return 0
	}
	if *format != "text" && *format != "json" {
		fmt.Fprintf(stderr, "sema-flow-matrix: unsupported output format %q\n", *format)
		return 2
	}
	parsedSeeds, err := parseSeeds(*seeds)
	if err != nil {
		fmt.Fprintf(stderr, "sema-flow-matrix: %v\n", err)
		return 2
	}
	parsedProfiles, err := parseBatches(*batches)
	if err != nil {
		fmt.Fprintf(stderr, "sema-flow-matrix: %v\n", err)
		return 2
	}
	configuration.Seeds = parsedSeeds
	configuration.Profiles = parsedProfiles

	report, err := flowmatrix.Run(ctx, configuration)
	if err != nil {
		fmt.Fprintf(stderr, "sema-flow-matrix: %v\n", err)
		return 1
	}
	if *format == "json" {
		encoder := json.NewEncoder(stdout)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(report); err != nil {
			fmt.Fprintf(stderr, "sema-flow-matrix: write report: %v\n", err)
			return 1
		}
		return 0
	}
	if err := writeTextReport(stdout, report); err != nil {
		fmt.Fprintf(stderr, "sema-flow-matrix: write report: %v\n", err)
		return 1
	}
	return 0
}

func parseSeeds(value string) ([]int64, error) {
	parts := strings.Split(value, ",")
	seeds := make([]int64, 0, len(parts))
	seen := make(map[int64]struct{}, len(parts))
	for _, part := range parts {
		seed, err := strconv.ParseInt(strings.TrimSpace(part), 10, 64)
		if err != nil || seed < 0 {
			return nil, fmt.Errorf("invalid seed list %q", value)
		}
		if _, exists := seen[seed]; exists {
			return nil, fmt.Errorf("seed list contains duplicate %d", seed)
		}
		seen[seed] = struct{}{}
		seeds = append(seeds, seed)
	}
	return seeds, nil
}

func parseBatches(value string) ([]flowmatrix.Profile, error) {
	parts := strings.Split(value, ",")
	profiles := make([]flowmatrix.Profile, 0, len(parts))
	seen := make(map[flowmatrix.Profile]struct{}, len(parts))
	for _, part := range parts {
		batch, err := strconv.Atoi(strings.TrimSpace(part))
		if err != nil || batch <= 0 || batch > 8 {
			return nil, fmt.Errorf("invalid batch list %q", value)
		}
		profile := flowmatrix.Profile{MatchesPerCycle: batch}
		if _, exists := seen[profile]; exists {
			return nil, fmt.Errorf("batch list contains duplicate %d", batch)
		}
		seen[profile] = struct{}{}
		profiles = append(profiles, profile)
	}
	return profiles, nil
}

func writeTextReport(writer io.Writer, report flowmatrix.Report) error {
	seedValues := make([]string, 0, len(report.Seeds))
	for _, seed := range report.Seeds {
		seedValues = append(seedValues, strconv.FormatInt(seed, 10))
	}
	lines := []string{
		fmt.Sprintf("sema-flow-matrix schema=%s duration_ms=%d population=%d seeds=%s demand_comparable=%t", report.SchemaVersion, report.DurationMillis, report.PopulationPlayers, strings.Join(seedValues, ","), report.DemandComparable),
		fmt.Sprintf("workload reservation_ttl_ms=%d game_duration_ms=%d arrival_interval_ms=%d planning_interval_ms=%d max_return_delay_ms=%d tick_duration_ms=%d", report.Workload.ReservationTTLMillis, report.Workload.GameDurationMillis, report.Workload.ArrivalIntervalMillis, report.Workload.PlanningIntervalMillis, report.Workload.MaxReturnDelayMillis, report.Workload.TickDurationMillis),
	}
	for _, profile := range report.Profiles {
		lines = append(lines,
			fmt.Sprintf("profile name=%s matches_per_cycle=%d runs=%d initial_tickets=%s arrival_lag_ms=%s final_ingress_players=%s", profile.Name, profile.MatchesPerCycle, profile.Runs, rangeText(profile.InitialTickets), rangeText(profile.MaxArrivalLagMillis), rangeText(profile.FinalIngressBacklogPlayers)),
			fmt.Sprintf("capacity name=%s assignment_yield_bps=%s confirmed_mpm_milli=%s completed_mpm_milli=%s", profile.Name, rangeText(profile.AssignmentYieldBasisPoints), rangeText(profile.ConfirmedMatchesPerMinuteMilli), rangeText(profile.CompletedMatchesPerMinuteMilli)),
			fmt.Sprintf("wait name=%s p50_ms=%s p90_ms=%s", profile.Name, rangeText(profile.WaitP50Millis), rangeText(profile.WaitP90Millis)),
			fmt.Sprintf("queue name=%s mean_players=%s p95_players=%s", profile.Name, rangeText(profile.QueueMeanPlayers), rangeText(profile.QueueP95Players)),
			fmt.Sprintf("quality name=%s skill_gap_p90=%s", profile.Name, rangeText(profile.TeamSkillGapP90)),
		)
	}
	for _, line := range lines {
		if _, err := fmt.Fprintln(writer, line); err != nil {
			return err
		}
	}
	return nil
}

func rangeText(summary flowmatrix.Summary) string {
	return fmt.Sprintf("%d/%d/%d", summary.Minimum, summary.Median, summary.Maximum)
}
