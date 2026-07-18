package service

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"time"

	"github.com/zrma/sema/internal/domain"
	"github.com/zrma/sema/internal/repository"
)

const (
	assignmentPayloadSchema     = "sema.assignment.v1"
	acknowledgmentPayloadSchema = "sema.assignment-acknowledgment.v1"
)

type AssignmentRecord struct {
	Assignment     domain.Assignment
	StorageVersion repository.Version
}

type AssignmentSnapshot struct {
	RepositoryVersion repository.Version
	Records           []AssignmentRecord
}

type AssignmentMutation struct {
	Record   AssignmentRecord
	Replayed bool
}

type Assignments struct {
	repository repository.Repository
	now        func() time.Time
}

type persistedAssignment struct {
	Schema         string                    `json:"schema"`
	ID             domain.AssignmentID       `json:"id"`
	ReservationID  domain.ReservationID      `json:"reservation_id"`
	ProposalID     domain.ProposalID         `json:"proposal_id"`
	Kind           domain.ProposalKind       `json:"kind"`
	Teams          []persistedTeamAssignment `json:"teams"`
	Backfill       *persistedBackfillTarget  `json:"backfill,omitempty"`
	ConfirmedAt    time.Time                 `json:"confirmed_at"`
	Status         domain.AssignmentStatus   `json:"status"`
	Acknowledgment *persistedAcknowledgment  `json:"acknowledgment,omitempty"`
}

type persistedAcknowledgment struct {
	OperationID            domain.OperationID      `json:"operation_id"`
	Outcome                domain.AssignmentStatus `json:"outcome"`
	SessionID              domain.SessionID        `json:"session_id,omitempty"`
	ExpectedRosterVersion  domain.Revision         `json:"expected_roster_version,omitempty"`
	ResultingRosterVersion domain.Revision         `json:"resulting_roster_version,omitempty"`
	FailureCode            domain.FailureCode      `json:"failure_code,omitempty"`
	Reason                 string                  `json:"reason,omitempty"`
	AcknowledgedAt         time.Time               `json:"acknowledged_at"`
}

type persistedAcknowledgmentResource struct {
	Schema         string                  `json:"schema"`
	AssignmentID   domain.AssignmentID     `json:"assignment_id"`
	Acknowledgment persistedAcknowledgment `json:"acknowledgment"`
}

type assignmentCommandResult struct {
	Assignment      persistedAssignment `json:"assignment"`
	ResourceVersion uint64              `json:"resource_version,omitempty"`
}

func NewAssignments(owner repository.Repository, now func() time.Time) (*Assignments, error) {
	if owner == nil {
		return nil, domain.NewFailure(domain.FailureInvalidInput, "repository is required")
	}
	if now == nil {
		now = time.Now
	}
	return &Assignments{repository: owner, now: now}, nil
}

