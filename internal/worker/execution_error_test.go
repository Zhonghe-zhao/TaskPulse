package worker

import (
	"errors"
	"fmt"
	"testing"
	"time"
)

func TestNewExecutionErrorCreatesTransientError(t *testing.T) {
	cause := errors.New("upstream unavailable")
	executionError, err := NewExecutionError(
		ErrorTransient,
		"upstream_5xx",
		5*time.Second,
		cause,
	)
	if err != nil {
		t.Fatalf("NewExecutionError returned error: %v", err)
	}
	if !executionError.Retryable() {
		t.Fatal("expected transient error to be retryable")
	}
	if !errors.Is(executionError, cause) {
		t.Fatal("expected execution error to preserve its cause")
	}
	if executionError.Error() != "upstream_5xx: upstream unavailable" {
		t.Fatalf("unexpected error text: %q", executionError.Error())
	}
}

func TestNewExecutionErrorRejectsInvalidMetadata(t *testing.T) {
	tests := []struct {
		name       string
		kind       ErrorKind
		code       string
		retryAfter time.Duration
		want       error
	}{
		{
			name: "unknown kind",
			kind: "unknown",
			code: "unknown_error",
			want: ErrInvalidErrorKind,
		},
		{
			name: "empty code",
			kind: ErrorTransient,
			want: ErrEmptyErrorCode,
		},
		{
			name:       "negative retry after",
			kind:       ErrorTransient,
			code:       "network_timeout",
			retryAfter: -time.Second,
			want:       ErrInvalidRetryAfter,
		},
		{
			name:       "permanent retry after",
			kind:       ErrorPermanent,
			code:       "invalid_input",
			retryAfter: time.Second,
			want:       ErrPermanentRetryAfter,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := NewExecutionError(test.kind, test.code, test.retryAfter, nil)
			if err == nil {
				t.Fatal("expected invalid execution error metadata to be rejected")
			}
			if test.want != nil && !errors.Is(err, test.want) {
				t.Fatalf("expected %v, got %v", test.want, err)
			}
		})
	}
}

func TestExecutionErrorValidateRejectsMutatedError(t *testing.T) {
	executionError, err := NewExecutionError(
		ErrorTransient,
		"network_timeout",
		0,
		nil,
	)
	if err != nil {
		t.Fatalf("NewExecutionError returned error: %v", err)
	}
	executionError.Code = " "

	if err := executionError.Validate(); !errors.Is(err, ErrEmptyErrorCode) {
		t.Fatalf("expected ErrEmptyErrorCode, got %v", err)
	}
}

func TestAsExecutionErrorFindsWrappedError(t *testing.T) {
	executionError, err := NewExecutionError(
		ErrorPermanent,
		"invalid_input",
		0,
		errors.New("missing documents"),
	)
	if err != nil {
		t.Fatalf("NewExecutionError returned error: %v", err)
	}

	got, ok := AsExecutionError(fmt.Errorf("execute workflow: %w", executionError))
	if !ok || got != executionError {
		t.Fatalf("expected wrapped execution error, got %#v, %t", got, ok)
	}
	if _, ok := AsExecutionError(errors.New("unclassified")); ok {
		t.Fatal("expected ordinary error to remain unclassified")
	}
}
