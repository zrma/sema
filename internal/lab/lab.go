// Package lab provides deterministic, built-in workloads for inspecting Sema.
package lab

import (
	"fmt"
	"slices"
	"sort"
	"time"

	"github.com/zrma/sema/internal/domain"
	"github.com/zrma/sema/internal/evaluation"
	"github.com/zrma/sema/internal/simulation"
)

const SchemaVersion = "v0alpha5"

var fixtureNow = time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)

type Workload struct {
	ID          string
	Description string
	Policy      domain.MatchmakingPolicy
	Scenario    simulation.Scenario
}

type Report struct {
	SchemaVersion string           `json:"schema_version"`
	Scenarios     []ScenarioResult `json:"scenarios"`
}

type ScenarioResult struct {
	ID          string            `json:"id"`
	Description string            `json:"description"`
	Policy      PolicySummary     `json:"policy"`
	Demand      DemandSummary     `json:"demand"`
	Outcome     OutcomeSummary    `json:"outcome"`
	Oracle      *OracleSummary    `json:"oracle,omitempty"`
	Frontier    *FrontierSummary  `json:"frontier,omitempty"`
	Proposals   []ProposalSummary `json:"proposals"`
}

type PolicySummary struct {
	Version     string                   `json:"version"`
	Fingerprint domain.PolicyFingerprint `json:"fingerprint"`
}

type DemandSummary struct {
	MatchTickets    int `json:"match_tickets"`
	BackfillTickets int `json:"backfill_tickets"`
	Players         int `json:"players"`
}

type OutcomeSummary struct {
	ProposalCount             int              `json:"proposal_count"`
	MatchedTickets            int              `json:"matched_tickets"`
	MatchedPlayers            int              `json:"matched_players"`
	UnmatchedTickets          int              `json:"unmatched_tickets"`
	UnmatchedPlayers          int              `json:"unmatched_players"`
	CoverageBasisPoints       int              `json:"coverage_basis_points"`
	OldestMatchedWaitMillis   int64            `json:"oldest_matched_wait_millis"`
	OldestUnmatchedWaitMillis int64            `json:"oldest_unmatched_wait_millis"`
	UnmatchedReasons          []UnmatchedCount `json:"unmatched_reasons"`
	BudgetExhausted           bool             `json:"budget_exhausted"`
	Batch                     BatchSummary     `json:"batch"`
	Search                    SearchSummary    `json:"search"`
}

type BatchSummary struct {
	CandidateProposals           int   `json:"candidate_proposals"`
	SelectedProposals            int   `json:"selected_proposals"`
	SelectedBackfills            int   `json:"selected_backfills"`
	TotalUtility                 int64 `json:"total_utility"`
	CandidateGenerationNodes     int   `json:"candidate_generation_nodes"`
	CandidateGenerationTruncated bool  `json:"candidate_generation_truncated"`
	SelectionNodes               int   `json:"selection_nodes"`
	SelectionTruncated           bool  `json:"selection_truncated"`
}

type OracleSummary struct {
	Relation             evaluation.QualityRelation `json:"relation"`
	PlannerFound         bool                       `json:"planner_found"`
	OracleFound          bool                       `json:"oracle_found"`
	CandidatesEvaluated  int                        `json:"candidates_evaluated"`
	AdmissibleCandidates int                        `json:"admissible_candidates"`
	PlannerQuality       *QualityVector             `json:"planner_quality,omitempty"`
	OracleQuality        *QualityVector             `json:"oracle_quality,omitempty"`
}

type QualityVector struct {
	RelaxationLevel  int   `json:"relaxation_level"`
	WaitPriority     bool  `json:"wait_priority"`
	RolePenalty      int   `json:"role_penalty"`
	TeamSkillGap     int   `json:"team_skill_gap"`
	OldestWaitMillis int64 `json:"oldest_wait_millis"`
	TotalWaitMillis  int64 `json:"total_wait_millis"`
	MaxLatencyMillis int   `json:"max_latency_millis"`
}