func (service *Reservations) Confirm(
	ctx context.Context,
	scope string,
	operationID domain.OperationID,
	reservationID domain.ReservationID,
	assignmentID domain.AssignmentID,
) (AssignmentMutation, error) {
	command, err := json.Marshal(struct {
		Kind          string               `json:"kind"`
		ReservationID domain.ReservationID `json:"reservation_id"`
		AssignmentID  domain.AssignmentID  `json:"assignment_id"`
	}{Kind: "reservation.confirm", ReservationID: reservationID, AssignmentID: assignmentID})
	if err != nil {
		return AssignmentMutation{}, fmt.Errorf("encode reservation confirmation: %w", err)
	}
	operation, err := service.operation(scope, operationID, "reservation.confirm", command)
	if err != nil {
		return AssignmentMutation{}, err
	}
	if reservationID == "" || assignmentID == "" {
		return AssignmentMutation{}, domain.NewFailure(
			domain.FailureInvalidInput,
			"reservation and assignment identities are required",
		)
	}
	if replay, exists, err := service.repository.Replay(ctx, operation); err != nil {
		return AssignmentMutation{}, err
	} else if exists {
		return replayedAssignmentMutation(
			ctx, service.repository, scope, operationID, "reservation.confirm", replay,
		)
	}
	if err := service.expireDue(ctx, scope, operation.At); err != nil {
		return AssignmentMutation{}, err
	}

	snapshot, err := service.repository.Snapshot(ctx, scope)
	if err != nil {
		return AssignmentMutation{}, err
	}
	reservationResource, exists := findResource(snapshot, Key(scope, ResourceReservation, string(reservationID)))
	if !exists || reservationResource.Deleted {
		return AssignmentMutation{}, ErrResourceNotFound
	}
	reservation, err := decodeReservation(reservationResource.Payload)
	if err != nil {
		return AssignmentMutation{}, err
	}
	switch reservation.Status {
	case domain.ReservationExpired:
		return AssignmentMutation{}, domain.NewFailure(
			domain.FailureReservationExpired, "reservation %q has expired", reservationID,
		)
	case domain.ReservationActive:
	case domain.ReservationCancelled, domain.ReservationConfirmed:
		return AssignmentMutation{}, domain.NewFailure(
			domain.FailureInvalidTransition, "reservation %q is not active", reservationID,
		)
	default:
		return AssignmentMutation{}, fmt.Errorf("reservation %q has invalid status", reservationID)
	}
	if _, exists := findResource(snapshot, Key(scope, ResourceAssignment, string(assignmentID))); exists {
		return AssignmentMutation{}, domain.NewFailure(
			domain.FailureInvalidRevision, "assignment %q is already registered", assignmentID,
		)
	}
	proposalResource, exists := findResource(snapshot, Key(scope, ResourceProposal, string(reservation.ProposalID)))
	if !exists || proposalResource.Deleted {
		return AssignmentMutation{}, fmt.Errorf("reservation %q proposal is missing", reservationID)
	}
	proposalRecord, err := decodeProposal(proposalResource.Payload)
	if err != nil {
		return AssignmentMutation{}, err
	}
	proposal := proposalRecord.Proposal
	if !sameReservationProposal(reservation, proposal) {
		return AssignmentMutation{}, fmt.Errorf("reservation %q proposal reference is inconsistent", reservationID)
	}

	assignment := domain.Assignment{
		ID: assignmentID, ReservationID: reservationID, ProposalID: proposal.ID, Kind: proposal.Kind,
		Teams: domain.CloneTeams(proposal.Teams), Backfill: domain.CloneBackfillTarget(proposal.Backfill),
		ConfirmedAt: operation.At, Status: domain.AssignmentPending,
	}
	mutations, stale, err := confirmDemandMutations(snapshot, scope, reservation, proposal)
	if err != nil {
		return AssignmentMutation{}, err
	}
	if stale != nil {
		if err := service.invalidateStale(ctx, scope, reservation, operation.At); err != nil {
			if repository.IsConflict(err) {
				return AssignmentMutation{}, domain.NewFailure(
					domain.FailureReservationConflict,
					"reservation %q changed while stale demand was released",
					reservationID,
				)
			}
			return AssignmentMutation{}, err
		}
		return AssignmentMutation{}, stale
	}

	reservation.Status = domain.ReservationConfirmed
	reservationPayload, err := encodeReservation(reservation)
	if err != nil {
		return AssignmentMutation{}, err
	}
	assignmentPayload, err := encodeAssignment(assignment)
	if err != nil {
		return AssignmentMutation{}, err
	}
	resultPayload, err := encodeAssignmentCommandResult("reservation.confirm", assignment, 0)
	if err != nil {
		return AssignmentMutation{}, err
	}
	mutations = append(mutations,
		repository.Mutation{
			Key: reservationResource.Key, ExpectedVersion: reservationResource.Version, Payload: reservationPayload,
		},
		repository.Mutation{
			Key: Key(scope, ResourceAssignment, string(assignmentID)), Payload: assignmentPayload,
		},
		repository.Mutation{
			Key: Key(scope, ResourceOperationResult, string(operationID)), Payload: resultPayload,
		},
	)
	result, err := service.repository.Commit(ctx, operation, mutations)
	if err != nil {
		if replay, exists, replayErr := service.repository.Replay(ctx, operation); replayErr != nil {
			return AssignmentMutation{}, replayErr
		} else if exists {
			return replayedAssignmentMutation(
				ctx, service.repository, scope, operationID, "reservation.confirm", replay,
			)
		}
		if repository.IsConflict(err) {
			return AssignmentMutation{}, domain.NewFailure(
				domain.FailureReservationConflict,
				"reservation %q demand changed while confirmation was committed",
				reservationID,
			)
		}
		return AssignmentMutation{}, err
	}
	return AssignmentMutation{Record: AssignmentRecord{
		Assignment: cloneAssignment(assignment), StorageVersion: result.Version,
	}, Replayed: result.Replayed}, nil
}

