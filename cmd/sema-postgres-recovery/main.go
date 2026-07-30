package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"regexp"
	"time"

	"github.com/zrma/sema/internal/postgresrecovery"
)

const postgresTestDSNEnvironment = "SEMA_POSTGRES_TEST_DSN"

var schemaNamePattern = regexp.MustCompile(`^[a-z][a-z0-9_]{0,62}$`)

type configuration struct {
	phase    string
	schema   string
	manifest string
	timeout  time.Duration
}

func main() {
	os.Exit(run(context.Background(), os.Args[1:], os.Stdout, os.Stderr))
}

func run(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	config, err := parseConfiguration(args, stderr)
	if err != nil {
		return 2
	}
	phaseContext, cancel := context.WithTimeout(ctx, config.timeout)
	defer cancel()
	options := postgresrecovery.Options{
		PostgresDSN: os.Getenv(postgresTestDSNEnvironment),
		Schema:      config.schema,
		Manifest:    config.manifest,
		Now: func() time.Time {
			return time.Date(2026, time.July, 30, 0, 0, 0, 0, time.UTC)
		},
	}
	var report postgresrecovery.Report
	switch config.phase {
	case "seed":
		report, err = postgresrecovery.Seed(phaseContext, options)
	case "advance":
		report, err = postgresrecovery.Advance(phaseContext, options)
	case "verify":
		report, err = postgresrecovery.Verify(phaseContext, options)
	default:
		err = fmt.Errorf("unsupported recovery phase")
	}
	if err != nil {
		fmt.Fprintf(stderr, "sema-postgres-recovery: %v\n", err)
		return 1
	}
	if err := json.NewEncoder(stdout).Encode(report); err != nil {
		fmt.Fprintf(stderr, "sema-postgres-recovery: write report: %v\n", err)
		return 1
	}
	return 0
}

func parseConfiguration(args []string, stderr io.Writer) (configuration, error) {
	config := configuration{}
	flags := flag.NewFlagSet("sema-postgres-recovery", flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.StringVar(&config.phase, "phase", "", "recovery phase: seed, advance, or verify")
	flags.StringVar(&config.schema, "schema", "", "isolated disposable PostgreSQL schema")
	flags.StringVar(&config.manifest, "manifest", "", "private temporary checkpoint manifest")
	flags.DurationVar(&config.timeout, "timeout", time.Minute, "whole phase timeout")
	flags.Usage = func() {
		fmt.Fprintln(
			stderr,
			"usage: sema-postgres-recovery -phase seed|advance|verify -schema <name> -manifest <path>",
		)
		flags.PrintDefaults()
	}
	if err := flags.Parse(args); err != nil {
		return configuration{}, err
	}
	if flags.NArg() != 0 {
		flags.Usage()
		return configuration{}, fmt.Errorf("unexpected positional arguments")
	}
	if config.phase != "seed" && config.phase != "advance" && config.phase != "verify" {
		fmt.Fprintln(stderr, "sema-postgres-recovery: phase must be seed, advance, or verify")
		return configuration{}, fmt.Errorf("invalid phase")
	}
	if !schemaNamePattern.MatchString(config.schema) {
		fmt.Fprintln(stderr, "sema-postgres-recovery: a safe isolated schema name is required")
		return configuration{}, fmt.Errorf("invalid schema")
	}
	if config.manifest == "" || config.timeout <= 0 {
		fmt.Fprintln(stderr, "sema-postgres-recovery: manifest and positive timeout are required")
		return configuration{}, fmt.Errorf("invalid configuration")
	}
	if os.Getenv(postgresTestDSNEnvironment) == "" {
		fmt.Fprintf(stderr, "sema-postgres-recovery: %s is required\n", postgresTestDSNEnvironment)
		return configuration{}, fmt.Errorf("missing test database")
	}
	return config, nil
}