type FrontierSummary struct {
	Relation             evaluation.BatchFrontierRelation `json:"relation"`
	PlacementsEvaluated  int                              `json:"placements_evaluated"`
	AdmissibleCandidates int                              `json:"admissible_candidates"`
	BatchesEvaluated     int                              `json:"batches_evaluated"`
	FrontierPoints       int                              `json:"frontier_points"`
	Planner              BatchQualitySummary              `json:"planner"`
	Dominating           *BatchQualitySummary             `json:"dominating,omitempty"`
	Points               []BatchQualitySummary            `json:"points"`
}

type BatchQualitySummary struct {
	SelectedBackfills     int   `json:"selected_backfills"`
	ProposalCount         int   `json:"proposal_count"`
	MatchedTickets        int   `json:"matched_tickets"`
	MatchedPlayers        int   `json:"matched_players"`
	MaxRolePenalty        int   `json:"max_role_penalty"`
	MeanRolePenaltyMilli  int   `json:"mean_role_penalty_milli"`
	TotalRolePenalty      int   `json:"total_role_penalty"`
	MaxTeamSkillGap       int   `json:"max_team_skill_gap"`
	MeanTeamSkillGapMilli int   `json:"mean_team_skill_gap_milli"`
	TotalTeamSkillGap     int   `json:"total_team_skill_gap"`
	MaxLatencyMillis      int   `json:"max_latency_millis"`
	OldestWaitMillis      int64 `json:"oldest_wait_millis"`
	MeanWaitMillis        int64 `json:"mean_wait_millis"`
	TotalWaitMillis       int64 `json:"total_wait_millis"`
}

type UnmatchedCount struct {
	Reason domain.UnmatchedReason `json:"reason"`
	Count  int                    `json:"count"`
}

type SearchSummary struct {
	CandidateTickets      int   `json:"candidate_tickets"`
	TruncatedWindows      int   `json:"truncated_candidate_windows"`
	CandidatesEvaluated   int   `json:"candidates_evaluated"`
	Nodes                 int   `json:"nodes"`
	TruncatedProposals    int   `json:"truncated_proposals"`
	MaxRelaxationLevel    int   `json:"max_relaxation_level"`
	WaitPriorityProposals int   `json:"wait_priority_proposals"`
	TotalRolePenalty      int   `json:"total_role_penalty"`
	MaxTeamSkillGap       int   `json:"max_team_skill_gap"`
	OldestWaitMillis      int64 `json:"oldest_wait_millis"`
	TotalWaitMillis       int64 `json:"total_wait_millis"`
	MaxLatencyMillis      int   `json:"max_latency_millis"`
}

type ProposalSummary struct {
	ID       domain.ProposalID   `json:"id"`
	Kind     domain.ProposalKind `json:"kind"`
	Backfill *BackfillSummary    `json:"backfill,omitempty"`
	Teams    []TeamSummary       `json:"teams"`
	Evidence EvidenceSummary     `json:"evidence"`
}

type BackfillSummary struct {
	TicketID      domain.TicketID  `json:"ticket_id"`
	SessionID     domain.SessionID `json:"session_id"`
	RosterVersion domain.Revision  `json:"roster_version"`
}

type TeamSummary struct {
	Team    int               `json:"team"`
	Tickets []domain.TicketID `json:"tickets"`
}

type EvidenceSummary struct {
	RelaxationLevel          int   `json:"relaxation_level"`
	WaitPriority             bool  `json:"wait_priority"`
	RolePenalty              int   `json:"role_penalty"`
	TeamSkillGap             int   `json:"team_skill_gap"`
	OldestWaitMillis         int64 `json:"oldest_wait_millis"`
	TotalWaitMillis          int64 `json:"total_wait_millis"`
	MaxLatencyMillis         int   `json:"max_latency_millis"`
	CandidateTickets         int   `json:"candidate_tickets"`
	CandidatesEvaluated      int   `json:"candidates_evaluated"`
	SearchNodes              int   `json:"search_nodes"`
	CandidateWindowTruncated bool  `json:"candidate_window_truncated"`
	SearchTruncated          bool  `json:"search_truncated"`
	SelectionUtility         int64 `json:"selection_utility"`
}

