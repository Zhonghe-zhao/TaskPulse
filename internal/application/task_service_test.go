package application

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/zhaozhonghe/taskpulse/internal/domain"
	"github.com/zhaozhonghe/taskpulse/internal/store"
)

func TestTaskServiceCreateTask(t *testing.T) {
	ctx := context.Background()
	service := newMemoryTaskService()

	result, err := service.CreateTask(ctx, CreateTaskInput{
		Workflow:   "url_check",
		Input:      json.RawMessage(`{"urls":["https://example.com"]}`),
		MaxRetries: 3,
	})
	if err != nil {
		t.Fatalf("CreateTask returned error: %v", err)
	}
	task := result.Task
	if !result.Created {
		t.Fatal("expected first request to create a task")
	}
	if task.ID == "" {
		t.Fatal("expected generated task id")
	}
	if task.Status != domain.TaskStatusQueued {
		t.Fatalf("expected status %s, got %s", domain.TaskStatusQueued, task.Status)
	}

	got, err := service.GetTask(ctx, task.ID)
	if err != nil {
		t.Fatalf("GetTask returned error: %v", err)
	}
	if got.Workflow != "url_check" {
		t.Fatalf("expected workflow url_check, got %s", got.Workflow)
	}

	events, err := service.ListTaskEvents(ctx, task.ID)
	if err != nil {
		t.Fatalf("ListTaskEvents returned error: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].Type != domain.EventTaskCreated {
		t.Fatalf("expected event type %s, got %s", domain.EventTaskCreated, events[0].Type)
	}
}

func TestTaskServiceReplaysIdempotentCreation(t *testing.T) {
	ctx := context.Background()
	service := newMemoryTaskService()
	first, err := service.CreateTask(ctx, CreateTaskInput{
		IdempotencyKey: "memobridge-analysis-1",
		Workflow:       "llm_analysis",
		Input:          json.RawMessage(`{"subject":"go","notes":[1,2]}`),
		MaxRetries:     3,
	})
	if err != nil {
		t.Fatalf("first CreateTask returned error: %v", err)
	}
	replayed, err := service.CreateTask(ctx, CreateTaskInput{
		IdempotencyKey: "memobridge-analysis-1",
		Workflow:       "llm_analysis",
		Input:          json.RawMessage(`{"notes":[1,2],"subject":"go"}`),
		MaxRetries:     3,
	})
	if err != nil {
		t.Fatalf("replayed CreateTask returned error: %v", err)
	}
	if !first.Created || replayed.Created {
		t.Fatalf("unexpected creation flags: first=%t replayed=%t", first.Created, replayed.Created)
	}
	if first.Task.ID != replayed.Task.ID {
		t.Fatalf("expected replayed task %s, got %s", first.Task.ID, replayed.Task.ID)
	}
	events, err := service.ListTaskEvents(ctx, first.Task.ID)
	if err != nil {
		t.Fatalf("ListTaskEvents returned error: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected one created event after replay, got %d", len(events))
	}
}

func TestTaskServiceCreatesDistinctTasksWithoutIdempotencyKey(t *testing.T) {
	ctx := context.Background()
	service := newMemoryTaskService()
	input := CreateTaskInput{
		Workflow:   "llm_analysis",
		Input:      json.RawMessage(`{"subject":"go"}`),
		MaxRetries: 3,
	}

	first, err := service.CreateTask(ctx, input)
	if err != nil {
		t.Fatalf("first CreateTask returned error: %v", err)
	}
	second, err := service.CreateTask(ctx, input)
	if err != nil {
		t.Fatalf("second CreateTask returned error: %v", err)
	}
	if !first.Created || !second.Created {
		t.Fatalf("expected both requests to create tasks: first=%t second=%t", first.Created, second.Created)
	}
	if first.Task.ID == second.Task.ID {
		t.Fatalf("expected distinct task IDs without idempotency key, got %s", first.Task.ID)
	}
}

func TestTaskServiceTreatsIdempotencyKeysAsCaseSensitive(t *testing.T) {
	ctx := context.Background()
	service := newMemoryTaskService()
	input := CreateTaskInput{
		IdempotencyKey: "request-a",
		Workflow:       "llm_analysis",
		Input:          json.RawMessage(`{"subject":"go"}`),
		MaxRetries:     3,
	}
	first, err := service.CreateTask(ctx, input)
	if err != nil {
		t.Fatalf("first CreateTask returned error: %v", err)
	}

	input.IdempotencyKey = "REQUEST-A"
	second, err := service.CreateTask(ctx, input)
	if err != nil {
		t.Fatalf("second CreateTask returned error: %v", err)
	}
	if !first.Created || !second.Created || first.Task.ID == second.Task.ID {
		t.Fatalf("expected case-distinct keys to create different tasks: first=%+v second=%+v", first, second)
	}
}

func TestTaskServiceRejectsIdempotencyKeyReuseWithDifferentInput(t *testing.T) {
	ctx := context.Background()
	service := newMemoryTaskService()
	if _, err := service.CreateTask(ctx, CreateTaskInput{
		IdempotencyKey: "memobridge-analysis-1",
		Workflow:       "llm_analysis",
		Input:          json.RawMessage(`{"subject":"go"}`),
		MaxRetries:     3,
	}); err != nil {
		t.Fatalf("first CreateTask returned error: %v", err)
	}

	_, err := service.CreateTask(ctx, CreateTaskInput{
		IdempotencyKey: "memobridge-analysis-1",
		Workflow:       "llm_analysis",
		Input:          json.RawMessage(`{"subject":"database"}`),
		MaxRetries:     3,
	})
	if !errors.Is(err, store.ErrIdempotencyConflict) {
		t.Fatalf("expected ErrIdempotencyConflict, got %v", err)
	}
}

func TestTaskServiceRejectsInvalidInput(t *testing.T) {
	ctx := context.Background()
	service := newMemoryTaskService()

	testCases := []struct {
		name  string
		input CreateTaskInput
	}{
		{
			name: "empty workflow",
			input: CreateTaskInput{
				Input: json.RawMessage(`{}`),
			},
		},
		{
			name: "idempotency key with surrounding whitespace",
			input: CreateTaskInput{
				IdempotencyKey: " request-1 ",
				Workflow:       "llm_analysis",
				Input:          json.RawMessage(`{}`),
			},
		},
		{
			name: "idempotency key exceeds byte limit",
			input: CreateTaskInput{
				IdempotencyKey: string(make([]byte, 129)),
				Workflow:       "llm_analysis",
				Input:          json.RawMessage(`{}`),
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			_, err := service.CreateTask(ctx, testCase.input)
			if !errors.Is(err, ErrInvalidInput) {
				t.Fatalf("expected ErrInvalidInput, got %v", err)
			}
		})
	}
}

func TestTaskServiceListTaskEventsRequiresExistingTask(t *testing.T) {
	ctx := context.Background()
	service := newMemoryTaskService()

	_, err := service.ListTaskEvents(ctx, "missing_task")
	if !errors.Is(err, store.ErrTaskNotFound) {
		t.Fatalf("expected ErrTaskNotFound, got %v", err)
	}
}

func TestTaskServiceCancelsQueuedTaskIdempotently(t *testing.T) {
	ctx := context.Background()
	service := newMemoryTaskService()
	created, err := service.CreateTask(ctx, CreateTaskInput{
		Workflow: "llm_analysis",
		Input:    json.RawMessage(`{"subject":"go"}`),
	})
	if err != nil {
		t.Fatalf("CreateTask returned error: %v", err)
	}

	canceled, err := service.CancelTask(ctx, created.Task.ID)
	if err != nil {
		t.Fatalf("CancelTask returned error: %v", err)
	}
	if canceled.Status != domain.TaskStatusCanceled || canceled.FinishedAt == nil {
		t.Fatalf("unexpected canceled task: %+v", canceled)
	}
	replayed, err := service.CancelTask(ctx, created.Task.ID)
	if err != nil {
		t.Fatalf("replayed CancelTask returned error: %v", err)
	}
	if replayed.ID != canceled.ID || replayed.Version != canceled.Version {
		t.Fatalf("unexpected cancellation replay: %+v", replayed)
	}

	events, err := service.ListTaskEvents(ctx, created.Task.ID)
	if err != nil {
		t.Fatalf("ListTaskEvents returned error: %v", err)
	}
	if len(events) != 2 || events[1].Type != domain.EventTaskCanceled {
		t.Fatalf("unexpected events after repeated cancellation: %+v", events)
	}
}

func TestTaskServiceRejectsRunningTaskCancellation(t *testing.T) {
	ctx := context.Background()
	taskStore := store.NewMemoryTaskStore()
	eventStore := store.NewMemoryEventStore()
	taskCreationStore := store.NewMemoryTaskCreationStore(taskStore, eventStore)
	taskTransitionStore := store.NewMemoryTaskTransitionStore(taskStore, eventStore)
	service := NewTaskService(taskStore, eventStore, taskCreationStore, taskTransitionStore)
	created, err := service.CreateTask(ctx, CreateTaskInput{
		Workflow: "llm_analysis",
		Input:    json.RawMessage(`{"subject":"go"}`),
	})
	if err != nil {
		t.Fatalf("CreateTask returned error: %v", err)
	}
	if _, err := taskTransitionStore.ClaimNextWithEvent(
		ctx,
		store.ClaimOptions{
			WorkerID:      "worker_1",
			Now:           created.Task.CreatedAt.Add(time.Second),
			LeaseDuration: time.Minute,
		},
		"event_started",
	); err != nil {
		t.Fatalf("ClaimNextWithEvent returned error: %v", err)
	}

	if _, err := service.CancelTask(ctx, created.Task.ID); !errors.Is(err, store.ErrTaskNotCancelable) {
		t.Fatalf("expected ErrTaskNotCancelable, got %v", err)
	}
}

func newMemoryTaskService() *TaskService {
	taskStore := store.NewMemoryTaskStore()
	eventStore := store.NewMemoryEventStore()
	taskCreationStore := store.NewMemoryTaskCreationStore(taskStore, eventStore)
	taskTransitionStore := store.NewMemoryTaskTransitionStore(taskStore, eventStore)
	return NewTaskService(taskStore, eventStore, taskCreationStore, taskTransitionStore)
}
