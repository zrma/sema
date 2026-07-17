package durable

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"reflect"
	"slices"
	"time"

	"github.com/zrma/sema/internal/domain"
	"github.com/zrma/sema/internal/engine"
)

type eventKind string

const (
	eventRuntimeConfigured    eventKind = "runtime_configured"
	eventPolicyRegistered     eventKind = "policy_registered"
	eventMatchTicketSubmitted eventKind = "match_ticket_submitted"
	eventBackfillSubmitted    eventKind = "backfill_ticket_submitted"
	eventMatchTicketCancelled eventKind = "match_ticket_cancelled"
	eventBackfillCancelled    eventKind = "backfill_ticket_cancelled"
	eventPlanCompleted        eventKind = "plan_completed"
	eventProposalReserved     eventKind = "proposal_reserved"
	eventReservationConfirmed eventKind = "reservation_confirmed"
	eventReservationCancelled eventKind = "reservation_cancelled"
	eventAssignmentAcked      eventKind = "assignment_acknowledged"
)

type runtimeConfiguredEvent struct {
	ReservationTTLNanos int64 `json:"reservation_ttl_nanos"`
}

type policyRegisteredEvent struct {
	Policy domain.MatchmakingPolicy `json:"policy"`
}

type matchTicketSubmittedEvent struct {
	Ticket domain.MatchTicket `json:"ticket"`
}

type backfillSubmittedEvent struct {
	Ticket domain.BackfillTicket `json:"ticket"`
}

type matchTicketCancelledEvent struct {
	TicketID domain.TicketID `json:"ticket_id"`
	Revision domain.Revision `json:"revision"`
}

type backfillCancelledEvent struct {
	TicketID      domain.TicketID `json:"ticket_id"`
	Revision      domain.Revision `json:"revision"`
	RosterVersion domain.Revision `json:"roster_version"`
}

type planCompletedEvent struct {
	SnapshotID       domain.SnapshotID         `json:"snapshot_id"`
	Now              time.Time                 `json:"now"`
	PolicyVersion    string                    `json:"policy_version"`
	Proposals        []domain.MatchProposal    `json:"proposals"`
	Unmatched        []domain.UnmatchedTicket  `json:"unmatched"`
	UnmatchedTickets int                       `json:"unmatched_tickets"`
	UnmatchedDigest  string                    `json:"unmatched_digest"`
	BudgetExhausted  bool                      `json:"budget_exhausted"`
	Evidence         domain.BatchScoreEvidence `json:"evidence"`
}

type proposalReservedEvent struct {
	Proposal      domain.MatchProposal `json:"proposal"`
	ReservationID domain.ReservationID `json:"reservation_id"`
	Now           time.Time            `json:"now"`
}

type reservationConfirmedEvent struct {
	ReservationID domain.ReservationID `json:"reservation_id"`
	AssignmentID  domain.AssignmentID  `json:"assignment_id"`
	Now           time.Time            `json:"now"`
}

type reservationCancelledEvent struct {
	ReservationID domain.ReservationID `json:"reservation_id"`
	Now           time.Time            `json:"now"`
}

type assignmentAcknowledgedEvent struct {
	AssignmentID domain.AssignmentID                    `json:"assignment_id"`
	Request      domain.AssignmentAcknowledgmentRequest `json:"request"`
	Now          time.Time                              `json:"now"`
}

func replay(reservationTTL time.Duration, records []Record) (*engine.Engine, error) {
	if len(records) == 0 || eventKind(records[0].Kind) != eventRuntimeConfigured {
		return nil, fmt.Errorf("durable journal is missing its runtime configuration")
	}
	runtime, err := engine.New(reservationTTL)
	if err != nil {
		return nil, err
	}
	for index, record := range records {
		if eventKind(record.Kind) == eventRuntimeConfigured {
			if index != 0 {
				return nil, fmt.Errorf("runtime configuration appears after the first durable event")
			}
			var event runtimeConfiguredEvent
			if err := decodePayload(record, &event); err != nil {
				return nil, err
			}
			if event.ReservationTTLNanos != int64(reservationTTL) {
				return nil, fmt.Errorf(
					"reservation TTL is %s; journal requires %s",
					reservationTTL,
					time.Duration(event.ReservationTTLNanos),
				)
			}
			continue
		}
		if err := replayRecord(runtime, record); err != nil {
			return nil, fmt.Errorf("replay durable event %d (%s): %w", record.Sequence, record.Kind, err)
		}
	}
	return runtime, nil
}

func newPlanCompletedEvent(
	snapshotID domain.SnapshotID,
	now time.Time,
	policyVersion string,
	batch domain.ProposalBatch,
) (planCompletedEvent, error) {
	unmatchedDigest, err := digestUnmatched(batch.Unmatched)
	if err != nil {
		return planCompletedEvent{}, err
	}
	proposals := make([]domain.MatchProposal, len(batch.Proposals))
	for index, proposal := range batch.Proposals {
		proposals[index] = domain.CloneProposal(proposal)
	}
	return planCompletedEvent{
		SnapshotID: snapshotID, Now: now, PolicyVersion: policyVersion,
		Proposals: proposals, Unmatched: slices.Clone(batch.Unmatched),
		UnmatchedTickets: len(batch.Unmatched),
		UnmatchedDigest:  unmatchedDigest, BudgetExhausted: batch.BudgetExhausted,
		Evidence: batch.Evidence,
	}, nil
}