// Workloads returns defensive copies of the canonical built-in corpus.
func Workloads() []Workload {
	workloads := builtInWorkloads()
	for index := range workloads {
		workloads[index] = cloneWorkload(workloads[index])
	}
	return workloads
}

// Run evaluates selected built-in workloads. Empty IDs select the full corpus.
func Run(ids []string) (Report, error) {
	selected, err := selectWorkloads(ids)
	if err != nil {
		return Report{}, err
	}

	report := Report{SchemaVersion: SchemaVersion, Scenarios: make([]ScenarioResult, 0, len(selected))}
	for _, workload := range selected {
		simulationReport, err := simulation.Run(
			[]domain.MatchmakingPolicy{workload.Policy},
			[]simulation.Scenario{workload.Scenario},
		)
		if err != nil {
			return Report{}, fmt.Errorf("run workload %q: %w", workload.ID, err)
		}
		policyResult := simulationReport.Policies[0]
		scenarioResult := policyResult.Scenarios[0]
		summarized, err := summarizeWorkload(workload, policyResult, scenarioResult)
		if err != nil {
			return Report{}, fmt.Errorf("summarize workload %q: %w", workload.ID, err)
		}
		report.Scenarios = append(report.Scenarios, summarized)
	}
	return report, nil
}

func selectWorkloads(ids []string) ([]Workload, error) {
	available := builtInWorkloads()
	if len(ids) == 0 {
		return available, nil
	}

	byID := make(map[string]Workload, len(available))
	for _, workload := range available {
		byID[workload.ID] = workload
	}
	seen := make(map[string]struct{}, len(ids))
	selected := make([]Workload, 0, len(ids))
	for _, id := range ids {
		workload, exists := byID[id]
		if !exists {
			return nil, domain.NewFailure(domain.FailureInvalidInput, "unknown lab workload %q", id)
		}
		if _, duplicate := seen[id]; duplicate {
			continue
		}
		seen[id] = struct{}{}
		selected = append(selected, workload)
	}
	sort.Slice(selected, func(left, right int) bool { return selected[left].ID < selected[right].ID })
	return selected, nil
}

