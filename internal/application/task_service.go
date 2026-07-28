package application

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

var (
	ErrInvalidInput = errors.New("invalid input")
)

type CreateTaskInput struct {
	Workflow   string
	Input      json.RawMessage
	MaxRetries int
}

type TaskService struct {
	taskStore         store.TaskStore
	eventStore        store.EventStore
	taskCreationStore store.TaskCreationStore
	now               func() time.Time
}

func NewTaskService(
	taskStore store.TaskStore,
	eventStore store.EventStore,
	taskCreationStore store.TaskCreationStore,
) *TaskService {
	return &TaskService{
		taskStore:         taskStore,
		eventStore:        eventStore,
		taskCreationStore: taskCreationStore,
		now:               time.Now,
	}
}

func (s *TaskService) CreateTask(ctx context.Context, input CreateTaskInput) (*domain.Task, error) {
	if input.Workflow == "" {
		return nil, fmt.Errorf("%w: workflow is required", ErrInvalidInput)
	}
	if input.MaxRetries < 0 {
		return nil, fmt.Errorf("%w: max retries cannot be negative", ErrInvalidInput)
	}

	now := s.now()
	taskID := identity.New("task")
	eventID := identity.New("event")

	task, err := domain.NewTask(taskID, input.Workflow, input.Input, input.MaxRetries, now)
	if err != nil {
		return nil, err
	}

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

	if err := s.taskCreationStore.CreateTaskWithEvent(ctx, task, event); err != nil {
		return nil, err
	}

	return task, nil
}

func (s *TaskService) GetTask(ctx context.Context, taskID string) (*domain.Task, error) {
	if taskID == "" {
		return nil, fmt.Errorf("%w: task id is required", ErrInvalidInput)
	}

	return s.taskStore.Get(ctx, taskID)
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
