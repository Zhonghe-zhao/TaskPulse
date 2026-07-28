package store

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/zhaozhonghe/taskpulse/internal/domain"
)

func TestMemoryTaskCreationStoreCreatesTaskAndEvent(t *testing.T) {
	ctx := context.Background()
	taskStore := NewMemoryTaskStore()
	eventStore := NewMemoryEventStore()
	creator := NewMemoryTaskCreationStore(taskStore, eventStore)
	task, event := newTaskCreationPair(t, "task_1", "event_1")

	if err := creator.CreateTaskWithEvent(ctx, task, event); err != nil {
		t.Fatalf("CreateTaskWithEvent returned error: %v", err)
	}
	if _, err := taskStore.Get(ctx, task.ID); err != nil {
		t.Fatalf("Get task returned error: %v", err)
	}
	events, err := eventStore.ListByTaskID(ctx, task.ID)
	if err != nil {
		t.Fatalf("ListByTaskID returned error: %v", err)
	}
	if len(events) != 1 || events[0].ID != event.ID {
		t.Fatalf("unexpected events: %+v", events)
	}
}

func TestMemoryTaskCreationStoreDoesNotCreateTaskWhenEventConflicts(t *testing.T) {
	ctx := context.Background()
	taskStore := NewMemoryTaskStore()
	eventStore := NewMemoryEventStore()
	creator := NewMemoryTaskCreationStore(taskStore, eventStore)
	task, event := newTaskCreationPair(t, "task_1", "event_1")

	if err := eventStore.Append(ctx, event); err != nil {
		t.Fatalf("seed event Append returned error: %v", err)
	}
	if err := creator.CreateTaskWithEvent(ctx, task, event); !errors.Is(err, ErrEventAlreadyExists) {
		t.Fatalf("expected ErrEventAlreadyExists, got %v", err)
	}
	if _, err := taskStore.Get(ctx, task.ID); !errors.Is(err, ErrTaskNotFound) {
		t.Fatalf("task was created despite event conflict: %v", err)
	}
}

func newTaskCreationPair(t *testing.T, taskID, eventID string) (*domain.Task, *domain.TaskEvent) {
	t.Helper()
	now := time.Date(2026, 7, 27, 12, 0, 0, 0, time.UTC)
	task, err := domain.NewTask(
		taskID,
		"url_check",
		json.RawMessage(`{"urls":["https://example.com"]}`),
		3,
		now,
	)
	if err != nil {
		t.Fatalf("NewTask returned error: %v", err)
	}
	event, err := domain.NewTaskEvent(
		eventID,
		task.ID,
		domain.EventTaskCreated,
		"task created",
		nil,
		0,
		now,
	)
	if err != nil {
		t.Fatalf("NewTaskEvent returned error: %v", err)
	}
	return task, event
}
