package application

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/zhaozhonghe/taskpulse/internal/domain"
	"github.com/zhaozhonghe/taskpulse/internal/identity"
	"github.com/zhaozhonghe/taskpulse/internal/store"
)

var (
	ErrInvalidInput = errors.New("invalid input")
)

type CreateTaskInput struct {
	IdempotencyKey string
	Workflow       string
	Input          json.RawMessage
	MaxRetries     int
}

type CreateTaskResult struct {
	Task    *domain.Task
	Created bool
}

type TaskService struct {
	taskStore             store.TaskStore
	eventStore            store.EventStore
	taskCreationStore     store.TaskCreationStore
	taskCancellationStore store.TaskCancellationStore
	now                   func() time.Time
}

func NewTaskService(
	taskStore store.TaskStore,
	eventStore store.EventStore,
	taskCreationStore store.TaskCreationStore,
	taskCancellationStore store.TaskCancellationStore,
) *TaskService {
	return &TaskService{
		taskStore:             taskStore,
		eventStore:            eventStore,
		taskCreationStore:     taskCreationStore,
		taskCancellationStore: taskCancellationStore,
		now:                   time.Now,
	}
}

func (s *TaskService) CreateTask(ctx context.Context, input CreateTaskInput) (*CreateTaskResult, error) {
	if input.Workflow == "" {
		return nil, fmt.Errorf("%w: workflow is required", ErrInvalidInput)
	}
	if input.MaxRetries < 0 {
		return nil, fmt.Errorf("%w: max retries cannot be negative", ErrInvalidInput)
	}
	if input.IdempotencyKey != "" {
		if strings.TrimSpace(input.IdempotencyKey) != input.IdempotencyKey {
			return nil, fmt.Errorf("%w: idempotency key cannot have surrounding whitespace", ErrInvalidInput)
		}
		if len(input.IdempotencyKey) > 128 {
			return nil, fmt.Errorf("%w: idempotency key exceeds 128 bytes", ErrInvalidInput)
		}
	}

	now := s.now()
	taskID := identity.New("task")
	eventID := identity.New("event")

	task, err := domain.NewTask(taskID, input.Workflow, input.Input, input.MaxRetries, now)
	if err != nil {
		return nil, err
	}
	task.IdempotencyKey = input.IdempotencyKey

	event, err := domain.NewTaskEvent(
		eventID,
		task.ID,
		domain.EventTaskCreated,
		"task created",
		json.RawMessage("{}"),
		task.Progress,
		now,
	)
	if err != nil {
		return nil, err
	}

	creationResult, err := s.taskCreationStore.CreateTaskWithEvent(ctx, task, event)
	if err != nil {
		return nil, err
	}

	return &CreateTaskResult{
		Task:    creationResult.Task,
		Created: creationResult.Created,
	}, nil
}

func (s *TaskService) GetTask(ctx context.Context, taskID string) (*domain.Task, error) {
	if taskID == "" {
		return nil, fmt.Errorf("%w: task id is required", ErrInvalidInput)
	}

	return s.taskStore.Get(ctx, taskID)
}

func (s *TaskService) CancelTask(ctx context.Context, taskID string) (*domain.Task, error) {
	if taskID == "" {
		return nil, fmt.Errorf("%w: task id is required", ErrInvalidInput)
	}

	result, err := s.taskCancellationStore.CancelTaskWithEvent(
		ctx,
		taskID,
		identity.New("event"),
		s.now(),
	)
	if err != nil {
		return nil, err
	}
	return result.Task, nil
}

func (s *TaskService) ListTaskEvents(ctx context.Context, taskID string) ([]*domain.TaskEvent, error) {
	if taskID == "" {
		return nil, fmt.Errorf("%w: task id is required", ErrInvalidInput)
	}

	if _, err := s.taskStore.Get(ctx, taskID); err != nil {
		return nil, err
	}

	return s.eventStore.ListByTaskID(ctx, taskID)
}
