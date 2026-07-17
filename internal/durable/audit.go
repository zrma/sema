package durable

import (
	"fmt"

	"github.com/zrma/sema/internal/domain"
)

// AuditSummary is a redacted operational view of one durable record.
type AuditSummary struct {
	Sequence uint64
	Kind     string
	Checksum string
	Counts   map[string]int
	Flags    map[string]bool
	Outcome  string
}

// AuditSummaries returns records without resource identities or raw payloads.
func (runtime *Runtime) AuditSummaries(after uint64, limit int) ([]AuditSummary, error) {
	records, err := runtime.Audit(after, limit)
	if err != nil {
		return nil, err
	}
	summaries := make([]AuditSummary, len(records))
	for index, record := range records {
		summary, err := summarizeRecord(record)
		if err != nil {
			return nil, fmt.Errorf("summarize durable event %d: %w", record.Sequence, err)
		}
		summaries[index] = summary
	}
	return summaries, nil
}

func summarizeRecord(record Record) (AuditSummary, error) {
	summary := AuditSummary{
		Sequence: record.Sequence, Kind: record.Kind, Checksum: record.Checksum,
		Counts: make(map[string]int), Flags: make(map[string]bool),
	}
	switch eventKind(record.Kind) {
	case eventRuntimeConfigured:
		var event runtimeConfiguredEvent
		if err := decodePayload(record, &event); err != nil {
			return AuditSummary{}, err
		}
	case eventPolicyRegistered:
		var event policyRegisteredEvent
		if err := decodePayload(record, &event); err != nil {
			return AuditSummary{}, err
		}
		summary.Counts["role_requirements"] = len(event.Policy.RoleRequirements)
		summary.Counts["relaxation_steps"] = len(event.Policy.RelaxationSteps)
	case eventMatchTicketSubmitted:
		var event matchTicketSubmittedEvent
		if err := decodePayload(record, &event); err != nil {
			return AuditSummary{}, err
		}
		summary.Counts["players"] = len(event.Ticket.Players)
	case eventBackfillSubmitted:
		var event backfillSubmittedEvent
		if err := decodePayload(record, &event); err != nil {
			return AuditSummary{}, err
		}
		for _, slots := range event.Ticket.OpenSlotsByTeam {
			summary.Counts["open_slots"] += slots
		}
		summary.Counts["teams"] = len(event.Ticket.OpenSlotsByTeam)
	case eventMatchTicketCancelled:
		var event matchTicketCancelledEvent
		if err := decodePayload(record, &event); err != nil {
			return AuditSummary{}, err
		}
	case eventBackfillCancelled:
		var event backfillCancelledEvent
		if err := decodePayload(record, &event); err != nil {
			return AuditSummary{}, err
		}
	case eventPlanCompleted:
		var event planCompletedEvent
		if err := decodePayload(record, &event); err != nil {
			return AuditSummary{}, err
		}
		summary.Counts["proposals"] = len(event.Proposals)
		summary.Counts["unmatched_tickets"] = event.UnmatchedTickets
		summary.Flags["budget_exhausted"] = event.BudgetExhausted
	case eventProposalReserved:
		var event proposalReservedEvent
		if err := decodePayload(record, &event); err != nil {
			return AuditSummary{}, err
		}
		summary.Counts["tickets"] = len(event.Proposal.Tickets)
		summary.Flags["backfill"] = event.Proposal.Backfill != nil
	case eventReservationConfirmed:
		var event reservationConfirmedEvent
		if err := decodePayload(record, &event); err != nil {
			return AuditSummary{}, err
		}
		summary.Outcome = string(domain.AssignmentPending)
	case eventReservationCancelled:
		var event reservationCancelledEvent
		if err := decodePayload(record, &event); err != nil {
			return AuditSummary{}, err
		}
		summary.Outcome = string(domain.ReservationCancelled)
	case eventAssignmentAcked:
		var event assignmentAcknowledgedEvent
		if err := decodePayload(record, &event); err != nil {
			return AuditSummary{}, err
		}
		summary.Outcome = string(event.Request.Outcome)
	default:
		return AuditSummary{}, fmt.Errorf("unsupported event kind %q", record.Kind)
	}
	if len(summary.Counts) == 0 {
		summary.Counts = nil
	}
	if len(summary.Flags) == 0 {
		summary.Flags = nil
	}
	return summary, nil
}
