package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"time"

	"github.com/zrma/sema/internal/domain"
	"github.com/zrma/sema/internal/repository"
)

const backfillTicketPayloadSchema = "sema.backfill-ticket.v1"

type BackfillTicketRecord struct {
	Ticket         domain.BackfillTicket
	StorageVersion repository.Version
	Deleted        bool
}

type BackfillTicketSnapshot struct {
	RepositoryVersion repository.Version
	Records           []BackfillTicketRecord
}

type BackfillTicketMutation struct {
	Ticket         domain.BackfillTicket
	StorageVersion repository.Version
	Replayed       bool
}

type BackfillTicketCancellation struct {
	ID             domain.TicketID
	Revision       domain.Revision
	RosterVersion  domain.Revision
	StorageVersion repository.Version
	Replayed       bool
}

// BackfillTickets owns target backfill demand semantics and the one-active-
// demand-per-session claim above any repository adapter.
type BackfillTickets struct {
	repository repository.Repository
	now        func() time.Time
}

func NewBackfillTickets(owner repository.Repository, now func() time.Time) (*BackfillTickets, error) {
	if owner == nil {
		return nil, domain.NewFailure(domain.FailureInvalidInput, "repository is required")
	}
	if now == nil {
		now = time.Now
	}
	return &BackfillTickets{repository: owner, now: now}, nil
}

func (service *BackfillTickets) Put(
	ctx context.Context,
	scope string,
	operationID domain.OperationID,
	ticket domain.BackfillTicket,
) (BackfillTicketMutation, error) {
	ticket.EnqueuedAt = ticket.EnqueuedAt.UTC()
	if err := domain.ValidateBackfillTicket(ticket); err != nil {
		return BackfillTicketMutation{}, err
	}
	payload, err := encodeBackfillTicket(ticket)
	if err != nil {
		return BackfillTicketMutation{}, err
	}
	operation, err := service.operation(
		scope, operationID, "backfill_ticket.put", append([]byte("put\x00"), payload...),
	)
	if err != nil {
		return BackfillTicketMutation{}, err
	}
	if replayed, exists, err := service.repository.Replay(ctx, operation); err != nil {
		return BackfillTicketMutation{}, err
	} else if exists {
		return BackfillTicketMutation{
			Ticket: ticket, StorageVersion: replayed.Version, Replayed: true,
		}, nil
	}

	snapshot, err := service.repository.Snapshot(ctx, scope)
	if err != nil {
		return BackfillTicketMutation{}, err
	}
	key := Key(scope, ResourceBackfillTicket, string(ticket.ID))
	mutations := make([]repository.Mutation, 0, 3)
	expected := repository.Version(0)
	if current, exists := findResource(snapshot, key); exists {
		if current.Deleted {
			return BackfillTicketMutation{}, domain.NewFailure(
				domain.FailureInvalidRevision,
				"backfill ticket %q is cancelled and its identity cannot be reused",
				ticket.ID,
			)
		}
		stored, err := decodeBackfillTicket(current.Payload)
		if err != nil {
			return BackfillTicketMutation{}, err
		}
		if err := validateBackfillReplacement(stored, ticket); err != nil {
			return BackfillTicketMutation{}, err
		}
		if err := validateDemandIdentity(snapshot, scope, ResourceBackfillTicket, string(ticket.ID)); err != nil {
			return BackfillTicketMutation{}, err
		}
		if err := validateBackfillSessionClaim(snapshot, scope, ticket.SessionID, ticket.ID); err != nil {
			return BackfillTicketMutation{}, err
		}
		expected = current.Version
	} else {
		identityPayload, err := encodeDemandIdentityClaim(ResourceBackfillTicket, string(ticket.ID))
		if err != nil {
			return BackfillTicketMutation{}, err
		}
		identityKey := Key(scope, ResourceDemandIdentity, string(ticket.ID))
		if _, exists := findResource(snapshot, identityKey); exists {
			return BackfillTicketMutation{}, domain.NewFailure(
				domain.FailureInvalidRevision,
				"ticket identity %q is already claimed",
				ticket.ID,
			)
		}
		mutations = append(mutations, repository.Mutation{Key: identityKey, Payload: identityPayload})

		sessionPayload, err := encodeBackfillSessionClaim(ticket)
		if err != nil {
			return BackfillTicketMutation{}, err
		}
		sessionKey := Key(scope, ResourceBackfillSessionClaim, string(ticket.SessionID))
		sessionExpected := repository.Version(0)
		if currentSession, exists := findResource(snapshot, sessionKey); exists {
			if !currentSession.Deleted {
				return BackfillTicketMutation{}, domain.NewFailure(
					domain.FailureInvalidInput,
					"session %q already has an active backfill ticket",
					ticket.SessionID,
				)
			}
			sessionExpected = currentSession.Version
		}
		mutations = append(mutations, repository.Mutation{
			Key: sessionKey, ExpectedVersion: sessionExpected, Payload: sessionPayload,
		})
	}
	mutations = append(mutations, repository.Mutation{
		Key: key, ExpectedVersion: expected, Payload: payload,
	})
	result, err := service.repository.Commit(ctx, operation, mutations)
	if err != nil {
		return BackfillTicketMutation{}, mapBackfillConflict(err, ticket)
	}
	return BackfillTicketMutation{
		Ticket: ticket, StorageVersion: result.Version, Replayed: result.Replayed,
	}, nil
}

