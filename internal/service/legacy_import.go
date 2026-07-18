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
	"github.com/zrma/sema/internal/durable"
	"github.com/zrma/sema/internal/engine"
	"github.com/zrma/sema/internal/repository"
)

const legacyImportPayloadSchema = "sema.legacy-import.v1"

type LegacyImportState string

const (
	LegacyImportInProgress LegacyImportState = "importing"
	LegacyImportCompleted  LegacyImportState = "completed"
)

var (
	ErrLegacyImportIncomplete = errors.New("legacy import is incomplete and the target scope must be discarded")
	ErrLegacyImportTargetUsed = errors.New("legacy import target scope is not empty")
)

type LegacyImportOptions struct {
	Now       func() time.Time
	BatchSize int
}

type LegacyImporter struct {
	repository repository.Repository
	now        func() time.Time
	batchSize  int
}

type LegacyImportStatus struct {
	ID                string
	State             LegacyImportState
	SourceSchema      string
	SourceDigest      string
	SourceRecords     int
	ReservationTTL    time.Duration
	StartedAt         time.Time
	CompletedAt       time.Time
	ImportedResources int
	StorageVersion    repository.Version
}

type LegacyImportResult struct {
	Status   LegacyImportStatus
	Replayed bool
}

type persistedLegacyImport struct {
	Schema                string            `json:"schema"`
	ID                    string            `json:"id"`
	State                 LegacyImportState `json:"state"`
	SourceSchema          string            `json:"source_schema"`
	SourceDigest          string            `json:"source_digest"`
	SourceRecords         int               `json:"source_records"`
	ReservationTTLNanos   int64             `json:"reservation_ttl_nanos"`
	StartedAt             time.Time         `json:"started_at"`
	CompletedAt           *time.Time        `json:"completed_at,omitempty"`
	ImportedResourceCount int               `json:"imported_resource_count"`
}

type legacyImportModel struct {
	runtime *engine.Engine

	policies        map[string]domain.MatchmakingPolicy
	matchTickets    map[domain.TicketID]domain.MatchTicket
	backfillTickets map[domain.TicketID]domain.BackfillTicket
	demandKinds     map[domain.TicketID]ResourceKind
	sessions        map[domain.SessionID]struct{}

	planningInputs map[string]domain.MatchmakingSnapshot
	planningRuns   map[string]PlanningRunRecord
	proposals      map[domain.ProposalID]ProposalRecord
	unmatched      map[string]UnmatchedRecord

	reservations      map[domain.ReservationID]domain.Reservation
	reservationClaims map[domain.TicketID]struct{}
	assignments       map[domain.AssignmentID]domain.Assignment
}

func NewLegacyImporter(owner repository.Repository, options LegacyImportOptions) (*LegacyImporter, error) {
	if owner == nil {
		return nil, domain.NewFailure(domain.FailureInvalidInput, "repository is required")
	}
	if options.BatchSize <= 0 {
		return nil, domain.NewFailure(domain.FailureInvalidInput, "legacy import batch size must be positive")
	}
	if options.Now == nil {
		options.Now = time.Now
	}
	return &LegacyImporter{repository: owner, now: options.Now, batchSize: options.BatchSize}, nil
}

