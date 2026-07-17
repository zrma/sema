package alpha

import (
	"errors"
	"fmt"

	"github.com/zrma/sema/internal/domain"
)

type ErrorCode string

const (
	ErrorInvalidInput ErrorCode = "InvalidInput"
	ErrorInternal     ErrorCode = "Internal"
)

type Error struct {
	Code    ErrorCode
	Message string
}

func (failure *Error) Error() string {
	if failure.Message == "" {
		return string(failure.Code)
	}
	return fmt.Sprintf("%s: %s", failure.Code, failure.Message)
}

func ErrorCodeOf(err error) (ErrorCode, bool) {
	var failure *Error
	if !errors.As(err, &failure) {
		return "", false
	}
	return failure.Code, true
}

func translateError(err error) error {
	var failure *domain.Failure
	if errors.As(err, &failure) {
		if failure.Code == domain.FailureInvalidInput {
			return &Error{Code: ErrorInvalidInput, Message: failure.Detail}
		}
	}
	return &Error{Code: ErrorInternal, Message: err.Error()}
}
