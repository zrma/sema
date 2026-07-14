package domain

import "time"

type (
	PlayerID      string
	TicketID      string
	SessionID     string
	SnapshotID    string
	ProposalID    string
	ReservationID string
	AssignmentID  string
	Revision      uint64
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

// MatchmakingPolicy defines the bounded P0 planning envelope.
type MatchmakingPolicy struct {
	Version          string
	TeamCount        int
	TeamSize         int
	MaxLatencyMillis int
	MaxProposals     int
	MaxSearchNodes   int
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
	TeamSkillGap     int
	MaxLatencyMillis int
	OldestWaitMillis int64
	SearchNodes      int
}

// MatchProposal is a side-effect-free placement proposal.
type MatchProposal struct {
	ID            ProposalID
	Kind          ProposalKind
	PolicyVersion string
	Teams         []TeamAssignment
	Tickets       []TicketRef
	Backfill      *BackfillTarget
	Evidence      ScoreEvidence
}

type ProposalBatch struct {
	SnapshotID      SnapshotID
	Proposals       []MatchProposal
	Unmatched       []TicketRef
	BudgetExhausted bool
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
	ID            AssignmentID
	ReservationID ReservationID
	ProposalID    ProposalID
	Kind          ProposalKind
	Teams         []TeamAssignment
	Backfill      *BackfillTarget
	ConfirmedAt   time.Time
}
