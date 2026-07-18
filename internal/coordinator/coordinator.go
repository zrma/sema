// Package coordinator owns in-memory active input, reservations, and assignments.
package coordinator

import (
	"reflect"
	"sort"
	"sync"
	"time"

	"github.com/zrma/sema/internal/domain"
)

type reservationRecord struct {
	reservation domain.Reservation
	proposal    domain.MatchProposal
	assignment  domain.AssignmentID
}

// Coordinator is the sole P0 reservation authority.
type Coordinator struct {
	mu sync.Mutex

	ttl time.Duration

	matchTickets         map[domain.TicketID]domain.MatchTicket
	backfillTickets      map[domain.TicketID]domain.BackfillTicket
	playerTickets        map[domain.PlayerID]domain.TicketID
	ticketReservations   map[domain.TicketID]domain.ReservationID
	backfillReservations map[domain.TicketID]domain.ReservationID
	reservations         map[domain.ReservationID]*reservationRecord
	assignments          map[domain.AssignmentID]domain.Assignment
}

func New(ttl time.Duration) (*Coordinator, error) {
	if ttl <= 0 {
		return nil, domain.NewFailure(domain.FailureInvalidInput, "reservation TTL must be positive")
	}
	return &Coordinator{
		ttl:                  ttl,
		matchTickets:         make(map[domain.TicketID]domain.MatchTicket),
		backfillTickets:      make(map[domain.TicketID]domain.BackfillTicket),
		playerTickets:        make(map[domain.PlayerID]domain.TicketID),
		ticketReservations:   make(map[domain.TicketID]domain.ReservationID),
		backfillReservations: make(map[domain.TicketID]domain.ReservationID),
		reservations:         make(map[domain.ReservationID]*reservationRecord),
		assignments:          make(map[domain.AssignmentID]domain.Assignment),
	}, nil
}

func (coordinator *Coordinator) UpsertMatchTicket(ticket domain.MatchTicket) error {
	if err := domain.ValidateMatchTicket(ticket); err != nil {
		return err
	}
	coordinator.mu.Lock()
	defer coordinator.mu.Unlock()

	if _, exists := coordinator.backfillTickets[ticket.ID]; exists {
		return domain.NewFailure(domain.FailureInvalidInput, "ticket %q is already a backfill demand", ticket.ID)
	}
	if current, exists := coordinator.matchTickets[ticket.ID]; exists {
		if ticket.Revision < current.Revision {
			return domain.NewFailure(domain.FailureInvalidRevision, "ticket %q revision moved backwards", ticket.ID)
		}
		if ticket.Revision == current.Revision {
			if reflect.DeepEqual(current, ticket) {
				return nil
			}
			return domain.NewFailure(domain.FailureInvalidRevision, "ticket %q reused a revision with different content", ticket.ID)
		}
	}
	if duplicatedPlayer := coordinator.duplicatedPlayerLocked(ticket); duplicatedPlayer != "" {
		return domain.NewFailure(domain.FailureInvalidInput, "player %q already belongs to another active ticket", duplicatedPlayer)
	}
	if current, exists := coordinator.matchTickets[ticket.ID]; exists {
		coordinator.releasePlayerOwnershipLocked(current)
	}
	cloned := domain.CloneMatchTicket(ticket)
	coordinator.matchTickets[ticket.ID] = cloned
	coordinator.acquirePlayerOwnershipLocked(cloned)
	return nil
}

func (coordinator *Coordinator) UpsertBackfillTicket(ticket domain.BackfillTicket) error {
	if err := domain.ValidateBackfillTicket(ticket); err != nil {
		return err
	}
	coordinator.mu.Lock()
	defer coordinator.mu.Unlock()

	if _, exists := coordinator.matchTickets[ticket.ID]; exists {
		return domain.NewFailure(domain.FailureInvalidInput, "ticket %q is already match supply", ticket.ID)
	}
	for id, active := range coordinator.backfillTickets {
		if id != ticket.ID && active.SessionID == ticket.SessionID {
			return domain.NewFailure(domain.FailureInvalidInput, "session %q already has an active backfill ticket", ticket.SessionID)
		}
	}
	if current, exists := coordinator.backfillTickets[ticket.ID]; exists {
		if ticket.Revision < current.Revision {
			return domain.NewFailure(domain.FailureInvalidRevision, "backfill ticket %q revision moved backwards", ticket.ID)
		}
		if ticket.Revision == current.Revision {
			if reflect.DeepEqual(current, ticket) {
				return nil
			}
			return domain.NewFailure(domain.FailureInvalidRevision, "backfill ticket %q reused a revision with different content", ticket.ID)
		}
	}
	coordinator.backfillTickets[ticket.ID] = domain.CloneBackfillTicket(ticket)
	return nil
}

