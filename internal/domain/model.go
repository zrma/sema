package domain

import "time"

type (
	PlayerID          string
	TicketID          string
	SessionID         string
	SnapshotID        string
	ProposalID        string
	ReservationID     string
	AssignmentID      string
	OperationID       string
	PolicyFingerprint string
	Revision          uint64
)

// Player contains the P0 attributes used by constraints and evidence.
type Player struct {
	ID            PlayerID
	Skill         int
	Role          string
	LatencyMillis int
}

// MatchTicket is an indivisible party seeking placement.
type MatchTicket struct {
	ID         TicketID
	Revision   Revision
	EnqueuedAt time.Time
	Players    []Player
}

// BackfillTicket describes exact vacancies in an existing session roster.
type BackfillTicket struct {
	ID              TicketID
	Revision        Revision
	SessionID       SessionID
	RosterVersion   Revision
	OpenSlotsByTeam []int
	EnqueuedAt      time.Time
}

type RoleRequirement struct {
	Role       string
	MinPerTeam int
	Hard       bool
}

type RelaxationStep struct {
	AfterWait       time.Duration
	MaxTeamSkillGap int
	MaxRolePenalty  int
	PrioritizeWait  bool
}

// MatchmakingPolicy defines the bounded P0 planning envelope.
type MatchmakingPolicy struct {
	Version                  string
	TeamCount                int
	TeamSize                 int
	MaxLatencyMillis         int
	MaxProposals             int
	MaxSearchNodes           int
	MaxCandidateTickets      int
	MaxCandidatesPerProposal int
	MaxBatchCandidates       int
	MaxBatchSearchNodes      int
	RoleRequirements         []RoleRequirement
	RelaxationSteps          []RelaxationStep
}

// MatchmakingSnapshot is an immutable input to the planner.
type MatchmakingSnapshot struct {
	ID              SnapshotID
	Now             time.Time
	MatchTickets    []MatchTicket
	BackfillTickets []BackfillTicket
	Policy          MatchmakingPolicy
}

type TicketRef struct {
	ID       TicketID
	Revision Revision
}

type BackfillTarget struct {
	Ticket        TicketRef
	SessionID     SessionID
	RosterVersion Revision
}

type ProposalKind string

const (
	ProposalNewMatch ProposalKind = "new_match"
	ProposalBackfill ProposalKind = "backfill"
)

type TeamAssignment struct {
	Team    int
	Tickets []TicketRef
}

type ScoreEvidence struct {
	RelaxationLevel          int
	WaitPriority             bool
	RolePenalty              int
	TeamSkillGap             int
	OldestWaitMillis         int64
	TotalWaitMillis          int64
	MaxLatencyMillis         int
	CandidateTickets         int
	CandidatesEvaluated      int
	SearchNodes              int
	CandidateWindowTruncated bool
	SearchTruncated          bool
	SelectionUtility         int64
}

// BatchScoreEvidence explains the bounded global proposal selection.
type BatchScoreEvidence struct {
	CandidateProposals           int
	SelectedProposals            int
	SelectedBackfills            int
	TotalUtility                 int64
	CandidateGenerationNodes     int
	CandidateGenerationTruncated bool
	SelectionNodes               int
	SelectionTruncated           bool
}

// MatchProposal is a side-effect-free placement proposal.
type MatchProposal struct {
	ID                ProposalID
	Kind              ProposalKind
	PolicyVersion     string
	PolicyFingerprint PolicyFingerprint
	Teams             []TeamAssignment
	Tickets           []TicketRef
	Backfill          *BackfillTarget
	Evidence          ScoreEvidence
}

type ProposalBatch struct {
	SnapshotID      SnapshotID
	Proposals       []MatchProposal
	Unmatched       []UnmatchedTicket
	BudgetExhausted bool
	Evidence        BatchScoreEvidence
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
	Ticket TicketRef
	Reason UnmatchedReason
}

type ReservationStatus string

const (
	ReservationActive    ReservationStatus = "active"
	ReservationConfirmed ReservationStatus = "confirmed"
	ReservationCancelled ReservationStatus = "cancelled"
	ReservationExpired   ReservationStatus = "expired"
)

type Reservation struct {
	ID         ReservationID
	ProposalID ProposalID
	Tickets    []TicketRef
	Backfill   *BackfillTarget
	ExpiresAt  time.Time
	Status     ReservationStatus
}

type Assignment struct {
	ID             AssignmentID
	ReservationID  ReservationID
	ProposalID     ProposalID
	Kind           ProposalKind
	Teams          []TeamAssignment
	Backfill       *BackfillTarget
	ConfirmedAt    time.Time
	Status         AssignmentStatus
	Acknowledgment *AssignmentAcknowledgment
}

type AssignmentStatus string

const (
	AssignmentPending   AssignmentStatus = "pending"
	AssignmentCompleted AssignmentStatus = "completed"
	AssignmentCancelled AssignmentStatus = "cancelled"
	AssignmentFailed    AssignmentStatus = "failed"
)

type AssignmentAcknowledgmentRequest struct {
	OperationID            OperationID
	Outcome                AssignmentStatus
	SessionID              SessionID
	ExpectedRosterVersion  Revision
	ResultingRosterVersion Revision
	FailureCode            FailureCode
	Reason                 string
}

type AssignmentAcknowledgment struct {
	AssignmentAcknowledgmentRequest
	AcknowledgedAt time.Time
}
