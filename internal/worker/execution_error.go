package worker

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

type ErrorKind string

const (
	ErrorTransient ErrorKind = "transient"
	ErrorPermanent ErrorKind = "permanent"
)

var (
	ErrInvalidErrorKind    = errors.New("invalid execution error kind")
	ErrEmptyErrorCode      = errors.New("execution error code is required")
	ErrInvalidRetryAfter   = errors.New("retry after cannot be negative")
	ErrPermanentRetryAfter = errors.New("permanent execution error cannot specify retry after")
	ErrNilExecutionError   = errors.New("execution error is nil")
)

type ExecutionError struct {
	Kind       ErrorKind
	Code       string
	RetryAfter time.Duration
	Err        error
}

func NewExecutionError(
	kind ErrorKind,
	code string,
	retryAfter time.Duration,
	cause error,
) (*ExecutionError, error) {
	executionError := &ExecutionError{
		Kind:       kind,
		Code:       code,
		RetryAfter: retryAfter,
		Err:        cause,
	}
	if err := executionError.Validate(); err != nil {
		return nil, err
	}
	return executionError, nil
}

func (e *ExecutionError) Validate() error {
	if e == nil {
		return ErrNilExecutionError
	}
	if e.Kind != ErrorTransient && e.Kind != ErrorPermanent {
		return ErrInvalidErrorKind
	}
	if strings.TrimSpace(e.Code) == "" {
		return ErrEmptyErrorCode
	}
	if e.RetryAfter < 0 {
		return ErrInvalidRetryAfter
	}
	if e.Kind == ErrorPermanent && e.RetryAfter != 0 {
		return ErrPermanentRetryAfter
	}
	return nil
}

func (e *ExecutionError) Error() string {
	if e == nil {
		return "execution error"
	}
	if e.Err == nil {
		return e.Code
	}
	return fmt.Sprintf("%s: %v", e.Code, e.Err)
}

func (e *ExecutionError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

func (e *ExecutionError) Retryable() bool {
	return e != nil && e.Kind == ErrorTransient
}

func AsExecutionError(err error) (*ExecutionError, bool) {
	var executionError *ExecutionError
	if !errors.As(err, &executionError) {
		return nil, false
	}
	return executionError, true
}