func (service *BackfillTickets) Cancel(
	ctx context.Context,
	scope string,
	operationID domain.OperationID,
	id domain.TicketID,
	revision domain.Revision,
	rosterVersion domain.Revision,
) (BackfillTicketCancellation, error) {
	command, err := json.Marshal(struct {
		Kind          string          `json:"kind"`
		ID            domain.TicketID `json:"id"`
		Revision      domain.Revision `json:"revision"`
		RosterVersion domain.Revision `json:"roster_version"`
	}{
		Kind: "backfill_ticket.cancel", ID: id, Revision: revision, RosterVersion: rosterVersion,
	})
	if err != nil {
		return BackfillTicketCancellation{}, fmt.Errorf("encode backfill ticket cancellation: %w", err)
	}
	operation, err := service.operation(scope, operationID, "backfill_ticket.cancel", command)
	if err != nil {
		return BackfillTicketCancellation{}, err
	}
	if id == "" || revision == 0 || rosterVersion == 0 {
		return BackfillTicketCancellation{}, domain.NewFailure(
			domain.FailureInvalidInput,
			"backfill ticket identity, revision, and roster version are required",
		)
	}
	if replayed, exists, err := service.repository.Replay(ctx, operation); err != nil {
		return BackfillTicketCancellation{}, err
	} else if exists {
		return BackfillTicketCancellation{
			ID: id, Revision: revision, RosterVersion: rosterVersion,
			StorageVersion: replayed.Version, Replayed: true,
		}, nil
	}

	snapshot, err := service.repository.Snapshot(ctx, scope)
	if err != nil {
		return BackfillTicketCancellation{}, err
	}
	key := Key(scope, ResourceBackfillTicket, string(id))
	current, exists := findResource(snapshot, key)
	if !exists || current.Deleted {
		return BackfillTicketCancellation{}, ErrResourceNotFound
	}
	stored, err := decodeBackfillTicket(current.Payload)
	if err != nil {
		return BackfillTicketCancellation{}, err
	}
	if stored.Revision != revision || stored.RosterVersion != rosterVersion {
		return BackfillTicketCancellation{}, domain.NewFailure(
			domain.FailureStaleSnapshot,
			"backfill ticket %q freshness changed",
			id,
		)
	}
	sessionKey := Key(scope, ResourceBackfillSessionClaim, string(stored.SessionID))
	sessionClaim, exists := findResource(snapshot, sessionKey)
	if !exists || sessionClaim.Deleted {
		return BackfillTicketCancellation{}, fmt.Errorf("backfill session claim %q is missing", stored.SessionID)
	}
	claim, err := decodeBackfillSessionClaim(sessionClaim.Payload)
	if err != nil {
		return BackfillTicketCancellation{}, err
	}
	if claim.TicketID != id || claim.SessionID != stored.SessionID {
		return BackfillTicketCancellation{}, fmt.Errorf("backfill session claim %q belongs to another ticket", stored.SessionID)
	}
	result, err := service.repository.Commit(ctx, operation, []repository.Mutation{
		{Key: key, ExpectedVersion: current.Version, Delete: true},
		{Key: sessionKey, ExpectedVersion: sessionClaim.Version, Delete: true},
	})
	if err != nil {
		if repository.IsConflict(err) {
			return BackfillTicketCancellation{}, domain.NewFailure(
				domain.FailureStaleSnapshot,
				"backfill ticket %q freshness changed while cancellation was committed",
				id,
			)
		}
		return BackfillTicketCancellation{}, err
	}
	return BackfillTicketCancellation{
		ID: id, Revision: revision, RosterVersion: rosterVersion,
		StorageVersion: result.Version, Replayed: result.Replayed,
	}, nil
}