func (service *Assignments) Acknowledge(
	ctx context.Context,
	scope string,
	assignmentID domain.AssignmentID,
	request domain.AssignmentAcknowledgmentRequest,
) (AssignmentMutation, error) {
	command, err := json.Marshal(struct {
		Kind                   string                  `json:"kind"`
		AssignmentID           domain.AssignmentID     `json:"assignment_id"`
		Outcome                domain.AssignmentStatus `json:"outcome"`
		SessionID              domain.SessionID        `json:"session_id,omitempty"`
		ExpectedRosterVersion  domain.Revision         `json:"expected_roster_version,omitempty"`
		ResultingRosterVersion domain.Revision         `json:"resulting_roster_version,omitempty"`
		FailureCode            domain.FailureCode      `json:"failure_code,omitempty"`
		Reason                 string                  `json:"reason,omitempty"`
	}{
		Kind: "assignment.acknowledge", AssignmentID: assignmentID, Outcome: request.Outcome,
		SessionID: request.SessionID, ExpectedRosterVersion: request.ExpectedRosterVersion,
		ResultingRosterVersion: request.ResultingRosterVersion,
		FailureCode:            request.FailureCode, Reason: request.Reason,
	})
	if err != nil {
		return AssignmentMutation{}, fmt.Errorf("encode assignment acknowledgment: %w", err)
	}
	operation := repository.Operation{
		Scope: scope, ID: request.OperationID, Kind: "assignment.acknowledge",
		Digest: repository.Digest(command), At: service.now().UTC(),
	}
	if assignmentID == "" {
		return AssignmentMutation{}, domain.NewFailure(domain.FailureInvalidInput, "assignment identity is required")
	}
	if err := repository.ValidateOperation(operation); err != nil {
		return AssignmentMutation{}, err
	}
	if replay, exists, err := service.repository.Replay(ctx, operation); err != nil {
		return AssignmentMutation{}, err
	} else if exists {
		return replayedAssignmentMutation(
			ctx, service.repository, scope, request.OperationID, "assignment.acknowledge", replay,
		)
	}

	snapshot, err := service.repository.Snapshot(ctx, scope)
	if err != nil {
		return AssignmentMutation{}, err
	}
	resource, exists := findResource(snapshot, Key(scope, ResourceAssignment, string(assignmentID)))
	if !exists || resource.Deleted {
		return AssignmentMutation{}, ErrResourceNotFound
	}
	assignment, err := decodeAssignment(resource.Payload)
	if err != nil {
		return AssignmentMutation{}, err
	}
	if assignment.Status != domain.AssignmentPending {
		return AssignmentMutation{}, domain.NewFailure(
			domain.FailureInvalidTransition, "assignment %q is already terminal", assignmentID,
		)
	}
	if err := domain.ValidateAssignmentAcknowledgment(assignment, request); err != nil {
		return AssignmentMutation{}, err
	}
	assignment.Status = request.Outcome
	assignment.Acknowledgment = &domain.AssignmentAcknowledgment{
		AssignmentAcknowledgmentRequest: request,
		AcknowledgedAt:                  operation.At,
	}
	payload, err := encodeAssignment(assignment)
	if err != nil {
		return AssignmentMutation{}, err
	}
	acknowledgmentPayload, err := encodeAcknowledgmentResource(assignment)
	if err != nil {
		return AssignmentMutation{}, err
	}
	resultPayload, err := encodeAssignmentCommandResult("assignment.acknowledge", assignment, 0)
	if err != nil {
		return AssignmentMutation{}, err
	}
	result, err := service.repository.Commit(ctx, operation, []repository.Mutation{
		{Key: resource.Key, ExpectedVersion: resource.Version, Payload: payload},
		{Key: Key(scope, ResourceAcknowledgment, string(assignmentID)), Payload: acknowledgmentPayload},
		{Key: Key(scope, ResourceOperationResult, string(request.OperationID)), Payload: resultPayload},
	})
	if err != nil {
		if replay, exists, replayErr := service.repository.Replay(ctx, operation); replayErr != nil {
			return AssignmentMutation{}, replayErr
		} else if exists {
			return replayedAssignmentMutation(
				ctx, service.repository, scope, request.OperationID, "assignment.acknowledge", replay,
			)
		}
		if repository.IsConflict(err) {
			return AssignmentMutation{}, domain.NewFailure(
				domain.FailureInvalidTransition,
				"assignment %q reached a terminal state concurrently",
				assignmentID,
			)
		}
		return AssignmentMutation{}, err
	}
	return AssignmentMutation{Record: AssignmentRecord{
		Assignment: cloneAssignment(assignment), StorageVersion: result.Version,
	}, Replayed: result.Replayed}, nil
}

