package targetapi

import (
	"fmt"
	"net/http"
	"testing"

	api "github.com/zrma/sema/internal/api/v0alpha2"
	"github.com/zrma/sema/internal/repository"
)

func TestBackfillTicketLifecyclePreservesRosterFreshnessAndHistoricalReplay(t *testing.T) {
	handler := newTestHandler(t, repository.NewMemory())
	first := backfillTicket("backfill-a", "session-a", 1, 7)
	created := requestData[api.BackfillTicketMutation](
		t, handler, "tenant-a", "create-backfill-a", http.MethodPut,
		"/v0alpha2/backfill-tickets/backfill-a", first, http.StatusOK,
	)
	if created.Replayed || created.Resource.StorageVersion == 0 || created.Resource.Ticket.RosterVersion != 7 {
		t.Fatalf("created backfill = %#v", created)
	}
	second := backfillTicket("backfill-a", "session-a", 2, 8)
	second.ExistingTeams[0].SkillTotal = 1520
	replaced := requestData[api.BackfillTicketMutation](
		t, handler, "tenant-a", "replace-backfill-a", http.MethodPut,
		"/v0alpha2/backfill-tickets/backfill-a", second, http.StatusOK,
	)
	if replaced.Resource.StorageVersion <= created.Resource.StorageVersion || replaced.Resource.Ticket.Revision != 2 {
		t.Fatalf("replaced backfill = %#v; created=%#v", replaced, created)
	}

	replayed := requestData[api.BackfillTicketMutation](
		t, handler, "tenant-a", "create-backfill-a", http.MethodPut,
		"/v0alpha2/backfill-tickets/backfill-a", first, http.StatusOK,
	)
	if !replayed.Replayed || replayed.Resource.StorageVersion != created.Resource.StorageVersion ||
		replayed.Resource.Ticket.Revision != 1 {
		t.Fatalf("historical backfill replay = %#v", replayed)
	}
	current := requestData[api.BackfillTicketResource](
		t, handler, "tenant-a", "", http.MethodGet,
		"/v0alpha2/backfill-tickets/backfill-a", nil, http.StatusOK,
	)
	if current.Ticket.Revision != 2 || current.Ticket.RosterVersion != 8 ||
		current.StorageVersion != replaced.Resource.StorageVersion {
		t.Fatalf("current backfill = %#v", current)
	}

	changedWithoutRosterAdvance := second
	changedWithoutRosterAdvance.Revision = 3
	changedWithoutRosterAdvance.OpenSlotsByTeam = []int{2, 0}
	requestFailure(
		t, handler, "tenant-a", "invalid-roster-context", http.MethodPut,
		"/v0alpha2/backfill-tickets/backfill-a", changedWithoutRosterAdvance,
		http.StatusConflict, "InvalidRevision",
	)
	requestFailure(
		t, handler, "tenant-a", "cancel-stale-backfill", http.MethodDelete,
		"/v0alpha2/backfill-tickets/backfill-a?revision=2&roster_version=7", nil,
		http.StatusConflict, "StaleSnapshot",
	)
	cancelled := requestData[api.BackfillTicketCancellation](
		t, handler, "tenant-a", "cancel-backfill-a", http.MethodDelete,
		"/v0alpha2/backfill-tickets/backfill-a?revision=2&roster_version=8", nil, http.StatusOK,
	)
	if cancelled.Replayed || cancelled.RosterVersion != 8 {
		t.Fatalf("cancelled backfill = %#v", cancelled)
	}
	retried := requestData[api.BackfillTicketCancellation](
		t, handler, "tenant-a", "cancel-backfill-a", http.MethodDelete,
		"/v0alpha2/backfill-tickets/backfill-a?revision=2&roster_version=8", nil, http.StatusOK,
	)
	if !retried.Replayed || retried.StorageVersion != cancelled.StorageVersion {
		t.Fatalf("replayed backfill cancellation = %#v", retried)
	}
	requestFailure(
		t, handler, "tenant-a", "", http.MethodGet,
		"/v0alpha2/backfill-tickets/backfill-a", nil, http.StatusNotFound, "NotFound",
	)
	page := requestData[api.BackfillTicketPage](
		t, handler, "tenant-a", "", http.MethodGet,
		"/v0alpha2/backfill-tickets", nil, http.StatusOK,
	)
	if len(page.Items) != 0 {
		t.Fatalf("cancelled backfill remained active: %#v", page)
	}
	requestData[api.BackfillTicketMutation](
		t, handler, "tenant-a", "reuse-session", http.MethodPut,
		"/v0alpha2/backfill-tickets/backfill-b", backfillTicket("backfill-b", "session-a", 1, 9),
		http.StatusOK,
	)
}

