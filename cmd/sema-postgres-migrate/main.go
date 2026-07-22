package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	postgresrepository "github.com/zrma/sema/internal/repository/postgres"
)

const postgresDSNEnvironment = "SEMA_POSTGRES_DSN"

var version = "dev"

type configuration struct {
	postgresDSN string
	timeout     time.Duration
}

func main() {
	os.Exit(run(context.Background(), os.Args[1:], os.LookupEnv, os.Stdout, os.Stderr, migrate))
}

func run(
	ctx context.Context,
	args []string,
	lookupEnvironment func(string) (string, bool),
	stdout io.Writer,
	stderr io.Writer,
	migrateSchema func(context.Context, string) error,
) int {
	config, showVersion, err := parseConfiguration(args, lookupEnvironment, stderr)
	if err != nil {
		return 2
	}
	if showVersion {
		fmt.Fprintf(stdout, "sema-postgres-migrate %s\n", version)
		return 0
	}
	if migrateSchema == nil {
		fmt.Fprintln(stderr, "sema-postgres-migrate: migration dependency is unavailable")
		return 1
	}
	migrationContext, cancel := context.WithTimeout(ctx, config.timeout)
	defer cancel()
	if err := migrateSchema(migrationContext, config.postgresDSN); err != nil {
		fmt.Fprintln(stderr, "sema-postgres-migrate: PostgreSQL migration failed")
		return 1
	}
	fmt.Fprintln(stdout, "sema-postgres-migrate completed")
	return 0
}

func migrate(ctx context.Context, dsn string) error {
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return fmt.Errorf("open PostgreSQL migration pool: %w", err)
	}
	defer pool.Close()
	if err := pool.Ping(ctx); err != nil {
		return fmt.Errorf("ping PostgreSQL migration pool: %w", err)
	}
	if err := postgresrepository.Migrate(ctx, pool); err != nil {
		return fmt.Errorf("apply repository migration: %w", err)
	}
	return nil
}

func parseConfiguration(
	args []string,
	lookupEnvironment func(string) (string, bool),
	stderr io.Writer,
) (configuration, bool, error) {
	config := configuration{}
	flags := flag.NewFlagSet("sema-postgres-migrate", flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.DurationVar(&config.timeout, "timeout", time.Minute, "whole migration timeout")
	showVersion := flags.Bool("version", false, "print version")
	flags.Usage = func() {
		fmt.Fprintln(stderr, "usage: sema-postgres-migrate [flags]")
		flags.PrintDefaults()
	}
	if err := flags.Parse(args); err != nil {
		return configuration{}, false, err
	}
	if flags.NArg() != 0 {
		flags.Usage()
		return configuration{}, false, fmt.Errorf("unexpected positional arguments")
	}
	if *showVersion {
		return config, true, nil
	}
	if lookupEnvironment == nil {
		return configuration{}, false, fmt.Errorf("environment lookup is required")
	}
	dsn, exists := lookupEnvironment(postgresDSNEnvironment)
	if !exists || dsn == "" || dsn != strings.TrimSpace(dsn) {
		fmt.Fprintln(stderr, "sema-postgres-migrate: SEMA_POSTGRES_DSN is required")
		return configuration{}, false, fmt.Errorf("missing PostgreSQL connection configuration")
	}
	if config.timeout <= 0 {
		fmt.Fprintln(stderr, "sema-postgres-migrate: timeout must be positive")
		return configuration{}, false, fmt.Errorf("invalid migration timeout")
	}
	config.postgresDSN = dsn
	return config, false, nil
}
