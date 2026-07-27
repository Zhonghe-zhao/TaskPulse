package worker

import (
	"context"
	"encoding/json"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/zhaozhonghe/taskpulse/internal/domain"
	"github.com/zhaozhonghe/taskpulse/internal/store"
)

type fakeExecutor struct {
	result ExecutionResult
	err    error
}

type delayedExecutor struct {
	delay  time.Duration
	result ExecutionResult
}

func (e delayedExecutor) Execute(ctx context.Context, _ *domain.Task) (ExecutionResult, error) {
	timer := time.NewTimer(e.delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ExecutionResult{}, ctx.Err()
	case <-timer.C:
		return e.result, nil
	}
}

type countingTaskStore struct {
	store.TaskStore
	renewals atomic.Int32
}

type lostLeaseTaskStore struct {
	store.TaskStore
}

func (s *lostLeaseTaskStore) RenewLease(context.Context, store.RenewLeaseOptions) error {
	return store.ErrLeaseLost
}

func (s *countingTaskStore) RenewLease(ctx context.Context, options store.RenewLeaseOptions) error {
	s.renewals.Add(1)
	return s.TaskStore.RenewLease(ctx, options)
}

func (e fakeExecutor) Execute(context.Context, *domain.Task) (ExecutionResult, error) {
	return e.result, e.err
}

func TestWorkerCompletesTask(t *testing.T) {
	ctx := context.Background()
	taskStore := store.NewMemoryTaskStore()
	eventStore := store.NewMemoryEventStore()
	task := createTask(t, taskStore, eventStore)

	w := New(taskStore, eventStore, map[string]Executor{
		"url_check": fakeExecutor{result: ExecutionResult{
			Output:  json.RawMessage(`{"checked":1}`),
			Outcome: OutcomeSucceeded,
		}},
	})
	processed, err := w.ProcessNext(ctx)
	if err != nil {
		t.Fatalf("ProcessNext returned error: %v", err)
	}
	if !processed {
		t.Fatal("expected one task to be processed")
	}

	got, err := taskStore.Get(ctx, task.ID)
	if err != nil {
		t.Fatalf("Get returned error: %v", err)
	}
	if got.Status != domain.TaskStatusSucceeded || got.Progress != 100 {
		t.Fatalf("unexpected completed task: %+v", got)
	}
	if got.LeaseOwner != "" || got.LeaseExpiresAt != nil {
		t.Fatalf("expected completed task lease to be cleared: %+v", got)
	}

	events, err := eventStore.ListByTaskID(ctx, task.ID)
	if err != nil {
		t.Fatalf("ListByTaskID returned error: %v", err)
	}
	if len(events) != 3 {
		t.Fatalf("expected 3 events, got %d", len(events))
	}
	if events[1].Type != domain.EventTaskStarted || events[2].Type != domain.EventTaskSucceeded {
		t.Fatalf("unexpected event sequence: %+v", events)
	}
}

func TestWorkerPreservesPartialResult(t *testing.T) {
	ctx := context.Background()
	taskStore := store.NewMemoryTaskStore()
	eventStore := store.NewMemoryEventStore()
	task := createTask(t, taskStore, eventStore)

	w := New(taskStore, eventStore, map[string]Executor{
		"url_check": fakeExecutor{result: ExecutionResult{
			Output:       json.RawMessage(`{"succeeded":1,"failed":1}`),
			Outcome:      OutcomePartial,
			ErrorMessage: "1 of 2 URL checks failed",
		}},
	})
	if _, err := w.ProcessNext(ctx); err != nil {
		t.Fatalf("ProcessNext returned error: %v", err)
	}

	got, err := taskStore.Get(ctx, task.ID)
	if err != nil {
		t.Fatalf("Get returned error: %v", err)
	}
	if got.Status != domain.TaskStatusPartial {
		t.Fatalf("expected partially_succeeded, got %s", got.Status)
	}
	if len(got.Result) == 0 || got.ErrorMessage == "" {
		t.Fatalf("expected partial output and error summary, got %+v", got)
	}
}

func TestWorkerMarksExecutionFailure(t *testing.T) {
	ctx := context.Background()
	taskStore := store.NewMemoryTaskStore()
	eventStore := store.NewMemoryEventStore()
	task := createTask(t, taskStore, eventStore)

	w := New(taskStore, eventStore, map[string]Executor{
		"url_check": fakeExecutor{err: errors.New("request timed out")},
	})
	processed, err := w.ProcessNext(ctx)
	if err != nil {
		t.Fatalf("ProcessNext returned error: %v", err)
	}
	if !processed {
		t.Fatal("expected one task to be processed")
	}

	got, err := taskStore.Get(ctx, task.ID)
	if err != nil {
		t.Fatalf("Get returned error: %v", err)
	}
	if got.Status != domain.TaskStatusFailed || got.ErrorMessage != "request timed out" {
		t.Fatalf("unexpected failed task: %+v", got)
	}
}