func plansFromRecords(records []Record) (map[domain.SnapshotID]planCompletedEvent, error) {
	plans := make(map[domain.SnapshotID]planCompletedEvent)
	for _, record := range records {
		if eventKind(record.Kind) != eventPlanCompleted {
			continue
		}
		var event planCompletedEvent
		if err := decodePayload(record, &event); err != nil {
			return nil, fmt.Errorf("decode plan at event %d: %w", record.Sequence, err)
		}
		if event.SnapshotID == "" || len(event.Unmatched) != event.UnmatchedTickets {
			return nil, fmt.Errorf("plan event %d has incomplete batch content", record.Sequence)
		}
		digest, err := digestUnmatched(event.Unmatched)
		if err != nil {
			return nil, fmt.Errorf("validate plan event %d unmatched digest: %w", record.Sequence, err)
		}
		if digest != event.UnmatchedDigest {
			return nil, fmt.Errorf("plan event %d has an unmatched digest mismatch", record.Sequence)
		}
		if existing, exists := plans[event.SnapshotID]; exists && !reflect.DeepEqual(existing, event) {
			return nil, fmt.Errorf("snapshot %q has conflicting durable plan content", event.SnapshotID)
		}
		plans[event.SnapshotID] = clonePlanEvent(event)
	}
	return plans, nil
}

func digestUnmatched(unmatched []domain.UnmatchedTicket) (string, error) {
	encoded, err := json.Marshal(unmatched)
	if err != nil {
		return "", fmt.Errorf("encode unmatched decision digest: %w", err)
	}
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:]), nil
}

func proposalsFromPlans(plans map[domain.SnapshotID]planCompletedEvent) (map[domain.ProposalID]domain.MatchProposal, error) {
	proposals := make(map[domain.ProposalID]domain.MatchProposal)
	for _, event := range plans {
		for _, proposal := range event.Proposals {
			if existing, exists := proposals[proposal.ID]; exists && !reflect.DeepEqual(existing, proposal) {
				return nil, fmt.Errorf("proposal %q has conflicting durable plan content", proposal.ID)
			}
			proposals[proposal.ID] = domain.CloneProposal(proposal)
		}
	}
	return proposals, nil
}

func clonePlanEvent(event planCompletedEvent) planCompletedEvent {
	proposals := make([]domain.MatchProposal, len(event.Proposals))
	for index, proposal := range event.Proposals {
		proposals[index] = domain.CloneProposal(proposal)
	}
	event.Proposals = proposals
	event.Unmatched = slices.Clone(event.Unmatched)
	return event
}

func batchFromPlanEvent(event planCompletedEvent) domain.ProposalBatch {
	cloned := clonePlanEvent(event)
	return domain.ProposalBatch{
		SnapshotID: cloned.SnapshotID, Proposals: cloned.Proposals,
		Unmatched: cloned.Unmatched, BudgetExhausted: cloned.BudgetExhausted,
		Evidence: cloned.Evidence,
	}
}

func replayRecord(runtime *engine.Engine, record Record) error {
	switch eventKind(record.Kind) {
	case eventPolicyRegistered:
		var event policyRegisteredEvent
		if err := decodePayload(record, &event); err != nil {
			return err
		}
		_, err := runtime.RegisterPolicy(event.Policy)
		return err
	case eventMatchTicketSubmitted:
		var event matchTicketSubmittedEvent
		if err := decodePayload(record, &event); err != nil {
			return err
		}
		return runtime.SubmitMatchTicket(event.Ticket)
	case eventBackfillSubmitted:
		var event backfillSubmittedEvent
		if err := decodePayload(record, &event); err != nil {
			return err
		}
		return runtime.SubmitBackfillTicket(event.Ticket)
	case eventMatchTicketCancelled:
		var event matchTicketCancelledEvent
		if err := decodePayload(record, &event); err != nil {
			return err
		}
		return runtime.CancelMatchTicket(event.TicketID, event.Revision)
	case eventBackfillCancelled:
		var event backfillCancelledEvent
		if err := decodePayload(record, &event); err != nil {
			return err
		}
		return runtime.CancelBackfillTicket(event.TicketID, event.Revision, event.RosterVersion)
	case eventPlanCompleted:
		var event planCompletedEvent
		return decodePayload(record, &event)
	case eventProposalReserved:
		var event proposalReservedEvent
		if err := decodePayload(record, &event); err != nil {
			return err
		}
		_, err := runtime.Reserve(event.Proposal, event.ReservationID, event.Now)
		return err
	case eventReservationConfirmed:
		var event reservationConfirmedEvent
		if err := decodePayload(record, &event); err != nil {
			return err
		}
		_, err := runtime.Confirm(event.ReservationID, event.AssignmentID, event.Now)
		return err
	case eventReservationCancelled:
		var event reservationCancelledEvent
		if err := decodePayload(record, &event); err != nil {
			return err
		}
		_, err := runtime.CancelReservation(event.ReservationID, event.Now)
		return err
	case eventAssignmentAcked:
		var event assignmentAcknowledgedEvent
		if err := decodePayload(record, &event); err != nil {
			return err
		}
		_, err := runtime.AcknowledgeAssignment(event.AssignmentID, event.Request, event.Now)
		return err
	default:
		return fmt.Errorf("unsupported event kind %q", record.Kind)
	}
}

func decodePayload(record Record, target any) error {
	if err := json.Unmarshal(record.Payload, target); err != nil {
		return fmt.Errorf("decode payload: %w", err)
	}
	return nil
}
