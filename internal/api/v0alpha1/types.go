// Package v0alpha1 defines the experimental Sema service wire DTOs.
package v0alpha1

import "time"

const Version = "v0alpha1"

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

type BackfillTicket struct {
	ID              string    `json:"id"`
	Revision        uint64    `json:"revision"`
	SessionID       string    `json:"session_id"`
	RosterVersion   uint64    `json:"roster_version"`
	OpenSlotsByTeam []int     `json:"open_slots_by_team"`
	EnqueuedAt      time.Time `json:"enqueued_at"`
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

type ProposalBatch struct {
	SnapshotID      string             `json:"snapshot_id"`
	Proposals       []MatchProposal    `json:"proposals"`
	Unmatched       []UnmatchedTicket  `json:"unmatched"`
	BudgetExhausted bool               `json:"budget_exhausted"`
	Evidence        BatchScoreEvidence `json:"evidence"`
}

type Reservation struct {
	ID         string          `json:"id"`
	ProposalID string          `json:"proposal_id"`
	Tickets    []TicketRef     `json:"tickets"`
	Backfill   *BackfillTarget `json:"backfill,omitempty"`
	ExpiresAt  time.Time       `json:"expires_at"`
	Status     string          `json:"status"`
}

type AssignmentAcknowledgment struct {
	OperationID            string    `json:"operation_id"`
	Outcome                string    `json:"outcome"`
	SessionID              string    `json:"session_id,omitempty"`
	ExpectedRosterVersion  uint64    `json:"expected_roster_version,omitempty"`
	ResultingRosterVersion uint64    `json:"resulting_roster_version,omitempty"`
	FailureCode            string    `json:"failure_code,omitempty"`
	Reason                 string    `json:"reason,omitempty"`
	AcknowledgedAt         time.Time `json:"acknowledged_at"`
}

type Assignment struct {
	ID             string                    `json:"id"`
	ReservationID  string                    `json:"reservation_id"`
	ProposalID     string                    `json:"proposal_id"`
	Kind           string                    `json:"kind"`
	Teams          []TeamAssignment          `json:"teams"`
	Backfill       *BackfillTarget           `json:"backfill,omitempty"`
	ConfirmedAt    time.Time                 `json:"confirmed_at"`
	Status         string                    `json:"status"`
	Acknowledgment *AssignmentAcknowledgment `json:"acknowledgment,omitempty"`
}

type PolicyRegistration struct {
	Policy      MatchmakingPolicy `json:"policy"`
	Fingerprint string            `json:"fingerprint"`
}

type MutationResult struct {
	Status string `json:"status"`
}

type AuditSummary struct {
	Sequence uint64          `json:"sequence"`
	Kind     string          `json:"kind"`
	Checksum string          `json:"checksum"`
	Counts   map[string]int  `json:"counts,omitempty"`
	Flags    map[string]bool `json:"flags,omitempty"`
	Outcome  string          `json:"outcome,omitempty"`
}

type AuditPage struct {
	Records      []AuditSummary `json:"records"`
	NextSequence uint64         `json:"next_sequence"`
}

type PlanRequest struct {
	SnapshotID    string `json:"snapshot_id"`
	PolicyVersion string `json:"policy_version"`
}

type ReserveRequest struct {
	ProposalID string `json:"proposal_id"`
}

type ConfirmRequest struct {
	AssignmentID string `json:"assignment_id"`
}

type CancelReservationRequest struct{}

type AcknowledgeAssignmentRequest struct {
	OperationID            string `json:"operation_id"`
	Outcome                string `json:"outcome"`
	SessionID              string `json:"session_id,omitempty"`
	ExpectedRosterVersion  uint64 `json:"expected_roster_version,omitempty"`
	ResultingRosterVersion uint64 `json:"resulting_roster_version,omitempty"`
	FailureCode            string `json:"failure_code,omitempty"`
	Reason                 string `json:"reason,omitempty"`
}