func (importer *LegacyImporter) Import(
	ctx context.Context,
	scope string,
	importID string,
	path string,
) (LegacyImportResult, error) {
	if scope == "" || importID == "" {
		return LegacyImportResult{}, domain.NewFailure(
			domain.FailureInvalidInput, "legacy import scope and identity are required",
		)
	}
	source, err := durable.ReadLegacyJournal(path)
	if err != nil {
		return LegacyImportResult{}, err
	}
	model, err := buildLegacyImportModel(source)
	if err != nil {
		return LegacyImportResult{}, err
	}

	snapshot, err := importer.repository.Snapshot(ctx, scope)
	if err != nil {
		return LegacyImportResult{}, err
	}
	if len(snapshot.Resources) > 0 || snapshot.Version != 0 {
		status, exists, statusErr := legacyImportStatus(snapshot, scope, importID)
		if statusErr != nil {
			return LegacyImportResult{}, statusErr
		}
		if exists && status.SourceDigest == source.Digest && status.SourceSchema == source.Schema {
			if status.State == LegacyImportCompleted {
				return LegacyImportResult{Status: status, Replayed: true}, nil
			}
			return LegacyImportResult{}, ErrLegacyImportIncomplete
		}
		return LegacyImportResult{}, ErrLegacyImportTargetUsed
	}

	startedAt := importer.now().UTC()
	started := LegacyImportStatus{
		ID: importID, State: LegacyImportInProgress, SourceSchema: source.Schema,
		SourceDigest: source.Digest, SourceRecords: source.RecordCount,
		ReservationTTL: source.ReservationTTL, StartedAt: startedAt,
	}
	startedPayload, err := encodeLegacyImport(started)
	if err != nil {
		return LegacyImportResult{}, err
	}
	startOperation, err := legacyImportOperation(
		scope, domain.OperationID("/legacy.import/"+importID+"/start"),
		"legacy_import.start", startedAt,
		struct {
			ImportID     string `json:"import_id"`
			SourceDigest string `json:"source_digest"`
		}{ImportID: importID, SourceDigest: source.Digest},
	)
	if err != nil {
		return LegacyImportResult{}, err
	}
	startResult, err := importer.repository.Commit(ctx, startOperation, []repository.Mutation{{
		Key: Key(scope, ResourceLegacyImport, importID), Payload: startedPayload,
	}})
	if err != nil {
		if repository.IsConflict(err) {
			return LegacyImportResult{}, ErrLegacyImportTargetUsed
		}
		return LegacyImportResult{}, err
	}
	started.StorageVersion = startResult.Version

	mutations, err := model.mutations(scope, startResult.Version)
	if err != nil {
		return LegacyImportResult{}, err
	}
	for offset, batchIndex := 0, 0; offset < len(mutations); offset, batchIndex = offset+importer.batchSize, batchIndex+1 {
		end := min(offset+importer.batchSize, len(mutations))
		batch := mutations[offset:end]
		operation, err := legacyImportOperation(
			scope,
			domain.OperationID(fmt.Sprintf("/legacy.import/%s/batch/%06d", importID, batchIndex)),
			"legacy_import.batch", startedAt,
			struct {
				ImportID     string                `json:"import_id"`
				SourceDigest string                `json:"source_digest"`
				Batch        int                   `json:"batch"`
				Mutations    []repository.Mutation `json:"mutations"`
			}{ImportID: importID, SourceDigest: source.Digest, Batch: batchIndex, Mutations: batch},
		)
		if err != nil {
			return LegacyImportResult{}, err
		}
		if _, err := importer.repository.Commit(ctx, operation, batch); err != nil {
			return LegacyImportResult{}, fmt.Errorf("commit legacy import batch %d: %w", batchIndex, err)
		}
	}

	completedAt := importer.now().UTC()
	completed := started
	completed.State = LegacyImportCompleted
	completed.CompletedAt = completedAt
	completed.ImportedResources = len(mutations)
	completedPayload, err := encodeLegacyImport(completed)
	if err != nil {
		return LegacyImportResult{}, err
	}
	completeOperation, err := legacyImportOperation(
		scope, domain.OperationID("/legacy.import/"+importID+"/complete"),
		"legacy_import.complete", completedAt,
		struct {
			ImportID          string `json:"import_id"`
			SourceDigest      string `json:"source_digest"`
			ImportedResources int    `json:"imported_resources"`
		}{ImportID: importID, SourceDigest: source.Digest, ImportedResources: len(mutations)},
	)
	if err != nil {
		return LegacyImportResult{}, err
	}
	result, err := importer.repository.Commit(ctx, completeOperation, []repository.Mutation{{
		Key: Key(scope, ResourceLegacyImport, importID), ExpectedVersion: startResult.Version,
		Payload: completedPayload,
	}})
	if err != nil {
		return LegacyImportResult{}, fmt.Errorf("commit legacy import completion: %w", err)
	}
	completed.StorageVersion = result.Version
	return LegacyImportResult{Status: completed}, nil
}

