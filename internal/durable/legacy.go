package durable

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/zrma/sema/internal/domain"
)

type LegacyEventKind string

const (
	LegacyRuntimeConfigured      LegacyEventKind = "runtime_configured"
	LegacyPolicyRegistered       LegacyEventKind = "policy_registered"
	LegacyMatchTicketSubmitted   LegacyEventKind = "match_ticket_submitted"
	LegacyBackfillSubmitted      LegacyEventKind = "backfill_ticket_submitted"
	LegacyMatchTicketCancelled   LegacyEventKind = "match_ticket_cancelled"
	LegacyBackfillCancelled      LegacyEventKind = "backfill_ticket_cancelled"
	LegacyPlanCompleted          LegacyEventKind = "plan_completed"
	LegacyProposalReserved       LegacyEventKind = "proposal_reserved"
	LegacyReservationConfirmed   LegacyEventKind = "reservation_confirmed"
	LegacyReservationCancelled   LegacyEventKind = "reservation_cancelled"
	LegacyAssignmentAcknowledged LegacyEventKind = "assignment_acknowledged"
)

type LegacyJournal struct {
	Schema         string
	Digest         string
	RecordCount    int
	ReservationTTL time.Duration
	Events         []LegacyEvent
}

type LegacyEvent struct {
	Sequence uint64
	Kind     LegacyEventKind

	Policy                  *domain.MatchmakingPolicy
	MatchTicket             *domain.MatchTicket
	BackfillTicket          *domain.BackfillTicket
	MatchTicketCancellation *LegacyMatchTicketCancellation
	BackfillCancellation    *LegacyBackfillCancellation
	Plan                    *LegacyPlan
	Reservation             *LegacyReservation
	Confirmation            *LegacyConfirmation
	ReservationCancellation *LegacyReservationCancellation
	Acknowledgment          *LegacyAcknowledgment
}

type LegacyMatchTicketCancellation struct {
	TicketID domain.TicketID
	Revision domain.Revision
}

type LegacyBackfillCancellation struct {
	TicketID      domain.TicketID
	Revision      domain.Revision
	RosterVersion domain.Revision
}

type LegacyPlan struct {
	SnapshotID    domain.SnapshotID
	Now           time.Time
	PolicyVersion string
	Batch         domain.ProposalBatch
}

type LegacyReservation struct {
	Proposal      domain.MatchProposal
	ReservationID domain.ReservationID
	Now           time.Time
}

type LegacyConfirmation struct {
	ReservationID domain.ReservationID
	AssignmentID  domain.AssignmentID
	Now           time.Time
}

type LegacyReservationCancellation struct {
	ReservationID domain.ReservationID
	Now           time.Time
}

type LegacyAcknowledgment struct {
	AssignmentID domain.AssignmentID
	Request      domain.AssignmentAcknowledgmentRequest
	Now          time.Time
}

// ReadLegacyJournal reads and validates a stable V0 journal snapshot without
// locking, truncating, chmodding, or otherwise writing to the source file.
func ReadLegacyJournal(path string) (LegacyJournal, error) {
	if path == "" {
		return LegacyJournal{}, fmt.Errorf("legacy journal path is required")
	}
	file, err := os.Open(path)
	if err != nil {
		return LegacyJournal{}, fmt.Errorf("open legacy journal read-only: %w", err)
	}
	defer file.Close()
	before, err := file.Stat()
	if err != nil {
		return LegacyJournal{}, fmt.Errorf("stat legacy journal before read: %w", err)
	}
	if !before.Mode().IsRegular() {
		return LegacyJournal{}, fmt.Errorf("legacy journal must be a regular file")
	}
	contents, err := io.ReadAll(file)
	if err != nil {
		return LegacyJournal{}, fmt.Errorf("read legacy journal: %w", err)
	}
	after, err := file.Stat()
	if err != nil {
		return LegacyJournal{}, fmt.Errorf("stat legacy journal after read: %w", err)
	}
	if before.Size() != after.Size() || before.ModTime() != after.ModTime() || int64(len(contents)) != after.Size() {
		return LegacyJournal{}, fmt.Errorf("legacy journal changed while it was read")
	}
	if len(contents) == 0 || contents[len(contents)-1] != '\n' {
		return LegacyJournal{}, fmt.Errorf("legacy journal has an incomplete tail")
	}
	records, err := decodeReadOnlyRecords(contents)
	if err != nil {
		return LegacyJournal{}, err
	}
	var configuration runtimeConfiguredEvent
	if eventKind(records[0].Kind) != eventRuntimeConfigured {
		return LegacyJournal{}, fmt.Errorf("legacy journal is missing its runtime configuration")
	}
	if err := decodePayload(records[0], &configuration); err != nil {
		return LegacyJournal{}, err
	}
	ttl := time.Duration(configuration.ReservationTTLNanos)
	if ttl <= 0 {
		return LegacyJournal{}, fmt.Errorf("legacy journal reservation TTL is invalid")
	}
	if _, err := replay(ttl, records); err != nil {
		return LegacyJournal{}, fmt.Errorf("validate legacy journal replay: %w", err)
	}
	events := make([]LegacyEvent, len(records))
	for index, record := range records {
		events[index], err = legacyEvent(record)
		if err != nil {
			return LegacyJournal{}, fmt.Errorf("decode legacy event %d: %w", record.Sequence, err)
		}
	}
	digest := sha256.Sum256(contents)
	return LegacyJournal{
		Schema: journalSchema, Digest: hex.EncodeToString(digest[:]), RecordCount: len(records),
		ReservationTTL: ttl, Events: events,
	}, nil
}

