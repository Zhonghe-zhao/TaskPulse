package worker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/zhaozhonghe/taskpulse/internal/domain"
	"github.com/zhaozhonghe/taskpulse/internal/identity"
	"github.com/zhaozhonghe/taskpulse/internal/store"
)

type Reaper struct {
	taskStore  store.TaskStore
	eventStore store.EventStore
	now        func() time.Time
}

func NewReaper(taskStore store.TaskStore, eventStore store.EventStore) *Reaper {
	return &Reaper{
		taskStore:  taskStore,
		eventStore: eventStore,
		now:        time.Now,
	}
}

func (r *Reaper) ProcessNext(ctx context.Context) (bool, error) {
	task, err := r.taskStore.FailNextExpired(ctx, r.now())
	if errors.Is(err, store.ErrNoExpiredTask) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("fail expired task: %w", err)
	}

	event, err := domain.NewTaskEvent(
		identity.New("event"),
		task.ID,
		domain.EventTaskFailed,
		"task failed after lease expiration",
		json.RawMessage(`{"reason":"retry_budget_exhausted"}`),
		task.Progress,
		r.now(),
	)
	if err != nil {
		return true, fmt.Errorf("create expired task failure event: %w", err)
	}
	if err := r.eventStore.Append(ctx, event); err != nil {
		return true, fmt.Errorf("append expired task failure event: %w", err)
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