func summarizeWorkload(
	workload Workload,
	policyResult simulation.PolicyResult,
	result simulation.ScenarioResult,
) (ScenarioResult, error) {
	metrics := evaluation.Measure(workload.Scenario, result.Batch)

	unmatchedReasons := make([]UnmatchedCount, len(result.Summary.Unmatched))
	for index, count := range result.Summary.Unmatched {
		unmatchedReasons[index] = UnmatchedCount{Reason: count.Reason, Count: count.Count}
	}

	scores := result.Summary.Scores
	summary := ScenarioResult{
		ID:          workload.ID,
		Description: workload.Description,
		Policy: PolicySummary{
			Version:     policyResult.Version,
			Fingerprint: policyResult.Fingerprint,
		},
		Demand: DemandSummary{
			MatchTickets:    len(workload.Scenario.MatchTickets),
			BackfillTickets: len(workload.Scenario.BackfillTickets),
			Players:         metrics.DemandPlayers,
		},
		Outcome: OutcomeSummary{
			ProposalCount:             result.Summary.ProposalCount,
			MatchedTickets:            metrics.MatchedTickets,
			MatchedPlayers:            metrics.MatchedPlayers,
			UnmatchedTickets:          result.Summary.UnmatchedTicketCount,
			UnmatchedPlayers:          metrics.UnmatchedPlayers,
			CoverageBasisPoints:       metrics.CoverageBasisPoints,
			OldestMatchedWaitMillis:   metrics.OldestMatchedWaitMillis,
			OldestUnmatchedWaitMillis: metrics.OldestUnmatchedWaitMillis,
			UnmatchedReasons:          unmatchedReasons,
			BudgetExhausted:           result.Summary.BudgetExhausted,
			Batch: BatchSummary{
				CandidateProposals:           result.Batch.Evidence.CandidateProposals,
				SelectedProposals:            result.Batch.Evidence.SelectedProposals,
				SelectedBackfills:            result.Batch.Evidence.SelectedBackfills,
				TotalUtility:                 result.Batch.Evidence.TotalUtility,
				CandidateGenerationNodes:     result.Batch.Evidence.CandidateGenerationNodes,
				CandidateGenerationTruncated: result.Batch.Evidence.CandidateGenerationTruncated,
				SelectionNodes:               result.Batch.Evidence.SelectionNodes,
				SelectionTruncated:           result.Batch.Evidence.SelectionTruncated,
			},
			Search: SearchSummary{
				CandidateTickets:      scores.CandidateTickets,
				TruncatedWindows:      scores.TruncatedCandidateWindows,
				CandidatesEvaluated:   scores.CandidatesEvaluated,
				Nodes:                 scores.SearchNodes,
				TruncatedProposals:    scores.SearchTruncatedProposals,
				MaxRelaxationLevel:    scores.MaxRelaxationLevel,
				WaitPriorityProposals: scores.WaitPriorityProposals,
				TotalRolePenalty:      scores.TotalRolePenalty,
				MaxTeamSkillGap:       scores.MaxTeamSkillGap,
				OldestWaitMillis:      scores.OldestWaitMillis,
				TotalWaitMillis:       scores.TotalWaitMillis,
				MaxLatencyMillis:      scores.MaxLatencyMillis,
			},
		},
		Proposals: summarizeProposals(result.Batch.Proposals),
	}
	snapshot := domain.MatchmakingSnapshot{
		ID: workload.Scenario.ID, Now: workload.Scenario.Now,
		MatchTickets: workload.Scenario.MatchTickets, BackfillTickets: workload.Scenario.BackfillTickets,
		Policy: workload.Policy,
	}
	if evaluation.OracleEligible(snapshot) {
		comparison, err := evaluation.CompareBatch(snapshot, result.Batch)
		if err != nil {
			return ScenarioResult{}, err
		}
		summary.Oracle = summarizeOracle(comparison)
	}
	if evaluation.BatchFrontierEligible(snapshot) {
		comparison, err := evaluation.CompareBatchFrontier(snapshot, result.Batch)
		if err != nil {
			return ScenarioResult{}, err
		}
		summary.Frontier = summarizeFrontier(comparison)
	}
	return summary, nil
}

func summarizeOracle(comparison evaluation.OracleComparison) *OracleSummary {
	summary := &OracleSummary{
		Relation:             comparison.Relation,
		PlannerFound:         comparison.PlannerFound,
		OracleFound:          comparison.Oracle.Found,
		CandidatesEvaluated:  comparison.Oracle.CandidatesEvaluated,
		AdmissibleCandidates: comparison.Oracle.AdmissibleCandidates,
	}
	if comparison.PlannerFound {
		quality := qualityVector(comparison.PlannerEvidence)
		summary.PlannerQuality = &quality
	}
	if comparison.Oracle.Found {
		quality := qualityVector(comparison.Oracle.BestEvidence)
		summary.OracleQuality = &quality
	}
	return summary
}

func qualityVector(evidence domain.ScoreEvidence) QualityVector {
	return QualityVector{
		RelaxationLevel: evidence.RelaxationLevel, WaitPriority: evidence.WaitPriority,
		RolePenalty: evidence.RolePenalty, TeamSkillGap: evidence.TeamSkillGap,
		OldestWaitMillis: evidence.OldestWaitMillis, TotalWaitMillis: evidence.TotalWaitMillis,
		MaxLatencyMillis: evidence.MaxLatencyMillis,
	}
}

func summarizeFrontier(comparison evaluation.BatchFrontierComparison) *FrontierSummary {
	points := make([]BatchQualitySummary, len(comparison.Frontier.Points))
	for index, point := range comparison.Frontier.Points {
		points[index] = batchQualitySummary(point)
	}
	summary := &FrontierSummary{
		Relation:             comparison.Relation,
		PlacementsEvaluated:  comparison.Frontier.PlacementsEvaluated,
		AdmissibleCandidates: comparison.Frontier.AdmissibleCandidates,
		BatchesEvaluated:     comparison.Frontier.BatchesEvaluated,
		FrontierPoints:       len(comparison.Frontier.Points),
		Planner:              batchQualitySummary(comparison.Planner),
		Points:               points,
	}
	if comparison.Dominating != nil {
		dominating := batchQualitySummary(*comparison.Dominating)
		summary.Dominating = &dominating
	}
	return summary
}

