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
	reservationPayloadSchema      = "sema.reservation.v1"
	reservationClaimPayloadSchema = "sema.demand-reservation-claim.v1"
	maxReservationAttempts        = 8
)

type ReservationRecord struct {
	Reservation    domain.Reservation
	StorageVersion repository.Version
}

type ReservationSnapshot struct {
	RepositoryVersion repository.Version
	Records           []ReservationRecord
}

type ReservationMutation struct {
	Record   ReservationRecord
	Replayed bool
}

type Reservations struct {
	repository repository.Repository
	now        func() time.Time
	ttl        time.Duration
}

type persistedReservation struct {
	Schema     string                   `json:"schema"`
	ID         domain.ReservationID     `json:"id"`
	ProposalID domain.ProposalID        `json:"proposal_id"`
	Tickets    []persistedTicketRef     `json:"tickets"`
	Backfill   *persistedBackfillTarget `json:"backfill,omitempty"`
	ExpiresAt  time.Time                `json:"expires_at"`
	Status     domain.ReservationStatus `json:"status"`
}

type persistedReservationClaim struct {
	Schema        string               `json:"schema"`
	ReservationID domain.ReservationID `json:"reservation_id"`
	ProposalID    domain.ProposalID    `json:"proposal_id"`
	Ticket        persistedTicketRef   `json:"ticket"`
	Kind          domain.ProposalKind  `json:"kind"`
}

type reservationCommandResult struct {
	Reservation     persistedReservation `json:"reservation"`
	ResourceVersion uint64               `json:"resource_version,omitempty"`
}

func NewReservations(owner repository.Repository, now func() time.Time, ttl time.Duration) (*Reservations, error) {
	if owner == nil {
		return nil, domain.NewFailure(domain.FailureInvalidInput, "repository is required")
	}
	if ttl <= 0 {
		return nil, domain.NewFailure(domain.FailureInvalidInput, "reservation TTL must be positive")
	}
	if now == nil {
		now = time.Now
	}
	return &Reservations{repository: owner, now: now, ttl: ttl}, nil
}

func (service *Reservations) Reserve(
	ctx context.Context,
	scope string,
	operationID domain.OperationID,
	reservationID domain.ReservationID,
	proposalID domain.ProposalID,
) (ReservationMutation, error) {
	command, err := json.Marshal(struct {
		Kind          string               `json:"kind"`
		ReservationID domain.ReservationID `json:"reservation_id"`
		ProposalID    domain.ProposalID    `json:"proposal_id"`
	}{Kind: "reservation.reserve", ReservationID: reservationID, ProposalID: proposalID})
	if err != nil {
		return ReservationMutation{}, fmt.Errorf("encode reservation command: %w", err)
	}
	operation, err := service.operation(scope, operationID, "reservation.reserve", command)
	if err != nil {
		return ReservationMutation{}, err
	}
	if reservationID == "" || proposalID == "" {
		return ReservationMutation{}, domain.NewFailure(
			domain.FailureInvalidInput,
			"reservation and proposal identities are required",
		)
	}
	if replay, exists, err := service.repository.Replay(ctx, operation); err != nil {
		return ReservationMutation{}, err
	} else if exists {
		return service.replayedMutation(ctx, scope, operationID, "reservation.reserve", replay)
	}

	if err := service.expireDue(ctx, scope, operation.At); err != nil {
		return ReservationMutation{}, err
	}
	for attempt := 0; attempt < maxReservationAttempts; attempt++ {
		snapshot, err := service.repository.Snapshot(ctx, scope)
		if err != nil {
			return ReservationMutation{}, err
		}
		if _, exists := findResource(snapshot, Key(scope, ResourceReservation, string(reservationID))); exists {
			return ReservationMutation{}, domain.NewFailure(
				domain.FailureInvalidRevision,
				"reservation %q is already registered",
				reservationID,
			)
		}
		proposalResource, exists := findResource(snapshot, Key(scope, ResourceProposal, string(proposalID)))
		if !exists || proposalResource.Deleted {
			return ReservationMutation{}, ErrResourceNotFound
		}
		proposalRecord, err := decodeProposal(proposalResource.Payload)
		if err != nil {
			return ReservationMutation{}, err
		}
		proposal := proposalRecord.Proposal
		if err := domain.ValidateProposal(proposal); err != nil {
			return ReservationMutation{}, err
		}
		if proposal.ID != proposalID {
			return ReservationMutation{}, fmt.Errorf("proposal resource identity is inconsistent")
		}
		reservation := domain.Reservation{
			ID: reservationID, ProposalID: proposalID,
			Tickets: cloneTicketRefs(proposal.Tickets), Backfill: cloneBackfillTarget(proposal.Backfill),
			ExpiresAt: operation.At.Add(service.ttl), Status: domain.ReservationActive,
		}
		mutations, err := service.reserveMutations(snapshot, scope, reservation, proposal)
		if err != nil {
			return ReservationMutation{}, err
		}
		resultPayload, err := encodeReservationCommandResult(reservation, 0)
		if err != nil {
			return ReservationMutation{}, err
		}
		mutations = append(mutations, repository.Mutation{
			Key: Key(scope, ResourceOperationResult, string(operationID)), Payload: resultPayload,
		})
		result, err := service.repository.Commit(ctx, operation, mutations)
		if err == nil {
			return ReservationMutation{Record: ReservationRecord{
				Reservation: cloneReservation(reservation), StorageVersion: result.Version,
			}, Replayed: result.Replayed}, nil
		}
		if !repository.IsConflict(err) {
			return ReservationMutation{}, err
		}
		if replay, exists, replayErr := service.repository.Replay(ctx, operation); replayErr != nil {
			return ReservationMutation{}, replayErr
		} else if exists {
			return service.replayedMutation(ctx, scope, operationID, "reservation.reserve", replay)
		}
		if err := service.expireDue(ctx, scope, operation.At); err != nil {
			return ReservationMutation{}, err
		}
	}
	return ReservationMutation{}, domain.NewFailure(
		domain.FailureReservationConflict,
		"reservation demand changed repeatedly while it was being claimed",
	)
}

