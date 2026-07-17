// Package httpapi adapts the durable runtime to the experimental HTTP API.
package httpapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	api "github.com/zrma/sema/internal/api/v0alpha1"
	"github.com/zrma/sema/internal/domain"
	"github.com/zrma/sema/internal/durable"
	"github.com/zrma/sema/internal/observability"
)

const maxRequestBytes = 1 << 20

type Runtime interface {
	RegisterPolicy(domain.MatchmakingPolicy) (domain.PolicyFingerprint, error)
	Policy(string) (domain.MatchmakingPolicy, domain.PolicyFingerprint, bool, error)
	SubmitMatchTicket(domain.MatchTicket) error
	SubmitBackfillTicket(domain.BackfillTicket) error
	CancelMatchTicket(domain.TicketID, domain.Revision) error
	CancelBackfillTicket(domain.TicketID, domain.Revision, domain.Revision) error
	Plan(domain.SnapshotID, time.Time, string) (domain.ProposalBatch, error)
	Proposal(domain.ProposalID) (domain.MatchProposal, bool, error)
	Reserve(domain.MatchProposal, domain.ReservationID, time.Time) (domain.Reservation, error)
	Confirm(domain.ReservationID, domain.AssignmentID, time.Time) (domain.Assignment, error)
	CancelReservation(domain.ReservationID, time.Time) (domain.Reservation, error)
	AcknowledgeAssignment(domain.AssignmentID, domain.AssignmentAcknowledgmentRequest, time.Time) (domain.Assignment, error)
	Assignment(domain.AssignmentID) (domain.Assignment, bool, error)
	Ready() error
	AuditSummaries(uint64, int) ([]durable.AuditSummary, error)
}

func New(runtime Runtime) http.Handler {
	return NewWithOptions(runtime, Options{})
}

// NewWithClock constructs the handler with an explicit authoritative clock.
func NewWithClock(runtime Runtime, now func() time.Time) http.Handler {
	return NewWithOptions(runtime, Options{Now: now})
}

type Options struct {
	Now      func() time.Time
	Observer *observability.Recorder
}

func NewWithOptions(runtime Runtime, options Options) http.Handler {
	if options.Now == nil {
		options.Now = time.Now
	}
	if options.Observer == nil {
		options.Observer = observability.New(nil, options.Now)
	}
	server := &server{runtime: runtime, now: options.Now}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /livez", server.live)
	mux.HandleFunc("GET /readyz", server.ready)
	mux.Handle("GET /metrics", options.Observer)
	mux.HandleFunc("GET /v0alpha1/audit", server.getAudit)
	mux.HandleFunc("PUT /v0alpha1/policies/{version}", server.putPolicy)
	mux.HandleFunc("GET /v0alpha1/policies/{version}", server.getPolicy)
	mux.HandleFunc("PUT /v0alpha1/match-tickets/{ticket_id}", server.putMatchTicket)
	mux.HandleFunc("DELETE /v0alpha1/match-tickets/{ticket_id}", server.deleteMatchTicket)
	mux.HandleFunc("PUT /v0alpha1/backfill-tickets/{ticket_id}", server.putBackfillTicket)
	mux.HandleFunc("DELETE /v0alpha1/backfill-tickets/{ticket_id}", server.deleteBackfillTicket)
	mux.HandleFunc("POST /v0alpha1/plans", server.postPlan)
	mux.HandleFunc("POST /v0alpha1/reservations/{reservation_id}", server.postReservation)
	mux.HandleFunc("POST /v0alpha1/reservations/{reservation_id}/confirm", server.confirmReservation)
	mux.HandleFunc("POST /v0alpha1/reservations/{reservation_id}/cancel", server.cancelReservation)
	mux.HandleFunc("GET /v0alpha1/assignments/{assignment_id}", server.getAssignment)
	mux.HandleFunc("POST /v0alpha1/assignments/{assignment_id}/acknowledgments", server.acknowledgeAssignment)
	mux.HandleFunc("/v0alpha1/policies/{version}", methodNotAllowed("GET, PUT"))
	mux.HandleFunc("/v0alpha1/match-tickets/{ticket_id}", methodNotAllowed("DELETE, PUT"))
	mux.HandleFunc("/v0alpha1/backfill-tickets/{ticket_id}", methodNotAllowed("DELETE, PUT"))
	mux.HandleFunc("/v0alpha1/plans", methodNotAllowed("POST"))
	mux.HandleFunc("/v0alpha1/reservations/{reservation_id}", methodNotAllowed("POST"))
	mux.HandleFunc("/v0alpha1/reservations/{reservation_id}/confirm", methodNotAllowed("POST"))
	mux.HandleFunc("/v0alpha1/reservations/{reservation_id}/cancel", methodNotAllowed("POST"))
	mux.HandleFunc("/v0alpha1/assignments/{assignment_id}", methodNotAllowed("GET"))
	mux.HandleFunc("/v0alpha1/assignments/{assignment_id}/acknowledgments", methodNotAllowed("POST"))
	mux.HandleFunc("/livez", methodNotAllowed("GET"))
	mux.HandleFunc("/readyz", methodNotAllowed("GET"))
	mux.HandleFunc("/metrics", methodNotAllowed("GET"))
	mux.HandleFunc("/v0alpha1/audit", methodNotAllowed("GET"))
	mux.HandleFunc("/", func(writer http.ResponseWriter, _ *http.Request) {
		writeNotFound(writer, "endpoint")
	})
	return options.Observer.Middleware(recoverPanics(mux))
}

