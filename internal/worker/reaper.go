package worker

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/zhaozhonghe/taskpulse/internal/identity"
	"github.com/zhaozhonghe/taskpulse/internal/store"
)

type Reaper struct {
	transitionStore store.TaskTransitionStore
	now             func() time.Time
}

func NewReaper(transitionStore store.TaskTransitionStore) *Reaper {
	return &Reaper{
		transitionStore: transitionStore,
		now:             time.Now,
	}
}

func (r *Reaper) ProcessNext(ctx context.Context) (bool, error) {
	_, err := r.transitionStore.FailNextExpiredWithEvent(
		ctx,
		r.now(),
		identity.New("event"),
	)
	if errors.Is(err, store.ErrNoExpiredTask) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("fail expired task and append event: %w", err)
	}
	return true, nil
}

func (r *Reaper) Run(ctx context.Context, pollInterval time.Duration) error {
	if pollInterval <= 0 {
		return errors.New("poll interval must be positive")
	}

	for {
		processed, err := r.ProcessNext(ctx)
		if err != nil {
			if errors.Is(err, context.Canceled) {
				return nil
			}
			return err
		}
		if processed {
			continue
		}

		timer := time.NewTimer(pollInterval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil
		case <-timer.C:
		}
	}
}
