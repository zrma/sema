package domain_test

import (
	"testing"
	"time"

	"sema/internal/domain"
)

func TestValidateSnapshotRejectsDuplicatePlayers(t *testing.T) {
	now := time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC)
	snapshot := domain.MatchmakingSnapshot{
		ID:  "duplicate-player",
		Now: now,
		Policy: domain.MatchmakingPolicy{
			Version:          "test-v1",
			TeamCount:        2,
			TeamSize:         1,
			MaxLatencyMillis: 200,
		},
		MatchTickets: []domain.MatchTicket{
			{
				ID:         "ticket-a",
				Revision:   1,
				EnqueuedAt: now.Add(-time.Second),
				Players:    []domain.Player{{ID: "same-player", Skill: 1000, LatencyMillis: 20}},
			},
			{
				ID:         "ticket-b",
				Revision:   1,
				EnqueuedAt: now.Add(-time.Second),
				Players:    []domain.Player{{ID: "same-player", Skill: 1000, LatencyMillis: 20}},
			},
		},
	}

	err := domain.ValidateSnapshot(snapshot)
	assertFailureCode(t, err, domain.FailureInvalidInput)
}

func TestValidateProposalRequiresCanonicalFlattening(t *testing.T) {
	proposal := domain.MatchProposal{
		ID:            "proposal-1",
		Kind:          domain.ProposalNewMatch,
		PolicyVersion: "test-v1",
		Teams: []domain.TeamAssignment{
			{Team: 0, Tickets: []domain.TicketRef{{ID: "ticket-a", Revision: 1}}},
			{Team: 1, Tickets: []domain.TicketRef{{ID: "ticket-b", Revision: 1}}},
		},
		Tickets: []domain.TicketRef{{ID: "ticket-b", Revision: 1}, {ID: "ticket-a", Revision: 1}},
	}

	err := domain.ValidateProposal(proposal)
	assertFailureCode(t, err, domain.FailureInvalidInput)
}

func assertFailureCode(t *testing.T, err error, expected domain.FailureCode) {
	t.Helper()
	code, ok := domain.FailureCodeOf(err)
	if !ok || code != expected {
		t.Fatalf("failure code = %q, %v; want %q", code, err, expected)
	}
}