type server struct {
	runtime Runtime
	now     func() time.Time
}

func (server *server) live(writer http.ResponseWriter, _ *http.Request) {
	writeData(writer, http.StatusOK, api.MutationResult{Status: "ok"})
}

func (server *server) ready(writer http.ResponseWriter, _ *http.Request) {
	if err := server.runtime.Ready(); err != nil {
		writeFailure(writer, err)
		return
	}
	writeData(writer, http.StatusOK, api.MutationResult{Status: "ready"})
}

func (server *server) getAudit(writer http.ResponseWriter, request *http.Request) {
	after, ok := optionalUint(writer, request, "after", 0, 0, ^uint64(0))
	if !ok {
		return
	}
	limit, ok := optionalUint(writer, request, "limit", 100, 1, 1000)
	if !ok {
		return
	}
	summaries, err := server.runtime.AuditSummaries(after, int(limit))
	if err != nil {
		writeFailure(writer, err)
		return
	}
	records := make([]api.AuditSummary, len(summaries))
	next := after
	for index, summary := range summaries {
		records[index] = api.AuditSummary{
			Sequence: summary.Sequence, Kind: summary.Kind, Checksum: summary.Checksum,
			Counts: summary.Counts, Flags: summary.Flags, Outcome: summary.Outcome,
		}
		next = summary.Sequence
	}
	writeData(writer, http.StatusOK, api.AuditPage{Records: records, NextSequence: next})
}

func (server *server) putPolicy(writer http.ResponseWriter, request *http.Request) {
	var policy api.MatchmakingPolicy
	if !decodeRequest(writer, request, &policy) {
		return
	}
	if request.PathValue("version") != policy.Version {
		writeFailure(writer, domain.NewFailure(domain.FailureInvalidInput, "path version and policy version differ"))
		return
	}
	domainPolicy, err := api.ToDomainPolicy(policy)
	if err != nil {
		writeFailure(writer, domain.NewFailure(domain.FailureInvalidInput, "%v", err))
		return
	}
	fingerprint, err := server.runtime.RegisterPolicy(domainPolicy)
	if err != nil {
		writeFailure(writer, err)
		return
	}
	writeData(writer, http.StatusOK, api.PolicyRegistration{Policy: policy, Fingerprint: string(fingerprint)})
}

