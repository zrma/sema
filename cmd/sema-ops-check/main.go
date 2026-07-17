package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http/httptest"
	"os"
	"path/filepath"
	"time"

	"github.com/zrma/sema/internal/domain"
	"github.com/zrma/sema/internal/durable"
	"github.com/zrma/sema/internal/httpapi"
	"github.com/zrma/sema/internal/observability"
	"github.com/zrma/sema/internal/operational"
)

var version = "dev"

const tornTailMarker = `{"schema":"injected-torn-tail"`

type config struct {
	cycles          int
	ticketsPerCycle int
	concurrency     int
	timeout         time.Duration
	reservationTTL  time.Duration
}

func main() {
	os.Exit(run(context.Background(), os.Args[1:], os.Stdout, os.Stderr))
}

func run(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	configuration, showVersion, err := parseConfig(args, stderr)
	if err != nil {
		return 2
	}
	if showVersion {
		fmt.Fprintf(stdout, "sema-ops-check %s\n", version)
		return 0
	}

	validationContext, cancel := context.WithTimeout(ctx, configuration.timeout)
	defer cancel()
	report, err := validate(validationContext, configuration)
	if err != nil {
		fmt.Fprintf(stderr, "sema-ops-check: %v\n", err)
		return 1
	}
	encoder := json.NewEncoder(stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(report); err != nil {
		fmt.Fprintf(stderr, "sema-ops-check: write report: %v\n", err)
		return 1
	}
	return 0
}

func parseConfig(args []string, stderr io.Writer) (config, bool, error) {
	configuration := config{}
	flags := flag.NewFlagSet("sema-ops-check", flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.IntVar(&configuration.cycles, "cycles", 2, "complete lifecycle cycles")
	flags.IntVar(&configuration.ticketsPerCycle, "tickets-per-cycle", 20, "solo tickets per cycle (multiple of 10, maximum 250)")
	flags.IntVar(&configuration.concurrency, "concurrency", 8, "maximum concurrent HTTP mutations")
	flags.DurationVar(&configuration.timeout, "timeout", time.Minute, "whole validation timeout")
	flags.DurationVar(&configuration.reservationTTL, "reservation-ttl", 30*time.Second, "fixed runtime reservation TTL")
	showVersion := flags.Bool("version", false, "print version")
	flags.Usage = func() {
		fmt.Fprintln(stderr, "usage: sema-ops-check [flags]")
		flags.PrintDefaults()
	}
	if err := flags.Parse(args); err != nil {
		return config{}, false, err
	}
	if flags.NArg() != 0 {
		flags.Usage()
		return config{}, false, fmt.Errorf("unexpected positional arguments")
	}
	if configuration.timeout <= 0 || configuration.reservationTTL <= 0 {
		fmt.Fprintln(stderr, "sema-ops-check: timeout and reservation TTL must be positive")
		return config{}, false, fmt.Errorf("invalid duration configuration")
	}
	if configuration.cycles <= 0 || configuration.cycles > 10_000 ||
		configuration.ticketsPerCycle <= 0 || configuration.ticketsPerCycle > 250 ||
		configuration.ticketsPerCycle%10 != 0 ||
		configuration.concurrency <= 0 || configuration.concurrency > 256 {
		fmt.Fprintln(stderr, "sema-ops-check: cycles, tickets per cycle, or concurrency are outside supported bounds")
		return config{}, false, fmt.Errorf("invalid workload configuration")
	}
	return configuration, *showVersion, nil
}

func validate(ctx context.Context, configuration config) (operational.Report, error) {
	directory, err := os.MkdirTemp("", "sema-ops-")
	if err != nil {
		return operational.Report{}, fmt.Errorf("create isolated validation directory: %w", err)
	}
	defer os.RemoveAll(directory)
	journalPath := filepath.Join(directory, "sema.journal")

	runtime, err := durable.Open(journalPath, configuration.reservationTTL)
	if err != nil {
		return operational.Report{}, fmt.Errorf("open validation runtime: %w", err)
	}
	server := httptest.NewServer(httpapi.NewWithOptions(runtime, httpapi.Options{
		Observer: observability.New(io.Discard, time.Now),
	}))
	report, runErr := operational.Run(ctx, operational.Config{
		BaseURL: server.URL, Cycles: configuration.cycles,
		TicketsPerCycle: configuration.ticketsPerCycle, Concurrency: configuration.concurrency,
	})
	server.Close()
	closeErr := runtime.Close()
	if err := errors.Join(runErr, closeErr); err != nil {
		return operational.Report{}, fmt.Errorf("execute service lifecycle: %w", err)
	}

	if err := appendTornTail(journalPath); err != nil {
		return operational.Report{}, err
	}
	recovered, err := durable.Open(journalPath, configuration.reservationTTL)
	if err != nil {
		return operational.Report{}, fmt.Errorf("reopen after injected torn tail: %w", err)
	}
	recoveryErr := verifyRecovered(recovered, report.AssignmentIDs, report.AuditRecords)
	closeErr = recovered.Close()
	if err := errors.Join(recoveryErr, closeErr); err != nil {
		return operational.Report{}, fmt.Errorf("verify recovered runtime: %w", err)
	}
	contents, err := os.ReadFile(journalPath)
	if err != nil {
		return operational.Report{}, fmt.Errorf("read recovered journal: %w", err)
	}
	if bytes.Contains(contents, []byte(tornTailMarker)) || len(contents) == 0 || contents[len(contents)-1] != '\n' {
		return operational.Report{}, fmt.Errorf("incomplete journal tail was not removed")
	}
	report.Recovery.RestartVerified = true
	report.Recovery.TornTailRecovered = true
	return report, nil
}

func appendTornTail(path string) error {
	file, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		return fmt.Errorf("open journal for failure injection: %w", err)
	}
	_, writeErr := io.WriteString(file, tornTailMarker)
	syncErr := file.Sync()
	closeErr := file.Close()
	if err := errors.Join(writeErr, syncErr, closeErr); err != nil {
		return fmt.Errorf("inject incomplete journal tail: %w", err)
	}
	return nil
}

func verifyRecovered(runtime *durable.Runtime, assignmentIDs []string, expectedAuditRecords int) error {
	if err := runtime.Ready(); err != nil {
		return err
	}
	for _, id := range assignmentIDs {
		assignment, exists, err := runtime.Assignment(domain.AssignmentID(id))
		if err != nil {
			return err
		}
		if !exists || assignment.Status != domain.AssignmentCompleted {
			return fmt.Errorf("completed assignment was not recovered")
		}
	}
	var after uint64
	auditRecords := 0
	for {
		summaries, err := runtime.AuditSummaries(after, 1000)
		if err != nil {
			return err
		}
		if len(summaries) == 0 {
			break
		}
		for _, summary := range summaries {
			if summary.Sequence != after+1 {
				return fmt.Errorf("audit sequence was not recovered contiguously")
			}
			after = summary.Sequence
			auditRecords++
		}
		if len(summaries) < 1000 {
			break
		}
	}
	if auditRecords != expectedAuditRecords {
		return fmt.Errorf("recovered audit record count is %d; want %d", auditRecords, expectedAuditRecords)
	}
	return nil
}