func (service *Reservations) Cancel(
	ctx context.Context,
	scope string,
	operationID domain.OperationID,
	reservationID domain.ReservationID,
) (ReservationMutation, error) {
	command, err := json.Marshal(struct {
		Kind          string               `json:"kind"`
		ReservationID domain.ReservationID `json:"reservation_id"`
	}{Kind: "reservation.cancel", ReservationID: reservationID})
	if err != nil {
		return ReservationMutation{}, fmt.Errorf("encode reservation cancellation: %w", err)
	}
	operation, err := service.operation(scope, operationID, "reservation.cancel", command)
	if err != nil {
		return ReservationMutation{}, err
	}
	if reservationID == "" {
		return ReservationMutation{}, domain.NewFailure(domain.FailureInvalidInput, "reservation identity is required")
	}
	if replay, exists, err := service.repository.Replay(ctx, operation); err != nil {
		return ReservationMutation{}, err
	} else if exists {
		return service.replayedMutation(ctx, scope, operationID, "reservation.cancel", replay)
	}
	if err := service.expireDue(ctx, scope, operation.At); err != nil {
		return ReservationMutation{}, err
	}

	snapshot, err := service.repository.Snapshot(ctx, scope)
	if err != nil {
		return ReservationMutation{}, err
	}
	resource, exists := findResource(snapshot, Key(scope, ResourceReservation, string(reservationID)))
	if !exists || resource.Deleted {
		return ReservationMutation{}, ErrResourceNotFound
	}
	reservation, err := decodeReservation(resource.Payload)
	if err != nil {
		return ReservationMutation{}, err
	}
	switch reservation.Status {
	case domain.ReservationExpired:
		return ReservationMutation{}, domain.NewFailure(
			domain.FailureReservationExpired, "reservation %q has expired", reservationID,
		)
	case domain.ReservationConfirmed:
		return ReservationMutation{}, domain.NewFailure(
			domain.FailureInvalidTransition, "confirmed reservation %q cannot be cancelled", reservationID,
		)
	case domain.ReservationActive, domain.ReservationCancelled:
	default:
		return ReservationMutation{}, fmt.Errorf("reservation %q has invalid status", reservationID)
	}

	mutations := make([]repository.Mutation, 0, len(reservation.Tickets)+3)
	resourceVersion := resource.Version
	if reservation.Status == domain.ReservationActive {
		reservation.Status = domain.ReservationCancelled
		payload, err := encodeReservation(reservation)
		if err != nil {
			return ReservationMutation{}, err
		}
		mutations = append(mutations, repository.Mutation{
			Key: resource.Key, ExpectedVersion: resource.Version, Payload: payload,
		})
		claimMutations, err := reservationClaimDeletions(snapshot, scope, reservation)
		if err != nil {
			return ReservationMutation{}, err
		}
		mutations = append(mutations, claimMutations...)
		resourceVersion = 0
	}
	resultPayload, err := encodeReservationCommandResult(reservation, resourceVersion)
	if err != nil {
		return ReservationMutation{}, err
	}
	mutations = append(mutations, repository.Mutation{
		Key: Key(scope, ResourceOperationResult, string(operationID)), Payload: resultPayload,
	})
	result, err := service.repository.Commit(ctx, operation, mutations)
	if err != nil {
		if repository.IsConflict(err) {
			return ReservationMutation{}, domain.NewFailure(
				domain.FailureReservationConflict,
				"reservation %q changed while cancellation was committed",
				reservationID,
			)
		}
		return ReservationMutation{}, err
	}
	if resourceVersion == 0 {
		resourceVersion = result.Version
	}
	return ReservationMutation{Record: ReservationRecord{
		Reservation: cloneReservation(reservation), StorageVersion: resourceVersion,
	}, Replayed: result.Replayed}, nil
}

