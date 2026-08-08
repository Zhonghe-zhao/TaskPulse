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

	progressed, err := workerService.ReportProgress(ctx, ReportProgressInput{
		TaskID:   claimed.ID,
		WorkerID: "external-worker-1",
		Version:  heartbeated.Version,
		Progress: 50,
		Message:  "calling llm provider",
	})
	if err != nil {
		t.Fatalf("ReportProgress returned error: %v", err)
	}
	if progressed.Progress != 50 || progressed.Version <= heartbeated.Version {
		t.Fatalf("unexpected progressed task: %+v", progressed)
	}

	completed, err := workerService.CompleteTask(ctx, CompleteTaskInput{
		TaskID:   claimed.ID,
		WorkerID: "external-worker-1",
		Version:  progressed.Version,
		Output:   json.RawMessage(`{"summary":"ok"}`),
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
		TaskID:   created.Task.ID,
		WorkerID: "external-worker-1",
		Version:  claimed.Version - 1,
	}); !errors.Is(err, store.ErrTaskConflict) {
		t.Fatalf("expected stale completion conflict, got %v", err)
	}
}

func TestWorkerTaskServiceLeaseTokenAndCompleteReplay(t *testing.T) {
	ctx := context.Background()
	taskService := newMemoryTaskService()
	workerService := NewWorkerTaskService(taskService.taskStore, taskService.taskCancellationStore.(store.TaskTransitionStore))
	created, err := taskService.CreateTask(ctx, CreateTaskInput{Workflow: "memobridge.semantic_profile"})
	if err != nil {
		t.Fatalf("CreateTask returned error: %v", err)
	}
	claimed, err := workerService.ClaimTask(ctx, ClaimTaskInput{
		WorkerID:      "memobridge-worker-1",
		Workflow:      "memobridge.semantic_profile",
		LeaseDuration: time.Minute,
	})
	if err != nil {
		t.Fatalf("ClaimTask returned error: %v", err)
	}
	if claimed.TaskID != created.Task.ID || claimed.LeaseToken == "" || claimed.LeaseUntil == nil {
		t.Fatalf("claim did not return external lease fields: %+v", claimed)
	}

	completed, err := workerService.CompleteTask(ctx, CompleteTaskInput{
		TaskID:     claimed.ID,
		WorkerID:   "memobridge-worker-1",
		LeaseToken: claimed.LeaseToken,
		ResultRef:  json.RawMessage(`{"source_item_id":11778,"content_hash":"sha256:test"}`),
	})
	if err != nil {
		t.Fatalf("CompleteTask returned error: %v", err)
	}
	replayed, err := workerService.CompleteTask(ctx, CompleteTaskInput{
		TaskID:     claimed.ID,
		WorkerID:   "memobridge-worker-1",
		LeaseToken: claimed.LeaseToken,
		Output:     json.RawMessage(`{"different":true}`),
	})
	if err != nil {
		t.Fatalf("duplicate CompleteTask returned error: %v", err)
	}
	if replayed.ID != completed.ID || replayed.Status != domain.TaskStatusSucceeded {
		t.Fatalf("unexpected replayed task: %+v", replayed)
	}
	if string(replayed.Result) != `{"source_item_id":11778,"content_hash":"sha256:test"}` {
		t.Fatalf("result_ref was not persisted as task reference: %s", replayed.Result)
	}
}