// CancelMatchTicket removes the exact active revision. Existing reservations become stale.
func (coordinator *Coordinator) CancelMatchTicket(id domain.TicketID, revision domain.Revision) error {
	coordinator.mu.Lock()
	defer coordinator.mu.Unlock()
	current, exists := coordinator.matchTickets[id]
	if !exists || current.Revision != revision {
		return domain.NewFailure(domain.FailureStaleSnapshot, "match ticket %q is no longer at revision %d", id, revision)
	}
	coordinator.releasePlayerOwnershipLocked(current)
	delete(coordinator.matchTickets, id)
	return nil
}

// CancelBackfillTicket removes the exact demand and roster revision.
func (coordinator *Coordinator) CancelBackfillTicket(
	id domain.TicketID,
	revision domain.Revision,
	rosterVersion domain.Revision,
) error {
	coordinator.mu.Lock()
	defer coordinator.mu.Unlock()
	current, exists := coordinator.backfillTickets[id]
	if !exists || current.Revision != revision || current.RosterVersion != rosterVersion {
		return domain.NewFailure(domain.FailureStaleSnapshot, "backfill ticket %q freshness changed", id)
	}
	delete(coordinator.backfillTickets, id)
	return nil
}

// Snapshot returns defensive copies in canonical queue order.
func (coordinator *Coordinator) Snapshot(
	id domain.SnapshotID,
	now time.Time,
	policy domain.MatchmakingPolicy,
) (domain.MatchmakingSnapshot, error) {
	if id == "" || now.IsZero() {
		return domain.MatchmakingSnapshot{}, domain.NewFailure(domain.FailureInvalidInput, "snapshot identity and time are required")
	}
	if err := domain.ValidatePolicy(policy); err != nil {
		return domain.MatchmakingSnapshot{}, err
	}

	coordinator.mu.Lock()
	defer coordinator.mu.Unlock()
	coordinator.expireLocked(now)

	snapshot := domain.MatchmakingSnapshot{ID: id, Now: now, Policy: policy}
	for _, ticket := range coordinator.matchTickets {
		if _, reserved := coordinator.ticketReservations[ticket.ID]; reserved {
			continue
		}
		snapshot.MatchTickets = append(snapshot.MatchTickets, domain.CloneMatchTicket(ticket))
	}
	for _, ticket := range coordinator.backfillTickets {
		if _, reserved := coordinator.backfillReservations[ticket.ID]; reserved {
			continue
		}
		snapshot.BackfillTickets = append(snapshot.BackfillTickets, domain.CloneBackfillTicket(ticket))
	}
	sort.Slice(snapshot.MatchTickets, func(left, right int) bool {
		if !snapshot.MatchTickets[left].EnqueuedAt.Equal(snapshot.MatchTickets[right].EnqueuedAt) {
			return snapshot.MatchTickets[left].EnqueuedAt.Before(snapshot.MatchTickets[right].EnqueuedAt)
		}
		return snapshot.MatchTickets[left].ID < snapshot.MatchTickets[right].ID
	})
	sort.Slice(snapshot.BackfillTickets, func(left, right int) bool {
		if !snapshot.BackfillTickets[left].EnqueuedAt.Equal(snapshot.BackfillTickets[right].EnqueuedAt) {
			return snapshot.BackfillTickets[left].EnqueuedAt.Before(snapshot.BackfillTickets[right].EnqueuedAt)
		}
		return snapshot.BackfillTickets[left].ID < snapshot.BackfillTickets[right].ID
	})
	if err := domain.ValidateSnapshot(snapshot); err != nil {
		return domain.MatchmakingSnapshot{}, err
	}
	return snapshot, nil
}

