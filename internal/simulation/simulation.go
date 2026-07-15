// Package simulation compares versioned policies over immutable demand scenarios.
package simulation

import (
	"sort"
	"time"

	"github.com/zrma/sema/internal/domain"
	"github.com/zrma/sema/internal/planner"
	"github.com/zrma/sema/internal/policy"
)

type Scenario struct {
	ID              domain.SnapshotID
	Now             time.Time
	MatchTickets    []domain.MatchTicket
	BackfillTickets []domain.BackfillTicket
}

type UnmatchedCount struct {
	Reason domain.UnmatchedReason
	Count  int
}

type ScoreSummary struct {
	CandidatesEvaluated      int
	SearchNodes              int
	SearchTruncatedProposals int
	MaxRelaxationLevel       int
	WaitPriorityProposals    int
	TotalRolePenalty         int
	MaxTeamSkillGap          int
	OldestWaitMillis         int64
	TotalWaitMillis          int64
	MaxLatencyMillis         int
}

type Summary struct {
	ProposalCount        int
	MatchedTicketCount   int
	UnmatchedTicketCount int
	Unmatched            []UnmatchedCount
	BudgetExhausted      bool
	Scores               ScoreSummary
}

type ScenarioResult struct {
	ScenarioID domain.SnapshotID
	Batch      domain.ProposalBatch
	Summary    Summary
}

type PolicyResult struct {
	Version     string
	Fingerprint domain.PolicyFingerprint
	Scenarios   []ScenarioResult
}

type Report struct {
	Policies []PolicyResult
}

func Run(candidates []domain.MatchmakingPolicy, corpus []Scenario) (Report, error) {
	if len(candidates) == 0 || len(corpus) == 0 {
		return Report{}, domain.NewFailure(
			domain.FailureInvalidInput,
			"simulation needs at least one policy and scenario",
		)
	}

	catalog := policy.NewCatalog()
	entriesByVersion := make(map[string]policy.Entry, len(candidates))
	for _, candidate := range candidates {
		entry, err := catalog.Register(candidate)
		if err != nil {
			return Report{}, err
		}
		entriesByVersion[entry.Policy.Version] = entry
	}
	entries := make([]policy.Entry, 0, len(entriesByVersion))
	for _, entry := range entriesByVersion {
		entries = append(entries, entry)
	}
	sort.Slice(entries, func(left, right int) bool {
		if entries[left].Policy.Version != entries[right].Policy.Version {
			return entries[left].Policy.Version < entries[right].Policy.Version
		}
		return entries[left].Fingerprint < entries[right].Fingerprint
	})

	scenarios := cloneScenarios(corpus)
	sort.Slice(scenarios, func(left, right int) bool {
		return scenarios[left].ID < scenarios[right].ID
	})
	for index, scenario := range scenarios {
		if scenario.ID == "" || scenario.Now.IsZero() {
			return Report{}, domain.NewFailure(
				domain.FailureInvalidInput,
				"simulation scenario identity and time are required",
			)
		}
		if index > 0 && scenarios[index-1].ID == scenario.ID {
			return Report{}, domain.NewFailure(
				domain.FailureInvalidInput,
				"simulation scenario %q is duplicated",
				scenario.ID,
			)
		}
	}

	report := Report{Policies: make([]PolicyResult, 0, len(entries))}
	for _, entry := range entries {
		policyResult := PolicyResult{
			Version:     entry.Policy.Version,
			Fingerprint: entry.Fingerprint,
			Scenarios:   make([]ScenarioResult, 0, len(scenarios)),
		}
		for _, scenario := range scenarios {
			snapshot := domain.MatchmakingSnapshot{
				ID:              scenario.ID,
				Now:             scenario.Now,
				MatchTickets:    cloneMatchTickets(scenario.MatchTickets),
				BackfillTickets: cloneBackfillTickets(scenario.BackfillTickets),
				Policy:          domain.ClonePolicy(entry.Policy),
			}
			batch, err := planner.Plan(snapshot)
			if err != nil {
				return Report{}, err
			}
			policyResult.Scenarios = append(policyResult.Scenarios, ScenarioResult{
				ScenarioID: scenario.ID,
				Batch:      batch,
				Summary:    summarize(batch),
			})
		}
		report.Policies = append(report.Policies, policyResult)
	}
	return report, nil
}

func summarize(batch domain.ProposalBatch) Summary {
	summary := Summary{
		ProposalCount:        len(batch.Proposals),
		UnmatchedTicketCount: len(batch.Unmatched),
		BudgetExhausted:      batch.BudgetExhausted,
	}
	counts := make(map[domain.UnmatchedReason]int)
	for _, unmatched := range batch.Unmatched {
		counts[unmatched.Reason]++
	}
	for _, reason := range []domain.UnmatchedReason{
		domain.UnmatchedHardConstraint,
		domain.UnmatchedInsufficientCapacity,
		domain.UnmatchedQualityThreshold,
		domain.UnmatchedSearchBudget,
		domain.UnmatchedProposalLimit,
	} {
		if counts[reason] > 0 {
			summary.Unmatched = append(summary.Unmatched, UnmatchedCount{Reason: reason, Count: counts[reason]})
		}
	}
	for _, proposal := range batch.Proposals {
		summary.MatchedTicketCount += len(proposal.Tickets)
		evidence := proposal.Evidence
		summary.Scores.CandidatesEvaluated += evidence.CandidatesEvaluated
		summary.Scores.SearchNodes += evidence.SearchNodes
		if evidence.SearchTruncated {
			summary.Scores.SearchTruncatedProposals++
		}
		summary.Scores.MaxRelaxationLevel = max(summary.Scores.MaxRelaxationLevel, evidence.RelaxationLevel)
		if evidence.WaitPriority {
			summary.Scores.WaitPriorityProposals++
		}
		summary.Scores.TotalRolePenalty += evidence.RolePenalty
		summary.Scores.MaxTeamSkillGap = max(summary.Scores.MaxTeamSkillGap, evidence.TeamSkillGap)
		summary.Scores.OldestWaitMillis = max(summary.Scores.OldestWaitMillis, evidence.OldestWaitMillis)
		summary.Scores.TotalWaitMillis += evidence.TotalWaitMillis
		summary.Scores.MaxLatencyMillis = max(summary.Scores.MaxLatencyMillis, evidence.MaxLatencyMillis)
	}
	return summary
}

func cloneScenarios(scenarios []Scenario) []Scenario {
	cloned := make([]Scenario, len(scenarios))
	for index, scenario := range scenarios {
		cloned[index] = Scenario{
			ID:              scenario.ID,
			Now:             scenario.Now,
			MatchTickets:    cloneMatchTickets(scenario.MatchTickets),
			BackfillTickets: cloneBackfillTickets(scenario.BackfillTickets),
		}
	}
	return cloned
}

func cloneMatchTickets(tickets []domain.MatchTicket) []domain.MatchTicket {
	cloned := make([]domain.MatchTicket, len(tickets))
	for index, ticket := range tickets {
		cloned[index] = domain.CloneMatchTicket(ticket)
	}
	return cloned
}

func cloneBackfillTickets(tickets []domain.BackfillTicket) []domain.BackfillTicket {
	cloned := make([]domain.BackfillTicket, len(tickets))
	for index, ticket := range tickets {
		cloned[index] = domain.CloneBackfillTicket(ticket)
	}
	return cloned
}
