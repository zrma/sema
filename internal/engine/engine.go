// Package engine composes the deterministic planner and in-memory coordinator.
package engine

import (
	"time"

	"github.com/zrma/sema/internal/coordinator"
	"github.com/zrma/sema/internal/domain"
	"github.com/zrma/sema/internal/planner"
	policycatalog "github.com/zrma/sema/internal/policy"
)

// Engine is the transport-neutral application boundary for the single-process runtime.
type Engine struct {
	coordinator *coordinator.Coordinator
	policies    *policycatalog.Catalog
}

func New(reservationTTL time.Duration) (*Engine, error) {
	owner, err := coordinator.New(reservationTTL)
	if err != nil {
		return nil, err
	}
	return &Engine{coordinator: owner, policies: policycatalog.NewCatalog()}, nil
}

func (engine *Engine) RegisterPolicy(policy domain.MatchmakingPolicy) (domain.PolicyFingerprint, error) {
	entry, err := engine.policies.Register(policy)
	if err != nil {
		return "", err
	}
	return entry.Fingerprint, nil
}

func (engine *Engine) Policy(version string) (domain.MatchmakingPolicy, domain.PolicyFingerprint, bool) {
	entry, exists := engine.policies.Get(version)
	if !exists {
		return domain.MatchmakingPolicy{}, "", false
	}
	return entry.Policy, entry.Fingerprint, true
}

func (engine *Engine) SubmitMatchTicket(ticket domain.MatchTicket) error {
	return engine.coordinator.UpsertMatchTicket(ticket)
}

func (engine *Engine) SubmitBackfillTicket(ticket domain.BackfillTicket) error {
	return engine.coordinator.UpsertBackfillTicket(ticket)
}

func (engine *Engine) CancelMatchTicket(id domain.TicketID, revision domain.Revision) error {
	return engine.coordinator.CancelMatchTicket(id, revision)
}

func (engine *Engine) CancelBackfillTicket(
	id domain.TicketID,
	revision domain.Revision,
	rosterVersion domain.Revision,
) error {
	return engine.coordinator.CancelBackfillTicket(id, revision, rosterVersion)
}

// Snapshot returns the immutable, currently unreserved planning input.
func (engine *Engine) Snapshot(
	id domain.SnapshotID,
	now time.Time,
	policyVersion string,
) (domain.MatchmakingSnapshot, error) {
	entry, exists := engine.policies.Get(policyVersion)
	if !exists {
		return domain.MatchmakingSnapshot{}, domain.NewFailure(
			domain.FailureInvalidInput,
			"policy version %q is not registered",
			policyVersion,
		)
	}
	return engine.coordinator.Snapshot(id, now, entry.Policy)
}

// Plan runs a side-effect-free cycle over the current unreserved input.
func (engine *Engine) Plan(
	snapshotID domain.SnapshotID,
	now time.Time,
	policyVersion string,
) (domain.ProposalBatch, error) {
	snapshot, err := engine.Snapshot(snapshotID, now, policyVersion)
	if err != nil {
		return domain.ProposalBatch{}, err
	}
	return planner.Plan(snapshot)
}

func (engine *Engine) Reserve(
	proposal domain.MatchProposal,
	reservationID domain.ReservationID,
	now time.Time,
) (domain.Reservation, error) {
	return engine.coordinator.Reserve(proposal, reservationID, now)
}

func (engine *Engine) Confirm(
	reservationID domain.ReservationID,
	assignmentID domain.AssignmentID,
	now time.Time,
) (domain.Assignment, error) {
	return engine.coordinator.Confirm(reservationID, assignmentID, now)
}

func (engine *Engine) CancelReservation(
	reservationID domain.ReservationID,
	now time.Time,
) (domain.Reservation, error) {
	return engine.coordinator.Cancel(reservationID, now)
}

func (engine *Engine) AcknowledgeAssignment(
	assignmentID domain.AssignmentID,
	request domain.AssignmentAcknowledgmentRequest,
	now time.Time,
) (domain.Assignment, error) {
	return engine.coordinator.AcknowledgeAssignment(assignmentID, request, now)
}

func (engine *Engine) Assignment(assignmentID domain.AssignmentID) (domain.Assignment, bool) {
	return engine.coordinator.Assignment(assignmentID)
}