func batchQualitySummary(quality evaluation.BatchQuality) BatchQualitySummary {
	return BatchQualitySummary{
		SelectedBackfills:     quality.SelectedBackfills,
		ProposalCount:         quality.ProposalCount,
		MatchedTickets:        quality.MatchedTickets,
		MatchedPlayers:        quality.MatchedPlayers,
		MaxRolePenalty:        quality.MaxRolePenalty,
		MeanRolePenaltyMilli:  quality.MeanRolePenaltyMilli,
		TotalRolePenalty:      quality.TotalRolePenalty,
		MaxTeamSkillGap:       quality.MaxTeamSkillGap,
		MeanTeamSkillGapMilli: quality.MeanTeamSkillGapMilli,
		TotalTeamSkillGap:     quality.TotalTeamSkillGap,
		MaxLatencyMillis:      quality.MaxLatencyMillis,
		OldestWaitMillis:      quality.OldestWaitMillis,
		MeanWaitMillis:        quality.MeanWaitMillis,
		TotalWaitMillis:       quality.TotalWaitMillis,
	}
}

func summarizeProposals(proposals []domain.MatchProposal) []ProposalSummary {
	summaries := make([]ProposalSummary, len(proposals))
	for index, proposal := range proposals {
		teams := make([]TeamSummary, len(proposal.Teams))
		for teamIndex, team := range proposal.Teams {
			tickets := make([]domain.TicketID, len(team.Tickets))
			for ticketIndex, ticket := range team.Tickets {
				tickets[ticketIndex] = ticket.ID
			}
			teams[teamIndex] = TeamSummary{Team: team.Team, Tickets: tickets}
		}
		var backfill *BackfillSummary
		if proposal.Backfill != nil {
			backfill = &BackfillSummary{
				TicketID:      proposal.Backfill.Ticket.ID,
				SessionID:     proposal.Backfill.SessionID,
				RosterVersion: proposal.Backfill.RosterVersion,
			}
		}
		evidence := proposal.Evidence
		summaries[index] = ProposalSummary{
			ID:       proposal.ID,
			Kind:     proposal.Kind,
			Backfill: backfill,
			Teams:    teams,
			Evidence: EvidenceSummary{
				RelaxationLevel:          evidence.RelaxationLevel,
				WaitPriority:             evidence.WaitPriority,
				RolePenalty:              evidence.RolePenalty,
				TeamSkillGap:             evidence.TeamSkillGap,
				OldestWaitMillis:         evidence.OldestWaitMillis,
				TotalWaitMillis:          evidence.TotalWaitMillis,
				MaxLatencyMillis:         evidence.MaxLatencyMillis,
				CandidateTickets:         evidence.CandidateTickets,
				CandidatesEvaluated:      evidence.CandidatesEvaluated,
				SearchNodes:              evidence.SearchNodes,
				CandidateWindowTruncated: evidence.CandidateWindowTruncated,
				SearchTruncated:          evidence.SearchTruncated,
				SelectionUtility:         evidence.SelectionUtility,
			},
		}
	}
	return summaries
}

func builtInWorkloads() []Workload {
	workloads := make([]Workload, 0, 30)
	for _, teamSize := range []int{2, 3, 5, 10, 16, 20, 50} {
		workloads = append(workloads,
			teamWorkload(teamSize, "solo", repeatedPartySizes(teamSize*2, 1)),
			teamWorkload(teamSize, "full-party", []int{teamSize, teamSize}),
			teamWorkload(teamSize, "mixed", mixedPartySizes(teamSize)),
		)
	}
	workloads = append(workloads,
		battleRoyaleWorkload("duo", 2),
		battleRoyaleWorkload("squad", 4),
		backfillWorkload(),
		noMatchWorkload(),
		latencyHardLimitWorkload(),
		roleQualityWorkload(),
		waitRelaxationWorkload(),
		boundedQualityGapWorkload(),
		candidateWindowGapWorkload(),
		batchFrontierMixedWorkload(),
		batchFrontierGapWorkload(),
		syntheticQueueWorkload(),
	)
	sort.Slice(workloads, func(left, right int) bool { return workloads[left].ID < workloads[right].ID })
	return workloads
}

