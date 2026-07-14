package constraint_test

import (
	"testing"
	"time"

	"sema/internal/constraint"
	"sema/internal/domain"
)

func TestTicketAllowedKeepsPartyAndLatencyHard(t *testing.T) {
	ticket := domain.MatchTicket{
		ID:         "party",
		Revision:   1,
		EnqueuedAt: time.Unix(1, 0),
		Players: []domain.Player{
			{ID: "a", LatencyMillis: 20},
			{ID: "b", LatencyMillis: 20},
		},
	}
	if constraint.TicketAllowed(ticket, 1, 200) {
		t.Fatal("oversized party passed the hard constraint")
	}
	ticket.Players = ticket.Players[:1]
	ticket.Players[0].LatencyMillis = 201
	if constraint.TicketAllowed(ticket, 1, 200) {
		t.Fatal("latency above the absolute cap passed the hard constraint")
	}
}

func TestHardRoleIsNotAppliedToBackfillWithoutRosterData(t *testing.T) {
	policy := domain.MatchmakingPolicy{
		MaxLatencyMillis: 200,
		RoleRequirements: []domain.RoleRequirement{{Role: "healer", MinPerTeam: 1, Hard: true}},
	}
	teams := [][]domain.MatchTicket{{{
		ID:         "dps-party",
		Revision:   1,
		EnqueuedAt: time.Unix(1, 0),
		Players:    []domain.Player{{ID: "dps", Role: "dps", LatencyMillis: 20}},
	}}}
	if !constraint.HardViolation(teams, policy, domain.ProposalNewMatch) {
		t.Fatal("new match ignored the hard role requirement")
	}
	if constraint.HardViolation(teams, policy, domain.ProposalBackfill) {
		t.Fatal("backfill applied a hard role without existing roster data")
	}
}
