// Package alpha exposes Sema's experimental side-effect-free composition API.
//
// The package is source-unstable until the repository declares a stable API.
package alpha

import "time"

const APIVersion = "v0alpha5"

type (
	PlayerID          string
	TicketID          string
	SessionID         string
	SnapshotID        string
	ProposalID        string
	PolicyFingerprint string
	Revision          uint64
)

type Player struct {
	ID            PlayerID `json:"id"`
	Skill         int      `json:"skill"`
	Role          string   `json:"role,omitempty"`
	LatencyMillis int      `json:"latency_millis"`
}

type MatchTicket struct {
	ID         TicketID  `json:"id"`
	Revision   Revision  `json:"revision"`
	EnqueuedAt time.Time `json:"enqueued_at"`
	Players    []Player  `json:"players"`
}

type RoleCount struct {
	Role  string `json:"role"`
	Count int    `json:"count"`
}

type RosterTeamSummary struct {
	PlayerCount      int         `json:"player_count"`
	SkillTotal       int         `json:"skill_total"`
	RoleCounts       []RoleCount `json:"role_counts,omitempty"`
	MaxLatencyMillis int         `json:"max_latency_millis"`
}

type BackfillTicket struct {
	ID              TicketID            `json:"id"`
	Revision        Revision            `json:"revision"`
	SessionID       SessionID           `json:"session_id"`
	RosterVersion   Revision            `json:"roster_version"`
	OpenSlotsByTeam []int               `json:"open_slots_by_team"`
	ExistingTeams   []RosterTeamSummary `json:"existing_teams,omitempty"`
	EnqueuedAt      time.Time           `json:"enqueued_at"`
}

type RoleRequirement struct {
	Role       string `json:"role"`
	MinPerTeam int    `json:"min_per_team"`
	Hard       bool   `json:"hard"`
}

type RelaxationStep struct {
	AfterWait       time.Duration `json:"after_wait"`
	MaxTeamSkillGap int           `json:"max_team_skill_gap"`
	MaxRolePenalty  int           `json:"max_role_penalty"`
	PrioritizeWait  bool          `json:"prioritize_wait"`
}

type MatchmakingPolicy struct {
	Version                  string            `json:"version"`
	TeamCount                int               `json:"team_count"`
	TeamSize                 int               `json:"team_size"`
	MaxLatencyMillis         int               `json:"max_latency_millis"`
	MaxProposals             int               `json:"max_proposals,omitempty"`
	MaxSearchNodes           int               `json:"max_search_nodes,omitempty"`
	MaxCandidateTickets      int               `json:"max_candidate_tickets,omitempty"`
	MaxCandidatesPerProposal int               `json:"max_candidates_per_proposal,omitempty"`
	MaxBatchCandidates       int               `json:"max_batch_candidates,omitempty"`
	MaxBatchSearchNodes      int               `json:"max_batch_search_nodes,omitempty"`
	RoleRequirements         []RoleRequirement `json:"role_requirements,omitempty"`
	RelaxationSteps          []RelaxationStep  `json:"relaxation_steps,omitempty"`
}

type Snapshot struct {
	ID              SnapshotID        `json:"id"`
	Now             time.Time         `json:"now"`
	MatchTickets    []MatchTicket     `json:"match_tickets"`
	BackfillTickets []BackfillTicket  `json:"backfill_tickets,omitempty"`
	Policy          MatchmakingPolicy `json:"policy"`
}

type TicketRef struct {
	ID       TicketID `json:"id"`
	Revision Revision `json:"revision"`
}

type BackfillTarget struct {
	Ticket        TicketRef `json:"ticket"`
	SessionID     SessionID `json:"session_id"`
	RosterVersion Revision  `json:"roster_version"`
}

type ProposalKind string

const (
	ProposalNewMatch ProposalKind = "new_match"
	ProposalBackfill ProposalKind = "backfill"
)

type TeamAssignment struct {
	Team    int         `json:"team"`
	Tickets []TicketRef `json:"tickets"`
}

type ScoreEvidence struct {
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

type BatchScoreEvidence struct {
	CandidateProposals           int   `json:"candidate_proposals"`
	SelectedProposals            int   `json:"selected_proposals"`
	SelectedBackfills            int   `json:"selected_backfills"`
	WaitPriorityEligibleDemands  int   `json:"wait_priority_eligible_demands"`
	WaitPrioritySelectedDemands  int   `json:"wait_priority_selected_demands"`
	OldestWaitPriorityMillis     int64 `json:"oldest_wait_priority_millis"`
	OldestSelectedPriorityMillis int64 `json:"oldest_selected_priority_millis"`
	TotalUtility                 int64 `json:"total_utility"`
	CandidateGenerationNodes     int   `json:"candidate_generation_nodes"`
	CandidateGenerationTruncated bool  `json:"candidate_generation_truncated"`
	SelectionNodes               int   `json:"selection_nodes"`
	SelectionTruncated           bool  `json:"selection_truncated"`
}

type MatchProposal struct {
	ID                ProposalID        `json:"id"`
	Kind              ProposalKind      `json:"kind"`
	PolicyVersion     string            `json:"policy_version"`
	PolicyFingerprint PolicyFingerprint `json:"policy_fingerprint"`
	Teams             []TeamAssignment  `json:"teams"`
	Tickets           []TicketRef       `json:"tickets"`
	Backfill          *BackfillTarget   `json:"backfill,omitempty"`
	Evidence          ScoreEvidence     `json:"evidence"`
}

type UnmatchedReason string

const (
	UnmatchedHardConstraint       UnmatchedReason = "hard_constraint"
	UnmatchedInsufficientCapacity UnmatchedReason = "insufficient_capacity"
	UnmatchedQualityThreshold     UnmatchedReason = "quality_threshold"
	UnmatchedSearchBudget         UnmatchedReason = "search_budget"
	UnmatchedProposalLimit        UnmatchedReason = "proposal_limit"
	UnmatchedBatchObjective       UnmatchedReason = "batch_objective"
)

type UnmatchedTicket struct {
	Ticket TicketRef       `json:"ticket"`
	Reason UnmatchedReason `json:"reason"`
}

type ProposalBatch struct {
	APIVersion      string             `json:"api_version"`
	SnapshotID      SnapshotID         `json:"snapshot_id"`
	Proposals       []MatchProposal    `json:"proposals"`
	Unmatched       []UnmatchedTicket  `json:"unmatched"`
	BudgetExhausted bool               `json:"budget_exhausted"`
	Evidence        BatchScoreEvidence `json:"evidence"`
}