func (service *Assignments) Get(
	ctx context.Context,
	scope string,
	assignmentID domain.AssignmentID,
) (AssignmentRecord, bool, error) {
	if assignmentID == "" {
		return AssignmentRecord{}, false, domain.NewFailure(domain.FailureInvalidInput, "assignment identity is required")
	}
	snapshot, err := service.repository.Snapshot(ctx, scope)
	if err != nil {
		return AssignmentRecord{}, false, err
	}
	resource, exists := findResource(snapshot, Key(scope, ResourceAssignment, string(assignmentID)))
	if !exists || resource.Deleted {
		return AssignmentRecord{}, false, nil
	}
	assignment, err := decodeAssignment(resource.Payload)
	if err != nil {
		return AssignmentRecord{}, false, err
	}
	return AssignmentRecord{Assignment: assignment, StorageVersion: resource.Version}, true, nil
}

func (service *Assignments) Snapshot(ctx context.Context, scope string) (AssignmentSnapshot, error) {
	snapshot, err := service.repository.Snapshot(ctx, scope)
	if err != nil {
		return AssignmentSnapshot{}, err
	}
	records := make([]AssignmentRecord, 0)
	for _, resource := range snapshot.Resources {
		if resource.Key.Kind != string(ResourceAssignment) || resource.Deleted {
			continue
		}
		assignment, err := decodeAssignment(resource.Payload)
		if err != nil {
			return AssignmentSnapshot{}, err
		}
		records = append(records, AssignmentRecord{Assignment: assignment, StorageVersion: resource.Version})
	}
	slices.SortFunc(records, func(left, right AssignmentRecord) int {
		if left.Assignment.ID < right.Assignment.ID {
			return -1
		}
		if left.Assignment.ID > right.Assignment.ID {
			return 1
		}
		return 0
	})
	return AssignmentSnapshot{RepositoryVersion: snapshot.Version, Records: records}, nil
}

func confirmDemandMutations(
	snapshot repository.Snapshot,
	scope string,
	reservation domain.Reservation,
	proposal domain.MatchProposal,
) ([]repository.Mutation, error, error) {
	mutations := make([]repository.Mutation, 0, len(proposal.Tickets)+4)
	for _, reference := range proposal.Tickets {
		resource, exists := findResource(snapshot, Key(scope, ResourceMatchTicket, string(reference.ID)))
		if !exists || resource.Deleted {
			return nil, domain.NewFailure(
				domain.FailureStaleSnapshot, "match ticket %q is no longer active", reference.ID,
			), nil
		}
		ticket, err := decodeMatchTicket(resource.Payload)
		if err != nil {
			return nil, nil, err
		}
		if ticket.Revision != reference.Revision {
			return nil, domain.NewFailure(
				domain.FailureStaleSnapshot, "match ticket %q revision changed", reference.ID,
			), nil
		}
		mutations = append(mutations, repository.Mutation{
			Key: resource.Key, ExpectedVersion: resource.Version, Delete: true,
		})
	}
	if proposal.Backfill != nil {
		target := proposal.Backfill
		resource, exists := findResource(snapshot, Key(scope, ResourceBackfillTicket, string(target.Ticket.ID)))
		if !exists || resource.Deleted {
			return nil, domain.NewFailure(
				domain.FailureStaleSnapshot, "backfill ticket %q is no longer active", target.Ticket.ID,
			), nil
		}
		ticket, err := decodeBackfillTicket(resource.Payload)
		if err != nil {
			return nil, nil, err
		}
		if ticket.Revision != target.Ticket.Revision || ticket.SessionID != target.SessionID ||
			ticket.RosterVersion != target.RosterVersion {
			return nil, domain.NewFailure(
				domain.FailureStaleSnapshot, "backfill ticket %q freshness changed", target.Ticket.ID,
			), nil
		}
		sessionKey := Key(scope, ResourceBackfillSessionClaim, string(target.SessionID))
		sessionResource, exists := findResource(snapshot, sessionKey)
		if !exists || sessionResource.Deleted {
			return nil, nil, fmt.Errorf("backfill session claim %q is missing", target.SessionID)
		}
		sessionClaim, err := decodeBackfillSessionClaim(sessionResource.Payload)
		if err != nil {
			return nil, nil, err
		}
		if sessionClaim.TicketID != target.Ticket.ID {
			return nil, nil, fmt.Errorf("backfill session claim %q belongs to another ticket", target.SessionID)
		}
		mutations = append(mutations,
			repository.Mutation{Key: resource.Key, ExpectedVersion: resource.Version, Delete: true},
			repository.Mutation{Key: sessionKey, ExpectedVersion: sessionResource.Version, Delete: true},
		)
	}
	claimMutations, err := reservationClaimDeletions(snapshot, scope, reservation)
	if err != nil {
		return nil, nil, err
	}
	mutations = append(mutations, claimMutations...)
	return mutations, nil, nil
}