func batchFrontierMixedWorkload() Workload {
	id := "batch-frontier-mixed-party-backfill"
	policy := referencePolicy(id, 2, 5)
	policy.MaxProposals = 2
	policy.MaxBatchCandidates = 1024
	policy.MaxBatchSearchNodes = 500_000
	policy.MaxSearchNodes = 500_000
	policy.MaxCandidatesPerProposal = 512
	scenario := scenarioWithParties(id, []int{1, 2, 3, 2, 3})
	for index := range scenario.MatchTickets {
		for playerIndex := range scenario.MatchTickets[index].Players {
			scenario.MatchTickets[index].Players[playerIndex].Skill = 1500
			scenario.MatchTickets[index].Players[playerIndex].LatencyMillis = 30
		}
	}
	scenario.BackfillTickets = []domain.BackfillTicket{{
		ID: "batch-frontier-backfill", Revision: 1,
		SessionID: "batch-frontier-session", RosterVersion: 1,
		OpenSlotsByTeam: []int{1, 0}, EnqueuedAt: fixtureNow.Add(-time.Minute),
	}}
	return Workload{
		ID:          id,
		Description: "small exhaustive batch frontier with solo, duo, trio, and one backfill vacancy",
		Policy:      policy,
		Scenario:    scenario,
	}
}

func batchFrontierGapWorkload() Workload {
	id := "diagnostic-batch-frontier-gap"
	policy := referencePolicy(id, 2, 1)
	policy.MaxProposals = 2
	policy.MaxBatchCandidates = 1
	policy.MaxBatchSearchNodes = 100_000
	scenario := scenarioWithParties(id, []int{1, 1, 1, 1})
	for index := range scenario.MatchTickets {
		scenario.MatchTickets[index].EnqueuedAt = fixtureNow.Add(-time.Minute)
		scenario.MatchTickets[index].Players[0].Skill = 1500
		scenario.MatchTickets[index].Players[0].LatencyMillis = 30
	}
	return Workload{
		ID:          id,
		Description: "small exhaustive diagnostic where a one-candidate batch budget leaves a second match uncovered",
		Policy:      policy,
		Scenario:    scenario,
	}
}

func candidateWindowGapWorkload() Workload {
	id := "diagnostic-candidate-window-gap"
	return qualityGapWorkload(id, "1:1 diagnostic where an oldest-two ticket window misses the best skill balance", 2, 64)
}

func boundedQualityGapWorkload() Workload {
	id := "diagnostic-bounded-quality-gap"
	return qualityGapWorkload(id, "1:1 diagnostic where a one-candidate budget misses the best skill balance", 0, 1)
}

func qualityGapWorkload(id, description string, maxCandidateTickets, maxCandidates int) Workload {
	policy := referencePolicy(id, 2, 1)
	policy.MaxProposals = 1
	policy.MaxCandidateTickets = maxCandidateTickets
	policy.MaxCandidatesPerProposal = maxCandidates
	scenario := scenarioWithParties(id, []int{1, 1, 1, 1})
	skills := []int{0, 1000, 500, 500}
	for index := range scenario.MatchTickets {
		scenario.MatchTickets[index].Players[0].Skill = skills[index]
		if index < 2 {
			scenario.MatchTickets[index].EnqueuedAt = fixtureNow.Add(-time.Minute)
		} else {
			scenario.MatchTickets[index].EnqueuedAt = fixtureNow.Add(-10 * time.Second)
		}
	}
	return Workload{
		ID: id, Description: description,
		Policy: policy, Scenario: scenario,
	}
}

