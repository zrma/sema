package targetapi

import (
	"net/http"
	"strconv"

	api "github.com/zrma/sema/internal/api/v0alpha2"
	"github.com/zrma/sema/internal/domain"
	"github.com/zrma/sema/internal/service"
)

func (server *server) putBackfillTicket(writer http.ResponseWriter, request *http.Request) {
	principal, ok := authorize(writer, request, PermissionBackfillTicketsWrite)
	if !ok || !validateNoQuery(writer, request) {
		return
	}
	id, ok := ticketID(writer, request)
	if !ok {
		return
	}
	operationID, ok := idempotencyKey(writer, request)
	if !ok {
		return
	}
	var ticket api.BackfillTicket
	if !decodeRequest(writer, request, &ticket) {
		return
	}
	if ticket.ID != id {
		writeFailure(writer, domain.NewFailure(
			domain.FailureInvalidInput,
			"path ticket ID and body ticket ID differ",
		))
		return
	}
	result, err := server.backfills.Put(
		request.Context(), principal.Tenant, domain.OperationID(operationID), api.ToDomainBackfillTicket(ticket),
	)
	if err != nil {
		writeFailure(writer, err)
		return
	}
	writeData(writer, http.StatusOK, api.BackfillTicketMutation{
		Resource: api.BackfillTicketResource{
			Ticket:         api.FromDomainBackfillTicket(result.Ticket),
			StorageVersion: uint64(result.StorageVersion),
		},
		Replayed: result.Replayed,
	})
}

func (server *server) deleteBackfillTicket(writer http.ResponseWriter, request *http.Request) {
	principal, ok := authorize(writer, request, PermissionBackfillTicketsWrite)
	if !ok {
		return
	}
	id, ok := ticketID(writer, request)
	if !ok {
		return
	}
	operationID, ok := idempotencyKey(writer, request)
	if !ok {
		return
	}
	query, ok := parseQuery(writer, request, "revision", "roster_version")
	if !ok {
		return
	}
	revision, revisionOK := positiveRevisionQuery(writer, query, "revision")
	rosterVersion, rosterOK := positiveRevisionQuery(writer, query, "roster_version")
	if !revisionOK || !rosterOK {
		return
	}
	result, err := server.backfills.Cancel(
		request.Context(), principal.Tenant, domain.OperationID(operationID), domain.TicketID(id),
		domain.Revision(revision), domain.Revision(rosterVersion),
	)
	if err != nil {
		writeFailure(writer, err)
		return
	}
	writeData(writer, http.StatusOK, api.BackfillTicketCancellation{
		ID: string(result.ID), Revision: uint64(result.Revision),
		RosterVersion: uint64(result.RosterVersion), StorageVersion: uint64(result.StorageVersion),
		Replayed: result.Replayed,
	})
}

func (server *server) getBackfillTicket(writer http.ResponseWriter, request *http.Request) {
	principal, ok := authorize(writer, request, PermissionBackfillTicketsRead)
	if !ok || !validateNoQuery(writer, request) {
		return
	}
	id, ok := ticketID(writer, request)
	if !ok {
		return
	}
	record, exists, err := server.backfills.Get(request.Context(), principal.Tenant, domain.TicketID(id))
	if err != nil {
		writeFailure(writer, err)
		return
	}
	if !exists {
		writeError(writer, apiError{
			status: http.StatusNotFound, code: "NotFound", message: "backfill ticket was not found",
		})
		return
	}
	writeData(writer, http.StatusOK, backfillTicketResource(record))
}

func (server *server) listBackfillTickets(writer http.ResponseWriter, request *http.Request) {
	principal, ok := authorize(writer, request, PermissionBackfillTicketsRead)
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
		Tenant: principal.Tenant, ResourceKind: string(service.ResourceBackfillTicket),
		Filter: "active", Order: "resource_id.asc",
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
	snapshot, err := server.backfills.Snapshot(request.Context(), principal.Tenant)
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
	items := make([]api.BackfillTicketResource, 0, limit)
	hasMore := false
	for _, record := range snapshot.Records {
		if record.Deleted || string(record.Ticket.ID) <= position.After {
			continue
		}
		if len(items) == limit {
			hasMore = true
			break
		}
		items = append(items, backfillTicketResource(record))
	}
	next := ""
	if hasMore {
		next, err = server.cursors.encode(binding, cursorPosition{
			RepositoryVersion: snapshot.RepositoryVersion,
			After:             items[len(items)-1].Ticket.ID,
		})
		if err != nil {
			writeFailure(writer, err)
			return
		}
	}
	writeData(writer, http.StatusOK, api.BackfillTicketPage{
		Items: items, RepositoryVersion: uint64(snapshot.RepositoryVersion), NextCursor: next,
	})
}

func backfillTicketResource(record service.BackfillTicketRecord) api.BackfillTicketResource {
	return api.BackfillTicketResource{
		Ticket: api.FromDomainBackfillTicket(record.Ticket), StorageVersion: uint64(record.StorageVersion),
	}
}

func positiveRevisionQuery(writer http.ResponseWriter, query map[string][]string, name string) (uint64, bool) {
	value := singleQuery(query, name)
	parsed, err := strconv.ParseUint(value, 10, 64)
	if err != nil || parsed == 0 {
		writeFailure(writer, domain.NewFailure(
			domain.FailureInvalidInput,
			"query parameter %s must be a positive integer",
			name,
		))
		return 0, false
	}
	return parsed, true
}
