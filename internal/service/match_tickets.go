package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"
	"time"

	"github.com/zrma/sema/internal/domain"
	"github.com/zrma/sema/internal/repository"
)

const matchTicketPayloadSchema = "sema.match-ticket.v1"

type MatchTicketRecord struct {
	Ticket         domain.MatchTicket
	StorageVersion repository.Version
	Deleted        bool
}

type MatchTicketSnapshot struct {
	RepositoryVersion repository.Version
	Records           []MatchTicketRecord
}

type MatchTicketMutation struct {
	Ticket         domain.MatchTicket
	StorageVersion repository.Version
	Replayed       bool
}

type MatchTicketCancellation struct {
	ID             domain.TicketID
	Revision       domain.Revision
	StorageVersion repository.Version
	Replayed       bool
}

// MatchTickets owns target match-ticket command semantics above any repository
// adapter. Tenant scope and operation identity are required on every mutation.
type MatchTickets struct {
	repository repository.Repository
	now        func() time.Time
}

func NewMatchTickets(owner repository.Repository, now func() time.Time) (*MatchTickets, error) {
	if owner == nil {
		return nil, domain.NewFailure(domain.FailureInvalidInput, "repository is required")
	}
	if now == nil {
		now = time.Now
	}
	return &MatchTickets{repository: owner, now: now}, nil
}

func (service *MatchTickets) Put(
	ctx context.Context,
	scope string,
	operationID domain.OperationID,
	ticket domain.MatchTicket,
) (MatchTicketMutation, error) {
	ticket.EnqueuedAt = ticket.EnqueuedAt.UTC()
	if err := domain.ValidateMatchTicket(ticket); err != nil {
		return MatchTicketMutation{}, err
	}
	payload, err := encodeMatchTicket(ticket)
	if err != nil {
		return MatchTicketMutation{}, err
	}
	operation, err := service.operation(scope, operationID, "match_ticket.put", append([]byte("put\x00"), payload...))
	if err != nil {
		return MatchTicketMutation{}, err
	}
	if replayed, exists, err := service.repository.Replay(ctx, operation); err != nil {
		return MatchTicketMutation{}, err
	} else if exists {
		return MatchTicketMutation{
			Ticket: ticket, StorageVersion: replayed.Version, Replayed: true,
		}, nil
	}

	snapshot, err := service.repository.Snapshot(ctx, scope)
	if err != nil {
		return MatchTicketMutation{}, err
	}
	key := Key(scope, ResourceMatchTicket, string(ticket.ID))
	expected := repository.Version(0)
	if current, exists := findResource(snapshot, key); exists {
		if current.Deleted {
			return MatchTicketMutation{}, domain.NewFailure(
				domain.FailureInvalidRevision,
				"match ticket %q is cancelled and its identity cannot be reused",
				ticket.ID,
			)
		}
		stored, err := decodeMatchTicket(current.Payload)
		if err != nil {
			return MatchTicketMutation{}, err
		}
		if ticket.Revision <= stored.Revision {
			return MatchTicketMutation{}, domain.NewFailure(
				domain.FailureInvalidRevision,
				"match ticket %q revision %d must be higher than current revision %d",
				ticket.ID,
				ticket.Revision,
				stored.Revision,
			)
		}
		expected = current.Version
	}
	result, err := service.repository.Commit(ctx, operation, []repository.Mutation{{
		Key: key, ExpectedVersion: expected, Payload: payload,
	}})
	if err != nil {
		if repository.IsConflict(err) {
			return MatchTicketMutation{}, domain.NewFailure(
				domain.FailureInvalidRevision,
				"match ticket %q changed while the command was being committed",
				ticket.ID,
			)
		}
		return MatchTicketMutation{}, err
	}
	return MatchTicketMutation{
		Ticket: ticket, StorageVersion: result.Version, Replayed: result.Replayed,
	}, nil
}

func (service *MatchTickets) Cancel(
	ctx context.Context,
	scope string,
	operationID domain.OperationID,
	id domain.TicketID,
	revision domain.Revision,
) (MatchTicketCancellation, error) {
	command, err := json.Marshal(struct {
		Kind     string          `json:"kind"`
		ID       domain.TicketID `json:"id"`
		Revision domain.Revision `json:"revision"`
	}{Kind: "match_ticket.cancel", ID: id, Revision: revision})
	if err != nil {
		return MatchTicketCancellation{}, fmt.Errorf("encode match ticket cancellation: %w", err)
	}
	operation, err := service.operation(scope, operationID, "match_ticket.cancel", command)
	if err != nil {
		return MatchTicketCancellation{}, err
	}
	if id == "" || revision == 0 {
		return MatchTicketCancellation{}, domain.NewFailure(
			domain.FailureInvalidInput,
			"match ticket identity and exact revision are required",
		)
	}
	if replayed, exists, err := service.repository.Replay(ctx, operation); err != nil {
		return MatchTicketCancellation{}, err
	} else if exists {
		return MatchTicketCancellation{
			ID: id, Revision: revision, StorageVersion: replayed.Version, Replayed: true,
		}, nil
	}

	snapshot, err := service.repository.Snapshot(ctx, scope)
	if err != nil {
		return MatchTicketCancellation{}, err
	}
	key := Key(scope, ResourceMatchTicket, string(id))
	current, exists := findResource(snapshot, key)
	if !exists || current.Deleted {
		return MatchTicketCancellation{}, ErrResourceNotFound
	}
	stored, err := decodeMatchTicket(current.Payload)
	if err != nil {
		return MatchTicketCancellation{}, err
	}
	if stored.Revision != revision {
		return MatchTicketCancellation{}, domain.NewFailure(
			domain.FailureInvalidRevision,
			"match ticket %q is at revision %d; expected %d",
			id,
			stored.Revision,
			revision,
		)
	}
	result, err := service.repository.Commit(ctx, operation, []repository.Mutation{{
		Key: key, ExpectedVersion: current.Version, Delete: true,
	}})
	if err != nil {
		if repository.IsConflict(err) {
			return MatchTicketCancellation{}, domain.NewFailure(
				domain.FailureInvalidRevision,
				"match ticket %q changed while the cancellation was being committed",
				id,
			)
		}
		return MatchTicketCancellation{}, err
	}
	return MatchTicketCancellation{
		ID: id, Revision: revision, StorageVersion: result.Version, Replayed: result.Replayed,
	}, nil
}