func (service *BackfillTickets) Get(
	ctx context.Context,
	scope string,
	id domain.TicketID,
) (BackfillTicketRecord, bool, error) {
	snapshot, err := service.repository.Snapshot(ctx, scope)
	if err != nil {
		return BackfillTicketRecord{}, false, err
	}
	resource, exists := findResource(snapshot, Key(scope, ResourceBackfillTicket, string(id)))
	if !exists || resource.Deleted {
		return BackfillTicketRecord{}, false, nil
	}
	ticket, err := decodeBackfillTicket(resource.Payload)
	if err != nil {
		return BackfillTicketRecord{}, false, err
	}
	return BackfillTicketRecord{Ticket: ticket, StorageVersion: resource.Version}, true, nil
}

func (service *BackfillTickets) Snapshot(
	ctx context.Context,
	scope string,
) (BackfillTicketSnapshot, error) {
	snapshot, err := service.repository.Snapshot(ctx, scope)
	if err != nil {
		return BackfillTicketSnapshot{}, err
	}
	records := make([]BackfillTicketRecord, 0)
	for _, resource := range snapshot.Resources {
		if resource.Key.Kind != string(ResourceBackfillTicket) {
			continue
		}
		record := BackfillTicketRecord{
			Ticket:         domain.BackfillTicket{ID: domain.TicketID(resource.Key.ID)},
			StorageVersion: resource.Version,
			Deleted:        resource.Deleted,
		}
		if !resource.Deleted {
			record.Ticket, err = decodeBackfillTicket(resource.Payload)
			if err != nil {
				return BackfillTicketSnapshot{}, err
			}
		}
		records = append(records, record)
	}
	slices.SortFunc(records, func(left, right BackfillTicketRecord) int {
		if left.Ticket.ID < right.Ticket.ID {
			return -1
		}
		if left.Ticket.ID > right.Ticket.ID {
			return 1
		}
		return 0
	})
	return BackfillTicketSnapshot{RepositoryVersion: snapshot.Version, Records: records}, nil
}

