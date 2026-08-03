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
	ErrInvalidWorkerRequest = errors.New("invalid worker request")
	ErrWorkerTaskNotOwned   = errors.New("worker does not own task")
)

type WorkerTaskService struct {
	taskStore       store.TaskStore
	transitionStore store.TaskTransitionStore
	now             func() time.Time
}

type ClaimTaskInput struct {
	WorkerID      string
	LeaseDuration time.Duration
}

type HeartbeatTaskInput struct {
	TaskID        string
	WorkerID      string
	LeaseDuration time.Duration
}

type CompleteTaskInput struct {
	TaskID  string
	WorkerID string
	Version uint64
	Output  json.RawMessage
}

type FailTaskInput struct {
	TaskID      string
	WorkerID    string
	Version     uint64
	ErrorCode   string
	ErrorMessage string
}

func NewWorkerTaskService(
	taskStore store.TaskStore,
	transitionStore store.TaskTransitionStore,
) *WorkerTaskService {
	return &WorkerTaskService{
		taskStore:       taskStore,
		transitionStore: transitionStore,
		now:             time.Now,
	}
}

func (s *WorkerTaskService) ClaimTask(
	ctx context.Context,
	input ClaimTaskInput,
) (*domain.Task, error) {
	if strings.TrimSpace(input.WorkerID) == "" || input.LeaseDuration <= 0 {
		return nil, ErrInvalidWorkerRequest
	}
	return s.transitionStore.ClaimNextWithEvent(
		ctx,
		store.ClaimOptions{
			WorkerID:      input.WorkerID,
			Now:           s.now(),
			LeaseDuration: input.LeaseDuration,
		},
		identity.New("event"),
	)
}

func (s *WorkerTaskService) HeartbeatTask(
	ctx context.Context,
	input HeartbeatTaskInput,
) (*domain.Task, error) {
	if input.TaskID == "" || strings.TrimSpace(input.WorkerID) == "" || input.LeaseDuration <= 0 {
		return nil, ErrInvalidWorkerRequest
	}
	now := s.now()
	if err := s.taskStore.RenewLease(ctx, store.RenewLeaseOptions{
		TaskID:        input.TaskID,
		WorkerID:      input.WorkerID,
		Now:           now,
		LeaseDuration: input.LeaseDuration,
	}); err != nil {
		return nil, err
	}
	return s.taskStore.Get(ctx, input.TaskID)
}

func (s *WorkerTaskService) CompleteTask(
	ctx context.Context,
	input CompleteTaskInput,
) (*domain.Task, error) {
	task, err := s.prepareRunningTask(ctx, input.TaskID, input.WorkerID, input.Version)
	if err != nil {
		return nil, err
	}
	if len(input.Output) == 0 {
		input.Output = json.RawMessage("{}")
	}
	task.Result = append(json.RawMessage(nil), input.Output...)
	task.Progress = 100
	now := s.now()
	if err := task.MoveTo(domain.TaskStatusSucceeded, now); err != nil {
		return nil, err
	}
	event, err := domain.NewTaskEvent(
		identity.New("event"),
		task.ID,
		domain.EventTaskSucceeded,
		"task succeeded by external worker",
		json.RawMessage("{}"),
		task.Progress,
		now,
	)
	if err != nil {
		return nil, err
	}
	if err := s.transitionStore.UpdateTaskWithEvent(ctx, task, event); err != nil {
		return nil, err
	}
	return task, nil
}

func (s *WorkerTaskService) FailTask(
	ctx context.Context,
	input FailTaskInput,
) (*domain.Task, error) {
	task, err := s.prepareRunningTask(ctx, input.TaskID, input.WorkerID, input.Version)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(input.ErrorCode) == "" || strings.TrimSpace(input.ErrorMessage) == "" {
		return nil, fmt.Errorf("%w: error_code and error_message are required", ErrInvalidWorkerRequest)
	}
	task.ErrorMessage = input.ErrorCode + ": " + input.ErrorMessage
	now := s.now()
	if err := task.MoveTo(domain.TaskStatusFailed, now); err != nil {
		return nil, err
	}
	payload, err := json.Marshal(map[string]string{
		"error_code": input.ErrorCode,
	})
	if err != nil {
		return nil, err
	}
	event, err := domain.NewTaskEvent(
		identity.New("event"),
		task.ID,
		domain.EventTaskFailed,
		"task failed by external worker",
		payload,
		task.Progress,
		now,
	)
	if err != nil {
		return nil, err
	}
	if err := s.transitionStore.UpdateTaskWithEvent(ctx, task, event); err != nil {
		return nil, err
	}
	return task, nil
}

func (s *WorkerTaskService) prepareRunningTask(
	ctx context.Context,
	taskID string,
	workerID string,
	version uint64,
) (*domain.Task, error) {
	if taskID == "" || strings.TrimSpace(workerID) == "" {
		return nil, ErrInvalidWorkerRequest
	}
	task, err := s.taskStore.Get(ctx, taskID)
	if err != nil {
		return nil, err
	}
	now := s.now()
	if task.Status != domain.TaskStatusRunning ||
		task.LeaseOwner != workerID ||
		task.LeaseExpiresAt == nil ||
		!task.LeaseExpiresAt.After(now) {
		return nil, store.ErrLeaseLost
	}
	if task.Version != version {
		return nil, store.ErrTaskConflict
	}
	return task, nil
}