func (service *Reservations) Get(
	ctx context.Context,
	scope string,
	reservationID domain.ReservationID,
) (ReservationRecord, bool, error) {
	if reservationID == "" {
		return ReservationRecord{}, false, domain.NewFailure(domain.FailureInvalidInput, "reservation identity is required")
	}
	if err := service.expireDue(ctx, scope, service.now().UTC()); err != nil {
		return ReservationRecord{}, false, err
	}
	snapshot, err := service.repository.Snapshot(ctx, scope)
	if err != nil {
		return ReservationRecord{}, false, err
	}
	resource, exists := findResource(snapshot, Key(scope, ResourceReservation, string(reservationID)))
	if !exists || resource.Deleted {
		return ReservationRecord{}, false, nil
	}
	reservation, err := decodeReservation(resource.Payload)
	if err != nil {
		return ReservationRecord{}, false, err
	}
	return ReservationRecord{Reservation: reservation, StorageVersion: resource.Version}, true, nil
}

func (service *Reservations) Snapshot(ctx context.Context, scope string) (ReservationSnapshot, error) {
	if err := service.expireDue(ctx, scope, service.now().UTC()); err != nil {
		return ReservationSnapshot{}, err
	}
	snapshot, err := service.repository.Snapshot(ctx, scope)
	if err != nil {
		return ReservationSnapshot{}, err
	}
	records := make([]ReservationRecord, 0)
	for _, resource := range snapshot.Resources {
		if resource.Key.Kind != string(ResourceReservation) || resource.Deleted {
			continue
		}
		reservation, err := decodeReservation(resource.Payload)
		if err != nil {
			return ReservationSnapshot{}, err
		}
		records = append(records, ReservationRecord{Reservation: reservation, StorageVersion: resource.Version})
	}
	slices.SortFunc(records, func(left, right ReservationRecord) int {
		if left.Reservation.ID < right.Reservation.ID {
			return -1
		}
		if left.Reservation.ID > right.Reservation.ID {
			return 1
		}
		return 0
	})
	return ReservationSnapshot{RepositoryVersion: snapshot.Version, Records: records}, nil
}

