package worker

import (
	"errors"
	"math/rand"
	"sync"
	"time"
)

var (
	ErrInvalidMaxRetries = errors.New("max retries cannot be negative")
	ErrInvalidBaseDelay  = errors.New("base delay must be positive")
	ErrInvalidMaxDelay   = errors.New("max delay must be greater than or equal to base delay")
	ErrInvalidRetryCount = errors.New("retry count is outside policy budget")
	ErrNilJitterSource   = errors.New("jitter source is nil")
)

type RetryPolicy struct {
	MaxRetries int
	BaseDelay  time.Duration
	MaxDelay   time.Duration
}

func (p RetryPolicy) Validate() error {
	if p.MaxRetries < 0 {
		return ErrInvalidMaxRetries
	}
	if p.BaseDelay <= 0 {
		return ErrInvalidBaseDelay
	}
	if p.MaxDelay < p.BaseDelay {
		return ErrInvalidMaxDelay
	}
	return nil
}

type JitterSource interface {
	Int63n(n int64) int64
}

type BackoffCalculator struct {
	mu     sync.Mutex
	jitter JitterSource
}

func NewBackoffCalculator(jitter JitterSource) (*BackoffCalculator, error) {
	if jitter == nil {
		return nil, ErrNilJitterSource
	}
	return &BackoffCalculator{jitter: jitter}, nil
}

func NewDefaultBackoffCalculator() *BackoffCalculator {
	return &BackoffCalculator{
		jitter: rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

func (c *BackoffCalculator) Delay(
	policy RetryPolicy,
	retryCount int,
	retryAfter time.Duration,
) (time.Duration, error) {
	if err := policy.Validate(); err != nil {
		return 0, err
	}
	if retryCount < 1 || retryCount > policy.MaxRetries {
		return 0, ErrInvalidRetryCount
	}
	if retryAfter < 0 {
		return 0, ErrInvalidRetryAfter
	}

	capDelay := policy.BaseDelay
	for retry := 1; retry < retryCount && capDelay < policy.MaxDelay; retry++ {
		if capDelay > policy.MaxDelay/2 {
			capDelay = policy.MaxDelay
			break
		}
		capDelay *= 2
	}

	half := capDelay / 2
	delay := half
	if half > 0 {
		c.mu.Lock()
		jitter := c.jitter.Int63n(int64(half) + 1)
		c.mu.Unlock()
		delay += time.Duration(jitter)
	} else {
		delay = capDelay
	}
	if retryAfter > delay {
		return retryAfter, nil
	}
	return delay, nil
}
