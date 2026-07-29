package worker

import (
	"errors"
	"testing"
	"time"
)

type minimumJitter struct{}

func (minimumJitter) Int63n(int64) int64 {
	return 0
}

type maximumJitter struct{}

func (maximumJitter) Int63n(n int64) int64 {
	return n - 1
}

func TestRetryPolicyValidate(t *testing.T) {
	valid := RetryPolicy{
		MaxRetries: 3,
		BaseDelay:  time.Second,
		MaxDelay:   8 * time.Second,
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("Validate returned error: %v", err)
	}

	tests := []struct {
		name   string
		policy RetryPolicy
		want   error
	}{
		{
			name: "negative retries",
			policy: RetryPolicy{
				MaxRetries: -1,
				BaseDelay:  time.Second,
				MaxDelay:   time.Second,
			},
			want: ErrInvalidMaxRetries,
		},
		{
			name: "zero base delay",
			policy: RetryPolicy{
				MaxRetries: 1,
				MaxDelay:   time.Second,
			},
			want: ErrInvalidBaseDelay,
		},
		{
			name: "max below base",
			policy: RetryPolicy{
				MaxRetries: 1,
				BaseDelay:  2 * time.Second,
				MaxDelay:   time.Second,
			},
			want: ErrInvalidMaxDelay,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := test.policy.Validate(); !errors.Is(err, test.want) {
				t.Fatalf("expected %v, got %v", test.want, err)
			}
		})
	}
}

func TestBackoffCalculatorUsesEqualJitterAndExponentialCap(t *testing.T) {
	policy := RetryPolicy{
		MaxRetries: 5,
		BaseDelay:  2 * time.Second,
		MaxDelay:   8 * time.Second,
	}
	minimum, err := NewBackoffCalculator(minimumJitter{})
	if err != nil {
		t.Fatalf("NewBackoffCalculator returned error: %v", err)
	}
	maximum, err := NewBackoffCalculator(maximumJitter{})
	if err != nil {
		t.Fatalf("NewBackoffCalculator returned error: %v", err)
	}

	tests := []struct {
		name       string
		calculator *BackoffCalculator
		retryCount int
		want       time.Duration
	}{
		{name: "first retry minimum jitter", calculator: minimum, retryCount: 1, want: time.Second},
		{name: "second retry maximum jitter", calculator: maximum, retryCount: 2, want: 4 * time.Second},
		{name: "fourth retry remains capped", calculator: maximum, retryCount: 4, want: 8 * time.Second},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			delay, err := test.calculator.Delay(policy, test.retryCount, 0)
			if err != nil {
				t.Fatalf("Delay returned error: %v", err)
			}
			if delay != test.want {
				t.Fatalf("expected delay %s, got %s", test.want, delay)
			}
		})
	}
}

func TestBackoffCalculatorHonorsRetryAfter(t *testing.T) {
	calculator, err := NewBackoffCalculator(minimumJitter{})
	if err != nil {
		t.Fatalf("NewBackoffCalculator returned error: %v", err)
	}
	policy := RetryPolicy{
		MaxRetries: 3,
		BaseDelay:  2 * time.Second,
		MaxDelay:   8 * time.Second,
	}

	delay, err := calculator.Delay(policy, 1, 30*time.Second)
	if err != nil {
		t.Fatalf("Delay returned error: %v", err)
	}
	if delay != 30*time.Second {
		t.Fatalf("expected RetryAfter to win, got %s", delay)
	}
}

func TestBackoffCalculatorKeepsSubNanosecondHalfPositive(t *testing.T) {
	calculator, err := NewBackoffCalculator(minimumJitter{})
	if err != nil {
		t.Fatalf("NewBackoffCalculator returned error: %v", err)
	}
	policy := RetryPolicy{
		MaxRetries: 1,
		BaseDelay:  time.Nanosecond,
		MaxDelay:   time.Nanosecond,
	}

	delay, err := calculator.Delay(policy, 1, 0)
	if err != nil {
		t.Fatalf("Delay returned error: %v", err)
	}
	if delay != time.Nanosecond {
		t.Fatalf("expected positive delay, got %s", delay)
	}
}

func TestBackoffCalculatorRejectsRetryOutsideBudget(t *testing.T) {
	calculator, err := NewBackoffCalculator(minimumJitter{})
	if err != nil {
		t.Fatalf("NewBackoffCalculator returned error: %v", err)
	}
	policy := RetryPolicy{
		MaxRetries: 2,
		BaseDelay:  time.Second,
		MaxDelay:   2 * time.Second,
	}

	for _, retryCount := range []int{0, 3} {
		if _, err := calculator.Delay(policy, retryCount, 0); !errors.Is(err, ErrInvalidRetryCount) {
			t.Fatalf("retry count %d: expected ErrInvalidRetryCount, got %v", retryCount, err)
		}
	}
}