// Reserve atomically binds every proposal resource to an opaque idempotency token.
func (coordinator *Coordinator) Reserve(
	proposal domain.MatchProposal,
	reservationID domain.ReservationID,
	now time.Time,
) (domain.Reservation, error) {
	if reservationID == "" || now.IsZero() {
		return domain.Reservation{}, domain.NewFailure(domain.FailureInvalidInput, "reservation identity and time are required")
	}
	if err := domain.ValidateProposal(proposal); err != nil {
		return domain.Reservation{}, err
	}
	proposal = domain.CloneProposal(proposal)

	coordinator.mu.Lock()
	defer coordinator.mu.Unlock()
	coordinator.expireLocked(now)

	if existing, ok := coordinator.reservations[reservationID]; ok {
		if !reflect.DeepEqual(existing.proposal, proposal) {
			return domain.Reservation{}, domain.NewFailure(domain.FailureIdempotencyConflict, "reservation ID %q was used for another proposal", reservationID)
		}
		switch existing.reservation.Status {
		case domain.ReservationActive, domain.ReservationConfirmed:
			return cloneReservation(existing.reservation), nil
		case domain.ReservationExpired:
			return domain.Reservation{}, domain.NewFailure(domain.FailureReservationExpired, "reservation %q has expired", reservationID)
		default:
			return domain.Reservation{}, domain.NewFailure(domain.FailureInvalidTransition, "reservation %q is cancelled", reservationID)
		}
	}
	if err := coordinator.validateFreshnessLocked(proposal); err != nil {
		return domain.Reservation{}, err
	}
	if err := coordinator.validateAvailabilityLocked(proposal); err != nil {
		return domain.Reservation{}, err
	}

	reservation := domain.Reservation{
		ID:         reservationID,
		ProposalID: proposal.ID,
		Tickets:    append([]domain.TicketRef(nil), proposal.Tickets...),
		Backfill:   domain.CloneBackfillTarget(proposal.Backfill),
		ExpiresAt:  now.Add(coordinator.ttl),
		Status:     domain.ReservationActive,
	}
	coordinator.reservations[reservationID] = &reservationRecord{reservation: reservation, proposal: proposal}
	for _, ref := range proposal.Tickets {
		coordinator.ticketReservations[ref.ID] = reservationID
	}
	if proposal.Backfill != nil {
		coordinator.backfillReservations[proposal.Backfill.Ticket.ID] = reservationID
	}
	return cloneReservation(reservation), nil
}

// Confirm performs a second CAS check and consumes the active input atomically.
func (coordinator *Coordinator) Confirm(
	reservationID domain.ReservationID,
	assignmentID domain.AssignmentID,
	now time.Time,
) (domain.Assignment, error) {
	if reservationID == "" || assignmentID == "" || now.IsZero() {
		return domain.Assignment{}, domain.NewFailure(domain.FailureInvalidInput, "reservation, assignment, and time are required")
	}
	coordinator.mu.Lock()
	defer coordinator.mu.Unlock()
	coordinator.expireLocked(now)

	record, exists := coordinator.reservations[reservationID]
	if !exists {
		return domain.Assignment{}, domain.NewFailure(domain.FailureInvalidTransition, "reservation %q does not exist", reservationID)
	}
	if record.reservation.Status == domain.ReservationConfirmed {
		if record.assignment != assignmentID {
			return domain.Assignment{}, domain.NewFailure(domain.FailureIdempotencyConflict, "reservation %q was confirmed with another assignment", reservationID)
		}
		return cloneAssignment(coordinator.assignments[assignmentID]), nil
	}
	if record.reservation.Status == domain.ReservationExpired {
		return domain.Assignment{}, domain.NewFailure(domain.FailureReservationExpired, "reservation %q has expired", reservationID)
	}
	if record.reservation.Status != domain.ReservationActive {
		return domain.Assignment{}, domain.NewFailure(domain.FailureInvalidTransition, "reservation %q is not active", reservationID)
	}
	if err := coordinator.validateFreshnessLocked(record.proposal); err != nil {
		record.reservation.Status = domain.ReservationCancelled
		coordinator.releaseLocked(record.reservation)
		return domain.Assignment{}, err
	}
	if existing, ok := coordinator.assignments[assignmentID]; ok {
		if existing.ReservationID == reservationID {
			return cloneAssignment(existing), nil
		}
		return domain.Assignment{}, domain.NewFailure(domain.FailureIdempotencyConflict, "assignment ID %q was used by another reservation", assignmentID)
	}

	assignment := domain.Assignment{
		ID:            assignmentID,
		ReservationID: reservationID,
		ProposalID:    record.proposal.ID,
		Kind:          record.proposal.Kind,
		Teams:         domain.CloneTeams(record.proposal.Teams),
		Backfill:      domain.CloneBackfillTarget(record.proposal.Backfill),
		ConfirmedAt:   now,
		Status:        domain.AssignmentPending,
	}
	for _, ref := range record.proposal.Tickets {
		if ticket, exists := coordinator.matchTickets[ref.ID]; exists {
			coordinator.releasePlayerOwnershipLocked(ticket)
		}
		delete(coordinator.matchTickets, ref.ID)
	}
	if record.proposal.Backfill != nil {
		delete(coordinator.backfillTickets, record.proposal.Backfill.Ticket.ID)
	}
	coordinator.releaseLocked(record.reservation)
	record.reservation.Status = domain.ReservationConfirmed
	record.assignment = assignmentID
	coordinator.assignments[assignmentID] = assignment
	return cloneAssignment(assignment), nil
}

