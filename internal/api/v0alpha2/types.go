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
