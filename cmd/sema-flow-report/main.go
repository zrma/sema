package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/zrma/sema/internal/flow"
)

var version = "dev"

func main() {
	os.Exit(run(context.Background(), os.Args[1:], os.Stdout, os.Stderr))
}

func run(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	configuration := flow.DefaultConfig()
	flags := flag.NewFlagSet("sema-flow-report", flag.ContinueOnError)
	flags.SetOutput(stderr)
	format := flags.String("format", "text", "output format: text or json")
	duration := flags.Duration("duration", 30*time.Minute, "simulated measurement duration")
	showVersion := flags.Bool("version", false, "print version")
	flags.Int64Var(&configuration.Seed, "seed", configuration.Seed, "deterministic population seed")
	flags.IntVar(&configuration.PopulationSize, "population", configuration.PopulationSize, "closed population player count")
	flags.IntVar(&configuration.MatchesPerCycle, "matches-per-cycle", configuration.MatchesPerCycle, "maximum proposals per planning cycle")
	flags.IntVar(&configuration.MaxConcurrentMatches, "concurrent-matches", configuration.MaxConcurrentMatches, "maximum concurrent matches")
	flags.DurationVar(&configuration.GameDuration, "game-duration", configuration.GameDuration, "simulated game duration")
	flags.DurationVar(&configuration.ArrivalInterval, "arrival-interval", configuration.ArrivalInterval, "initial party arrival interval")
	flags.DurationVar(&configuration.PlanningInterval, "planning-interval", configuration.PlanningInterval, "minimum planning interval")
	flags.DurationVar(&configuration.MaxReturnDelay, "max-return-delay", configuration.MaxReturnDelay, "maximum post-game return delay")
	flags.Usage = func() {
		fmt.Fprintln(stderr, "usage: sema-flow-report [flags]")
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
		fmt.Fprintf(stdout, "sema-flow-report %s\n", version)
		return 0
	}
	if *format != "text" && *format != "json" {
		fmt.Fprintf(stderr, "sema-flow-report: unsupported output format %q\n", *format)
		return 2
	}

	report, err := flow.Measure(ctx, configuration, *duration)
	if err != nil {
		fmt.Fprintf(stderr, "sema-flow-report: %v\n", err)
		return 1
	}
	if *format == "json" {
		encoder := json.NewEncoder(stdout)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(report); err != nil {
			fmt.Fprintf(stderr, "sema-flow-report: write report: %v\n", err)
			return 1
		}
		return 0
	}
	if err := writeTextReport(stdout, report); err != nil {
		fmt.Fprintf(stderr, "sema-flow-report: write report: %v\n", err)
		return 1
	}
	return 0
}

func writeTextReport(writer io.Writer, report flow.MeasurementReport) error {
	lines := []string{
		fmt.Sprintf("sema-flow-report schema=%s seed=%d duration_ms=%d population=%d", report.SchemaVersion, report.Seed, report.DurationMillis, report.PopulationPlayers),
		fmt.Sprintf("config matches_per_cycle=%d concurrent_matches=%d reservation_ttl_ms=%d game_duration_ms=%d arrival_interval_ms=%d planning_interval_ms=%d max_return_delay_ms=%d tick_duration_ms=%d", report.Configuration.MatchesPerCycle, report.Configuration.MaxConcurrentMatches, report.Configuration.ReservationTTLMillis, report.Configuration.GameDurationMillis, report.Configuration.ArrivalIntervalMillis, report.Configuration.PlanningIntervalMillis, report.Configuration.MaxReturnDelayMillis, report.Configuration.TickDurationMillis),
		fmt.Sprintf("flow steps=%d cycles=%d arrivals_tickets=%d arrivals_players=%d initial_tickets=%d returned_tickets=%d assignments=%d assigned_tickets=%d assigned_players=%d completions=%d completed_players=%d assignment_yield_bps=%d", report.Steps, report.Cycles, report.QueueEntries.Tickets, report.QueueEntries.Players, report.QueueEntries.InitialTickets, report.QueueEntries.ReturnedTickets, report.Assignments.Matches, report.Assignments.Tickets, report.Assignments.Players, report.Completions.Matches, report.Completions.Players, report.AssignmentYieldBasisPoints),
		fmt.Sprintf("wait samples_players=%d p50_ms=%d p90_ms=%d p99_ms=%d max_ms=%d", report.Wait.SamplesPlayers, report.Wait.P50Millis, report.Wait.P90Millis, report.Wait.P99Millis, report.Wait.MaxMillis),
		fmt.Sprintf("throughput confirmed_matches_per_minute_milli=%d completed_matches_per_minute_milli=%d", report.Throughput.ConfirmedMatchesPerMinuteMilli, report.Throughput.CompletedMatchesPerMinuteMilli),
		fmt.Sprintf("queue mean_players=%d p95_players=%d peak_players=%d mean_saturation_bps=%d p95_saturation_bps=%d peak_saturation_bps=%d", report.Queue.MeanPlayers, report.Queue.P95Players, report.Queue.PeakPlayers, report.Queue.MeanSaturationBasisPoints, report.Queue.P95SaturationBasisPoints, report.Queue.PeakSaturationBasisPoints),
		fmt.Sprintf("quality samples=%d skill_gap_p50=%d skill_gap_p90=%d skill_gap_p99=%d skill_gap_max=%d latency_p50_ms=%d latency_p90_ms=%d latency_p99_ms=%d latency_max_ms=%d", report.Quality.TeamSkillGap.Samples, report.Quality.TeamSkillGap.P50, report.Quality.TeamSkillGap.P90, report.Quality.TeamSkillGap.P99, report.Quality.TeamSkillGap.Max, report.Quality.MaxLatencyMillis.P50, report.Quality.MaxLatencyMillis.P90, report.Quality.MaxLatencyMillis.P99, report.Quality.MaxLatencyMillis.Max),
		fmt.Sprintf("final idle=%d queued=%d in_game=%d cooldown=%d games=%d rating_min=%d rating_p10=%d rating_median=%d rating_p90=%d rating_max=%d rating_mean=%d rating_sd=%d", report.Final.IdlePlayers, report.Final.QueuedPlayers, report.Final.InGamePlayers, report.Final.CooldownPlayers, report.Final.Rating.GamesPlayed, report.Final.Rating.Minimum, report.Final.Rating.Percentile10, report.Final.Rating.Median, report.Final.Rating.Percentile90, report.Final.Rating.Maximum, report.Final.Rating.Mean, report.Final.Rating.StdDev),
	}
	for _, line := range lines {
		if _, err := fmt.Fprintln(writer, line); err != nil {
			return err
		}
	}
	return nil
}