func (service *Reservations) reserveMutations(
	snapshot repository.Snapshot,
	scope string,
	reservation domain.Reservation,
	proposal domain.MatchProposal,
) ([]repository.Mutation, error) {
	payload, err := encodeReservation(reservation)
	if err != nil {
		return nil, err
	}
	mutations := []repository.Mutation{{
		Key: Key(scope, ResourceReservation, string(reservation.ID)), Payload: payload,
	}}
	for _, reference := range proposal.Tickets {
		resource, exists := findResource(snapshot, Key(scope, ResourceMatchTicket, string(reference.ID)))
		if !exists || resource.Deleted {
			return nil, domain.NewFailure(domain.FailureStaleSnapshot, "match ticket %q is no longer active", reference.ID)
		}
		ticket, err := decodeMatchTicket(resource.Payload)
		if err != nil {
			return nil, err
		}
		if ticket.Revision != reference.Revision {
			return nil, domain.NewFailure(domain.FailureStaleSnapshot, "match ticket %q revision changed", reference.ID)
		}
		claim, err := reservationClaimMutation(snapshot, scope, reservation, reference, proposal.Kind)
		if err != nil {
			return nil, err
		}
		mutations = append(mutations, claim)
	}
	if proposal.Backfill != nil {
		target := proposal.Backfill
		resource, exists := findResource(snapshot, Key(scope, ResourceBackfillTicket, string(target.Ticket.ID)))
		if !exists || resource.Deleted {
			return nil, domain.NewFailure(domain.FailureStaleSnapshot, "backfill ticket %q is no longer active", target.Ticket.ID)
		}
		ticket, err := decodeBackfillTicket(resource.Payload)
		if err != nil {
			return nil, err
		}
		if ticket.Revision != target.Ticket.Revision || ticket.SessionID != target.SessionID ||
			ticket.RosterVersion != target.RosterVersion {
			return nil, domain.NewFailure(domain.FailureStaleSnapshot, "backfill ticket %q freshness changed", target.Ticket.ID)
		}
		claim, err := reservationClaimMutation(snapshot, scope, reservation, target.Ticket, proposal.Kind)
		if err != nil {
			return nil, err
		}
		mutations = append(mutations, claim)
	}
	return mutations, nil
}

func reservationClaimMutation(
	snapshot repository.Snapshot,
	scope string,
	reservation domain.Reservation,
	ticket domain.TicketRef,
	kind domain.ProposalKind,
) (repository.Mutation, error) {
	payload, err := encodeReservationClaim(reservation, ticket, kind)
	if err != nil {
		return repository.Mutation{}, err
	}
	key := Key(scope, ResourceDemandReservation, string(ticket.ID))
	expected := repository.Version(0)
	if resource, exists := findResource(snapshot, key); exists {
		if !resource.Deleted {
			claim, err := decodeReservationClaim(resource.Payload)
			if err != nil {
				return repository.Mutation{}, err
			}
			return repository.Mutation{}, domain.NewFailure(
				domain.FailureReservationConflict,
				"ticket %q is reserved by %q",
				ticket.ID,
				claim.ReservationID,
			)
		}
		expected = resource.Version
	}
	return repository.Mutation{Key: key, ExpectedVersion: expected, Payload: payload}, nil
}

func reservationClaimDeletions(
	snapshot repository.Snapshot,
	scope string,
	reservation domain.Reservation,
) ([]repository.Mutation, error) {
	references := cloneTicketRefs(reservation.Tickets)
	if reservation.Backfill != nil {
		references = append(references, reservation.Backfill.Ticket)
	}
	mutations := make([]repository.Mutation, 0, len(references))
	for _, reference := range references {
		key := Key(scope, ResourceDemandReservation, string(reference.ID))
		resource, exists := findResource(snapshot, key)
		if !exists || resource.Deleted {
			return nil, fmt.Errorf("reservation claim %q is missing", reference.ID)
		}
		claim, err := decodeReservationClaim(resource.Payload)
		if err != nil {
			return nil, err
		}
		if claim.ReservationID != reservation.ID || claim.Ticket.ID != reference.ID ||
			claim.Ticket.Revision != reference.Revision {
			return nil, fmt.Errorf("reservation claim %q belongs to another reservation", reference.ID)
		}
		mutations = append(mutations, repository.Mutation{
			Key: key, ExpectedVersion: resource.Version, Delete: true,
		})
	}
	return mutations, nil
}

