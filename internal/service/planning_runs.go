package service

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"time"

	"github.com/zrma/sema/internal/domain"
	"github.com/zrma/sema/internal/planner"
	"github.com/zrma/sema/internal/repository"
)

const maxPlanningCompletionAttempts = 8

type PlanningRunStatus string

const (
	PlanningRunPlanning  PlanningRunStatus = "planning"
	PlanningRunCompleted PlanningRunStatus = "completed"
)

type PlanningRunRecord struct {
	ID                      string
	SnapshotID              domain.SnapshotID
	PolicyVersion           string
	PolicyFingerprint       domain.PolicyFingerprint
	SourceRepositoryVersion repository.Version
	CapturedAt              time.Time
	CompletedAt             time.Time
	Status                  PlanningRunStatus
	ProposalCount           int
	UnmatchedCount          int
	BudgetExhausted         bool
	Evidence                domain.BatchScoreEvidence
	StorageVersion          repository.Version
}

type PlanningRunMutation struct {
	Run      PlanningRunRecord
	Replayed bool
}

type ProposalRecord struct {
	RunID          string
	Proposal       domain.MatchProposal
	StorageVersion repository.Version
}

type ProposalSnapshot struct {
	RunStorageVersion repository.Version
	Records           []ProposalRecord
}

type UnmatchedRecord struct {
	RunID          string
	Unmatched      domain.UnmatchedTicket
	StorageVersion repository.Version
}

type UnmatchedSnapshot struct {
	RunStorageVersion repository.Version
	Records           []UnmatchedRecord
}

type PlanFunc func(domain.MatchmakingSnapshot) (domain.ProposalBatch, error)

// PlanningRuns captures an immutable repository snapshot, releases the
// storage transaction while the matcher runs, and atomically records the
// completed run plus all proposal and unmatched resources.
type PlanningRuns struct {
	repository repository.Repository
	now        func() time.Time
	plan       PlanFunc
}

func NewPlanningRuns(owner repository.Repository, now func() time.Time, plan PlanFunc) (*PlanningRuns, error) {
	if owner == nil {
		return nil, domain.NewFailure(domain.FailureInvalidInput, "repository is required")
	}
	if now == nil {
		now = time.Now
	}
	if plan == nil {
		plan = planner.Plan
	}
	return &PlanningRuns{repository: owner, now: now, plan: plan}, nil
}

func (service *PlanningRuns) Execute(
	ctx context.Context,
	scope string,
	operationID domain.OperationID,
	runID string,
	policyVersion string,
) (PlanningRunMutation, error) {
	command, err := json.Marshal(struct {
		Kind          string `json:"kind"`
		RunID         string `json:"run_id"`
		PolicyVersion string `json:"policy_version"`
	}{Kind: "planning_run.execute", RunID: runID, PolicyVersion: policyVersion})
	if err != nil {
		return PlanningRunMutation{}, fmt.Errorf("encode planning command: %w", err)
	}
	commandAt := service.now().UTC()
	operation := repository.Operation{
		Scope: scope, ID: operationID, Kind: "planning_run.capture",
		Digest: repository.Digest(command), At: commandAt,
	}
	if runID == "" || policyVersion == "" {
		return PlanningRunMutation{}, domain.NewFailure(
			domain.FailureInvalidInput,
			"planning run identity and policy version are required",
		)
	}
	if err := repository.ValidateOperation(operation); err != nil {
		return PlanningRunMutation{}, err
	}
	if _, exists, err := service.repository.Replay(ctx, operation); err != nil {
		return PlanningRunMutation{}, err
	} else if exists {
		return service.resume(ctx, scope, runID, true)
	}

	snapshot, err := service.repository.Snapshot(ctx, scope)
	if err != nil {
		return PlanningRunMutation{}, err
	}
	if _, exists := findResource(snapshot, Key(scope, ResourcePlanningRun, runID)); exists {
		return PlanningRunMutation{}, domain.NewFailure(
			domain.FailureInvalidRevision,
			"planning run %q is already registered",
			runID,
		)
	}
	input, fingerprint, err := planningInput(snapshot, scope, runID, policyVersion, commandAt)
	if err != nil {
		return PlanningRunMutation{}, err
	}
	planningSnapshot, err := NewPlanningSnapshot(snapshot.Version, input)
	if err != nil {
		return PlanningRunMutation{}, err
	}
	snapshotPayload, err := encodePlanningSnapshot(planningSnapshot)
	if err != nil {
		return PlanningRunMutation{}, err
	}
	run := PlanningRunRecord{
		ID: runID, SnapshotID: input.ID, PolicyVersion: policyVersion,
		PolicyFingerprint: fingerprint, SourceRepositoryVersion: snapshot.Version,
		CapturedAt: commandAt, Status: PlanningRunPlanning,
	}
	runPayload, err := encodePlanningRun(run)
	if err != nil {
		return PlanningRunMutation{}, err
	}
	result, err := service.repository.Commit(ctx, operation, []repository.Mutation{
		{Key: Key(scope, ResourcePlanningRun, runID), Payload: runPayload},
		{Key: Key(scope, ResourcePlanningSnapshot, runID), Payload: snapshotPayload},
	})
	if err != nil {
		if repository.IsConflict(err) {
			return PlanningRunMutation{}, domain.NewFailure(
				domain.FailureInvalidRevision,
				"planning run %q was registered concurrently",
				runID,
			)
		}
		return PlanningRunMutation{}, err
	}
	return service.resume(ctx, scope, runID, result.Replayed)
}