func TestWorkerReturnsNoWork(t *testing.T) {
	w := New(store.NewMemoryTaskStore(), store.NewMemoryEventStore(), nil)
	processed, err := w.ProcessNext(context.Background())
	if err != nil {
		t.Fatalf("ProcessNext returned error: %v", err)
	}
	if processed {
		t.Fatal("expected no task to be processed")
	}
}

func TestWorkerRenewsLeaseDuringLongExecution(t *testing.T) {
	ctx := context.Background()
	memoryStore := store.NewMemoryTaskStore()
	taskStore := &countingTaskStore{TaskStore: memoryStore}
	eventStore := store.NewMemoryEventStore()
	createTask(t, taskStore, eventStore)

	w := New(taskStore, eventStore, map[string]Executor{
		"url_check": delayedExecutor{
			delay: 80 * time.Millisecond,
			result: ExecutionResult{
				Output:  json.RawMessage(`{"checked":1}`),
				Outcome: OutcomeSucceeded,
			},
		},
	})
	w.leaseDuration = 60 * time.Millisecond

	processed, err := w.ProcessNext(ctx)
	if err != nil {
		t.Fatalf("ProcessNext returned error: %v", err)
	}
	if !processed {
		t.Fatal("expected one task to be processed")
	}
	if taskStore.renewals.Load() == 0 {
		t.Fatal("expected at least one lease renewal")
	}
}

func TestWorkerStopsWritingAfterLeaseIsLost(t *testing.T) {
	ctx := context.Background()
	memoryStore := store.NewMemoryTaskStore()
	taskStore := &lostLeaseTaskStore{TaskStore: memoryStore}
	eventStore := store.NewMemoryEventStore()
	task := createTask(t, taskStore, eventStore)

	w := New(taskStore, eventStore, map[string]Executor{
		"url_check": delayedExecutor{
			delay: time.Second,
			result: ExecutionResult{
				Outcome: OutcomeSucceeded,
			},
		},
	})
	w.leaseDuration = 15 * time.Millisecond

	processed, err := w.ProcessNext(ctx)
	if !processed {
		t.Fatal("expected one task to be claimed")
	}
	if !errors.Is(err, store.ErrLeaseLost) {
		t.Fatalf("expected ErrLeaseLost, got %v", err)
	}

	stored, getErr := memoryStore.Get(ctx, task.ID)
	if getErr != nil {
		t.Fatalf("Get returned error: %v", getErr)
	}
	if stored.Status != domain.TaskStatusRunning {
		t.Fatalf("worker without lease changed task status to %s", stored.Status)
	}
}

func TestWorkerEmitsRecoveredEventForExpiredTask(t *testing.T) {
	ctx := context.Background()
	taskStore := store.NewMemoryTaskStore()
	eventStore := store.NewMemoryEventStore()
	task := createTask(t, taskStore, eventStore)

	claimedAt := task.CreatedAt.Add(time.Minute)
	if _, err := taskStore.ClaimNext(ctx, store.ClaimOptions{
		WorkerID:      "crashed_worker",
		Now:           claimedAt,
		LeaseDuration: time.Minute,
	}); err != nil {
		t.Fatalf("initial ClaimNext returned error: %v", err)
	}

	recoveredAt := claimedAt.Add(time.Minute)
	w := New(taskStore, eventStore, map[string]Executor{
		"url_check": fakeExecutor{result: ExecutionResult{
			Output:  json.RawMessage(`{"checked":1}`),
			Outcome: OutcomeSucceeded,
		}},
	})
	w.id = "recovery_worker"
	w.leaseDuration = time.Minute
	w.now = func() time.Time { return recoveredAt }

	processed, err := w.ProcessNext(ctx)
	if err != nil {
		t.Fatalf("ProcessNext returned error: %v", err)
	}
	if !processed {
		t.Fatal("expected expired task to be recovered")
	}

	events, err := eventStore.ListByTaskID(ctx, task.ID)
	if err != nil {
		t.Fatalf("ListByTaskID returned error: %v", err)
	}
	if len(events) != 3 {
		t.Fatalf("expected 3 events, got %d", len(events))
	}
	if events[1].Type != domain.EventTaskRecovered {
		t.Fatalf("expected recovered event, got %s", events[1].Type)
	}
}

func createTask(t *testing.T, taskStore store.TaskStore, eventStore store.EventStore) *domain.Task {
	t.Helper()
	now := time.Date(2026, 7, 14, 12, 0, 0, 0, time.UTC)
	task, err := domain.NewTask("task_1", "url_check", json.RawMessage(`{"urls":["https://example.com"]}`), 3, now)
	if err != nil {
		t.Fatalf("NewTask returned error: %v", err)
	}
	if err := taskStore.Create(context.Background(), task); err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	event, err := domain.NewTaskEvent("event_created", task.ID, domain.EventTaskCreated, "task created", nil, 0, now)
	if err != nil {
		t.Fatalf("NewTaskEvent returned error: %v", err)
	}
	if err := eventStore.Append(context.Background(), event); err != nil {
		t.Fatalf("Append returned error: %v", err)
	}
	return task
}