func sameReservationProposal(reservation domain.Reservation, proposal domain.MatchProposal) bool {
	return reservation.ProposalID == proposal.ID && slices.Equal(reservation.Tickets, proposal.Tickets) &&
		sameBackfillTarget(reservation.Backfill, proposal.Backfill)
}

func sameBackfillTarget(left, right *domain.BackfillTarget) bool {
	if left == nil || right == nil {
		return left == right
	}
	return *left == *right
}

func replayedAssignmentMutation(
	ctx context.Context,
	owner repository.Repository,
	scope string,
	operationID domain.OperationID,
	kind string,
	replay repository.CommitResult,
) (AssignmentMutation, error) {
	snapshot, err := owner.Snapshot(ctx, scope)
	if err != nil {
		return AssignmentMutation{}, err
	}
	resource, exists := findResource(snapshot, Key(scope, ResourceOperationResult, string(operationID)))
	if !exists || resource.Deleted {
		return AssignmentMutation{}, fmt.Errorf("%s operation result is missing", kind)
	}
	var result assignmentCommandResult
	if err := decodeOperationResult(resource.Payload, kind, &result); err != nil {
		return AssignmentMutation{}, err
	}
	assignment, err := fromPersistedAssignment(result.Assignment)
	if err != nil {
		return AssignmentMutation{}, err
	}
	resourceVersion := repository.Version(result.ResourceVersion)
	if resourceVersion == 0 {
		resourceVersion = replay.Version
	}
	return AssignmentMutation{Record: AssignmentRecord{
		Assignment: assignment, StorageVersion: resourceVersion,
	}, Replayed: true}, nil
}

func encodeAssignment(assignment domain.Assignment) ([]byte, error) {
	stored, err := toPersistedAssignment(assignment)
	if err != nil {
		return nil, err
	}
	encoded, err := json.Marshal(stored)
	if err != nil {
		return nil, fmt.Errorf("encode assignment resource: %w", err)
	}
	return encoded, nil
}

func decodeAssignment(payload []byte) (domain.Assignment, error) {
	var stored persistedAssignment
	if err := decodeStrict(payload, &stored); err != nil {
		return domain.Assignment{}, fmt.Errorf("decode assignment resource: %w", err)
	}
	return fromPersistedAssignment(stored)
}