// AcknowledgeAssignment records the external application outcome exactly once.
func (coordinator *Coordinator) AcknowledgeAssignment(
	assignmentID domain.AssignmentID,
	request domain.AssignmentAcknowledgmentRequest,
	now time.Time,
) (domain.Assignment, error) {
	if assignmentID == "" || request.OperationID == "" || now.IsZero() {
		return domain.Assignment{}, domain.NewFailure(domain.FailureInvalidInput, "assignment, operation, and acknowledgment time are required")
	}
	coordinator.mu.Lock()
	defer coordinator.mu.Unlock()

	assignment, exists := coordinator.assignments[assignmentID]
	if !exists {
		return domain.Assignment{}, domain.NewFailure(domain.FailureInvalidTransition, "assignment %q does not exist", assignmentID)
	}
	if assignment.Status != domain.AssignmentPending {
		if assignment.Acknowledgment != nil && assignment.Acknowledgment.OperationID == request.OperationID {
			if reflect.DeepEqual(assignment.Acknowledgment.AssignmentAcknowledgmentRequest, request) {
				return cloneAssignment(assignment), nil
			}
			return domain.Assignment{}, domain.NewFailure(domain.FailureIdempotencyConflict, "operation ID %q was reused with another outcome", request.OperationID)
		}
		return domain.Assignment{}, domain.NewFailure(domain.FailureInvalidTransition, "assignment %q is already terminal", assignmentID)
	}
	if err := domain.ValidateAssignmentAcknowledgment(assignment, request); err != nil {
		return domain.Assignment{}, err
	}
	acknowledgment := &domain.AssignmentAcknowledgment{
		AssignmentAcknowledgmentRequest: request,
		AcknowledgedAt:                  now,
	}
	assignment.Status = request.Outcome
	assignment.Acknowledgment = acknowledgment
	coordinator.assignments[assignmentID] = assignment
	return cloneAssignment(assignment), nil
}

// Assignment returns a defensive copy of the current assignment read model.
func (coordinator *Coordinator) Assignment(assignmentID domain.AssignmentID) (domain.Assignment, bool) {
	coordinator.mu.Lock()
	defer coordinator.mu.Unlock()
	assignment, exists := coordinator.assignments[assignmentID]
	if !exists {
		return domain.Assignment{}, false
	}
	return cloneAssignment(assignment), true
}

// Cancel releases an active reservation. Repeating the same cancel is idempotent.
func (coordinator *Coordinator) Cancel(reservationID domain.ReservationID, now time.Time) (domain.Reservation, error) {
	if reservationID == "" || now.IsZero() {
		return domain.Reservation{}, domain.NewFailure(domain.FailureInvalidInput, "reservation identity and time are required")
	}
	coordinator.mu.Lock()
	defer coordinator.mu.Unlock()
	coordinator.expireLocked(now)

	record, exists := coordinator.reservations[reservationID]
	if !exists {
		return domain.Reservation{}, domain.NewFailure(domain.FailureInvalidTransition, "reservation %q does not exist", reservationID)
	}
	switch record.reservation.Status {
	case domain.ReservationActive:
		coordinator.releaseLocked(record.reservation)
		record.reservation.Status = domain.ReservationCancelled
		return cloneReservation(record.reservation), nil
	case domain.ReservationCancelled:
		return cloneReservation(record.reservation), nil
	case domain.ReservationExpired:
		return domain.Reservation{}, domain.NewFailure(domain.FailureReservationExpired, "reservation %q has expired", reservationID)
	default:
		return domain.Reservation{}, domain.NewFailure(domain.FailureInvalidTransition, "confirmed reservation %q cannot be cancelled", reservationID)
	}
}