func TestBackfillTicketEnforcesDemandAndSessionClaims(t *testing.T) {
	handler := newTestHandler(t, repository.NewMemory())
	requestData[api.MatchTicketMutation](
		t, handler, "tenant-a", "create-shared-match", http.MethodPut,
		"/v0alpha2/match-tickets/shared-ticket", matchTicket("shared-ticket", 1), http.StatusOK,
	)
	requestFailure(
		t, handler, "tenant-a", "create-shared-backfill", http.MethodPut,
		"/v0alpha2/backfill-tickets/shared-ticket", backfillTicket("shared-ticket", "session-shared", 1, 7),
		http.StatusConflict, "InvalidRevision",
	)
	requestData[api.BackfillTicketMutation](
		t, handler, "tenant-a", "create-session-owner", http.MethodPut,
		"/v0alpha2/backfill-tickets/backfill-owner", backfillTicket("backfill-owner", "session-one", 1, 7),
		http.StatusOK,
	)
	requestFailure(
		t, handler, "tenant-a", "create-session-duplicate", http.MethodPut,
		"/v0alpha2/backfill-tickets/backfill-other", backfillTicket("backfill-other", "session-one", 1, 7),
		http.StatusBadRequest, "InvalidInput",
	)
	requestFailure(
		t, handler, "tenant-b", "", http.MethodGet,
		"/v0alpha2/backfill-tickets/backfill-owner", nil, http.StatusNotFound, "NotFound",
	)
	requestFailure(
		t, handler, "reader-a", "reader-backfill-create", http.MethodPut,
		"/v0alpha2/backfill-tickets/reader-backfill", backfillTicket("reader-backfill", "reader-session", 1, 7),
		http.StatusForbidden, "PermissionDenied",
	)
}

func TestBackfillTicketPaginationIsStableAndKindBound(t *testing.T) {
	handler := newTestHandler(t, repository.NewMemory())
	for index, id := range []string{"backfill-a", "backfill-b", "backfill-c"} {
		requestData[api.BackfillTicketMutation](
			t, handler, "tenant-a", fmt.Sprintf("create-backfill-%d", index), http.MethodPut,
			"/v0alpha2/backfill-tickets/"+id,
			backfillTicket(id, fmt.Sprintf("session-%d", index), 1, 7), http.StatusOK,
		)
	}
	first := requestData[api.BackfillTicketPage](
		t, handler, "tenant-a", "", http.MethodGet,
		"/v0alpha2/backfill-tickets?limit=2", nil, http.StatusOK,
	)
	if len(first.Items) != 2 || first.Items[0].Ticket.ID != "backfill-a" ||
		first.Items[1].Ticket.ID != "backfill-b" || first.NextCursor == "" {
		t.Fatalf("first backfill page = %#v", first)
	}
	second := requestData[api.BackfillTicketPage](
		t, handler, "tenant-a", "", http.MethodGet,
		"/v0alpha2/backfill-tickets?cursor="+first.NextCursor, nil, http.StatusOK,
	)
	if len(second.Items) != 1 || second.Items[0].Ticket.ID != "backfill-c" || second.NextCursor != "" {
		t.Fatalf("second backfill page = %#v", second)
	}
	requestFailure(
		t, handler, "tenant-a", "", http.MethodGet,
		"/v0alpha2/match-tickets?cursor="+first.NextCursor, nil,
		http.StatusBadRequest, "InvalidInput",
	)
}

func backfillTicket(
	id string,
	sessionID string,
	revision uint64,
	rosterVersion uint64,
) api.BackfillTicket {
	return api.BackfillTicket{
		ID: id, Revision: revision, SessionID: sessionID, RosterVersion: rosterVersion,
		OpenSlotsByTeam: []int{1, 1},
		ExistingTeams: []api.RosterTeamSummary{
			{PlayerCount: 1, SkillTotal: 1500, RoleCounts: []api.RoleCount{{Role: "front", Count: 1}}, MaxLatencyMillis: 30},
			{PlayerCount: 1, SkillTotal: 1500, RoleCounts: []api.RoleCount{{Role: "back", Count: 1}}, MaxLatencyMillis: 40},
		},
		EnqueuedAt: targetFixtureNow,
	}
}
