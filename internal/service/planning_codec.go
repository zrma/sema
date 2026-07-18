package service

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"github.com/zrma/sema/internal/domain"
	"github.com/zrma/sema/internal/repository"
)

const (
	planningSnapshotPayloadSchema = "sema.planning-snapshot.v1"
	planningRunPayloadSchema      = "sema.planning-run.v1"
	proposalPayloadSchema         = "sema.proposal.v1"
	unmatchedPayloadSchema        = "sema.planning-unmatched.v1"
)

type persistedPlanningSnapshot struct {
	Schema            string            `json:"schema"`
	RepositoryVersion uint64            `json:"repository_version"`
	ID                domain.SnapshotID `json:"id"`
	Now               time.Time         `json:"now"`
	Policy            json.RawMessage   `json:"policy"`
	MatchTickets      []json.RawMessage `json:"match_tickets"`
	BackfillTickets   []json.RawMessage `json:"backfill_tickets"`
}

type persistedPlanningRun struct {
	Schema                  string                      `json:"schema"`
	ID                      string                      `json:"id"`
	SnapshotID              domain.SnapshotID           `json:"snapshot_id"`
	PolicyVersion           string                      `json:"policy_version"`
	PolicyFingerprint       domain.PolicyFingerprint    `json:"policy_fingerprint"`
	SourceRepositoryVersion uint64                      `json:"source_repository_version"`
	CapturedAt              time.Time                   `json:"captured_at"`
	CompletedAt             *time.Time                  `json:"completed_at,omitempty"`
	Status                  PlanningRunStatus           `json:"status"`
	ProposalCount           int                         `json:"proposal_count"`
	UnmatchedCount          int                         `json:"unmatched_count"`
	BudgetExhausted         bool                        `json:"budget_exhausted"`
	Evidence                persistedBatchScoreEvidence `json:"evidence"`
}

type persistedBatchScoreEvidence struct {
	CandidateProposals           int   `json:"candidate_proposals"`
	SelectedProposals            int   `json:"selected_proposals"`
	SelectedBackfills            int   `json:"selected_backfills"`
	WaitPriorityEligibleDemands  int   `json:"wait_priority_eligible_demands"`
	WaitPrioritySelectedDemands  int   `json:"wait_priority_selected_demands"`
	OldestWaitPriorityMillis     int64 `json:"oldest_wait_priority_millis"`
	OldestSelectedPriorityMillis int64 `json:"oldest_selected_priority_millis"`
	TotalUtility                 int64 `json:"total_utility"`
	CandidateGenerationNodes     int   `json:"candidate_generation_nodes"`
	CandidateGenerationTruncated bool  `json:"candidate_generation_truncated"`
	SelectionNodes               int   `json:"selection_nodes"`
	SelectionTruncated           bool  `json:"selection_truncated"`
}

type persistedProposalResource struct {
	Schema   string                 `json:"schema"`
	RunID    string                 `json:"run_id"`
	Proposal persistedMatchProposal `json:"proposal"`
}

type persistedMatchProposal struct {
	ID                domain.ProposalID         `json:"id"`
	Kind              domain.ProposalKind       `json:"kind"`
	PolicyVersion     string                    `json:"policy_version"`
	PolicyFingerprint domain.PolicyFingerprint  `json:"policy_fingerprint"`
	Teams             []persistedTeamAssignment `json:"teams"`
	Tickets           []persistedTicketRef      `json:"tickets"`
	Backfill          *persistedBackfillTarget  `json:"backfill,omitempty"`
	Evidence          persistedScoreEvidence    `json:"evidence"`
}

type persistedTicketRef struct {
	ID       domain.TicketID `json:"id"`
	Revision domain.Revision `json:"revision"`
}

type persistedBackfillTarget struct {
	Ticket        persistedTicketRef `json:"ticket"`
	SessionID     domain.SessionID   `json:"session_id"`
	RosterVersion domain.Revision    `json:"roster_version"`
}

type persistedTeamAssignment struct {
	Team    int                  `json:"team"`
	Tickets []persistedTicketRef `json:"tickets"`
}

