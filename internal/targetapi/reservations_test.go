package targetapi

import (
	"bytes"
	"fmt"
	"net/http"
	"testing"
	"time"

	api "github.com/zrma/sema/internal/api/v0alpha2"
	"github.com/zrma/sema/internal/repository"
)

func TestReservationAPIClaimsCancelsListsAndReplays(t *testing.T) {
	handler := newTestHandler(t, repository.NewMemory())
	proposalID := seedReservationAPIProposal(t, handler, "tenant-a")
	created := requestData[api.ReservationMutation](
		t, handler, "tenant-a", "reserve-api", http.MethodPost,
		"/v0alpha2/reservations/reservation-api",
		api.ReservationRequest{ProposalID: proposalID}, http.StatusOK,
	)
	if created.Replayed || created.Resource.Reservation.Status != "active" ||
		created.Resource.Reservation.ProposalID != proposalID || created.Resource.StorageVersion == 0 {
		t.Fatalf("created reservation = %#v", created)
	}
	polled := requestData[api.ReservationResource](
		t, handler, "tenant-a", "", http.MethodGet,
		"/v0alpha2/reservations/reservation-api", nil, http.StatusOK,
	)
	if polled.StorageVersion != created.Resource.StorageVersion || polled.Reservation.Status != "active" {
		t.Fatalf("polled reservation = %#v; created=%#v", polled, created)
	}
	requestFailure(
		t, handler, "tenant-b", "", http.MethodGet,
		"/v0alpha2/reservations/reservation-api", nil, http.StatusNotFound, "NotFound",
	)
	requestFailure(
		t, handler, "reader-a", "reader-reserve", http.MethodPost,
		"/v0alpha2/reservations/reader-reservation",
		api.ReservationRequest{ProposalID: proposalID}, http.StatusForbidden, "PermissionDenied",
	)
	page := requestData[api.ReservationPage](
		t, handler, "tenant-a", "", http.MethodGet,
		"/v0alpha2/reservations?limit=1", nil, http.StatusOK,
	)
	if len(page.Items) != 1 || page.Items[0].Reservation.ID != "reservation-api" {
		t.Fatalf("reservation page = %#v", page)
	}
	cancelled := requestData[api.ReservationMutation](
		t, handler, "tenant-a", "cancel-api", http.MethodPost,
		"/v0alpha2/reservations/reservation-api/cancel", struct{}{}, http.StatusOK,
	)
	if cancelled.Replayed || cancelled.Resource.Reservation.Status != "cancelled" {
		t.Fatalf("cancelled reservation = %#v", cancelled)
	}
	replayedCancel := requestData[api.ReservationMutation](
		t, handler, "tenant-a", "cancel-api", http.MethodPost,
		"/v0alpha2/reservations/reservation-api/cancel", struct{}{}, http.StatusOK,
	)
	if !replayedCancel.Replayed || replayedCancel.Resource.StorageVersion != cancelled.Resource.StorageVersion {
		t.Fatalf("replayed cancellation = %#v; cancelled=%#v", replayedCancel, cancelled)
	}
	replayedReserve := requestData[api.ReservationMutation](
		t, handler, "tenant-a", "reserve-api", http.MethodPost,
		"/v0alpha2/reservations/reservation-api",
		api.ReservationRequest{ProposalID: proposalID}, http.StatusOK,
	)
	if !replayedReserve.Replayed || replayedReserve.Resource.StorageVersion != created.Resource.StorageVersion ||
		replayedReserve.Resource.Reservation.Status != "active" {
		t.Fatalf("historical reserve replay = %#v; created=%#v", replayedReserve, created)
	}
}

func TestReservationAPIExpiresFromServerClockAndReleasesDemand(t *testing.T) {
	now := targetFixtureNow
	handler, err := New(repository.NewMemory(), fixtureAuthenticator(), Options{
		Now:            func() time.Time { return now },
		CursorKey:      bytes.Repeat([]byte{5}, 32),
		ReservationTTL: 5 * time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	proposalID := seedReservationAPIProposal(t, handler, "tenant-a")
	requestData[api.ReservationMutation](
		t, handler, "tenant-a", "reserve-expiring-api", http.MethodPost,
		"/v0alpha2/reservations/reservation-expiring-api",
		api.ReservationRequest{ProposalID: proposalID}, http.StatusOK,
	)
	now = now.Add(5 * time.Second)
	expired := requestData[api.ReservationResource](
		t, handler, "tenant-a", "", http.MethodGet,
		"/v0alpha2/reservations/reservation-expiring-api", nil, http.StatusOK,
	)
	if expired.Reservation.Status != "expired" {
		t.Fatalf("expired reservation = %#v", expired)
	}
	requestData[api.ReservationMutation](
		t, handler, "tenant-a", "reserve-after-expiry-api", http.MethodPost,
		"/v0alpha2/reservations/reservation-after-expiry-api",
		api.ReservationRequest{ProposalID: proposalID}, http.StatusOK,
	)
}

func TestTargetAPIRequiresExplicitReservationTTL(t *testing.T) {
	if _, err := New(repository.NewMemory(), fixtureAuthenticator(), Options{
		CursorKey: bytes.Repeat([]byte{4}, 32),
	}); err == nil {
		t.Fatal("target API accepted an implicit reservation TTL")
	}
}

func seedReservationAPIProposal(t *testing.T, handler http.Handler, tenant string) string {
	t.Helper()
	policy := matchmakingPolicy("reservation-policy")
	policy.RoleRequirements = nil
	policy.MaxProposals = 1
	policy.MaxSearchNodes = 100_000
	requestData[api.PolicyMutation](
		t, handler, tenant, "register-reservation-policy", http.MethodPut,
		"/v0alpha2/policies/reservation-policy", policy, http.StatusOK,
	)
	for index := 0; index < 4; index++ {
		id := fmt.Sprintf("reservation-ticket-%02d", index)
		requestData[api.MatchTicketMutation](
			t, handler, tenant, "create-"+id, http.MethodPut,
			"/v0alpha2/match-tickets/"+id, matchTicket(id, 1), http.StatusOK,
		)
	}
	created := requestData[api.PlanningRunMutation](
		t, handler, tenant, "execute-reservation-run", http.MethodPost,
		"/v0alpha2/planning-runs/reservation-run",
		api.PlanningRunRequest{PolicyVersion: "reservation-policy"}, http.StatusOK,
	)
	if created.Resource.ProposalCount != 1 {
		t.Fatalf("reservation planning run = %#v", created)
	}
	proposals := requestData[api.ProposalPage](
		t, handler, tenant, "", http.MethodGet,
		"/v0alpha2/planning-runs/reservation-run/proposals", nil, http.StatusOK,
	)
	if len(proposals.Items) != 1 {
		t.Fatalf("reservation proposals = %#v", proposals)
	}
	return proposals.Items[0].Proposal.ID
}