func (service *PlanningRuns) resume(
	ctx context.Context,
	scope string,
	runID string,
	replayed bool,
) (PlanningRunMutation, error) {
	run, planningSnapshot, err := service.load(ctx, scope, runID)
	if err != nil {
		return PlanningRunMutation{}, err
	}
	if run.Status == PlanningRunCompleted {
		return PlanningRunMutation{Run: run, Replayed: replayed}, nil
	}
	batch, err := service.plan(planningSnapshot.MatchmakingInput())
	if err != nil {
		return PlanningRunMutation{}, err
	}
	if err := validatePlanningBatch(planningSnapshot.MatchmakingInput(), run, batch); err != nil {
		return PlanningRunMutation{}, err
	}
	return service.complete(ctx, scope, run, batch, replayed)
}

func (service *PlanningRuns) complete(
	ctx context.Context,
	scope string,
	initial PlanningRunRecord,
	batch domain.ProposalBatch,
	replayed bool,
) (PlanningRunMutation, error) {
	completed := initial
	completed.Status = PlanningRunCompleted
	completed.CompletedAt = service.now().UTC()
	completed.ProposalCount = len(batch.Proposals)
	completed.UnmatchedCount = len(batch.Unmatched)
	completed.BudgetExhausted = batch.BudgetExhausted
	completed.Evidence = batch.Evidence
	runPayload, err := encodePlanningRun(completed)
	if err != nil {
		return PlanningRunMutation{}, err
	}
	proposalPayloads := make([][]byte, len(batch.Proposals))
	for index, proposal := range batch.Proposals {
		proposalPayloads[index], err = encodeProposal(initial.ID, proposal)
		if err != nil {
			return PlanningRunMutation{}, err
		}
	}
	unmatchedPayloads := make([][]byte, len(batch.Unmatched))
	for index, unmatched := range batch.Unmatched {
		unmatchedPayloads[index], err = encodeUnmatched(initial.ID, unmatched)
		if err != nil {
			return PlanningRunMutation{}, err
		}
	}
	canonical, err := encodePlanningCompletionCommand(completed, proposalPayloads, unmatchedPayloads)
	if err != nil {
		return PlanningRunMutation{}, err
	}
	operation := repository.Operation{
		Scope: scope, ID: domain.OperationID("/planning.complete/" + initial.ID),
		Kind: "planning_run.complete", Digest: repository.Digest(canonical), At: completed.CompletedAt,
	}
	for attempt := 0; attempt < maxPlanningCompletionAttempts; attempt++ {
		snapshot, err := service.repository.Snapshot(ctx, scope)
		if err != nil {
			return PlanningRunMutation{}, err
		}
		resource, exists := findResource(snapshot, Key(scope, ResourcePlanningRun, initial.ID))
		if !exists || resource.Deleted {
			return PlanningRunMutation{}, fmt.Errorf("planning run %q disappeared before completion", initial.ID)
		}
		current, err := decodePlanningRun(resource.Payload)
		if err != nil {
			return PlanningRunMutation{}, err
		}
		current.StorageVersion = resource.Version
		if current.Status == PlanningRunCompleted {
			return PlanningRunMutation{Run: current, Replayed: replayed}, nil
		}
		mutations := make([]repository.Mutation, 0, 1+len(batch.Proposals)+len(batch.Unmatched))
		mutations = append(mutations, repository.Mutation{
			Key:             Key(scope, ResourcePlanningRun, initial.ID),
			ExpectedVersion: resource.Version, Payload: runPayload,
		})
		for index, proposal := range batch.Proposals {
			mutations = append(mutations, repository.Mutation{
				Key: Key(scope, ResourceProposal, string(proposal.ID)), Payload: proposalPayloads[index],
			})
		}
		for index, unmatched := range batch.Unmatched {
			mutations = append(mutations, repository.Mutation{
				Key:     Key(scope, ResourcePlanningUnmatched, unmatchedResourceID(initial.ID, unmatched.Ticket.ID)),
				Payload: unmatchedPayloads[index],
			})
		}
		result, err := service.repository.Commit(ctx, operation, mutations)
		if err == nil {
			final, exists, loadErr := service.Get(ctx, scope, initial.ID)
			if loadErr != nil {
				return PlanningRunMutation{}, loadErr
			}
			if !exists || final.Status != PlanningRunCompleted {
				return PlanningRunMutation{}, fmt.Errorf("completed planning run %q cannot be read", initial.ID)
			}
			return PlanningRunMutation{Run: final, Replayed: replayed || result.Replayed}, nil
		}
		if !repository.IsConflict(err) {
			return PlanningRunMutation{}, err
		}
	}
	return PlanningRunMutation{}, domain.NewFailure(
		domain.FailureStaleSnapshot,
		"planning run %q changed repeatedly while completion was committed",
		initial.ID,
	)
}

