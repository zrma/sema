package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/zrma/sema/internal/runtimevalidation"
)

const postgresTestDSNEnvironment = "SEMA_POSTGRES_TEST_DSN"

var version = "dev"

func main() {
	flags := flag.NewFlagSet("sema-runtime-matrix", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	timeout := flags.Duration("timeout", 45*time.Second, "whole runtime validation timeout")
	showVersion := flags.Bool("version", false, "print version")
	flags.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: sema-runtime-matrix [flags]")
		flags.PrintDefaults()
	}
	if err := flags.Parse(os.Args[1:]); err != nil {
		os.Exit(2)
	}
	if flags.NArg() != 0 {
		flags.Usage()
		os.Exit(2)
	}
	if *showVersion {
		fmt.Printf("sema-runtime-matrix %s\n", version)
		return
	}
	dsn := os.Getenv(postgresTestDSNEnvironment)
	if dsn == "" {
		fmt.Fprintf(os.Stderr, "sema-runtime-matrix: %s is required\n", postgresTestDSNEnvironment)
		os.Exit(2)
	}
	report, err := runtimevalidation.Run(context.Background(), runtimevalidation.Options{
		PostgresDSN: dsn,
		Timeout:     *timeout,
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "sema-runtime-matrix: validation failed")
		os.Exit(1)
	}
	if err := json.NewEncoder(os.Stdout).Encode(report); err != nil {
		fmt.Fprintln(os.Stderr, "sema-runtime-matrix: write report failed")
		os.Exit(1)
	}
}
