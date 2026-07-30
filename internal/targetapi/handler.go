// Package targetapi exposes the authenticated, tenant-scoped target HTTP
// boundary without choosing an identity provider or listener configuration.
package targetapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"

	api "github.com/zrma/sema/internal/api/v0alpha2"
	"github.com/zrma/sema/internal/domain"
	"github.com/zrma/sema/internal/repository"
	"github.com/zrma/sema/internal/service"
)

const (
	maxRequestBytes = 1 << 20
	defaultPageSize = 50
	maxPageSize     = 200

	// StableAPIVersion is the canonical stable service-wire marker.
	StableAPIVersion = "v1"
)

var identifierPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$`)

type Options struct {
	Now            func() time.Time
	CursorKey      []byte
	ReservationTTL time.Duration
	Middleware     func(http.Handler) http.Handler
}

func New(
	owner repository.Repository,
	authenticator Authenticator,
	options Options,
) (http.Handler, error) {
	if authenticator == nil {
		return nil, fmt.Errorf("target API authenticator is required")
	}
	codec, err := newCursorCodec(options.CursorKey)
	if err != nil {
		return nil, err
	}
	tickets, err := service.NewMatchTickets(owner, options.Now)
	if err != nil {
		return nil, err
	}
	backfills, err := service.NewBackfillTickets(owner, options.Now)
	if err != nil {
		return nil, err
	}
	policies, err := service.NewPolicies(owner, options.Now)
	if err != nil {
		return nil, err
	}
	planningRuns, err := service.NewPlanningRuns(owner, options.Now, nil)
	if err != nil {
		return nil, err
	}
	reservations, err := service.NewReservations(owner, options.Now, options.ReservationTTL)
	if err != nil {
		return nil, err
	}
	assignments, err := service.NewAssignments(owner, options.Now)
	if err != nil {
		return nil, err
	}
	server := &server{
		authenticator: authenticator, tickets: tickets, backfills: backfills,
		policies: policies, planningRuns: planningRuns, reservations: reservations,
		assignments: assignments, cursors: codec,
	}
	mux := http.NewServeMux()
	handle := func(pattern string, handler http.HandlerFunc) {
		endpoint := http.Handler(server.authenticate(handler))
		if options.Middleware != nil {
			endpoint = options.Middleware(endpoint)
		}
		mux.Handle(pattern, endpoint)
	}
	registerRoutes := func(prefix string) {
		handle("GET "+prefix+"/match-tickets", server.listMatchTickets)
		handle("GET "+prefix+"/match-tickets/{ticket_id}", server.getMatchTicket)
		handle("PUT "+prefix+"/match-tickets/{ticket_id}", server.putMatchTicket)
		handle("DELETE "+prefix+"/match-tickets/{ticket_id}", server.deleteMatchTicket)
		handle("GET "+prefix+"/backfill-tickets", server.listBackfillTickets)
		handle("GET "+prefix+"/backfill-tickets/{ticket_id}", server.getBackfillTicket)
		handle("PUT "+prefix+"/backfill-tickets/{ticket_id}", server.putBackfillTicket)
		handle("DELETE "+prefix+"/backfill-tickets/{ticket_id}", server.deleteBackfillTicket)
		handle("GET "+prefix+"/policies", server.listPolicies)
		handle("GET "+prefix+"/policies/{version}", server.getPolicy)
		handle("PUT "+prefix+"/policies/{version}", server.putPolicy)
		handle("POST "+prefix+"/planning-runs/{run_id}", server.postPlanningRun)
		handle("GET "+prefix+"/planning-runs/{run_id}", server.getPlanningRun)
		handle("GET "+prefix+"/planning-runs/{run_id}/proposals", server.listPlanningRunProposals)
		handle("GET "+prefix+"/planning-runs/{run_id}/unmatched", server.listPlanningRunUnmatched)
		handle("POST "+prefix+"/reservations/{reservation_id}", server.postReservation)
		handle("GET "+prefix+"/reservations/{reservation_id}", server.getReservation)
		handle("GET "+prefix+"/reservations", server.listReservations)
		handle("POST "+prefix+"/reservations/{reservation_id}/cancel", server.cancelReservation)
		handle("POST "+prefix+"/reservations/{reservation_id}/confirm", server.confirmReservation)
		handle("GET "+prefix+"/assignments/{assignment_id}", server.getAssignment)
		handle("GET "+prefix+"/assignments", server.listAssignments)
		handle("POST "+prefix+"/assignments/{assignment_id}/acknowledgments", server.acknowledgeAssignment)
		handle(prefix+"/match-tickets", methodNotAllowed("GET"))
		handle(prefix+"/match-tickets/{ticket_id}", methodNotAllowed("DELETE, GET, PUT"))
		handle(prefix+"/backfill-tickets", methodNotAllowed("GET"))
		handle(prefix+"/backfill-tickets/{ticket_id}", methodNotAllowed("DELETE, GET, PUT"))
		handle(prefix+"/policies", methodNotAllowed("GET"))
		handle(prefix+"/policies/{version}", methodNotAllowed("GET, PUT"))
		handle(prefix+"/planning-runs/{run_id}", methodNotAllowed("GET, POST"))
		handle(prefix+"/planning-runs/{run_id}/proposals", methodNotAllowed("GET"))
		handle(prefix+"/planning-runs/{run_id}/unmatched", methodNotAllowed("GET"))
		handle(prefix+"/reservations", methodNotAllowed("GET"))
		handle(prefix+"/reservations/{reservation_id}", methodNotAllowed("GET, POST"))
		handle(prefix+"/reservations/{reservation_id}/cancel", methodNotAllowed("POST"))
		handle(prefix+"/reservations/{reservation_id}/confirm", methodNotAllowed("POST"))
		handle(prefix+"/assignments", methodNotAllowed("GET"))
		handle(prefix+"/assignments/{assignment_id}", methodNotAllowed("GET"))
		handle(prefix+"/assignments/{assignment_id}/acknowledgments", methodNotAllowed("POST"))
	}
	registerRoutes("/" + api.Version)
	registerRoutes("/" + StableAPIVersion)
	handle("/", func(writer http.ResponseWriter, _ *http.Request) {
		writeError(writer, apiError{status: http.StatusNotFound, code: "NotFound", message: "endpoint was not found"})
	})
	return withResponseAPIVersion(recoverPanics(mux)), nil
}

type server struct {
	authenticator Authenticator
	tickets       *service.MatchTickets
	backfills     *service.BackfillTickets
	policies      *service.Policies
	planningRuns  *service.PlanningRuns
	reservations  *service.Reservations
	assignments   *service.Assignments
	cursors       cursorCodec
}

type principalContextKey struct{}

func (server *server) authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		principal, err := server.authenticator.Authenticate(request)
		if err != nil {
			if errors.Is(err, ErrUnauthenticated) {
				writeError(writer, apiError{
					status: http.StatusUnauthorized, code: "Unauthenticated",
					message: "valid credentials are required",
				})
				return
			}
			writeError(writer, apiError{
				status: http.StatusServiceUnavailable, code: "AuthenticationUnavailable",
				message: "authentication provider is unavailable", retryable: true,
			})
			return
		}
		if !validPrincipalValue(principal.Subject) || !validPrincipalValue(principal.Tenant) {
			writeError(writer, apiError{
				status: http.StatusUnauthorized, code: "Unauthenticated",
				message: "authenticated principal is incomplete",
			})
			return
		}
		ctx := context.WithValue(request.Context(), principalContextKey{}, principal)
		next.ServeHTTP(writer, request.WithContext(ctx))
	})
}

func (server *server) putMatchTicket(writer http.ResponseWriter, request *http.Request) {
	principal, ok := authorize(writer, request, PermissionMatchTicketsWrite)
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
	var ticket api.MatchTicket
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
	result, err := server.tickets.Put(
		request.Context(), principal.Tenant, domain.OperationID(operationID), api.ToDomainMatchTicket(ticket),
	)
	if err != nil {
		writeFailure(writer, err)
		return
	}
	writeData(writer, http.StatusOK, api.MatchTicketMutation{
		Resource: api.MatchTicketResource{
			Ticket: api.FromDomainMatchTicket(result.Ticket), StorageVersion: uint64(result.StorageVersion),
		},
		Replayed: result.Replayed,
	})
}

func (server *server) deleteMatchTicket(writer http.ResponseWriter, request *http.Request) {
	principal, ok := authorize(writer, request, PermissionMatchTicketsWrite)
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
	query, ok := parseQuery(writer, request, "revision")
	if !ok {
		return
	}
	revision, err := strconv.ParseUint(singleQuery(query, "revision"), 10, 64)
	if err != nil || revision == 0 {
		writeFailure(writer, domain.NewFailure(
			domain.FailureInvalidInput,
			"query parameter revision must be a positive integer",
		))
		return
	}
	result, err := server.tickets.Cancel(
		request.Context(), principal.Tenant, domain.OperationID(operationID),
		domain.TicketID(id), domain.Revision(revision),
	)
	if err != nil {
		writeFailure(writer, err)
		return
	}
	writeData(writer, http.StatusOK, api.MatchTicketCancellation{
		ID: string(result.ID), Revision: uint64(result.Revision),
		StorageVersion: uint64(result.StorageVersion), Replayed: result.Replayed,
	})
}

func (server *server) getMatchTicket(writer http.ResponseWriter, request *http.Request) {
	principal, ok := authorize(writer, request, PermissionMatchTicketsRead)
	if !ok || !validateNoQuery(writer, request) {
		return
	}
	id, ok := ticketID(writer, request)
	if !ok {
		return
	}
	record, exists, err := server.tickets.Get(request.Context(), principal.Tenant, domain.TicketID(id))
	if err != nil {
		writeFailure(writer, err)
		return
	}
	if !exists {
		writeError(writer, apiError{
			status: http.StatusNotFound, code: "NotFound", message: "match ticket was not found",
		})
		return
	}
	writeData(writer, http.StatusOK, matchTicketResource(record))
}

func (server *server) listMatchTickets(writer http.ResponseWriter, request *http.Request) {
	principal, ok := authorize(writer, request, PermissionMatchTicketsRead)
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
		Tenant: principal.Tenant, ResourceKind: string(service.ResourceMatchTicket),
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
	snapshot, err := server.tickets.Snapshot(request.Context(), principal.Tenant)
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
	items := make([]api.MatchTicketResource, 0, limit)
	hasMore := false
	for _, record := range snapshot.Records {
		if record.Deleted || string(record.Ticket.ID) <= position.After {
			continue
		}
		if len(items) == limit {
			hasMore = true
			break
		}
		items = append(items, matchTicketResource(record))
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
	writeData(writer, http.StatusOK, api.MatchTicketPage{
		Items: items, RepositoryVersion: uint64(snapshot.RepositoryVersion), NextCursor: next,
	})
}

func matchTicketResource(record service.MatchTicketRecord) api.MatchTicketResource {
	return api.MatchTicketResource{
		Ticket: api.FromDomainMatchTicket(record.Ticket), StorageVersion: uint64(record.StorageVersion),
	}
}

func authorize(
	writer http.ResponseWriter,
	request *http.Request,
	permission Permission,
) (Principal, bool) {
	principal, ok := request.Context().Value(principalContextKey{}).(Principal)
	if !ok {
		writeError(writer, apiError{
			status: http.StatusUnauthorized, code: "Unauthenticated", message: "valid credentials are required",
		})
		return Principal{}, false
	}
	if !principal.Allows(permission) {
		writeError(writer, apiError{
			status: http.StatusForbidden, code: "PermissionDenied", message: "permission is denied",
		})
		return Principal{}, false
	}
	return principal, true
}

func ticketID(writer http.ResponseWriter, request *http.Request) (string, bool) {
	id := request.PathValue("ticket_id")
	if !validIdentifier(id) {
		writeFailure(writer, domain.NewFailure(domain.FailureInvalidInput, "ticket ID is invalid"))
		return "", false
	}
	return id, true
}

func idempotencyKey(writer http.ResponseWriter, request *http.Request) (string, bool) {
	values := request.Header.Values("Idempotency-Key")
	if len(values) != 1 || !validIdentifier(values[0]) {
		writeFailure(writer, domain.NewFailure(
			domain.FailureInvalidInput,
			"exactly one valid Idempotency-Key header is required",
		))
		return "", false
	}
	return values[0], true
}

func validIdentifier(value string) bool {
	return identifierPattern.MatchString(value)
}

func validPrincipalValue(value string) bool {
	if value == "" || len(value) > 256 || strings.TrimSpace(value) != value {
		return false
	}
	for _, character := range value {
		if unicode.IsControl(character) {
			return false
		}
	}
	return true
}

func decodeRequest(writer http.ResponseWriter, request *http.Request, target any) bool {
	mediaType, _, err := mime.ParseMediaType(request.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" {
		writeError(writer, apiError{
			status: http.StatusUnsupportedMediaType, code: "UnsupportedMediaType",
			message: "Content-Type must be application/json",
		})
		return false
	}
	request.Body = http.MaxBytesReader(writer, request.Body, maxRequestBytes)
	decoder := json.NewDecoder(request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		writeFailure(writer, domain.NewFailure(domain.FailureInvalidInput, "decode JSON request: %v", err))
		return false
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		writeFailure(writer, domain.NewFailure(
			domain.FailureInvalidInput,
			"request must contain exactly one JSON value",
		))
		return false
	}
	return true
}

func validateNoQuery(writer http.ResponseWriter, request *http.Request) bool {
	_, ok := parseQuery(writer, request)
	return ok
}

func parseQuery(writer http.ResponseWriter, request *http.Request, allowed ...string) (url.Values, bool) {
	query, err := url.ParseQuery(request.URL.RawQuery)
	if err != nil {
		writeFailure(writer, domain.NewFailure(domain.FailureInvalidInput, "query string is invalid"))
		return nil, false
	}
	allowedSet := make(map[string]bool, len(allowed))
	for _, name := range allowed {
		allowedSet[name] = true
	}
	for name, values := range query {
		if !allowedSet[name] || len(values) != 1 {
			writeFailure(writer, domain.NewFailure(
				domain.FailureInvalidInput,
				"query parameter %s is not allowed or is repeated",
				name,
			))
			return nil, false
		}
	}
	return query, true
}

func singleQuery(query url.Values, name string) string {
	values := query[name]
	if len(values) != 1 {
		return ""
	}
	return values[0]
}

type apiError struct {
	status    int
	code      string
	message   string
	retryable bool
}

func methodNotAllowed(allowed string) http.HandlerFunc {
	return func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Allow", allowed)
		writeError(writer, apiError{
			status: http.StatusMethodNotAllowed, code: "MethodNotAllowed",
			message: "HTTP method is not allowed for this endpoint",
		})
	}
}

func writeFailure(writer http.ResponseWriter, err error) {
	if errors.Is(err, service.ErrResourceNotFound) {
		writeError(writer, apiError{
			status: http.StatusNotFound, code: "NotFound", message: "resource was not found",
		})
		return
	}
	mapped := apiError{
		status: http.StatusServiceUnavailable, code: "Unavailable",
		message: "target repository is unavailable", retryable: true,
	}
	var failure *domain.Failure
	if errors.As(err, &failure) {
		mapped.code = string(failure.Code)
		mapped.message = failure.Detail
		switch failure.Code {
		case domain.FailureInvalidInput:
			mapped.status = http.StatusBadRequest
			mapped.retryable = false
		case domain.FailureInvalidRevision, domain.FailureIdempotencyConflict,
			domain.FailureInvalidTransition, domain.FailurePolicyConflict:
			mapped.status = http.StatusConflict
			mapped.retryable = false
		case domain.FailureStaleSnapshot, domain.FailureReservationConflict:
			mapped.status = http.StatusConflict
			mapped.retryable = true
		case domain.FailureReservationExpired:
			mapped.status = http.StatusGone
			mapped.retryable = true
		}
	}
	writeError(writer, mapped)
}

type apiVersionResponseWriter struct {
	http.ResponseWriter
	apiVersion string
}

func (writer *apiVersionResponseWriter) SemaAPIVersion() string {
	return writer.apiVersion
}

func (writer *apiVersionResponseWriter) Unwrap() http.ResponseWriter {
	return writer.ResponseWriter
}

// ResponseAPIVersion returns the API marker bound to the current response.
// Middleware wrappers that implement Unwrap are traversed without exposing
// request paths, credentials, or resource identities.
func ResponseAPIVersion(writer http.ResponseWriter) string {
	for writer != nil {
		if versioned, ok := writer.(interface{ SemaAPIVersion() string }); ok {
			if version := versioned.SemaAPIVersion(); version != "" {
				return version
			}
		}
		unwrapper, ok := writer.(interface{ Unwrap() http.ResponseWriter })
		if !ok {
			break
		}
		next := unwrapper.Unwrap()
		if next == writer {
			break
		}
		writer = next
	}
	return api.Version
}

func withResponseAPIVersion(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		version := api.Version
		if request.URL.Path == "/"+StableAPIVersion ||
			strings.HasPrefix(request.URL.Path, "/"+StableAPIVersion+"/") {
			version = StableAPIVersion
		}
		next.ServeHTTP(&apiVersionResponseWriter{
			ResponseWriter: writer,
			apiVersion:     version,
		}, request)
	})
}

func writeData(writer http.ResponseWriter, status int, data any) {
	writeEnvelope(writer, status, api.Envelope{APIVersion: api.Version, Data: data})
}

func writeError(writer http.ResponseWriter, failure apiError) {
	writeEnvelope(writer, failure.status, api.Envelope{
		APIVersion: api.Version,
		Error: &api.Failure{
			Code: failure.code, Message: failure.message, Retryable: failure.retryable,
		},
	})
}

func writeEnvelope(writer http.ResponseWriter, status int, envelope api.Envelope) {
	envelope.APIVersion = ResponseAPIVersion(writer)
	if envelope.Error != nil {
		writer.Header().Set("X-Sema-Error-Code", envelope.Error.Code)
	}
	writer.Header().Set("Content-Type", "application/json")
	writer.Header().Set("Cache-Control", "no-store")
	writer.Header().Set("X-Content-Type-Options", "nosniff")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(envelope)
}

func recoverPanics(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		defer func() {
			if recovered := recover(); recovered != nil {
				writeError(writer, apiError{
					status: http.StatusInternalServerError, code: "Internal",
					message: "internal server error", retryable: true,
				})
			}
		}()
		next.ServeHTTP(writer, request)
	})
}