func (service *PlanningRuns) Get(
	ctx context.Context,
	scope string,
	runID string,
) (PlanningRunRecord, bool, error) {
	snapshot, err := service.repository.Snapshot(ctx, scope)
	if err != nil {
		return PlanningRunRecord{}, false, err
	}
	resource, exists := findResource(snapshot, Key(scope, ResourcePlanningRun, runID))
	if !exists || resource.Deleted {
		return PlanningRunRecord{}, false, nil
	}
	run, err := decodePlanningRun(resource.Payload)
	if err != nil {
		return PlanningRunRecord{}, false, err
	}
	run.StorageVersion = resource.Version
	return run, true, nil
}

func (service *PlanningRuns) Proposals(
	ctx context.Context,
	scope string,
	runID string,
) (ProposalSnapshot, error) {
	snapshot, err := service.repository.Snapshot(ctx, scope)
	if err != nil {
		return ProposalSnapshot{}, err
	}
	run, err := completedRun(snapshot, scope, runID)
	if err != nil {
		return ProposalSnapshot{}, err
	}
	records := make([]ProposalRecord, 0, run.ProposalCount)
	for _, resource := range snapshot.Resources {
		if resource.Key.Kind != string(ResourceProposal) || resource.Deleted {
			continue
		}
		record, err := decodeProposal(resource.Payload)
		if err != nil {
			return ProposalSnapshot{}, err
		}
		if record.RunID == runID {
			record.StorageVersion = resource.Version
			records = append(records, record)
		}
	}
	slices.SortFunc(records, func(left, right ProposalRecord) int {
		if left.Proposal.ID < right.Proposal.ID {
			return -1
		}
		if left.Proposal.ID > right.Proposal.ID {
			return 1
		}
		return 0
	})
	if len(records) != run.ProposalCount {
		return ProposalSnapshot{}, fmt.Errorf("planning run %q proposal count is incomplete", runID)
	}
	return ProposalSnapshot{RunStorageVersion: run.StorageVersion, Records: records}, nil
}

