package application

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/zhaozhonghe/taskpulse/internal/domain"
	"github.com/zhaozhonghe/taskpulse/internal/store"
)

func TestTaskServiceCreateTask(t *testing.T) {
	ctx := context.Background()
	service := newMemoryTaskService()

	task, err := service.CreateTask(ctx, CreateTaskInput{
		Workflow:   "url_check",
		Input:      json.RawMessage(`{"urls":["https://example.com"]}`),
		MaxRetries: 3,
	})
	if err != nil {
		t.Fatalf("CreateTask returned error: %v", err)
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

func TestTaskServiceRejectsInvalidInput(t *testing.T) {
	ctx := context.Background()
	service := newMemoryTaskService()

	_, err := service.CreateTask(ctx, CreateTaskInput{
		Workflow: "",
		Input:    json.RawMessage(`{}`),
	})
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected ErrInvalidInput, got %v", err)
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

func newMemoryTaskService() *TaskService {
	taskStore := store.NewMemoryTaskStore()
	eventStore := store.NewMemoryEventStore()
	taskCreationStore := store.NewMemoryTaskCreationStore(taskStore, eventStore)
	return NewTaskService(taskStore, eventStore, taskCreationStore)
}