func toPersistedAssignment(assignment domain.Assignment) (persistedAssignment, error) {
	if assignment.ID == "" || assignment.ReservationID == "" || assignment.ProposalID == "" ||
		(assignment.Kind != domain.ProposalNewMatch && assignment.Kind != domain.ProposalBackfill) ||
		len(assignment.Teams) == 0 || assignment.ConfirmedAt.IsZero() || !validAssignmentStatus(assignment.Status) {
		return persistedAssignment{}, domain.NewFailure(domain.FailureInvalidInput, "assignment is invalid")
	}
	if (assignment.Kind == domain.ProposalBackfill) != (assignment.Backfill != nil) {
		return persistedAssignment{}, domain.NewFailure(
			domain.FailureInvalidInput, "assignment kind and backfill target differ",
		)
	}
	teams := make([]persistedTeamAssignment, len(assignment.Teams))
	for index, team := range assignment.Teams {
		if team.Team != index {
			return persistedAssignment{}, domain.NewFailure(domain.FailureInvalidInput, "assignment team is invalid")
		}
		references := make([]persistedTicketRef, len(team.Tickets))
		for ticketIndex, reference := range team.Tickets {
			if reference.ID == "" || reference.Revision == 0 {
				return persistedAssignment{}, domain.NewFailure(domain.FailureInvalidInput, "assignment ticket reference is invalid")
			}
			references[ticketIndex] = toPersistedTicketRef(reference)
		}
		teams[index] = persistedTeamAssignment{Team: team.Team, Tickets: references}
	}
	var backfill *persistedBackfillTarget
	if assignment.Backfill != nil {
		backfill = &persistedBackfillTarget{
			Ticket:    toPersistedTicketRef(assignment.Backfill.Ticket),
			SessionID: assignment.Backfill.SessionID, RosterVersion: assignment.Backfill.RosterVersion,
		}
	}
	var acknowledgment *persistedAcknowledgment
	if assignment.Acknowledgment != nil {
		if assignment.Acknowledgment.AcknowledgedAt.IsZero() {
			return persistedAssignment{}, domain.NewFailure(
				domain.FailureInvalidInput, "assignment acknowledgment time is required",
			)
		}
		if err := domain.ValidateAssignmentAcknowledgment(
			assignment,
			assignment.Acknowledgment.AssignmentAcknowledgmentRequest,
		); err != nil {
			return persistedAssignment{}, err
		}
		stored := toPersistedAcknowledgment(*assignment.Acknowledgment)
		acknowledgment = &stored
	}
	if (assignment.Status == domain.AssignmentPending) != (acknowledgment == nil) {
		return persistedAssignment{}, domain.NewFailure(
			domain.FailureInvalidInput, "assignment status and acknowledgment differ",
		)
	}
	if acknowledgment != nil && acknowledgment.Outcome != assignment.Status {
		return persistedAssignment{}, domain.NewFailure(
			domain.FailureInvalidInput, "assignment acknowledgment outcome differs from status",
		)
	}
	return persistedAssignment{
		Schema: assignmentPayloadSchema, ID: assignment.ID, ReservationID: assignment.ReservationID,
		ProposalID: assignment.ProposalID, Kind: assignment.Kind, Teams: teams, Backfill: backfill,
		ConfirmedAt: assignment.ConfirmedAt.UTC(), Status: assignment.Status, Acknowledgment: acknowledgment,
	}, nil
}

func fromPersistedAssignment(stored persistedAssignment) (domain.Assignment, error) {
	if stored.Schema != assignmentPayloadSchema || stored.ID == "" || stored.ReservationID == "" ||
		stored.ProposalID == "" || (stored.Kind != domain.ProposalNewMatch && stored.Kind != domain.ProposalBackfill) ||
		len(stored.Teams) == 0 || stored.ConfirmedAt.IsZero() || !validAssignmentStatus(stored.Status) {
		return domain.Assignment{}, fmt.Errorf("assignment resource header is invalid")
	}
	if (stored.Kind == domain.ProposalBackfill) != (stored.Backfill != nil) {
		return domain.Assignment{}, fmt.Errorf("assignment kind and backfill target differ")
	}
	assignment := domain.Assignment{
		ID: stored.ID, ReservationID: stored.ReservationID, ProposalID: stored.ProposalID,
		Kind: stored.Kind, Teams: make([]domain.TeamAssignment, len(stored.Teams)),
		ConfirmedAt: stored.ConfirmedAt.UTC(), Status: stored.Status,
	}
	for index, team := range stored.Teams {
		if team.Team != index {
			return domain.Assignment{}, fmt.Errorf("assignment team is invalid")
		}
		references := make([]domain.TicketRef, len(team.Tickets))
		for ticketIndex, reference := range team.Tickets {
			if reference.ID == "" || reference.Revision == 0 {
				return domain.Assignment{}, fmt.Errorf("assignment ticket reference is invalid")
			}
			references[ticketIndex] = domain.TicketRef{ID: reference.ID, Revision: reference.Revision}
		}
		assignment.Teams[index] = domain.TeamAssignment{Team: team.Team, Tickets: references}
	}
	if stored.Backfill != nil {
		assignment.Backfill = &domain.BackfillTarget{
			Ticket:    domain.TicketRef{ID: stored.Backfill.Ticket.ID, Revision: stored.Backfill.Ticket.Revision},
			SessionID: stored.Backfill.SessionID, RosterVersion: stored.Backfill.RosterVersion,
		}
	}
	if stored.Acknowledgment != nil {
		acknowledgment := fromPersistedAcknowledgment(*stored.Acknowledgment)
		assignment.Acknowledgment = &acknowledgment
	}
	if (assignment.Status == domain.AssignmentPending) != (assignment.Acknowledgment == nil) ||
		(assignment.Acknowledgment != nil && assignment.Acknowledgment.Outcome != assignment.Status) {
		return domain.Assignment{}, fmt.Errorf("assignment status and acknowledgment are inconsistent")
	}
	if assignment.Acknowledgment != nil {
		if assignment.Acknowledgment.AcknowledgedAt.IsZero() {
			return domain.Assignment{}, fmt.Errorf("assignment acknowledgment time is missing")
		}
		if err := domain.ValidateAssignmentAcknowledgment(
			assignment,
			assignment.Acknowledgment.AssignmentAcknowledgmentRequest,
		); err != nil {
			return domain.Assignment{}, fmt.Errorf("assignment acknowledgment is invalid: %w", err)
		}
	}
	return assignment, nil
}