func (server *server) getPolicy(writer http.ResponseWriter, request *http.Request) {
	policy, fingerprint, exists, err := server.runtime.Policy(request.PathValue("version"))
	if err != nil {
		writeFailure(writer, err)
		return
	}
	if !exists {
		writeNotFound(writer, "policy")
		return
	}
	dto, err := fromDomainPolicy(policy)
	if err != nil {
		writeFailure(writer, err)
		return
	}
	writeData(writer, http.StatusOK, api.PolicyRegistration{Policy: dto, Fingerprint: string(fingerprint)})
}

func (server *server) putMatchTicket(writer http.ResponseWriter, request *http.Request) {
	var ticket api.MatchTicket
	if !decodeRequest(writer, request, &ticket) {
		return
	}
	if request.PathValue("ticket_id") != ticket.ID {
		writeFailure(writer, domain.NewFailure(domain.FailureInvalidInput, "path ticket ID and body ticket ID differ"))
		return
	}
	if err := server.runtime.SubmitMatchTicket(api.ToDomainMatchTicket(ticket)); err != nil {
		writeFailure(writer, err)
		return
	}
	writeAccepted(writer)
}

func (server *server) deleteMatchTicket(writer http.ResponseWriter, request *http.Request) {
	revision, ok := queryRevision(writer, request, "revision")
	if !ok {
		return
	}
	if err := server.runtime.CancelMatchTicket(domain.TicketID(request.PathValue("ticket_id")), revision); err != nil {
		writeFailure(writer, err)
		return
	}
	writeAccepted(writer)
}

func (server *server) putBackfillTicket(writer http.ResponseWriter, request *http.Request) {
	var ticket api.BackfillTicket
	if !decodeRequest(writer, request, &ticket) {
		return
	}
	if request.PathValue("ticket_id") != ticket.ID {
		writeFailure(writer, domain.NewFailure(domain.FailureInvalidInput, "path ticket ID and body ticket ID differ"))
		return
	}
	if err := server.runtime.SubmitBackfillTicket(api.ToDomainBackfillTicket(ticket)); err != nil {
		writeFailure(writer, err)
		return
	}
	writeAccepted(writer)
}

func (server *server) deleteBackfillTicket(writer http.ResponseWriter, request *http.Request) {
	revision, ok := queryRevision(writer, request, "revision")
	if !ok {
		return
	}
	rosterVersion, ok := queryRevision(writer, request, "roster_version")
	if !ok {
		return
	}
	err := server.runtime.CancelBackfillTicket(
		domain.TicketID(request.PathValue("ticket_id")),
		revision,
		rosterVersion,
	)
	if err != nil {
		writeFailure(writer, err)
		return
	}
	writeAccepted(writer)
}

func (server *server) postPlan(writer http.ResponseWriter, request *http.Request) {
	var plan api.PlanRequest
	if !decodeRequest(writer, request, &plan) {
		return
	}
	batch, err := server.runtime.Plan(domain.SnapshotID(plan.SnapshotID), server.now().UTC(), plan.PolicyVersion)
	if err != nil {
		writeFailure(writer, err)
		return
	}
	writeData(writer, http.StatusOK, api.FromDomainProposalBatch(batch))
}

func (server *server) postReservation(writer http.ResponseWriter, request *http.Request) {
	var reserve api.ReserveRequest
	if !decodeRequest(writer, request, &reserve) {
		return
	}
	proposal, exists, err := server.runtime.Proposal(domain.ProposalID(reserve.ProposalID))
	if err != nil {
		writeFailure(writer, err)
		return
	}
	if !exists {
		writeNotFound(writer, "proposal")
		return
	}
	reservation, err := server.runtime.Reserve(
		proposal,
		domain.ReservationID(request.PathValue("reservation_id")),
		server.now().UTC(),
	)
	if err != nil {
		writeFailure(writer, err)
		return
	}
	writeData(writer, http.StatusOK, api.FromDomainReservation(reservation))
}

