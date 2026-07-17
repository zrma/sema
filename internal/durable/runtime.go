// Package durable provides the single-writer journaled Sema runtime.
package durable

import (
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/zrma/sema/internal/domain"
	"github.com/zrma/sema/internal/engine"
)

const (
	defaultAuditLimit = 100
	maxAuditLimit     = 1000
)

// Runtime serializes engine access and returns success only after its journal is synced.
type Runtime struct {
	mu sync.Mutex

	reservationTTL time.Duration
	journal        *journal
	engine         *engine.Engine
	closed         bool
	poisoned       error
}

// Open locks, recovers, and replays a durable runtime journal.
func Open(path string, reservationTTL time.Duration) (*Runtime, error) {
	if reservationTTL <= 0 {
		return nil, domain.NewFailure(domain.FailureInvalidInput, "reservation TTL must be positive")
	}
	journal, err := openJournal(path)
	if err != nil {
		return nil, err
	}
	records := journal.Records()
	if len(records) == 0 {
		if _, err := journal.append(eventRuntimeConfigured, runtimeConfiguredEvent{ReservationTTLNanos: int64(reservationTTL)}); err != nil {
			_ = journal.Close()
			return nil, err
		}
		records = journal.Records()
	}
	runtime, err := replay(reservationTTL, records)
	if err != nil {
		_ = journal.Close()
		return nil, err
	}
	return &Runtime{reservationTTL: reservationTTL, journal: journal, engine: runtime}, nil
}

func (runtime *Runtime) RegisterPolicy(policy domain.MatchmakingPolicy) (domain.PolicyFingerprint, error) {
	runtime.mu.Lock()
	defer runtime.mu.Unlock()
	if err := runtime.readyLocked(); err != nil {
		return "", err
	}
	policy = domain.ClonePolicy(policy)
	fingerprint, err := runtime.engine.RegisterPolicy(policy)
	if err != nil {
		return "", err
	}
	if err := runtime.persistLocked(eventPolicyRegistered, policyRegisteredEvent{Policy: policy}); err != nil {
		return "", err
	}
	return fingerprint, nil
}

func (runtime *Runtime) Policy(version string) (domain.MatchmakingPolicy, domain.PolicyFingerprint, bool, error) {
	runtime.mu.Lock()
	defer runtime.mu.Unlock()
	if err := runtime.readyLocked(); err != nil {
		return domain.MatchmakingPolicy{}, "", false, err
	}
	policy, fingerprint, exists := runtime.engine.Policy(version)
	return policy, fingerprint, exists, nil
}

func (runtime *Runtime) SubmitMatchTicket(ticket domain.MatchTicket) error {
	runtime.mu.Lock()
	defer runtime.mu.Unlock()
	if err := runtime.readyLocked(); err != nil {
		return err
	}
	ticket = domain.CloneMatchTicket(ticket)
	if err := runtime.engine.SubmitMatchTicket(ticket); err != nil {
		return err
	}
	return runtime.persistLocked(eventMatchTicketSubmitted, matchTicketSubmittedEvent{Ticket: ticket})
}

func (runtime *Runtime) SubmitBackfillTicket(ticket domain.BackfillTicket) error {
	runtime.mu.Lock()
	defer runtime.mu.Unlock()
	if err := runtime.readyLocked(); err != nil {
		return err
	}
	ticket = domain.CloneBackfillTicket(ticket)
	if err := runtime.engine.SubmitBackfillTicket(ticket); err != nil {
		return err
	}
	return runtime.persistLocked(eventBackfillSubmitted, backfillSubmittedEvent{Ticket: ticket})
}

func (runtime *Runtime) CancelMatchTicket(id domain.TicketID, revision domain.Revision) error {
	runtime.mu.Lock()
	defer runtime.mu.Unlock()
	if err := runtime.readyLocked(); err != nil {
		return err
	}
	if err := runtime.engine.CancelMatchTicket(id, revision); err != nil {
		return err
	}
	return runtime.persistLocked(eventMatchTicketCancelled, matchTicketCancelledEvent{TicketID: id, Revision: revision})
}

func (runtime *Runtime) CancelBackfillTicket(
	id domain.TicketID,
	revision domain.Revision,
	rosterVersion domain.Revision,
) error {
	runtime.mu.Lock()
	defer runtime.mu.Unlock()
	if err := runtime.readyLocked(); err != nil {
		return err
	}
	if err := runtime.engine.CancelBackfillTicket(id, revision, rosterVersion); err != nil {
		return err
	}
	event := backfillCancelledEvent{TicketID: id, Revision: revision, RosterVersion: rosterVersion}
	return runtime.persistLocked(eventBackfillCancelled, event)
}

// Plan records the complete deterministic decision before returning it to the caller.
func (runtime *Runtime) Plan(
	snapshotID domain.SnapshotID,
	now time.Time,
	policyVersion string,
) (domain.ProposalBatch, error) {
	runtime.mu.Lock()
	defer runtime.mu.Unlock()
	if err := runtime.readyLocked(); err != nil {
		return domain.ProposalBatch{}, err
	}
	batch, err := runtime.engine.Plan(snapshotID, now, policyVersion)
	if err != nil {
		return domain.ProposalBatch{}, err
	}
	event, err := newPlanCompletedEvent(snapshotID, now, policyVersion, batch)
	if err != nil {
		return domain.ProposalBatch{}, err
	}
	if err := runtime.persistLocked(eventPlanCompleted, event); err != nil {
		return domain.ProposalBatch{}, err
	}
	return batch, nil
}