func (service *PlanningRuns) Unmatched(
	ctx context.Context,
	scope string,
	runID string,
) (UnmatchedSnapshot, error) {
	snapshot, err := service.repository.Snapshot(ctx, scope)
	if err != nil {
		return UnmatchedSnapshot{}, err
	}
	run, err := completedRun(snapshot, scope, runID)
	if err != nil {
		return UnmatchedSnapshot{}, err
	}
	records := make([]UnmatchedRecord, 0, run.UnmatchedCount)
	for _, resource := range snapshot.Resources {
		if resource.Key.Kind != string(ResourcePlanningUnmatched) || resource.Deleted {
			continue
		}
		record, err := decodeUnmatched(resource.Payload)
		if err != nil {
			return UnmatchedSnapshot{}, err
		}
		if record.RunID == runID {
			record.StorageVersion = resource.Version
			records = append(records, record)
		}
	}
	slices.SortFunc(records, func(left, right UnmatchedRecord) int {
		if left.Unmatched.Ticket.ID < right.Unmatched.Ticket.ID {
			return -1
		}
		if left.Unmatched.Ticket.ID > right.Unmatched.Ticket.ID {
			return 1
		}
		return 0
	})
	if len(records) != run.UnmatchedCount {
		return UnmatchedSnapshot{}, fmt.Errorf("planning run %q unmatched count is incomplete", runID)
	}
	return UnmatchedSnapshot{RunStorageVersion: run.StorageVersion, Records: records}, nil
}

func (service *PlanningRuns) Proposal(
	ctx context.Context,
	scope string,
	proposalID domain.ProposalID,
) (ProposalRecord, bool, error) {
	snapshot, err := service.repository.Snapshot(ctx, scope)
	if err != nil {
		return ProposalRecord{}, false, err
	}
	resource, exists := findResource(snapshot, Key(scope, ResourceProposal, string(proposalID)))
	if !exists || resource.Deleted {
		return ProposalRecord{}, false, nil
	}
	record, err := decodeProposal(resource.Payload)
	if err != nil {
		return ProposalRecord{}, false, err
	}
	record.StorageVersion = resource.Version
	return record, true, nil
}

func (service *PlanningRuns) load(
	ctx context.Context,
	scope string,
	runID string,
) (PlanningRunRecord, PlanningSnapshot, error) {
	snapshot, err := service.repository.Snapshot(ctx, scope)
	if err != nil {
		return PlanningRunRecord{}, PlanningSnapshot{}, err
	}
	runResource, exists := findResource(snapshot, Key(scope, ResourcePlanningRun, runID))
	if !exists || runResource.Deleted {
		return PlanningRunRecord{}, PlanningSnapshot{}, ErrResourceNotFound
	}
	run, err := decodePlanningRun(runResource.Payload)
	if err != nil {
		return PlanningRunRecord{}, PlanningSnapshot{}, err
	}
	run.StorageVersion = runResource.Version
	snapshotResource, exists := findResource(snapshot, Key(scope, ResourcePlanningSnapshot, runID))
	if !exists || snapshotResource.Deleted {
		return PlanningRunRecord{}, PlanningSnapshot{}, fmt.Errorf("planning snapshot %q is missing", runID)
	}
	planningSnapshot, err := decodePlanningSnapshot(snapshotResource.Payload)
	if err != nil {
		return PlanningRunRecord{}, PlanningSnapshot{}, err
	}
	if planningSnapshot.RepositoryVersion() != run.SourceRepositoryVersion ||
		planningSnapshot.MatchmakingInput().ID != run.SnapshotID {
		return PlanningRunRecord{}, PlanningSnapshot{}, fmt.Errorf("planning run %q snapshot reference is inconsistent", runID)
	}
	input := planningSnapshot.MatchmakingInput()
	fingerprint, err := domain.FingerprintPolicy(input.Policy)
	if err != nil {
		return PlanningRunRecord{}, PlanningSnapshot{}, err
	}
	if input.Policy.Version != run.PolicyVersion || fingerprint != run.PolicyFingerprint {
		return PlanningRunRecord{}, PlanningSnapshot{}, fmt.Errorf("planning run %q policy reference is inconsistent", runID)
	}
	return run, planningSnapshot, nil
}