func (server *server) confirmReservation(writer http.ResponseWriter, request *http.Request) {
	var confirm api.ConfirmRequest
	if !decodeRequest(writer, request, &confirm) {
		return
	}
	assignment, err := server.runtime.Confirm(
		domain.ReservationID(request.PathValue("reservation_id")),
		domain.AssignmentID(confirm.AssignmentID),
		server.now().UTC(),
	)
	if err != nil {
		writeFailure(writer, err)
		return
	}
	writeData(writer, http.StatusOK, api.FromDomainAssignment(assignment))
}

func (server *server) cancelReservation(writer http.ResponseWriter, request *http.Request) {
	var cancel api.CancelReservationRequest
	if !decodeRequest(writer, request, &cancel) {
		return
	}
	reservation, err := server.runtime.CancelReservation(
		domain.ReservationID(request.PathValue("reservation_id")),
		server.now().UTC(),
	)
	if err != nil {
		writeFailure(writer, err)
		return
	}
	writeData(writer, http.StatusOK, api.FromDomainReservation(reservation))
}

func (server *server) getAssignment(writer http.ResponseWriter, request *http.Request) {
	assignment, exists, err := server.runtime.Assignment(domain.AssignmentID(request.PathValue("assignment_id")))
	if err != nil {
		writeFailure(writer, err)
		return
	}
	if !exists {
		writeNotFound(writer, "assignment")
		return
	}
	writeData(writer, http.StatusOK, api.FromDomainAssignment(assignment))
}

func (server *server) acknowledgeAssignment(writer http.ResponseWriter, request *http.Request) {
	var acknowledgment api.AcknowledgeAssignmentRequest
	if !decodeRequest(writer, request, &acknowledgment) {
		return
	}
	assignment, err := server.runtime.AcknowledgeAssignment(
		domain.AssignmentID(request.PathValue("assignment_id")),
		api.ToDomainAcknowledgment(acknowledgment),
		server.now().UTC(),
	)
	if err != nil {
		writeFailure(writer, err)
		return
	}
	writeData(writer, http.StatusOK, api.FromDomainAssignment(assignment))
}

func decodeRequest(writer http.ResponseWriter, request *http.Request, target any) bool {
	request.Body = http.MaxBytesReader(writer, request.Body, maxRequestBytes)
	decoder := json.NewDecoder(request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		writeFailure(writer, domain.NewFailure(domain.FailureInvalidInput, "decode JSON request: %v", err))
		return false
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		writeFailure(writer, domain.NewFailure(domain.FailureInvalidInput, "request must contain exactly one JSON value"))
		return false
	}
	return true
}

func queryRevision(writer http.ResponseWriter, request *http.Request, name string) (domain.Revision, bool) {
	value := request.URL.Query().Get(name)
	parsed, err := strconv.ParseUint(value, 10, 64)
	if err != nil || parsed == 0 {
		writeFailure(writer, domain.NewFailure(domain.FailureInvalidInput, "query parameter %s must be a positive integer", name))
		return 0, false
	}
	return domain.Revision(parsed), true
}

func optionalUint(
	writer http.ResponseWriter,
	request *http.Request,
	name string,
	defaultValue uint64,
	minimum uint64,
	maximum uint64,
) (uint64, bool) {
	value := request.URL.Query().Get(name)
	if value == "" {
		return defaultValue, true
	}
	parsed, err := strconv.ParseUint(value, 10, 64)
	if err != nil || parsed < minimum || parsed > maximum {
		writeFailure(writer, domain.NewFailure(
			domain.FailureInvalidInput,
			"query parameter %s must be an integer between %d and %d",
			name,
			minimum,
			maximum,
		))
		return 0, false
	}
	return parsed, true
}

func writeAccepted(writer http.ResponseWriter) {
	writeData(writer, http.StatusOK, api.MutationResult{Status: "accepted"})
}

func writeData(writer http.ResponseWriter, status int, data any) {
	writeEnvelope(writer, status, api.Envelope{APIVersion: api.Version, Data: data})
}

