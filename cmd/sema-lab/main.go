package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/zrma/sema/internal/lab"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("sema-lab", flag.ContinueOnError)
	flags.SetOutput(stderr)
	format := flags.String("format", "text", "output format: text or json")
	list := flags.Bool("list", false, "list built-in workloads")
	details := flags.Bool("details", false, "include proposal and team placement in text output")
	flags.Usage = func() {
		fmt.Fprintln(stderr, "usage: sema-lab [flags] [workload ...]")
		flags.PrintDefaults()
	}
	if err := flags.Parse(args); err != nil {
		return 2
	}

	if *list {
		if err := writeWorkloadList(stdout); err != nil {
			fmt.Fprintf(stderr, "sema-lab: %v\n", err)
			return 1
		}
		return 0
	}
	if *format != "text" && *format != "json" {
		fmt.Fprintf(stderr, "sema-lab: unsupported output format %q\n", *format)
		return 2
	}

	report, err := lab.Run(flags.Args())
	if err != nil {
		fmt.Fprintf(stderr, "sema-lab: %v\n", err)
		return 2
	}
	if *format == "json" {
		encoder := json.NewEncoder(stdout)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(report); err != nil {
			fmt.Fprintf(stderr, "sema-lab: write report: %v\n", err)
			return 1
		}
		return 0
	}
	if err := writeTextReport(stdout, report, *details); err != nil {
		fmt.Fprintf(stderr, "sema-lab: write report: %v\n", err)
		return 1
	}
	return 0
}

func writeWorkloadList(writer io.Writer) error {
	for _, workload := range lab.Workloads() {
		if _, err := fmt.Fprintf(writer, "%s\t%s\n", workload.ID, workload.Description); err != nil {
			return err
		}
	}
	return nil
}

func writeTextReport(writer io.Writer, report lab.Report, details bool) error {
	if _, err := fmt.Fprintf(writer, "sema-lab schema=%s scenarios=%d\n", report.SchemaVersion, len(report.Scenarios)); err != nil {
		return err
	}
	for _, scenario := range report.Scenarios {
		outcome := scenario.Outcome
		if _, err := fmt.Fprintf(
			writer,
			"scenario=%s policy=%s fingerprint=%s demand_tickets=%d demand_players=%d backfills=%d proposals=%d matched_tickets=%d matched_players=%d unmatched_tickets=%d unmatched_players=%d budget_exhausted=%t\n",
			scenario.ID,
			scenario.Policy.Version,
			scenario.Policy.Fingerprint,
			scenario.Demand.MatchTickets,
			scenario.Demand.Players,
			scenario.Demand.BackfillTickets,
			outcome.ProposalCount,
			outcome.MatchedTickets,
			outcome.MatchedPlayers,
			outcome.UnmatchedTickets,
			outcome.UnmatchedPlayers,
			outcome.BudgetExhausted,
		); err != nil {
			return err
		}
		if _, err := fmt.Fprintf(
			writer,
			"  search candidates=%d nodes=%d truncated_proposals=%d relaxation_level=%d role_penalty=%d skill_gap=%d max_latency_ms=%d unmatched=%s\n",
			outcome.Search.CandidatesEvaluated,
			outcome.Search.Nodes,
			outcome.Search.TruncatedProposals,
			outcome.Search.MaxRelaxationLevel,
			outcome.Search.TotalRolePenalty,
			outcome.Search.MaxTeamSkillGap,
			outcome.Search.MaxLatencyMillis,
			formatUnmatched(outcome.UnmatchedReasons),
		); err != nil {
			return err
		}
		if details {
			if err := writeProposalDetails(writer, scenario.Proposals); err != nil {
				return err
			}
		}
	}
	return nil
}

func writeProposalDetails(writer io.Writer, proposals []lab.ProposalSummary) error {
	for _, proposal := range proposals {
		if _, err := fmt.Fprintf(writer, "  proposal=%s kind=%s\n", proposal.ID, proposal.Kind); err != nil {
			return err
		}
		for _, team := range proposal.Teams {
			tickets := make([]string, len(team.Tickets))
			for index, ticket := range team.Tickets {
				tickets[index] = string(ticket)
			}
			if _, err := fmt.Fprintf(writer, "    team=%d tickets=%s\n", team.Team, strings.Join(tickets, ",")); err != nil {
				return err
			}
		}
	}
	return nil
}

func formatUnmatched(counts []lab.UnmatchedCount) string {
	if len(counts) == 0 {
		return "none"
	}
	parts := make([]string, len(counts))
	for index, count := range counts {
		parts[index] = fmt.Sprintf("%s:%d", count.Reason, count.Count)
	}
	return strings.Join(parts, ",")
}
