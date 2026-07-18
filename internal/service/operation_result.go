package service

import (
	"encoding/json"
	"fmt"
)

const operationResultPayloadSchema = "sema.operation-result.v1"

type persistedOperationResult struct {
	Schema  string          `json:"schema"`
	Kind    string          `json:"kind"`
	Payload json.RawMessage `json:"payload"`
}

func encodeOperationResult(kind string, value any) ([]byte, error) {
	if kind == "" {
		return nil, fmt.Errorf("operation result kind is required")
	}
	payload, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("encode operation result payload: %w", err)
	}
	encoded, err := json.Marshal(persistedOperationResult{
		Schema:  operationResultPayloadSchema,
		Kind:    kind,
		Payload: payload,
	})
	if err != nil {
		return nil, fmt.Errorf("encode operation result: %w", err)
	}
	return encoded, nil
}

func decodeOperationResult(payload []byte, kind string, target any) error {
	var stored persistedOperationResult
	if err := decodeStrict(payload, &stored); err != nil {
		return fmt.Errorf("decode operation result: %w", err)
	}
	if stored.Schema != operationResultPayloadSchema || stored.Kind != kind || len(stored.Payload) == 0 {
		return fmt.Errorf("operation result header is invalid")
	}
	if err := decodeStrict(stored.Payload, target); err != nil {
		return fmt.Errorf("decode %s operation result payload: %w", kind, err)
	}
	return nil
}
