package targetapi

import (
	"net/http"
	"testing"

	api "github.com/zrma/sema/internal/api/v0alpha2"
	"github.com/zrma/sema/internal/repository"
)

func TestAssignmentAPIConfirmsPollsAcknowledgesAndReplays(t *testing.T) {
	handler := newTestHandler(t, repository.NewMemory())
	proposalID := seedReservationAPIProposal(t, handler, "tenant-a")
	requestData[api.ReservationMutation](
		t, handler, "tenant-a", "reserve-assignment-api", http.MethodPost,
		"/v0alpha2/reservations/reservation-assignment-api",
		api.ReservationRequest{ProposalID: proposalID}, http.StatusOK,
	)
	confirmed := requestData[api.AssignmentMutation](
		t, handler, "tenant-a", "confirm-assignment-api", http.MethodPost,
		"/v0alpha2/reservations/reservation-assignment-api/confirm",
		api.ConfirmReservationRequest{AssignmentID: "assignment-api"}, http.StatusOK,
	)
	if confirmed.Replayed || confirmed.Resource.Assignment.Status != "pending" ||
		confirmed.Resource.Assignment.ReservationID != "reservation-assignment-api" {
		t.Fatalf("confirmed assignment = %#v", confirmed)
	}
	polled := requestData[api.AssignmentResource](
		t, handler, "tenant-a", "", http.MethodGet,
		"/v0alpha2/assignments/assignment-api", nil, http.StatusOK,
	)
	if polled.StorageVersion != confirmed.Resource.StorageVersion || polled.Assignment.Status != "pending" {
		t.Fatalf("polled assignment = %#v; confirmed=%#v", polled, confirmed)
	}
	requestFailure(
		t, handler, "tenant-b", "", http.MethodGet,
		"/v0alpha2/assignments/assignment-api", nil, http.StatusNotFound, "NotFound",
	)
	page := requestData[api.AssignmentPage](
		t, handler, "tenant-a", "", http.MethodGet,
		"/v0alpha2/assignments?limit=1", nil, http.StatusOK,
	)
	if len(page.Items) != 1 || page.Items[0].Assignment.ID != "assignment-api" {
		t.Fatalf("assignment page = %#v", page)
	}
	requestFailure(
		t, handler, "reader-a", "reader-ack", http.MethodPost,
		"/v0alpha2/assignments/assignment-api/acknowledgments",
		api.AcknowledgeAssignmentRequest{Outcome: "completed"},
		http.StatusForbidden, "PermissionDenied",
	)
	completed := requestData[api.AssignmentMutation](
		t, handler, "tenant-a", "ack-assignment-api", http.MethodPost,
		"/v0alpha2/assignments/assignment-api/acknowledgments",
		api.AcknowledgeAssignmentRequest{Outcome: "completed"}, http.StatusOK,
	)
	if completed.Replayed || completed.Resource.Assignment.Status != "completed" ||
		completed.Resource.Assignment.Acknowledgment == nil ||
		completed.Resource.Assignment.Acknowledgment.OperationID != "ack-assignment-api" {
		t.Fatalf("completed assignment = %#v", completed)
	}
	replayedAck := requestData[api.AssignmentMutation](
		t, handler, "tenant-a", "ack-assignment-api", http.MethodPost,
		"/v0alpha2/assignments/assignment-api/acknowledgments",
		api.AcknowledgeAssignmentRequest{Outcome: "completed"}, http.StatusOK,
	)
	if !replayedAck.Replayed || replayedAck.Resource.StorageVersion != completed.Resource.StorageVersion {
		t.Fatalf("replayed acknowledgment = %#v; completed=%#v", replayedAck, completed)
	}
	replayedConfirm := requestData[api.AssignmentMutation](
		t, handler, "tenant-a", "confirm-assignment-api", http.MethodPost,
		"/v0alpha2/reservations/reservation-assignment-api/confirm",
		api.ConfirmReservationRequest{AssignmentID: "assignment-api"}, http.StatusOK,
	)
	if !replayedConfirm.Replayed || replayedConfirm.Resource.StorageVersion != confirmed.Resource.StorageVersion ||
		replayedConfirm.Resource.Assignment.Status != "pending" {
		t.Fatalf("historical confirm replay = %#v; confirmed=%#v", replayedConfirm, confirmed)
	}
	requestFailure(
		t, handler, "tenant-a", "ack-assignment-again", http.MethodPost,
		"/v0alpha2/assignments/assignment-api/acknowledgments",
		api.AcknowledgeAssignmentRequest{Outcome: "cancelled", Reason: "late cancellation"},
		http.StatusConflict, "InvalidTransition",
	)
}
