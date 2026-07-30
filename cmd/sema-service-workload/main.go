package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/zrma/sema/internal/serviceworkload"
)

const postgresTestDSNEnvironment = "SEMA_POSTGRES_TEST_DSN"

var version = "dev"

type configuration struct {
	options serviceworkload.Options
	timeout time.Duration
	format  string
}

func main() {
	os.Exit(run(context.Background(), os.Args[1:], os.Getenv, os.Stdout, os.Stderr))
}

func run(
	ctx context.Context,
	args []string,
	lookupEnvironment func(string) string,
	stdout io.Writer,
	stderr io.Writer,
) int {
	configuration, showVersion, err := parseConfiguration(args, lookupEnvironment, stderr)
	if err != nil {
		return 2
	}
	if showVersion {
		fmt.Fprintf(stdout, "sema-service-workload %s\n", version)
		return 0
	}
	ctx, cancel := context.WithTimeout(ctx, configuration.timeout)
	defer cancel()
	report, err := serviceworkload.Run(ctx, configuration.options, serviceworkload.ReferenceBudget())
	if report.Schema != "" {
		encoder := json.NewEncoder(stdout)
		if configuration.format == "json" {
			encoder.SetIndent("", "  ")
		}
		if encodeErr := encoder.Encode(report); encodeErr != nil {
			fmt.Fprintf(stderr, "sema-service-workload: write report: %v\n", encodeErr)
			return 1
		}
	}
	if err != nil {
		fmt.Fprintf(stderr, "sema-service-workload: %v\n", err)
		return 1
	}
	return 0
}

func parseConfiguration(
	args []string,
	lookupEnvironment func(string) string,
	stderr io.Writer,
) (configuration, bool, error) {
	configuration := configuration{
		options: serviceworkload.DefaultOptions(),
		timeout: 2 * time.Minute,
		format:  "json",
	}
	flags := flag.NewFlagSet("sema-service-workload", flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.IntVar(&configuration.options.Runs, "runs", configuration.options.Runs, "independent workload runs")
	flags.IntVar(&configuration.options.Cycles, "cycles", configuration.options.Cycles, "lifecycle cycles per run")
	flags.IntVar(
		&configuration.options.TicketsPerCycle,
		"tickets-per-cycle",
		configuration.options.TicketsPerCycle,
		"solo tickets per cycle (positive multiple of 10, maximum 250)",
	)
	flags.IntVar(
		&configuration.options.Concurrency,
		"concurrency",
		configuration.options.Concurrency,
		"maximum concurrent workload requests",
	)
	flags.DurationVar(&configuration.timeout, "timeout", configuration.timeout, "whole workload timeout")
	flags.StringVar(&configuration.format, "format", configuration.format, "report format: json or jsonl")
	showVersion := flags.Bool("version", false, "print version")
	flags.Usage = func() {
		fmt.Fprintln(stderr, "usage: sema-service-workload [flags]")
		flags.PrintDefaults()
	}
	if err := flags.Parse(args); err != nil {
		return configuration, false, err
	}
	if flags.NArg() != 0 {
		flags.Usage()
		return configuration, false, fmt.Errorf("unexpected positional arguments")
	}
	if *showVersion {
		return configuration, true, nil
	}
	if lookupEnvironment == nil {
		return configuration, false, fmt.Errorf("environment lookup is required")
	}
	configuration.options.PostgresDSN = lookupEnvironment(postgresTestDSNEnvironment)
	if configuration.options.PostgresDSN == "" {
		fmt.Fprintf(stderr, "sema-service-workload: %s is required\n", postgresTestDSNEnvironment)
		return configuration, false, fmt.Errorf("PostgreSQL test DSN is required")
	}
	if configuration.timeout <= 0 {
		fmt.Fprintln(stderr, "sema-service-workload: timeout must be positive")
		return configuration, false, fmt.Errorf("invalid timeout")
	}
	if configuration.format != "json" && configuration.format != "jsonl" {
		fmt.Fprintln(stderr, "sema-service-workload: format must be json or jsonl")
		return configuration, false, fmt.Errorf("invalid format")
	}
	if err := serviceworkload.Validate(configuration.options, serviceworkload.ReferenceBudget()); err != nil {
		fmt.Fprintf(stderr, "sema-service-workload: invalid workload configuration: %v\n", err)
		return configuration, false, err
	}
	return configuration, false, nil
}