func writeNotFound(writer http.ResponseWriter, resource string) {
	writeEnvelope(writer, http.StatusNotFound, api.Envelope{
		APIVersion: api.Version,
		Error:      &api.Failure{Code: "NotFound", Message: resource + " was not found", Retryable: false},
	})
}

func methodNotAllowed(allowed string) http.HandlerFunc {
	return func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Allow", allowed)
		writeEnvelope(writer, http.StatusMethodNotAllowed, api.Envelope{
			APIVersion: api.Version,
			Error: &api.Failure{
				Code: "MethodNotAllowed", Message: "HTTP method is not allowed for this endpoint", Retryable: false,
			},
		})
	}
}

func writeFailure(writer http.ResponseWriter, err error) {
	status := http.StatusServiceUnavailable
	failure := &api.Failure{Code: "Unavailable", Message: "durable runtime is unavailable", Retryable: true}
	var domainFailure *domain.Failure
	if errors.As(err, &domainFailure) {
		failure.Code = string(domainFailure.Code)
		failure.Message = domainFailure.Detail
		switch domainFailure.Code {
		case domain.FailureInvalidInput:
			status = http.StatusBadRequest
			failure.Retryable = false
		case domain.FailureInvalidRevision, domain.FailureIdempotencyConflict,
			domain.FailureInvalidTransition, domain.FailurePolicyConflict:
			status = http.StatusConflict
			failure.Retryable = false
		case domain.FailureStaleSnapshot, domain.FailureReservationConflict:
			status = http.StatusConflict
			failure.Retryable = true
		case domain.FailureReservationExpired:
			status = http.StatusGone
			failure.Retryable = true
		}
	}
	writeEnvelope(writer, status, api.Envelope{APIVersion: api.Version, Error: failure})
}

func writeEnvelope(writer http.ResponseWriter, status int, envelope api.Envelope) {
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
				writeEnvelope(writer, http.StatusInternalServerError, api.Envelope{
					APIVersion: api.Version,
					Error:      &api.Failure{Code: "Internal", Message: "internal server error", Retryable: true},
				})
			}
		}()
		next.ServeHTTP(writer, request)
	})
}

func fromDomainPolicy(policy domain.MatchmakingPolicy) (api.MatchmakingPolicy, error) {
	requirements := make([]api.RoleRequirement, len(policy.RoleRequirements))
	for index, requirement := range policy.RoleRequirements {
		requirements[index] = api.RoleRequirement{
			Role: requirement.Role, MinPerTeam: requirement.MinPerTeam, Hard: requirement.Hard,
		}
	}
	steps := make([]api.RelaxationStep, len(policy.RelaxationSteps))
	for index, step := range policy.RelaxationSteps {
		if step.AfterWait%time.Millisecond != 0 {
			return api.MatchmakingPolicy{}, fmt.Errorf("policy relaxation wait is not representable in milliseconds")
		}
		steps[index] = api.RelaxationStep{
			AfterWaitMillis: step.AfterWait.Milliseconds(), MaxTeamSkillGap: step.MaxTeamSkillGap,
			MaxRolePenalty: step.MaxRolePenalty, PrioritizeWait: step.PrioritizeWait,
		}
	}
	return api.MatchmakingPolicy{
		Version: policy.Version, TeamCount: policy.TeamCount, TeamSize: policy.TeamSize,
		MaxLatencyMillis: policy.MaxLatencyMillis, MaxProposals: policy.MaxProposals,
		MaxSearchNodes: policy.MaxSearchNodes, MaxCandidateTickets: policy.MaxCandidateTickets,
		MaxCandidatesPerProposal: policy.MaxCandidatesPerProposal,
		MaxBatchCandidates:       policy.MaxBatchCandidates, MaxBatchSearchNodes: policy.MaxBatchSearchNodes,
		RoleRequirements: requirements, RelaxationSteps: steps,
	}, nil
}