type persistedScoreEvidence struct {
	RelaxationLevel          int   `json:"relaxation_level"`
	WaitPriority             bool  `json:"wait_priority"`
	RolePenalty              int   `json:"role_penalty"`
	TeamSkillGap             int   `json:"team_skill_gap"`
	OldestWaitMillis         int64 `json:"oldest_wait_millis"`
	TotalWaitMillis          int64 `json:"total_wait_millis"`
	MaxLatencyMillis         int   `json:"max_latency_millis"`
	CandidateTickets         int   `json:"candidate_tickets"`
	CandidatesEvaluated      int   `json:"candidates_evaluated"`
	SearchNodes              int   `json:"search_nodes"`
	CandidateWindowTruncated bool  `json:"candidate_window_truncated"`
	SearchTruncated          bool  `json:"search_truncated"`
	SelectionUtility         int64 `json:"selection_utility"`
}

type persistedUnmatchedResource struct {
	Schema    string                   `json:"schema"`
	RunID     string                   `json:"run_id"`
	Unmatched persistedUnmatchedTicket `json:"unmatched"`
}

type persistedUnmatchedTicket struct {
	Ticket persistedTicketRef     `json:"ticket"`
	Reason domain.UnmatchedReason `json:"reason"`
}

func encodePlanningSnapshot(snapshot PlanningSnapshot) ([]byte, error) {
	input := snapshot.MatchmakingInput()
	fingerprint, err := domain.FingerprintPolicy(input.Policy)
	if err != nil {
		return nil, err
	}
	policyPayload, err := encodePolicy(canonicalPolicy(input.Policy), fingerprint)
	if err != nil {
		return nil, err
	}
	matchTickets := make([]json.RawMessage, len(input.MatchTickets))
	for index, ticket := range input.MatchTickets {
		payload, err := encodeMatchTicket(ticket)
		if err != nil {
			return nil, err
		}
		matchTickets[index] = payload
	}
	backfillTickets := make([]json.RawMessage, len(input.BackfillTickets))
	for index, ticket := range input.BackfillTickets {
		payload, err := encodeBackfillTicket(ticket)
		if err != nil {
			return nil, err
		}
		backfillTickets[index] = payload
	}
	encoded, err := json.Marshal(persistedPlanningSnapshot{
		Schema: planningSnapshotPayloadSchema, RepositoryVersion: uint64(snapshot.RepositoryVersion()),
		ID: input.ID, Now: input.Now.UTC(), Policy: policyPayload,
		MatchTickets: matchTickets, BackfillTickets: backfillTickets,
	})
	if err != nil {
		return nil, fmt.Errorf("encode planning snapshot resource: %w", err)
	}
	return encoded, nil
}

func decodePlanningSnapshot(payload []byte) (PlanningSnapshot, error) {
	var stored persistedPlanningSnapshot
	if err := decodeStrict(payload, &stored); err != nil {
		return PlanningSnapshot{}, fmt.Errorf("decode planning snapshot resource: %w", err)
	}
	if stored.Schema != planningSnapshotPayloadSchema || stored.RepositoryVersion == 0 {
		return PlanningSnapshot{}, fmt.Errorf("planning snapshot resource header is invalid")
	}
	policy, _, err := decodePolicy(stored.Policy)
	if err != nil {
		return PlanningSnapshot{}, err
	}
	input := domain.MatchmakingSnapshot{ID: stored.ID, Now: stored.Now.UTC(), Policy: policy}
	input.MatchTickets = make([]domain.MatchTicket, len(stored.MatchTickets))
	for index, ticket := range stored.MatchTickets {
		input.MatchTickets[index], err = decodeMatchTicket(ticket)
		if err != nil {
			return PlanningSnapshot{}, err
		}
	}
	input.BackfillTickets = make([]domain.BackfillTicket, len(stored.BackfillTickets))
	for index, ticket := range stored.BackfillTickets {
		input.BackfillTickets[index], err = decodeBackfillTicket(ticket)
		if err != nil {
			return PlanningSnapshot{}, err
		}
	}
	return NewPlanningSnapshot(repository.Version(stored.RepositoryVersion), input)
}

