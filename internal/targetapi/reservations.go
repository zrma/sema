package targetapi

import (
	"net/http"
	"strconv"

	api "github.com/zrma/sema/internal/api/v0alpha2"
	"github.com/zrma/sema/internal/domain"
	"github.com/zrma/sema/internal/service"
)

func (server *server) postReservation(writer http.ResponseWriter, request *http.Request) {
	principal, ok := authorize(writer, request, PermissionReservationsWrite)
	if !ok || !validateNoQuery(writer, request) {
		return
	}
	id, ok := reservationID(writer, request)
	if !ok {
		return
	}
	operationID, ok := idempotencyKey(writer, request)
	if !ok {
		return
	}
	var command api.ReservationRequest
	if !decodeRequest(writer, request, &command) {
		return
	}
	if !validOpaqueResourceID(command.ProposalID) {
		writeFailure(writer, domain.NewFailure(domain.FailureInvalidInput, "proposal ID is invalid"))
		return
	}
	result, err := server.reservations.Reserve(
		request.Context(), principal.Tenant, domain.OperationID(operationID),
		domain.ReservationID(id), domain.ProposalID(command.ProposalID),
	)
	if err != nil {
		writeFailure(writer, err)
		return
	}
	writeData(writer, http.StatusOK, reservationMutation(result))
}

func validOpaqueResourceID(value string) bool {
	return len(value) <= 512 && validPrincipalValue(value)
}

func (server *server) cancelReservation(writer http.ResponseWriter, request *http.Request) {
	principal, ok := authorize(writer, request, PermissionReservationsWrite)
	if !ok || !validateNoQuery(writer, request) {
		return
	}
	id, ok := reservationID(writer, request)
	if !ok {
		return
	}
	operationID, ok := idempotencyKey(writer, request)
	if !ok {
		return
	}
	var command struct{}
	if !decodeRequest(writer, request, &command) {
		return
	}
	result, err := server.reservations.Cancel(
		request.Context(), principal.Tenant, domain.OperationID(operationID), domain.ReservationID(id),
	)
	if err != nil {
		writeFailure(writer, err)
		return
	}
	writeData(writer, http.StatusOK, reservationMutation(result))
}

func (server *server) getReservation(writer http.ResponseWriter, request *http.Request) {
	principal, ok := authorize(writer, request, PermissionReservationsRead)
	if !ok || !validateNoQuery(writer, request) {
		return
	}
	id, ok := reservationID(writer, request)
	if !ok {
		return
	}
	record, exists, err := server.reservations.Get(request.Context(), principal.Tenant, domain.ReservationID(id))
	if err != nil {
		writeFailure(writer, err)
		return
	}
	if !exists {
		writeError(writer, apiError{
			status: http.StatusNotFound, code: "NotFound", message: "reservation was not found",
		})
		return
	}
	writeData(writer, http.StatusOK, reservationResource(record))
}

func (server *server) listReservations(writer http.ResponseWriter, request *http.Request) {
	principal, ok := authorize(writer, request, PermissionReservationsRead)
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
		Tenant: principal.Tenant, ResourceKind: string(service.ResourceReservation),
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
	snapshot, err := server.reservations.Snapshot(request.Context(), principal.Tenant)
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
	items := make([]api.ReservationResource, 0, limit)
	hasMore := false
	for _, record := range snapshot.Records {
		if string(record.Reservation.ID) <= position.After {
			continue
		}
		if len(items) == limit {
			hasMore = true
			break
		}
		items = append(items, reservationResource(record))
	}
	next := ""
	if hasMore {
		next, err = server.cursors.encode(binding, cursorPosition{
			RepositoryVersion: snapshot.RepositoryVersion,
			After:             items[len(items)-1].Reservation.ID,
		})
		if err != nil {
			writeFailure(writer, err)
			return
		}
	}
	writeData(writer, http.StatusOK, api.ReservationPage{
		Items: items, RepositoryVersion: uint64(snapshot.RepositoryVersion), NextCursor: next,
	})
}

func reservationID(writer http.ResponseWriter, request *http.Request) (string, bool) {
	id := request.PathValue("reservation_id")
	if !validIdentifier(id) {
		writeFailure(writer, domain.NewFailure(domain.FailureInvalidInput, "reservation ID is invalid"))
		return "", false
	}
	return id, true
}

func reservationMutation(result service.ReservationMutation) api.ReservationMutation {
	return api.ReservationMutation{Resource: reservationResource(result.Record), Replayed: result.Replayed}
}

func reservationResource(record service.ReservationRecord) api.ReservationResource {
	return api.ReservationResource{
		Reservation:    api.FromDomainReservation(record.Reservation),
		StorageVersion: uint64(record.StorageVersion),
	}
}
