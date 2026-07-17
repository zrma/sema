package alpha_test

import (
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/zrma/sema/alpha"
)

var fixtureNow = time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)

func TestComposeProvidesDeterministicExternalSurface(t *testing.T) {
	snapshot := alpha.Snapshot{
		ID: "external-2v2", Now: fixtureNow,
		Policy: alpha.MatchmakingPolicy{
			Version: "external-v1", TeamCount: 2, TeamSize: 2, MaxLatencyMillis: 200,
		},
		MatchTickets: soloTickets(8),
	}
	first, err := alpha.Compose(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	reversed := snapshot
	reversed.MatchTickets = slices.Clone(snapshot.MatchTickets)
	slices.Reverse(reversed.MatchTickets)
	second, err := alpha.Compose(reversed)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("input order changed public result: first=%#v second=%#v", first, second)
	}
	if first.APIVersion != alpha.APIVersion || len(first.Proposals) != 2 || len(first.Unmatched) != 0 {
		t.Fatalf("public batch = %#v", first)
	}
	if first.Evidence.SelectedProposals != 2 || first.Evidence.CandidateProposals < 2 || first.Evidence.TotalUtility <= 0 {
		t.Fatalf("public batch selection evidence = %#v", first.Evidence)
	}
	seen := make(map[alpha.TicketID]struct{})
	for _, proposal := range first.Proposals {
		for _, ticket := range proposal.Tickets {
			if _, duplicate := seen[ticket.ID]; duplicate {
				t.Fatalf("ticket %q appears twice", ticket.ID)
			}
			seen[ticket.ID] = struct{}{}
		}
	}
}

func TestComposeReturnsTypedAlphaFailure(t *testing.T) {
	_, err := alpha.Compose(alpha.Snapshot{})
	if code, ok := alpha.ErrorCodeOf(err); !ok || code != alpha.ErrorInvalidInput {
		t.Fatalf("error = %v, code = %q; want %q", err, code, alpha.ErrorInvalidInput)
	}
}

func TestComposeCopiesInputAndExposesCandidateWindow(t *testing.T) {
	tickets := soloTickets(4)
	snapshot := alpha.Snapshot{
		ID: "external-window", Now: fixtureNow, MatchTickets: tickets,
		Policy: alpha.MatchmakingPolicy{
			Version: "external-window-v1", TeamCount: 2, TeamSize: 1, MaxLatencyMillis: 200,
			MaxProposals: 1, MaxCandidateTickets: 2,
		},
	}
	original := slices.Clone(snapshot.MatchTickets[0].Players)
	batch, err := alpha.Compose(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(snapshot.MatchTickets[0].Players, original) {
		t.Fatal("Compose mutated public input")
	}
	evidence := batch.Proposals[0].Evidence
	if evidence.CandidateTickets != 2 || !evidence.CandidateWindowTruncated || !batch.BudgetExhausted {
		t.Fatalf("candidate window evidence = %#v, batch=%#v", evidence, batch)
	}
}

func soloTickets(count int) []alpha.MatchTicket {
	tickets := make([]alpha.MatchTicket, count)
	for index := range tickets {
		tickets[index] = alpha.MatchTicket{
			ID: alpha.TicketID(string(rune('a'+index)) + "-ticket"), Revision: 1,
			EnqueuedAt: fixtureNow.Add(-time.Duration(count-index) * time.Second),
			Players: []alpha.Player{{
				ID: alpha.PlayerID(string(rune('a'+index)) + "-player"), Skill: 1000 + index, LatencyMillis: 20,
			}},
		}
	}
	return tickets
}