func encodePlanningRun(run PlanningRunRecord) ([]byte, error) {
	if err := validatePlanningRun(run); err != nil {
		return nil, err
	}
	encoded, err := json.Marshal(persistedPlanningRun{
		Schema: planningRunPayloadSchema, ID: run.ID, SnapshotID: run.SnapshotID,
		PolicyVersion: run.PolicyVersion, PolicyFingerprint: run.PolicyFingerprint,
		SourceRepositoryVersion: uint64(run.SourceRepositoryVersion), CapturedAt: run.CapturedAt.UTC(),
		CompletedAt: optionalTime(run.CompletedAt), Status: run.Status,
		ProposalCount: run.ProposalCount, UnmatchedCount: run.UnmatchedCount,
		BudgetExhausted: run.BudgetExhausted, Evidence: toPersistedBatchEvidence(run.Evidence),
	})
	if err != nil {
		return nil, fmt.Errorf("encode planning run resource: %w", err)
	}
	return encoded, nil
}

func decodePlanningRun(payload []byte) (PlanningRunRecord, error) {
	var stored persistedPlanningRun
	if err := decodeStrict(payload, &stored); err != nil {
		return PlanningRunRecord{}, fmt.Errorf("decode planning run resource: %w", err)
	}
	if stored.Schema != planningRunPayloadSchema {
		return PlanningRunRecord{}, fmt.Errorf("unsupported planning run resource schema %q", stored.Schema)
	}
	run := PlanningRunRecord{
		ID: stored.ID, SnapshotID: stored.SnapshotID, PolicyVersion: stored.PolicyVersion,
		PolicyFingerprint:       stored.PolicyFingerprint,
		SourceRepositoryVersion: repository.Version(stored.SourceRepositoryVersion),
		CapturedAt:              stored.CapturedAt.UTC(), CompletedAt: timeValue(stored.CompletedAt), Status: stored.Status,
		ProposalCount: stored.ProposalCount, UnmatchedCount: stored.UnmatchedCount,
		BudgetExhausted: stored.BudgetExhausted, Evidence: fromPersistedBatchEvidence(stored.Evidence),
	}
	if err := validatePlanningRun(run); err != nil {
		return PlanningRunRecord{}, fmt.Errorf("stored planning run is invalid: %w", err)
	}
	return run, nil
}

func validatePlanningRun(run PlanningRunRecord) error {
	if run.ID == "" || run.SnapshotID == "" || run.PolicyVersion == "" || run.PolicyFingerprint == "" ||
		run.SourceRepositoryVersion == 0 || run.CapturedAt.IsZero() {
		return fmt.Errorf("planning run identity, policy, source version, and capture time are required")
	}
	if run.SnapshotID != domain.SnapshotID(run.ID) {
		return fmt.Errorf("planning run and snapshot identity differ")
	}
	if run.ProposalCount < 0 || run.UnmatchedCount < 0 {
		return fmt.Errorf("planning result counts cannot be negative")
	}
	switch run.Status {
	case PlanningRunPlanning:
		if !run.CompletedAt.IsZero() || run.ProposalCount != 0 || run.UnmatchedCount != 0 {
			return fmt.Errorf("planning run has completion state before completion")
		}
	case PlanningRunCompleted:
		if run.CompletedAt.IsZero() || run.CompletedAt.Before(run.CapturedAt) {
			return fmt.Errorf("completed planning run has an invalid completion time")
		}
	default:
		return fmt.Errorf("planning run has unknown status %q", run.Status)
	}
	return nil
}

func encodeProposal(runID string, proposal domain.MatchProposal) ([]byte, error) {
	if runID == "" {
		return nil, domain.NewFailure(domain.FailureInvalidInput, "proposal run identity is required")
	}
	if err := domain.ValidateProposal(proposal); err != nil {
		return nil, err
	}
	encoded, err := json.Marshal(persistedProposalResource{
		Schema: proposalPayloadSchema, RunID: runID, Proposal: toPersistedProposal(proposal),
	})
	if err != nil {
		return nil, fmt.Errorf("encode proposal resource: %w", err)
	}
	return encoded, nil
}