func (importer *LegacyImporter) Status(
	ctx context.Context,
	scope string,
	importID string,
) (LegacyImportStatus, bool, error) {
	snapshot, err := importer.repository.Snapshot(ctx, scope)
	if err != nil {
		return LegacyImportStatus{}, false, err
	}
	return legacyImportStatus(snapshot, scope, importID)
}

func RequireLegacyImportCompleted(
	ctx context.Context,
	owner repository.Repository,
	scope string,
	importID string,
	sourceDigest string,
) (LegacyImportStatus, error) {
	if owner == nil || scope == "" || importID == "" || sourceDigest == "" {
		return LegacyImportStatus{}, domain.NewFailure(
			domain.FailureInvalidInput, "legacy import completion identity is required",
		)
	}
	snapshot, err := owner.Snapshot(ctx, scope)
	if err != nil {
		return LegacyImportStatus{}, err
	}
	status, exists, err := legacyImportStatus(snapshot, scope, importID)
	if err != nil {
		return LegacyImportStatus{}, err
	}
	if !exists || status.State != LegacyImportCompleted || status.SourceDigest != sourceDigest {
		return LegacyImportStatus{}, ErrLegacyImportIncomplete
	}
	return status, nil
}

func buildLegacyImportModel(source durable.LegacyJournal) (*legacyImportModel, error) {
	runtime, err := engine.New(source.ReservationTTL)
	if err != nil {
		return nil, err
	}
	model := &legacyImportModel{
		runtime:         runtime,
		policies:        make(map[string]domain.MatchmakingPolicy),
		matchTickets:    make(map[domain.TicketID]domain.MatchTicket),
		backfillTickets: make(map[domain.TicketID]domain.BackfillTicket),
		demandKinds:     make(map[domain.TicketID]ResourceKind), sessions: make(map[domain.SessionID]struct{}),
		planningInputs: make(map[string]domain.MatchmakingSnapshot), planningRuns: make(map[string]PlanningRunRecord),
		proposals: make(map[domain.ProposalID]ProposalRecord), unmatched: make(map[string]UnmatchedRecord),
		reservations:      make(map[domain.ReservationID]domain.Reservation),
		reservationClaims: make(map[domain.TicketID]struct{}),
		assignments:       make(map[domain.AssignmentID]domain.Assignment),
	}
	for _, event := range source.Events {
		if err := model.apply(event); err != nil {
			return nil, fmt.Errorf("normalize legacy event %d (%s): %w", event.Sequence, event.Kind, err)
		}
	}
	return model, nil
}