func decodeReadOnlyRecords(contents []byte) ([]Record, error) {
	lines := bytes.Split(contents, []byte{'\n'})
	records := make([]Record, 0, len(lines)-1)
	for _, line := range lines {
		if len(line) == 0 {
			continue
		}
		var record Record
		if err := decodeJSONStrict(line, &record); err != nil {
			return nil, fmt.Errorf("decode legacy journal record %d: %w", len(records)+1, err)
		}
		expected := uint64(len(records) + 1)
		if err := validateRecord(record, expected); err != nil {
			return nil, fmt.Errorf("validate legacy journal record %d: %w", expected, err)
		}
		records = append(records, record)
	}
	if len(records) == 0 {
		return nil, fmt.Errorf("legacy journal contains no records")
	}
	return records, nil
}

func legacyEvent(record Record) (LegacyEvent, error) {
	event := LegacyEvent{Sequence: record.Sequence, Kind: LegacyEventKind(record.Kind)}
	switch eventKind(record.Kind) {
	case eventRuntimeConfigured:
		var configuration runtimeConfiguredEvent
		if err := decodePayload(record, &configuration); err != nil {
			return LegacyEvent{}, err
		}
	case eventPolicyRegistered:
		var stored policyRegisteredEvent
		if err := decodePayload(record, &stored); err != nil {
			return LegacyEvent{}, err
		}
		policy := domain.ClonePolicy(stored.Policy)
		event.Policy = &policy
	case eventMatchTicketSubmitted:
		var stored matchTicketSubmittedEvent
		if err := decodePayload(record, &stored); err != nil {
			return LegacyEvent{}, err
		}
		ticket := domain.CloneMatchTicket(stored.Ticket)
		event.MatchTicket = &ticket
	case eventBackfillSubmitted:
		var stored backfillSubmittedEvent
		if err := decodePayload(record, &stored); err != nil {
			return LegacyEvent{}, err
		}
		ticket := domain.CloneBackfillTicket(stored.Ticket)
		event.BackfillTicket = &ticket
	case eventMatchTicketCancelled:
		var stored matchTicketCancelledEvent
		if err := decodePayload(record, &stored); err != nil {
			return LegacyEvent{}, err
		}
		event.MatchTicketCancellation = &LegacyMatchTicketCancellation{
			TicketID: stored.TicketID, Revision: stored.Revision,
		}
	case eventBackfillCancelled:
		var stored backfillCancelledEvent
		if err := decodePayload(record, &stored); err != nil {
			return LegacyEvent{}, err
		}
		event.BackfillCancellation = &LegacyBackfillCancellation{
			TicketID: stored.TicketID, Revision: stored.Revision, RosterVersion: stored.RosterVersion,
		}
	case eventPlanCompleted:
		var stored planCompletedEvent
		if err := decodePayload(record, &stored); err != nil {
			return LegacyEvent{}, err
		}
		event.Plan = &LegacyPlan{
			SnapshotID: stored.SnapshotID, Now: stored.Now.UTC(), PolicyVersion: stored.PolicyVersion,
			Batch: batchFromPlanEvent(stored),
		}
	case eventProposalReserved:
		var stored proposalReservedEvent
		if err := decodePayload(record, &stored); err != nil {
			return LegacyEvent{}, err
		}
		event.Reservation = &LegacyReservation{
			Proposal: domain.CloneProposal(stored.Proposal), ReservationID: stored.ReservationID, Now: stored.Now.UTC(),
		}
	case eventReservationConfirmed:
		var stored reservationConfirmedEvent
		if err := decodePayload(record, &stored); err != nil {
			return LegacyEvent{}, err
		}
		event.Confirmation = &LegacyConfirmation{
			ReservationID: stored.ReservationID, AssignmentID: stored.AssignmentID, Now: stored.Now.UTC(),
		}
	case eventReservationCancelled:
		var stored reservationCancelledEvent
		if err := decodePayload(record, &stored); err != nil {
			return LegacyEvent{}, err
		}
		event.ReservationCancellation = &LegacyReservationCancellation{
			ReservationID: stored.ReservationID, Now: stored.Now.UTC(),
		}
	case eventAssignmentAcked:
		var stored assignmentAcknowledgedEvent
		if err := decodePayload(record, &stored); err != nil {
			return LegacyEvent{}, err
		}
		event.Acknowledgment = &LegacyAcknowledgment{
			AssignmentID: stored.AssignmentID, Request: stored.Request, Now: stored.Now.UTC(),
		}
	default:
		return LegacyEvent{}, fmt.Errorf("unsupported legacy event kind %q", record.Kind)
	}
	return event, nil
}

func decodeJSONStrict(payload []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return fmt.Errorf("payload has trailing data")
	}
	return nil
}