func decodeProposal(payload []byte) (ProposalRecord, error) {
	var stored persistedProposalResource
	if err := decodeStrict(payload, &stored); err != nil {
		return ProposalRecord{}, fmt.Errorf("decode proposal resource: %w", err)
	}
	if stored.Schema != proposalPayloadSchema || stored.RunID == "" {
		return ProposalRecord{}, fmt.Errorf("proposal resource header is invalid")
	}
	proposal := fromPersistedProposal(stored.Proposal)
	if err := domain.ValidateProposal(proposal); err != nil {
		return ProposalRecord{}, fmt.Errorf("stored proposal is invalid: %w", err)
	}
	return ProposalRecord{RunID: stored.RunID, Proposal: proposal}, nil
}

func encodeUnmatched(runID string, unmatched domain.UnmatchedTicket) ([]byte, error) {
	if runID == "" || unmatched.Ticket.ID == "" || unmatched.Ticket.Revision == 0 ||
		!validUnmatchedReason(unmatched.Reason) {
		return nil, domain.NewFailure(domain.FailureInvalidInput, "unmatched planning result is invalid")
	}
	encoded, err := json.Marshal(persistedUnmatchedResource{
		Schema: unmatchedPayloadSchema, RunID: runID,
		Unmatched: persistedUnmatchedTicket{
			Ticket: toPersistedTicketRef(unmatched.Ticket), Reason: unmatched.Reason,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("encode unmatched resource: %w", err)
	}
	return encoded, nil
}

func decodeUnmatched(payload []byte) (UnmatchedRecord, error) {
	var stored persistedUnmatchedResource
	if err := decodeStrict(payload, &stored); err != nil {
		return UnmatchedRecord{}, fmt.Errorf("decode unmatched resource: %w", err)
	}
	unmatched := domain.UnmatchedTicket{
		Ticket: fromPersistedTicketRef(stored.Unmatched.Ticket), Reason: stored.Unmatched.Reason,
	}
	if stored.Schema != unmatchedPayloadSchema || stored.RunID == "" || unmatched.Ticket.ID == "" ||
		unmatched.Ticket.Revision == 0 || !validUnmatchedReason(unmatched.Reason) {
		return UnmatchedRecord{}, fmt.Errorf("unmatched resource is invalid")
	}
	return UnmatchedRecord{RunID: stored.RunID, Unmatched: unmatched}, nil
}

func encodePlanningCompletionCommand(
	run PlanningRunRecord,
	proposals [][]byte,
	unmatched [][]byte,
) ([]byte, error) {
	run.CompletedAt = run.CapturedAt
	run.StorageVersion = 0
	runPayload, err := encodePlanningRun(run)
	if err != nil {
		return nil, err
	}
	encoded, err := json.Marshal(struct {
		Kind      string            `json:"kind"`
		Run       json.RawMessage   `json:"run"`
		Proposals []json.RawMessage `json:"proposals"`
		Unmatched []json.RawMessage `json:"unmatched"`
	}{
		Kind: "planning_run.complete", Run: runPayload,
		Proposals: rawMessages(proposals), Unmatched: rawMessages(unmatched),
	})
	if err != nil {
		return nil, fmt.Errorf("encode planning completion command: %w", err)
	}
	return encoded, nil
}

func rawMessages(payloads [][]byte) []json.RawMessage {
	messages := make([]json.RawMessage, len(payloads))
	for index, payload := range payloads {
		messages[index] = payload
	}
	return messages
}

func optionalTime(value time.Time) *time.Time {
	if value.IsZero() {
		return nil
	}
	canonical := value.UTC()
	return &canonical
}

func timeValue(value *time.Time) time.Time {
	if value == nil {
		return time.Time{}
	}
	return value.UTC()
}

func unmatchedResourceID(runID string, ticketID domain.TicketID) string {
	digest := sha256.Sum256([]byte(runID + "\x00" + string(ticketID)))
	return hex.EncodeToString(digest[:])
}

func validUnmatchedReason(reason domain.UnmatchedReason) bool {
	switch reason {
	case domain.UnmatchedHardConstraint,
		domain.UnmatchedInsufficientCapacity,
		domain.UnmatchedQualityThreshold,
		domain.UnmatchedSearchBudget,
		domain.UnmatchedProposalLimit,
		domain.UnmatchedBatchObjective:
		return true
	default:
		return false
	}
}

func toPersistedProposal(proposal domain.MatchProposal) persistedMatchProposal {
	teams := make([]persistedTeamAssignment, len(proposal.Teams))
	for index, team := range proposal.Teams {
		teams[index] = persistedTeamAssignment{Team: team.Team, Tickets: toPersistedTicketRefs(team.Tickets)}
	}
	return persistedMatchProposal{
		ID: proposal.ID, Kind: proposal.Kind, PolicyVersion: proposal.PolicyVersion,
		PolicyFingerprint: proposal.PolicyFingerprint, Teams: teams,
		Tickets: toPersistedTicketRefs(proposal.Tickets), Backfill: toPersistedBackfillTarget(proposal.Backfill),
		Evidence: persistedScoreEvidence(proposal.Evidence),
	}
}

func fromPersistedProposal(proposal persistedMatchProposal) domain.MatchProposal {
	teams := make([]domain.TeamAssignment, len(proposal.Teams))
	for index, team := range proposal.Teams {
		teams[index] = domain.TeamAssignment{Team: team.Team, Tickets: fromPersistedTicketRefs(team.Tickets)}
	}
	return domain.MatchProposal{
		ID: proposal.ID, Kind: proposal.Kind, PolicyVersion: proposal.PolicyVersion,
		PolicyFingerprint: proposal.PolicyFingerprint, Teams: teams,
		Tickets: fromPersistedTicketRefs(proposal.Tickets), Backfill: fromPersistedBackfillTarget(proposal.Backfill),
		Evidence: domain.ScoreEvidence(proposal.Evidence),
	}
}

func toPersistedTicketRefs(references []domain.TicketRef) []persistedTicketRef {
	converted := make([]persistedTicketRef, len(references))
	for index, reference := range references {
		converted[index] = toPersistedTicketRef(reference)
	}
	return converted
}

func fromPersistedTicketRefs(references []persistedTicketRef) []domain.TicketRef {
	converted := make([]domain.TicketRef, len(references))
	for index, reference := range references {
		converted[index] = fromPersistedTicketRef(reference)
	}
	return converted
}

func toPersistedTicketRef(reference domain.TicketRef) persistedTicketRef {
	return persistedTicketRef{ID: reference.ID, Revision: reference.Revision}
}

func fromPersistedTicketRef(reference persistedTicketRef) domain.TicketRef {
	return domain.TicketRef{ID: reference.ID, Revision: reference.Revision}
}

func toPersistedBackfillTarget(target *domain.BackfillTarget) *persistedBackfillTarget {
	if target == nil {
		return nil
	}
	return &persistedBackfillTarget{
		Ticket: toPersistedTicketRef(target.Ticket), SessionID: target.SessionID,
		RosterVersion: target.RosterVersion,
	}
}

func fromPersistedBackfillTarget(target *persistedBackfillTarget) *domain.BackfillTarget {
	if target == nil {
		return nil
	}
	return &domain.BackfillTarget{
		Ticket: fromPersistedTicketRef(target.Ticket), SessionID: target.SessionID,
		RosterVersion: target.RosterVersion,
	}
}

func toPersistedBatchEvidence(evidence domain.BatchScoreEvidence) persistedBatchScoreEvidence {
	return persistedBatchScoreEvidence(evidence)
}

func fromPersistedBatchEvidence(evidence persistedBatchScoreEvidence) domain.BatchScoreEvidence {
	return domain.BatchScoreEvidence(evidence)
}