func encodeAcknowledgmentResource(assignment domain.Assignment) ([]byte, error) {
	if assignment.Acknowledgment == nil {
		return nil, fmt.Errorf("assignment acknowledgment is required")
	}
	encoded, err := json.Marshal(persistedAcknowledgmentResource{
		Schema: acknowledgmentPayloadSchema, AssignmentID: assignment.ID,
		Acknowledgment: toPersistedAcknowledgment(*assignment.Acknowledgment),
	})
	if err != nil {
		return nil, fmt.Errorf("encode assignment acknowledgment resource: %w", err)
	}
	return encoded, nil
}

func toPersistedAcknowledgment(acknowledgment domain.AssignmentAcknowledgment) persistedAcknowledgment {
	return persistedAcknowledgment{
		OperationID: acknowledgment.OperationID, Outcome: acknowledgment.Outcome,
		SessionID: acknowledgment.SessionID, ExpectedRosterVersion: acknowledgment.ExpectedRosterVersion,
		ResultingRosterVersion: acknowledgment.ResultingRosterVersion,
		FailureCode:            acknowledgment.FailureCode, Reason: acknowledgment.Reason,
		AcknowledgedAt: acknowledgment.AcknowledgedAt.UTC(),
	}
}

func fromPersistedAcknowledgment(stored persistedAcknowledgment) domain.AssignmentAcknowledgment {
	return domain.AssignmentAcknowledgment{
		AssignmentAcknowledgmentRequest: domain.AssignmentAcknowledgmentRequest{
			OperationID: stored.OperationID, Outcome: stored.Outcome, SessionID: stored.SessionID,
			ExpectedRosterVersion:  stored.ExpectedRosterVersion,
			ResultingRosterVersion: stored.ResultingRosterVersion,
			FailureCode:            stored.FailureCode, Reason: stored.Reason,
		},
		AcknowledgedAt: stored.AcknowledgedAt.UTC(),
	}
}

func encodeAssignmentCommandResult(
	kind string,
	assignment domain.Assignment,
	resourceVersion repository.Version,
) ([]byte, error) {
	stored, err := toPersistedAssignment(assignment)
	if err != nil {
		return nil, err
	}
	return encodeOperationResult(kind, assignmentCommandResult{
		Assignment: stored, ResourceVersion: uint64(resourceVersion),
	})
}

func validAssignmentStatus(status domain.AssignmentStatus) bool {
	return status == domain.AssignmentPending || status == domain.AssignmentCompleted ||
		status == domain.AssignmentCancelled || status == domain.AssignmentFailed
}

func cloneAssignment(assignment domain.Assignment) domain.Assignment {
	cloned := assignment
	cloned.Teams = domain.CloneTeams(assignment.Teams)
	cloned.Backfill = domain.CloneBackfillTarget(assignment.Backfill)
	if assignment.Acknowledgment != nil {
		acknowledgment := *assignment.Acknowledgment
		cloned.Acknowledgment = &acknowledgment
	}
	return cloned
}
