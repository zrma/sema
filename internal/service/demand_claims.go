package service

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/zrma/sema/internal/domain"
)

const (
	demandIdentityClaimSchema  = "sema.demand-identity-claim.v1"
	backfillSessionClaimSchema = "sema.backfill-session-claim.v1"
)

type demandIdentityClaim struct {
	Schema string       `json:"schema"`
	Kind   ResourceKind `json:"kind"`
	ID     string       `json:"id"`
}

type backfillSessionClaim struct {
	Schema    string           `json:"schema"`
	SessionID domain.SessionID `json:"session_id"`
	TicketID  domain.TicketID  `json:"ticket_id"`
}

func encodeDemandIdentityClaim(kind ResourceKind, id string) ([]byte, error) {
	encoded, err := json.Marshal(demandIdentityClaim{
		Schema: demandIdentityClaimSchema, Kind: kind, ID: id,
	})
	if err != nil {
		return nil, fmt.Errorf("encode demand identity claim: %w", err)
	}
	return encoded, nil
}

func decodeDemandIdentityClaim(payload []byte) (demandIdentityClaim, error) {
	var claim demandIdentityClaim
	if err := decodeStrict(payload, &claim); err != nil {
		return demandIdentityClaim{}, fmt.Errorf("decode demand identity claim: %w", err)
	}
	if claim.Schema != demandIdentityClaimSchema || !claim.Kind.Valid() || claim.ID == "" {
		return demandIdentityClaim{}, fmt.Errorf("demand identity claim is invalid")
	}
	return claim, nil
}

func encodeBackfillSessionClaim(ticket domain.BackfillTicket) ([]byte, error) {
	encoded, err := json.Marshal(backfillSessionClaim{
		Schema: backfillSessionClaimSchema, SessionID: ticket.SessionID, TicketID: ticket.ID,
	})
	if err != nil {
		return nil, fmt.Errorf("encode backfill session claim: %w", err)
	}
	return encoded, nil
}

func decodeBackfillSessionClaim(payload []byte) (backfillSessionClaim, error) {
	var claim backfillSessionClaim
	if err := decodeStrict(payload, &claim); err != nil {
		return backfillSessionClaim{}, fmt.Errorf("decode backfill session claim: %w", err)
	}
	if claim.Schema != backfillSessionClaimSchema || claim.SessionID == "" || claim.TicketID == "" {
		return backfillSessionClaim{}, fmt.Errorf("backfill session claim is invalid")
	}
	return claim, nil
}

func decodeStrict(payload []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return errors.New("payload has trailing data")
	}
	return nil
}