func (service *Reservations) expireDue(ctx context.Context, scope string, now time.Time) error {
	for attempt := 0; attempt < maxReservationAttempts; attempt++ {
		snapshot, err := service.repository.Snapshot(ctx, scope)
		if err != nil {
			return err
		}
		expired := make([]domain.Reservation, 0)
		for _, resource := range snapshot.Resources {
			if resource.Key.Kind != string(ResourceReservation) || resource.Deleted {
				continue
			}
			reservation, err := decodeReservation(resource.Payload)
			if err != nil {
				return err
			}
			if reservation.Status == domain.ReservationActive && !now.Before(reservation.ExpiresAt) {
				expired = append(expired, cloneReservation(reservation))
			}
		}
		if len(expired) == 0 {
			return nil
		}
		conflicted := false
		for _, reservation := range expired {
			if err := service.expireOne(ctx, scope, reservation); err != nil {
				if repository.IsConflict(err) {
					conflicted = true
					continue
				}
				return err
			}
		}
		if !conflicted {
			return nil
		}
	}
	return domain.NewFailure(domain.FailureStaleSnapshot, "expired reservations changed repeatedly")
}

func (service *Reservations) expireOne(
	ctx context.Context,
	scope string,
	reservation domain.Reservation,
) error {
	snapshot, err := service.repository.Snapshot(ctx, scope)
	if err != nil {
		return err
	}
	resource, exists := findResource(snapshot, Key(scope, ResourceReservation, string(reservation.ID)))
	if !exists || resource.Deleted {
		return nil
	}
	current, err := decodeReservation(resource.Payload)
	if err != nil {
		return err
	}
	if current.Status != domain.ReservationActive {
		return nil
	}
	current.Status = domain.ReservationExpired
	payload, err := encodeReservation(current)
	if err != nil {
		return err
	}
	mutations := []repository.Mutation{{
		Key: resource.Key, ExpectedVersion: resource.Version, Payload: payload,
	}}
	claimMutations, err := reservationClaimDeletions(snapshot, scope, current)
	if err != nil {
		return err
	}
	mutations = append(mutations, claimMutations...)
	command, err := json.Marshal(struct {
		Kind      string               `json:"kind"`
		ID        domain.ReservationID `json:"id"`
		ExpiresAt time.Time            `json:"expires_at"`
	}{Kind: "reservation.expire", ID: current.ID, ExpiresAt: current.ExpiresAt})
	if err != nil {
		return fmt.Errorf("encode reservation expiry: %w", err)
	}
	operation := repository.Operation{
		Scope: scope, ID: domain.OperationID("/reservation.expire/" + string(current.ID)),
		Kind: "reservation.expire", Digest: repository.Digest(command), At: current.ExpiresAt,
	}
	_, err = service.repository.Commit(ctx, operation, mutations)
	return err
}

func (service *Reservations) invalidateStale(
	ctx context.Context,
	scope string,
	reservation domain.Reservation,
	at time.Time,
) error {
	snapshot, err := service.repository.Snapshot(ctx, scope)
	if err != nil {
		return err
	}
	resource, exists := findResource(snapshot, Key(scope, ResourceReservation, string(reservation.ID)))
	if !exists || resource.Deleted {
		return nil
	}
	current, err := decodeReservation(resource.Payload)
	if err != nil {
		return err
	}
	if current.Status != domain.ReservationActive {
		return nil
	}
	current.Status = domain.ReservationCancelled
	payload, err := encodeReservation(current)
	if err != nil {
		return err
	}
	mutations := []repository.Mutation{{
		Key: resource.Key, ExpectedVersion: resource.Version, Payload: payload,
	}}
	claimMutations, err := reservationClaimDeletions(snapshot, scope, current)
	if err != nil {
		return err
	}
	mutations = append(mutations, claimMutations...)
	command, err := json.Marshal(struct {
		Kind       string               `json:"kind"`
		ID         domain.ReservationID `json:"id"`
		ProposalID domain.ProposalID    `json:"proposal_id"`
	}{Kind: "reservation.invalidate_stale", ID: current.ID, ProposalID: current.ProposalID})
	if err != nil {
		return fmt.Errorf("encode stale reservation invalidation: %w", err)
	}
	operation := repository.Operation{
		Scope: scope, ID: domain.OperationID("/reservation.invalidate-stale/" + string(current.ID)),
		Kind: "reservation.invalidate_stale", Digest: repository.Digest(command), At: at,
	}
	_, err = service.repository.Commit(ctx, operation, mutations)
	return err
}