func (model *legacyImportModel) apply(event durable.LegacyEvent) error {
	switch event.Kind {
	case durable.LegacyRuntimeConfigured:
		return nil
	case durable.LegacyPolicyRegistered:
		policy := domain.ClonePolicy(*event.Policy)
		if _, err := model.runtime.RegisterPolicy(policy); err != nil {
			return err
		}
		model.policies[policy.Version] = policy
	case durable.LegacyMatchTicketSubmitted:
		ticket := domain.CloneMatchTicket(*event.MatchTicket)
		if err := model.claimDemandKind(ticket.ID, ResourceMatchTicket); err != nil {
			return err
		}
		if err := model.runtime.SubmitMatchTicket(ticket); err != nil {
			return err
		}
		model.matchTickets[ticket.ID] = ticket
	case durable.LegacyBackfillSubmitted:
		ticket := domain.CloneBackfillTicket(*event.BackfillTicket)
		if err := model.claimDemandKind(ticket.ID, ResourceBackfillTicket); err != nil {
			return err
		}
		if err := model.runtime.SubmitBackfillTicket(ticket); err != nil {
			return err
		}
		model.backfillTickets[ticket.ID] = ticket
		model.sessions[ticket.SessionID] = struct{}{}
	case durable.LegacyMatchTicketCancelled:
		cancel := event.MatchTicketCancellation
		if err := model.runtime.CancelMatchTicket(cancel.TicketID, cancel.Revision); err != nil {
			return err
		}
		delete(model.matchTickets, cancel.TicketID)
	case durable.LegacyBackfillCancelled:
		cancel := event.BackfillCancellation
		if err := model.runtime.CancelBackfillTicket(cancel.TicketID, cancel.Revision, cancel.RosterVersion); err != nil {
			return err
		}
		delete(model.backfillTickets, cancel.TicketID)
	case durable.LegacyPlanCompleted:
		plan := event.Plan
		model.expireReservations(plan.Now)
		input, err := model.runtime.Snapshot(plan.SnapshotID, plan.Now, plan.PolicyVersion)
		if err != nil {
			return err
		}
		fingerprint, err := domain.FingerprintPolicy(input.Policy)
		if err != nil {
			return err
		}
		runID := string(plan.SnapshotID)
		run := PlanningRunRecord{
			ID: runID, SnapshotID: plan.SnapshotID, PolicyVersion: plan.PolicyVersion,
			PolicyFingerprint: fingerprint, SourceRepositoryVersion: 1,
			CapturedAt: plan.Now.UTC(), CompletedAt: plan.Now.UTC(), Status: PlanningRunCompleted,
			ProposalCount: len(plan.Batch.Proposals), UnmatchedCount: len(plan.Batch.Unmatched),
			BudgetExhausted: plan.Batch.BudgetExhausted, Evidence: plan.Batch.Evidence,
		}
		if err := validatePlanningBatch(input, run, plan.Batch); err != nil {
			return err
		}
		model.planningInputs[runID] = cloneSnapshot(input)
		model.planningRuns[runID] = run
		for _, proposal := range plan.Batch.Proposals {
			if current, exists := model.proposals[proposal.ID]; exists &&
				!reflect.DeepEqual(current.Proposal, proposal) {
				return fmt.Errorf("proposal %q has conflicting legacy content", proposal.ID)
			}
			model.proposals[proposal.ID] = ProposalRecord{RunID: runID, Proposal: domain.CloneProposal(proposal)}
		}
		for _, unmatched := range plan.Batch.Unmatched {
			id := unmatchedResourceID(runID, unmatched.Ticket.ID)
			model.unmatched[id] = UnmatchedRecord{RunID: runID, Unmatched: unmatched}
		}
	case durable.LegacyProposalReserved:
		reservationEvent := event.Reservation
		model.expireReservations(reservationEvent.Now)
		current, exists := model.proposals[reservationEvent.Proposal.ID]
		if !exists {
			return fmt.Errorf("reserved proposal %q has no durable plan event", reservationEvent.Proposal.ID)
		}
		if !reflect.DeepEqual(current.Proposal, reservationEvent.Proposal) {
			return fmt.Errorf("reserved proposal %q differs from planned content", reservationEvent.Proposal.ID)
		}
		reservation, err := model.runtime.Reserve(
			reservationEvent.Proposal, reservationEvent.ReservationID, reservationEvent.Now,
		)
		if err != nil {
			return err
		}
		model.reservations[reservation.ID] = reservation
		for _, reference := range reservation.Tickets {
			model.reservationClaims[reference.ID] = struct{}{}
		}
		if reservation.Backfill != nil {
			model.reservationClaims[reservation.Backfill.Ticket.ID] = struct{}{}
		}
	case durable.LegacyReservationConfirmed:
		confirmation := event.Confirmation
		model.expireReservations(confirmation.Now)
		assignment, err := model.runtime.Confirm(
			confirmation.ReservationID, confirmation.AssignmentID, confirmation.Now,
		)
		if err != nil {
			return err
		}
		reservation := model.reservations[confirmation.ReservationID]
		reservation.Status = domain.ReservationConfirmed
		model.reservations[confirmation.ReservationID] = reservation
		for _, team := range assignment.Teams {
			for _, reference := range team.Tickets {
				delete(model.matchTickets, reference.ID)
			}
		}
		if assignment.Backfill != nil {
			delete(model.backfillTickets, assignment.Backfill.Ticket.ID)
		}
		model.assignments[assignment.ID] = assignment
	case durable.LegacyReservationCancelled:
		cancellation := event.ReservationCancellation
		model.expireReservations(cancellation.Now)
		reservation, err := model.runtime.CancelReservation(cancellation.ReservationID, cancellation.Now)
		if err != nil {
			return err
		}
		model.reservations[reservation.ID] = reservation
	case durable.LegacyAssignmentAcknowledged:
		acknowledgment := event.Acknowledgment
		assignment, err := model.runtime.AcknowledgeAssignment(
			acknowledgment.AssignmentID, acknowledgment.Request, acknowledgment.Now,
		)
		if err != nil {
			return err
		}
		model.assignments[assignment.ID] = assignment
	default:
		return fmt.Errorf("unsupported legacy event kind %q", event.Kind)
	}
	return nil
}

