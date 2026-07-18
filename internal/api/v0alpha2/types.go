// Package v0alpha2 defines the authenticated target service wire schema.
package v0alpha2

import "time"

const Version = "v0alpha2"

type Envelope struct {
	APIVersion string   `json:"api_version"`
	Data       any      `json:"data,omitempty"`
	Error      *Failure `json:"error,omitempty"`
}

type Failure struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	Retryable bool   `json:"retryable"`
}

type Player struct {
	ID            string `json:"id"`
	Skill         int    `json:"skill"`
	Role          string `json:"role,omitempty"`
	LatencyMillis int    `json:"latency_millis"`
}

type MatchTicket struct {
	ID         string    `json:"id"`
	Revision   uint64    `json:"revision"`
	EnqueuedAt time.Time `json:"enqueued_at"`
	Players    []Player  `json:"players"`
}

type MatchTicketResource struct {
	Ticket         MatchTicket `json:"ticket"`
	StorageVersion uint64      `json:"storage_version"`
}

type MatchTicketMutation struct {
	Resource MatchTicketResource `json:"resource"`
	Replayed bool                `json:"replayed"`
}

type MatchTicketCancellation struct {
	ID             string `json:"id"`
	Revision       uint64 `json:"revision"`
	StorageVersion uint64 `json:"storage_version"`
	Replayed       bool   `json:"replayed"`
}

type MatchTicketPage struct {
	Items             []MatchTicketResource `json:"items"`
	RepositoryVersion uint64                `json:"repository_version"`
	NextCursor        string                `json:"next_cursor,omitempty"`
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
	ID              string              `json:"id"`
	Revision        uint64              `json:"revision"`
	SessionID       string              `json:"session_id"`
	RosterVersion   uint64              `json:"roster_version"`
	OpenSlotsByTeam []int               `json:"open_slots_by_team"`
	ExistingTeams   []RosterTeamSummary `json:"existing_teams,omitempty"`
	EnqueuedAt      time.Time           `json:"enqueued_at"`
}

type BackfillTicketResource struct {
	Ticket         BackfillTicket `json:"ticket"`
	StorageVersion uint64         `json:"storage_version"`
}

type BackfillTicketMutation struct {
	Resource BackfillTicketResource `json:"resource"`
	Replayed bool                   `json:"replayed"`
}

type BackfillTicketCancellation struct {
	ID             string `json:"id"`
	Revision       uint64 `json:"revision"`
	RosterVersion  uint64 `json:"roster_version"`
	StorageVersion uint64 `json:"storage_version"`
	Replayed       bool   `json:"replayed"`
}

type BackfillTicketPage struct {
	Items             []BackfillTicketResource `json:"items"`
	RepositoryVersion uint64                   `json:"repository_version"`
	NextCursor        string                   `json:"next_cursor,omitempty"`
}

type RoleRequirement struct {
	Role       string `json:"role"`
	MinPerTeam int    `json:"min_per_team"`
	Hard       bool   `json:"hard"`
}

type RelaxationStep struct {
	AfterWaitMillis int64 `json:"after_wait_millis"`
	MaxTeamSkillGap int   `json:"max_team_skill_gap"`
	MaxRolePenalty  int   `json:"max_role_penalty"`
	PrioritizeWait  bool  `json:"prioritize_wait"`
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

type PolicyResource struct {
	Policy         MatchmakingPolicy `json:"policy"`
	Fingerprint    string            `json:"fingerprint"`
	StorageVersion uint64            `json:"storage_version"`
}

type PolicyMutation struct {
	Resource PolicyResource `json:"resource"`
	Replayed bool           `json:"replayed"`
}

type PolicyPage struct {
	Items             []PolicyResource `json:"items"`
	RepositoryVersion uint64           `json:"repository_version"`
	NextCursor        string           `json:"next_cursor,omitempty"`
}

type TicketRef struct {
	ID       string `json:"id"`
	Revision uint64 `json:"revision"`
}

type BackfillTarget struct {
	Ticket        TicketRef `json:"ticket"`
	SessionID     string    `json:"session_id"`
	RosterVersion uint64    `json:"roster_version"`
}

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
	ID                string           `json:"id"`
	Kind              string           `json:"kind"`
	PolicyVersion     string           `json:"policy_version"`
	PolicyFingerprint string           `json:"policy_fingerprint"`
	Teams             []TeamAssignment `json:"teams"`
	Tickets           []TicketRef      `json:"tickets"`
	Backfill          *BackfillTarget  `json:"backfill,omitempty"`
	Evidence          ScoreEvidence    `json:"evidence"`
}

type UnmatchedTicket struct {
	Ticket TicketRef `json:"ticket"`
	Reason string    `json:"reason"`
}

type PlanningRunRequest struct {
	PolicyVersion string `json:"policy_version"`
}

type PlanningRunResource struct {
	ID                      string             `json:"id"`
	SnapshotID              string             `json:"snapshot_id"`
	PolicyVersion           string             `json:"policy_version"`
	PolicyFingerprint       string             `json:"policy_fingerprint"`
	SourceRepositoryVersion uint64             `json:"source_repository_version"`
	CapturedAt              time.Time          `json:"captured_at"`
	CompletedAt             *time.Time         `json:"completed_at,omitempty"`
	Status                  string             `json:"status"`
	ProposalCount           int                `json:"proposal_count"`
	UnmatchedCount          int                `json:"unmatched_count"`
	BudgetExhausted         bool               `json:"budget_exhausted"`
	Evidence                BatchScoreEvidence `json:"evidence"`
	StorageVersion          uint64             `json:"storage_version"`
}

type PlanningRunMutation struct {
	Resource PlanningRunResource `json:"resource"`
	Replayed bool                `json:"replayed"`
}

type ProposalResource struct {
	RunID          string        `json:"run_id"`
	Proposal       MatchProposal `json:"proposal"`
	StorageVersion uint64        `json:"storage_version"`
}

type ProposalPage struct {
	Items             []ProposalResource `json:"items"`
	RunStorageVersion uint64             `json:"run_storage_version"`
	NextCursor        string             `json:"next_cursor,omitempty"`
}

type UnmatchedResource struct {
	RunID          string          `json:"run_id"`
	Unmatched      UnmatchedTicket `json:"unmatched"`
	StorageVersion uint64          `json:"storage_version"`
}

type UnmatchedPage struct {
	Items             []UnmatchedResource `json:"items"`
	RunStorageVersion uint64              `json:"run_storage_version"`
	NextCursor        string              `json:"next_cursor,omitempty"`
}
