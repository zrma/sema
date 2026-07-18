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
