// Package engine composes the deterministic planner and in-memory coordinator.
package engine

import (
	"time"

	"sema/internal/coordinator"
	"sema/internal/domain"
	"sema/internal/planner"
)

// Engine is the transport-neutral application boundary for the single-process runtime.
type Engine struct {
	coordinator *coordinator.Coordinator
}

func New(reservationTTL time.Duration) (*Engine, error) {
	owner, err := coordinator.New(reservationTTL)
	if err != nil {
		return nil, err
	}
	return &Engine{coordinator: owner}, nil
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
	policy domain.MatchmakingPolicy,
) (domain.MatchmakingSnapshot, error) {
	return engine.coordinator.Snapshot(id, now, policy)
}

// Plan runs a side-effect-free cycle over the current unreserved input.
func (engine *Engine) Plan(
	snapshotID domain.SnapshotID,
	now time.Time,
	policy domain.MatchmakingPolicy,
) (domain.ProposalBatch, error) {
	snapshot, err := engine.Snapshot(snapshotID, now, policy)
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