func planningInput(
	snapshot repository.Snapshot,
	scope string,
	runID string,
	policyVersion string,
	now time.Time,
) (domain.MatchmakingSnapshot, domain.PolicyFingerprint, error) {
	policyResource, exists := findResource(snapshot, Key(scope, ResourcePolicy, policyVersion))
	if !exists || policyResource.Deleted {
		return domain.MatchmakingSnapshot{}, "", domain.NewFailure(
			domain.FailureInvalidInput,
			"policy version %q is not registered",
			policyVersion,
		)
	}
	policy, fingerprint, err := decodePolicy(policyResource.Payload)
	if err != nil {
		return domain.MatchmakingSnapshot{}, "", err
	}
	input := domain.MatchmakingSnapshot{ID: domain.SnapshotID(runID), Now: now, Policy: policy}
	for _, resource := range snapshot.Resources {
		if resource.Deleted {
			continue
		}
		switch resource.Key.Kind {
		case string(ResourceMatchTicket):
			ticket, err := decodeMatchTicket(resource.Payload)
			if err != nil {
				return domain.MatchmakingSnapshot{}, "", err
			}
			if err := validateDemandIdentity(snapshot, scope, ResourceMatchTicket, string(ticket.ID)); err != nil {
				return domain.MatchmakingSnapshot{}, "", err
			}
			input.MatchTickets = append(input.MatchTickets, ticket)
		case string(ResourceBackfillTicket):
			ticket, err := decodeBackfillTicket(resource.Payload)
			if err != nil {
				return domain.MatchmakingSnapshot{}, "", err
			}
			if err := validateDemandIdentity(snapshot, scope, ResourceBackfillTicket, string(ticket.ID)); err != nil {
				return domain.MatchmakingSnapshot{}, "", err
			}
			if err := validateBackfillSessionClaim(snapshot, scope, ticket.SessionID, ticket.ID); err != nil {
				return domain.MatchmakingSnapshot{}, "", err
			}
			input.BackfillTickets = append(input.BackfillTickets, ticket)
		}
	}
	if err := domain.ValidateSnapshot(input); err != nil {
		return domain.MatchmakingSnapshot{}, "", err
	}
	return input, fingerprint, nil
}

func completedRun(snapshot repository.Snapshot, scope string, runID string) (PlanningRunRecord, error) {
	resource, exists := findResource(snapshot, Key(scope, ResourcePlanningRun, runID))
	if !exists || resource.Deleted {
		return PlanningRunRecord{}, ErrResourceNotFound
	}
	run, err := decodePlanningRun(resource.Payload)
	if err != nil {
		return PlanningRunRecord{}, err
	}
	run.StorageVersion = resource.Version
	if run.Status != PlanningRunCompleted {
		return PlanningRunRecord{}, domain.NewFailure(
			domain.FailureInvalidTransition,
			"planning run %q is not completed",
			runID,
		)
	}
	return run, nil
}