func syntheticQueueWorkload() Workload {
	id := "synthetic-5v5-seeded-queue"
	scenario, err := evaluation.Generate(evaluation.WorkloadModel{
		ID: domain.SnapshotID(id), Seed: 20260717, Now: fixtureNow, TicketCount: 40, MaxPartySize: 5,
		PartySizes: []evaluation.PartySizeWeight{
			{Size: 1, Weight: 55}, {Size: 2, Weight: 25}, {Size: 3, Weight: 15}, {Size: 5, Weight: 5},
		},
		SkillCenter: 1000, SkillSpread: 300,
		Roles: []evaluation.RoleWeight{
			{Role: "healer", Weight: 12}, {Role: "tank", Weight: 18}, {Role: "dps", Weight: 70},
		},
		MinLatencyMS: 20, MaxLatencyMS: 120, MinWait: 0, MaxWait: 2 * time.Minute,
	})
	if err != nil {
		panic(fmt.Sprintf("invalid built-in synthetic workload: %v", err))
	}
	policy := referencePolicy(id, 2, 5)
	policy.MaxProposals = 4
	policy.RoleRequirements = []domain.RoleRequirement{
		{Role: "healer", MinPerTeam: 1}, {Role: "tank", MinPerTeam: 1},
	}
	policy.RelaxationSteps = []domain.RelaxationStep{
		{AfterWait: 0, MaxTeamSkillGap: 100, MaxRolePenalty: 0},
		{AfterWait: 30 * time.Second, MaxTeamSkillGap: 250, MaxRolePenalty: 2, PrioritizeWait: true},
	}
	return Workload{
		ID: id, Description: "seeded 5:5 queue with weighted party, skill, role, latency, and wait distributions",
		Policy: policy, Scenario: scenario,
	}
}

func teamWorkload(teamSize int, variant string, partySizes []int) Workload {
	id := fmt.Sprintf("team-%dv%d-%s", teamSize, teamSize, variant)
	return Workload{
		ID:          id,
		Description: fmt.Sprintf("%d:%d team match with %s party distribution", teamSize, teamSize, variant),
		Policy:      referencePolicy(id, 2, teamSize),
		Scenario:    scenarioWithParties(id, partySizes),
	}
}

func battleRoyaleWorkload(variant string, partySize int) Workload {
	id := "battle-royale-" + variant
	return Workload{
		ID:          id,
		Description: fmt.Sprintf("100-player battle royale with %d-player parties", partySize),
		Policy:      referencePolicy(id, 1, 100),
		Scenario:    scenarioWithParties(id, repeatedPartySizes(100/partySize, partySize)),
	}
}

func backfillWorkload() Workload {
	id := "backfill-2v2-two-slots"
	scenario := scenarioWithParties(id, []int{1, 1, 1, 1})
	scenario.BackfillTickets = []domain.BackfillTicket{{
		ID:              "backfill-demand",
		Revision:        1,
		SessionID:       "session-backfill",
		RosterVersion:   7,
		OpenSlotsByTeam: []int{1, 1},
		EnqueuedAt:      fixtureNow.Add(-time.Minute),
	}}
	return Workload{
		ID:          id,
		Description: "2:2 session backfill before new-match planning",
		Policy:      referencePolicy(id, 2, 2),
		Scenario:    scenario,
	}
}

func noMatchWorkload() Workload {
	id := "no-match-insufficient-capacity"
	return Workload{
		ID:          id,
		Description: "2:2 demand with only three available players",
		Policy:      referencePolicy(id, 2, 2),
		Scenario:    scenarioWithParties(id, []int{1, 1, 1}),
	}
}

func latencyHardLimitWorkload() Workload {
	id := "no-match-latency-hard-limit"
	scenario := scenarioWithParties(id, []int{1, 1, 1, 1})
	scenario.MatchTickets[len(scenario.MatchTickets)-1].Players[0].LatencyMillis = 201
	return Workload{
		ID:          id,
		Description: "2:2 demand with one player above the absolute latency cap",
		Policy:      referencePolicy(id, 2, 2),
		Scenario:    scenario,
	}
}