func (model *legacyImportModel) claimDemandKind(id domain.TicketID, kind ResourceKind) error {
	if current, exists := model.demandKinds[id]; exists && current != kind {
		return fmt.Errorf("legacy ticket %q changes demand kind", id)
	}
	model.demandKinds[id] = kind
	return nil
}

func (model *legacyImportModel) expireReservations(now time.Time) {
	for id, reservation := range model.reservations {
		if reservation.Status == domain.ReservationActive && !now.Before(reservation.ExpiresAt) {
			reservation.Status = domain.ReservationExpired
			model.reservations[id] = reservation
		}
	}
}

func (model *legacyImportModel) mutations(
	scope string,
	importVersion repository.Version,
) ([]repository.Mutation, error) {
	mutations := make([]repository.Mutation, 0)
	for _, version := range sortedMapKeys(model.policies) {
		policy := model.policies[version]
		fingerprint, err := domain.FingerprintPolicy(policy)
		if err != nil {
			return nil, err
		}
		payload, err := encodePolicy(canonicalPolicy(policy), fingerprint)
		if err != nil {
			return nil, err
		}
		mutations = append(mutations, repository.Mutation{Key: Key(scope, ResourcePolicy, version), Payload: payload})
	}
	for _, id := range sortedMapKeys(model.demandKinds) {
		kind := model.demandKinds[id]
		claimPayload, err := encodeDemandIdentityClaim(kind, string(id))
		if err != nil {
			return nil, err
		}
		mutations = append(mutations, repository.Mutation{
			Key: Key(scope, ResourceDemandIdentity, string(id)), Payload: claimPayload,
		})
		switch kind {
		case ResourceMatchTicket:
			if ticket, exists := model.matchTickets[id]; exists {
				payload, err := encodeMatchTicket(ticket)
				if err != nil {
					return nil, err
				}
				mutations = append(mutations, repository.Mutation{
					Key: Key(scope, kind, string(id)), Payload: payload,
				})
			} else {
				mutations = append(mutations, repository.Mutation{Key: Key(scope, kind, string(id)), Delete: true})
			}
		case ResourceBackfillTicket:
			if ticket, exists := model.backfillTickets[id]; exists {
				payload, err := encodeBackfillTicket(ticket)
				if err != nil {
					return nil, err
				}
				mutations = append(mutations, repository.Mutation{
					Key: Key(scope, kind, string(id)), Payload: payload,
				})
			} else {
				mutations = append(mutations, repository.Mutation{Key: Key(scope, kind, string(id)), Delete: true})
			}
		}
	}
	for _, sessionID := range sortedMapKeys(model.sessions) {
		var active *domain.BackfillTicket
		for id := range model.backfillTickets {
			ticket := model.backfillTickets[id]
			if ticket.SessionID == sessionID {
				cloned := domain.CloneBackfillTicket(ticket)
				active = &cloned
				break
			}
		}
		mutation := repository.Mutation{Key: Key(scope, ResourceBackfillSessionClaim, string(sessionID))}
		if active == nil {
			mutation.Delete = true
		} else {
			payload, err := encodeBackfillSessionClaim(*active)
			if err != nil {
				return nil, err
			}
			mutation.Payload = payload
		}
		mutations = append(mutations, mutation)
	}
	for _, runID := range sortedMapKeys(model.planningRuns) {
		run := model.planningRuns[runID]
		run.SourceRepositoryVersion = importVersion
		runPayload, err := encodePlanningRun(run)
		if err != nil {
			return nil, err
		}
		planningSnapshot, err := NewPlanningSnapshot(importVersion, model.planningInputs[runID])
		if err != nil {
			return nil, err
		}
		snapshotPayload, err := encodePlanningSnapshot(planningSnapshot)
		if err != nil {
			return nil, err
		}
		mutations = append(mutations,
			repository.Mutation{Key: Key(scope, ResourcePlanningRun, runID), Payload: runPayload},
			repository.Mutation{Key: Key(scope, ResourcePlanningSnapshot, runID), Payload: snapshotPayload},
		)
	}
	for _, proposalID := range sortedMapKeys(model.proposals) {
		record := model.proposals[proposalID]
		payload, err := encodeProposal(record.RunID, record.Proposal)
		if err != nil {
			return nil, err
		}
		mutations = append(mutations, repository.Mutation{
			Key: Key(scope, ResourceProposal, string(proposalID)), Payload: payload,
		})
	}
	for _, id := range sortedMapKeys(model.unmatched) {
		record := model.unmatched[id]
		payload, err := encodeUnmatched(record.RunID, record.Unmatched)
		if err != nil {
			return nil, err
		}
		mutations = append(mutations, repository.Mutation{
			Key: Key(scope, ResourcePlanningUnmatched, id), Payload: payload,
		})
	}
	for _, reservationID := range sortedMapKeys(model.reservations) {
		reservation := model.reservations[reservationID]
		payload, err := encodeReservation(reservation)
		if err != nil {
			return nil, err
		}
		mutations = append(mutations, repository.Mutation{
			Key: Key(scope, ResourceReservation, string(reservationID)), Payload: payload,
		})
	}
	activeClaims := make(map[domain.TicketID]persistedReservationClaim)
	for _, reservation := range model.reservations {
		if reservation.Status != domain.ReservationActive {
			continue
		}
		proposal := model.proposals[reservation.ProposalID].Proposal
		for _, reference := range reservation.Tickets {
			activeClaims[reference.ID] = persistedReservationClaim{
				ReservationID: reservation.ID, ProposalID: reservation.ProposalID,
				Ticket: toPersistedTicketRef(reference), Kind: proposal.Kind,
			}
		}
		if reservation.Backfill != nil {
			reference := reservation.Backfill.Ticket
			activeClaims[reference.ID] = persistedReservationClaim{
				ReservationID: reservation.ID, ProposalID: reservation.ProposalID,
				Ticket: toPersistedTicketRef(reference), Kind: proposal.Kind,
			}
		}
	}
	for _, ticketID := range sortedMapKeys(model.reservationClaims) {
		mutation := repository.Mutation{Key: Key(scope, ResourceDemandReservation, string(ticketID))}
		if claim, exists := activeClaims[ticketID]; exists {
			reservation := model.reservations[claim.ReservationID]
			payload, err := encodeReservationClaim(
				reservation,
				domain.TicketRef{ID: claim.Ticket.ID, Revision: claim.Ticket.Revision},
				claim.Kind,
			)
			if err != nil {
				return nil, err
			}
			mutation.Payload = payload
		} else {
			mutation.Delete = true
		}
		mutations = append(mutations, mutation)
	}
	for _, assignmentID := range sortedMapKeys(model.assignments) {
		assignment := model.assignments[assignmentID]
		payload, err := encodeAssignment(assignment)
		if err != nil {
			return nil, err
		}
		mutations = append(mutations, repository.Mutation{
			Key: Key(scope, ResourceAssignment, string(assignmentID)), Payload: payload,
		})
		if assignment.Acknowledgment != nil {
			acknowledgmentPayload, err := encodeAcknowledgmentResource(assignment)
			if err != nil {
				return nil, err
			}
			mutations = append(mutations, repository.Mutation{
				Key: Key(scope, ResourceAcknowledgment, string(assignmentID)), Payload: acknowledgmentPayload,
			})
		}
	}
	slices.SortFunc(mutations, func(left, right repository.Mutation) int {
		if left.Key.Kind != right.Key.Kind {
			if left.Key.Kind < right.Key.Kind {
				return -1
			}
			return 1
		}
		if left.Key.ID < right.Key.ID {
			return -1
		}
		if left.Key.ID > right.Key.ID {
			return 1
		}
		return 0
	})
	return mutations, nil
}