func (service *Reservations) replayedMutation(
	ctx context.Context,
	scope string,
	operationID domain.OperationID,
	kind string,
	replay repository.CommitResult,
) (ReservationMutation, error) {
	snapshot, err := service.repository.Snapshot(ctx, scope)
	if err != nil {
		return ReservationMutation{}, err
	}
	resource, exists := findResource(snapshot, Key(scope, ResourceOperationResult, string(operationID)))
	if !exists || resource.Deleted {
		return ReservationMutation{}, fmt.Errorf("%s operation result is missing", kind)
	}
	var result reservationCommandResult
	if err := decodeOperationResult(resource.Payload, kind, &result); err != nil {
		return ReservationMutation{}, err
	}
	reservation, err := fromPersistedReservation(result.Reservation)
	if err != nil {
		return ReservationMutation{}, err
	}
	resourceVersion := repository.Version(result.ResourceVersion)
	if resourceVersion == 0 {
		resourceVersion = replay.Version
	}
	return ReservationMutation{Record: ReservationRecord{
		Reservation: reservation, StorageVersion: resourceVersion,
	}, Replayed: true}, nil
}

func (service *Reservations) operation(
	scope string,
	id domain.OperationID,
	kind string,
	canonical []byte,
) (repository.Operation, error) {
	operation := repository.Operation{
		Scope: scope, ID: id, Kind: kind, Digest: repository.Digest(canonical), At: service.now().UTC(),
	}
	if err := repository.ValidateOperation(operation); err != nil {
		return repository.Operation{}, err
	}
	return operation, nil
}

func encodeReservation(reservation domain.Reservation) ([]byte, error) {
	stored, err := toPersistedReservation(reservation)
	if err != nil {
		return nil, err
	}
	encoded, err := json.Marshal(stored)
	if err != nil {
		return nil, fmt.Errorf("encode reservation resource: %w", err)
	}
	return encoded, nil
}

func decodeReservation(payload []byte) (domain.Reservation, error) {
	var stored persistedReservation
	if err := decodeStrict(payload, &stored); err != nil {
		return domain.Reservation{}, fmt.Errorf("decode reservation resource: %w", err)
	}
	return fromPersistedReservation(stored)
}

func toPersistedReservation(reservation domain.Reservation) (persistedReservation, error) {
	if reservation.ID == "" || reservation.ProposalID == "" || len(reservation.Tickets) == 0 ||
		reservation.ExpiresAt.IsZero() || !validReservationStatus(reservation.Status) {
		return persistedReservation{}, domain.NewFailure(domain.FailureInvalidInput, "reservation is invalid")
	}
	tickets := make([]persistedTicketRef, len(reservation.Tickets))
	for index, reference := range reservation.Tickets {
		if reference.ID == "" || reference.Revision == 0 {
			return persistedReservation{}, domain.NewFailure(domain.FailureInvalidInput, "reservation ticket reference is invalid")
		}
		tickets[index] = toPersistedTicketRef(reference)
	}
	var backfill *persistedBackfillTarget
	if reservation.Backfill != nil {
		if reservation.Backfill.Ticket.ID == "" || reservation.Backfill.Ticket.Revision == 0 ||
			reservation.Backfill.SessionID == "" || reservation.Backfill.RosterVersion == 0 {
			return persistedReservation{}, domain.NewFailure(domain.FailureInvalidInput, "reservation backfill target is invalid")
		}
		backfill = &persistedBackfillTarget{
			Ticket:    toPersistedTicketRef(reservation.Backfill.Ticket),
			SessionID: reservation.Backfill.SessionID, RosterVersion: reservation.Backfill.RosterVersion,
		}
	}
	return persistedReservation{
		Schema: reservationPayloadSchema, ID: reservation.ID, ProposalID: reservation.ProposalID,
		Tickets: tickets, Backfill: backfill, ExpiresAt: reservation.ExpiresAt.UTC(), Status: reservation.Status,
	}, nil
}

