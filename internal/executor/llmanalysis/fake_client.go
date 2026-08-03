package llmanalysis

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
)

const (
	FakeFailureNone            = ""
	FakeFailureRateLimitedOnce = "rate_limited_once"
)

var ErrInvalidFakeFailureMode = errors.New("invalid fake llm failure mode")
var ErrInvalidFakeDelay = errors.New("fake llm delay must not be negative")

type FakeClient struct {
	Model       string
	failureMode string
	delay       time.Duration
	mu          sync.Mutex
	failedOnce  map[string]bool
}

func NewFakeClient(model string) *FakeClient {
	if strings.TrimSpace(model) == "" {
		model = "fake-llm"
	}
	return &FakeClient{
		Model:      model,
		failedOnce: make(map[string]bool),
	}
}

func (c *FakeClient) SetFailureMode(mode string) error {
	mode = strings.TrimSpace(mode)
	switch mode {
	case FakeFailureNone, FakeFailureRateLimitedOnce:
		c.failureMode = mode
		return nil
	default:
		return ErrInvalidFakeFailureMode
	}
}

// SetDelay makes the fake provider slow enough for timeout and crash-recovery experiments.
func (c *FakeClient) SetDelay(delay time.Duration) error {
	if delay < 0 {
		return ErrInvalidFakeDelay
	}
	c.delay = delay
	return nil
}

func (c *FakeClient) Analyze(ctx context.Context, request AnalysisRequest) (AnalysisResponse, error) {
	if err := ctx.Err(); err != nil {
		return AnalysisResponse{}, err
	}
	if strings.TrimSpace(request.Subject) == "" {
		return AnalysisResponse{}, NewInvalidPromptError(ErrEmptySubject)
	}
	if strings.TrimSpace(request.Goal) == "" {
		return AnalysisResponse{}, NewInvalidPromptError(ErrEmptyGoal)
	}
	if err := c.wait(ctx); err != nil {
		return AnalysisResponse{}, err
	}
	if err := c.maybeFailOnce(request); err != nil {
		return AnalysisResponse{}, err
	}

	summary := fmt.Sprintf(
		"%s has %d note(s). Goal: %s.",
		strings.TrimSpace(request.Subject),
		len(request.Notes),
		strings.TrimSpace(request.Goal),
	)
	return AnalysisResponse{
		Summary: summary,
		Plan: []string{
			"identify key concepts",
			"group related notes",
			"produce an executable study plan",
		},
		Model: c.Model,
	}, nil
}

func (c *FakeClient) wait(ctx context.Context) error {
	if c.delay <= 0 {
		return nil
	}
	timer := time.NewTimer(c.delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func (c *FakeClient) maybeFailOnce(request AnalysisRequest) error {
	if c.failureMode != FakeFailureRateLimitedOnce {
		return nil
	}
	key := fakeRequestKey(request)

	c.mu.Lock()
	defer c.mu.Unlock()
	if c.failedOnce[key] {
		return nil
	}
	c.failedOnce[key] = true
	return NewProviderRateLimitError(time.Second, ErrProviderRateLimit)
}

func fakeRequestKey(request AnalysisRequest) string {
	return strings.TrimSpace(request.Subject) + "\x00" +
		strings.TrimSpace(request.Goal) + "\x00" +
		strings.Join(request.Notes, "\x00")
}