func legacyImportOperation(
	scope string,
	id domain.OperationID,
	kind string,
	at time.Time,
	command any,
) (repository.Operation, error) {
	payload, err := json.Marshal(command)
	if err != nil {
		return repository.Operation{}, fmt.Errorf("encode %s command: %w", kind, err)
	}
	operation := repository.Operation{
		Scope: scope, ID: id, Kind: kind, Digest: repository.Digest(payload), At: at,
	}
	if err := repository.ValidateOperation(operation); err != nil {
		return repository.Operation{}, err
	}
	return operation, nil
}

func encodeLegacyImport(status LegacyImportStatus) ([]byte, error) {
	if status.ID == "" || status.SourceSchema == "" || status.SourceDigest == "" ||
		status.SourceRecords <= 0 || status.ReservationTTL <= 0 || status.StartedAt.IsZero() {
		return nil, fmt.Errorf("legacy import status is invalid")
	}
	if status.State != LegacyImportInProgress && status.State != LegacyImportCompleted {
		return nil, fmt.Errorf("legacy import state is invalid")
	}
	if status.State == LegacyImportCompleted && status.CompletedAt.IsZero() {
		return nil, fmt.Errorf("completed legacy import has no completion time")
	}
	completedAt := optionalTime(status.CompletedAt)
	encoded, err := json.Marshal(persistedLegacyImport{
		Schema: legacyImportPayloadSchema, ID: status.ID, State: status.State,
		SourceSchema: status.SourceSchema, SourceDigest: status.SourceDigest,
		SourceRecords: status.SourceRecords, ReservationTTLNanos: int64(status.ReservationTTL),
		StartedAt: status.StartedAt.UTC(), CompletedAt: completedAt,
		ImportedResourceCount: status.ImportedResources,
	})
	if err != nil {
		return nil, fmt.Errorf("encode legacy import status: %w", err)
	}
	return encoded, nil
}