func (service *BackfillTickets) operation(
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

func validateBackfillReplacement(current, next domain.BackfillTicket) error {
	if next.Revision <= current.Revision {
		return domain.NewFailure(
			domain.FailureInvalidRevision,
			"backfill ticket %q revision %d must be higher than current revision %d",
			next.ID,
			next.Revision,
			current.Revision,
		)
	}
	if next.SessionID != current.SessionID {
		return domain.NewFailure(
			domain.FailureInvalidRevision,
			"backfill ticket %q cannot change session identity",
			next.ID,
		)
	}
	if next.RosterVersion < current.RosterVersion {
		return domain.NewFailure(
			domain.FailureInvalidRevision,
			"backfill ticket %q roster version moved backwards",
			next.ID,
		)
	}
	if next.RosterVersion == current.RosterVersion && !sameBackfillRoster(current, next) {
		return domain.NewFailure(
			domain.FailureInvalidRevision,
			"backfill ticket %q changed roster content without advancing roster version",
			next.ID,
		)
	}
	return nil
}

func sameBackfillRoster(left, right domain.BackfillTicket) bool {
	return left.SessionID == right.SessionID && left.RosterVersion == right.RosterVersion &&
		reflect.DeepEqual(left.OpenSlotsByTeam, right.OpenSlotsByTeam) &&
		reflect.DeepEqual(left.ExistingTeams, right.ExistingTeams)
}

func validateBackfillSessionClaim(
	snapshot repository.Snapshot,
	scope string,
	sessionID domain.SessionID,
	ticketID domain.TicketID,
) error {
	resource, exists := findResource(snapshot, Key(scope, ResourceBackfillSessionClaim, string(sessionID)))
	if !exists || resource.Deleted {
		return fmt.Errorf("backfill session claim %q is missing", sessionID)
	}
	claim, err := decodeBackfillSessionClaim(resource.Payload)
	if err != nil {
		return err
	}
	if claim.SessionID != sessionID || claim.TicketID != ticketID {
		return fmt.Errorf("backfill session claim %q belongs to another ticket", sessionID)
	}
	return nil
}

func mapBackfillConflict(err error, ticket domain.BackfillTicket) error {
	var conflict *repository.Conflict
	if !errors.As(err, &conflict) {
		return err
	}
	if conflict.Key.Kind == string(ResourceBackfillSessionClaim) {
		return domain.NewFailure(
			domain.FailureInvalidInput,
			"session %q already has an active backfill ticket",
			ticket.SessionID,
		)
	}
	return domain.NewFailure(
		domain.FailureInvalidRevision,
		"backfill ticket %q changed while the command was being committed",
		ticket.ID,
	)
}

type persistedBackfillTicket struct {
	Schema          string                       `json:"schema"`
	ID              domain.TicketID              `json:"id"`
	Revision        domain.Revision              `json:"revision"`
	SessionID       domain.SessionID             `json:"session_id"`
	RosterVersion   domain.Revision              `json:"roster_version"`
	OpenSlotsByTeam []int                        `json:"open_slots_by_team"`
	ExistingTeams   []persistedRosterTeamSummary `json:"existing_teams,omitempty"`
	EnqueuedAt      time.Time                    `json:"enqueued_at"`
}

type persistedRosterTeamSummary struct {
	PlayerCount      int                  `json:"player_count"`
	SkillTotal       int                  `json:"skill_total"`
	RoleCounts       []persistedRoleCount `json:"role_counts,omitempty"`
	MaxLatencyMillis int                  `json:"max_latency_millis"`
}

type persistedRoleCount struct {
	Role  string `json:"role"`
	Count int    `json:"count"`
}

func encodeBackfillTicket(ticket domain.BackfillTicket) ([]byte, error) {
	encoded, err := json.Marshal(toPersistedBackfillTicket(ticket))
	if err != nil {
		return nil, fmt.Errorf("encode backfill ticket resource: %w", err)
	}
	return encoded, nil
}

func decodeBackfillTicket(payload []byte) (domain.BackfillTicket, error) {
	var stored persistedBackfillTicket
	if err := decodeStrict(payload, &stored); err != nil {
		return domain.BackfillTicket{}, fmt.Errorf("decode backfill ticket resource: %w", err)
	}
	if stored.Schema != backfillTicketPayloadSchema {
		return domain.BackfillTicket{}, fmt.Errorf("unsupported backfill ticket resource schema %q", stored.Schema)
	}
	ticket := fromPersistedBackfillTicket(stored)
	if err := domain.ValidateBackfillTicket(ticket); err != nil {
		return domain.BackfillTicket{}, fmt.Errorf("stored backfill ticket is invalid: %w", err)
	}
	return ticket, nil
}

func toPersistedBackfillTicket(ticket domain.BackfillTicket) persistedBackfillTicket {
	teams := make([]persistedRosterTeamSummary, len(ticket.ExistingTeams))
	for teamIndex, team := range ticket.ExistingTeams {
		roles := make([]persistedRoleCount, len(team.RoleCounts))
		for roleIndex, role := range team.RoleCounts {
			roles[roleIndex] = persistedRoleCount(role)
		}
		teams[teamIndex] = persistedRosterTeamSummary{
			PlayerCount: team.PlayerCount, SkillTotal: team.SkillTotal,
			RoleCounts: roles, MaxLatencyMillis: team.MaxLatencyMillis,
		}
	}
	return persistedBackfillTicket{
		Schema: backfillTicketPayloadSchema, ID: ticket.ID, Revision: ticket.Revision,
		SessionID: ticket.SessionID, RosterVersion: ticket.RosterVersion,
		OpenSlotsByTeam: slices.Clone(ticket.OpenSlotsByTeam), ExistingTeams: teams,
		EnqueuedAt: ticket.EnqueuedAt.UTC(),
	}
}

func fromPersistedBackfillTicket(stored persistedBackfillTicket) domain.BackfillTicket {
	teams := make([]domain.RosterTeamSummary, len(stored.ExistingTeams))
	for teamIndex, team := range stored.ExistingTeams {
		roles := make([]domain.RoleCount, len(team.RoleCounts))
		for roleIndex, role := range team.RoleCounts {
			roles[roleIndex] = domain.RoleCount(role)
		}
		teams[teamIndex] = domain.RosterTeamSummary{
			PlayerCount: team.PlayerCount, SkillTotal: team.SkillTotal,
			RoleCounts: roles, MaxLatencyMillis: team.MaxLatencyMillis,
		}
	}
	return domain.BackfillTicket{
		ID: stored.ID, Revision: stored.Revision, SessionID: stored.SessionID,
		RosterVersion: stored.RosterVersion, OpenSlotsByTeam: slices.Clone(stored.OpenSlotsByTeam),
		ExistingTeams: teams, EnqueuedAt: stored.EnqueuedAt.UTC(),
	}
}