func (runtime *Runtime) Reserve(
	proposal domain.MatchProposal,
	reservationID domain.ReservationID,
	now time.Time,
) (domain.Reservation, error) {
	runtime.mu.Lock()
	defer runtime.mu.Unlock()
	if err := runtime.readyLocked(); err != nil {
		return domain.Reservation{}, err
	}
	proposal = domain.CloneProposal(proposal)
	reservation, err := runtime.engine.Reserve(proposal, reservationID, now)
	if err != nil {
		return domain.Reservation{}, err
	}
	event := proposalReservedEvent{Proposal: proposal, ReservationID: reservationID, Now: now}
	if err := runtime.persistLocked(eventProposalReserved, event); err != nil {
		return domain.Reservation{}, err
	}
	return reservation, nil
}

func (runtime *Runtime) Confirm(
	reservationID domain.ReservationID,
	assignmentID domain.AssignmentID,
	now time.Time,
) (domain.Assignment, error) {
	runtime.mu.Lock()
	defer runtime.mu.Unlock()
	if err := runtime.readyLocked(); err != nil {
		return domain.Assignment{}, err
	}
	assignment, err := runtime.engine.Confirm(reservationID, assignmentID, now)
	if err != nil {
		return domain.Assignment{}, err
	}
	event := reservationConfirmedEvent{ReservationID: reservationID, AssignmentID: assignmentID, Now: now}
	if err := runtime.persistLocked(eventReservationConfirmed, event); err != nil {
		return domain.Assignment{}, err
	}
	return assignment, nil
}

func (runtime *Runtime) CancelReservation(
	reservationID domain.ReservationID,
	now time.Time,
) (domain.Reservation, error) {
	runtime.mu.Lock()
	defer runtime.mu.Unlock()
	if err := runtime.readyLocked(); err != nil {
		return domain.Reservation{}, err
	}
	reservation, err := runtime.engine.CancelReservation(reservationID, now)
	if err != nil {
		return domain.Reservation{}, err
	}
	event := reservationCancelledEvent{ReservationID: reservationID, Now: now}
	if err := runtime.persistLocked(eventReservationCancelled, event); err != nil {
		return domain.Reservation{}, err
	}
	return reservation, nil
}

func (runtime *Runtime) AcknowledgeAssignment(
	assignmentID domain.AssignmentID,
	request domain.AssignmentAcknowledgmentRequest,
	now time.Time,
) (domain.Assignment, error) {
	runtime.mu.Lock()
	defer runtime.mu.Unlock()
	if err := runtime.readyLocked(); err != nil {
		return domain.Assignment{}, err
	}
	assignment, err := runtime.engine.AcknowledgeAssignment(assignmentID, request, now)
	if err != nil {
		return domain.Assignment{}, err
	}
	event := assignmentAcknowledgedEvent{AssignmentID: assignmentID, Request: request, Now: now}
	if err := runtime.persistLocked(eventAssignmentAcked, event); err != nil {
		return domain.Assignment{}, err
	}
	return assignment, nil
}

func (runtime *Runtime) Assignment(id domain.AssignmentID) (domain.Assignment, bool, error) {
	runtime.mu.Lock()
	defer runtime.mu.Unlock()
	if err := runtime.readyLocked(); err != nil {
		return domain.Assignment{}, false, err
	}
	assignment, exists := runtime.engine.Assignment(id)
	return assignment, exists, nil
}

// Audit returns a defensive, sequence-ordered page after the requested sequence.
func (runtime *Runtime) Audit(after uint64, limit int) ([]Record, error) {
	runtime.mu.Lock()
	defer runtime.mu.Unlock()
	if err := runtime.readyLocked(); err != nil {
		return nil, err
	}
	if limit == 0 {
		limit = defaultAuditLimit
	}
	if limit < 0 || limit > maxAuditLimit {
		return nil, fmt.Errorf("audit limit must be between 1 and %d", maxAuditLimit)
	}
	records := runtime.journal.Records()
	page := make([]Record, 0, min(limit, len(records)))
	for _, record := range records {
		if record.Sequence <= after {
			continue
		}
		page = append(page, record)
		if len(page) == limit {
			break
		}
	}
	return page, nil
}

func (runtime *Runtime) Close() error {
	runtime.mu.Lock()
	defer runtime.mu.Unlock()
	if runtime.closed {
		return nil
	}
	runtime.closed = true
	return runtime.journal.Close()
}

func (runtime *Runtime) persistLocked(kind eventKind, event any) error {
	if _, err := runtime.journal.append(kind, event); err != nil {
		records, reloadErr := runtime.journal.reload()
		if reloadErr != nil {
			runtime.poisoned = errors.Join(err, reloadErr)
			return fmt.Errorf("persist %s and recover journal: %w", kind, runtime.poisoned)
		}
		recovered, replayErr := replay(runtime.reservationTTL, records)
		if replayErr != nil {
			runtime.poisoned = errors.Join(err, replayErr)
			return fmt.Errorf("persist %s and replay journal: %w", kind, runtime.poisoned)
		}
		runtime.engine = recovered
		return fmt.Errorf("persist %s: %w", kind, err)
	}
	return nil
}

func (runtime *Runtime) readyLocked() error {
	if runtime.closed {
		return fmt.Errorf("durable runtime is closed")
	}
	if runtime.poisoned != nil {
		return fmt.Errorf("durable runtime needs operator recovery: %w", runtime.poisoned)
	}
	return nil
}