func fromPersistedReservation(stored persistedReservation) (domain.Reservation, error) {
	if stored.Schema != reservationPayloadSchema || stored.ID == "" || stored.ProposalID == "" ||
		len(stored.Tickets) == 0 || stored.ExpiresAt.IsZero() || !validReservationStatus(stored.Status) {
		return domain.Reservation{}, fmt.Errorf("reservation resource header is invalid")
	}
	reservation := domain.Reservation{
		ID: stored.ID, ProposalID: stored.ProposalID, ExpiresAt: stored.ExpiresAt.UTC(), Status: stored.Status,
		Tickets: make([]domain.TicketRef, len(stored.Tickets)),
	}
	for index, reference := range stored.Tickets {
		if reference.ID == "" || reference.Revision == 0 {
			return domain.Reservation{}, fmt.Errorf("reservation ticket reference is invalid")
		}
		reservation.Tickets[index] = domain.TicketRef{ID: reference.ID, Revision: reference.Revision}
	}
	if stored.Backfill != nil {
		if stored.Backfill.Ticket.ID == "" || stored.Backfill.Ticket.Revision == 0 ||
			stored.Backfill.SessionID == "" || stored.Backfill.RosterVersion == 0 {
			return domain.Reservation{}, fmt.Errorf("reservation backfill target is invalid")
		}
		reservation.Backfill = &domain.BackfillTarget{
			Ticket:    domain.TicketRef{ID: stored.Backfill.Ticket.ID, Revision: stored.Backfill.Ticket.Revision},
			SessionID: stored.Backfill.SessionID, RosterVersion: stored.Backfill.RosterVersion,
		}
	}
	return reservation, nil
}

func encodeReservationClaim(
	reservation domain.Reservation,
	ticket domain.TicketRef,
	kind domain.ProposalKind,
) ([]byte, error) {
	encoded, err := json.Marshal(persistedReservationClaim{
		Schema: reservationClaimPayloadSchema, ReservationID: reservation.ID, ProposalID: reservation.ProposalID,
		Ticket: toPersistedTicketRef(ticket), Kind: kind,
	})
	if err != nil {
		return nil, fmt.Errorf("encode reservation claim: %w", err)
	}
	return encoded, nil
}

func decodeReservationClaim(payload []byte) (persistedReservationClaim, error) {
	var claim persistedReservationClaim
	if err := decodeStrict(payload, &claim); err != nil {
		return persistedReservationClaim{}, fmt.Errorf("decode reservation claim: %w", err)
	}
	if claim.Schema != reservationClaimPayloadSchema || claim.ReservationID == "" || claim.ProposalID == "" ||
		claim.Ticket.ID == "" || claim.Ticket.Revision == 0 ||
		(claim.Kind != domain.ProposalNewMatch && claim.Kind != domain.ProposalBackfill) {
		return persistedReservationClaim{}, fmt.Errorf("reservation claim is invalid")
	}
	return claim, nil
}

func encodeReservationCommandResult(reservation domain.Reservation, resourceVersion repository.Version) ([]byte, error) {
	stored, err := toPersistedReservation(reservation)
	if err != nil {
		return nil, err
	}
	return encodeOperationResult("reservation."+reservationCommandSuffix(reservation.Status), reservationCommandResult{
		Reservation: stored, ResourceVersion: uint64(resourceVersion),
	})
}

func reservationCommandSuffix(status domain.ReservationStatus) string {
	if status == domain.ReservationActive {
		return "reserve"
	}
	return "cancel"
}

func validReservationStatus(status domain.ReservationStatus) bool {
	return status == domain.ReservationActive || status == domain.ReservationConfirmed ||
		status == domain.ReservationCancelled || status == domain.ReservationExpired
}

func cloneReservation(reservation domain.Reservation) domain.Reservation {
	cloned := reservation
	cloned.Tickets = cloneTicketRefs(reservation.Tickets)
	cloned.Backfill = cloneBackfillTarget(reservation.Backfill)
	return cloned
}

func cloneTicketRefs(references []domain.TicketRef) []domain.TicketRef {
	return slices.Clone(references)
}

func cloneBackfillTarget(target *domain.BackfillTarget) *domain.BackfillTarget {
	if target == nil {
		return nil
	}
	cloned := *target
	return &cloned
}