func roleQualityWorkload() Workload {
	id := "quality-role-balanced-2v2"
	policy := qualityPolicy(id)
	scenario := scenarioWithParties(id, []int{1, 1, 1, 1, 1, 1})
	roles := []string{"healer", "dps", "healer", "dps", "dps", "dps"}
	for index := range scenario.MatchTickets {
		scenario.MatchTickets[index].Players[0].Role = roles[index]
	}
	return Workload{
		ID:          id,
		Description: "2:2 candidate selection with a healer on each team",
		Policy:      policy,
		Scenario:    scenario,
	}
}

func waitRelaxationWorkload() Workload {
	id := "quality-wait-relaxed-2v2"
	policy := qualityPolicy(id)
	scenario := scenarioWithParties(id, []int{1, 1, 1, 1})
	for index := range scenario.MatchTickets {
		scenario.MatchTickets[index].Players[0].Role = "dps"
		scenario.MatchTickets[index].EnqueuedAt = fixtureNow.Add(-time.Minute)
	}
	return Workload{
		ID:          id,
		Description: "2:2 role requirement relaxed after a one-minute wait",
		Policy:      policy,
		Scenario:    scenario,
	}
}

func referencePolicy(id string, teamCount, teamSize int) domain.MatchmakingPolicy {
	return domain.MatchmakingPolicy{
		Version:                  id + "-v1",
		TeamCount:                teamCount,
		TeamSize:                 teamSize,
		MaxLatencyMillis:         200,
		MaxSearchNodes:           100_000,
		MaxCandidatesPerProposal: 64,
	}
}

func qualityPolicy(id string) domain.MatchmakingPolicy {
	policy := referencePolicy(id, 2, 2)
	policy.MaxProposals = 1
	policy.RoleRequirements = []domain.RoleRequirement{{Role: "healer", MinPerTeam: 1}}
	policy.RelaxationSteps = []domain.RelaxationStep{
		{AfterWait: 0, MaxTeamSkillGap: 10, MaxRolePenalty: 0},
		{AfterWait: 30 * time.Second, MaxTeamSkillGap: 100, MaxRolePenalty: 2, PrioritizeWait: true},
	}
	return policy
}

func scenarioWithParties(id string, partySizes []int) simulation.Scenario {
	tickets := make([]domain.MatchTicket, len(partySizes))
	playerSequence := 0
	for ticketIndex, partySize := range partySizes {
		players := make([]domain.Player, partySize)
		for playerIndex := range players {
			players[playerIndex] = domain.Player{
				ID:            domain.PlayerID(fmt.Sprintf("%s-player-%03d", id, playerSequence)),
				Skill:         1000 + playerSequence%7,
				LatencyMillis: 20 + playerSequence%5,
			}
			playerSequence++
		}
		tickets[ticketIndex] = domain.MatchTicket{
			ID:         domain.TicketID(fmt.Sprintf("%s-ticket-%03d", id, ticketIndex)),
			Revision:   1,
			EnqueuedAt: fixtureNow.Add(-time.Duration(len(partySizes)-ticketIndex) * time.Second),
			Players:    players,
		}
	}
	return simulation.Scenario{ID: domain.SnapshotID(id), Now: fixtureNow, MatchTickets: tickets}
}

func mixedPartySizes(teamSize int) []int {
	if teamSize == 2 {
		return []int{2, 1, 1}
	}
	return []int{teamSize - 1, 1, teamSize - 1, 1}
}

func repeatedPartySizes(count, partySize int) []int {
	partySizes := make([]int, count)
	for index := range partySizes {
		partySizes[index] = partySize
	}
	return partySizes
}

func cloneWorkload(workload Workload) Workload {
	workload.Policy = domain.ClonePolicy(workload.Policy)
	workload.Scenario.MatchTickets = slices.Clone(workload.Scenario.MatchTickets)
	for index := range workload.Scenario.MatchTickets {
		workload.Scenario.MatchTickets[index] = domain.CloneMatchTicket(workload.Scenario.MatchTickets[index])
	}
	workload.Scenario.BackfillTickets = slices.Clone(workload.Scenario.BackfillTickets)
	for index := range workload.Scenario.BackfillTickets {
		workload.Scenario.BackfillTickets[index] = domain.CloneBackfillTicket(workload.Scenario.BackfillTickets[index])
	}
	return workload
}
