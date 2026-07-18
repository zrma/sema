package targetapi

import (
	"net/http"
	"strconv"

	api "github.com/zrma/sema/internal/api/v0alpha2"
	"github.com/zrma/sema/internal/domain"
	"github.com/zrma/sema/internal/service"
)

func (server *server) confirmReservation(writer http.ResponseWriter, request *http.Request) {
	principal, ok := authorize(writer, request, PermissionReservationsWrite)
	if !ok || !validateNoQuery(writer, request) {
		return
	}
	reservationID, ok := reservationID(writer, request)
	if !ok {
		return
	}
	operationID, ok := idempotencyKey(writer, request)
	if !ok {
		return
	}
	var command api.ConfirmReservationRequest
	if !decodeRequest(writer, request, &command) {
		return
	}
	if !validIdentifier(command.AssignmentID) {
		writeFailure(writer, domain.NewFailure(domain.FailureInvalidInput, "assignment ID is invalid"))
		return
	}
	result, err := server.reservations.Confirm(
		request.Context(), principal.Tenant, domain.OperationID(operationID),
		domain.ReservationID(reservationID), domain.AssignmentID(command.AssignmentID),
	)
	if err != nil {
		writeFailure(writer, err)
		return
	}
	writeData(writer, http.StatusOK, assignmentMutation(result))
}

func (server *server) acknowledgeAssignment(writer http.ResponseWriter, request *http.Request) {
	principal, ok := authorize(writer, request, PermissionAssignmentsWrite)
	if !ok || !validateNoQuery(writer, request) {
		return
	}
	id, ok := assignmentID(writer, request)
	if !ok {
		return
	}
	operationID, ok := idempotencyKey(writer, request)
	if !ok {
		return
	}
	var command api.AcknowledgeAssignmentRequest
	if !decodeRequest(writer, request, &command) {
		return
	}
	result, err := server.assignments.Acknowledge(
		request.Context(), principal.Tenant, domain.AssignmentID(id),
		api.ToDomainAcknowledgment(operationID, command),
	)
	if err != nil {
		writeFailure(writer, err)
		return
	}
	writeData(writer, http.StatusOK, assignmentMutation(result))
}

func (server *server) getAssignment(writer http.ResponseWriter, request *http.Request) {
	principal, ok := authorize(writer, request, PermissionAssignmentsRead)
	if !ok || !validateNoQuery(writer, request) {
		return
	}
	id, ok := assignmentID(writer, request)
	if !ok {
		return
	}
	record, exists, err := server.assignments.Get(request.Context(), principal.Tenant, domain.AssignmentID(id))
	if err != nil {
		writeFailure(writer, err)
		return
	}
	if !exists {
		writeError(writer, apiError{
			status: http.StatusNotFound, code: "NotFound", message: "assignment was not found",
		})
		return
	}
	writeData(writer, http.StatusOK, assignmentResource(record))
}

func (server *server) listAssignments(writer http.ResponseWriter, request *http.Request) {
	principal, ok := authorize(writer, request, PermissionAssignmentsRead)
	if !ok {
		return
	}
	query, ok := parseQuery(writer, request, "cursor", "limit")
	if !ok {
		return
	}
	limit := defaultPageSize
	if value := singleQuery(query, "limit"); value != "" {
		parsed, err := strconv.Atoi(value)
		if err != nil || parsed < 1 || parsed > maxPageSize {
			writeFailure(writer, domain.NewFailure(
				domain.FailureInvalidInput,
				"query parameter limit must be an integer between 1 and %d",
				maxPageSize,
			))
			return
		}
		limit = parsed
	}
	binding := cursorBinding{
		Tenant: principal.Tenant, ResourceKind: string(service.ResourceAssignment),
		Filter: "all", Order: "resource_id.asc",
	}
	var position cursorPosition
	if token := singleQuery(query, "cursor"); token != "" {
		var err error
		position, err = server.cursors.decode(token, binding)
		if err != nil {
			writeFailure(writer, domain.NewFailure(domain.FailureInvalidInput, "invalid cursor"))
			return
		}
	}
	snapshot, err := server.assignments.Snapshot(request.Context(), principal.Tenant)
	if err != nil {
		writeFailure(writer, err)
		return
	}
	if position.RepositoryVersion != 0 && position.RepositoryVersion != snapshot.RepositoryVersion {
		writeFailure(writer, domain.NewFailure(
			domain.FailureStaleSnapshot,
			"cursor snapshot changed; restart pagination",
		))
		return
	}
	items := make([]api.AssignmentResource, 0, limit)
	hasMore := false
	for _, record := range snapshot.Records {
		if string(record.Assignment.ID) <= position.After {
			continue
		}
		if len(items) == limit {
			hasMore = true
			break
		}
		items = append(items, assignmentResource(record))
	}
	next := ""
	if hasMore {
		next, err = server.cursors.encode(binding, cursorPosition{
			RepositoryVersion: snapshot.RepositoryVersion,
			After:             items[len(items)-1].Assignment.ID,
		})
		if err != nil {
			writeFailure(writer, err)
			return
		}
	}
	writeData(writer, http.StatusOK, api.AssignmentPage{
		Items: items, RepositoryVersion: uint64(snapshot.RepositoryVersion), NextCursor: next,
	})
}

func assignmentID(writer http.ResponseWriter, request *http.Request) (string, bool) {
	id := request.PathValue("assignment_id")
	if !validIdentifier(id) {
		writeFailure(writer, domain.NewFailure(domain.FailureInvalidInput, "assignment ID is invalid"))
		return "", false
	}
	return id, true
}

func assignmentMutation(result service.AssignmentMutation) api.AssignmentMutation {
	return api.AssignmentMutation{Resource: assignmentResource(result.Record), Replayed: result.Replayed}
}

func assignmentResource(record service.AssignmentRecord) api.AssignmentResource {
	return api.AssignmentResource{
		Assignment:     api.FromDomainAssignment(record.Assignment),
		StorageVersion: uint64(record.StorageVersion),
	}
}