func (service *MatchTickets) Get(
	ctx context.Context,
	scope string,
	id domain.TicketID,
) (MatchTicketRecord, bool, error) {
	snapshot, err := service.repository.Snapshot(ctx, scope)
	if err != nil {
		return MatchTicketRecord{}, false, err
	}
	resource, exists := findResource(snapshot, Key(scope, ResourceMatchTicket, string(id)))
	if !exists || resource.Deleted {
		return MatchTicketRecord{}, false, nil
	}
	ticket, err := decodeMatchTicket(resource.Payload)
	if err != nil {
		return MatchTicketRecord{}, false, err
	}
	return MatchTicketRecord{Ticket: ticket, StorageVersion: resource.Version}, true, nil
}

func (service *MatchTickets) Snapshot(
	ctx context.Context,
	scope string,
) (MatchTicketSnapshot, error) {
	snapshot, err := service.repository.Snapshot(ctx, scope)
	if err != nil {
		return MatchTicketSnapshot{}, err
	}
	records := make([]MatchTicketRecord, 0)
	for _, resource := range snapshot.Resources {
		if resource.Key.Kind != string(ResourceMatchTicket) {
			continue
		}
		record := MatchTicketRecord{
			Ticket:         domain.MatchTicket{ID: domain.TicketID(resource.Key.ID)},
			StorageVersion: resource.Version,
			Deleted:        resource.Deleted,
		}
		if !resource.Deleted {
			record.Ticket, err = decodeMatchTicket(resource.Payload)
			if err != nil {
				return MatchTicketSnapshot{}, err
			}
		}
		records = append(records, record)
	}
	slices.SortFunc(records, func(left, right MatchTicketRecord) int {
		if left.Ticket.ID < right.Ticket.ID {
			return -1
		}
		if left.Ticket.ID > right.Ticket.ID {
			return 1
		}
		return 0
	})
	return MatchTicketSnapshot{RepositoryVersion: snapshot.Version, Records: records}, nil
}

func (service *MatchTickets) operation(
	scope string,
	id domain.OperationID,
	kind string,
	canonical []byte,
) (repository.Operation, error) {
	now := service.now().UTC()
	operation := repository.Operation{
		Scope: scope, ID: id, Kind: kind, Digest: repository.Digest(canonical), At: now,
	}
	if err := repository.ValidateOperation(operation); err != nil {
		return repository.Operation{}, err
	}
	return operation, nil
}

type persistedMatchTicket struct {
	Schema     string            `json:"schema"`
	ID         domain.TicketID   `json:"id"`
	Revision   domain.Revision   `json:"revision"`
	EnqueuedAt time.Time         `json:"enqueued_at"`
	Players    []persistedPlayer `json:"players"`
}

type persistedPlayer struct {
	ID            domain.PlayerID `json:"id"`
	Skill         int             `json:"skill"`
	Role          string          `json:"role,omitempty"`
	LatencyMillis int             `json:"latency_millis"`
}

func encodeMatchTicket(ticket domain.MatchTicket) ([]byte, error) {
	players := make([]persistedPlayer, len(ticket.Players))
	for index, player := range ticket.Players {
		players[index] = persistedPlayer(player)
	}
	encoded, err := json.Marshal(persistedMatchTicket{
		Schema: matchTicketPayloadSchema, ID: ticket.ID, Revision: ticket.Revision,
		EnqueuedAt: ticket.EnqueuedAt.UTC(), Players: players,
	})
	if err != nil {
		return nil, fmt.Errorf("encode match ticket resource: %w", err)
	}
	return encoded, nil
}

func decodeMatchTicket(payload []byte) (domain.MatchTicket, error) {
	var stored persistedMatchTicket
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&stored); err != nil {
		return domain.MatchTicket{}, fmt.Errorf("decode match ticket resource: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return domain.MatchTicket{}, fmt.Errorf("decode match ticket resource: trailing data")
	}
	if stored.Schema != matchTicketPayloadSchema {
		return domain.MatchTicket{}, fmt.Errorf("unsupported match ticket resource schema %q", stored.Schema)
	}
	players := make([]domain.Player, len(stored.Players))
	for index, player := range stored.Players {
		players[index] = domain.Player(player)
	}
	ticket := domain.MatchTicket{
		ID: stored.ID, Revision: stored.Revision, EnqueuedAt: stored.EnqueuedAt.UTC(), Players: players,
	}
	if err := domain.ValidateMatchTicket(ticket); err != nil {
		return domain.MatchTicket{}, fmt.Errorf("stored match ticket is invalid: %w", err)
	}
	return ticket, nil
}

func findResource(snapshot repository.Snapshot, key repository.Key) (repository.Resource, bool) {
	for _, resource := range snapshot.Resources {
		if resource.Key == key {
			return resource, true
		}
	}
	return repository.Resource{}, false
}

var ErrResourceNotFound = errors.New("service resource not found")
