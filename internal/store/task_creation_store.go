package store

import (
	"context"
	"errors"

	"github.com/zhaozhonghe/taskpulse/internal/domain"
)

var ErrTaskEventMismatch = errors.New("task event does not belong to task")

type TaskCreationStore interface {
	CreateTaskWithEvent(ctx context.Context, task *domain.Task, event *domain.TaskEvent) error
}

type MemoryTaskCreationStore struct {
	taskStore  *MemoryTaskStore
	eventStore *MemoryEventStore
}

var _ TaskCreationStore = (*MemoryTaskCreationStore)(nil)

func NewMemoryTaskCreationStore(
	taskStore *MemoryTaskStore,
	eventStore *MemoryEventStore,
) *MemoryTaskCreationStore {
	return &MemoryTaskCreationStore{
		taskStore:  taskStore,
		eventStore: eventStore,
	}
}

func (s *MemoryTaskCreationStore) CreateTaskWithEvent(
	ctx context.Context,
	task *domain.Task,
	event *domain.TaskEvent,
) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if task == nil {
		return ErrNilTask
	}
	if event == nil {
		return ErrNilEvent
	}
	if event.TaskID != task.ID {
		return ErrTaskEventMismatch
	}

	s.taskStore.mu.Lock()
	defer s.taskStore.mu.Unlock()
	s.eventStore.mu.Lock()
	defer s.eventStore.mu.Unlock()

	if _, exists := s.taskStore.tasks[task.ID]; exists {
		return ErrTaskAlreadyExists
	}
	if _, exists := s.eventStore.eventsByID[event.ID]; exists {
		return ErrEventAlreadyExists
	}

	copiedTask := cloneTask(task)
	copiedEvent := cloneTaskEvent(event)
	s.taskStore.tasks[task.ID] = copiedTask
	s.eventStore.eventsByID[event.ID] = copiedEvent
	s.eventStore.eventsByTaskID[event.TaskID] = append(
		s.eventStore.eventsByTaskID[event.TaskID],
		copiedEvent,
	)
	return nil
}