func (coordinator *Coordinator) validateFreshnessLocked(proposal domain.MatchProposal) error {
	for _, ref := range proposal.Tickets {
		current, exists := coordinator.matchTickets[ref.ID]
		if !exists || current.Revision != ref.Revision {
			return domain.NewFailure(domain.FailureStaleSnapshot, "match ticket %q freshness changed", ref.ID)
		}
	}
	if proposal.Backfill != nil {
		target := proposal.Backfill
		current, exists := coordinator.backfillTickets[target.Ticket.ID]
		if !exists || current.Revision != target.Ticket.Revision || current.SessionID != target.SessionID || current.RosterVersion != target.RosterVersion {
			return domain.NewFailure(domain.FailureStaleSnapshot, "backfill ticket %q freshness changed", target.Ticket.ID)
		}
	}
	return nil
}

func (coordinator *Coordinator) validateAvailabilityLocked(proposal domain.MatchProposal) error {
	for _, ref := range proposal.Tickets {
		if owner, reserved := coordinator.ticketReservations[ref.ID]; reserved {
			return domain.NewFailure(domain.FailureReservationConflict, "match ticket %q is reserved by %q", ref.ID, owner)
		}
	}
	if proposal.Backfill != nil {
		id := proposal.Backfill.Ticket.ID
		if owner, reserved := coordinator.backfillReservations[id]; reserved {
			return domain.NewFailure(domain.FailureReservationConflict, "backfill ticket %q is reserved by %q", id, owner)
		}
	}
	return nil
}

func (coordinator *Coordinator) expireLocked(now time.Time) {
	for _, record := range coordinator.reservations {
		if record.reservation.Status != domain.ReservationActive || now.Before(record.reservation.ExpiresAt) {
			continue
		}
		coordinator.releaseLocked(record.reservation)
		record.reservation.Status = domain.ReservationExpired
	}
}

func (coordinator *Coordinator) releaseLocked(reservation domain.Reservation) {
	for _, ref := range reservation.Tickets {
		if coordinator.ticketReservations[ref.ID] == reservation.ID {
			delete(coordinator.ticketReservations, ref.ID)
		}
	}
	if reservation.Backfill != nil && coordinator.backfillReservations[reservation.Backfill.Ticket.ID] == reservation.ID {
		delete(coordinator.backfillReservations, reservation.Backfill.Ticket.ID)
	}
}

func (coordinator *Coordinator) duplicatedPlayerLocked(ticket domain.MatchTicket) domain.PlayerID {
	for _, player := range ticket.Players {
		if owner, exists := coordinator.playerTickets[player.ID]; exists && owner != ticket.ID {
			return player.ID
		}
	}
	return ""
}

func (coordinator *Coordinator) acquirePlayerOwnershipLocked(ticket domain.MatchTicket) {
	for _, player := range ticket.Players {
		coordinator.playerTickets[player.ID] = ticket.ID
	}
}

func (coordinator *Coordinator) releasePlayerOwnershipLocked(ticket domain.MatchTicket) {
	for _, player := range ticket.Players {
		if coordinator.playerTickets[player.ID] == ticket.ID {
			delete(coordinator.playerTickets, player.ID)
		}
	}
}

func cloneReservation(reservation domain.Reservation) domain.Reservation {
	reservation.Tickets = append([]domain.TicketRef(nil), reservation.Tickets...)
	reservation.Backfill = domain.CloneBackfillTarget(reservation.Backfill)
	return reservation
}

func cloneAssignment(assignment domain.Assignment) domain.Assignment {
	assignment.Teams = domain.CloneTeams(assignment.Teams)
	assignment.Backfill = domain.CloneBackfillTarget(assignment.Backfill)
	if assignment.Acknowledgment != nil {
		acknowledgment := *assignment.Acknowledgment
		assignment.Acknowledgment = &acknowledgment
	}
	return assignment
}
