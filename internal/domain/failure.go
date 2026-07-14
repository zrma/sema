package domain

import (
	"errors"
	"fmt"
)

// FailureCode is a machine-readable outcome that callers can use for retry policy.
type FailureCode string

const (
	FailureInvalidInput        FailureCode = "InvalidInput"
	FailureInvalidRevision     FailureCode = "InvalidRevision"
	FailureStaleSnapshot       FailureCode = "StaleSnapshot"
	FailureReservationConflict FailureCode = "ReservationConflict"
	FailureReservationExpired  FailureCode = "ReservationExpired"
	FailureInvalidTransition   FailureCode = "InvalidTransition"
	FailureIdempotencyConflict FailureCode = "IdempotencyConflict"
)

// Failure carries a stable code while keeping the detail suitable for diagnostics.
type Failure struct {
	Code   FailureCode
	Detail string
}

func (f *Failure) Error() string {
	if f.Detail == "" {
		return string(f.Code)
	}
	return fmt.Sprintf("%s: %s", f.Code, f.Detail)
}

// NewFailure constructs a typed domain failure.
func NewFailure(code FailureCode, format string, args ...any) error {
	return &Failure{Code: code, Detail: fmt.Sprintf(format, args...)}
}

// FailureCodeOf extracts the stable code from a typed domain failure.
func FailureCodeOf(err error) (FailureCode, bool) {
	var failure *Failure
	if !errors.As(err, &failure) {
		return "", false
	}
	return failure.Code, true
}

// ValidFailureCode reports whether a code belongs to the stable domain outcome set.
func ValidFailureCode(code FailureCode) bool {
	switch code {
	case FailureInvalidInput,
		FailureInvalidRevision,
		FailureStaleSnapshot,
		FailureReservationConflict,
		FailureReservationExpired,
		FailureInvalidTransition,
		FailureIdempotencyConflict:
		return true
	default:
		return false
	}
}