func validatePlanningBatch(
	input domain.MatchmakingSnapshot,
	run PlanningRunRecord,
	batch domain.ProposalBatch,
) error {
	if batch.SnapshotID != run.SnapshotID || batch.SnapshotID != input.ID {
		return fmt.Errorf("planner returned another snapshot identity")
	}
	fingerprint, err := domain.FingerprintPolicy(input.Policy)
	if err != nil {
		return err
	}
	if run.PolicyVersion != input.Policy.Version || run.PolicyFingerprint != fingerprint {
		return fmt.Errorf("planning run references another policy")
	}
	matchTickets := make(map[domain.TicketID]domain.Revision, len(input.MatchTickets))
	for _, ticket := range input.MatchTickets {
		matchTickets[ticket.ID] = ticket.Revision
	}
	backfillTickets := make(map[domain.TicketID]domain.BackfillTicket, len(input.BackfillTickets))
	for _, ticket := range input.BackfillTickets {
		backfillTickets[ticket.ID] = ticket
	}
	proposalIDs := make(map[domain.ProposalID]struct{}, len(batch.Proposals))
	selected := make(map[domain.TicketID]struct{}, len(input.MatchTickets))
	selectedBackfillIDs := make(map[domain.TicketID]struct{}, len(input.BackfillTickets))
	selectedBackfills := 0
	for _, proposal := range batch.Proposals {
		if err := domain.ValidateProposal(proposal); err != nil {
			return err
		}
		if proposal.PolicyVersion != input.Policy.Version || proposal.PolicyFingerprint != fingerprint {
			return fmt.Errorf("proposal %q references another policy", proposal.ID)
		}
		if _, exists := proposalIDs[proposal.ID]; exists {
			return fmt.Errorf("planning batch repeats proposal %q", proposal.ID)
		}
		proposalIDs[proposal.ID] = struct{}{}
		for _, reference := range proposal.Tickets {
			revision, exists := matchTickets[reference.ID]
			if !exists || revision != reference.Revision {
				return fmt.Errorf("proposal %q references stale ticket %q", proposal.ID, reference.ID)
			}
			if _, exists := selected[reference.ID]; exists {
				return fmt.Errorf("planning batch reuses ticket %q", reference.ID)
			}
			selected[reference.ID] = struct{}{}
		}
		if proposal.Backfill != nil {
			ticket, exists := backfillTickets[proposal.Backfill.Ticket.ID]
			if !exists || ticket.Revision != proposal.Backfill.Ticket.Revision ||
				ticket.SessionID != proposal.Backfill.SessionID ||
				ticket.RosterVersion != proposal.Backfill.RosterVersion {
				return fmt.Errorf("proposal %q references stale backfill", proposal.ID)
			}
			if _, exists := selectedBackfillIDs[ticket.ID]; exists {
				return fmt.Errorf("planning batch reuses backfill %q", ticket.ID)
			}
			selectedBackfillIDs[ticket.ID] = struct{}{}
			selectedBackfills++
		}
	}
	unmatched := make(map[domain.TicketID]struct{}, len(batch.Unmatched))
	for _, result := range batch.Unmatched {
		revision, exists := matchTickets[result.Ticket.ID]
		if !exists || revision != result.Ticket.Revision || !validUnmatchedReason(result.Reason) {
			return fmt.Errorf("planning batch has invalid unmatched ticket %q", result.Ticket.ID)
		}
		if _, exists := selected[result.Ticket.ID]; exists {
			return fmt.Errorf("planning batch selects and leaves ticket %q unmatched", result.Ticket.ID)
		}
		if _, exists := unmatched[result.Ticket.ID]; exists {
			return fmt.Errorf("planning batch repeats unmatched ticket %q", result.Ticket.ID)
		}
		unmatched[result.Ticket.ID] = struct{}{}
	}
	if len(selected)+len(unmatched) != len(matchTickets) {
		return fmt.Errorf("planning batch does not account for every captured match ticket")
	}
	if batch.Evidence.SelectedProposals != len(batch.Proposals) ||
		batch.Evidence.SelectedBackfills != selectedBackfills {
		return fmt.Errorf("planning batch evidence does not match selected results")
	}
	return nil
}
