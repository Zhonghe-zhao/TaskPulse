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

func TestWorkerTaskServiceClaimHeartbeatAndComplete(t *testing.T) {
	ctx := context.Background()
	taskService := newMemoryTaskService()
	workerService := NewWorkerTaskService(taskService.taskStore, taskService.taskCancellationStore.(store.TaskTransitionStore))
	created, err := taskService.CreateTask(ctx, CreateTaskInput{
		Workflow:   "llm_analysis",
		Input:      json.RawMessage(`{"subject":"go"}`),
		MaxRetries: 1,
	})
	if err != nil {
		t.Fatalf("CreateTask returned error: %v", err)
	}

	claimed, err := workerService.ClaimTask(ctx, ClaimTaskInput{
		WorkerID:      "external-worker-1",
		LeaseDuration: time.Minute,
	})
	if err != nil {
		t.Fatalf("ClaimTask returned error: %v", err)
	}
	if claimed.ID != created.Task.ID || claimed.Status != domain.TaskStatusRunning {
		t.Fatalf("unexpected claimed task: %+v", claimed)
	}

	heartbeated, err := workerService.HeartbeatTask(ctx, HeartbeatTaskInput{
		TaskID:        claimed.ID,
		WorkerID:      "external-worker-1",
		LeaseDuration: time.Minute,
	})
	if err != nil {
		t.Fatalf("HeartbeatTask returned error: %v", err)
	}
	if heartbeated.Status != domain.TaskStatusRunning {
		t.Fatalf("unexpected heartbeated task: %+v", heartbeated)
	}

	completed, err := workerService.CompleteTask(ctx, CompleteTaskInput{
		TaskID:  claimed.ID,
		WorkerID: "external-worker-1",
		Version: heartbeated.Version,
		Output:  json.RawMessage(`{"summary":"ok"}`),
	})
	if err != nil {
		t.Fatalf("CompleteTask returned error: %v", err)
	}
	if completed.Status != domain.TaskStatusSucceeded || completed.Progress != 100 {
		t.Fatalf("unexpected completed task: %+v", completed)
	}
}

func TestWorkerTaskServiceRejectsStaleCompletion(t *testing.T) {
	ctx := context.Background()
	taskService := newMemoryTaskService()
	workerService := NewWorkerTaskService(taskService.taskStore, taskService.taskCancellationStore.(store.TaskTransitionStore))
	created, err := taskService.CreateTask(ctx, CreateTaskInput{Workflow: "llm_analysis"})
	if err != nil {
		t.Fatalf("CreateTask returned error: %v", err)
	}
	claimed, err := workerService.ClaimTask(ctx, ClaimTaskInput{
		WorkerID:      "external-worker-1",
		LeaseDuration: time.Minute,
	})
	if err != nil {
		t.Fatalf("ClaimTask returned error: %v", err)
	}
	if _, err := workerService.CompleteTask(ctx, CompleteTaskInput{
		TaskID:  created.Task.ID,
		WorkerID: "external-worker-1",
		Version: claimed.Version - 1,
	}); !errors.Is(err, store.ErrTaskConflict) {
		t.Fatalf("expected stale completion conflict, got %v", err)
	}
}
