package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/zrma/sema/internal/performance"
)

var version = "dev"

func main() {
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

func run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("sema-benchmark-gate", flag.ContinueOnError)
	flags.SetOutput(stderr)
	minimumSamples := flags.Int("minimum-samples", 3, "minimum samples required per benchmark")
	compact := flags.Bool("compact", false, "write one-line JSON")
	showVersion := flags.Bool("version", false, "print version")
	flags.Usage = func() {
		fmt.Fprintln(stderr, "usage: sema-benchmark-gate [flags] < go-benchmark-output")
		flags.PrintDefaults()
	}
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if flags.NArg() != 0 || *minimumSamples <= 0 {
		flags.Usage()
		return 2
	}
	if *showVersion {
		fmt.Fprintf(stdout, "sema-benchmark-gate %s\n", version)
		return 0
	}

	report, evaluationErr := performance.Evaluate(stdin, *minimumSamples)
	encoder := json.NewEncoder(stdout)
	if !*compact {
		encoder.SetIndent("", "  ")
	}
	if err := encoder.Encode(report); err != nil {
		fmt.Fprintf(stderr, "sema-benchmark-gate: write report: %v\n", err)
		return 1
	}
	if evaluationErr != nil {
		fmt.Fprintf(stderr, "sema-benchmark-gate: %v\n", evaluationErr)
		return 1
	}
	return 0
}