func decodeLegacyImport(payload []byte) (LegacyImportStatus, error) {
	var stored persistedLegacyImport
	if err := decodeStrict(payload, &stored); err != nil {
		return LegacyImportStatus{}, fmt.Errorf("decode legacy import status: %w", err)
	}
	status := LegacyImportStatus{
		ID: stored.ID, State: stored.State, SourceSchema: stored.SourceSchema,
		SourceDigest: stored.SourceDigest, SourceRecords: stored.SourceRecords,
		ReservationTTL: time.Duration(stored.ReservationTTLNanos), StartedAt: stored.StartedAt.UTC(),
		CompletedAt: timeValue(stored.CompletedAt), ImportedResources: stored.ImportedResourceCount,
	}
	if _, err := encodeLegacyImport(status); err != nil {
		return LegacyImportStatus{}, fmt.Errorf("stored legacy import status is invalid: %w", err)
	}
	return status, nil
}

func legacyImportStatus(
	snapshot repository.Snapshot,
	scope string,
	importID string,
) (LegacyImportStatus, bool, error) {
	resource, exists := findResource(snapshot, Key(scope, ResourceLegacyImport, importID))
	if !exists || resource.Deleted {
		return LegacyImportStatus{}, false, nil
	}
	status, err := decodeLegacyImport(resource.Payload)
	if err != nil {
		return LegacyImportStatus{}, false, err
	}
	status.StorageVersion = resource.Version
	return status, true, nil
}

func sortedMapKeys[K ~string, V any](values map[K]V) []K {
	keys := make([]K, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	return keys
}
