package store

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"github.com/zhaozhonghe/taskpulse/internal/domain"
)

type MemoryTaskStore struct {
	mu    sync.RWMutex
	tasks map[string]*domain.Task
}

func NewMemoryTaskStore() *MemoryTaskStore {
	return &MemoryTaskStore{
		tasks: make(map[string]*domain.Task),
	}
}

func (s *MemoryTaskStore) Create(ctx context.Context, task *domain.Task) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if task == nil {
		return ErrNilTask
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.tasks[task.ID]; exists {
		return ErrTaskAlreadyExists
	}

	s.tasks[task.ID] = cloneTask(task)
	return nil
}

func (s *MemoryTaskStore) Get(ctx context.Context, id string) (*domain.Task, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	task, exists := s.tasks[id]
	if !exists {
		return nil, ErrTaskNotFound
	}

	return cloneTask(task), nil
}

func (s *MemoryTaskStore) Update(ctx context.Context, task *domain.Task) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if task == nil {
		return ErrNilTask
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	stored, exists := s.tasks[task.ID]
	if !exists {
		return ErrTaskNotFound
	}
	if stored.Version != task.Version {
		return ErrTaskConflict
	}

	updated := cloneTask(task)
	updated.Version++
	s.tasks[task.ID] = updated
	return nil
}

func (s *MemoryTaskStore) ClaimNext(ctx context.Context, options ClaimOptions) (*domain.Task, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := options.Validate(); err != nil {
		return nil, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	var selected *domain.Task
	recovering := false
	for _, task := range s.tasks {
		if task.Status != domain.TaskStatusRunning ||
			task.LeaseExpiresAt == nil ||
			task.LeaseExpiresAt.After(options.Now) ||
			task.RetryCount >= task.MaxRetries {
			continue
		}
		if selected == nil ||
			task.LeaseExpiresAt.Before(*selected.LeaseExpiresAt) ||
			(task.LeaseExpiresAt.Equal(*selected.LeaseExpiresAt) && task.ID < selected.ID) {
			selected = task
			recovering = true
		}
	}

	if selected == nil {
		for _, task := range s.tasks {
			if task.Status != domain.TaskStatusQueued {
				continue
			}
			//1.还没选过任务 2.最早创建的 queued 任务 3. 创建时间一样，用ID 最小的 queued 任务
			if selected == nil || task.CreatedAt.Before(selected.CreatedAt) ||
				(task.CreatedAt.Equal(selected.CreatedAt) && task.ID < selected.ID) {
				selected = task
			}
		}
	}
	if selected == nil {
		return nil, ErrNoTaskAvailable
	}
	if recovering {
		selected.RetryCount++
		selected.UpdatedAt = options.Now
	} else {
		if err := selected.MoveTo(domain.TaskStatusRunning, options.Now); err != nil {
			return nil, err
		}
	}
	leaseExpiresAt := options.Now.Add(options.LeaseDuration)
	selected.LeaseOwner = options.WorkerID
	selected.LeaseExpiresAt = &leaseExpiresAt
	selected.Version++

	return cloneTask(selected), nil
}

func (s *MemoryTaskStore) RenewLease(ctx context.Context, options RenewLeaseOptions) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := options.Validate(); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	task, exists := s.tasks[options.TaskID]
	if !exists ||
		task.Status != domain.TaskStatusRunning ||
		task.LeaseOwner != options.WorkerID ||
		task.LeaseExpiresAt == nil ||
		!task.LeaseExpiresAt.After(options.Now) {
		return ErrLeaseLost
	}

	leaseExpiresAt := options.Now.Add(options.LeaseDuration)
	task.LeaseExpiresAt = &leaseExpiresAt
	task.UpdatedAt = options.Now
	return nil
}

func (s *MemoryTaskStore) FailNextExpired(ctx context.Context, now time.Time) (*domain.Task, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if now.IsZero() {
		return nil, ErrInvalidCleanupTime
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	var selected *domain.Task
	for _, task := range s.tasks {
		if task.Status != domain.TaskStatusRunning ||
			task.LeaseExpiresAt == nil ||
			task.LeaseExpiresAt.After(now) ||
			task.RetryCount < task.MaxRetries {
			continue
		}
		if selected == nil ||
			task.LeaseExpiresAt.Before(*selected.LeaseExpiresAt) ||
			(task.LeaseExpiresAt.Equal(*selected.LeaseExpiresAt) && task.ID < selected.ID) {
			selected = task
		}
	}
	if selected == nil {
		return nil, ErrNoExpiredTask
	}

	selected.ErrorMessage = "task lease expired and retry budget exhausted"
	if err := selected.MoveTo(domain.TaskStatusFailed, now); err != nil {
		return nil, err
	}
	selected.Version++
	return cloneTask(selected), nil
}

func cloneTask(task *domain.Task) *domain.Task {
	if task == nil {
		return nil
	}

	copied := *task
	copied.Input = cloneRawMessage(task.Input)
	copied.Result = cloneRawMessage(task.Result)
	if task.StartedAt != nil {
		startedAt := *task.StartedAt
		copied.StartedAt = &startedAt
	}
	if task.FinishedAt != nil {
		finishedAt := *task.FinishedAt
		copied.FinishedAt = &finishedAt
	}
	if task.LeaseExpiresAt != nil {
		leaseExpiresAt := *task.LeaseExpiresAt
		copied.LeaseExpiresAt = &leaseExpiresAt
	}

	return &copied
}

func cloneRawMessage(raw json.RawMessage) json.RawMessage {
	if raw == nil {
		return nil
	}
	return append(json.RawMessage(nil), raw...)
}
